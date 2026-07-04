package http_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	apirules "github.com/kurisu1024/ledgerly/api/rules"
	"github.com/kurisu1024/ledgerly/internal/audit"
	domainrules "github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

var (
	pass = "\u2705"
	fail = "\u274C"
)

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

// testKey signs test JWTs; servers built with testConfig verify against its
// public half.
var testKey = func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}()

// testConfig is DefaultConfig with signature verification enabled.
func testConfig() apihttp.Config {
	cfg := apihttp.DefaultConfig()
	cfg.JWTPublicKey = &testKey.PublicKey
	return cfg
}

// createJWT creates an RS256-signed JWT for testing.
func createJWT(tenantID uuid.UUID) string {
	return signJWT(jwt.MapClaims{
		"tenant_id": tenantID.String(),
		"sub":       "test-user",
		"iat":       time.Now().Unix(),
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
	})
}

// signJWT signs arbitrary claims with the test key.
func signJWT(claims jwt.MapClaims) string {
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(testKey)
	if err != nil {
		panic(err)
	}
	return s
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
		JWTPublicKey:  &testKey.PublicKey,
	}

	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	tenantID := uuid.New()
	eventCount := 5

	{
		t.Log("\tWhen creating multiple events")
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
			req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
			req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code != http.StatusAccepted {
				t.Fatalf("\t%s\tFailed to create event %d: status %d, body: %s", fail, i, w.Code, w.Body.String())
			}
		}
		t.Logf("\t%s\tSuccessfully queued %d events\n", pass, eventCount)

		// 5 events with chain size 3 → 1 full chain written on ingest,
		// 1 partial chain of 2 written by the flush.
		if err := server.Flush(ctx); err != nil {
			t.Fatalf("\t%s\tFailed to flush worker: %v", fail, err)
		}
	}

	var exportedEvents []Event
	{
		t.Log("\tWhen exporting events via GET")
		req := httptest.NewRequest(http.MethodGet, "/v1/export", nil)
		req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("\t%s\tFailed to export events: status %d, body: %s", fail, w.Code, w.Body.String())
		}

		if err := json.NewDecoder(w.Body).Decode(&exportedEvents); err != nil {
			t.Fatalf("\t%s\tFailed to decode exported events: %v", fail, err)
		}
		t.Logf("\t%s\tSuccessfully exported events\n", pass)
	}

	{
		t.Log("\tThen all events should be present and properly chained")
		if len(exportedEvents) != eventCount {
			t.Fatalf("\t%s\tExpected %d events, got %d", fail, eventCount, len(exportedEvents))
		}

		for i, event := range exportedEvents {
			if event.ChainID == "" {
				t.Fatalf("\t%s\tEvent %d is missing chain ID", fail, i)
			}
			if event.EventHash == nil || len(event.EventHash) == 0 {
				t.Fatalf("\t%s\tEvent %d is missing event hash", fail, i)
			}
			if event.TenantID != tenantID.String() {
				t.Fatalf("\t%s\tEvent %d has wrong tenant ID: %s", fail, i, event.TenantID)
			}
		}
		t.Logf("\t%s\tVerified all %d events and chain integrity\n", pass, len(exportedEvents))
	}
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
		JWTPublicKey:  &testKey.PublicKey,
	}

	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	tenant1 := uuid.New()
	tenant2 := uuid.New()

	{
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
				req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
				req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
				w := httptest.NewRecorder()

				server.ServeHTTP(w, req)

				if w.Code != http.StatusAccepted {
					t.Fatalf("\t%s\tFailed to create event for tenant %d: %d", fail, tenantIdx, w.Code)
				}
			}
		}
		t.Logf("\t%s\tSuccessfully queued events for multiple tenants\n", pass)

		if err := server.Flush(ctx); err != nil {
			t.Fatalf("\t%s\tFailed to flush worker: %v", fail, err)
		}
	}

	{
		t.Log("\tThen each tenant should only see their own events")
		for tenantIdx, tenantID := range []uuid.UUID{tenant1, tenant2} {
			req := httptest.NewRequest(http.MethodGet, "/v1/export", nil)
			req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			var events []Event
			json.NewDecoder(w.Body).Decode(&events)

			if len(events) != 3 {
				t.Fatalf("\t%s\tTenant %d expected 3 events, got %d", fail, tenantIdx, len(events))
			}

			// Verify all events belong to this tenant
			for _, event := range events {
				if event.TenantID != tenantID.String() {
					t.Fatalf("\t%s\tEvent belongs to wrong tenant: %s", fail, event.TenantID)
				}
			}
		}
		t.Logf("\t%s\tEach tenant correctly isolated\n", pass)
	}
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
		JWTPublicKey:  &testKey.PublicKey,
	}

	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)

	tenantID := uuid.New()

	{
		t.Log("\tWhen creating events and shutting down before flush interval")
		for i := 0; i < 3; i++ {
			event := Event{
				OccurredAt: time.Now().UTC(),
				Action:     fmt.Sprintf("action.%d", i),
				Actor:      map[string]string{"id": "user1", "type": "user"},
				Resource:   map[string]string{"type": "project", "id": "proj1"},
			}

			eventJSON, _ := json.Marshal(event)
			req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
			req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
		}

		server.Close()

		// Give it a moment to complete shutdown
		time.Sleep(100 * time.Millisecond)
	}

	{
		t.Log("\tThen events should be flushed to storage on shutdown")
		req := httptest.NewRequest(http.MethodGet, "/v1/export", nil)
		req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
		w := httptest.NewRecorder()

		// Create new server just to use the export handler
		server2 := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
		defer server2.Close()
		server2.ServeHTTP(w, req)

		var events []Event
		json.NewDecoder(w.Body).Decode(&events)

		if len(events) != 3 {
			t.Fatalf("\t%s\tExpected 3 events after shutdown, got %d", fail, len(events))
		}
		t.Logf("\t%s\tAll events flushed on graceful shutdown\n", pass)
	}
}

