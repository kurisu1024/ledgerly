package ledgerly

import "log/slog"

// Reserved Metadata keys the SDK stamps onto every captured event.
// Metadata is already hashed (internal/audit/events.go writeMapSorted), so
// stamping these costs zero chain-format change. The SDK overwrites these
// keys unconditionally on every captured record so a host application
// cannot spoof them via its own slog attrs (ADR-0001 amendments).
const (
	MetaInstanceID = "ledgerly.sdk-instance-id"
	MetaSeq        = "ledgerly.sdk-seq"
)

// internalAttr marks a log record as SDK-internal diagnostics. Handle()
// short-circuits any record carrying it (self-suppression guard #2); the
// SDK's own internal logger also never routes through this Handler at all
// (guard #1) — see handler.go.
const internalAttr = "ledgerly.internal"

// eventTypeAttr is the reserved slog attribute a record must carry to be
// matcher-eligible: every v1 rule requires event-type, so a record with no
// event-type can never trigger and is skipped before matching.
const eventTypeAttr = "event-type"

// capturedRecord is the flattened view of a slog.Record the matcher and
// stamping pipeline operate on.
type capturedRecord struct {
	Level     string
	EventType string
	Actor     map[string]string
	Resource  map[string]string
	Fields    map[string]string
	Metadata  map[string]string
}

// flattenRecord extracts a capturedRecord from r. Actor and Resource come
// from "actor" and "resource" slog groups; every other top-level attr
// becomes a Fields entry keyed by its name.
//
// STUB: not implemented — always returns a zero-value capturedRecord.
func flattenRecord(r slog.Record) capturedRecord {
	return capturedRecord{}
}

// isInternal reports whether r carries the self-suppression attr.
//
// STUB: not implemented — always reports false.
func isInternal(r slog.Record) bool {
	return false
}

// applyReservedStamps returns a copy of metadata with MetaInstanceID and
// MetaSeq unconditionally set to instanceID and seq — overwriting any
// value already present under those keys, so a host application cannot
// spoof its own event's provenance stamps.
//
// STUB: not implemented — returns metadata unchanged, so reserved keys are
// NOT stamped and a spoofed value is NOT overwritten.
func applyReservedStamps(metadata map[string]string, instanceID string, seq uint64) map[string]string {
	return metadata
}
