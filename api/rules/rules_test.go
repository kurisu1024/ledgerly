package rules_test

import (
	"testing"

	"github.com/google/uuid"
	apirules "github.com/kurisu1024/ledgerly/api/rules"
	domain "github.com/kurisu1024/ledgerly/internal/rules"
)

func TestRoundTrip_DomainToWireToDomain(t *testing.T) {
	tenantID := uuid.New()
	d := domain.Rule{
		ID:            uuid.New(),
		TenantID:      tenantID,
		SchemaVersion: domain.SchemaVersion,
		EventType:     "project.delete",
		Fields: []domain.FieldCond{
			{Key: "actor.type", Op: domain.OpEquals, Value: "user"},
		},
	}

	wire := apirules.FromDomain(d)
	if wire.ID != d.ID.String() {
		t.Fatalf("expected wire ID %q, got %q", d.ID.String(), wire.ID)
	}

	back, err := apirules.ToDomain(tenantID, wire)
	if err != nil {
		t.Fatalf("expected round-trip ToDomain to succeed, got %v", err)
	}
	if back.EventType != d.EventType {
		t.Fatalf("expected round-tripped EventType %q, got %q", d.EventType, back.EventType)
	}
	if back.TenantID != tenantID {
		t.Fatalf("expected ToDomain to scope rule to tenantID %s, got %s", tenantID, back.TenantID)
	}
}

func TestParseRuleID_RejectsBadUUID(t *testing.T) {
	if _, err := apirules.ParseRuleID("not-a-uuid"); err == nil {
		t.Fatal("expected error for malformed rule id, got nil")
	}
}

func TestParseRuleID_AcceptsValidUUID(t *testing.T) {
	id := uuid.New()
	got, err := apirules.ParseRuleID(id.String())
	if err != nil {
		t.Fatalf("expected valid uuid to parse, got %v", err)
	}
	if got != id {
		t.Fatalf("expected parsed id %s, got %s", id, got)
	}
}

func TestToDomain_RejectsUnknownSchemaVersion(t *testing.T) {
	tenantID := uuid.New()
	wire := apirules.Rule{
		SchemaVersion: 99,
		EventType:     "project.delete",
		Fields: []apirules.FieldCondition{
			{Key: "actor.type", Op: domain.OpEquals, Value: "user"},
		},
	}
	if _, err := apirules.ToDomain(tenantID, wire); err == nil {
		t.Fatal("expected error for unknown schema version, got nil")
	}
}
