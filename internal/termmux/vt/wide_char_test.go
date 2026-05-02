package vt

import "testing"

// ── Wide character boundary repair tests ─────────────────────────────
// These tests verify that EraseChars, InsertChars, DeleteChars, EraseLine,
// EraseDisplay, and PutChar correctly handle wide character (width-2) pairs.
// A wide character occupies two cells: the first holds the rune, the second
// holds a NUL (Ch==0) placeholder.  Operations that split this pair must
// blank the orphaned half.

// placeWide writes a width-2 character at (row, col) and its NUL placeholder
// at col+1.  It does not use PutChar to avoid coupling tests to cursor logic.
func placeWide(s *Screen, row, col int, ch rune) {
	s.Cells[row][col] = Cell{Ch: ch}
	s.Cells[row][col+1] = Cell{Ch: 0, SecondHalf: true}
}

// rowChars returns the rune values for a row as a slice for easy assertion.
func rowChars(s *Screen, row int) []rune {
	out := make([]rune, s.Cols)
	for c := range s.Cols {
		out[c] = s.Cells[row][c].Ch
	}
	return out
}

// ── EraseChars (ECH) ─────────────────────────────────────────────────

func TestEraseChars_CursorOnPlaceholder(t *testing.T) {
	// [A][漢][0][B][C]  cursor at col 2 (placeholder)
	// Erasing 1 char should also blank col 1 (漢's first half).
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	placeWide(s, 0, 1, '漢')
	s.Cells[0][3].Ch = 'B'
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 2
	s.EraseChars(1)

	if s.Cells[0][1].Ch != ' ' {
		t.Errorf("col 1 = %c, want space (orphaned first half should be cleared)", s.Cells[0][1].Ch)
	}
	if s.Cells[0][2].Ch != ' ' {
		t.Errorf("col 2 = %c, want space (erased)", s.Cells[0][2].Ch)
	}
	if s.Cells[0][3].Ch != 'B' {
		t.Errorf("col 3 = %c, want B (untouched)", s.Cells[0][3].Ch)
	}
}

func TestEraseChars_EndSplitsWideChar(t *testing.T) {
	// [A][B][漢][0][C]  cursor at col 1, erase 2 chars → range [1,3)
	// col 3 is the placeholder of 漢 — should be blanked.
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	s.Cells[0][1].Ch = 'B'
	placeWide(s, 0, 2, '漢')
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 1
	s.EraseChars(2)

	if s.Cells[0][1].Ch != ' ' {
		t.Errorf("col 1 = %c, want space (erased)", s.Cells[0][1].Ch)
	}
	if s.Cells[0][2].Ch != ' ' {
		t.Errorf("col 2 = %c, want space (erased)", s.Cells[0][2].Ch)
	}
	if s.Cells[0][3].Ch != ' ' {
		t.Errorf("col 3 = %c, want space (orphaned placeholder should be cleared)", s.Cells[0][3].Ch)
	}
	if s.Cells[0][4].Ch != 'C' {
		t.Errorf("col 4 = %c, want C (untouched)", s.Cells[0][4].Ch)
	}
}

func TestEraseChars_WideFullyInRange(t *testing.T) {
	// [漢][0][A][B][C]  cursor at col 0, erase 2 → both cells of 漢 erased
	// No orphan because both halves are in range.
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.CurCol = 0
	s.EraseChars(2)

	if s.Cells[0][0].Ch != ' ' || s.Cells[0][1].Ch != ' ' {
		t.Errorf("cols 0-1 = %c/%c, want spaces", s.Cells[0][0].Ch, s.Cells[0][1].Ch)
	}
	if s.Cells[0][2].Ch != 'A' {
		t.Errorf("col 2 = %c, want A (untouched)", s.Cells[0][2].Ch)
	}
}

// ── InsertChars (ICH) ────────────────────────────────────────────────

