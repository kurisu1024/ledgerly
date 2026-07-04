package audit

import (
	"slices"
	"time"
)

// CutAtTime returns the longest genesis-anchored prefix of chain in which
// every event's OccurredAt is at-or-before t (inclusive nanosecond boundary
// via !After(t)). It stops at the first event strictly after t, so the
// result is always a verifiable prefix by construction.
//
// This is the load-bearing decision behind issue #20: occurred-at ordering
// within a chain is not guaranteed (client-supplied timestamps, out-of-order
// queue arrival — see worker.go), so a naive `occurred-at <= t` filter over
// the full event set can select a non-prefix subset that fails VerifyChain.
// CutAtTime instead treats t purely as a selector for where to stop walking
// the append-ordered chain, never as a filter predicate applied per-event
// independent of position.
//
// CutAtTime does not mutate chain; it returns a new EventChain value whose
// Events slice is a copy, so callers may modify the result freely.
func CutAtTime(chain EventChain, t time.Time) EventChain {
	cut := len(chain.Events)
	for i, e := range chain.Events {
		if e.OccurredAt.After(t) {
			cut = i
			break
		}
	}
	return EventChain{
		ID:     chain.ID,
		Events: slices.Clone(chain.Events[:cut]),
	}
}
