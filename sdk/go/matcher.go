package ledgerly

// MatchInput is the record-shaped view the matcher evaluates rules
// against. Fields is a flat map of dotted field paths (e.g. "actor.type",
// "resource.id") to string values, mirroring how internal/rules.FieldCond
// keys are authored server-side.
type MatchInput struct {
	Level     string            `json:"level"`
	EventType string            `json:"event-type"`
	Fields    map[string]string `json:"fields"`
}

// levelRank mirrors slog level ordering: debug < info < warn < error.
var levelRank = map[string]int{
	"debug": -4,
	"info":  0,
	"warn":  4,
	"error": 8,
}

// Match reports whether input matches any rule in rules — conditions
// within a single rule are ANDed, rules are ORed — and returns the IDs of
// every rule that matched (server assigns rule IDs; a fallback rule with no
// ID contributes an empty string to the slice).
//
// STUB: matcher semantics are not implemented yet. The conformance suite in
// sdk/conformance/*.json is the spec this must eventually satisfy.
func Match(rules []Rule, input MatchInput) (matched bool, matchedRuleIDs []string) {
	return false, nil
}
