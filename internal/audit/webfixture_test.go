package audit_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/api/events"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// TestGenerateWebFixtures writes golden chain fixtures for the web verifier
// (web/src/lib/verify). It is a generator, not a test: it only runs when
// LEDGERLY_GEN_WEB_FIXTURES=1 is set, and its output is committed so the
// client-side verifier is pinned byte-for-byte to this package's AppendEvent
// hashing. Regeneration is deterministic — all IDs and timestamps are fixed.
//
// Regenerate with:
//
//	LEDGERLY_GEN_WEB_FIXTURES=1 go test -run TestGenerateWebFixtures ./internal/audit/
func TestGenerateWebFixtures(t *testing.T) {
	if os.Getenv("LEDGERLY_GEN_WEB_FIXTURES") != "1" {
		t.Skip("set LEDGERLY_GEN_WEB_FIXTURES=1 to regenerate web verifier fixtures")
	}

	fixture := webFixture{
		Cases: []webFixtureCase{
			validNanoCase(),
			validOffsetCase(),
			validNilMetadataCase(),
			tamperedActionCase(),
			brokenLinkCase(),
			foreignChainCase(),
		},
	}
	// The genesis hash is unexported in audit; every chain's first event links
	// to it, so read it off the first generated chain.
	fixture.GenesisHash = fixture.Cases[0].Chain.Events[0].PrevHash

	// Sanity-check expectations against the real verifier before writing.
	for _, c := range fixture.Cases {
		chain := audit.EventChain{ID: uuid.MustParse(c.Chain.ID)}
		for _, we := range c.Chain.Events {
			ae, err := events.ToAuditEvent(we)
			if err != nil {
				t.Fatalf("case %q: round-trip event: %v", c.Name, err)
			}
			chain.Events = append(chain.Events, ae)
		}
		if got := audit.VerifyChain(chain); got != c.Expected.Verified {
			t.Fatalf("case %q: VerifyChain = %v, fixture expects verified=%v", c.Name, got, c.Expected.Verified)
		}
	}

	out, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	out = append(out, '\n')

	path := filepath.Join("..", "..", "web", "src", "lib", "verify", "__fixtures__", "golden-chains.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixtures dir: %v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Logf("wrote %s (%d cases)", path, len(fixture.Cases))
}

// TestGoldenFixturesVerifyChainReport pins VerifyChainReport to the committed
// golden fixtures: for every case the structured result's status, reason, and
// failed index must match the fixture's Expected fields, so the Go verifier
// and the web verifier stay asserted against the same goldens.
func TestGoldenFixturesVerifyChainReport(t *testing.T) {
	path := filepath.Join("..", "..", "web", "src", "lib", "verify", "__fixtures__", "golden-chains.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	var fixture webFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("unmarshal golden fixture: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("golden fixture has no cases")
	}

	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			chain := audit.EventChain{ID: uuid.MustParse(c.Chain.ID)}
			for _, we := range c.Chain.Events {
				ae, err := events.ToAuditEvent(we)
				if err != nil {
					t.Fatalf("round-trip event: %v", err)
				}
				chain.Events = append(chain.Events, ae)
			}

			got := audit.VerifyChainReport(chain)

			wantStatus := audit.StatusTampered
			wantReason := audit.VerifyReason(c.Expected.Reason)
			wantFailedIndex := -1
			if c.Expected.Verified {
				wantStatus = audit.StatusVerified
				wantReason = audit.ReasonNone
			} else if c.Expected.FailedIndex != nil {
				wantFailedIndex = *c.Expected.FailedIndex
			}

			if got.Status != wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, wantStatus)
			}
			if got.Reason != wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, wantReason)
			}
			if got.FailedIndex != wantFailedIndex {
				t.Errorf("FailedIndex = %d, want %d", got.FailedIndex, wantFailedIndex)
			}
			if got.Length != len(chain.Events) {
				t.Errorf("Length = %d, want %d", got.Length, len(chain.Events))
			}
		})
	}
}

type webFixture struct {
	// GenesisHash is the anchor every chain's first event links to.
	GenesisHash []byte           `json:"genesis-hash"`
	Cases       []webFixtureCase `json:"cases"`
}

type webFixtureCase struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Chain       webFixtureChain    `json:"chain"`
	Expected    webFixtureExpected `json:"expected"`
	// CanonicalTimestamps[i] is OccurredAt.UTC().Format(time.RFC3339Nano) for
	// event i — the exact string AppendEvent hashed, which the JS verifier
	// must reproduce from the wire "occurred-at" string.
	CanonicalTimestamps []string `json:"canonical-timestamps"`
}

type webFixtureChain struct {
	ID     string         `json:"id"`
	Events []events.Event `json:"events"`
}

type webFixtureExpected struct {
	Verified    bool   `json:"verified"`
	FailedIndex *int   `json:"failedIndex,omitempty"`
	Reason      string `json:"reason,omitempty"` // hash-mismatch | link-broken | foreign-chain
}

// Fixed identities so regeneration is byte-identical.
var (
	fixtureTenantID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	fixtureChainIDs = map[string]uuid.UUID{
		"valid-nano":         uuid.MustParse("aaaaaaaa-0001-4000-8000-000000000001"),
		"valid-offset":       uuid.MustParse("aaaaaaaa-0002-4000-8000-000000000002"),
		"valid-nil-metadata": uuid.MustParse("aaaaaaaa-0003-4000-8000-000000000003"),
		"tampered-action":    uuid.MustParse("bbbbbbbb-0001-4000-8000-000000000001"),
		"broken-link":        uuid.MustParse("bbbbbbbb-0002-4000-8000-000000000002"),
		"foreign-chain":      uuid.MustParse("bbbbbbbb-0003-4000-8000-000000000003"),
		"foreign-donor":      uuid.MustParse("cccccccc-0001-4000-8000-000000000001"),
	}
)