// TestEndToEnd_RuleLifecycleChained exercises the full ADR-0002 contract:
// create, update, and delete a rule, flush the worker, export the tenant's
// events, and verify each mutation landed as its own chained audit event
// with the chain still verifying end to end.
func TestEndToEnd_RuleLifecycleChained(t *testing.T) {
	t.Log("\tGiven an HTTP server with rule and event storage")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	cfg.ChainSize = 100
	cfg.FlushInterval = time.Hour

	server, store, _ := newRuleServer(ctx, cfg)
	defer server.Close()

	tenantID := uuid.New()

	var ruleID string
	{
		t.Log("\tWhen creating a rule")
		w := doJSON(t, server, http.MethodPost, "/v1/rules", tenantID, newTestRule("project.delete"))
		if w.Code != http.StatusCreated {
			t.Fatalf("\t%s\tExpected 201 creating rule, got %d: %s", fail, w.Code, w.Body.String())
		}
		var created apirules.Rule
		if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
			t.Fatalf("\t%s\tDecoding created rule: %v", fail, err)
		}
		ruleID = created.ID
		t.Logf("\t%s\tRule created with id %s", pass, ruleID)
	}

	{
		t.Log("\tWhen updating the rule")
		updated := apirules.Rule{
			ID:            ruleID,
			SchemaVersion: domainrules.SchemaVersion,
			EventType:     "project.delete",
			LevelAtLeast:  "warn",
		}
		w := doJSON(t, server, http.MethodPut, "/v1/rules/"+ruleID, tenantID, updated)
		if w.Code != http.StatusOK {
			t.Fatalf("\t%s\tExpected 200 updating rule, got %d: %s", fail, w.Code, w.Body.String())
		}
	}

	{
		t.Log("\tWhen deleting the rule")
		w := doJSON(t, server, http.MethodDelete, "/v1/rules/"+ruleID, tenantID, nil)
		if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
			t.Fatalf("\t%s\tExpected 200/204 deleting rule, got %d: %s", fail, w.Code, w.Body.String())
		}
	}

	if err := server.Flush(ctx); err != nil {
		t.Fatalf("\t%s\tFlush: %v", fail, err)
	}

	t.Log("\tThen the export should contain three chained rule-mutation events")
	exportW := doJSON(t, server, http.MethodGet, "/v1/export", tenantID, nil)
	if exportW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200 exporting events, got %d: %s", fail, exportW.Code, exportW.Body.String())
	}
	var exported []Event
	if err := json.NewDecoder(exportW.Body).Decode(&exported); err != nil {
		t.Fatalf("\t%s\tDecoding exported events: %v", fail, err)
	}

	wantActions := []string{
		domainrules.ActionRuleCreated,
		domainrules.ActionRuleUpdated,
		domainrules.ActionRuleDeleted,
	}
	for _, action := range wantActions {
		found := false
		for _, e := range exported {
			if e.Action == action && e.Resource["id"] == ruleID {
				found = true
			}
		}
		if !found {
			t.Fatalf("\t%s\tExpected a %s event for rule %s in export, got: %+v", fail, action, ruleID, exported)
		}
	}

	blocks, err := store.FetchBlocks(ctx, tenantID, storage.FetchOptions{})
	if err != nil {
		t.Fatalf("\t%s\tFetchBlocks: %v", fail, err)
	}
	for _, block := range blocks {
		if !audit.VerifyChain(block.Chain) {
			t.Fatalf("\t%s\tExpected chain %s to verify after rule lifecycle", fail, block.Chain.ID)
		}
	}
	t.Logf("\t%s\tRule lifecycle fully chained and verifiable\n", pass)
}
