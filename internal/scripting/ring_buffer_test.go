package scripting

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/storage"
)

// makeTestSM creates a bare StateManager with pre-configured ring buffer
// fields for testing getFlatHistoryInternal. It bypasses NewStateManager
// to avoid needing a real storage backend.
func makeTestSM() *StateManager {
	return &StateManager{
		historyBuf:   make([]storage.HistoryEntry, maxHistoryEntries),
		historyStart: 0,
		historyLen:   0,
	}
}

func entryAt(id string) storage.HistoryEntry {
	return storage.HistoryEntry{EntryID: id}
}

// ── Empty buffer ───────────────────────────────────────────────────

func TestGetFlatHistoryInternal_Empty(t *testing.T) {
	sm := makeTestSM()
	got := sm.getFlatHistoryInternal()
	if got != nil {
		t.Errorf("empty buffer: got %v, want nil", got)
	}
}

// ── Contiguous (no wrap-around) ────────────────────────────────────

func TestGetFlatHistoryInternal_SingleEntry(t *testing.T) {
	sm := makeTestSM()
	sm.historyBuf[0] = entryAt("A")
	sm.historyLen = 1
	got := sm.getFlatHistoryInternal()
	if len(got) != 1 || got[0].EntryID != "A" {
		t.Errorf("single entry: got %v, want [A]", entryIDs(got))
	}
}

func TestGetFlatHistoryInternal_ContiguousFromZero(t *testing.T) {
	sm := makeTestSM()
	for i := 0; i < 5; i++ {
		sm.historyBuf[i] = entryAt(string(rune('A' + i)))
	}
	sm.historyStart = 0
	sm.historyLen = 5
	got := sm.getFlatHistoryInternal()
	assertEntryIDs(t, got, "A", "B", "C", "D", "E")
}

func TestGetFlatHistoryInternal_ContiguousNonZeroStart(t *testing.T) {
	sm := makeTestSM()
	// Entries at positions 10..14
	for i := 0; i < 5; i++ {
		sm.historyBuf[10+i] = entryAt(string(rune('A' + i)))
	}
	sm.historyStart = 10
	sm.historyLen = 5
	got := sm.getFlatHistoryInternal()
	assertEntryIDs(t, got, "A", "B", "C", "D", "E")
}

func TestGetFlatHistoryInternal_ContiguousFull(t *testing.T) {
	sm := makeTestSM()
	// Fill the entire buffer contiguously (start=0)
	for i := 0; i < maxHistoryEntries; i++ {
		sm.historyBuf[i] = entryAt(string(rune('A' + (i % 26))))
	}
	sm.historyStart = 0
	sm.historyLen = maxHistoryEntries
	got := sm.getFlatHistoryInternal()
	if len(got) != maxHistoryEntries {
		t.Fatalf("contiguous full: got %d entries, want %d", len(got), maxHistoryEntries)
	}
	if got[0].EntryID != "A" {
		t.Errorf("first entry: got %q, want %q", got[0].EntryID, "A")
	}
}

// ── Wrap-around ────────────────────────────────────────────────────

func TestGetFlatHistoryInternal_WrapAround_Simple(t *testing.T) {
	sm := makeTestSM()
	// Simulate: buffer size 200, start=198, len=5
	// Entries at positions: 198(A), 199(B), 0(C), 1(D), 2(E)
	sm.historyBuf[198] = entryAt("A")
	sm.historyBuf[199] = entryAt("B")
	sm.historyBuf[0] = entryAt("C")
	sm.historyBuf[1] = entryAt("D")
	sm.historyBuf[2] = entryAt("E")
	sm.historyStart = 198
	sm.historyLen = 5
	got := sm.getFlatHistoryInternal()
	assertEntryIDs(t, got, "A", "B", "C", "D", "E")
}

func TestGetFlatHistoryInternal_WrapAround_Full(t *testing.T) {
	sm := makeTestSM()
	// Full buffer with wrap: start=100, len=200
	// Entries 100..199 then 0..99
	for i := 0; i < maxHistoryEntries; i++ {
		idx := (100 + i) % maxHistoryEntries
		sm.historyBuf[idx] = entryAt(string(rune('A' + (i % 26))))
	}
	sm.historyStart = 100
	sm.historyLen = maxHistoryEntries
	got := sm.getFlatHistoryInternal()
	if len(got) != maxHistoryEntries {
		t.Fatalf("wrap full: got %d entries, want %d", len(got), maxHistoryEntries)
	}
	// First entry should be the one at index 100
	if got[0].EntryID != "A" {
		t.Errorf("first entry: got %q, want %q", got[0].EntryID, "A")
	}
}

func TestGetFlatHistoryInternal_WrapAround_OneElement(t *testing.T) {
	sm := makeTestSM()
	// start=199, len=1 — edge case: last slot, no actual wrap
	sm.historyBuf[199] = entryAt("X")
	sm.historyStart = 199
	sm.historyLen = 1
	got := sm.getFlatHistoryInternal()
	assertEntryIDs(t, got, "X")
}

func TestGetFlatHistoryInternal_WrapAround_TwoElements(t *testing.T) {
	sm := makeTestSM()
	// start=199, len=2 — wraps from 199 to 0
	sm.historyBuf[199] = entryAt("Y")
	sm.historyBuf[0] = entryAt("Z")
	sm.historyStart = 199
	sm.historyLen = 2
	got := sm.getFlatHistoryInternal()
	assertEntryIDs(t, got, "Y", "Z")
}

// ── Returned slice is independent copy ─────────────────────────────

func TestGetFlatHistoryInternal_ReturnsCopy(t *testing.T) {
	sm := makeTestSM()
	sm.historyBuf[0] = entryAt("A")
	sm.historyBuf[1] = entryAt("B")
	sm.historyLen = 2
	got := sm.getFlatHistoryInternal()
	// Mutate the returned slice
	got[0].EntryID = "MUTATED"
	// Original buffer should be unchanged
	if sm.historyBuf[0].EntryID != "A" {
		t.Error("mutations to returned slice should not affect buffer")
	}
}

// ── helpers ────────────────────────────────────────────────────────

func entryIDs(entries []storage.HistoryEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.EntryID
	}
	return ids
}

func assertEntryIDs(t *testing.T, got []storage.HistoryEntry, wantIDs ...string) {
	t.Helper()
	if len(got) != len(wantIDs) {
		t.Fatalf("len mismatch: got %d, want %d — got IDs: %v", len(got), len(wantIDs), entryIDs(got))
	}
	for i, id := range wantIDs {
		if got[i].EntryID != id {
			t.Errorf("entry[%d]: got %q, want %q", i, got[i].EntryID, id)
		}
	}
}
