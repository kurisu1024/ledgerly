// Package rules holds the wire-format DTOs for the trigger-rules API and
// their conversion to/from internal/rules, mirroring the api/events split
// between wire format (string IDs, kebab-case JSON) and domain type.
package rules

import (
	"fmt"

	"github.com/google/uuid"
	domain "github.com/kurisu1024/ledgerly/internal/rules"
)

// FieldCondition is the wire form of internal/rules.FieldCond.
type FieldCondition struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Rule is the wire form of a single trigger rule. ID is empty on create.
type Rule struct {
	ID            string           `json:"id,omitempty"`
	SchemaVersion int              `json:"schema-version"`
	EventType     string           `json:"event-type"`
	LevelAtLeast  string           `json:"level-at-least,omitempty"`
	Fields        []FieldCondition `json:"fields,omitempty"`
}

// RuleList is the GET /v1/rules envelope. SchemaVersion is carried at the
// envelope level so SDKs can refuse a version they don't understand before
// looking at individual rules.
type RuleList struct {
	SchemaVersion int    `json:"schema-version"`
	Rules         []Rule `json:"rules"`
}

// ParseRuleID parses a rule ID path parameter. Not a stub: uuid.Parse
// already does exactly what's needed here.
func ParseRuleID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing rule id: %w", err)
	}
	return parsed, nil
}

// ToDomain converts the wire DTO to the internal domain type, scoping it to
// tenantID — the tenant on the wire, if any, is never trusted.
//
// STUB: not implemented yet — always errors. Real conversion (including
// rejecting unknown SchemaVersion and delegating to domain.Validate) lands
// in GREEN.
func ToDomain(tenantID uuid.UUID, r Rule) (domain.Rule, error) {
	return domain.Rule{}, fmt.Errorf("rules: ToDomain not implemented")
}

// FromDomain converts a domain rule to its wire form.
func FromDomain(r domain.Rule) Rule {
	fields := make([]FieldCondition, 0, len(r.Fields))
	for _, f := range r.Fields {
		fields = append(fields, FieldCondition{Key: f.Key, Op: f.Op, Value: f.Value})
	}
	return Rule{
		ID:            r.ID.String(),
		SchemaVersion: r.SchemaVersion,
		EventType:     r.EventType,
		LevelAtLeast:  r.LevelAtLeast,
		Fields:        fields,
	}
}
