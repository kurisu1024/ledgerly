package events

import (
	"encoding/json"
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
	Action string `jsonL:"action"`
	// Actor performing the action on the resource, including
	// the id and type of the actor, as well as the ip address (optional).
	Actor actor `json:"actor"`
	// Resource type and id represented as string values.
	Resource resource `json:"resource"`
	// Metadata is user defined key value pairs.
	Metadata metadata `json:"metadata"`

	PrevHash  []byte `json:"prev-hash,omitempty"`
	EventHash []byte `json:"event-hash,omitempty"`
}

func ToAuditEvent(e Event) (audit.Event, error) {
	var ae audit.Event
	ae.ID = uuid.MustParse(e.ID)
	ae.ChainID = uuid.MustParse(e.ChainID)
	ae.TenantID = uuid.MustParse(e.TenantID)
	ae.OccurredAt = e.OccurredAt
	ae.Action = e.Action

	ae.Actor = e.Actor.ToBytes()
	ae.Resource = e.Resource.ToBytes()
	ae.Metadata = e.Metadata.ToBytes()

	ae.PrevHash = e.PrevHash
	ae.EventHash = e.EventHash
	return ae, nil
}

type actor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	IP   string `json:"ip,omitempty"`
}

func (a actor) ToBytes() []byte {
	b, _ := json.Marshal(a)
	return b
}

type resource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (r resource) ToBytes() []byte {
	b, _ := json.Marshal(r)
	return b
}

type metadata map[string]string

func (m metadata) ToBytes() []byte {
	b, _ := json.Marshal(m)
	return b
}

func (m metadata) Size() int {
	return mapByteSize(m)
}
func mapByteSize(m map[string]string) int {
	size := 0
	for k, v := range m {
		size += len(k) + len(v)
	}
	return size
}
