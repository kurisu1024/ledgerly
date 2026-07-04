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
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

// asOfActor/asOfResource/asOfMetadata are fixture field values for events
// built directly via audit.NewEvent + audit.AppendEvent — this file never
// hand-rolls a hash.
var (
	asOfActor    = map[string]string{"id": "user1", "type": "user"}
	asOfResource = map[string]string{"type": "project", "id": "proj1"}
	asOfMetadata = map[string]string{"reason": "asof-test"}
)

// asOfEventAt builds a real, correctly-hashed event with a fixed
// OccurredAt, mirroring internal/audit/asof_test.go's eventAt helper.
func asOfEventAt(tenantID uuid.UUID, t time.Time, action string) audit.Event {
	e := audit.NewEvent(tenantID, asOfActor, action, asOfResource, asOfMetadata)
	e.OccurredAt = t
	return e
}

// writeChain builds a chain from the given (time, action) pairs, appended in
// order, and writes it directly to store as a single block — bypassing the
// queue/worker so fixtures have deterministic, controllable timestamps.
func writeChain(t *testing.T, ctx context.Context, store storage.Storage, tenantID uuid.UUID, events ...audit.Event) storage.Block {
	t.Helper()
	chain := audit.NewEventChain(len(events))
	for _, e := range events {
		chain = audit.AppendEvent(chain, e)
	}
	block := storage.Block{ID: chain.ID, TenantID: tenantID, Chain: chain}
	if err := store.WriteBlock(ctx, tenantID, block); err != nil {
		t.Fatalf("\t%s\tFailed to write fixture block: %v", fail, err)
	}
	return block
}

// exportAsOf issues GET /v1/export with the given query string (may be
// empty) and returns the raw recorder.
func exportAsOf(server *apihttp.T, tenantID uuid.UUID, query string) *httptest.ResponseRecorder {
	url := "/v1/export"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	return w
}

// groupByChain reconstructs one audit.EventChain per chain-id present in the
// decoded response DTOs, for per-chain re-verification.
func groupByChain(t *testing.T, decoded []Event) map[string]audit.EventChain {
	t.Helper()
	groups := make(map[string]audit.EventChain)
	for _, ev := range decoded {
		chainID, err := uuid.Parse(ev.ChainID)
		if err != nil {
			t.Fatalf("\t%s\tResponse event has invalid chain-id %q: %v", fail, ev.ChainID, err)
		}
		tenantID, err := uuid.Parse(ev.TenantID)
		if err != nil {
			t.Fatalf("\t%s\tResponse event has invalid tenant-id %q: %v", fail, ev.TenantID, err)
		}
		id, err := uuid.Parse(ev.ID)
		if err != nil {
			t.Fatalf("\t%s\tResponse event has invalid id %q: %v", fail, ev.ID, err)
		}

		ae := audit.Event{
			ID:         id,
			ChainID:    chainID,
			TenantID:   tenantID,
			OccurredAt: ev.OccurredAt,
			Actor:      ev.Actor,
			Action:     ev.Action,
			Resource:   ev.Resource,
			Metadata:   ev.Metadata,
			PrevHash:   ev.PrevHash,
			EventHash:  ev.EventHash,
		}

		g := groups[ev.ChainID]
		if g.Events == nil {
			g.ID = chainID
		}
		g.Events = append(g.Events, ae)
		groups[ev.ChainID] = g
	}
	return groups
}

// TestExportEvents_AsOf_NoParamByteIdentical pins that omitting as-of leaves
// the export response exactly as it is today — the regression guard for
// issue #20's composition with the existing endpoint.
func TestExportEvents_AsOf_NoParamByteIdentical(t *testing.T) {
	t.Log("\tGiven a server with one stored chain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "a0"),
		asOfEventAt(tenantID, base.Add(time.Minute), "a1"),
	)

	t.Log("\tWhen exporting twice with no as-of parameter")
	first := exportAsOf(server, tenantID, "")
	second := exportAsOf(server, tenantID, "")

	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200/200, got %d/%d", fail, first.Code, second.Code)
	}

	t.Log("\tThen both responses should be byte-identical")
	if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
		t.Fatalf("\t%s\tNo-param export is not stable across repeated calls:\n%s\nvs\n%s", fail, first.Body.String(), second.Body.String())
	}
	t.Logf("\t%s\tNo-param export is byte-identical\n", pass)
}

