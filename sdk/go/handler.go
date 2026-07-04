// Package ledgerly is the Go SDK for the ledgerly audit-log API (issue
// #26, ADR-0001): a slog.Handler that tees an app's existing logging to
// its current handler unconditionally, evaluates a locally-cached trigger
// rule set against every eligible record, and ships matches into
// ledgerly's async ingest path.
package ledgerly

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ErrEmptyFallback is returned by NewHandler when fallback has no rules.
// The constructor requires a compiled-in fallback rule-set (ADR-0001
// amendments): neither fail-open (an evidence gap at boot) nor
// fail-closed (unbounded capture) is acceptable, and an empty fallback is
// indistinguishable from having none at all.
var ErrEmptyFallback = errors.New("ledgerly: NewHandler requires a non-empty fallback rule-set")

// ErrMissingBufferDir is returned by NewHandler when WithBufferDir was not
// supplied. The disk buffer that makes delivery crash-safe (spill on
// failure, replay in order on restart) has nowhere to live without one.
var ErrMissingBufferDir = errors.New("ledgerly: NewHandler requires WithBufferDir")

// ErrNilNext is returned by NewHandler when next is nil: the SDK always
// tees to the app's existing handler, so there is nothing to wrap.
var ErrNilNext = errors.New("ledgerly: NewHandler requires a non-nil next handler")

// config holds every option a Handler is constructed with.
type config struct {
	eventsURL       string
	rulesURL        string
	apiKey          string
	refreshInterval time.Duration
	bufferDir       string
	httpClient      *http.Client
	queueSize       int
	baseBackoff     time.Duration
	maxBackoff      time.Duration
}

func defaultConfig() config {
	return config{
		refreshInterval: time.Minute,
		queueSize:       1000,
		baseBackoff:     500 * time.Millisecond,
		maxBackoff:      30 * time.Second,
	}
}

// Option configures a Handler constructed by NewHandler.
type Option func(*config)

// WithEventsURL sets the POST /v1/events endpoint. Required for delivery;
// a Handler built without it cannot ship any captured event.
func WithEventsURL(url string) Option { return func(c *config) { c.eventsURL = url } }

// WithRulesURL sets the GET /v1/rules endpoint the poller fetches from.
func WithRulesURL(url string) Option { return func(c *config) { c.rulesURL = url } }

// WithAPIKey sets the bearer token sent with every server request.
func WithAPIKey(key string) Option { return func(c *config) { c.apiKey = key } }

// WithRefreshInterval sets how often the poller re-fetches GET /v1/rules.
func WithRefreshInterval(d time.Duration) Option {
	return func(c *config) { c.refreshInterval = d }
}

// WithBufferDir sets the local disk buffer directory. Required — NewHandler
// returns ErrMissingBufferDir without it.
func WithBufferDir(dir string) Option { return func(c *config) { c.bufferDir = dir } }

// WithHTTPClient overrides the default *http.Client used for both delivery
// and rule polling.
func WithHTTPClient(hc *http.Client) Option { return func(c *config) { c.httpClient = hc } }

// WithQueueSize overrides the default bounded delivery queue size.
func WithQueueSize(n int) Option { return func(c *config) { c.queueSize = n } }

// WithBackoff overrides the default base/max retry backoff.
func WithBackoff(base, max time.Duration) Option {
	return func(c *config) { c.baseBackoff = base; c.maxBackoff = max }
}

// handlerState is the mutable core shared by a Handler and every clone
// produced by WithAttrs/WithGroup, so a rule-set swap or delivery-queue
// state is visible from any of them.
type handlerState struct {
	cfg        config
	instanceID string
	startedAt  time.Time

	activeRules atomic.Pointer[[]Rule]

	seq    *sequencer
	buf    *buffer
	client *apiClient
	sender *sender
	poller *poller
	queue  chan []byte

	// captureAttempts counts Handle() calls that reached the capture
	// pipeline (i.e. were NOT self-suppressed). Exported to tests only via
	// the unexported captureAttempts() accessor, to pin the self-suppression
	// guards independent of whatever capture ends up doing.
	captureAttempts int64
}

// Handler is a slog.Handler implementing the ledgerly SDK pipeline. See
// the package doc and ADR-0001.
type Handler struct {
	shared *handlerState
	next   slog.Handler
	groups []string
	attrs  []slog.Attr
}

