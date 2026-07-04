package audit

// CompareStatus is the outcome of comparing a submitted (e.g. from a prior
// export) sequence of event hashes against a chain's stored hashes.
type CompareStatus string

const (
	// CompareMatch means the submitted hashes equal the stored hashes
	// exactly, position for position.
	CompareMatch CompareStatus = "match"
	// CompareDiverged means a submitted hash disagrees with the stored hash
	// at the same index — the submission and the stored copy disagree about
	// history. DivergenceIndex names the first such index.
	CompareDiverged CompareStatus = "diverged"
	// CompareStoredShorter means the stored chain has fewer events than the
	// submission, with the submission's prefix matching every stored event.
	// This is the tail-truncation detector: the auditor's prior export IS
	// the external anchor VerifyChainReport cannot provide on its own —
	// events present in an earlier export but missing from storage now were
	// truncated from the tail after that export was taken.
	CompareStoredShorter CompareStatus = "stored-shorter"
	// CompareSubmittedShorter means the submission has fewer events than the
	// stored chain, with the submission matching the stored chain's prefix.
	// This is benign: the submission is simply an older export, and the
	// chain has grown since.
	CompareSubmittedShorter CompareStatus = "submitted-shorter"
)

// CompareResult is the discriminated result of CompareChainHashes.
type CompareResult struct {
	Status CompareStatus
	// DivergenceIndex is the index of the first hash disagreement, or -1
	// when Status is CompareMatch, CompareStoredShorter, or
	// CompareSubmittedShorter (no disagreement within the compared prefix).
	DivergenceIndex int
	// SubmittedLength is len(submitted).
	SubmittedLength int
	// StoredLength is the number of events in the stored chain.
	StoredLength int
}

// CompareChainHashes compares submitted — a sequence of event hashes from a
// prior export, in chain order — against stored's own event hashes,
// position for position. It is a pure domain function (no storage access),
// reusable by the SDK and CLI as well as the verify-chain HTTP endpoint.
//
// STUB (issue #23, RED stage): not yet implemented.
func CompareChainHashes(stored EventChain, submitted [][]byte) CompareResult {
	return CompareResult{}
}
