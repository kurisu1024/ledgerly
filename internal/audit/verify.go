package audit

import "bytes"

// VerifyStatus is the top-level outcome of verifying an EventChain.
type VerifyStatus string

const (
	// StatusVerified means every event re-hashes correctly and links to the
	// previous event's hash, anchored at genesis, all within one chain and
	// tenant.
	StatusVerified VerifyStatus = "verified"
	// StatusTampered means at least one event failed re-hashing, linkage, or
	// chain/tenant membership. Reason and FailedIndex explain why.
	StatusTampered VerifyStatus = "tampered"
	// StatusUnverifiable means the chain has no events to check.
	StatusUnverifiable VerifyStatus = "unverifiable"
)

// VerifyReason further explains a non-verified VerifyResult.
type VerifyReason string

const (
	// ReasonNone applies to a verified result.
	ReasonNone VerifyReason = ""
	// ReasonHashMismatch means an event's stored hash doesn't match its
	// recomputed hash — its fields were mutated after signing.
	ReasonHashMismatch VerifyReason = "hash-mismatch"
	// ReasonLinkBroken means an event's PrevHash doesn't match the previous
	// event's EventHash (or genesis for the first event) — reordering,
	// splicing, or dropping an event breaks the link.
	ReasonLinkBroken VerifyReason = "link-broken"
	// ReasonForeignChain means an event's ChainID doesn't match the chain
	// it's being verified against.
	ReasonForeignChain VerifyReason = "foreign-chain"
	// ReasonTenantMixed means an event's TenantID doesn't match the other
	// events in the chain.
	ReasonTenantMixed VerifyReason = "tenant-mixed"
	// ReasonEmpty means the chain has zero events.
	ReasonEmpty VerifyReason = "empty"
)

// VerifyResult is the discriminated result of verifying an EventChain. It
// mirrors the result shape the web viewer's client-side verifier produces so
// both can be asserted against the same golden fixtures.
type VerifyResult struct {
	Status VerifyStatus
	Reason VerifyReason
	// FailedIndex is the index of the first event that failed verification,
	// or -1 when Status is Verified.
	FailedIndex int
	// Length is the number of events in the chain.
	Length int
}

// VerifyChainReport checks that every event in chain belongs to it and a
// single tenant, re-hashes each event, and verifies linkage anchored at the
// genesis hash, returning a discriminated result rather than a bool so
// callers (the CLI's `verify` command, the web viewer) can report what went
// wrong and where.
//
// Limitation: truncating events from the tail leaves a valid prefix, which
// still verifies. Detecting tail truncation requires an external anchor
// (e.g. storage recording the expected head hash), which this function
// cannot provide on its own.
func VerifyChainReport(chain EventChain) VerifyResult {
	length := len(chain.Events)
	if length == 0 {
		return VerifyResult{
			Status:      StatusUnverifiable,
			Reason:      ReasonEmpty,
			FailedIndex: -1,
			Length:      0,
		}
	}

	tampered := func(reason VerifyReason, i int) VerifyResult {
		return VerifyResult{
			Status:      StatusTampered,
			Reason:      reason,
			FailedIndex: i,
			Length:      length,
		}
	}

	tenantID := chain.Events[0].TenantID
	prev := genesisHash[:]
	for i, e := range chain.Events {
		if e.ChainID != chain.ID {
			return tampered(ReasonForeignChain, i)
		}
		if e.TenantID != tenantID {
			return tampered(ReasonTenantMixed, i)
		}
		if !bytes.Equal(e.PrevHash, prev) {
			return tampered(ReasonLinkBroken, i)
		}
		if !VerifyEvent(e) {
			return tampered(ReasonHashMismatch, i)
		}
		prev = e.EventHash
	}

	return VerifyResult{
		Status:      StatusVerified,
		Reason:      ReasonNone,
		FailedIndex: -1,
		Length:      length,
	}
}
