package ledgerly

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// spyHandler is a minimal slog.Handler that records every record it
// receives, for asserting tee behavior without a real sink.
type spyHandler struct {
	mu      sync.Mutex
	records []slog.Record
	enabled bool
	groups  []string
	attrs   []slog.Attr
}

func newSpyHandler(enabled bool) *spyHandler {
	return &spyHandler{enabled: enabled}
}

func (s *spyHandler) Enabled(ctx context.Context, level slog.Level) bool { return s.enabled }

func (s *spyHandler) Handle(ctx context.Context, r slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	return nil
}

func (s *spyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := &spyHandler{enabled: s.enabled, groups: s.groups}
	clone.attrs = append(append([]slog.Attr{}, s.attrs...), attrs...)
	return clone
}

func (s *spyHandler) WithGroup(name string) slog.Handler {
	clone := &spyHandler{enabled: s.enabled, attrs: s.attrs}
	clone.groups = append(append([]string{}, s.groups...), name)
	return clone
}

func (s *spyHandler) recordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func fallbackRules() []Rule {
	return []Rule{
		{SchemaVersion: SchemaVersion, EventType: "project.delete", LevelAtLeast: "debug"},
	}
}

func newTestHandler(t *testing.T, next slog.Handler, opts ...Option) *Handler {
	t.Helper()
	allOpts := append([]Option{WithBufferDir(t.TempDir())}, opts...)
	h, err := NewHandler(fallbackRules(), next, allOpts...)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func TestNewHandler_ErrorsOnEmptyFallback(t *testing.T) {
	_, err := NewHandler(nil, newSpyHandler(true), WithBufferDir(t.TempDir()))
	if err == nil {
		t.Fatal("expected an error for an empty fallback rule-set, got nil")
	}
}

func TestNewHandler_ErrorsOnMissingBufferDir(t *testing.T) {
	_, err := NewHandler(fallbackRules(), newSpyHandler(true))
	if err == nil {
		t.Fatal("expected an error when WithBufferDir is not supplied, got nil")
	}
}

func TestNewHandler_ErrorsOnNilNext(t *testing.T) {
	_, err := NewHandler(fallbackRules(), nil, WithBufferDir(t.TempDir()))
	if err == nil {
		t.Fatal("expected an error for a nil next handler, got nil")
	}
}

func TestHandler_Tee_AlwaysReachesNext(t *testing.T) {
	spy := newSpyHandler(true)
	h := newTestHandler(t, spy)

	logger := slog.New(h)
	logger.Info("plain log line, no event-type attr")
	logger.Info("matcher-eligible line", "event-type", "project.delete")

	if got := spy.recordCount(); got != 2 {
		t.Fatalf("expected next to receive both records regardless of matcher outcome, got %d", got)
	}
}

func TestHandler_Enabled_TrueWhenOnlyRulesWantLevel(t *testing.T) {
	// next only enables Warn and above; the fallback rule set asks for
	// debug. The SDK should still report Enabled(debug) == true so a
	// matching debug-level record is not filtered out before Handle ever
	// sees it.
	spy := newSpyHandler(false) // next disables everything by default in this test
	h := newTestHandler(t, spy)

	if !h.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected Enabled(debug) to be true because the active rule set wants debug, even though next does not enable it")
	}
}

func TestHandler_SelfSuppression_InternalLogNeverReachesCapture(t *testing.T) {
	spy := newSpyHandler(true)
	h := newTestHandler(t, spy)

	h.logInternal(context.Background(), slog.LevelInfo, "sdk internal diagnostic")

	if got := spy.recordCount(); got != 1 {
		t.Fatalf("expected the internal diagnostic to still reach next (guard #1 bypasses Handle, not next), got %d records", got)
	}
	if attempts := h.captureAttempts(); attempts != 0 {
		t.Fatalf("expected logInternal to bypass the capture pipeline entirely, got %d capture attempts", attempts)
	}
}

