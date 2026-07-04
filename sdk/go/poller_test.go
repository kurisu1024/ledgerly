package ledgerly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// rulesServer serves GET /v1/rules with ETag support and lets a test swap
// in a new envelope + etag on the fly, to model a rule-set change the
// poller should pick up on its next fetch.
type rulesServer struct {
	*httptest.Server
	body      []byte
	etag      string
	requestsN atomic.Int64
}

func newRulesServer(body []byte, etag string) *rulesServer {
	rs := &rulesServer{body: body, etag: etag}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.requestsN.Add(1)
		if inm := r.Header.Get("If-None-Match"); inm != "" && inm == rs.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", rs.etag)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rs.body)
	}))
	return rs
}

const validRulesEnvelope = `{"schema-version":1,"rules":[{"id":"r1","schema-version":1,"event-type":"project.delete","level-at-least":"warn"}]}`
const schemaV2RulesEnvelope = `{"schema-version":2,"rules":[]}`

func TestPoller_FirstFetchTimeout_ShorterThanSteadyState(t *testing.T) {
	// The synchronous first fetch runs inside NewHandler, so its budget
	// must be tighter than the steady-state poll timeout.
	if firstFetchTimeout >= fetchTimeout {
		t.Fatalf("expected the constructor's first-fetch timeout (%s) to be shorter than the steady-state fetch timeout (%s)", firstFetchTimeout, fetchTimeout)
	}
}

func TestPoller_SyncFirstFetch_BoundedByFirstFetchTimeout(t *testing.T) {
	// A hung rules endpoint must not hang construction beyond the (short)
	// first-fetch timeout.
	release := make(chan struct{})
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer hung.Close()
	defer close(release)

	p := newPoller(newAPIClient("", hung.URL+"/v1/rules", "", nil), time.Hour, false, func(RuleList, regimeEvent) {})
	p.firstFetchTimeout = 50 * time.Millisecond

	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.start(ctx)
	elapsed := time.Since(start)
	p.stop()

	if elapsed >= time.Second {
		t.Fatalf("expected start's synchronous first fetch to give up after its own short timeout, took %s", elapsed)
	}
}

func TestNewHandler_SyncFirstFetch_RulesActiveOnReturn(t *testing.T) {
	server := newRulesServer([]byte(validRulesEnvelope), `"etag-1"`)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithRulesURL(server.URL+"/v1/rules"))

	// Sync is the default: the server's rules must already be active the
	// moment NewHandler returns, no polling wait.
	active := h.shared.activeRules.Load()
	if active == nil || len(*active) == 0 || (*active)[0].ID != "r1" {
		t.Fatal("expected the default synchronous first fetch to have the server's rules active when NewHandler returns")
	}
}

func TestNewHandler_AsyncFirstFetch_ReturnsImmediatelyThenActivates(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Header().Set("ETag", `"etag-1"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(validRulesEnvelope))
	}))
	defer slow.Close()

	h := newTestHandler(t, newSpyHandler(true),
		WithRulesURL(slow.URL+"/v1/rules"),
		WithAsyncFirstFetch(),
	)

	// Construction must not have waited on the (still-blocked) fetch: the
	// compiled-in fallback is still active.
	active := h.shared.activeRules.Load()
	if active == nil || len(*active) != 1 || (*active)[0].EventType != "project.delete" {
		t.Fatal("expected WithAsyncFirstFetch to return with the fallback rules still active, not block on the first fetch")
	}

	close(release)
	if !waitFor(t, time.Second, func() bool {
		active := h.shared.activeRules.Load()
		return active != nil && len(*active) > 0 && (*active)[0].ID == "r1"
	}) {
		t.Fatal("expected the async first fetch to eventually activate the server's rules")
	}
}

func TestPoller_Startup_EmitsSDKStartedRegimeFallback(t *testing.T) {
	server := newRulesServer([]byte(validRulesEnvelope), `"etag-1"`)
	defer server.Close()

	spy := newSpyHandler(true)
	_ = newTestHandler(t, spy, WithRulesURL(server.URL+"/v1/rules"))

	if !waitFor(t, 500*time.Millisecond, func() bool { return spy.recordCount() > 0 }) {
		t.Fatal("expected NewHandler to emit a chained sdk.started (regime=fallback) event via next on construction")
	}
}

func TestPoller_FirstFetch_SwapsRulesAndEmitsActivatedRegime(t *testing.T) {
	server := newRulesServer([]byte(validRulesEnvelope), `"etag-1"`)
	defer server.Close()

	spy := newSpyHandler(true)
	h := newTestHandler(t, spy, WithRulesURL(server.URL+"/v1/rules"), WithRefreshInterval(20*time.Millisecond))

	if !waitFor(t, time.Second, func() bool { return server.requestsN.Load() > 0 }) {
		t.Fatal("expected the poller to fetch GET /v1/rules at least once")
	}

	active := h.shared.activeRules.Load()
	if active == nil || len(*active) == 0 || (*active)[0].ID != "r1" {
		t.Fatal("expected the first successful fetch to swap the active rule set to the server's rules")
	}

	// sdk.rules-activated should appear as a distinct record from
	// sdk.started once the fetch succeeds.
	if !waitFor(t, time.Second, func() bool { return spy.recordCount() >= 2 }) {
		t.Fatal("expected a second chained event (sdk.rules-activated) after the first successful rules fetch")
	}
}

func TestPoller_ETagSentOnSubsequentFetch(t *testing.T) {
	server := newRulesServer([]byte(validRulesEnvelope), `"etag-1"`)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithRulesURL(server.URL+"/v1/rules"), WithRefreshInterval(20*time.Millisecond))
	_ = h

	if !waitFor(t, time.Second, func() bool { return server.requestsN.Load() >= 2 }) {
		t.Fatal("expected the poller to make at least two fetches, the second carrying If-None-Match")
	}
}

func TestPoller_304_NoOp(t *testing.T) {
	server := newRulesServer([]byte(validRulesEnvelope), `"etag-1"`)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithRulesURL(server.URL+"/v1/rules"), WithRefreshInterval(20*time.Millisecond))

	firstSwap := h.shared.activeRules.Load()
	if !waitFor(t, time.Second, func() bool { return server.requestsN.Load() >= 3 }) {
		t.Fatal("expected the poller to keep polling on the refresh interval")
	}
	secondSwap := h.shared.activeRules.Load()

	if firstSwap != secondSwap {
		t.Fatal("expected a 304 Not Modified response to be a no-op: the active rule set pointer should not change")
	}
}

func TestPoller_SchemaVersion2_Refused_KeepsCurrentRules(t *testing.T) {
	server := newRulesServer([]byte(schemaV2RulesEnvelope), `"etag-bad"`)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithRulesURL(server.URL+"/v1/rules"), WithRefreshInterval(20*time.Millisecond))

	waitFor(t, 500*time.Millisecond, func() bool { return server.requestsN.Load() > 0 })

	active := h.shared.activeRules.Load()
	if active == nil || len(*active) != 1 || (*active)[0].EventType != "project.delete" {
		t.Fatal("expected an envelope with an unrecognized schema-version to be refused, keeping the compiled-in fallback rules active")
	}
}
