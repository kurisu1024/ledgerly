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

func TestCreateEvent(t *testing.T) {
	t.Log("\tGiven an HTTP server with in-memory storage")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := apihttp.DefaultConfig()
	server := apihttp.New(ctx, store, cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

	event := Event{
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
			"reason": "test event",
		},
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("\t%s\tFailed to marshal event: %v", fail, err)
	}

	t.Log("\tWhen creating an event via POST")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tenants/%s/events", tenantID), bytes.NewReader(eventJSON))
	req.SetPathValue("tenantID", tenantID.String())
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusAccepted, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tSuccessfully queued event with status %d", pass, w.Code)

	var createdEvent Event
	if err := json.NewDecoder(w.Body).Decode(&createdEvent); err != nil {
		t.Fatalf("\t%s\tFailed to decode response: %v", fail, err)
	}

	t.Log("\tThen the created event should have an ID and tenant ID")
	if createdEvent.ID == "" {
		t.Errorf("\t%s\tEvent ID is empty", fail)
	}
	if createdEvent.TenantID != tenantID.String() {
		t.Errorf("\t%s\tTenant ID mismatch: got %s, want %s", fail, createdEvent.TenantID, tenantID.String())
	}
	if createdEvent.Action != event.Action {
		t.Errorf("\t%s\tAction mismatch: got %s, want %s", fail, createdEvent.Action, event.Action)
	}
	t.Logf("\t%s\tCreated event has correct fields", pass)
}

func TestExportEvents(t *testing.T) {
	t.Log("\tGiven an HTTP server with stored events")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := apihttp.DefaultConfig()
	cfg.FlushInterval = 100 * time.Millisecond
	server := apihttp.New(ctx, store, cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

	// Create an event first
	event := Event{
		OccurredAt: time.Now().UTC(),
		Action:     "project.create",
		Actor: map[string]string{
			"id":   "user123",
			"type": "user",
		},
		Resource: map[string]string{
			"type": "project",
			"id":   "proj123",
		},
	}

	eventJSON, _ := json.Marshal(event)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tenants/%s/events", tenantID), bytes.NewReader(eventJSON))
	req.SetPathValue("tenantID", tenantID.String())
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("\t%s\tFailed to create event: status %d", fail, w.Code)
	}

	// Wait for worker to process and flush
	time.Sleep(200 * time.Millisecond)

	t.Log("\tWhen exporting events via GET")
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tenants/%s/events", tenantID), nil)
	req.SetPathValue("tenantID", tenantID.String())
	w = httptest.NewRecorder()

	server.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusOK, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tSuccessfully exported events with status %d", pass, w.Code)

	var events []Event
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("\t%s\tFailed to decode response: %v", fail, err)
	}

	t.Log("\tThen the response should contain the created event")
	if len(events) == 0 {
		t.Fatalf("\t%s\tExpected at least 1 event, got 0", fail)
	}
	if events[0].Action != event.Action {
		t.Errorf("\t%s\tAction mismatch: got %s, want %s", fail, events[0].Action, event.Action)
	}
	t.Logf("\t%s\tExported events contain correct data", pass)
}

func TestInvalidTenantID(t *testing.T) {
	t.Log("\tGiven an HTTP server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := apihttp.DefaultConfig()
	server := apihttp.New(ctx, store, cfg, logger)
	defer server.Close()

	t.Log("\tWhen creating an event with an invalid tenant ID")
	event := Event{Action: "test"}
	eventJSON, _ := json.Marshal(event)

	req := httptest.NewRequest(http.MethodPost, "/tenants/invalid-uuid/events", bytes.NewReader(eventJSON))
	req.SetPathValue("tenantID", "invalid-uuid")
	w := httptest.NewRecorder()

	server.ServeHTTP(w, req)

	t.Log("\tThen the request should fail with 400")
	if w.Code != http.StatusBadRequest {
		t.Errorf("\t%s\tExpected status %d, got %d", fail, http.StatusBadRequest, w.Code)
	} else {
		t.Logf("\t%s\tCorrectly rejected invalid tenant ID with status %d", pass, w.Code)
	}
}
