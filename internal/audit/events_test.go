package audit_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

var pass = "\u2705"
var fail = "\u274C"

func TestVerifyNewEvent(t *testing.T) {
	t.Log("\tGiven a new signed event.")
	event := audit.NewEvent(uuid.New(), []byte(`{"id": "user_123", "type": "user", "ip": "203.0.113.42"}`),
		"project.create", []byte(` {"type": "project", "id": "proj_456" }`), []byte(`{ "reason": "user request" }`))
	{
		t.Logf("\tWhen event is signed.\n")
	}
	event = audit.SignEvent(event)
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
			event := audit.NewEvent(uuid.New(), []byte(fmt.Sprintf("{\"id\": \"user-%v\", \"type\": \"user\", \"ip\": \"203.0.113.42\"}", i)),
				"project.create", []byte(` {"type": "project", "id": "proj_456" }`), []byte(`{ "reason": "user request" }`))
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
