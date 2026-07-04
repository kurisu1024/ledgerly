package ledgerly

import (
	"log/slog"
	"testing"
	"time"
)

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

func TestIsInternal_TrueWhenAttrPresent(t *testing.T) {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	r.AddAttrs(slog.Bool(internalAttr, true))

	if !isInternal(r) {
		t.Fatal("expected isInternal to report true for a record carrying the reserved internal attr")
	}
}

func TestIsInternal_FalseWhenAttrAbsent(t *testing.T) {
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)

	if isInternal(r) {
		t.Fatal("expected isInternal to report false for an ordinary record")
	}
}