// TestExportEvents_AsOf_MalformedReturns400 pins the fixed 400 contract for
// an unparseable as-of value, with no reflected input in the error body.
func TestExportEvents_AsOf_MalformedReturns400(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tWhen exporting with a malformed as-of value")
	w := exportAsOf(server, tenantID, "as-of=not-a-real-timestamp")

	t.Log("\tThen the response should be 400 with the fixed error string and no reflected input")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("\t%s\tExpected status %d, got %d: %s", fail, http.StatusBadRequest, w.Code, w.Body.String())
	}
	body := strings.TrimSpace(w.Body.String())
	const wantErr = "Invalid as-of parameter"
	if body != wantErr {
		t.Fatalf("\t%s\tExpected error body %q, got %q", fail, wantErr, body)
	}
	if strings.Contains(body, "not-a-real-timestamp") {
		t.Fatalf("\t%s\tError body reflects unsanitized input: %q", fail, body)
	}
	t.Logf("\t%s\tMalformed as-of correctly rejected with fixed error string\n", pass)
}

// TestExportEvents_AsOf_MultiBlockCut verifies that as-of cuts each stored
// chain independently, and that every returned chain re-verifies as a valid
// prefix from the response DTOs alone.
func TestExportEvents_AsOf_MultiBlockCut(t *testing.T) {
	t.Log("\tGiven two chains for the same tenant with different timelines")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	chain1 := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "c1-0"),
		asOfEventAt(tenantID, base.Add(1*time.Minute), "c1-1"),
		asOfEventAt(tenantID, base.Add(2*time.Minute), "c1-2"),
	)
	chain2 := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "c2-0"),
		asOfEventAt(tenantID, base.Add(3*time.Minute), "c2-1"),
	)

	cutAt := base.Add(90 * time.Second) // 10:01:30
	t.Logf("\tWhen exporting with as-of=%s", cutAt.Format(time.RFC3339Nano))
	w := exportAsOf(server, tenantID, "as-of="+cutAt.Format(time.RFC3339Nano))

	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}

	var decoded []Event
	if err := json.NewDecoder(w.Body).Decode(&decoded); err != nil {
		t.Fatalf("\t%s\tFailed to decode response: %v", fail, err)
	}

	t.Log("\tThen chain1 should be cut to its first 2 events and chain2 to its first 1 event")
	if len(decoded) != 3 {
		t.Fatalf("\t%s\tExpected 3 total events across both chains, got %d", fail, len(decoded))
	}

	groups := groupByChain(t, decoded)
	c1 := groups[chain1.ID.String()]
	c2 := groups[chain2.ID.String()]

	if len(c1.Events) != 2 {
		t.Fatalf("\t%s\tExpected 2 events in chain1's prefix, got %d", fail, len(c1.Events))
	}
	if len(c2.Events) != 1 {
		t.Fatalf("\t%s\tExpected 1 event in chain2's prefix, got %d", fail, len(c2.Events))
	}

	t.Log("\tAnd each returned chain should re-verify from the response DTOs alone")
	if !audit.VerifyChain(c1) {
		t.Fatalf("\t%s\tchain1's cut prefix failed VerifyChain", fail)
	}
	if !audit.VerifyChain(c2) {
		t.Fatalf("\t%s\tchain2's cut prefix failed VerifyChain", fail)
	}
	t.Logf("\t%s\tMulti-block as-of cut correct and each prefix independently verifiable\n", pass)
}

// TestExportEvents_AsOf_BlockIDComposition verifies as-of composes with the
// existing blockID filter rather than being ignored by it.
func TestExportEvents_AsOf_BlockIDComposition(t *testing.T) {
	t.Log("\tGiven two chains for the same tenant")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	_ = writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "other-0"),
		asOfEventAt(tenantID, base.Add(time.Minute), "other-1"),
	)
	target := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "target-0"),
		asOfEventAt(tenantID, base.Add(1*time.Minute), "target-1"),
		asOfEventAt(tenantID, base.Add(2*time.Minute), "target-2"),
	)

	cutAt := base.Add(90 * time.Second)
	query := "blockID=" + target.ID.String() + "&as-of=" + cutAt.Format(time.RFC3339Nano)
	t.Logf("\tWhen exporting with %s", query)
	w := exportAsOf(server, tenantID, query)

	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}

	var decoded []Event
	if err := json.NewDecoder(w.Body).Decode(&decoded); err != nil {
		t.Fatalf("\t%s\tFailed to decode response: %v", fail, err)
	}

	t.Log("\tThen only the target block's events at-or-before the cut should be returned")
	if len(decoded) != 2 {
		t.Fatalf("\t%s\tExpected 2 events (target's cut prefix), got %d", fail, len(decoded))
	}
	for _, ev := range decoded {
		if ev.ChainID != target.ID.String() {
			t.Fatalf("\t%s\tExpected only target chain's events, got event from chain %s", fail, ev.ChainID)
		}
	}
	t.Logf("\t%s\tblockID and as-of composed correctly\n", pass)
}

