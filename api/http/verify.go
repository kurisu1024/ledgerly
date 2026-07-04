package http

import (
	"net/http"
)

// maxVerifyBodyBytes caps POST /v1/verify-chain request bodies. A compare
// request carries per-chain event-hash sequences from a prior export — the
// same order-of-magnitude reasoning as maxEventBodyBytes and
// maxRuleBodyBytes applies: 1 MiB is far beyond any legitimate submission,
// and this is a read-only endpoint so there's no reason to accept more.
const maxVerifyBodyBytes = 1 << 20

// statusNotFound is the per-chain result status for a chain-id the caller
// has no visibility into — whether it doesn't exist at all, or belongs to
// another tenant. Both cases produce byte-identical responses; this
// endpoint is not an existence oracle.
const statusNotFound = "not-found"

// VerifyChainQuery is one chain's submitted event-hash sequence from a prior
// export, in chain order — the wire format for audit.CompareChainHashes'
// submitted parameter.
type VerifyChainQuery struct {
	ChainID     string   `json:"chain-id"`
	EventHashes [][]byte `json:"event-hashes"`
}

// VerifyChainRequest is the POST /v1/verify-chain request body. Omitting
// Chains entirely (an empty/`{}` body) selects attest mode: verify every
// stored chain for the caller's tenant rather than comparing against a
// submission.
type VerifyChainRequest struct {
	Chains []VerifyChainQuery `json:"chains,omitempty"`
}

// VerifyReport is the wire form of audit.VerifyResult.
type VerifyReport struct {
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	FailedIndex int    `json:"failed-index"`
	Length      int    `json:"length"`
}

// VerifyCompare is the wire form of audit.CompareResult.
type VerifyCompare struct {
	Status          string `json:"status"`
	DivergenceIndex int    `json:"divergence-index"`
	SubmittedLength int    `json:"submitted-length"`
	StoredLength    int    `json:"stored-length"`
}

// VerifyChainResult is one chain's verification outcome. Valid is the
// single yes/no signal: true only when Storage verified and, in compare
// mode, Compare also found no divergence or truncation. Storage.Status
// doubles as the not-found marker (statusNotFound) for a chain-id the
// caller cannot see.
type VerifyChainResult struct {
	ChainID  string         `json:"chain-id"`
	Valid    bool           `json:"valid"`
	Storage  VerifyReport   `json:"storage"`
	Compare  *VerifyCompare `json:"compare,omitempty"`
	HeadHash []byte         `json:"head-hash,omitempty"`
}

// VerifyChainResponse is the POST /v1/verify-chain response body. HTTP
// status is always 200 for a well-formed request — a tampered or truncated
// chain is data, not a transport error. Limitations is always present and
// states the endpoint's prefix-completeness contract honestly: absent a
// submitted export to compare against (attest mode, or any chain with no
// submission), tail truncation is undetectable.
type VerifyChainResponse struct {
	Results     []VerifyChainResult `json:"results"`
	Limitations []string            `json:"limitations"`
}

// VerifyChains handles POST /v1/verify-chain (issue #23).
//
// STUB (RED stage): not yet implemented.
func (t *T) VerifyChains(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}
