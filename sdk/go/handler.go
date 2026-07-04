// Package ledgerly is the Go SDK for the ledgerly audit-log API (issue
// #26, ADR-0001): a slog.Handler that tees an app's existing logging to
// its current handler unconditionally, evaluates a locally-cached trigger
// rule set against every eligible record, and ships matches into
// ledgerly's async ingest path.
package ledgerly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
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
	insecureHTTP    bool
	asyncFirstFetch bool
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

// WithInsecureHTTP allows plain-http events/rules URLs to non-loopback
// hosts. Without it, NewHandler rejects any http:// endpoint except
// localhost/loopback (the dev flow): audit events carry an API key and
// sensitive payloads, so cleartext transport must be an explicit opt-in.
func WithInsecureHTTP() Option { return func(c *config) { c.insecureHTTP = true } }

// WithAsyncFirstFetch moves the first rules fetch off NewHandler's
// critical path onto the polling goroutine, for latency-sensitive callers
// that would rather start on the compiled-in fallback than wait up to the
// first-fetch timeout. By default the first fetch is synchronous: the
// server's rules are already active when NewHandler returns.
func WithAsyncFirstFetch() Option { return func(c *config) { c.asyncFirstFetch = true } }

// validateEndpointURL enforces the transport scheme for a configured
// endpoint: https always passes; http passes only for loopback hosts or
// with the WithInsecureHTTP opt-in; anything else is rejected.
func validateEndpointURL(name, raw string, allowInsecure bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("ledgerly: invalid %s URL %q: %w", name, raw, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure || isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("ledgerly: %s URL %q uses plain http to a non-loopback host; use https, or opt in explicitly with WithInsecureHTTP()", name, raw)
	default:
		return fmt.Errorf("ledgerly: %s URL %q must use https (or http via WithInsecureHTTP()), got scheme %q", name, raw, u.Scheme)
	}
}

// isLoopbackHost reports whether host is localhost or a loopback IP.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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

	// cancel tears down the sender/poller lifecycle context; closeOnce
	// makes Close idempotent.
	cancel    context.CancelFunc
	closeOnce sync.Once

	// mu guards closed and orders enqueues against Close closing the
	// queue: enqueuers hold the read side, Close the write side, so a
	// send on a closed channel is impossible.
	mu     sync.RWMutex
	closed bool

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
// ErrMissingBufferDir. When a rules endpoint is configured, construction
// records a chained sdk.started event (regime=fallback), then performs one
// synchronous rules fetch bounded by firstFetchTimeout — a successful
// fetch swaps the active rule set and records sdk.rules-activated before
// NewHandler returns. WithAsyncFirstFetch moves that first fetch onto the
// polling goroutine instead, keeping construction off the network.
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
	if cfg.eventsURL != "" {
		if err := validateEndpointURL("events", cfg.eventsURL, cfg.insecureHTTP); err != nil {
			return nil, err
		}
	}
	if cfg.rulesURL != "" {
		if err := validateEndpointURL("rules", cfg.rulesURL, cfg.insecureHTTP); err != nil {
			return nil, err
		}
	}

	shared := &handlerState{
		cfg:       cfg,
		startedAt: time.Now().UTC(),
		queue:     make(chan []byte, cfg.queueSize),
	}
	active := append([]Rule{}, fallback...)
	shared.activeRules.Store(&active)

	shared.client = newAPIClient(cfg.eventsURL, cfg.rulesURL, cfg.apiKey, cfg.httpClient)

	seq, err := openSequencer(cfg.bufferDir, defaultSeqBlockSize)
	if err != nil {
		return nil, fmt.Errorf("ledgerly: opening sequence counter: %w", err)
	}
	shared.seq = seq
	shared.instanceID = seq.instanceID

	buf, err := openBuffer(cfg.bufferDir)
	if err != nil {
		return nil, fmt.Errorf("ledgerly: opening disk buffer: %w", err)
	}
	shared.buf = buf

	shared.sender = newSender(shared.client, shared.buf, shared.queue, cfg.baseBackoff, cfg.maxBackoff)

	h := &Handler{shared: shared, next: next}

	shared.poller = newPoller(shared.client, cfg.refreshInterval, cfg.asyncFirstFetch, func(rl RuleList, ev regimeEvent) {
		rules := append([]Rule{}, rl.Rules...)
		shared.activeRules.Store(&rules)
		if ev.Action != "" {
			ev.FallbackDuration = time.Since(shared.startedAt)
			h.emitRegime(context.Background(), ev)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	shared.cancel = cancel
	go shared.sender.run(ctx)

	// A handler with no rules endpoint stays on its compiled-in fallback
	// for life; regime transitions only exist (and are only recorded)
	// when a poller can move the handler off fallback.
	if cfg.rulesURL != "" {
		h.emitRegime(context.Background(), regimeEvent{Action: ActionSDKStarted})
	}
	shared.poller.start(ctx)

	return h, nil
}

// defaultSeqBlockSize is the sequence-counter reservation block size used
// when a Handler doesn't override it.
const defaultSeqBlockSize = 256

// Enabled reports whether level should be handled: true if next would
// handle it, OR if the active rule set contains a level-at-least threshold
// this level satisfies — a rule can ask for more evidence than the host
// app's own configured log level admits.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.next.Enabled(ctx, level) {
		return true
	}
	for _, r := range *h.shared.activeRules.Load() {
		if r.LevelAtLeast == "" {
			// No threshold: the rule wants every level of its event-type.
			return true
		}
		if rank, ok := levelRank[r.LevelAtLeast]; ok && int(level) >= rank {
			return true
		}
	}
	return false
}

