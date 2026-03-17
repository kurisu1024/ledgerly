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

	{
		t.Logf("\t%s\tWhen chain is loaded with events.\n", pass)
		for i := 0; i < chainSize; i++ {
			a := maps.Clone(actor)
			a["id"] = fmt.Sprintf("user-%v", i)
			event := audit.NewEvent(uuid.New(), a, "project.create", resource, metadata)
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
