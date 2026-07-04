package ledgerly

import (
	"log/slog"
	"testing"
	"time"
)

// flattenVia flattens r as h's capture path would, exposing how a derived
// handler's bound groups/attrs route into the captured record.
func flattenVia(t *testing.T, h slog.Handler, r slog.Record) capturedRecord {
	t.Helper()
	hh, ok := h.(*Handler)
	if !ok {
		t.Fatalf("expected a *Handler, got %T", h)
	}
	return flattenForHandler(r, hh.groups, hh.attrs)
}

func TestWithGroup_BoundAttrsRouteToReservedGroups(t *testing.T) {
	// slog contract: attrs added after WithGroup belong under that group,
	// so logger.WithGroup("resource").With("id", "x") must flatten exactly
	// like an inline slog.Group("resource", "id", "x") — into Resource,
	// not the flat Fields map.
	base := newTestHandler(t, newSpyHandler(true))
	r := slog.NewRecord(time.Now(), slog.LevelWarn, "delete", 0)
	r.Add("event-type", "project.delete")

	cases := []struct {
		group string
		get   func(capturedRecord) map[string]string
	}{
		{actorGroup, func(cr capturedRecord) map[string]string { return cr.Actor }},
		{resourceGroup, func(cr capturedRecord) map[string]string { return cr.Resource }},
		{metadataGroup, func(cr capturedRecord) map[string]string { return cr.Metadata }},
	}
	for _, c := range cases {
		derived := base.WithGroup(c.group).WithAttrs([]slog.Attr{slog.String("id", "x")})
		cr := flattenVia(t, derived, r)

		if got := c.get(cr)["id"]; got != "x" {
			t.Errorf("WithGroup(%q).WithAttrs(id=x): expected the bound attr routed into %s, got %q (Fields = %v)", c.group, c.group, got, cr.Fields)
		}
		if _, leaked := cr.Fields["id"]; leaked {
			t.Errorf("WithGroup(%q).WithAttrs(id=x): bound attr leaked into top-level Fields: %v", c.group, cr.Fields)
		}
	}
}

func TestWithGroup_BoundAttrsNestedGroupsDotJoin(t *testing.T) {
	base := newTestHandler(t, newSpyHandler(true))
	derived := base.WithGroup("billing").WithGroup("invoice").WithAttrs([]slog.Attr{slog.String("id", "inv-1")})

	r := slog.NewRecord(time.Now(), slog.LevelWarn, "delete", 0)
	r.Add("event-type", "project.delete")
	cr := flattenVia(t, derived, r)

	if got := cr.Fields["billing.invoice.id"]; got != "inv-1" {
		t.Fatalf("expected nested bound groups to dot-join like inline nested groups, got Fields = %v", cr.Fields)
	}
}

func TestWithGroup_RecordAttrsFlattenLikeBoundAttrs(t *testing.T) {
	// Record attrs on a WithGroup("resource") handler must route the same
	// way bound attrs do — into Resource — keeping the two paths identical.
	base := newTestHandler(t, newSpyHandler(true))
	derived := base.WithGroup(resourceGroup)

	r := slog.NewRecord(time.Now(), slog.LevelWarn, "delete", 0)
	r.Add("id", "proj-9")
	cr := flattenVia(t, derived, r)

	if got := cr.Resource["id"]; got != "proj-9" {
		t.Fatalf("expected the record attr under the handler's resource group in Resource, got %q (Fields = %v)", got, cr.Fields)
	}
}

func TestApplyReservedStamps_PresentAndNotOverridableByUserAttrs(t *testing.T) {
	spoofed := map[string]string{
		MetaInstanceID: "attacker-supplied-instance",
		MetaSeq:        "999999",
		"other":        "kept-as-is",
	}

	got := applyReservedStamps(spoofed, "real-instance-id", 42)

	if got[MetaInstanceID] != "real-instance-id" {
		t.Fatalf("expected %s to be overwritten to %q, got %q", MetaInstanceID, "real-instance-id", got[MetaInstanceID])
	}
	if got[MetaSeq] != "42" {
		t.Fatalf("expected %s to be overwritten to %q, got %q", MetaSeq, "42", got[MetaSeq])
	}
	if got["other"] != "kept-as-is" {
		t.Fatalf("expected non-reserved metadata to survive stamping, got %q", got["other"])
	}
}

func TestApplyReservedStamps_PresentOnNilMetadata(t *testing.T) {
	got := applyReservedStamps(nil, "instance-a", 1)

	if got[MetaInstanceID] != "instance-a" {
		t.Fatalf("expected %s to be set even when starting from nil metadata, got %q", MetaInstanceID, got[MetaInstanceID])
	}
	if got[MetaSeq] != "1" {
		t.Fatalf("expected %s to be set even when starting from nil metadata, got %q", MetaSeq, got[MetaSeq])
	}
}
