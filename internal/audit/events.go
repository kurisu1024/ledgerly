package audit

import (
	"bytes"
	"crypto/sha256"
	"hash"
	"sort"
	"time"

	"github.com/google/uuid"
)

func NewEvent(
	tenantID uuid.UUID,
	actor map[string]string,
	action string,
	resource map[string]string,
	metadata map[string]string,
) Event {
	return Event{
		ID:         uuid.New(),
		TenantID:   tenantID,
		OccurredAt: time.Now().UTC(),
		Actor:      actor,
		Action:     action,
		Resource:   resource,
		Metadata:   metadata,
	}
}

type Event struct {
	ID         uuid.UUID `json:"id"`
	ChainID    uuid.UUID `json:"chain-id"`
	TenantID   uuid.UUID `json:"tenant-id"`
	OccurredAt time.Time `json:"occurred-at"`

	Actor    map[string]string `json:"actor"`
	Action   string            `json:"action"`
	Resource map[string]string `json:"resource"`
	Metadata map[string]string `json:"metadata"`

	PrevHash  []byte `json:"prev-hash"`
	EventHash []byte `json:"event-hash"`
}

func NewEventChain(maxChainSize int) EventChain {
	return EventChain{
		ID:     uuid.New(),
		Events: make([]Event, 0, maxChainSize),
	}
}

type EventChain struct {
	ID     uuid.UUID `json:"id"`
	Events []Event   `json:"events"`
}

// headHash returns the hash the next appended event must link to. Deriving it
// from Events (rather than a hidden field) keeps chain state intact across
// JSON round-trips.
func headHash(chain EventChain) []byte {
	if n := len(chain.Events); n > 0 {
		return chain.Events[n-1].EventHash
	}
	return genesisHash[:]
}

func AppendEvent(chain EventChain, e Event) EventChain {
	e.ChainID = chain.ID
	e.PrevHash = headHash(chain)
	e.EventHash = computeHash(e)

	chain.Events = append(chain.Events, e)
	return chain
}

func VerifyEvent(e Event) bool {
	return bytes.Equal(e.EventHash, computeHash(e))
}

// VerifyChain checks that every event belongs to this chain and a single
// tenant, re-hashes each event, and verifies linkage anchored at the genesis
// hash. Tampering, reordering, removal from the front or middle, and splicing
// events from another chain all fail. An empty chain is not verifiable —
// blocks are only ever written with at least one event, so an empty stored
// chain means the events were stripped.
//
// Limitation: truncating events from the tail leaves a valid prefix, which
// still verifies. Detecting tail truncation requires an external anchor
// (e.g. storage recording the expected head hash), which this function
// cannot provide on its own.
func VerifyChain(chain EventChain) bool {
	if len(chain.Events) == 0 {
		return false
	}
	tenantID := chain.Events[0].TenantID
	prev := genesisHash[:]
	for _, e := range chain.Events {
		if e.ChainID != chain.ID || e.TenantID != tenantID {
			return false
		}
		if !bytes.Equal(e.PrevHash, prev) {
			return false
		}
		if !VerifyEvent(e) {
			return false
		}
		prev = e.EventHash
	}
	return true
}

var genesisHash = sha256.Sum256([]byte("GENESIS"))

func computeHash(e Event) []byte {
	h := sha256.New()

	h.Write([]byte(e.ChainID.String()))
	h.Write([]byte(e.TenantID.String()))
	h.Write([]byte(e.OccurredAt.UTC().Format(time.RFC3339Nano)))
	writeMapSorted(h, e.Actor)
	h.Write([]byte(e.Action))
	writeMapSorted(h, e.Resource)
	writeMapSorted(h, e.Metadata)
	h.Write(e.PrevHash)

	return h.Sum(nil)
}

func writeMapSorted(h hash.Hash, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(m[k]))
	}
}
