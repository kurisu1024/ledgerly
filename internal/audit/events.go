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
		ID:       uuid.New(),
		Events:   make([]Event, 0, maxChainSize),
		prevHash: genesisHash[:],
	}
}

type EventChain struct {
	ID       uuid.UUID `json:"id"`
	Events   []Event   `json:"events"`
	prevHash []byte
}

func AppendEvent(chain EventChain, e Event) EventChain {
	e.ChainID = chain.ID
	e.PrevHash = chain.prevHash
	e.EventHash = computeHash(e)
	chain.prevHash = e.EventHash

	chain.Events = append(chain.Events, e)
	return chain
}

func VerifyEvent(e Event) bool {
	return bytes.Equal(e.EventHash, computeHash(e))
}

func VerifyChain(chain EventChain) bool {
	for i := 0; i < len(chain.Events)-1; i++ {
		if !VerifyEvent(chain.Events[i]) {
			return false
		}
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