func TestHandler_UserInternalAttr_HasNoSuppressionSemantics(t *testing.T) {
	// A host application setting ledgerly.internal=true on its own record
	// must NOT exempt that record from capture: the attr would otherwise be
	// an unauthenticated audit-evasion switch (no seq burned, no gap).
	server := newCountingEventsServer(0)
	defer server.Close()

	spy := newSpyHandler(true)
	h := newTestHandler(t, spy, WithEventsURL(server.URL+"/v1/events"))

	logger := slog.New(h)
	logger.Info("evasion attempt", "event-type", "project.delete", internalAttr, true)

	if attempts := h.captureAttempts(); attempts != 1 {
		t.Fatalf("expected a user record carrying %q to reach the capture pipeline like any other, got %d capture attempts", internalAttr, attempts)
	}
	if !waitFor(t, time.Second, func() bool { return server.acceptedCount() >= 1 }) {
		t.Fatalf("expected the record carrying %q to be captured and delivered normally, got none", internalAttr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestHandler_InternalDiagnostics_NeverEnterCaptureEvenWithMatchingRules(t *testing.T) {
	// Recursion guard: rules crafted to match the SDK's own regime events
	// must not cause the internal diagnostics to be re-captured — internal
	// diagnostics are written directly to next, never entering Handle. The
	// only chained events are the two directly-enqueued regime events.
	rules := newRulesServer([]byte(validRulesEnvelope), `"etag-1"`)
	defer rules.Close()
	events := newCountingEventsServer(0)
	defer events.Close()

	fallback := []Rule{
		{SchemaVersion: SchemaVersion, EventType: ActionSDKStarted},
		{SchemaVersion: SchemaVersion, EventType: ActionSDKRulesActivated},
	}
	spy := newSpyHandler(true)
	h, err := NewHandler(fallback, spy,
		WithBufferDir(t.TempDir()),
		WithEventsURL(events.URL+"/v1/events"),
		WithRulesURL(rules.URL+"/v1/rules"),
	)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	// Both regime transitions (sdk.started, sdk.rules-activated) have fired
	// by the time NewHandler returns; drain delivery via Close.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if attempts := h.captureAttempts(); attempts != 0 {
		t.Fatalf("expected internal diagnostics to bypass Handle entirely (no recursion path), got %d capture attempts", attempts)
	}
	if got := events.acceptedCount(); got != 2 {
		t.Fatalf("expected exactly the 2 directly-enqueued regime events (no recursive re-capture), got %d", got)
	}
}

func TestHandler_WithAttrs_VisibleOnNext(t *testing.T) {
	spy := newSpyHandler(true)
	h := newTestHandler(t, spy)

	derived, ok := h.WithAttrs([]slog.Attr{slog.String("component", "billing")}).(*Handler)
	if !ok {
		t.Fatalf("expected WithAttrs to return a *Handler, got %T", derived)
	}
	derivedSpy, ok := derived.next.(*spyHandler)
	if !ok {
		t.Fatalf("expected the derived handler's next to be a *spyHandler, got %T", derived.next)
	}

	logger := slog.New(derived)
	logger.Info("attributed line")

	if got := derivedSpy.recordCount(); got != 1 {
		t.Fatalf("expected the derived handler's record to reach its (attribute-carrying) next, got %d", got)
	}
}

func TestHandler_WithGroup_VisibleOnNext(t *testing.T) {
	spy := newSpyHandler(true)
	h := newTestHandler(t, spy)

	derived, ok := h.WithGroup("billing").(*Handler)
	if !ok {
		t.Fatalf("expected WithGroup to return a *Handler, got %T", derived)
	}
	derivedSpy, ok := derived.next.(*spyHandler)
	if !ok {
		t.Fatalf("expected the derived handler's next to be a *spyHandler, got %T", derived.next)
	}

	logger := slog.New(derived)
	logger.Info("grouped line", "amount", 100)

	if got := derivedSpy.recordCount(); got != 1 {
		t.Fatalf("expected the grouped handler's record to reach its (grouped) next, got %d", got)
	}
}
