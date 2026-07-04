package http_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	apihttp "github.com/kurisu1024/ledgerly/api/http"
	"github.com/kurisu1024/ledgerly/internal/audit"
	"github.com/kurisu1024/ledgerly/internal/storage/memory"
	"go.uber.org/zap"
)

// verifyChain issues POST /v1/verify-chain with the given body and, unless
// tenantID is uuid.Nil, an Authorization header for tenantID.
func verifyChain(server *apihttp.T, tenantID uuid.UUID, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/verify-chain", bytes.NewReader(body))
	if tenantID != uuid.Nil {
		req.Header.Set("Authorization", "Bearer "+createJWT(tenantID))
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	return w
}

// decodeVerifyResponse decodes a 200 response body into
// apihttp.VerifyChainResponse, failing the test on decode error.
func decodeVerifyResponse(t *testing.T, w *httptest.ResponseRecorder) apihttp.VerifyChainResponse {
	t.Helper()
	var resp apihttp.VerifyChainResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("\t%s\tFailed to decode verify-chain response: %v (body: %s)", fail, err, w.Body.String())
	}
	return resp
}

// verifyChainRequestJSON marshals a compare-mode request body for one
// chain's submitted hashes.
func verifyChainRequestJSON(t *testing.T, chainID uuid.UUID, hashes [][]byte) []byte {
	t.Helper()
	req := apihttp.VerifyChainRequest{
		Chains: []apihttp.VerifyChainQuery{
			{ChainID: chainID.String(), EventHashes: hashes},
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("\t%s\tFailed to marshal verify-chain request: %v", fail, err)
	}
	return b
}

// hashesOfChain extracts the event hashes of chain, in chain order.
func hashesOfChain(chain audit.EventChain) [][]byte {
	hashes := make([][]byte, len(chain.Events))
	for i, e := range chain.Events {
		hashes[i] = e.EventHash
	}
	return hashes
}

// TestVerifyChains_HappyPath_Compare pins the primary compare-mode contract:
// submitting a stored chain's own hashes back must report valid:true, a
// verified storage report, and a head-hash equal to the chain's last event
// hash.
func TestVerifyChains_HappyPath_Compare(t *testing.T) {
	t.Log("\tGiven a server with one stored, verified chain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	block := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "v-0"),
		asOfEventAt(tenantID, base.Add(time.Minute), "v-1"),
		asOfEventAt(tenantID, base.Add(2*time.Minute), "v-2"),
	)
	wantHead := block.Chain.Events[len(block.Chain.Events)-1].EventHash

	t.Log("\tWhen submitting the chain's own hashes back for comparison")
	body := verifyChainRequestJSON(t, block.ID, hashesOfChain(block.Chain))
	w := verifyChain(server, tenantID, body)

	t.Log("\tThen the response should be 200 with valid:true, storage verified, and a matching head-hash")
	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}
	resp := decodeVerifyResponse(t, w)
	if len(resp.Results) != 1 {
		t.Fatalf("\t%s\tExpected 1 result, got %d", fail, len(resp.Results))
	}
	result := resp.Results[0]
	if !result.Valid {
		t.Fatalf("\t%s\tExpected valid:true, got false (result: %+v)", fail, result)
	}
	if result.Storage.Status != string(audit.StatusVerified) {
		t.Fatalf("\t%s\tExpected storage status %q, got %q", fail, audit.StatusVerified, result.Storage.Status)
	}
	if !bytes.Equal(result.HeadHash, wantHead) {
		t.Fatalf("\t%s\tExpected head-hash %x, got %x", fail, wantHead, result.HeadHash)
	}
	t.Logf("\t%s\tHappy-path compare correctly reported valid\n", pass)
}

