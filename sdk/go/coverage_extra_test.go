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

// deliverAndDecodeMetadata drives one record through log → capture →
// delivery against a live test server and returns the delivered event's
// metadata, for pinning the reserved-stamp overwrite end to end.
func deliverAndDecodeMetadata(t *testing.T, logIt func(*slog.Logger)) map[string]string {
	t.Helper()

	var mu sync.Mutex
	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	h := newTestHandler(t, newSpyHandler(true), WithEventsURL(server.URL+"/v1/events"))
	logIt(slog.New(h))

	if !waitFor(t, time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return len(bodies) > 0 }) {
		t.Fatal("expected the record to be captured and delivered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	var ev struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(bodies[0], &ev); err != nil {
		t.Fatalf("decoding delivered event: %v", err)
	}
	return ev.Metadata
}

func TestReservedStamps_SpoofInsideMetadataGroup_Overwritten(t *testing.T) {
	meta := deliverAndDecodeMetadata(t, func(logger *slog.Logger) {
		logger.Warn("delete",
			"event-type", "project.delete",
			slog.Group("metadata", MetaSeq, "spoofed-nested"),
		)
	})

	if meta[MetaSeq] == "spoofed-nested" || meta[MetaSeq] == "" {
		t.Fatalf("expected a %s spoof nested in the metadata group to be overwritten by the real stamp, got %q", MetaSeq, meta[MetaSeq])
	}
	if meta[MetaInstanceID] == "" {
		t.Fatalf("expected the real %s stamp to be present, metadata = %v", MetaInstanceID, meta)
	}
}

func TestReservedStamps_SpoofViaBoundLoggerWith_Overwritten(t *testing.T) {
	meta := deliverAndDecodeMetadata(t, func(logger *slog.Logger) {
		logger.With(MetaSeq, "spoofed-bound").Warn("delete", "event-type", "project.delete")
	})

	if meta[MetaSeq] == "spoofed-bound" || meta[MetaSeq] == "" {
		t.Fatalf("expected a %s spoof bound via logger.With to be overwritten by the real stamp, got %q", MetaSeq, meta[MetaSeq])
	}
	if meta[MetaInstanceID] == "" {
		t.Fatalf("expected the real %s stamp to be present, metadata = %v", MetaInstanceID, meta)
	}
}

func TestReservedStamps_SpoofViaBoundMetadataGroupAttr_Overwritten(t *testing.T) {
	meta := deliverAndDecodeMetadata(t, func(logger *slog.Logger) {
		logger.With(slog.Group("metadata", MetaSeq, "spoofed-bound-group")).
			Warn("delete", "event-type", "project.delete")
	})

	if meta[MetaSeq] == "spoofed-bound-group" || meta[MetaSeq] == "" {
		t.Fatalf("expected a %s spoof bound inside a metadata group attr to be overwritten by the real stamp, got %q", MetaSeq, meta[MetaSeq])
	}
	if meta[MetaInstanceID] == "" {
		t.Fatalf("expected the real %s stamp to be present, metadata = %v", MetaInstanceID, meta)
	}
}

func TestReservedStamps_SpoofViaWithGroupMetadataBoundAttr_Overwritten(t *testing.T) {
	// The WithGroup("metadata").With(MetaSeq, ...) shape: the bound attr is
	// group-qualified into cr.Metadata, and the reserved-key overwrite must
	// still win when the event is stamped.
	base := newTestHandler(t, newSpyHandler(true))
	derived, ok := base.WithGroup(metadataGroup).WithAttrs([]slog.Attr{slog.String(MetaSeq, "spoofed-withgroup")}).(*Handler)
	if !ok {
		t.Fatal("expected WithAttrs to return a *Handler")
	}

	r := slog.NewRecord(time.Now(), slog.LevelWarn, "delete", 0)
	cr := flattenForHandler(r, derived.groups, derived.attrs)

	if cr.Metadata[MetaSeq] != "spoofed-withgroup" {
		t.Fatalf("expected the group-bound spoof routed into Metadata pre-stamping, got Metadata = %v, Fields = %v", cr.Metadata, cr.Fields)
	}

	stamped := applyReservedStamps(mergeMeta(cr.Metadata, cr.Fields), "real-instance", 7)
	if stamped[MetaSeq] != "7" {
		t.Fatalf("expected the group-bound %s spoof to be overwritten by the real stamp, got %q", MetaSeq, stamped[MetaSeq])
	}
	if stamped[MetaInstanceID] != "real-instance" {
		t.Fatalf("expected the real %s stamp, got %q", MetaInstanceID, stamped[MetaInstanceID])
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

func TestBuffer_TornCompactionStaleCursor_ClampedOnReopen(t *testing.T) {
	dir := t.TempDir()

	// Simulate a crash between compaction's Truncate(0) and its cursor
	// rewrite: an empty segment on disk with a cursor still holding the
	// pre-compaction ack count.
	if err := os.WriteFile(filepath.Join(dir, segmentFile), nil, 0o600); err != nil {
		t.Fatalf("seeding empty segment: %v", err)
	}
	cf, err := os.OpenFile(filepath.Join(dir, cursorFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("seeding cursor: %v", err)
	}
	if err := writeCounter(cf, 3); err != nil {
		t.Fatalf("writing stale cursor: %v", err)
	}
	if err := cf.Close(); err != nil {
		t.Fatalf("closing seeded cursor: %v", err)
	}

	b, err := openBuffer(dir)
	if err != nil {
		t.Fatalf("openBuffer: %v", err)
	}
	for _, rec := range []string{`{"n":1}`, `{"n":2}`} {
		if err := b.append([]byte(rec)); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	records, err := b.replay()
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected the stale cursor to be clamped so post-restart spills replay, got %d records: %q", len(records), records)
	}
	if err := b.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// The clamp must be persisted: a further reopen must still replay both.
	reopened, err := openBuffer(dir)
	if err != nil {
		t.Fatalf("re-openBuffer: %v", err)
	}
	records, err = reopened.replay()
	if err != nil {
		t.Fatalf("replay after reopen: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected the clamped cursor to persist across reopen, got %d records: %q", len(records), records)
	}
	if err := reopened.close(); err != nil {
		t.Fatalf("close after reopen: %v", err)
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
