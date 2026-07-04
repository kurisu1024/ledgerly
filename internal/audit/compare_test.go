package audit_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// hashesOf extracts the event hashes of chain, in chain order — mirroring
// what a client would carry forward from a prior GET /v1/export response.
func hashesOf(chain audit.EventChain) [][]byte {
	hashes := make([][]byte, len(chain.Events))
	for i, e := range chain.Events {
		hashes[i] = e.EventHash
	}
	return hashes
}

// prefixChain returns a copy of chain truncated to its first n events. Since
// each event's hash only depends on itself and its predecessor, truncating
// the tail never invalidates the hashes of the events that remain — this is
// exactly the tail-truncation scenario CompareChainHashes must detect
// against an external anchor (a prior export).
func prefixChain(chain audit.EventChain, n int) audit.EventChain {
	chain.Events = chain.Events[:n]
	return chain
}

// TestCompareChainHashes covers the comparison outcomes CompareChainHashes
// must discriminate between: an exact match, a positional divergence, tail
// truncation detected against a longer prior export (stored-shorter), a
// benign older export (submitted-shorter), and malformed/degenerate input
// (empty submission, a nil hash entry).
func TestCompareChainHashes(t *testing.T) {
	const chainSize = 5

	tests := []struct {
		name  string
		build func() (stored audit.EventChain, submitted [][]byte)
		want  audit.CompareResult
	}{
		{
			name: "match",
			build: func() (audit.EventChain, [][]byte) {
				full := newVerifyTestChain(chainSize, uuid.New())
				return full, hashesOf(full)
			},
			want: audit.CompareResult{
				Status:          audit.CompareMatch,
				DivergenceIndex: -1,
				SubmittedLength: chainSize,
				StoredLength:    chainSize,
			},
		},
		{
			name: "diverged-at-k",
			build: func() (audit.EventChain, [][]byte) {
				full := newVerifyTestChain(chainSize, uuid.New())
				submitted := hashesOf(full)
				submitted[2] = []byte("flipped-hash-value-not-real")
				return full, submitted
			},
			want: audit.CompareResult{
				Status:          audit.CompareDiverged,
				DivergenceIndex: 2,
				SubmittedLength: chainSize,
				StoredLength:    chainSize,
			},
		},
		{
			name: "stored-shorter (tail truncation detected)",
			build: func() (audit.EventChain, [][]byte) {
				// The auditor's export saw all 5 events; storage now only
				// has the first 3 — the tail was truncated after the
				// export was taken.
				full := newVerifyTestChain(chainSize, uuid.New())
				stored := prefixChain(full, 3)
				return stored, hashesOf(full)
			},
			want: audit.CompareResult{
				Status:          audit.CompareStoredShorter,
				DivergenceIndex: -1,
				SubmittedLength: chainSize,
				StoredLength:    3,
			},
		},
		{
			name: "submitted-shorter matching prefix (benign)",
			build: func() (audit.EventChain, [][]byte) {
				// An older export only saw the chain's first 3 events; the
				// chain has since grown. Benign, not tampering.
				full := newVerifyTestChain(chainSize, uuid.New())
				return full, hashesOf(full)[:3]
			},
			want: audit.CompareResult{
				Status:          audit.CompareSubmittedShorter,
				DivergenceIndex: -1,
				SubmittedLength: 3,
				StoredLength:    chainSize,
			},
		},
		{
			name: "empty submitted",
			build: func() (audit.EventChain, [][]byte) {
				full := newVerifyTestChain(chainSize, uuid.New())
				return full, [][]byte{}
			},
			want: audit.CompareResult{
				Status:          audit.CompareSubmittedShorter,
				DivergenceIndex: -1,
				SubmittedLength: 0,
				StoredLength:    chainSize,
			},
		},
		{
			name: "nil hash entry",
			build: func() (audit.EventChain, [][]byte) {
				full := newVerifyTestChain(chainSize, uuid.New())
				submitted := hashesOf(full)
				submitted[1] = nil
				return full, submitted
			},
			want: audit.CompareResult{
				Status:          audit.CompareDiverged,
				DivergenceIndex: 1,
				SubmittedLength: chainSize,
				StoredLength:    chainSize,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored, submitted := tt.build()
			got := audit.CompareChainHashes(stored, submitted)
			if got != tt.want {
				t.Fatalf("\t%s\tCompareChainHashes() = %+v, want %+v", fail, got, tt.want)
			}
			t.Logf("\t%s\tCompareChainHashes() matched expectation", pass)
		})
	}
}