// NewHandler constructs a Handler wrapping next. fallback must be
// non-empty and next must be non-nil, or construction fails outright — see
// ErrEmptyFallback / ErrNilNext. WithBufferDir must be supplied, or
// ErrMissingBufferDir. Successful construction records a chained
// sdk.started event (regime=fallback) once the delivery pipeline is
// wired up.
func NewHandler(fallback []Rule, next slog.Handler, opts ...Option) (*Handler, error) {
	if len(fallback) == 0 {
		return nil, ErrEmptyFallback
	}
	if next == nil {
		return nil, ErrNilNext
	}

	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.bufferDir == "" {
		return nil, ErrMissingBufferDir
	}

	shared := &handlerState{
		cfg:        cfg,
		instanceID: uuid.NewString(),
		startedAt:  time.Now().UTC(),
		queue:      make(chan []byte, cfg.queueSize),
	}
	active := append([]Rule{}, fallback...)
	shared.activeRules.Store(&active)

	shared.client = newAPIClient(cfg.eventsURL, cfg.rulesURL, cfg.apiKey, cfg.httpClient)

	seq, err := openSequencer(cfg.bufferDir, defaultSeqBlockSize)
	if err != nil {
		return nil, fmt.Errorf("ledgerly: opening sequence counter: %w", err)
	}
	shared.seq = seq

	buf, err := openBuffer(cfg.bufferDir)
	if err != nil {
		return nil, fmt.Errorf("ledgerly: opening disk buffer: %w", err)
	}
	shared.buf = buf

	shared.sender = newSender(shared.client, shared.buf, shared.queue, cfg.baseBackoff, cfg.maxBackoff)
	shared.poller = newPoller(shared.client, cfg.refreshInterval, func(rl RuleList, ev regimeEvent) {
		rules := append([]Rule{}, rl.Rules...)
		shared.activeRules.Store(&rules)
	})

	return &Handler{shared: shared, next: next}, nil
}

// defaultSeqBlockSize is the sequence-counter reservation block size used
// when a Handler doesn't override it.
const defaultSeqBlockSize = 256

// Enabled reports whether level should be handled: true if next would
// handle it, OR if the active rule set contains a level-at-least threshold
// this level satisfies — a rule can ask for more evidence than the host
// app's own configured log level admits.
//
// STUB: rule-driven widening is not implemented yet; this only defers to
// next, so a rule wanting a level next doesn't emit is invisible.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

// Handle tees r to next unconditionally (ADR-0001: non-audit logs still
// reach the app's normal handler), then — unless r is self-suppressed —
// attempts to capture it as an audit event. Capture failures never
// propagate to the app; only next's error does.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	err := h.next.Handle(ctx, r)

	if isInternal(r) {
		return err
	}
	atomic.AddInt64(&h.shared.captureAttempts, 1)
	_ = h.capture(ctx, r)

	return err
}

// captureAttempts reports how many Handle() calls reached the capture
// pipeline (i.e. were not self-suppressed). Test-only accessor.
func (h *Handler) captureAttempts() int64 {
	return atomic.LoadInt64(&h.shared.captureAttempts)
}

// capture stamps, matches, and (on a match) enqueues r for delivery.
//
// STUB: not implemented.
func (h *Handler) capture(ctx context.Context, r slog.Record) error {
	return ErrNotImplemented
}

// logInternal routes an SDK-internal diagnostic record directly to next,
// bypassing Handle() (and therefore capture()) entirely — guard #1 of the
// two-guard self-suppression design (ADR-0001 amendments). It tags the
// record with the reserved internalAttr so that even a miswired caller
// that routes it back through Handle() hits guard #2 instead of
// recursing.
func (h *Handler) logInternal(ctx context.Context, level slog.Level, msg string, args ...any) {
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.Add(args...)
	r.AddAttrs(slog.Bool(internalAttr, true))
	_ = h.next.Handle(ctx, r)
}

// WithAttrs returns a new Handler sharing this one's state, whose next
// carries the given attrs.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return &Handler{
		shared: h.shared,
		next:   h.next.WithAttrs(attrs),
		groups: h.groups,
		attrs:  append(append([]slog.Attr{}, h.attrs...), attrs...),
	}
}

// WithGroup returns a new Handler sharing this one's state, whose next
// opens the given group.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{
		shared: h.shared,
		next:   h.next.WithGroup(name),
		groups: append(append([]string{}, h.groups...), name),
		attrs:  h.attrs,
	}
}

// Close stops the poller, drains in-flight delivery, and checkpoints the
// sequence counter and buffer cursor so a subsequent NewHandler on the
// same buffer dir resumes cleanly.
//
// STUB: not implemented.
func (h *Handler) Close(ctx context.Context) error {
	return ErrNotImplemented
}