func TestInsertChars_CursorOnPlaceholder(t *testing.T) {
	// [漢][0][A][B][C]  cursor at col 1 (placeholder), insert 1
	// Should blank col 0 (漢's first half) and the placeholder.
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.Cells[0][3].Ch = 'B'
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 1
	s.InsertChars(1)

	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half)", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != ' ' {
		t.Errorf("col 1 = %c, want space (inserted blank)", s.Cells[0][1].Ch)
	}
	// Original col 1 was placeholder (now blanked to space), shifted to col 2.
	if s.Cells[0][2].Ch != ' ' {
		t.Errorf("col 2 = %c, want space (shifted blank placeholder)", s.Cells[0][2].Ch)
	}
	if s.Cells[0][3].Ch != 'A' {
		t.Errorf("col 3 = %c, want A (shifted from col 2)", s.Cells[0][3].Ch)
	}
}

func TestInsertChars_DiscardSplitsWideChar(t *testing.T) {
	// [A][B][C][漢][0]  cursor at col 0, insert 2
	// Cols-n = 5-2 = 3. Cell at col 3 is 漢 (first half).
	// Cell at col 4 is placeholder, which falls off.
	// But actually, the "discarded" cells are [3..4].
	// discard = 3, row[3].Ch = 漢 (not 0), so no repair needed at discard.
	// However, after shift: cells [0..2] from source become [2..4].
	// Cell that was at col 3 (漢) → col 5 (out of bounds, lost).
	// Cell that was at col 4 (placeholder) → col 6 (out of bounds, lost).
	// Source range: [0..3-1] = [0..2] → destination [2..4].
	// So cols 2-4 = ABC, cols 0-1 = blank.
	// Result: [ ][ ][A][B][C] — no wide char issue.
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	s.Cells[0][1].Ch = 'B'
	s.Cells[0][2].Ch = 'C'
	placeWide(s, 0, 3, '漢')
	s.CurCol = 0
	s.InsertChars(2)

	want := []rune{' ', ' ', 'A', 'B', 'C'}
	got := rowChars(s, 0)
	for i, w := range want {
		if got[i] != w {
			t.Errorf("col %d = %c, want %c", i, got[i], w)
		}
	}
}

func TestInsertChars_DiscardSplitsAtPlaceholder(t *testing.T) {
	// [A][B][漢][0][C]  cursor at col 0, insert 1
	// Cols-n = 5-1 = 4. Cell at col 4 is 'C' (not 0).
	// Source: [0..3] → destination [1..4]. Blanks: [0].
	// After: [ ][A][B][漢][0] — wait, that keeps the wide char intact.
	// Now try: [A][漢][0][B][C]  cursor at col 0, insert 3
	// Cols-n = 5-3 = 2. Cell at col 2 is placeholder (Ch==0).
	// Should blank col 1 (漢's first half).
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	placeWide(s, 0, 1, '漢')
	s.Cells[0][3].Ch = 'B'
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 0
	s.InsertChars(3)

	// After blanking row[1] and shift: source [0..1] → [3..4].
	// row[1] was blanked (from 漢 to space), so destination col 4 = space.
	// Col 3 = 'A' (original col 0). Cols 0-2 = blank.
	want := []rune{' ', ' ', ' ', 'A', ' '}
	got := rowChars(s, 0)
	for i, w := range want {
		if got[i] != w {
			t.Errorf("col %d = %c, want %c", i, got[i], w)
		}
	}
}

// ── DeleteChars (DCH) ────────────────────────────────────────────────

func TestDeleteChars_CursorOnPlaceholder(t *testing.T) {
	// [漢][0][A][B][C]  cursor at col 1 (placeholder), delete 1
	// Should blank col 0 (orphaned first half of 漢).
	// Shift: [A][B][C] → cols [1..3], col 4 = blank.
	// But col 0 should also be blanked.
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.Cells[0][3].Ch = 'B'
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 1
	s.DeleteChars(1)

	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half)", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != 'A' {
		t.Errorf("col 1 = %c, want A (shifted left)", s.Cells[0][1].Ch)
	}
	if s.Cells[0][4].Ch != ' ' {
		t.Errorf("col 4 = %c, want space (vacated)", s.Cells[0][4].Ch)
	}
}

