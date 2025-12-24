package audit

import (
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
	e.EventHash = computeHash(e)
	return e
}

func newEventChain(maxChainSize int) EventChain {
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

func appendEvent(chain EventChain, e Event) EventChain {
	e.ChainID = chain.ID
	e.PrevHash = chain.prevHash
	e = SignEvent(e)
	chain.prevHash = e.EventHash

	chain.Events = append(chain.Events, e)
	return chain
}
