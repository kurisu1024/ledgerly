package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	apievents "github.com/kurisu1024/ledgerly/api/events"
	httpapi "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

// TestDogfoodEndToEnd is the RED-stage proof for issue #27: the dogfood
// slog logger → SDK → server's own ledgerly-self chain loop, real and
// demo-able against our own stack (CONTEXT.md's "log → trigger → chain →
// verify" loop).
//
// Composition: a real HTTP server (httpapi.New over in-memory storage,
// wrapped in httptest.NewServer so the SDK's HTTP delivery is genuine, not
// an in-process shortcut) stands in for "the ledgerly server"; a dogfood
// Handler constructed by newDogfood, pointed at that server, stands in for
// "service/'s own logging". The server's own request/response logging
// (loggingMiddleware) and worker logging stay on a plain, non-dogfood zap
// logger throughout — breaker #1 (ingest-path logging never reaches the
// SDK) is deliberately exercised, not just asserted by inspection.
//
// The rules URL below is deliberately unregistered on the test server: the
// SDK's constructor-time rules fetch then always fails (404), so the
// handler stays on its compiled-in fallback for the entire test and never
// records sdk.rules-activated. That keeps the expected event set below
// exactly {sdk.started, service.started} regardless of scheduling — a
// rules-activated event would otherwise be racy against this test's
// timing.
func TestDogfoodEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	ruleStore := memory.NewRules()
	serverLogger := zap.NewNop() // the server's own logger; deliberately not dogfood-wrapped

	cfg := httpapi.DefaultConfig()
	cfg.AllowUnverifiedJWT = true
	cfg.ChainSize = 50            // large enough that this test's handful of events land in one block/chain
	cfg.FlushInterval = time.Hour // only the explicit Flush calls below should ever persist anything

	server := httpapi.New(ctx, store, ruleStore, cfg, serverLogger)
	defer server.Close()

	ts := httptest.NewServer(server)
	defer ts.Close()

	fixedNow := time.Now().UTC()

	dfBase := zap.NewNop()
	df, err := newDogfood(dfBase, dogfoodConfig{
		enabled:   true,
		bufferDir: t.TempDir(),
		eventsURL: ts.URL + "/v1/events",
		rulesURL:  ts.URL + "/v1/rules-unregistered", // see doc comment above
		now:       func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatalf("newDogfood: %v", err)
	}

	df.logger.Info("service started",
		slog.String("event-type", "service.started"),
		slog.Group("metadata", slog.String("backend", "memory")),
	)

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := df.Close(closeCtx); err != nil {
		t.Fatalf("dogfood Close: %v", err)
	}

	if err := server.Flush(ctx); err != nil {
		t.Fatalf("server Flush: %v", err)
	}

	exportToken, err := mintSelfToken(fixedNow, time.Hour)
	if err != nil {
		t.Fatalf("mintSelfToken (for the export/verify step): %v", err)
	}

	expectedActions := []string{"sdk.started", "service.started"}
	assertDogfoodExport(t, ts.URL, exportToken, expectedActions)

	// Second flush cycle: nothing should have been produced since the
	// first export/verify pass. A leak (e.g. the SDK's own delivery POST or
	// the server's request logging recursing back into the self chain)
	// would show up here as extra or duplicated events on otherwise
	// unchanged input — the recursion pin.
	if err := server.Flush(ctx); err != nil {
		t.Fatalf("second server Flush: %v", err)
	}
	assertDogfoodExport(t, ts.URL, exportToken, expectedActions)
}

// assertDogfoodExport GETs /v1/export for the ledgerly-self tenant, asserts
// that exactly expectedActions (as a set, order-independent) are present —
// no more, no fewer — and that the resulting chain passes
// audit.VerifyChainReport.
func assertDogfoodExport(t *testing.T, baseURL, token string, expectedActions []string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1/export", nil)
	if err != nil {
		t.Fatalf("building export request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/export: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/export: status %d", resp.StatusCode)
	}

	var exported []apievents.Event
	if err := json.NewDecoder(resp.Body).Decode(&exported); err != nil {
		t.Fatalf("decoding export response: %v", err)
	}

	gotActions := make([]string, 0, len(exported))
	for _, e := range exported {
		gotActions = append(gotActions, e.Action)
	}
	sort.Strings(gotActions)
	wantActions := append([]string(nil), expectedActions...)
	sort.Strings(wantActions)

	if !equalStrings(gotActions, wantActions) {
		t.Fatalf("exported actions = %v, want exactly %v (recursion/leak pin — no ingest/request logs leaked in)", gotActions, wantActions)
	}

	if len(exported) == 0 {
		return
	}

	auditEvents := make([]audit.Event, 0, len(exported))
	for _, e := range exported {
		ae, err := apievents.ToAuditEvent(e)
		if err != nil {
			t.Fatalf("converting exported event %q to an audit.Event: %v", e.Action, err)
		}
		auditEvents = append(auditEvents, ae)
	}
	chain := audit.EventChain{ID: auditEvents[0].ChainID, Events: auditEvents}

	report := audit.VerifyChainReport(chain)
	if report.Status != audit.StatusVerified {
		t.Fatalf("chain verification: status=%s reason=%s failed-index=%d (of %d events)",
			report.Status, report.Reason, report.FailedIndex, report.Length)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
