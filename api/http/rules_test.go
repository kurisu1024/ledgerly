package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	apirules "github.com/kurisu1024/ledgerly/api/rules"
	"github.com/kurisu1024/ledgerly/internal/audit"
	domainrules "github.com/kurisu1024/ledgerly/internal/rules"
	"github.com/kurisu1024/ledgerly/internal/storage"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

// newRuleServer builds a server wired with both event and rule storage,
// signature-verified JWTs (testConfig), and returns the rule store
// directly so tests can assert on it without another round-trip through
// the HTTP API.
func newRuleServer(ctx context.Context, cfg apihttp.Config) (*apihttp.T, *memory.InMemory, *memory.Rules) {
	store := memory.New()
	ruleStore := memory.NewRules()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, ruleStore, cfg, logger)
	return server, store, ruleStore
}

func doJSON(t *testing.T, server *apihttp.T, method, path string, tenantID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshaling request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	return w
}

func newTestRule(eventType string) apirules.Rule {
	return apirules.Rule{
		SchemaVersion: domainrules.SchemaVersion,
		EventType:     eventType,
		LevelAtLeast:  "error",
	}
}

func TestRulesCRUD_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, _, _ := newRuleServer(ctx, testConfig())
	defer server.Close()
	tenantID := uuid.New()

	t.Log("\tWhen creating a rule via POST /v1/rules")
	createW := doJSON(t, server, http.MethodPost, "/v1/rules", tenantID, newTestRule("project.delete"))
	if createW.Code != http.StatusCreated {
		t.Fatalf("\t%s\tExpected 201 creating rule, got %d: %s", fail, createW.Code, createW.Body.String())
	}
	var created apirules.Rule
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("\t%s\tDecoding created rule: %v", fail, err)
	}
	if created.ID == "" {
		t.Fatalf("\t%s\tExpected created rule to have an ID", fail)
	}
	t.Logf("\t%s\tRule created with id %s", pass, created.ID)

	t.Log("\tWhen fetching the rule via GET /v1/rules/{id}")
	getW := doJSON(t, server, http.MethodGet, "/v1/rules/"+created.ID, tenantID, nil)
	if getW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200 fetching rule, got %d: %s", fail, getW.Code, getW.Body.String())
	}

	t.Log("\tWhen listing rules via GET /v1/rules")
	listW := doJSON(t, server, http.MethodGet, "/v1/rules", tenantID, nil)
	if listW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200 listing rules, got %d: %s", fail, listW.Code, listW.Body.String())
	}
	var list apirules.RuleList
	if err := json.NewDecoder(listW.Body).Decode(&list); err != nil {
		t.Fatalf("\t%s\tDecoding rule list: %v", fail, err)
	}
	if list.SchemaVersion != domainrules.SchemaVersion {
		t.Fatalf("\t%s\tExpected envelope schema-version %d, got %d", fail, domainrules.SchemaVersion, list.SchemaVersion)
	}
	if len(list.Rules) != 1 {
		t.Fatalf("\t%s\tExpected 1 rule in list, got %d", fail, len(list.Rules))
	}

	t.Log("\tWhen updating the rule via PUT /v1/rules/{id}")
	updated := created
	updated.LevelAtLeast = "warn"
	updateW := doJSON(t, server, http.MethodPut, "/v1/rules/"+created.ID, tenantID, updated)
	if updateW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200 updating rule, got %d: %s", fail, updateW.Code, updateW.Body.String())
	}

	t.Log("\tWhen deleting the rule via DELETE /v1/rules/{id}")
	deleteW := doJSON(t, server, http.MethodDelete, "/v1/rules/"+created.ID, tenantID, nil)
	if deleteW.Code != http.StatusOK && deleteW.Code != http.StatusNoContent {
		t.Fatalf("\t%s\tExpected 200/204 deleting rule, got %d: %s", fail, deleteW.Code, deleteW.Body.String())
	}

	t.Log("\tThen fetching the deleted rule should 404")
	goneW := doJSON(t, server, http.MethodGet, "/v1/rules/"+created.ID, tenantID, nil)
	if goneW.Code != http.StatusNotFound {
		t.Fatalf("\t%s\tExpected 404 after delete, got %d", fail, goneW.Code)
	}
	t.Logf("\t%s\tFull CRUD round trip verified", pass)
}

