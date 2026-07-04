package rules_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/rules"
)

func validRule() rules.Rule {
	return rules.Rule{
		ID:            uuid.New(),
		TenantID:      uuid.New(),
		SchemaVersion: rules.SchemaVersion,
		EventType:     "project.delete",
		Fields: []rules.FieldCond{
			{Key: "actor.type", Op: rules.OpEquals, Value: "user"},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
}

func TestValidate_AcceptsFieldCondition(t *testing.T) {
	if err := rules.Validate(validRule()); err != nil {
		t.Fatalf("expected valid rule to pass, got %v", err)
	}
}

func TestValidate_AcceptsLevelAtLeastOnly(t *testing.T) {
	r := validRule()
	r.Fields = nil
	r.LevelAtLeast = "error"
	if err := rules.Validate(r); err != nil {
		t.Fatalf("expected valid rule to pass, got %v", err)
	}
}

func TestValidate_RejectsMissingEventType(t *testing.T) {
	r := validRule()
	r.EventType = ""
	if err := rules.Validate(r); err == nil {
		t.Fatal("expected error for missing event-type, got nil")
	}
}

func TestValidate_RejectsNoConditions(t *testing.T) {
	r := validRule()
	r.Fields = nil
	r.LevelAtLeast = ""
	if err := rules.Validate(r); err == nil {
		t.Fatal("expected error when no conditions are present, got nil")
	}
}

func TestValidate_RejectsUnknownSchemaVersion(t *testing.T) {
	r := validRule()
	r.SchemaVersion = 99
	if err := rules.Validate(r); err == nil {
		t.Fatal("expected error for unknown schema version, got nil")
	}
}

func TestValidate_RejectsUnknownFieldOp(t *testing.T) {
	r := validRule()
	r.Fields = []rules.FieldCond{{Key: "k", Op: "contains", Value: "v"}}
	if err := rules.Validate(r); err == nil {
		t.Fatal("expected error for unknown field op, got nil")
	}
}

func TestValidate_RejectsEqualsWithEmptyValue(t *testing.T) {
	r := validRule()
	r.Fields = []rules.FieldCond{{Key: "k", Op: rules.OpEquals, Value: ""}}
	if err := rules.Validate(r); err == nil {
		t.Fatal("expected error for equals condition with empty value, got nil")
	}
}

func TestCanonicalJSON_OrderIndependent(t *testing.T) {
	a, b := validRule(), validRule()
	b.ID = uuid.New()

	json1, err := rules.CanonicalJSON([]rules.Rule{a, b})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	json2, err := rules.CanonicalJSON([]rules.Rule{b, a})
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if string(json1) != string(json2) {
		t.Fatalf("expected canonical JSON to be order-independent:\n%s\nvs\n%s", json1, json2)
	}
}

func TestETag_StableForSameContent(t *testing.T) {
	r := validRule()
	tag1, err := rules.ETag([]rules.Rule{r})
	if err != nil {
		t.Fatalf("ETag: %v", err)
	}
	tag2, err := rules.ETag([]rules.Rule{r})
	if err != nil {
		t.Fatalf("ETag: %v", err)
	}
	if tag1 != tag2 {
		t.Fatalf("expected stable ETag for identical content, got %q vs %q", tag1, tag2)
	}
}

func TestETag_ChangesWithContent(t *testing.T) {
	r := validRule()
	tag1, err := rules.ETag([]rules.Rule{r})
	if err != nil {
		t.Fatalf("ETag: %v", err)
	}
	r.EventType = "project.create"
	tag2, err := rules.ETag([]rules.Rule{r})
	if err != nil {
		t.Fatalf("ETag: %v", err)
	}
	if tag1 == tag2 {
		t.Fatalf("expected ETag to change when rule content changes, got %q for both", tag1)
	}
}

func TestETag_EmptySetIsStable(t *testing.T) {
	tag1, err := rules.ETag(nil)
	if err != nil {
		t.Fatalf("ETag: %v", err)
	}
	tag2, err := rules.ETag([]rules.Rule{})
	if err != nil {
		t.Fatalf("ETag: %v", err)
	}
	if tag1 != tag2 {
		t.Fatalf("expected empty rule set to always produce the same ETag, got %q vs %q", tag1, tag2)
	}
}
