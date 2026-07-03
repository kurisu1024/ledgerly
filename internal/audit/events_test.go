package audit_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

var pass = "\u2705"
var fail = "\u274C"
var (
	actor = map[string]string{
		"id":   "user_123",
		"type": "user",
		"ip":   "203.0.113.42",
	}
	resource = map[string]string{
		"type": "project",
		"id":   "proj_123",
	}
	metadata = map[string]string{
		"reason": "user request",
	}
)

func TestVerifyNewEvent(t *testing.T) {
	t.Log("\tGiven a new signed event.")
	event := audit.NewEvent(uuid.New(), actor, "project.create", resource, metadata)
	{
		t.Logf("\tWhen event is signed.\n")
	}

	chain := audit.NewEventChain(10)
	chain = audit.AppendEvent(chain, event)
	event = chain.Events[0]
	{
		t.Logf("\tEvent should pass verification.\n")
		if !audit.VerifyEvent(event) {
			t.Fatalf("\t%s\tFailed to verify event", fail)
		}
		t.Logf("\t%s\tVerified event.\n", pass)

	}
	{
		t.Log("\tWhen marshalled to JSON and unmarshalled back")
		b, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("\t%s\tFailed to marshal event: %v", fail, err)
		}

		err = json.Unmarshal(b, &event)
		if err != nil {
			t.Fatalf("\t%s\tFailed to unmarshal event: %v\n", fail, err)
		}
		t.Logf("\t%s\tEvent marshalled and unmarshalled..\n", pass)
	}

	{
		t.Logf("\tEvent should pass verification")
		if !audit.VerifyEvent(event) {
			t.Fatalf("\t%s\tFailed to verify event.\n", fail)
		}
		t.Logf("\t%s\tVerified event after marshalling and unmarshalling event.\n", pass)
	}
}

func TestVerifyChain(t *testing.T) {
	var chainSize = 10
	t.Logf("\tGiven new chain of size %v with unique events", chainSize)
	c := audit.NewEventChain(chainSize)
	tenantID := uuid.New()

	{
		t.Logf("\t%s\tWhen chain is loaded with events.\n", pass)
		for i := 0; i < chainSize; i++ {
			a := maps.Clone(actor)
			a["id"] = fmt.Sprintf("user-%v", i)
			event := audit.NewEvent(tenantID, a, "project.create", resource, metadata)
			c = audit.AppendEvent(c, event)
		}
	}
	{
		t.Logf("\t%s\tChain should pass verification.\n", pass)
		if !audit.VerifyChain(c) {
			t.Fatalf("\t%s\tFailed to verify chain.\n", fail)
		}
	}

	{
		t.Logf("\tWhen Chain is marshalled and unmarshalled.\n")
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatalf("\t%s\tFailed to marshal chain: %v", fail, err)
		}
		err = json.Unmarshal(b, &c)
		if err != nil {
			t.Fatalf("\t%s\tFailed to unmarshal chain: %v\n", fail, err)
		}
	}
	{
		t.Logf("\tChain should pass verification.\n")
		if !audit.VerifyChain(c) {
			t.Fatalf("\t%s\tFailed to verify chain after marshalling and unmarshalling chain.\n", fail)
		}
		t.Logf("\t%s\tVerified chain after marshalling and unmarshalling chain.\n", pass)
	}

}

func TestVerifyChainTamper(t *testing.T) {
	newChain := func(n int) audit.EventChain {
		c := audit.NewEventChain(n)
		tenantID := uuid.New()
		for i := 0; i < n; i++ {
			a := maps.Clone(actor)
			a["id"] = fmt.Sprintf("user-%v", i)
			c = audit.AppendEvent(c, audit.NewEvent(tenantID, a, "project.create", resource, metadata))
		}
		return c
	}

	tests := []struct {
		name   string
		tamper func(c audit.EventChain) audit.EventChain
	}{
		{"tampered last event", func(c audit.EventChain) audit.EventChain {
			c.Events[len(c.Events)-1].Action = "project.delete"
			return c
		}},
		{"tampered middle event", func(c audit.EventChain) audit.EventChain {
			c.Events[1].Action = "project.delete"
			return c
		}},
		{"reordered events", func(c audit.EventChain) audit.EventChain {
			c.Events[0], c.Events[1] = c.Events[1], c.Events[0]
			return c
		}},
		{"first event dropped", func(c audit.EventChain) audit.EventChain {
			c.Events = c.Events[1:]
			return c
		}},
		{"middle event dropped", func(c audit.EventChain) audit.EventChain {
			c.Events = append(c.Events[:1], c.Events[2:]...)
			return c
		}},
		{"all events stripped", func(c audit.EventChain) audit.EventChain {
			c.Events = nil
			return c
		}},
		{"foreign chain spliced in wholesale", func(c audit.EventChain) audit.EventChain {
			other := newChain(3)
			c.Events = other.Events
			return c
		}},
		{"foreign tenant's event appended", func(c audit.EventChain) audit.EventChain {
			e := c.Events[len(c.Events)-1]
			e.TenantID = uuid.New()
			c.Events = append(c.Events, e)
			return c
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.tamper(newChain(3))
			if audit.VerifyChain(c) {
				t.Fatalf("\t%s\tChain passed verification after tampering: %s", fail, tt.name)
			}
			t.Logf("\t%s\tTampering detected: %s", pass, tt.name)
		})
	}
}

func TestAppendAfterUnmarshal(t *testing.T) {
	c := audit.NewEventChain(10)
	tenantID := uuid.New()
	c = audit.AppendEvent(c, audit.NewEvent(tenantID, actor, "project.create", resource, metadata))

	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("\t%s\tFailed to marshal chain: %v", fail, err)
	}
	var restored audit.EventChain
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatalf("\t%s\tFailed to unmarshal chain: %v", fail, err)
	}

	restored = audit.AppendEvent(restored, audit.NewEvent(tenantID, actor, "project.update", resource, metadata))
	if !audit.VerifyChain(restored) {
		t.Fatalf("\t%s\tChain appended after unmarshal failed verification", fail)
	}
	t.Logf("\t%s\tChain state survived serialization.", pass)
}
