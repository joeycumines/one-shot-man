package vt

import (
	"testing"
)

// TestDECRC_WithoutPriorDECSC verifies that DECRC without a prior DECSC
// does not crash and restores the cursor to (0,0) with default attributes.
// All Saved* fields default to zero, so DECRC restores cursor to (0,0).
func TestDECRC_WithoutPriorDECSC(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// Move cursor away from defaults.
	scr.CurRow = 10
	scr.CurCol = 20
	scr.ApplicationCursor = true
	scr.AutoWrap = false
	scr.PendingWrap = true

	// DECRC without prior DECSC — must not panic.
	v.esc.Dispatch(scr, '8')

	// Saved* fields default to zero, so cursor goes to (0,0).
	if scr.CurRow != 0 {
		t.Errorf("CurRow = %d, want 0 (no prior DECSC)", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Errorf("CurCol = %d, want 0 (no prior DECSC)", scr.CurCol)
	}
	// Default attr is zero-valued.
	if scr.CurAttr != (Attr{}) {
		t.Error("CurAttr should be zero-valued after DECRC with no prior DECSC")
	}
	// All modes are off (Saved* booleans default false, SavedAutoWrap=false).
	if scr.ApplicationCursor {
		t.Error("ApplicationCursor should be false after DECRC with no prior DECSC")
	}
	if scr.AutoWrap {
		t.Error("AutoWrap should be false after DECRC with no prior DECSC (SavedAutoWrap defaults false)")
	}
	if scr.PendingWrap {
		t.Error("PendingWrap should be false after DECRC with no prior DECSC")
	}
}

// TestDECSC_DoubleDECSC verifies that two DECSCs followed by one DECRC
// restores the second save (the Saved* fields are overwritten, not stacked).
func TestDECSC_DoubleDECSC(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// First DECSC — save state A.
	scr.CurRow = 1
	scr.CurCol = 2
	scr.ApplicationCursor = true
	v.esc.Dispatch(scr, '7')
	rowA, colA, appA := scr.SavedRow, scr.SavedCol, scr.SavedApplicationCursor

	if rowA != 1 || colA != 2 || !appA {
		t.Fatalf("first DECSC save unexpected: (%d,%d) app=%v", rowA, colA, appA)
	}

	// Modify cursor and mode flags.
	scr.CurRow = 5
	scr.CurCol = 6
	scr.ApplicationCursor = false

	// Second DECSC — saves state B (overwrites Saved*).
	v.esc.Dispatch(scr, '7')

	// DECRC should restore the second save (state B).
	v.esc.Dispatch(scr, '8')

	if scr.CurRow != 5 {
		t.Errorf("CurRow = %d, want 5 (second DECSC state)", scr.CurRow)
	}
	if scr.CurCol != 6 {
		t.Errorf("CurCol = %d, want 6 (second DECSC state)", scr.CurCol)
	}
	if scr.ApplicationCursor {
		t.Error("ApplicationCursor should be false (second DECSC state)")
	}

	// The first DECSC state is overwritten — verify it's gone.
	if scr.SavedRow != 5 || scr.SavedCol != 6 {
		t.Errorf("Saved* still hold first state: SavedRow=%d SavedCol=%d, want (5,6)", scr.SavedRow, scr.SavedCol)
	}
}

// TestDECSC_OriginModeScrollRegion verifies that DECRC with OriginMode and
// scroll region set correctly clamps the restored cursor relative to the
// scroll region.
func TestDECSC_OriginModeScrollRegion(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// Set scroll region to rows 5-10 (1-indexed internally).
	v.csi.Dispatch(scr, 'r', []int{5, 10}, false)

	// Save position that is outside the scroll region (row 2, which is below row 5).
	scr.SavedRow = 2
	scr.SavedCol = 10
	scr.SavedOriginMode = true
	v.esc.Dispatch(scr, '8')

	// With OriginMode, the cursor should be clamped to the scroll region.
	scrollTop, scrollBot := scr.ScrollRegion()
	wantRow := max(scrollTop, min(2, scrollBot-1))
	if scr.CurRow != wantRow {
		t.Errorf("CurRow = %d, want %d (clamped to scroll region [%d,%d])",
			scr.CurRow, wantRow, scrollTop, scrollBot-1)
	}

	// Now test with a SavedRow inside the scroll region.
	scr.SavedRow = 7
	scr.SavedCol = 10
	v.esc.Dispatch(scr, '7') // save this state

	scr.SavedRow = 15
	scr.SavedCol = 10
	v.esc.Dispatch(scr, '8')

	// Row 15 clamped to scroll region [5,10].
	wantRow2 := max(scrollTop, min(15, scrollBot-1))
	if scr.CurRow != wantRow2 {
		t.Errorf("CurRow = %d, want %d (clamped to scroll region)",
			scr.CurRow, wantRow2)
	}

	// Without OriginMode, clamping uses screen bounds instead.
	scr.OriginMode = false
	scr.SavedRow = 30
	scr.SavedCol = 10
	v.esc.Dispatch(scr, '7')

	scr.SavedRow = 30
	v.esc.Dispatch(scr, '8')

	if scr.CurRow != 23 {
		t.Errorf("CurRow = %d, want 23 (clamped to screen height 24)", scr.CurRow)
	}
}

// TestDECSC_Mode1047 verifies that switching to alternate screen via mode
// 1047 (clear on exit only) does not affect the DECSC saved state.
func TestDECSC_Mode1047(t *testing.T) {
	v := NewVTerm(24, 80)
	scr := v.active

	// Set up state on primary and save it.
	scr.CurRow = 5
	scr.CurCol = 10
	scr.ApplicationCursor = true
	v.esc.Dispatch(scr, '7')

	// Verify the save.
	if v.primary.SavedRow != 5 || v.primary.SavedCol != 10 {
		t.Fatalf("DECSC saved (%d,%d), want (5,10)", v.primary.SavedRow, v.primary.SavedCol)
	}
	if !v.primary.SavedApplicationCursor {
		t.Error("DECSC did not save ApplicationCursor=true")
	}

	// Switch to alternate screen via mode 1047 (cursor save on entry, clear on exit).
	v.csi.Dispatch(v.active, 'h', []int{1047}, true)
	if v.active != v.alternate {
		t.Fatal("should be on alternate screen after DECSET ?1047h")
	}

	// Work on alternate screen.
	v.alternate.CurRow = 15
	v.alternate.CurCol = 40
	v.alternate.ApplicationCursor = false

	// Switch back via mode 1047 — clears alternate but does NOT restore cursor.
	v.csi.Dispatch(v.active, 'l', []int{1047}, true)
	if v.active != v.primary {
		t.Fatal("should be on primary screen after DECRST ?1047l")
	}

	// Cursor should remain at the position it had on primary before 1047
	// (5, 10), not the alternate screen position.
	if v.primary.CurRow != 5 {
		t.Errorf("CurRow = %d, want 5 (primary position, 1047 does not restore cursor)", v.primary.CurRow)
	}
	if v.primary.CurCol != 10 {
		t.Errorf("CurCol = %d, want 10 (primary position, 1047 does not restore cursor)", v.primary.CurCol)
	}

	// DECSC save should still be intact.
	v.esc.Dispatch(v.primary, '8')
	if v.primary.CurRow != 5 {
		t.Errorf("CurRow = %d after DECRC, want 5 (DECSC state preserved)", v.primary.CurRow)
	}
	if v.primary.CurCol != 10 {
		t.Errorf("CurCol = %d after DECRC, want 10 (DECSC state preserved)", v.primary.CurCol)
	}
	if !v.primary.ApplicationCursor {
		t.Error("ApplicationCursor not restored by DECRC after 1047 round-trip")
	}
}

// TestDECRC_RestoresPendingWrap verifies that DECRC correctly restores
// the SavedPendingWrap flag.
func TestDECRC_RestoresPendingWrap(t *testing.T) {
	v := NewVTerm(5, 10)
	scr := v.active

	// Write characters to reach the right margin (10 chars in a 10-column line).
	for range 10 {
		scr.PutChar('X')
	}
	if !scr.PendingWrap {
		t.Fatal("PendingWrap should be true at right margin")
	}

	// Save this state (PendingWrap=true).
	v.esc.Dispatch(scr, '7')
	if !scr.SavedPendingWrap {
		t.Fatal("DECSC should have saved PendingWrap=true")
	}

	// Move cursor back, clear PendingWrap.
	scr.CurCol = 0
	scr.PendingWrap = false

	// Restore — PendingWrap should come back.
	v.esc.Dispatch(scr, '8')
	if !scr.PendingWrap {
		t.Error("PendingWrap not restored by DECRC")
	}
	if scr.CurCol != 9 {
		t.Errorf("CurCol = %d, want 9 (column at right margin when PendingWrap was set)", scr.CurCol)
	}
}
