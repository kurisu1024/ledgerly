// Package rules is the domain layer for tenant trigger rules (ADR-0002):
// server-side, tenant-scoped rules that SDKs fetch and evaluate against
// events. Every mutation of a tenant's rule set is written as a chained
// audit event by the caller (api/http/rules.go) — this package only owns
// rule shape, validation, and the content-derived ETag.
package rules

import (
	"fmt"
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

// Validate checks that r is well-formed: SchemaVersion == SchemaVersion,
// EventType is non-empty, every FieldCond has a known Op (and a non-empty
// Value when Op == OpEquals), and at least one condition (LevelAtLeast or a
// FieldCond) is present.
//
// STUB: not implemented yet — always errors. Real validation lands in
// GREEN; see internal/rules/rules_test.go for the pinned cases.
func Validate(r Rule) error {
	return fmt.Errorf("rules: Validate not implemented")
}

// CanonicalJSON returns rs marshaled deterministically: sorted by ID,
// stable field order, no whitespace variance. It is the sole input to
// ETag, so any change to this function's output for the same logical rule
// set is a breaking change to every cached SDK ETag.
//
// STUB: not implemented yet — always errors.
func CanonicalJSON(rs []Rule) ([]byte, error) {
	return nil, fmt.Errorf("rules: CanonicalJSON not implemented")
}

// ETag returns the content-derived ETag for rs: a quoted, hex-encoded
// SHA-256 of CanonicalJSON(rs). It is stateless and backend-agnostic — no
// separate ETag storage is ever required.
//
// STUB: not implemented yet — always errors.
func ETag(rs []Rule) (string, error) {
	return "", fmt.Errorf("rules: ETag not implemented")
}
