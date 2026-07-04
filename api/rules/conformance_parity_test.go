package rules_test

// Server-side parity guard for the SDK conformance fixtures (issue #26,
// deep-plan D4). The fixtures at sdk/conformance/*.json are the shared
// matcher spec for every language SDK; this test pins their rule-list wire
// shape to what api/rules actually accepts, so an SDK's re-declared wire
// types can never silently drift from the server's.
//
// This is a decode/encode round-trip of the wire envelope only — it does
// NOT exercise ToDomain (tenant scoping + uuid.Parse), because fixture rule
// IDs are short mnemonic strings ("r1", "r2") for matcher-test readability,
// not real UUIDs. Schema-version acceptance is checked directly against
// domain.SchemaVersion instead.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	apirules "github.com/kurisu1024/ledgerly/api/rules"
	domain "github.com/kurisu1024/ledgerly/internal/rules"
)

func conformancePath(t *testing.T, name string) string {
	t.Helper()
	// api/rules -> repo root -> sdk/conformance
	return filepath.Join("..", "..", "sdk", "conformance", name)
}

func loadFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(conformancePath(t, name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return b
}

// matcherFixtureCase mirrors the shape of sdk/conformance/matcher.json well
// enough to pull out the embedded rule lists; the matcher-specific fields
// are irrelevant here.
type matcherFixtureCase struct {
	Name  string          `json:"name"`
	Rules []apirules.Rule `json:"rules"`
}

func TestConformanceFixtures_MatcherRuleLists_RoundTripThroughAPIRules(t *testing.T) {
	raw := loadFixtureBytes(t, "matcher.json")

	var cases []matcherFixtureCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding sdk/conformance/matcher.json: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one matcher conformance case, got none")
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			for _, rule := range c.Rules {
				if rule.SchemaVersion != domain.SchemaVersion {
					t.Fatalf("rule %q: fixture schema-version %d does not match server domain.SchemaVersion %d", rule.ID, rule.SchemaVersion, domain.SchemaVersion)
				}
			}

			encoded, err := json.Marshal(c.Rules)
			if err != nil {
				t.Fatalf("re-encoding fixture rule list: %v", err)
			}

			var roundTripped []apirules.Rule
			if err := json.Unmarshal(encoded, &roundTripped); err != nil {
				t.Fatalf("decoding re-encoded fixture rule list: %v", err)
			}

			if len(roundTripped) != len(c.Rules) {
				t.Fatalf("expected %d rules to round-trip, got %d", len(c.Rules), len(roundTripped))
			}
			for i := range c.Rules {
				if !reflect.DeepEqual(roundTripped[i], c.Rules[i]) {
					t.Fatalf("rule %d did not round-trip: want %+v, got %+v", i, c.Rules[i], roundTripped[i])
				}
			}
		})
	}
}

// envelopeFixtureCase mirrors sdk/conformance/envelope.json.
type envelopeFixtureCase struct {
	Name     string            `json:"name"`
	Envelope apirules.RuleList `json:"envelope"`
	Accepted bool              `json:"accepted"`
}

func TestConformanceFixtures_EnvelopeSchemaVersion_MatchesServerRefusalRule(t *testing.T) {
	raw := loadFixtureBytes(t, "envelope.json")

	var cases []envelopeFixtureCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding sdk/conformance/envelope.json: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("expected at least one envelope conformance case, got none")
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			accepted := c.Envelope.SchemaVersion == domain.SchemaVersion
			if accepted != c.Accepted {
				t.Fatalf("envelope schema-version %d: fixture says accepted=%v, server rule (== %d) says %v", c.Envelope.SchemaVersion, c.Accepted, domain.SchemaVersion, accepted)
			}

			encoded, err := json.Marshal(c.Envelope)
			if err != nil {
				t.Fatalf("re-encoding fixture envelope: %v", err)
			}
			var roundTripped apirules.RuleList
			if err := json.Unmarshal(encoded, &roundTripped); err != nil {
				t.Fatalf("decoding re-encoded fixture envelope: %v", err)
			}
			if roundTripped.SchemaVersion != c.Envelope.SchemaVersion {
				t.Fatalf("envelope schema-version did not round-trip: want %d, got %d", c.Envelope.SchemaVersion, roundTripped.SchemaVersion)
			}
		})
	}
}