func fixtureEventID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("eeeeeeee-0000-4000-8000-%012d", n))
}

// fixtureChain builds a chain through the real NewEventChain/AppendEvent path,
// pinning the random chain ID to a fixed value for determinism.
func fixtureChain(name string, evs ...audit.Event) audit.EventChain {
	chain := audit.NewEventChain(len(evs))
	chain.ID = fixtureChainIDs[name]
	for _, e := range evs {
		chain = audit.AppendEvent(chain, e)
	}
	return chain
}

func fixtureEvent(n int, occurredAt time.Time, metadata map[string]string) audit.Event {
	return audit.Event{
		ID:         fixtureEventID(n),
		TenantID:   fixtureTenantID,
		OccurredAt: occurredAt,
		Actor:      map[string]string{"id": fmt.Sprintf("user-%d", n), "type": "user"},
		Action:     fmt.Sprintf("project.update.%d", n),
		Resource:   map[string]string{"type": "project", "id": fmt.Sprintf("proj-%d", n)},
		Metadata:   metadata,
	}
}

// nanoTimes exercises full nanosecond precision plus RFC3339Nano's
// trailing-zero trimming (123400000ns renders as ".1234").
var nanoTimes = []time.Time{
	time.Date(2026, 1, 15, 10, 30, 0, 123456789, time.UTC),
	time.Date(2026, 1, 15, 10, 30, 1, 123400000, time.UTC),
	time.Date(2026, 1, 15, 10, 30, 2, 7, time.UTC),
}

func makeCase(name, description string, chain audit.EventChain, expected webFixtureExpected) webFixtureCase {
	c := webFixtureCase{
		Name:        name,
		Description: description,
		Chain:       webFixtureChain{ID: chain.ID.String()},
		Expected:    expected,
	}
	for _, e := range chain.Events {
		c.Chain.Events = append(c.Chain.Events, events.FromAuditEvent(e))
		c.CanonicalTimestamps = append(c.CanonicalTimestamps, e.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
	return c
}

func validNanoChain(name string) audit.EventChain {
	return fixtureChain(name,
		fixtureEvent(1, nanoTimes[0], map[string]string{"request-id": "req-1"}),
		fixtureEvent(2, nanoTimes[1], map[string]string{"request-id": "req-2"}),
		fixtureEvent(3, nanoTimes[2], map[string]string{"request-id": "req-3"}),
	)
}

func validNanoCase() webFixtureCase {
	return makeCase("valid-nano",
		"3-event valid chain with nanosecond-precision timestamps, including trailing-zero trimming",
		validNanoChain("valid-nano"),
		webFixtureExpected{Verified: true})
}

func validOffsetCase() webFixtureCase {
	cest := time.FixedZone("CEST", 2*60*60)
	chain := fixtureChain("valid-offset",
		fixtureEvent(1, nanoTimes[0], nil),
		fixtureEvent(2, time.Date(2026, 1, 15, 12, 30, 1, 500000000, cest), nil),
	)
	return makeCase("valid-offset",
		"valid chain where one event's occurred-at carries a +02:00 offset on the wire but is hashed in UTC",
		chain,
		webFixtureExpected{Verified: true})
}

func validNilMetadataCase() webFixtureCase {
	chain := fixtureChain("valid-nil-metadata",
		fixtureEvent(1, nanoTimes[0], nil),
		fixtureEvent(2, nanoTimes[1], nil),
	)
	return makeCase("valid-nil-metadata",
		"valid chain whose events have nil metadata (omitted from the wire JSON)",
		chain,
		webFixtureExpected{Verified: true})
}

func tamperedActionCase() webFixtureCase {
	chain := validNanoChain("tampered-action")
	chain.Events[1].Action = "project.delete.tampered"
	idx := 1
	return makeCase("tampered-action",
		"event 1's action was mutated after hashing; its stored event-hash no longer matches",
		chain,
		webFixtureExpected{Verified: false, FailedIndex: &idx, Reason: "hash-mismatch"})
}

func brokenLinkCase() webFixtureCase {
	chain := validNanoChain("broken-link")
	tampered := make([]byte, len(chain.Events[1].PrevHash))
	copy(tampered, chain.Events[1].PrevHash)
	tampered[0] ^= 0xff
	chain.Events[1].PrevHash = tampered
	idx := 1
	return makeCase("broken-link",
		"event 1's prev-hash no longer links to event 0's event-hash",
		chain,
		webFixtureExpected{Verified: false, FailedIndex: &idx, Reason: "link-broken"})
}

func foreignChainCase() webFixtureCase {
	chain := validNanoChain("foreign-chain")
	donor := fixtureChain("foreign-donor",
		fixtureEvent(4, nanoTimes[0], nil),
	)
	chain.Events[1] = donor.Events[0]
	idx := 1
	return makeCase("foreign-chain",
		"event 1 was spliced in from another chain; its chain-id does not match",
		chain,
		webFixtureExpected{Verified: false, FailedIndex: &idx, Reason: "foreign-chain"})
}