// TestVerifyChains_FlippedSubmittedHash_Diverges pins that a submitted hash
// that disagrees with the stored chain is reported as diverged at the right
// index, not silently accepted or misreported as storage tampering.
func TestVerifyChains_FlippedSubmittedHash_Diverges(t *testing.T) {
	t.Log("\tGiven a server with one stored, verified chain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	block := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "d-0"),
		asOfEventAt(tenantID, base.Add(time.Minute), "d-1"),
		asOfEventAt(tenantID, base.Add(2*time.Minute), "d-2"),
	)

	t.Log("\tWhen submitting hashes with index 1 flipped")
	submitted := hashesOfChain(block.Chain)
	submitted[1] = []byte("not-the-real-hash-at-all")
	body := verifyChainRequestJSON(t, block.ID, submitted)
	w := verifyChain(server, tenantID, body)

	t.Log("\tThen the response should report diverged at index 1")
	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}
	resp := decodeVerifyResponse(t, w)
	if len(resp.Results) != 1 {
		t.Fatalf("\t%s\tExpected 1 result, got %d", fail, len(resp.Results))
	}
	result := resp.Results[0]
	if result.Valid {
		t.Fatalf("\t%s\tExpected valid:false for a diverged submission, got true", fail)
	}
	if result.Compare == nil {
		t.Fatalf("\t%s\tExpected a non-nil compare result", fail)
	}
	if result.Compare.Status != string(audit.CompareDiverged) {
		t.Fatalf("\t%s\tExpected compare status %q, got %q", fail, audit.CompareDiverged, result.Compare.Status)
	}
	if result.Compare.DivergenceIndex != 1 {
		t.Fatalf("\t%s\tExpected divergence index 1, got %d", fail, result.Compare.DivergenceIndex)
	}
	t.Logf("\t%s\tFlipped submitted hash correctly reported as diverged at index 1\n", pass)
}

// TestVerifyChains_TamperedStorage_ReportsTampered pins that a chain
// mutated in storage after being written (bypassing the audit domain's
// hashing) is reported as storage-tampered with a reason and failed index,
// independent of any submitted comparison.
func TestVerifyChains_TamperedStorage_ReportsTampered(t *testing.T) {
	t.Log("\tGiven a server with one stored, verified chain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	block := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "t-0"),
		asOfEventAt(tenantID, base.Add(time.Minute), "t-1"),
		asOfEventAt(tenantID, base.Add(2*time.Minute), "t-2"),
	)

	t.Log("\tWhen the stored copy is mutated directly in storage (bypassing the audit chain)")
	tampered := block
	tampered.Chain.Events[2].Action = "tampered-in-storage"
	if err := store.WriteBlock(ctx, tenantID, tampered); err != nil {
		t.Fatalf("\t%s\tFailed to write tampered fixture block: %v", fail, err)
	}

	t.Log("\tWhen verifying in attest mode (no submitted comparison)")
	w := verifyChain(server, tenantID, []byte(`{}`))

	t.Log("\tThen the response should report storage tampered with a reason and failed index")
	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}
	resp := decodeVerifyResponse(t, w)
	var result *apihttp.VerifyChainResult
	for i := range resp.Results {
		if resp.Results[i].ChainID == block.ID.String() {
			result = &resp.Results[i]
		}
	}
	if result == nil {
		t.Fatalf("\t%s\tExpected a result for chain %s, got: %+v", fail, block.ID, resp.Results)
	}
	if result.Valid {
		t.Fatalf("\t%s\tExpected valid:false for a tampered chain, got true", fail)
	}
	if result.Storage.Status != string(audit.StatusTampered) {
		t.Fatalf("\t%s\tExpected storage status %q, got %q", fail, audit.StatusTampered, result.Storage.Status)
	}
	if result.Storage.Reason != string(audit.ReasonHashMismatch) {
		t.Fatalf("\t%s\tExpected storage reason %q, got %q", fail, audit.ReasonHashMismatch, result.Storage.Reason)
	}
	if result.Storage.FailedIndex != 2 {
		t.Fatalf("\t%s\tExpected failed index 2, got %d", fail, result.Storage.FailedIndex)
	}
	t.Logf("\t%s\tTampered storage correctly reported with reason and failed index\n", pass)
}

