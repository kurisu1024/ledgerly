package ledgerly

// Supplementary unit tests for surfaces the issue-#26 spec suite exercises
// only indirectly: record flattening, option plumbing, and buffer
// crash-recovery details.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLevelName_BucketsAllLevels(t *testing.T) {
	cases := []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "debug"},
		{slog.LevelInfo, "info"},
		{slog.LevelInfo + 2, "info"},
		{slog.LevelWarn, "warn"},
		{slog.LevelError, "error"},
		{slog.LevelError + 4, "error"},
	}
	for _, c := range cases {
		if got := levelName(c.level); got != c.want {
			t.Errorf("levelName(%v) = %q, want %q", c.level, got, c.want)
		}
	}
}

func TestFlattenForHandler_GroupsAndBoundAttrs(t *testing.T) {
	r := slog.NewRecord(time.Now(), slog.LevelWarn, "delete", 0)
	r.Add(
		"event-type", "project.delete",
		slog.Group("actor", "type", "admin", "id", "u-1"),
		slog.Group("resource", "id", "proj-1"),
		slog.Group("metadata", "reason", "cleanup"),
		slog.Group("billing", "amount", 100),
		"plain", "value",
	)

	cr := flattenForHandler(r, nil, []slog.Attr{slog.String("component", "api")})

	if cr.EventType != "project.delete" {
		t.Fatalf("EventType = %q", cr.EventType)
	}
	if cr.Level != "warn" {
		t.Fatalf("Level = %q", cr.Level)
	}
	if cr.Actor["type"] != "admin" || cr.Actor["id"] != "u-1" {
		t.Fatalf("Actor = %v", cr.Actor)
	}
	if cr.Resource["id"] != "proj-1" {
		t.Fatalf("Resource = %v", cr.Resource)
	}
	if cr.Metadata["reason"] != "cleanup" {
		t.Fatalf("Metadata = %v", cr.Metadata)
	}
	if cr.Fields["billing.amount"] != "100" {
		t.Fatalf("nested group field = %v", cr.Fields)
	}
	if cr.Fields["plain"] != "value" || cr.Fields["component"] != "api" {
		t.Fatalf("Fields = %v", cr.Fields)
	}

	in := cr.matchInput()
	if in.Fields["actor.type"] != "admin" || in.Fields["resource.id"] != "proj-1" {
		t.Fatalf("matchInput fields = %v", in.Fields)
	}
}

func TestFlattenForHandler_HandlerGroupPrefixesRecordAttrs(t *testing.T) {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "grouped", 0)
	r.Add("amount", 100)

	cr := flattenForHandler(r, []string{"billing"}, nil)

	if cr.Fields["billing.amount"] != "100" {
		t.Fatalf("expected record attrs to flatten under the handler's group path, got %v", cr.Fields)
	}
}

func TestWithAttrsAndWithGroup_EmptyInputsReturnSameHandler(t *testing.T) {
	h := newTestHandler(t, newSpyHandler(true))

	if got := h.WithAttrs(nil); got != slog.Handler(h) {
		t.Fatal("expected WithAttrs(nil) to return the same handler")
	}
	if got := h.WithGroup(""); got != slog.Handler(h) {
		t.Fatal("expected WithGroup(\"\") to return the same handler")
	}
}

