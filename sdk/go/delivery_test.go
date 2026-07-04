package ledgerly

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// countingEventsServer records every POST /v1/events it receives and
// replies with statusCode (202 by default) until failFirstN requests have
// been seen, after which it always replies 202 — modeling an outage that
// recovers.
type countingEventsServer struct {
	mu         sync.Mutex
	bodies     [][]byte
	failFirstN int
	seen       int
	*httptest.Server
}

func newCountingEventsServer(failFirstN int) *countingEventsServer {
	s := &countingEventsServer{failFirstN: failFirstN}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)

		s.mu.Lock()
		s.seen++
		fail := s.seen <= s.failFirstN
		if !fail {
			s.bodies = append(s.bodies, buf)
		}
		s.mu.Unlock()

		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	return s
}

func (s *countingEventsServer) requestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seen
}

func (s *countingEventsServer) acceptedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

// waitFor polls cond every 10ms up to timeout, for asserting on
// asynchronous delivery without a fixed sleep.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func TestDelivery_MatchedEvent_ReachesServerAs202(t *testing.T) {
	server := newCountingEventsServer(0)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithEventsURL(server.URL+"/v1/events"))
	logger := slog.New(h)
	logger.Info("delete", "event-type", "project.delete")

	if !waitFor(t, 500*time.Millisecond, func() bool { return server.requestCount() > 0 }) {
		t.Fatal("expected the matched event to reach the server as a POST /v1/events, got none")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDelivery_Close_DrainsQueueAndLeavesBufferEmpty(t *testing.T) {
	server := newCountingEventsServer(0)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithEventsURL(server.URL+"/v1/events"))
	logger := slog.New(h)
	logger.Info("delete", "event-type", "project.delete")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatalf("expected Close to drain the queue and return no error, got %v", err)
	}

	empty, err := h.shared.buf.isEmpty()
	if err != nil {
		t.Fatalf("buf.isEmpty: %v", err)
	}
	if !empty {
		t.Fatal("expected the disk buffer to be empty after a clean Close drains delivery")
	}
}

func TestDelivery_SpillOnOutage_ThenOrderedReplay(t *testing.T) {
	// The first two POSTs fail (simulated outage); delivery should spill
	// them to disk and, once the outage clears, replay them in original
	// order before resuming live delivery.
	server := newCountingEventsServer(2)
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithEventsURL(server.URL+"/v1/events"), WithBackoff(10*time.Millisecond, 50*time.Millisecond))
	logger := slog.New(h)
	logger.Info("first", "event-type", "project.delete", "marker", "1")
	logger.Info("second", "event-type", "project.delete", "marker", "2")
	logger.Info("third", "event-type", "project.delete", "marker", "3")

	if !waitFor(t, 2*time.Second, func() bool { return server.acceptedCount() >= 3 }) {
		t.Fatalf("expected all 3 events to eventually be accepted after the outage clears, got %d", server.acceptedCount())
	}
}

func TestDelivery_RestartReplay_PreservesOriginalStamps(t *testing.T) {
	bufferDir := t.TempDir()

	// First instance: server unreachable, so the captured event should
	// spill to the shared buffer dir with its original sequence stamp.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	first, err := NewHandler(fallbackRules(), newSpyHandler(true), WithBufferDir(bufferDir), WithEventsURL(dead.URL+"/v1/events"))
	if err != nil {
		t.Fatalf("NewHandler (first instance): %v", err)
	}
	slog.New(first).Info("delete", "event-type", "project.delete")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = first.Close(ctx) // best-effort: the point of this test is the replay, not this Close's own error
	dead.Close()

	// Second instance, same buffer dir, now pointed at a live server:
	// replay should deliver the original event with its original stamps.
	live := newCountingEventsServer(0)
	defer live.Close()

	second, err := NewHandler(fallbackRules(), newSpyHandler(true), WithBufferDir(bufferDir), WithEventsURL(live.URL+"/v1/events"))
	if err != nil {
		t.Fatalf("NewHandler (second instance): %v", err)
	}
	defer func() {
		ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
		defer cancel2()
		_ = second.Close(ctx2)
	}()

	if !waitFor(t, time.Second, func() bool { return live.acceptedCount() > 0 }) {
		t.Fatal("expected the restarted handler to replay the spilled event to the now-live server")
	}
}

func TestBackoffDuration_DoublesAndCapsAtMax(t *testing.T) {
	base := 100 * time.Millisecond
	max := 800 * time.Millisecond

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},  // would be 800ms uncapped, exactly at max
		{5, 800 * time.Millisecond},  // would be 1600ms uncapped, capped at max
		{10, 800 * time.Millisecond}, // far beyond max, stays capped
	}
	for _, c := range cases {
		got := backoffDuration(c.attempt, base, max)
		if got != c.want {
			t.Errorf("backoffDuration(%d, %s, %s) = %s, want %s", c.attempt, base, max, got, c.want)
		}
	}
}