func TestRules_CrossTenant_404(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, _, _ := newRuleServer(ctx, testConfig())
	defer server.Close()

	owner := uuid.New()
	intruder := uuid.New()

	createW := doJSON(t, server, http.MethodPost, "/v1/rules", owner, newTestRule("project.delete"))
	if createW.Code != http.StatusCreated {
		t.Fatalf("\t%s\tExpected 201 creating rule, got %d: %s", fail, createW.Code, createW.Body.String())
	}
	var created apirules.Rule
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("\t%s\tDecoding created rule: %v", fail, err)
	}

	t.Log("\tWhen a different tenant fetches the owner's rule by ID")
	getW := doJSON(t, server, http.MethodGet, "/v1/rules/"+created.ID, intruder, nil)
	if getW.Code != http.StatusNotFound {
		t.Fatalf("\t%s\tExpected 404 for cross-tenant GET, got %d: %s", fail, getW.Code, getW.Body.String())
	}

	t.Log("\tWhen a different tenant tries to update the owner's rule")
	updateW := doJSON(t, server, http.MethodPut, "/v1/rules/"+created.ID, intruder, created)
	if updateW.Code != http.StatusNotFound {
		t.Fatalf("\t%s\tExpected 404 for cross-tenant PUT, got %d: %s", fail, updateW.Code, updateW.Body.String())
	}

	t.Log("\tWhen a different tenant tries to delete the owner's rule")
	deleteW := doJSON(t, server, http.MethodDelete, "/v1/rules/"+created.ID, intruder, nil)
	if deleteW.Code != http.StatusNotFound {
		t.Fatalf("\t%s\tExpected 404 for cross-tenant DELETE, got %d: %s", fail, deleteW.Code, deleteW.Body.String())
	}
	t.Logf("\t%s\tCross-tenant rule access correctly 404s", pass)
}

func TestRules_ETag_IfNoneMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, _, _ := newRuleServer(ctx, testConfig())
	defer server.Close()
	tenantID := uuid.New()

	createW := doJSON(t, server, http.MethodPost, "/v1/rules", tenantID, newTestRule("project.delete"))
	if createW.Code != http.StatusCreated {
		t.Fatalf("\t%s\tExpected 201 creating rule, got %d: %s", fail, createW.Code, createW.Body.String())
	}

	t.Log("\tWhen listing rules without If-None-Match")
	req := httptest.NewRequest(http.MethodGet, "/v1/rules", nil)
	req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200, got %d: %s", fail, w.Code, w.Body.String())
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("\t%s\tExpected an ETag header on the rule list response", fail)
	}

	t.Log("\tWhen re-listing with a matching If-None-Match")
	req2 := httptest.NewRequest(http.MethodGet, "/v1/rules", nil)
	req2.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	req2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	server.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotModified {
		t.Fatalf("\t%s\tExpected 304 for matching ETag, got %d: %s", fail, w2.Code, w2.Body.String())
	}

	t.Log("\tWhen re-listing with a stale If-None-Match")
	req3 := httptest.NewRequest(http.MethodGet, "/v1/rules", nil)
	req3.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	req3.Header.Set("If-None-Match", `"stale-etag"`)
	w3 := httptest.NewRecorder()
	server.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200 for stale ETag, got %d", fail, w3.Code)
	}
	t.Logf("\t%s\tETag / If-None-Match semantics verified", pass)
}

// TestRuleMutation_QueueFull_503_StoreUnchanged pins the ADR-0002
// invariant: a rule mutation is never accepted without its audit event.
// The worker is stopped (by cancelling its context) so the queue's single
// slot fills deterministically, forcing the handler's internal enqueue —
// and therefore the whole mutation — to fail.
func TestRuleMutation_QueueFull_503_StoreUnchanged(t *testing.T) {
	workerCtx, cancelWorker := context.WithCancel(context.Background())

	cfg := testConfig()
	cfg.QueueSize = 1
	server, _, ruleStore := newRuleServer(workerCtx, cfg)
	defer server.Close()

	tenantID := uuid.New()

	seeded := domainrules.Rule{
		ID:            uuid.New(),
		TenantID:      tenantID,
		SchemaVersion: domainrules.SchemaVersion,
		EventType:     "project.delete",
		LevelAtLeast:  "error",
	}
	if _, err := ruleStore.MutateRules(context.Background(), tenantID, func(current []domainrules.Rule) ([]domainrules.Rule, error) {
		return append(current, seeded), nil
	}); err != nil {
		t.Fatalf("\t%s\tSeeding rule store: %v", fail, err)
	}

	t.Log("\tGiven the worker has stopped draining the queue")
	cancelWorker()
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer flushCancel()
	for {
		err := server.Flush(flushCtx)
		if errors.Is(err, audit.ErrWorkerStopped) {
			break
		}
		if err != nil {
			t.Fatalf("\t%s\tWaiting for worker to stop: %v", fail, err)
		}
		time.Sleep(time.Millisecond)
	}

	t.Log("\tWhen the queue's only slot is filled by an unrelated event")
	fillW := doJSON(t, server, http.MethodPost, "/v1/events", tenantID, map[string]any{
		"action":   "noise",
		"actor":    map[string]string{"id": "u", "type": "user"},
		"resource": map[string]string{"type": "x", "id": "y"},
	})
	if fillW.Code != http.StatusAccepted {
		t.Fatalf("\t%s\tExpected the filler event to be queued (202), got %d: %s", fail, fillW.Code, fillW.Body.String())
	}

	t.Log("\tAnd a rule mutation is attempted")
	mutW := doJSON(t, server, http.MethodPost, "/v1/rules", tenantID, newTestRule("project.create"))
	if mutW.Code != http.StatusServiceUnavailable {
		t.Fatalf("\t%s\tExpected 503 when the audit queue is full, got %d: %s", fail, mutW.Code, mutW.Body.String())
	}

	t.Log("\tThen the rule store must be unchanged")
	got, err := ruleStore.ListRules(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("\t%s\tListRules: %v", fail, err)
	}
	if len(got) != 1 || got[0].ID != seeded.ID {
		t.Fatalf("\t%s\tExpected only the seeded rule to remain, got %+v", fail, got)
	}
	t.Logf("\t%s\tQueue-full mutation correctly aborted, store unchanged", pass)
}

