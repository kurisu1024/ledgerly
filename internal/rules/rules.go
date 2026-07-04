// Package rules is the domain layer for tenant trigger rules (ADR-0002):
// server-side, tenant-scoped rules that SDKs fetch and evaluate against
// events. Every mutation of a tenant's rule set is written as a chained
// audit event by the caller (api/http/rules.go) — this package only owns
// rule shape, validation, and the content-derived ETag.
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// SchemaVersion is the only schema version this build understands. Clients
// (SDKs) must refuse rule-list envelopes carrying any other value rather
// than guess at forward compatibility.
const SchemaVersion = 1

// Field condition operators.
const (
	OpEquals = "equals"
	OpExists = "exists"
)

// Resource bounds on tenant rules. Every accepted rule mutation embeds the
// rule's canonical JSON twice into the permanent audit chain (old-rule /
// new-rule metadata), so unbounded rule content or count is an unbounded
// chain-growth lever. Enforced by Validate (per-rule bounds) and by the
// CreateRule mutation in api/http (MaxRulesPerTenant).
const (
	// MaxFieldConds is the maximum number of field conditions per rule.
	MaxFieldConds = 16
	// MaxStringBytes is the maximum length in bytes of EventType and of
	// each field condition's Key and Value.
	MaxStringBytes = 256
	// MaxRulesPerTenant caps how many rules a single tenant may hold.
	MaxRulesPerTenant = 100
)

// ErrTooManyRules is returned when a create would push a tenant past
// MaxRulesPerTenant. The HTTP layer maps it to 409 Conflict: the request is
// well-formed but conflicts with the current state of the tenant's rule
// collection, and becomes retryable once a rule is deleted.
var ErrTooManyRules = fmt.Errorf("tenant already holds the maximum of %d rules", MaxRulesPerTenant)

// FieldCond matches a single field on an incoming event. Value is ignored
// for OpExists.
type FieldCond struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Rule is a single tenant-defined trigger rule. Conditions present on the
// rule are ANDed together; at least one of LevelAtLeast or Fields must be
// set, or the rule can never usefully narrow anything.
type Rule struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenant-id"`
	SchemaVersion int       `json:"schema-version"`

	EventType    string      `json:"event-type"`
	LevelAtLeast string      `json:"level-at-least,omitempty"`
	Fields       []FieldCond `json:"fields,omitempty"`

	CreatedAt time.Time `json:"created-at"`
	UpdatedAt time.Time `json:"updated-at"`
}

// levels is the allow-list for LevelAtLeast (slog levels).
var levels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// Validate checks that r is well-formed: SchemaVersion == SchemaVersion,
// EventType is non-empty and within MaxStringBytes, at most MaxFieldConds
// field conditions each with Key/Value within MaxStringBytes, every
// FieldCond has a known Op (and a non-empty Value when Op == OpEquals),
// and at least one condition (LevelAtLeast or a FieldCond) is present.
func Validate(r Rule) error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unknown schema-version %d (this build understands %d)", r.SchemaVersion, SchemaVersion)
	}
	if r.EventType == "" {
		return fmt.Errorf("event-type is required")
	}
	if len(r.EventType) > MaxStringBytes {
		return fmt.Errorf("event-type exceeds %d bytes", MaxStringBytes)
	}
	if r.LevelAtLeast != "" && !levels[r.LevelAtLeast] {
		return fmt.Errorf("unknown level-at-least %q", r.LevelAtLeast)
	}
	if len(r.Fields) > MaxFieldConds {
		return fmt.Errorf("too many field conditions: %d (max %d)", len(r.Fields), MaxFieldConds)
	}
	for i, f := range r.Fields {
		if f.Key == "" {
			return fmt.Errorf("fields[%d]: key is required", i)
		}
		if len(f.Key) > MaxStringBytes {
			return fmt.Errorf("fields[%d]: key exceeds %d bytes", i, MaxStringBytes)
		}
		if len(f.Value) > MaxStringBytes {
			return fmt.Errorf("fields[%d]: value exceeds %d bytes", i, MaxStringBytes)
		}
		switch f.Op {
		case OpEquals:
			if f.Value == "" {
				return fmt.Errorf("fields[%d]: op %q requires a non-empty value", i, OpEquals)
			}
		case OpExists:
			if f.Value != "" {
				return fmt.Errorf("fields[%d]: op %q must not carry a value", i, OpExists)
			}
		default:
			return fmt.Errorf("fields[%d]: unknown op %q", i, f.Op)
		}
	}
	if r.LevelAtLeast == "" && len(r.Fields) == 0 {
		return fmt.Errorf("at least one condition (level-at-least or a field condition) is required")
	}
	return nil
}

// canonicalRule is the fixed marshaling shape behind CanonicalJSON:
// explicit field order, string UUIDs, UTC RFC3339 timestamps, and no
// omitted-vs-empty ambiguity for Fields.
type canonicalRule struct {
	ID            string      `json:"id"`
	TenantID      string      `json:"tenant-id"`
	SchemaVersion int         `json:"schema-version"`
	EventType     string      `json:"event-type"`
	LevelAtLeast  string      `json:"level-at-least"`
	Fields        []FieldCond `json:"fields"`
	CreatedAt     string      `json:"created-at"`
	UpdatedAt     string      `json:"updated-at"`
}

// CanonicalJSON returns rs marshaled deterministically: sorted by ID,
// stable field order, no whitespace variance. It is the sole input to
// ETag, so any change to this function's output for the same logical rule
// set is a breaking change to every cached SDK ETag.
func CanonicalJSON(rs []Rule) ([]byte, error) {
	canonical := make([]canonicalRule, 0, len(rs))
	for _, r := range rs {
		fields := make([]FieldCond, 0, len(r.Fields))
		fields = append(fields, r.Fields...)
		canonical = append(canonical, canonicalRule{
			ID:            r.ID.String(),
			TenantID:      r.TenantID.String(),
			SchemaVersion: r.SchemaVersion,
			EventType:     r.EventType,
			LevelAtLeast:  r.LevelAtLeast,
			Fields:        fields,
			CreatedAt:     r.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:     r.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ID < canonical[j].ID })

	out, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshaling canonical rule set: %w", err)
	}
	return out, nil
}

// ETag returns the content-derived ETag for rs: a quoted, hex-encoded
// SHA-256 of CanonicalJSON(rs). It is stateless and backend-agnostic — no
// separate ETag storage is ever required.
func ETag(rs []Rule) (string, error) {
	canonical, err := CanonicalJSON(rs)
	if err != nil {
		return "", fmt.Errorf("computing rule-set etag: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return `"` + hex.EncodeToString(sum[:]) + `"`, nil
}
