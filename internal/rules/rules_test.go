package rules_test

import (
	"strings"
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

// TestValidate_Bounds pins the size limits added for issue #24: every
// accepted rule is embedded twice into the permanent audit chain, so
// unbounded rule content is an unbounded chain-growth lever. At-limit
// values must pass; one-over must fail.
func TestValidate_Bounds(t *testing.T) {
	manyFields := func(n int) []rules.FieldCond {
		fs := make([]rules.FieldCond, 0, n)
		for i := 0; i < n; i++ {
			fs = append(fs, rules.FieldCond{Key: "k" + strings.Repeat("x", i), Op: rules.OpExists})
		}
		return fs
	}

	tests := []struct {
		name    string
		mutate  func(*rules.Rule)
		wantErr bool
	}{
		{
			name:   "event-type at limit passes",
			mutate: func(r *rules.Rule) { r.EventType = strings.Repeat("a", rules.MaxStringBytes) },
		},
		{
			name:    "event-type over limit fails",
			mutate:  func(r *rules.Rule) { r.EventType = strings.Repeat("a", rules.MaxStringBytes+1) },
			wantErr: true,
		},
		{
			name: "field key at limit passes",
			mutate: func(r *rules.Rule) {
				r.Fields = []rules.FieldCond{{Key: strings.Repeat("k", rules.MaxStringBytes), Op: rules.OpExists}}
			},
		},
		{
			name: "field key over limit fails",
			mutate: func(r *rules.Rule) {
				r.Fields = []rules.FieldCond{{Key: strings.Repeat("k", rules.MaxStringBytes+1), Op: rules.OpExists}}
			},
			wantErr: true,
		},
		{
			name: "field value at limit passes",
			mutate: func(r *rules.Rule) {
				r.Fields = []rules.FieldCond{{Key: "k", Op: rules.OpEquals, Value: strings.Repeat("v", rules.MaxStringBytes)}}
			},
		},
		{
			name: "field value over limit fails",
			mutate: func(r *rules.Rule) {
				r.Fields = []rules.FieldCond{{Key: "k", Op: rules.OpEquals, Value: strings.Repeat("v", rules.MaxStringBytes+1)}}
			},
			wantErr: true,
		},
		{
			name:   "fields at limit passes",
			mutate: func(r *rules.Rule) { r.Fields = manyFields(rules.MaxFieldConds) },
		},
		{
			name:    "fields over limit fails",
			mutate:  func(r *rules.Rule) { r.Fields = manyFields(rules.MaxFieldConds + 1) },
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validRule()
			tc.mutate(&r)
			err := rules.Validate(r)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected rule to pass validation, got %v", err)
			}
		})
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
