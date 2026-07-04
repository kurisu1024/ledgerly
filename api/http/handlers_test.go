package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	cfg := testConfig()
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
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

	var createdEvent Event
	{
		t.Log("\tWhen creating an event via POST")
		eventJSON, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("\t%s\tFailed to marshal event: %v", fail, err)
		}

		req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
		req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusAccepted, w.Code, w.Body.String())
		}
		t.Logf("\t%s\tSuccessfully queued event with status %d\n", pass, w.Code)

		if err := json.NewDecoder(w.Body).Decode(&createdEvent); err != nil {
			t.Fatalf("\t%s\tFailed to decode response: %v", fail, err)
		}
	}

	{
		t.Log("\tThen the created event should have an ID and tenant ID")
		if createdEvent.ID == "" {
			t.Fatalf("\t%s\tEvent ID is empty", fail)
		}
		if createdEvent.TenantID != tenantID.String() {
			t.Fatalf("\t%s\tTenant ID mismatch: got %s, want %s", fail, createdEvent.TenantID, tenantID.String())
		}
		if createdEvent.Action != event.Action {
			t.Fatalf("\t%s\tAction mismatch: got %s, want %s", fail, createdEvent.Action, event.Action)
		}
		t.Logf("\t%s\tCreated event has correct fields\n", pass)
	}
}

func TestExportEvents(t *testing.T) {
	t.Log("\tGiven an HTTP server with stored events")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()
	cfg.FlushInterval = 100 * time.Millisecond
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

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

	{
		t.Log("\tWhen creating an event")
		eventJSON, _ := json.Marshal(event)
		req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
		req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("\t%s\tFailed to create event: status %d", fail, w.Code)
		}
		t.Logf("\t%s\tSuccessfully created event\n", pass)

		if err := server.Flush(ctx); err != nil {
			t.Fatalf("\t%s\tFailed to flush worker: %v", fail, err)
		}
	}

	var events []Event
	{
		t.Log("\tWhen exporting events via GET")
		req := httptest.NewRequest(http.MethodGet, "/v1/export", nil)
		req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusOK, w.Code, w.Body.String())
		}
		t.Logf("\t%s\tSuccessfully exported events with status %d\n", pass, w.Code)

		if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
			t.Fatalf("\t%s\tFailed to decode response: %v", fail, err)
		}
	}

	{
		t.Log("\tThen the response should contain the created event")
		if len(events) == 0 {
			t.Fatalf("\t%s\tExpected at least 1 event, got 0", fail)
		}
		if events[0].Action != event.Action {
			t.Fatalf("\t%s\tAction mismatch: got %s, want %s", fail, events[0].Action, event.Action)
		}
		t.Logf("\t%s\tExported events contain correct data\n", pass)
	}
}

func TestMissingAuthHeader(t *testing.T) {
	t.Log("\tGiven an HTTP server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	{
		t.Log("\tWhen creating an event without Authorization header")
		event := Event{Action: "test"}
		eventJSON, _ := json.Marshal(event)

		req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("\t%s\tExpected status %d, got %d", fail, http.StatusUnauthorized, w.Code)
		}
		t.Logf("\t%s\tCorrectly rejected missing auth header with status %d\n", pass, w.Code)
	}
}

func TestInvalidTenantID(t *testing.T) {
	t.Log("\tGiven an HTTP server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	{
		t.Log("\tWhen creating an event with an invalid JWT token")
		event := Event{Action: "test"}
		eventJSON, _ := json.Marshal(event)

		// Properly signed token with an invalid tenant ID, so the failure
		// exercised is the tenant check, not the signature check.
		invalidJWT := signJWT(jwt.MapClaims{
			"tenant_id": "invalid-uuid",
			"sub":       "test-user",
			"exp":       time.Now().Add(time.Hour).Unix(),
		})

		req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
		req.Header.Set("Authorization", "Bearer "+invalidJWT)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("\t%s\tExpected status %d, got %d", fail, http.StatusUnauthorized, w.Code)
		}
		t.Logf("\t%s\tCorrectly rejected invalid tenant ID with status %d\n", pass, w.Code)
	}
}