func TestDeleteChars_BoundarySplitsWideChar(t *testing.T) {
	// [A][B][漢][0][C]  cursor at col 1, delete 1
	// CurCol+n = 2. Cell at col 2 is 漢 (first cell of wide char).
	// The first surviving cell after delete is col 2 (CurCol+n).
	// Actually checking: row[CurCol+n] = row[2].Ch = '漢' (not 0).
	// So no repair at delete boundary. Shift: [漢][0][C] → [1..3], col 4 = blank.
	// Result: [A][漢][0][C][ ] — wide char intact. OK.
	//
	// Better test: [A][B][C][漢][0]  cursor at col 2, delete 1
	// CurCol+n = 3. Cell at col 3 is 漢 (not placeholder).
	// Shift: [漢][0] → [2..3], col 4 = blank.
	// Result: [A][B][漢][0][ ] — intact.
	//
	// Now the real case: [A][漢][0][B][C]  cursor at col 0, delete 1
	// CurCol+n = 1. Cell at col 1 is placeholder (Ch==0).
	// Should blank col 1 before shift.
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	placeWide(s, 0, 1, '漢')
	s.Cells[0][3].Ch = 'B'
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 0
	s.DeleteChars(1)

	// col 1 placeholder was blanked, then shift: [blank][blank][B][C] → [0..3], col 4 = blank.
	// Wait... let me trace: after blanking row[1] = blank:
	// Row: [A][ ][ ][B][C]
	// Wait, no. We only blank row[CurCol+n] = row[1]. Row becomes:
	// [A][ ][ ][B][C] — no, row[1] was 漢 and we blank the placeholder at row[1]?
	// Actually: row[1].Ch was '漢' (the rune). row[2].Ch was 0 (placeholder).
	// CurCol+n = 0+1 = 1. row[1].Ch = '漢' which is NOT 0. So no repair!
	// That means the delete simply removes 'A', shifts [漢][0][B][C] left:
	// Result: [漢][0][B][C][ ] — wide char intact. Correct!
	if s.Cells[0][0].Ch != '漢' || s.Cells[0][1].Ch != 0 {
		t.Errorf("cols 0-1 = %c/%d, want 漢/0 (shifted wide char)", s.Cells[0][0].Ch, s.Cells[0][1].Ch)
	}
	if s.Cells[0][2].Ch != 'B' || s.Cells[0][3].Ch != 'C' {
		t.Errorf("cols 2-3 = %c/%c, want BC", s.Cells[0][2].Ch, s.Cells[0][3].Ch)
	}
	if s.Cells[0][4].Ch != ' ' {
		t.Errorf("col 4 = %c, want space", s.Cells[0][4].Ch)
	}
}

func TestDeleteChars_FirstSurvivingIsPlaceholder(t *testing.T) {
	// [A][B][漢][0][C]  cursor at col 0, delete 3
	// CurCol+n = 3. Cell at col 3 is placeholder (Ch==0).
	// Should blank col 3 before shift.
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	s.Cells[0][1].Ch = 'B'
	placeWide(s, 0, 2, '漢')
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 0
	s.DeleteChars(3)

	// After blanking row[3]: row[3] = space (was placeholder)
	// Shift: [ ][C] → [0..1], cols [2..4] = blank
	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned placeholder was blanked then shifted)", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != 'C' {
		t.Errorf("col 1 = %c, want C (shifted)", s.Cells[0][1].Ch)
	}
	for c := 2; c < 5; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Errorf("col %d = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
}

// ── PutChar wide-char overlap repair ─────────────────────────────────

func TestPutChar_OverwritePlaceholderBlanksFirstHalf(t *testing.T) {
	// [漢][0][A][B][C]  place cursor at col 1 (placeholder) and write 'X'
	// Should blank col 0 (漢's first half) before writing 'X' at col 1.
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.CurCol = 1
	s.PutChar('X')

	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half)", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != 'X' {
		t.Errorf("col 1 = %c, want X (written)", s.Cells[0][1].Ch)
	}
}

