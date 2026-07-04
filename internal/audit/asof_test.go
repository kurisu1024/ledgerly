package audit_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurisu1024/ledgerly/internal/audit"
)

// eventAt builds a real, correctly-hashed event via audit.NewEvent, then
// overrides OccurredAt before it's appended (and hashed) — so tests never
// hand-roll a hash, per house style.
func eventAt(tenantID uuid.UUID, t time.Time, action string) audit.Event {
	e := audit.NewEvent(tenantID, actor, action, resource, metadata)
	e.OccurredAt = t
	return e
}

// appendAll appends events (already stamped with their OccurredAt) in order,
// returning the resulting chain.
func appendAll(chain audit.EventChain, events ...audit.Event) audit.EventChain {
	for _, e := range events {
		chain = audit.AppendEvent(chain, e)
	}
	return chain
}

// Case 1: in-order timestamps, cut lands in the middle of the chain.
func TestCutAtTime_BasicPrefix(t *testing.T) {
	t.Log("\tGiven a chain with strictly increasing occurred-at timestamps")
	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	chain := audit.NewEventChain(10)
	chain = appendAll(chain,
		eventAt(tenantID, base, "a0"),
		eventAt(tenantID, base.Add(1*time.Minute), "a1"),
		eventAt(tenantID, base.Add(2*time.Minute), "a2"),
		eventAt(tenantID, base.Add(3*time.Minute), "a3"),
		eventAt(tenantID, base.Add(4*time.Minute), "a4"),
	)

	cutAt := base.Add(2 * time.Minute)
	t.Logf("\tWhen cutting at %s (the third event's own timestamp)", cutAt)
	got := audit.CutAtTime(chain, cutAt)

	t.Log("\tThen the result should contain exactly the first 3 events")
	if len(got.Events) != 3 {
		t.Fatalf("\t%s\tExpected 3 events in prefix, got %d", fail, len(got.Events))
	}
	for i, want := range []string{"a0", "a1", "a2"} {
		if got.Events[i].Action != want {
			t.Fatalf("\t%s\tEvent %d action mismatch: got %s, want %s", fail, i, got.Events[i].Action, want)
		}
	}

	t.Log("\tAnd the result should verify as a valid chain")
	if !audit.VerifyChain(got) {
		t.Fatalf("\t%s\tCut prefix failed VerifyChain", fail)
	}
	t.Logf("\t%s\tBasic prefix cut correct and verifiable\n", pass)
}

// Case 2: THE adversarial case. Chain append order does not match
// occurred-at order — a naive `occurred-at <= t` filter over the full event
// set picks a non-prefix subset that fails VerifyChain. CutAtTime must
// instead walk in append order and stop at the first event after t.
func TestCutAtTime_AdversarialOutOfOrder(t *testing.T) {
	t.Log("\tGiven a chain appended in order but with out-of-order occurred-at timestamps [10:00, 10:05, 10:02, 10:07]")
	tenantID := uuid.New()
	day := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	t1000 := day.Add(10*time.Hour + 0*time.Minute)
	t1005 := day.Add(10*time.Hour + 5*time.Minute)
	t1002 := day.Add(10*time.Hour + 2*time.Minute)
	t1007 := day.Add(10*time.Hour + 7*time.Minute)

	chain := audit.NewEventChain(10)
	chain = appendAll(chain,
		eventAt(tenantID, t1000, "e0-1000"),
		eventAt(tenantID, t1005, "e1-1005"),
		eventAt(tenantID, t1002, "e2-1002"),
		eventAt(tenantID, t1007, "e3-1007"),
	)

	cutAt := day.Add(10*time.Hour + 3*time.Minute) // 10:03
	t.Logf("\tWhen cutting at %s", cutAt)
	got := audit.CutAtTime(chain, cutAt)

	t.Log("\tThen the result should contain only the genesis-anchored prefix [10:00] — the append-order walk must stop at 10:05, even though 10:02 (later in append order) is <= 10:03")
	if len(got.Events) != 1 {
		t.Fatalf("\t%s\tExpected 1 event in prefix, got %d (actions: %v)", fail, len(got.Events), actionsOf(got))
	}
	if got.Events[0].Action != "e0-1000" {
		t.Fatalf("\t%s\tExpected only the 10:00 event, got %s", fail, got.Events[0].Action)
	}

	t.Log("\tAnd the cut prefix should verify")
	if !audit.VerifyChain(got) {
		t.Fatalf("\t%s\tCut prefix failed VerifyChain", fail)
	}
	t.Logf("\t%s\tAppend-order cut produced the correct genesis-anchored prefix\n", pass)

	t.Log("\tWhen a naive occurred-at<=t filter is applied instead (selecting events 0 and 2 by timestamp, keeping their original hashes unchanged)")
	naive := audit.EventChain{
		ID:     chain.ID,
		Events: []audit.Event{chain.Events[0], chain.Events[2]},
	}

	t.Log("\tThen the naive filter's subset must FAIL verification — it disproves itself, because event 2's PrevHash links to event 1, which is missing")
	if audit.VerifyChain(naive) {
		t.Fatalf("\t%s\tNaive timestamp-filtered subset unexpectedly verified — this is the splice bug the append-order cut exists to prevent", fail)
	}
	t.Logf("\t%s\tConfirmed: naive occurred-at filter produces a self-disproving export\n", pass)
}

