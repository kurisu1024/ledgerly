package events_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/api/events"
)

var pass = "\u2705"
var fail = "\u274C"

func TestToAuditEvent(t *testing.T) {
	t.Log("\tGiven an api event when calling toAuditEvent")
	e := events.Event{
		ID:       uuid.New().String(),
		ChainID:  uuid.New().String(),
		TenantID: uuid.New().String(),
		Action:   "some-actrion-taken",
	}
	e.Actor.ID = "some-id"
	e.Actor.Type = "some-actor-type"
	e.Actor.IP = "127.0.0.1"

	e.Resource.Type = "some-resource-type"
	e.Resource.ID = "some-resource-id"

	t.Log("\tThen the event should be converted to an audit event.")

	ae, err := events.ToAuditEvent(e)
	if err != nil {
		t.Fatalf("\t%s\tFailed to convert event to audit event: %v", fail, err)
	}
	t.Logf("\t%s\tSuccessfully converted event to audit event.\n", pass)
	t.Log("\taudit event fields should match the api event fields.")

	if ae.ID.String() != e.ID {
		t.Errorf("\t%s\tAudit event ID does not match api event ID.", fail)
	}
	if ae.ChainID.String() != e.ChainID {
		t.Errorf("\t%s\tAudit event Chain ID does not match api event Chain ID.", fail)
	}
	if ae.TenantID.String() != e.TenantID {
		t.Errorf("\t%s\tAudit event Tenant ID does not match api event Tenant ID.", fail)
	}
	if ae.Action != e.Action {
		t.Errorf("\t%s\tAudit event Action does not match api event Action.", fail)
	}
	if ae.OccurredAt.String() != e.OccurredAt.String() {
		t.Errorf("\t%s\tAudit event OccurredAt does not match api event OccurredAt.", fail)
	}

	if want, _ := e.Resource.ToBytes(); !bytes.Equal(ae.Resource, want) {
		t.Errorf("\t%s\tAudit event Resource does not match api event Resource.", fail)
	}
	if want, _ := e.Actor.ToBytes(); !bytes.Equal(ae.Actor, want) {
		t.Errorf("\t%s\tAudit event Actor does not match api event Actor.", fail)
	}
	if want, _ := e.Metadata.ToBytes(); !bytes.Equal(ae.Metadata, want) {
		t.Errorf("\t%s\tAudit event Metadata does not match api event Metadata.", fail)
	}
	t.Logf("\t%s\tAudit event fields match api event fields.\n", pass)
}