// TestVerifyChains_CrossTenantProbe_IndistinguishableFromRandom pins that
// querying another tenant's real chain-id and querying a random, never-used
// UUID produce byte-identical response shapes — no existence oracle.
func TestVerifyChains_CrossTenantProbe_IndistinguishableFromRandom(t *testing.T) {
	t.Log("\tGiven a server with a chain belonging to another tenant")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	otherTenant := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	otherBlock := writeChain(t, ctx, store, otherTenant,
		asOfEventAt(otherTenant, base, "o-0"),
	)

	callingTenant := uuid.New()

	t.Log("\tWhen probing with the other tenant's real chain-id")
	crossTenantBody := []byte(`{"chains":[{"chain-id":"` + otherBlock.ID.String() + `"}]}`)
	crossTenantW := verifyChain(server, callingTenant, crossTenantBody)

	t.Log("\tAnd probing with a random UUID that has never been used")
	randomBody := []byte(`{"chains":[{"chain-id":"` + uuid.New().String() + `"}]}`)
	randomW := verifyChain(server, callingTenant, randomBody)

	t.Log("\tThen both responses should be 200 not-found, byte-identical in shape")
	if crossTenantW.Code != http.StatusOK || randomW.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected 200/200, got %d/%d", fail, crossTenantW.Code, randomW.Code)
	}
	crossResp := decodeVerifyResponse(t, crossTenantW)
	randomResp := decodeVerifyResponse(t, randomW)
	if len(crossResp.Results) != 1 || len(randomResp.Results) != 1 {
		t.Fatalf("\t%s\tExpected 1 result each, got %d/%d", fail, len(crossResp.Results), len(randomResp.Results))
	}
	crossResult := crossResp.Results[0]
	randomResult := randomResp.Results[0]
	if crossResult.Storage.Status != statusNotFoundForTest {
		t.Fatalf("\t%s\tExpected not-found status for cross-tenant probe, got %q", fail, crossResult.Storage.Status)
	}
	// Byte-identical in shape: same status/valid/reason/failed-index/length,
	// modulo the echoed chain-id itself.
	crossResult.ChainID = ""
	randomResult.ChainID = ""
	crossJSON, _ := json.Marshal(crossResult)
	randomJSON, _ := json.Marshal(randomResult)
	if !bytes.Equal(crossJSON, randomJSON) {
		t.Fatalf("\t%s\tCross-tenant probe and random-UUID probe are not shape-identical:\n%s\nvs\n%s", fail, crossJSON, randomJSON)
	}
	t.Logf("\t%s\tCross-tenant probe indistinguishable from a random UUID\n", pass)
}

// statusNotFoundForTest mirrors the unexported statusNotFound constant in
// api/http/verify.go, from the external test package.
const statusNotFoundForTest = "not-found"

// TestVerifyChains_MalformedJSON_400 pins that an unparseable body is
// rejected with 400.
func TestVerifyChains_MalformedJSON_400(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tWhen submitting malformed JSON")
	w := verifyChain(server, tenantID, []byte(`{not valid json`))

	t.Log("\tThen the response should be 400")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("\t%s\tExpected status 400, got %d: %s", fail, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tMalformed JSON correctly rejected with 400\n", pass)
}

// TestVerifyChains_BadUUID_400 pins that an unparseable chain-id is rejected
// with 400.
func TestVerifyChains_BadUUID_400(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tWhen submitting a chain-id that is not a valid UUID")
	w := verifyChain(server, tenantID, []byte(`{"chains":[{"chain-id":"not-a-uuid"}]}`))

	t.Log("\tThen the response should be 400")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("\t%s\tExpected status 400, got %d: %s", fail, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tInvalid chain-id correctly rejected with 400\n", pass)
}

// TestVerifyChains_BadBase64EventHash_400 pins that a malformed base64
// event-hash entry is rejected with 400 rather than silently truncated or
// treated as a zero-length hash.
func TestVerifyChains_BadBase64EventHash_400(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tWhen submitting an event-hash entry that is not valid base64")
	body := []byte(`{"chains":[{"chain-id":"` + uuid.New().String() + `","event-hashes":["not-valid-base64!!!"]}]}`)
	w := verifyChain(server, tenantID, body)

	t.Log("\tThen the response should be 400")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("\t%s\tExpected status 400, got %d: %s", fail, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tInvalid base64 event-hash correctly rejected with 400\n", pass)
}

// TestVerifyChains_OversizedBody_413 pins the 1 MiB transport cap on
// POST /v1/verify-chain.
func TestVerifyChains_OversizedBody_413(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()

	t.Log("\tGiven a body larger than the 1 MiB transport cap")
	padding := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("a"), (1<<20)+1))
	body := []byte(`{"chains":[{"chain-id":"` + uuid.New().String() + `","event-hashes":["` + padding + `"]}]}`)
	if len(body) <= 1<<20 {
		t.Fatalf("\t%s\tTest fixture is not actually oversized: %d bytes", fail, len(body))
	}

	t.Log("\tWhen POSTing it to /v1/verify-chain")
	w := verifyChain(server, tenantID, body)

	t.Log("\tThen the response must be 413")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("\t%s\tExpected status 413, got %d: %s", fail, w.Code, w.Body.String())
	}
	t.Logf("\t%s\tOversized body correctly rejected with 413\n", pass)
}