func TestPutChar_WideCharOverlapsFollowingWideChar(t *testing.T) {
	// [A][漢][0][B][C]  place cursor at col 0 and write a wide char '字'
	// '字' occupies cols 0-1. Col 1 was 漢's first half → col 2 (placeholder) orphaned.
	// repairWideBoundary([0,2)) should blank col 2.
	// Wait: start=0, end=2. cells[2].Ch=0 → repairWideBoundary blanks col 2. ✓
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	placeWide(s, 0, 1, '漢')
	s.Cells[0][3].Ch = 'B'
	s.CurCol = 0
	s.PutChar('字')

	if s.Cells[0][0].Ch != '字' || s.Cells[0][1].Ch != 0 {
		t.Errorf("cols 0-1 = %c/%d, want 字/0", s.Cells[0][0].Ch, s.Cells[0][1].Ch)
	}
	if s.Cells[0][2].Ch != ' ' {
		t.Errorf("col 2 = %c, want space (orphaned placeholder cleared)", s.Cells[0][2].Ch)
	}
	if s.Cells[0][3].Ch != 'B' {
		t.Errorf("col 3 = %c, want B (untouched)", s.Cells[0][3].Ch)
	}
}

func TestPutChar_NarrowOnWideFirstCell(t *testing.T) {
	// [漢][0][A]  place cursor at col 0 and write 'X'
	// Writing width-1 'X' at col 0 should also blank col 1 (orphaned placeholder).
	// repairWideBoundary([0,1)): end=1, cells[1].Ch=0 → blank col 1.
	s := NewScreen(1, 3)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.CurCol = 0
	s.PutChar('X')

	if s.Cells[0][0].Ch != 'X' {
		t.Errorf("col 0 = %c, want X", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != ' ' {
		t.Errorf("col 1 = %d, want space (orphaned placeholder)", s.Cells[0][1].Ch)
	}
}

// ── EraseLine wide-char repair ───────────────────────────────────────

func TestEraseLine_Mode0_CursorOnPlaceholder(t *testing.T) {
	// [漢][0][A][B][C]  cursor at col 1, EraseLine mode 0 (cursor to end)
	// Should blank col 0 (orphaned first half).
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.Cells[0][3].Ch = 'B'
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 1
	s.EraseLine(0)

	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half)", s.Cells[0][0].Ch)
	}
	for c := 1; c < 5; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Errorf("col %d = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
}

func TestEraseLine_Mode1_EndSplitsWideChar(t *testing.T) {
	// [A][B][漢][0][C]  cursor at col 2 (漢's first cell), EraseLine mode 1 (start to cursor)
	// Erases [0..2]. Col 3 is placeholder → should be blanked.
	s := NewScreen(1, 5)
	s.Cells[0][0].Ch = 'A'
	s.Cells[0][1].Ch = 'B'
	placeWide(s, 0, 2, '漢')
	s.Cells[0][4].Ch = 'C'
	s.CurCol = 2
	s.EraseLine(1)

	for c := 0; c <= 3; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Errorf("col %d = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
	if s.Cells[0][4].Ch != 'C' {
		t.Errorf("col 4 = %c, want C (untouched)", s.Cells[0][4].Ch)
	}
}

// ── EraseDisplay wide-char repair ────────────────────────────────────

func TestEraseDisplay_Mode0_CursorOnPlaceholder(t *testing.T) {
	// Row 0: [漢][0][A][B][C]  cursor at (0,1), EraseDisplay mode 0
	s := NewScreen(2, 5)
	placeWide(s, 0, 0, '漢')
	s.Cells[0][2].Ch = 'A'
	s.CurRow = 0
	s.CurCol = 1
	s.EraseDisplay(0)

	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half)", s.Cells[0][0].Ch)
	}
}

