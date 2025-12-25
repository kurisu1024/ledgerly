package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

func NewEvent(
	tenantID uuid.UUID,
	actor json.RawMessage,
	action string,
	resource json.RawMessage,
	metadata json.RawMessage,
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

	Actor    json.RawMessage `json:"actor"`
	Action   string          `jsonL:"action"`
	Resource json.RawMessage `json:"resource"`
	Metadata json.RawMessage `json:"metadata"`

	PrevHash  []byte `json:"prev-hash"`
	EventHash []byte `json:"event-hash"`
}

func SignEvent(e Event) Event {
	b, _ := json.Marshal(e)
	_ = json.Unmarshal(b, &e)
	e.EventHash = computeHash(e)
	return e
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
	e = SignEvent(e)
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
	h.Write(e.Actor)
	h.Write([]byte(e.Action))
	h.Write(e.Resource)
	h.Write(e.Metadata)
	h.Write(e.PrevHash)

	return h.Sum(nil)
}
