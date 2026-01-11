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
	Action string `json:"action"`
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
	// bytes are used to preserve the original order of the metadata
	// so that marshaling/unmarshaling is deterministic.
	bytes []byte
}

func (a actor) ToBytes() []byte {
	if a.bytes != nil {
		return a.bytes
	}
	a.bytes, _ = json.Marshal(a)
	return a.bytes
}

func (a *actor) MarshalJSON() ([]byte, error) {
	if a.bytes == nil {
		b, err := json.Marshal(a)
		if err != nil {
			return nil, err
		}
		a.bytes = b
	}
	return a.bytes, nil
}

func (a *actor) UnmarshalJSON(data []byte) error {
	a.bytes = data
	return json.Unmarshal(data, &a)
}

type resource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	// bytes are used to preserve the original order of the metadata
	// so that marshaling/unmarshaling is deterministic.
	bytes []byte
}

func (r resource) ToBytes() []byte {
	if r.bytes != nil {
		return r.bytes
	}
	r.bytes, _ = json.Marshal(r)
	return r.bytes
}

func (r *resource) MarshalJSON() ([]byte, error) {
	if r.bytes == nil {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		r.bytes = b
	}
	return r.bytes, nil
}

func (r *resource) UnmarshalJSON(data []byte) error {
	r.bytes = data
	return json.Unmarshal(data, &r)
}

type metadata struct {
	data map[string]string
	// bytes are used to preserve the original order of the metadata
	// so that marshaling/unmarshaling is deterministic.
	bytes []byte
}

func (m metadata) ToBytes() []byte {
	if m.bytes != nil {
		return m.bytes
	}
	b, _ := json.Marshal(m.data)
	m.bytes = b
	return m.bytes
}

func (m *metadata) MarshalJSON() ([]byte, error) {
	if m.bytes == nil {
		b, err := json.Marshal(m.data)
		if err != nil {
			return nil, err
		}
		m.bytes = b
	}
	return m.bytes, nil
}

func (m *metadata) UnmarshalJSON(data []byte) error {
	m.bytes = data
	return json.Unmarshal(data, &m.data)
}

func (m metadata) ByteSize() int {
	if m.bytes != nil {
		return len(m.bytes)
	}
	// Fallback if not yet marshaled
	b, _ := json.Marshal(m.data)
	return len(b)
}