func TestDelivery_OptionsPlumbing_APIKeyClientAndBody(t *testing.T) {
	type seen struct {
		auth string
		body []byte
	}
	var mu sync.Mutex
	var got []seen

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		mu.Lock()
		got = append(got, seen{auth: r.Header.Get("Authorization"), body: body})
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true),
		WithEventsURL(server.URL+"/v1/events"),
		WithAPIKey("secret-key"),
		WithHTTPClient(&http.Client{Timeout: 5 * time.Second}),
		WithQueueSize(4),
	)
	logger := slog.New(h)
	logger.Warn("delete",
		"event-type", "project.delete",
		slog.Group("actor", "type", "admin"),
		MetaSeq, "spoofed",
	)

	if !waitFor(t, time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) > 0 }) {
		t.Fatal("expected the event to reach the server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if got[0].auth != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", got[0].auth)
	}

	var ev struct {
		Action   string            `json:"action"`
		Actor    map[string]string `json:"actor"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(got[0].body, &ev); err != nil {
		t.Fatalf("decoding delivered event: %v", err)
	}
	if ev.Action != "project.delete" {
		t.Fatalf("action = %q", ev.Action)
	}
	if ev.Actor["type"] != "admin" {
		t.Fatalf("actor = %v", ev.Actor)
	}
	if ev.Metadata[MetaInstanceID] == "" {
		t.Fatalf("expected %s stamp, metadata = %v", MetaInstanceID, ev.Metadata)
	}
	if ev.Metadata[MetaSeq] == "spoofed" || ev.Metadata[MetaSeq] == "" {
		t.Fatalf("expected the spoofed %s to be overwritten, got %q", MetaSeq, ev.Metadata[MetaSeq])
	}
}

func TestHandler_Enabled_FallsBackToNextForUnwantedLevels(t *testing.T) {
	spy := newSpyHandler(false)
	fallback := []Rule{{SchemaVersion: SchemaVersion, EventType: "project.delete", LevelAtLeast: "error"}}
	h, err := NewHandler(fallback, spy, WithBufferDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected Enabled(debug) to be false: next disables it and no rule wants below error")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Fatal("expected Enabled(error) to be true: the rule wants error")
	}
}

func TestBuffer_AppendReplayAckAcrossReopen(t *testing.T) {
	dir := t.TempDir()

	b, err := openBuffer(dir)
	if err != nil {
		t.Fatalf("openBuffer: %v", err)
	}
	for _, rec := range []string{`{"n":1}`, `{"n":2}`, `{"n":3}`} {
		if err := b.append([]byte(rec)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := b.ack(2); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if err := b.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := b.close(); err != nil {
		t.Fatalf("second close should be a no-op, got %v", err)
	}

	reopened, err := openBuffer(dir)
	if err != nil {
		t.Fatalf("re-openBuffer: %v", err)
	}
	records, err := reopened.replay()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(records) != 1 || string(records[0]) != `{"n":3}` {
		t.Fatalf("expected only the unacked record to survive reopen, got %q", records)
	}

	empty, err := reopened.isEmpty()
	if err != nil || empty {
		t.Fatalf("isEmpty = %v, %v; want false, nil", empty, err)
	}
	if err := reopened.ack(1); err != nil {
		t.Fatalf("ack after reopen: %v", err)
	}
	empty, err = reopened.isEmpty()
	if err != nil || !empty {
		t.Fatalf("isEmpty after full ack = %v, %v; want true, nil", empty, err)
	}
}

func TestBuffer_CorruptTailTruncatedOnOpen(t *testing.T) {
	dir := t.TempDir()
	segment := filepath.Join(dir, segmentFile)
	if err := os.WriteFile(segment, []byte("{\"n\":1}\n{\"n\":2}\n{\"partial"), 0o600); err != nil {
		t.Fatalf("seeding segment: %v", err)
	}

	b, err := openBuffer(dir)
	if err != nil {
		t.Fatalf("openBuffer: %v", err)
	}
	records, err := b.replay()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected the partial tail record to be truncated away, got %d records: %q", len(records), records)
	}
}

func TestJittered_StaysWithinHalfToFullRange(t *testing.T) {
	d := 100 * time.Millisecond
	for range 100 {
		got := jittered(d)
		if got < d/2 || got >= d {
			t.Fatalf("jittered(%s) = %s, want in [%s, %s)", d, got, d/2, d)
		}
	}
	if got := jittered(1); got != 1 {
		t.Fatalf("jittered(1) = %d, want 1", got)
	}
}

func TestSequencer_DoubleCloseAndNextAfterClose(t *testing.T) {
	dir := t.TempDir()
	s, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("openSequencer: %v", err)
	}
	if _, err := s.next(); err != nil {
		t.Fatalf("next: %v", err)
	}
	if err := s.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := s.close(); err != nil {
		t.Fatalf("second close should be a no-op, got %v", err)
	}
	if _, err := s.next(); err == nil {
		t.Fatal("expected next() after close to error")
	}
}
