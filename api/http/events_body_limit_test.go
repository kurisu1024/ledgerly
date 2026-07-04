package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

// eventBodyCapBytes mirrors the transport limit issue #33 asks for on
// POST /v1/events: 1 MiB, an order of magnitude above any legitimate event
// payload. The production constant is maxEventBodyBytes beside CreateEvent
// in handlers.go (unexported, so mirrored here from the external test
// package), which mirrors maxRuleBodyBytes in rules.go.
const eventBodyCapBytes = 1 << 20 // 1 MiB, per issue #33

// bloatedEvent returns a structurally valid event whose Metadata field pads
// the JSON body past n bytes, so the oversized-ness of the payload — not an
// unrelated validation failure — is what should trip the transport limit.
func bloatedEvent(n int) Event {
	return Event{
		OccurredAt: time.Now().UTC(),
		Action:     "project.create",
		Actor: map[string]string{
			"id":   "user123",
			"type": "user",
			"ip":   "127.0.0.1",
		},
		Resource: map[string]string{
			"type": "project",
			"id":   "proj123",
		},
		Metadata: map[string]string{
			"padding": strings.Repeat("a", n),
		},
	}
}

// TestCreateEvent_OversizedBody_413 pins the issue #33 transport limit on
// POST /v1/events: a body over eventBodyCapBytes must be rejected with 413
// before decoding, and the event must never reach the audit chain (no
// enqueue, so it can never appear in an export, however long we wait or how
// many times we flush).
//
// RED: CreateEvent (api/http/handlers.go) currently decodes the request body
// unbounded, so this oversized body is accepted (202) and the event is
// enqueued and later exported. Both assertions below are expected to fail
// until a MaxBytesReader-style cap lands on this route.
func TestCreateEvent_OversizedBody_413(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()
	cfg.FlushInterval = 50 * time.Millisecond
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tGiven an event body larger than the 1 MiB transport cap")
	oversized := bloatedEvent(eventBodyCapBytes + 1)
	oversizedJSON, err := json.Marshal(oversized)
	if err != nil {
		t.Fatalf("\t%s\tFailed to marshal oversized event: %v", fail, err)
	}
	if len(oversizedJSON) <= eventBodyCapBytes {
		t.Fatalf("\t%s\tTest fixture is not actually oversized: %d bytes", fail, len(oversizedJSON))
	}

	t.Log("\tWhen POSTing it to /v1/events")
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(oversizedJSON))
	req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	t.Log("\tThen the response must be 413 Request Entity Too Large")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
	} else {
		t.Logf("\t%s\tOversized body correctly rejected with 413\n", pass)
	}

	t.Log("\tAnd no event may ever appear in the tenant's export, however long we wait")
	if err := server.Flush(ctx); err != nil {
		t.Fatalf("\t%s\tFlush: %v", fail, err)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/v1/export", nil)
	exportReq.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	exportW := httptest.NewRecorder()
	server.ServeHTTP(exportW, exportReq)

	if exportW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExport request failed: status %d: %s", fail, exportW.Code, exportW.Body.String())
	}

	var exported []Event
	if err := json.NewDecoder(exportW.Body).Decode(&exported); err != nil && exportW.Body.Len() > 0 {
		t.Fatalf("\t%s\tFailed to decode export response: %v", fail, err)
	}

	if len(exported) != 0 {
		t.Errorf("\t%s\tExpected 0 events after a rejected oversized body, got %d", fail, len(exported))
	} else {
		t.Logf("\t%s\tNo event was enqueued for the rejected body\n", pass)
	}
}

// TestCreateEvent_NormalBody_202 is the accompanying happy-path pin: a
// normal-size event must still be accepted (202) once a cap exists. This is
// expected to PASS both before and after the fix — it guards against an
// overzealous limit implementation breaking legitimate traffic.
func TestCreateEvent_NormalBody_202(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tGiven a normal, well-under-the-cap event body")
	normal := bloatedEvent(0)
	normalJSON, err := json.Marshal(normal)
	if err != nil {
		t.Fatalf("\t%s\tFailed to marshal event: %v", fail, err)
	}
	if len(normalJSON) >= eventBodyCapBytes {
		t.Fatalf("\t%s\tTest fixture is unexpectedly large: %d bytes", fail, len(normalJSON))
	}

	t.Log("\tWhen POSTing it to /v1/events")
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(normalJSON))
	req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	t.Log("\tThen it must still be accepted")
	if w.Code != http.StatusAccepted {
		t.Fatalf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusAccepted, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tNormal-size event still accepted with 202\n", pass)
}
