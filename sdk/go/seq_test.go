package ledgerly

import (
	"sync"
	"testing"
)

const testBlockSize = 8

func TestSequencer_FreshDir_StartsAtOne(t *testing.T) {
	dir := t.TempDir()

	s, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("openSequencer on fresh dir: %v", err)
	}

	got, err := s.next()
	if err != nil {
		t.Fatalf("next(): %v", err)
	}
	if got != 1 {
		t.Fatalf("expected the first sequence number on a fresh dir to be 1, got %d", got)
	}
}

func TestSequencer_SequentialAlloc_Increments(t *testing.T) {
	dir := t.TempDir()
	s, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("openSequencer: %v", err)
	}

	var prev uint64
	for i := 0; i < 5; i++ {
		got, err := s.next()
		if err != nil {
			t.Fatalf("next() call %d: %v", i, err)
		}
		if i > 0 && got != prev+1 {
			t.Fatalf("call %d: expected %d, got %d", i, prev+1, got)
		}
		prev = got
	}
}

func TestSequencer_CleanClose_ExactResume(t *testing.T) {
	dir := t.TempDir()
	s, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("openSequencer: %v", err)
	}

	var last uint64
	for i := 0; i < 3; i++ {
		last, err = s.next()
		if err != nil {
			t.Fatalf("next(): %v", err)
		}
	}
	if err := s.close(); err != nil {
		t.Fatalf("close(): %v", err)
	}

	reopened, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("re-openSequencer: %v", err)
	}
	got, err := reopened.next()
	if err != nil {
		t.Fatalf("next() after clean close/reopen: %v", err)
	}
	if got != last+1 {
		t.Fatalf("expected exact resume at %d after a clean close, got %d", last+1, got)
	}
}

func TestSequencer_CrashResume_GapBoundedByBlockSize(t *testing.T) {
	dir := t.TempDir()
	s, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("openSequencer: %v", err)
	}

	var last uint64
	for i := 0; i < 3; i++ {
		last, err = s.next()
		if err != nil {
			t.Fatalf("next(): %v", err)
		}
	}
	// Simulate a crash: no close() call, counter dropped without a
	// checkpoint of the exact last-issued value.

	reopened, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("re-openSequencer after simulated crash: %v", err)
	}
	got, err := reopened.next()
	if err != nil {
		t.Fatalf("next() after crash resume: %v", err)
	}

	if got <= last {
		t.Fatalf("expected crash resume to never reuse a sequence number: last issued %d, resumed at %d", last, got)
	}
	gap := got - last - 1
	if gap > testBlockSize {
		t.Fatalf("expected the crash-resume gap to be bounded by the reservation block size %d, got gap %d (resumed at %d, last %d)", testBlockSize, gap, got, last)
	}
}

func TestSequencer_ConcurrentAlloc_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	s, err := openSequencer(dir, testBlockSize)
	if err != nil {
		t.Fatalf("openSequencer: %v", err)
	}

	const goroutines = 20
	const perGoroutine = 25
	total := goroutines * perGoroutine

	results := make(chan uint64, total)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range perGoroutine {
				got, err := s.next()
				if err != nil {
					t.Errorf("next(): %v", err)
					return
				}
				results <- got
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[uint64]bool, total)
	for got := range results {
		if seen[got] {
			t.Fatalf("sequence number %d allocated more than once under concurrent access", got)
		}
		seen[got] = true
	}
	if len(seen) != total {
		t.Fatalf("expected %d unique sequence numbers, got %d", total, len(seen))
	}
}
