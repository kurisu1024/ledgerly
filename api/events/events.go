package events

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

type Event struct {
	ID         string    `json:"id"`
	ChainID    string    `json:"chain-id"`
	TenantID   string    `json:"tenant-id"`
	OccurredAt time.Time `json:"occurred-at"`
	// Action performed on the resource.
	Action string `json:"action"`
	// Actor performing the action on the resource, including
	// the id and type of the actor, as well as the ip address (optional).
	Actor map[string]string `json:"actor"`
	// Resource type and id represented as string values.
	Resource map[string]string `json:"resource"`
	// Metadata is user defined key value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`

	PrevHash  []byte `json:"prev-hash,omitempty"`
	EventHash []byte `json:"event-hash,omitempty"`
}

func ToAuditEvent(e Event) (audit.Event, error) {
	var ae audit.Event
	var err error
	if e.ID == "" {
		ae.ID = uuid.New()
	} else {
		if ae.ID, err = uuid.Parse(e.ID); err != nil {
			return ae, fmt.Errorf("failed to parse event id: %w", err)
		}
	}

	if e.ChainID != "" {
		ae.ChainID, err = uuid.Parse(e.ChainID)
		if err != nil {
			return ae, fmt.Errorf("failed to parse chain id: %w", err)
		}
	}

	ae.TenantID, err = uuid.Parse(e.TenantID)
	if err != nil {
		return ae, fmt.Errorf("failed to parse tenant id: %w", err)
	}

	ae.OccurredAt = e.OccurredAt
	ae.Action = e.Action

	ae.Actor = e.Actor
	ae.Resource = e.Resource
	ae.Metadata = e.Metadata

	ae.PrevHash = e.PrevHash
	ae.EventHash = e.EventHash
	return ae, nil
}

func FromAuditEvent(ae audit.Event) Event {
	return Event{
		ID:         ae.ID.String(),
		ChainID:    ae.ChainID.String(),
		TenantID:   ae.TenantID.String(),
		OccurredAt: ae.OccurredAt,
		Action:     ae.Action,
		Actor:      ae.Actor,
		Resource:   ae.Resource,
		Metadata:   ae.Metadata,
		PrevHash:   ae.PrevHash,
		EventHash:  ae.EventHash,
	}
}
