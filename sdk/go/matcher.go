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
// ID contributes an empty string to the slice). The conformance suite in
// sdk/conformance/*.json is the spec for these semantics.
func Match(rules []Rule, input MatchInput) (matched bool, matchedRuleIDs []string) {
	matchedRuleIDs = []string{}
	for _, r := range rules {
		if ruleMatches(r, input) {
			matchedRuleIDs = append(matchedRuleIDs, r.ID)
		}
	}
	return len(matchedRuleIDs) > 0, matchedRuleIDs
}

// ruleMatches reports whether every condition on r holds for input.
func ruleMatches(r Rule, input MatchInput) bool {
	if r.EventType != input.EventType {
		return false
	}

	if r.LevelAtLeast != "" {
		want, wantOK := levelRank[r.LevelAtLeast]
		have, haveOK := levelRank[input.Level]
		if !wantOK || !haveOK || have < want {
			return false
		}
	}

	for _, cond := range r.Fields {
		value, present := input.Fields[cond.Key]
		switch cond.Op {
		case OpExists:
			if !present {
				return false
			}
		case OpEquals:
			if !present || value != cond.Value {
				return false
			}
		default:
			// Unknown operator: fail closed for this rule rather than
			// guessing at semantics the server never defined.
			return false
		}
	}
	return true
}