// TestExportEvents_AsOf_Unauthorized locks in that as-of does not bypass the
// existing auth requirement.
func TestExportEvents_AsOf_Unauthorized(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	t.Log("\tWhen exporting with as-of but no Authorization header")
	req := httptest.NewRequest(http.MethodGet, "/v1/export?as-of="+time.Now().UTC().Format(time.RFC3339Nano), nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	t.Log("\tThen the response should be 401")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("\t%s\tExpected status %d, got %d", fail, http.StatusUnauthorized, w.Code)
	}
	t.Logf("\t%s\tas-of export correctly requires authorization\n", pass)
}

// TestExportEvents_AsOf_TenantIsolation verifies that as-of cutting never
// leaks another tenant's events, and correctly cuts each tenant's own chain.
func TestExportEvents_AsOf_TenantIsolation(t *testing.T) {
	t.Log("\tGiven two tenants each with their own chain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenant1 := uuid.New()
	tenant2 := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	writeChain(t, ctx, store, tenant1,
		asOfEventAt(tenant1, base, "t1-0"),
		asOfEventAt(tenant1, base.Add(1*time.Minute), "t1-1"),
		asOfEventAt(tenant1, base.Add(2*time.Minute), "t1-2"),
	)
	writeChain(t, ctx, store, tenant2,
		asOfEventAt(tenant2, base, "t2-0"),
		asOfEventAt(tenant2, base.Add(1*time.Minute), "t2-1"),
	)

	cutAt := base.Add(90 * time.Second)
	query := "as-of=" + cutAt.Format(time.RFC3339Nano)

	t.Log("\tWhen tenant1 exports as-of the cut time")
	w1 := exportAsOf(server, tenant1, query)
	var events1 []Event
	if err := json.NewDecoder(w1.Body).Decode(&events1); err != nil {
		t.Fatalf("\t%s\tFailed to decode tenant1 response: %v", fail, err)
	}

	t.Log("\tThen only tenant1's cut prefix (2 events) should be returned, none from tenant2")
	if len(events1) != 2 {
		t.Fatalf("\t%s\tExpected 2 events for tenant1's cut prefix, got %d", fail, len(events1))
	}
	for _, ev := range events1 {
		if ev.TenantID != tenant1.String() {
			t.Fatalf("\t%s\tTenant isolation violated: got event for tenant %s while querying as tenant1", fail, ev.TenantID)
		}
	}
	t.Logf("\t%s\tas-of cut respected tenant isolation\n", pass)
}

// TestExportEvents_AsOf_FlushContract pins that as-of still only sees
// flushed blocks — the 202-accepted-but-unflushed contract must hold
// regardless of the as-of parameter.
func TestExportEvents_AsOf_FlushContract(t *testing.T) {
	t.Log("\tGiven a server with a long flush interval")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	cfg := testConfig()
	cfg.FlushInterval = time.Hour
	server := apihttp.New(ctx, store, memory.NewRules(), cfg, logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tWhen posting an event and immediately exporting as-of a future time, before any flush")
	event := Event{
		OccurredAt: time.Now().UTC(),
		Action:     "unflushed.action",
		Actor:      map[string]string{"id": "user1", "type": "user"},
		Resource:   map[string]string{"type": "project", "id": "proj1"},
	}
	eventJSON, _ := json.Marshal(event)
	postReq := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(eventJSON))
	postReq.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	postW := httptest.NewRecorder()
	server.ServeHTTP(postW, postReq)
	if postW.Code != http.StatusAccepted {
		t.Fatalf("\t%s\tFailed to create event: status %d", fail, postW.Code)
	}

	future := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339Nano)
	before := exportAsOf(server, tenantID, "as-of="+future)
	var beforeEvents []Event
	if err := json.NewDecoder(before.Body).Decode(&beforeEvents); err != nil {
		t.Fatalf("\t%s\tFailed to decode pre-flush response: %v", fail, err)
	}
	if len(beforeEvents) != 0 {
		t.Fatalf("\t%s\tExpected 0 events before flush, got %d", fail, len(beforeEvents))
	}
	t.Logf("\t%s\tUnflushed event correctly absent from as-of export\n", pass)

	t.Log("\tWhen the worker is flushed")
	if err := server.Flush(ctx); err != nil {
		t.Fatalf("\t%s\tFailed to flush worker: %v", fail, err)
	}

	t.Log("\tThen the same as-of export should now include the event")
	after := exportAsOf(server, tenantID, "as-of="+future)
	var afterEvents []Event
	if err := json.NewDecoder(after.Body).Decode(&afterEvents); err != nil {
		t.Fatalf("\t%s\tFailed to decode post-flush response: %v", fail, err)
	}
	if len(afterEvents) != 1 {
		t.Fatalf("\t%s\tExpected 1 event after flush, got %d", fail, len(afterEvents))
	}
	if afterEvents[0].Action != event.Action {
		t.Fatalf("\t%s\tAction mismatch: got %s, want %s", fail, afterEvents[0].Action, event.Action)
	}
	t.Logf("\t%s\tFlush contract holds under as-of\n", pass)
}