func actionsOf(c audit.EventChain) []string {
	actions := make([]string, len(c.Events))
	for i, e := range c.Events {
		actions[i] = e.Action
	}
	return actions
}

// Case 3: cut before the first event's timestamp omits the whole chain.
func TestCutAtTime_BeforeFirstEvent(t *testing.T) {
	t.Log("\tGiven a chain whose first event occurs at 10:00")
	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	chain := audit.NewEventChain(10)
	chain = appendAll(chain,
		eventAt(tenantID, base, "a0"),
		eventAt(tenantID, base.Add(time.Minute), "a1"),
	)

	cutAt := base.Add(-1 * time.Hour)
	t.Logf("\tWhen cutting at %s (before the first event)", cutAt)
	got := audit.CutAtTime(chain, cutAt)

	t.Log("\tThen the result should have zero events")
	if len(got.Events) != 0 {
		t.Fatalf("\t%s\tExpected 0 events, got %d", fail, len(got.Events))
	}
	t.Logf("\t%s\tCut-before-first-event correctly produced an empty prefix\n", pass)
}

// Case 4: cut after the last event's timestamp returns the whole chain.
func TestCutAtTime_AfterLastEvent(t *testing.T) {
	t.Log("\tGiven a chain whose last event occurs at 10:01")
	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	chain := audit.NewEventChain(10)
	chain = appendAll(chain,
		eventAt(tenantID, base, "a0"),
		eventAt(tenantID, base.Add(time.Minute), "a1"),
	)

	cutAt := base.Add(1 * time.Hour)
	t.Logf("\tWhen cutting at %s (after the last event)", cutAt)
	got := audit.CutAtTime(chain, cutAt)

	t.Log("\tThen the result should contain every event, unchanged")
	if len(got.Events) != len(chain.Events) {
		t.Fatalf("\t%s\tExpected %d events, got %d", fail, len(chain.Events), len(got.Events))
	}
	if !audit.VerifyChain(got) {
		t.Fatalf("\t%s\tFull chain cut failed VerifyChain", fail)
	}
	t.Logf("\t%s\tCut-after-last-event correctly returned the full chain\n", pass)
}

// Case 5: nanosecond-precision inclusive boundary — an event occurring at
// exactly t is included (!After(t) semantics), one 1ns later is excluded.
func TestCutAtTime_NanosecondBoundaryInclusive(t *testing.T) {
	t.Log("\tGiven a chain with an event exactly at t and another 1ns after t")
	tenantID := uuid.New()
	boundary := time.Date(2025, 6, 1, 10, 0, 0, 500, time.UTC)

	chain := audit.NewEventChain(10)
	chain = appendAll(chain,
		eventAt(tenantID, boundary, "at-boundary"),
		eventAt(tenantID, boundary.Add(1*time.Nanosecond), "one-ns-after"),
	)

	t.Logf("\tWhen cutting at exactly %s", boundary)
	got := audit.CutAtTime(chain, boundary)

	t.Log("\tThen only the event at the exact boundary should be included (inclusive)")
	if len(got.Events) != 1 {
		t.Fatalf("\t%s\tExpected 1 event at the inclusive boundary, got %d", fail, len(got.Events))
	}
	if got.Events[0].Action != "at-boundary" {
		t.Fatalf("\t%s\tExpected the boundary event, got %s", fail, got.Events[0].Action)
	}
	t.Logf("\t%s\tNanosecond boundary is correctly inclusive\n", pass)
}

// Case 6: CutAtTime must not mutate the input chain — mutating the returned
// prefix's events must not be visible through the original chain.
func TestCutAtTime_Immutability(t *testing.T) {
	t.Log("\tGiven a chain and a cut that yields a non-empty prefix")
	tenantID := uuid.New()
	base := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	chain := audit.NewEventChain(10)
	chain = appendAll(chain,
		eventAt(tenantID, base, "original-action"),
		eventAt(tenantID, base.Add(time.Minute), "a1"),
	)
	originalAction := chain.Events[0].Action
	originalLen := len(chain.Events)

	t.Log("\tWhen CutAtTime is called and the returned chain's events are mutated")
	got := audit.CutAtTime(chain, base.Add(1*time.Hour))
	if len(got.Events) == 0 {
		t.Fatalf("\t%s\tExpected a non-empty prefix to mutate", fail)
	}
	got.Events[0].Action = "mutated-by-caller"

	t.Log("\tThen the original chain must be unaffected")
	if chain.Events[0].Action != originalAction {
		t.Fatalf("\t%s\tCutAtTime leaked mutation into the original chain: got action %q, want %q", fail, chain.Events[0].Action, originalAction)
	}
	if len(chain.Events) != originalLen {
		t.Fatalf("\t%s\tCutAtTime changed the original chain's length: got %d, want %d", fail, len(chain.Events), originalLen)
	}
	t.Logf("\t%s\tCutAtTime did not mutate its input\n", pass)
}