// TestVerifyChains_AttestMode_AllChainsWithLimitations pins the attest-mode
// contract: an empty `{}` body verifies every stored chain for the caller's
// tenant and always includes a non-empty limitations field stating the
// prefix-completeness caveat honestly.
func TestVerifyChains_AttestMode_AllChainsWithLimitations(t *testing.T) {
	t.Log("\tGiven a server with two stored chains for one tenant")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	chain1 := writeChain(t, ctx, store, tenantID, asOfEventAt(tenantID, base, "a-0"))
	chain2 := writeChain(t, ctx, store, tenantID, asOfEventAt(tenantID, base, "a-1"))

	t.Log("\tWhen verifying in attest mode (chains omitted)")
	w := verifyChain(server, tenantID, []byte(`{}`))

	t.Log("\tThen the response should include a result for every stored chain and a non-empty limitations field")
	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}
	resp := decodeVerifyResponse(t, w)
	if len(resp.Results) != 2 {
		t.Fatalf("\t%s\tExpected 2 results (attest mode covers every stored chain), got %d", fail, len(resp.Results))
	}
	seen := map[string]bool{}
	for _, r := range resp.Results {
		seen[r.ChainID] = true
	}
	if !seen[chain1.ID.String()] || !seen[chain2.ID.String()] {
		t.Fatalf("\t%s\tExpected results for both chains, got: %+v", fail, resp.Results)
	}
	if len(resp.Limitations) == 0 {
		t.Fatalf("\t%s\tExpected a non-empty limitations field in attest mode", fail)
	}
	t.Logf("\t%s\tAttest mode covered all stored chains with a non-empty limitations field\n", pass)
}

// TestVerifyChains_StoredShorter_DetectsTailTruncation pins the key
// tail-truncation-detection insight: submitting more hashes than storage
// currently has (as if from an earlier, longer export) must be reported as
// stored-shorter, not silently accepted as a valid partial match.
func TestVerifyChains_StoredShorter_DetectsTailTruncation(t *testing.T) {
	t.Log("\tGiven a server with a stored chain of 2 events")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	block := writeChain(t, ctx, store, tenantID,
		asOfEventAt(tenantID, base, "s-0"),
		asOfEventAt(tenantID, base.Add(time.Minute), "s-1"),
	)

	t.Log("\tWhen submitting the stored chain's export hashes plus one extra, as if from an earlier longer export")
	submitted := hashesOfChain(block.Chain)
	submitted = append(submitted, []byte("an-event-hash-from-a-later-append"))
	body := verifyChainRequestJSON(t, block.ID, submitted)
	w := verifyChain(server, tenantID, body)

	t.Log("\tThen the response should report stored-shorter — tail truncation detected against the export")
	if w.Code != http.StatusOK {
		t.Fatalf("\t%s\tExpected status 200, got %d: %s", fail, w.Code, w.Body.String())
	}
	resp := decodeVerifyResponse(t, w)
	if len(resp.Results) != 1 {
		t.Fatalf("\t%s\tExpected 1 result, got %d", fail, len(resp.Results))
	}
	result := resp.Results[0]
	if result.Valid {
		t.Fatalf("\t%s\tExpected valid:false for a stored-shorter (truncated) chain, got true", fail)
	}
	if result.Compare == nil {
		t.Fatalf("\t%s\tExpected a non-nil compare result", fail)
	}
	if result.Compare.Status != string(audit.CompareStoredShorter) {
		t.Fatalf("\t%s\tExpected compare status %q, got %q", fail, audit.CompareStoredShorter, result.Compare.Status)
	}
	t.Logf("\t%s\tStored-shorter tail truncation correctly detected\n", pass)
}

// TestVerifyChains_NoAuth_401 pins that verify-chain requires the same
// Authorization contract as every other endpoint.
func TestVerifyChains_NoAuth_401(t *testing.T) {
	t.Log("\tGiven a server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := memory.New()
	logger, _ := zap.NewDevelopment()
	server := apihttp.New(ctx, store, memory.NewRules(), testConfig(), logger)
	defer server.Close()

	t.Log("\tWhen verifying with no Authorization header")
	w := verifyChain(server, uuid.Nil, []byte(`{}`))

	t.Log("\tThen the response should be 401")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("\t%s\tExpected status 401, got %d", fail, w.Code)
	}
	t.Logf("\t%s\tverify-chain correctly requires authorization\n", pass)
}
