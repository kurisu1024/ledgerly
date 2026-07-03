package audit_test

import (
	"fmt"
	"maps"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// newVerifyTestChain builds a chain of n events for tenantID, always through
// NewEventChain + AppendEvent — never hand-rolled hashes.
func newVerifyTestChain(n int, tenantID uuid.UUID) audit.EventChain {
	c := audit.NewEventChain(n)
	for i := 0; i < n; i++ {
		a := maps.Clone(actor)
		a["id"] = fmt.Sprintf("user-%v", i)
		c = audit.AppendEvent(c, audit.NewEvent(tenantID, a, "project.create", resource, metadata))
	}
	return c
}

func TestVerifyChainReport(t *testing.T) {
	const chainSize = 5

	tests := []struct {
		name  string
		chain func() audit.EventChain
		want  audit.VerifyResult
	}{
		{
			name: "verified chain",
			chain: func() audit.EventChain {
				return newVerifyTestChain(chainSize, uuid.New())
			},
			want: audit.VerifyResult{
				Status:      audit.StatusVerified,
				Reason:      audit.ReasonNone,
				FailedIndex: -1,
				Length:      chainSize,
			},
		},
		{
			name: "mutated action at index 2",
			chain: func() audit.EventChain {
				c := newVerifyTestChain(chainSize, uuid.New())
				c.Events[2].Action = "project.delete"
				return c
			},
			want: audit.VerifyResult{
				Status:      audit.StatusTampered,
				Reason:      audit.ReasonHashMismatch,
				FailedIndex: 2,
				Length:      chainSize,
			},
		},
		{
			name: "broken prev hash",
			chain: func() audit.EventChain {
				c := newVerifyTestChain(chainSize, uuid.New())
				c.Events[3].PrevHash = []byte("not-a-real-hash")
				return c
			},
			want: audit.VerifyResult{
				Status:      audit.StatusTampered,
				Reason:      audit.ReasonLinkBroken,
				FailedIndex: 3,
				Length:      chainSize,
			},
		},
		{
			name: "foreign chain id",
			chain: func() audit.EventChain {
				c := newVerifyTestChain(chainSize, uuid.New())
				c.Events[1].ChainID = uuid.New()
				return c
			},
			want: audit.VerifyResult{
				Status:      audit.StatusTampered,
				Reason:      audit.ReasonForeignChain,
				FailedIndex: 1,
				Length:      chainSize,
			},
		},
		{
			name: "second tenant mixed in",
			chain: func() audit.EventChain {
				c := newVerifyTestChain(chainSize, uuid.New())
				c.Events[4].TenantID = uuid.New()
				return c
			},
			want: audit.VerifyResult{
				Status:      audit.StatusTampered,
				Reason:      audit.ReasonTenantMixed,
				FailedIndex: 4,
				Length:      chainSize,
			},
		},
		{
			name: "empty chain",
			chain: func() audit.EventChain {
				return audit.NewEventChain(chainSize)
			},
			want: audit.VerifyResult{
				Status:      audit.StatusUnverifiable,
				Reason:      audit.ReasonEmpty,
				FailedIndex: -1,
				Length:      0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := audit.VerifyChainReport(tt.chain())
			if got != tt.want {
				t.Fatalf("\t%s\tVerifyChainReport() = %+v, want %+v", fail, got, tt.want)
			}
			t.Logf("\t%s\tVerifyChainReport() matched expectation", pass)
		})
	}
}