func TestEraseDisplay_Mode1_EndSplitsWideChar(t *testing.T) {
	// Row 0: [A][漢][0][B][C]  cursor at (0,1), EraseDisplay mode 1
	// Erases [0..1]. Col 2 is placeholder → should be blanked.
	s := NewScreen(2, 5)
	s.Cells[0][0].Ch = 'A'
	placeWide(s, 0, 1, '漢')
	s.Cells[0][3].Ch = 'B'
	s.CurRow = 0
	s.CurCol = 1
	s.EraseDisplay(1)

	if s.Cells[0][2].Ch != ' ' {
		t.Errorf("col 2 = %c, want space (orphaned placeholder)", s.Cells[0][2].Ch)
	}
	if s.Cells[0][3].Ch != 'B' {
		t.Errorf("col 3 = %c, want B (untouched)", s.Cells[0][3].Ch)
	}
}

// ── No-op / edge cases ───────────────────────────────────────────────

func TestEraseChars_NoWideChars_Unchanged(t *testing.T) {
	// Pure ASCII — no wide char repair needed.
	s := NewScreen(1, 5)
	for c := range 5 {
		s.Cells[0][c].Ch = rune('A' + c)
	}
	s.CurCol = 1
	s.EraseChars(2)

	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("col 0 = %c, want A", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != ' ' || s.Cells[0][2].Ch != ' ' {
		t.Error("cols 1-2 should be blank")
	}
	if s.Cells[0][3].Ch != 'D' || s.Cells[0][4].Ch != 'E' {
		t.Error("cols 3-4 should be untouched")
	}
}

func TestDeleteChars_NoWideChars_Unchanged(t *testing.T) {
	s := NewScreen(1, 5)
	for c := range 5 {
		s.Cells[0][c].Ch = rune('A' + c)
	}
	s.CurCol = 1
	s.DeleteChars(2)

	want := []rune{'A', 'D', 'E', ' ', ' '}
	got := rowChars(s, 0)
	for i, w := range want {
		if got[i] != w {
			t.Errorf("col %d = %c, want %c", i, got[i], w)
		}
	}
}

func TestInsertChars_NoWideChars_Unchanged(t *testing.T) {
	s := NewScreen(1, 5)
	for c := range 5 {
		s.Cells[0][c].Ch = rune('A' + c)
	}
	s.CurCol = 1
	s.InsertChars(2)

	want := []rune{'A', ' ', ' ', 'B', 'C'}
	got := rowChars(s, 0)
	for i, w := range want {
		if got[i] != w {
			t.Errorf("col %d = %c, want %c", i, got[i], w)
		}
	}
}

// ── Consecutive wide chars ───────────────────────────────────────────

func TestDeleteChars_ConsecutiveWideChars(t *testing.T) {
	// [漢][0][字][0][A]  cursor at col 0, delete 2
	// Deletes cols 0-1 (漢 pair). CurCol+n=2, cell at 2 is '字' (not 0) → no repair.
	// Shift: [字][0][A] → [0..2], cols 3-4 = blank.
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	placeWide(s, 0, 2, '字')
	s.Cells[0][4].Ch = 'A'
	s.CurCol = 0
	s.DeleteChars(2)

	if s.Cells[0][0].Ch != '字' || s.Cells[0][1].Ch != 0 {
		t.Errorf("cols 0-1 = %c/%d, want 字/0", s.Cells[0][0].Ch, s.Cells[0][1].Ch)
	}
	if s.Cells[0][2].Ch != 'A' {
		t.Errorf("col 2 = %c, want A", s.Cells[0][2].Ch)
	}
}

func TestEraseChars_ConsecutiveWideChars(t *testing.T) {
	// [漢][0][字][0][A]  cursor at col 1 (placeholder), erase 2
	// Range [1,3). Start=1 is placeholder → blank col 0.
	// End=3 is placeholder → blank col 3.
	s := NewScreen(1, 5)
	placeWide(s, 0, 0, '漢')
	placeWide(s, 0, 2, '字')
	s.Cells[0][4].Ch = 'A'
	s.CurCol = 1
	s.EraseChars(2)

	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half of 漢)", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != ' ' || s.Cells[0][2].Ch != ' ' {
		t.Error("cols 1-2 should be blank (erased)")
	}
	if s.Cells[0][3].Ch != ' ' {
		t.Errorf("col 3 = %c, want space (orphaned placeholder of 字)", s.Cells[0][3].Ch)
	}
	if s.Cells[0][4].Ch != 'A' {
		t.Errorf("col 4 = %c, want A (untouched)", s.Cells[0][4].Ch)
	}
}

