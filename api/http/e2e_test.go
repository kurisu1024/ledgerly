package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

var pass = "\u2705"
var fail = "\u274C"

type Event struct {
	ID         string            `json:"id"`
	ChainID    string            `json:"chain-id"`
	TenantID   string            `json:"tenant-id"`
	OccurredAt time.Time         `json:"occurred-at"`
	Action     string            `json:"action"`
	Actor      map[string]string `json:"actor"`
	Resource   map[string]string `json:"resource"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	PrevHash   []byte            `json:"prev-hash,omitempty"`
	EventHash  []byte            `json:"event-hash,omitempty"`
}

func TestEndToEnd_CreateAndExportEvents(t *testing.T) {
	t.Log("\tGiven an HTTP server with worker and in-memory storage")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()

	cfg := apihttp.Config{
		QueueSize:     100,
		ChainSize:     3,                      // Small chain size to trigger writes quickly
		FlushInterval: 100 * time.Millisecond, // Fast flush for testing
	}

	server := apihttp.New(ctx, store, cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tWhen creating multiple events")
	eventCount := 5
	for i := 0; i < eventCount; i++ {
		event := Event{
			OccurredAt: time.Now().UTC(),
			Action:     fmt.Sprintf("action.%d", i),
			Actor: map[string]string{
				"id":   fmt.Sprintf("user%d", i),
				"type": "user",
			},
			Resource: map[string]string{
				"type": "project",
				"id":   "proj123",
			},
			Metadata: map[string]string{
				"index": fmt.Sprintf("%d", i),
			},
		}

		eventJSON, _ := json.Marshal(event)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tenants/%s/events", tenantID), bytes.NewReader(eventJSON))
		req.SetPathValue("tenantID", tenantID.String())
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("\t%s\tFailed to create event %d: status %d, body: %s", fail, i, w.Code, w.Body.String())
		}
	}
	t.Logf("\t%s\tSuccessfully queued %d events", pass, eventCount)

	t.Log("\tThen waiting for worker to process events")
	// Wait for worker to process and flush events to storage
	// We created 5 events with chain size 3, so we should have 2 chains written
	// (1 full chain of 3 + 1 partial chain of 2 after flush interval)
	time.Sleep(500 * time.Millisecond)

	t.Log("\tWhen exporting events via GET")
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tenants/%s/events", tenantID), nil)
	req.SetPathValue("tenantID", tenantID.String())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tFailed to export events: status %d, body: %s", fail, w.Code, w.Body.String())
	}

	var exportedEvents []Event
	if err := json.NewDecoder(w.Body).Decode(&exportedEvents); err != nil {
		t.Fatalf("\t%s\tFailed to decode exported events: %v", fail, err)
	}

	t.Log("\tThen all events should be present")
	if len(exportedEvents) != eventCount {
		t.Errorf("\t%s\tExpected %d events, got %d", fail, eventCount, len(exportedEvents))
	} else {
		t.Logf("\t%s\tSuccessfully exported all %d events", pass, len(exportedEvents))
	}

	t.Log("\tAnd events should be properly chained")
	for i, event := range exportedEvents {
		if event.ChainID == "" {
			t.Errorf("\t%s\tEvent %d is missing chain ID", fail, i)
		}
		if event.EventHash == nil || len(event.EventHash) == 0 {
			t.Errorf("\t%s\tEvent %d is missing event hash", fail, i)
		}
		if event.TenantID != tenantID.String() {
			t.Errorf("\t%s\tEvent %d has wrong tenant ID: %s", fail, i, event.TenantID)
		}
	}
	t.Logf("\t%s\tAll events are properly chained", pass)
}

func TestEndToEnd_MultipleTenants(t *testing.T) {
	t.Log("\tGiven an HTTP server with worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()

	cfg := apihttp.Config{
		QueueSize:     100,
		ChainSize:     2,
		FlushInterval: 100 * time.Millisecond,
	}

	server := apihttp.New(ctx, store, cfg, logger)
	defer server.Close()

	tenant1 := uuid.New()
	tenant2 := uuid.New()

	t.Log("\tWhen creating events for multiple tenants")
	for tenantIdx, tenantID := range []uuid.UUID{tenant1, tenant2} {
		for i := 0; i < 3; i++ {
			event := Event{
				OccurredAt: time.Now().UTC(),
				Action:     fmt.Sprintf("tenant%d.action.%d", tenantIdx, i),
				Actor: map[string]string{
					"id":   fmt.Sprintf("user%d", i),
					"type": "user",
				},
				Resource: map[string]string{
					"type": "project",
					"id":   fmt.Sprintf("proj%d", tenantIdx),
				},
			}

			eventJSON, _ := json.Marshal(event)
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tenants/%s/events", tenantID), bytes.NewReader(eventJSON))
			req.SetPathValue("tenantID", tenantID.String())
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != http.StatusAccepted {
				t.Fatalf("\t%s\tFailed to create event for tenant %d: %d", fail, tenantIdx, w.Code)
			}
		}
	}
	t.Logf("\t%s\tSuccessfully queued events for multiple tenants", pass)

	// Wait for worker to process
	time.Sleep(500 * time.Millisecond)

	t.Log("\tThen each tenant should only see their own events")
	for tenantIdx, tenantID := range []uuid.UUID{tenant1, tenant2} {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tenants/%s/events", tenantID), nil)
		req.SetPathValue("tenantID", tenantID.String())
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		var events []Event
		json.NewDecoder(w.Body).Decode(&events)

		if len(events) != 3 {
			t.Errorf("\t%s\tTenant %d expected 3 events, got %d", fail, tenantIdx, len(events))
		}

		// Verify all events belong to this tenant
		for _, event := range events {
			if event.TenantID != tenantID.String() {
				t.Errorf("\t%s\tEvent belongs to wrong tenant: %s", fail, event.TenantID)
			}
		}
	}
	t.Logf("\t%s\tEach tenant correctly isolated", pass)
}

func TestEndToEnd_GracefulShutdown(t *testing.T) {
	t.Log("\tGiven an HTTP server with pending events")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()

	cfg := apihttp.Config{
		QueueSize:     100,
		ChainSize:     100,           // Large chain so events stay in memory
		FlushInterval: 1 * time.Hour, // Very long interval
	}

	server := apihttp.New(ctx, store, cfg, logger)

	tenantID := uuid.New()

	t.Log("\tWhen creating events")
	for i := 0; i < 3; i++ {
		event := Event{
			OccurredAt: time.Now().UTC(),
			Action:     fmt.Sprintf("action.%d", i),
			Actor:      map[string]string{"id": "user1", "type": "user"},
			Resource:   map[string]string{"type": "project", "id": "proj1"},
		}

		eventJSON, _ := json.Marshal(event)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tenants/%s/events", tenantID), bytes.NewReader(eventJSON))
		req.SetPathValue("tenantID", tenantID.String())
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
	}

	t.Log("\tAnd shutting down the server before flush interval")
	server.Close()

	// Give it a moment to complete shutdown
	time.Sleep(100 * time.Millisecond)

	t.Log("\tThen events should be flushed to storage on shutdown")
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tenants/%s/events", tenantID), nil)
	req.SetPathValue("tenantID", tenantID.String())
	w := httptest.NewRecorder()

	// Create new server just to use the export handler
	server2 := apihttp.New(ctx, store, cfg, logger)
	defer server2.Close()
	server2.ServeHTTP(w, req)

	var events []Event
	json.NewDecoder(w.Body).Decode(&events)

	if len(events) != 3 {
		t.Errorf("\t%s\tExpected 3 events after shutdown, got %d", fail, len(events))
	} else {
		t.Logf("\t%s\tAll events flushed on graceful shutdown", pass)
	}
}
