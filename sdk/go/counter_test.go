package ledgerly

import (
	"os"
	"path/filepath"
	"testing"
)

func openCounterFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(t.TempDir(), "counter"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("opening counter file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestCounter_FreshFileReadsZero(t *testing.T) {
	f := openCounterFile(t)

	got, err := readCounter(f)
	if err != nil {
		t.Fatalf("readCounter on a fresh file: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected a genuinely-empty counter file to read as 0, got %d", got)
	}
}

func TestCounter_LastWriteWins_IncludingLowerValues(t *testing.T) {
	// The buffer cursor legitimately rewinds (reset to 0 on compaction), so
	// the counter must return the LAST durably written value, not the max.
	f := openCounterFile(t)

	for _, v := range []uint64{100, 250, 0, 7} {
		if err := writeCounter(f, v); err != nil {
			t.Fatalf("writeCounter(%d): %v", v, err)
		}
		got, err := readCounter(f)
		if err != nil {
			t.Fatalf("readCounter after writing %d: %v", v, err)
		}
		if got != v {
			t.Fatalf("expected the last written value %d, got %d", v, got)
		}
	}
}

func TestCounter_TornWrite_ResumesFromPreviousDurableValue(t *testing.T) {
	// A torn write may leave the slot being written garbled — including as
	// a plausible-looking smaller decimal. The checksum must reject it and
	// the read must fall back to the other slot's (previous, safe) value,
	// never silently parse the garbled number.
	f := openCounterFile(t)

	if err := writeCounter(f, 1000); err != nil { // gen 1 → slot 1
		t.Fatalf("writeCounter(1000): %v", err)
	}
	if err := writeCounter(f, 2000); err != nil { // gen 2 → slot 0
		t.Fatalf("writeCounter(2000): %v", err)
	}

	// Tear the newest slot (slot 0): splice a smaller decimal over the
	// value field without updating the checksum.
	torn := []byte("00000000000000000005")
	if _, err := f.WriteAt(torn, counterFieldWidth+1); err != nil {
		t.Fatalf("simulating torn write: %v", err)
	}

	got, err := readCounter(f)
	if err != nil {
		t.Fatalf("readCounter after torn write: %v", err)
	}
	if got == 5 {
		t.Fatal("readCounter silently parsed the garbled (rewound) value 5 from a torn write")
	}
	if got != 1000 {
		t.Fatalf("expected the previous durable value 1000 after a torn write, got %d", got)
	}
}

func TestCounter_GarbledFileFailsLoudly(t *testing.T) {
	f := openCounterFile(t)
	if _, err := f.WriteAt([]byte("total garbage, definitely not a counter record\n"), 0); err != nil {
		t.Fatalf("seeding garbage: %v", err)
	}

	if _, err := readCounter(f); err == nil {
		t.Fatal("expected a non-empty counter file with no valid slot to fail loudly, got nil error")
	}
}

func TestCounter_ShortPartialFileFailsLoudly(t *testing.T) {
	f := openCounterFile(t)
	if _, err := f.WriteAt([]byte("0000000000"), 0); err != nil {
		t.Fatalf("seeding partial slot: %v", err)
	}

	if _, err := readCounter(f); err == nil {
		t.Fatal("expected a short, partial counter file to fail loudly, got nil error")
	}
}

func TestSequencer_CorruptReservationFile_FailsLoudlyOnOpen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, seqReserveFile), []byte("87 bogus bytes pretending to be a reservation"), 0o600); err != nil {
		t.Fatalf("seeding corrupt reservation file: %v", err)
	}

	if _, err := openSequencer(dir, testBlockSize); err == nil {
		t.Fatal("expected openSequencer to fail loudly on a corrupt reservation file (a silent rewind would duplicate sequence numbers), got nil error")
	}
}

func TestBuffer_CorruptCursorFile_FailsLoudlyOnOpen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, cursorFile), []byte("not a cursor"), 0o600); err != nil {
		t.Fatalf("seeding corrupt cursor file: %v", err)
	}

	if _, err := openBuffer(dir); err == nil {
		t.Fatal("expected openBuffer to fail loudly on a corrupt replay cursor, got nil error")
	}
}
