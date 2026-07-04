package ledgerly

import (
	"log/slog"
	"strconv"
)

// Reserved Metadata keys the SDK stamps onto every captured event.
// Metadata is already hashed (internal/audit/events.go writeMapSorted), so
// stamping these costs zero chain-format change. The SDK overwrites these
// keys unconditionally on every captured record so a host application
// cannot spoof them via its own slog attrs (ADR-0001 amendments).
const (
	MetaInstanceID = "ledgerly.sdk-instance-id"
	MetaSeq        = "ledgerly.sdk-seq"
)

// internalAttr tags SDK-internal diagnostic records written onto next so
// an app can identify (or filter) them in its own log output. It is
// informational only — Handle() attaches NO semantics to it. A user
// record carrying it is captured like any other, so the attr cannot be
// used to evade audit capture; self-suppression relies solely on
// logInternal writing diagnostics directly to next, bypassing Handle()
// and capture() entirely (handler.go).
const internalAttr = "ledgerly.internal"

// eventTypeAttr is the reserved slog attribute a record must carry to be
// matcher-eligible: every v1 rule requires event-type, so a record with no
// event-type can never trigger and is skipped before matching.
const eventTypeAttr = "event-type"

// Reserved top-level slog group names routed into their own event fields
// rather than the flat Fields map.
const (
	actorGroup    = "actor"
	resourceGroup = "resource"
	metadataGroup = "metadata"
)

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

// levelName maps a slog level onto the four rule-level names, bucketing
// intermediate custom levels downward (e.g. INFO+2 is still "info").
func levelName(l slog.Level) string {
	name := "error"
	switch {
	case l < slog.LevelInfo:
		name = "debug"
	case l < slog.LevelWarn:
		name = "info"
	case l < slog.LevelError:
		name = "warn"
	}
	return name
}

// flattenForHandler extracts a capturedRecord from r as seen through a
// handler carrying bound groups and attrs (from WithGroup/WithAttrs).
// Actor, Resource, and Metadata come from the like-named top-level slog
// groups; the reserved event-type attr becomes EventType; every other attr
// becomes a Fields entry keyed by its (dotted, for nested groups) name.
// Bound attrs flatten first, so a record attr under the same key wins.
func flattenForHandler(r slog.Record, groups []string, bound []slog.Attr) capturedRecord {
	cr := capturedRecord{
		Level:    levelName(r.Level),
		Actor:    map[string]string{},
		Resource: map[string]string{},
		Fields:   map[string]string{},
		Metadata: map[string]string{},
	}
	for _, a := range bound {
		flattenAttr(&cr, "", a)
	}
	prefix := ""
	for _, g := range groups {
		prefix = joinKey(prefix, g)
	}
	r.Attrs(func(a slog.Attr) bool {
		flattenAttr(&cr, prefix, a)
		return true
	})
	return cr
}

// flattenAttr routes a single (possibly group) attr into cr. prefix is the
// dotted group path already entered, empty at the top level.
func flattenAttr(cr *capturedRecord, prefix string, a slog.Attr) {
	a.Value = a.Value.Resolve()

	if a.Value.Kind() == slog.KindGroup {
		var into map[string]string
		if prefix == "" {
			switch a.Key {
			case actorGroup:
				into = cr.Actor
			case resourceGroup:
				into = cr.Resource
			case metadataGroup:
				into = cr.Metadata
			}
		}
		for _, member := range a.Value.Group() {
			if into != nil {
				member.Value = member.Value.Resolve()
				into[member.Key] = member.Value.String()
				continue
			}
			flattenAttr(cr, joinKey(prefix, a.Key), member)
		}
		return
	}

	key := joinKey(prefix, a.Key)
	if key == eventTypeAttr {
		cr.EventType = a.Value.String()
		return
	}
	cr.Fields[key] = a.Value.String()
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// matchInput builds the matcher's view of cr: Fields plus the actor.* and
// resource.* dotted paths, mirroring how server-side FieldCond keys are
// authored.
func (cr capturedRecord) matchInput() MatchInput {
	fields := make(map[string]string, len(cr.Fields)+len(cr.Actor)+len(cr.Resource))
	for k, v := range cr.Fields {
		fields[k] = v
	}
	for k, v := range cr.Actor {
		fields[actorGroup+"."+k] = v
	}
	for k, v := range cr.Resource {
		fields[resourceGroup+"."+k] = v
	}
	return MatchInput{Level: cr.Level, EventType: cr.EventType, Fields: fields}
}

// applyReservedStamps returns a copy of metadata with MetaInstanceID and
// MetaSeq unconditionally set to instanceID and seq — overwriting any
// value already present under those keys, so a host application cannot
// spoof its own event's provenance stamps.
func applyReservedStamps(metadata map[string]string, instanceID string, seq uint64) map[string]string {
	stamped := make(map[string]string, len(metadata)+2)
	for k, v := range metadata {
		stamped[k] = v
	}
	stamped[MetaInstanceID] = instanceID
	stamped[MetaSeq] = strconv.FormatUint(seq, 10)
	return stamped
}