// ── Integration: CSI dispatch with wide chars (via VTerm) ────────────

func TestVTerm_DCH_SplitsWideChar(t *testing.T) {
	vt := NewVTerm(1, 10)
	// Write: A 漢 B C D E F G
	// A at col 0, 漢 at cols 1-2, B at 3, C at 4, etc.
	vt.Write([]byte("A\xe6\xbc\xa2BCDEFG")) // 漢 = U+6F22 = 3 bytes UTF-8
	scr := vt.active
	if scr.Cells[0][1].Ch != '漢' || scr.Cells[0][2].Ch != 0 {
		t.Fatalf("wide char not placed correctly: %c/%d", scr.Cells[0][1].Ch, scr.Cells[0][2].Ch)
	}
	// Move cursor to col 1 and delete 1 char (DCH): \e[1G\e[P positions and deletes
	// CUP to col 1 (1-indexed = 2): "\x1b[2G"
	// DCH 1: "\x1b[P"
	vt.Write([]byte("\x1b[2G\x1b[P"))
	// After delete: 漢 at col 1 is deleted. CurCol+n=2, cell 2 was placeholder(0) → blanked.
	// Shift: [blank][B][C][D][E][F][G] → [1..7], cols 8-9 = blank.
	// Actually, we delete from col 1. Row was: [A][漢][0][B][C][D][E][F][G][ ]
	// CurCol=1, n=1, CurCol+n=2, row[2].Ch=0 → blank row[2].
	// After repair: [A][漢][ ][B][C][D][E][F][G][ ]
	// Copy row[1:] ← row[2:] → [A][ ][B][C][D][E][F][G][ ][ ]
	// Wait: copy(row[1:], row[2:]) copies cells 2..9 to positions 1..8:
	// Result: [A][ ][B][C][D][E][F][G][ ][ ]
	// Row[9] = blank (from fill).
	if scr.Cells[0][1].Ch != ' ' {
		t.Errorf("col 1 = %c, want space (was placeholder, repaired then shifted)", scr.Cells[0][1].Ch)
	}
	if scr.Cells[0][2].Ch != 'B' {
		t.Errorf("col 2 = %c, want B", scr.Cells[0][2].Ch)
	}
}

func TestVTerm_ECH_OnPlaceholder(t *testing.T) {
	vt := NewVTerm(1, 10)
	vt.Write([]byte("\xe6\xbc\xa2ABCDEFGH")) // 漢ABCDEFGH
	scr := vt.active
	// 漢 at cols 0-1, A at 2, etc.
	if scr.Cells[0][0].Ch != '漢' || scr.Cells[0][1].Ch != 0 {
		t.Fatalf("wide char not placed: %c/%d", scr.Cells[0][0].Ch, scr.Cells[0][1].Ch)
	}
	// Move to col 1 (1-indexed=2) and erase 1 char: \e[2G\e[X
	vt.Write([]byte("\x1b[2G\x1b[X"))
	if scr.Cells[0][0].Ch != ' ' {
		t.Errorf("col 0 = %c, want space (orphaned first half)", scr.Cells[0][0].Ch)
	}
	if scr.Cells[0][1].Ch != ' ' {
		t.Errorf("col 1 = %c, want space (erased)", scr.Cells[0][1].Ch)
	}
}
