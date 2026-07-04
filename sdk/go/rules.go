package ledgerly

// This file re-declares the tiny trigger-rule wire types owned by the
// server (api/rules), pinned by a server-side parity test
// (api/rules/conformance_parity_test.go) rather than a shared Go module —
// the SDK deliberately has zero dependency on the server module so pgx/
// cobra/zap never leak into an adopter's go.sum. Field names, JSON tags,
// and shape must be kept byte-for-byte identical to api/rules.

// SchemaVersion is the only rules envelope schema version this SDK
// understands. Any other value must be refused, not guessed at.
const SchemaVersion = 1

// Field condition operators (mirrors internal/rules.OpEquals/OpExists).
const (
	OpEquals = "equals"
	OpExists = "exists"
)

// FieldCondition is the wire form of a single field condition on a rule.
// Value is meaningless for OpExists.
type FieldCondition struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// Rule is a single tenant-defined trigger rule as fetched from GET
// /v1/rules, or as compiled into a fallback rule-set. Conditions present on
// a rule are ANDed together.
type Rule struct {
	ID            string           `json:"id,omitempty"`
	SchemaVersion int              `json:"schema-version"`
	EventType     string           `json:"event-type"`
	LevelAtLeast  string           `json:"level-at-least,omitempty"`
	Fields        []FieldCondition `json:"fields,omitempty"`
}

// RuleList is the GET /v1/rules response envelope. SchemaVersion is carried
// at the envelope level so the poller can refuse a version it doesn't
// understand before looking at individual rules.
type RuleList struct {
	SchemaVersion int    `json:"schema-version"`
	Rules         []Rule `json:"rules"`
}