// TestRules_ChainedAuditEvent_OnCreate pins the ADR-0002 requirement that
// every rule mutation is itself a chained audit event: actor is the JWT
// subject, resource identifies the rule, metadata carries old/new
// canonical JSON and the post-mutation ETag, and the resulting chain still
// verifies.
func TestRules_ChainedAuditEvent_OnCreate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := testConfig()
	cfg.ChainSize = 100
	cfg.FlushInterval = time.Hour
	server, store, _ := newRuleServer(ctx, cfg)
	defer server.Close()

	tenantID := uuid.New()

	createW := doJSON(t, server, http.MethodPost, "/v1/rules", tenantID, newTestRule("project.delete"))
	if createW.Code != http.StatusCreated {
		t.Fatalf("\t%s\tExpected 201 creating rule, got %d: %s", fail, createW.Code, createW.Body.String())
	}
	var created apirules.Rule
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("\t%s\tDecoding created rule: %v", fail, err)
	}

	if err := server.Flush(ctx); err != nil {
		t.Fatalf("\t%s\tFlush: %v", fail, err)
	}

	exportW := doJSON(t, server, http.MethodGet, "/v1/export", tenantID, nil)
	if exportW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200 exporting events, got %d: %s", fail, exportW.Code, exportW.Body.String())
	}
	var exported []Event
	if err := json.NewDecoder(exportW.Body).Decode(&exported); err != nil {
		t.Fatalf("\t%s\tDecoding exported events: %v", fail, err)
	}

	var ruleEvent *Event
	for i := range exported {
		if exported[i].Action == domainrules.ActionRuleCreated {
			ruleEvent = &exported[i]
		}
	}
	if ruleEvent == nil {
		t.Fatalf("\t%s\tExpected a %s event in the export, got actions: %+v", fail, domainrules.ActionRuleCreated, exported)
	}
	if ruleEvent.Actor["sub"] != "test-user" {
		t.Fatalf("\t%s\tExpected actor sub %q, got %+v", fail, "test-user", ruleEvent.Actor)
	}
	if ruleEvent.Resource["type"] != "trigger-rule" || ruleEvent.Resource["id"] != created.ID {
		t.Fatalf("\t%s\tExpected resource {trigger-rule, %s}, got %+v", fail, created.ID, ruleEvent.Resource)
	}
	if ruleEvent.Metadata["new-rule"] == "" {
		t.Fatalf("\t%s\tExpected metadata[new-rule] to carry the created rule's canonical JSON", fail)
	}
	if ruleEvent.Metadata["old-rule"] != "" {
		t.Fatalf("\t%s\tExpected no old-rule metadata on create, got %q", fail, ruleEvent.Metadata["old-rule"])
	}
	if ruleEvent.Metadata["etag"] == "" {
		t.Fatalf("\t%s\tExpected metadata[etag] to carry the post-mutation ETag", fail)
	}

	blocks, err := store.FetchBlocks(ctx, tenantID, storage.FetchOptions{})
	if err != nil {
		t.Fatalf("\t%s\tFetchBlocks: %v", fail, err)
	}
	for _, block := range blocks {
		if !audit.VerifyChain(block.Chain) {
			t.Fatalf("\t%s\tExpected chain %s to verify after rule-mutation event", fail, block.Chain.ID)
		}
	}
	t.Logf("\t%s\tRule creation chained as a verifiable audit event", pass)
}
