package ledgerly

// Conformance suite (deep-plan D4): sdk/conformance/*.json IS the matcher
// spec, shared with every future language SDK. This test does not encode
// any matcher semantics of its own — it only asserts Match() against the
// fixtures.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type matcherCase struct {
	Name           string     `json:"name"`
	Rules          []Rule     `json:"rules"`
	Input          MatchInput `json:"input"`
	Matched        bool       `json:"matched"`
	MatchedRuleIDs []string   `json:"matched-rule-ids"`
}

func loadMatcherCases(t *testing.T) []matcherCase {
	t.Helper()
	// sdk/go -> sdk -> conformance
	path := filepath.Join("..", "conformance", "matcher.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var cases []matcherCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one matcher conformance case, got none")
	}
	return cases
}

func TestMatch_ConformanceSuite(t *testing.T) {
	for _, c := range loadMatcherCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			matched, ids := Match(c.Rules, c.Input)

			if matched != c.Matched {
				t.Fatalf("matched: want %v, got %v", c.Matched, matched)
			}

			want := append([]string{}, c.MatchedRuleIDs...)
			got := append([]string{}, ids...)
			sort.Strings(want)
			sort.Strings(got)
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("matched-rule-ids: want %v, got %v", c.MatchedRuleIDs, ids)
			}
		})
	}
}