// Handle tees r to next unconditionally (ADR-0001: non-audit logs still
// reach the app's normal handler), then attempts to capture it as an audit
// event. Every record entering Handle reaches the capture pipeline — there
// is deliberately no attr-based suppression signal available to callers
// (a public opt-out would be an unauthenticated audit-evasion switch);
// SDK-internal diagnostics avoid capture only by never entering Handle at
// all (logInternal writes directly to next). Capture failures never
// propagate to the app; only next's error does.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	err := h.next.Handle(ctx, r)

	atomic.AddInt64(&h.shared.captureAttempts, 1)
	_ = h.capture(ctx, r)

	return err
}

// captureAttempts reports how many Handle() calls reached the capture
// pipeline (i.e. were not self-suppressed). Test-only accessor.
func (h *Handler) captureAttempts() int64 {
	return atomic.LoadInt64(&h.shared.captureAttempts)
}

// capture stamps, matches, and (on a match) enqueues r for delivery. The
// only synchronous I/O on this path is the sequence allocation, which is
// memory-only except for its rare, bounded block-refill fallback; the
// enqueue itself never blocks (a full queue drops the record, its burned
// sequence number leaving a chain-evidenced gap).
func (h *Handler) capture(ctx context.Context, r slog.Record) error {
	_ = ctx

	cr := flattenForHandler(r, h.groups, h.attrs)
	if cr.EventType == "" {
		return nil
	}

	matched, _ := Match(*h.shared.activeRules.Load(), cr.matchInput())
	if !matched {
		return nil
	}

	// Burn the sequence number synchronously, before the non-blocking
	// enqueue: even a record dropped on a full queue is chain-evidenced.
	seq, err := h.shared.seq.next()
	if err != nil {
		return err
	}

	meta := applyReservedStamps(mergeMeta(cr.Metadata, cr.Fields), h.shared.instanceID, seq)
	body, err := json.Marshal(wireEvent{
		OccurredAt: r.Time.UTC(),
		Action:     cr.EventType,
		Actor:      cr.Actor,
		Resource:   cr.Resource,
		Metadata:   meta,
	})
	if err != nil {
		return fmt.Errorf("ledgerly: encoding captured event: %w", err)
	}

	h.enqueue(body)
	return nil
}

// wireEvent is the POST /v1/events request body (api/events.Event wire
// shape, kebab-case keys). The server assigns the event ID and derives the
// tenant from the caller's token, so neither is set here.
type wireEvent struct {
	OccurredAt time.Time         `json:"occurred-at"`
	Action     string            `json:"action"`
	Actor      map[string]string `json:"actor"`
	Resource   map[string]string `json:"resource"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// mergeMeta folds a record's loose Fields into its explicit metadata group
// (explicit metadata wins) to form the event's user-defined metadata.
func mergeMeta(metadata, fields map[string]string) map[string]string {
	merged := make(map[string]string, len(metadata)+len(fields))
	for k, v := range fields {
		merged[k] = v
	}
	for k, v := range metadata {
		merged[k] = v
	}
	return merged
}

// enqueue hands an encoded event body to the sender without ever blocking
// the host app: a full queue drops the body, and a closed handler ignores
// it.
func (h *Handler) enqueue(body []byte) {
	s := h.shared
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.queue <- body:
	default:
		// Queue full: dropped. The already-burned sequence number leaves
		// a chain-evidenced gap.
	}
}

// emitRegime records a rules-regime transition (sdk.started,
// sdk.rules-activated) both as a chained audit event and as an internal
// diagnostic on next.
func (h *Handler) emitRegime(ctx context.Context, ev regimeEvent) {
	meta := map[string]string{}
	switch ev.Action {
	case ActionSDKStarted:
		meta["regime"] = "fallback"
	case ActionSDKRulesActivated:
		meta["etag"] = ev.ETag
		meta["fallback-duration"] = ev.FallbackDuration.String()
	}

	identity := map[string]string{"type": "ledgerly-sdk", "id": h.shared.instanceID}
	if seq, err := h.shared.seq.next(); err == nil {
		body, merr := json.Marshal(wireEvent{
			OccurredAt: time.Now().UTC(),
			Action:     ev.Action,
			Actor:      identity,
			Resource:   identity,
			Metadata:   applyReservedStamps(meta, h.shared.instanceID, seq),
		})
		if merr == nil {
			h.enqueue(body)
		}
	}

	args := make([]any, 0, 2*len(meta))
	for k, v := range meta {
		args = append(args, k, v)
	}
	h.logInternal(ctx, slog.LevelInfo, ev.Action, args...)
}

// logInternal routes an SDK-internal diagnostic record directly to next,
// bypassing Handle() (and therefore capture()) entirely — the sole
// self-suppression mechanism (ADR-0001 amendments): the bypass is an
// unexported code path, unavailable to public slog callers. It tags the
// record with internalAttr purely as an informational marker for the
// app's own log output; nothing keys off that attr.
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
// same buffer dir resumes cleanly. If ctx expires before the drain
// finishes, delivery is aborted — anything undelivered stays spilled in
// the disk buffer for the next instance to replay — and ctx's error is
// returned.
func (h *Handler) Close(ctx context.Context) error {
	s := h.shared
	var err error
	s.closeOnce.Do(func() {
		s.poller.stop()

		s.mu.Lock()
		s.closed = true
		close(s.queue)
		s.mu.Unlock()

		select {
		case <-s.sender.done:
		case <-ctx.Done():
			s.cancel()
			<-s.sender.done
			err = ctx.Err()
		}
		s.cancel()

		err = errors.Join(err, s.seq.close(), s.buf.close())
	})
	return err
}
