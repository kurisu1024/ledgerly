package audit

import "time"

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
// CutAtTime does not mutate chain; it returns a new EventChain value.
func CutAtTime(chain EventChain, t time.Time) EventChain {
	// STUB (RED phase, issue #20): returns chain unchanged so callers and
	// tests compile. Real prefix-cut and copy-on-return semantics land in
	// the GREEN pass.
	return chain
}
