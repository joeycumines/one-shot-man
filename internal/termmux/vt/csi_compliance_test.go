package vt

import (
	"testing"
)

// csiComplianceHelper creates a VTerm with a ResponseWriter that captures
// response bytes. Returns the VTerm and a function to retrieve the response.
func csiComplianceHelper() (*VTerm, func() []byte) {
	var resp []byte
	v := NewVTerm(24, 80)
	v.ResponseWriter = func(data []byte) { resp = append(resp, data...) }
	return v, func() []byte { r := resp; resp = nil; return r }
}

// ─── CUU (A) — Cursor Up ────────────────────────────────────────────────

func TestCompliance_CUU_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	h := NewCSIHandler()
	h.Dispatch(scr, 'A', []int{3}, false)
	if scr.CurRow != 7 {
		t.Fatalf("CUU 3: want row 7, got %d", scr.CurRow)
	}
}

func TestCompliance_CUU_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	h := NewCSIHandler()
	h.Dispatch(scr, 'A', nil, false)
	if scr.CurRow != 4 {
		t.Fatalf("CUU default: want row 4, got %d", scr.CurRow)
	}
}

func TestCompliance_CUU_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'A', []int{9999}, false)
	if scr.CurRow != 0 {
		t.Fatalf("CUU overflow: want row 0, got %d", scr.CurRow)
	}
}

func TestCompliance_CUU_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.PendingWrap = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'A', []int{1}, false)
	if scr.PendingWrap {
		t.Fatal("CUU should clear PendingWrap")
	}
}

// ─── CUD (B) — Cursor Down ──────────────────────────────────────────────

func TestCompliance_CUD_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	h := NewCSIHandler()
	h.Dispatch(scr, 'B', []int{3}, false)
	if scr.CurRow != 8 {
		t.Fatalf("CUD 3: want row 8, got %d", scr.CurRow)
	}
}

func TestCompliance_CUD_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	h := NewCSIHandler()
	h.Dispatch(scr, 'B', nil, false)
	if scr.CurRow != 6 {
		t.Fatalf("CUD default: want row 6, got %d", scr.CurRow)
	}
}

func TestCompliance_CUD_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 22
	h := NewCSIHandler()
	h.Dispatch(scr, 'B', []int{9999}, false)
	if scr.CurRow != 23 {
		t.Fatalf("CUD overflow: want row 23, got %d", scr.CurRow)
	}
}

func TestCompliance_CUD_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.PendingWrap = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'B', []int{1}, false)
	if scr.PendingWrap {
		t.Fatal("CUD should clear PendingWrap")
	}
}

// ─── CUF (C) — Cursor Forward ───────────────────────────────────────────

func TestCompliance_CUF_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'C', []int{10}, false)
	if scr.CurCol != 10 {
		t.Fatalf("CUF 10: want col 10, got %d", scr.CurCol)
	}
}

func TestCompliance_CUF_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'C', nil, false)
	if scr.CurCol != 1 {
		t.Fatalf("CUF default: want col 1, got %d", scr.CurCol)
	}
}

func TestCompliance_CUF_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'C', []int{9999}, false)
	if scr.CurCol != 79 {
		t.Fatalf("CUF overflow: want col 79, got %d", scr.CurCol)
	}
}

// ─── CUB (D) — Cursor Backward ──────────────────────────────────────────

func TestCompliance_CUB_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 10
	h := NewCSIHandler()
	h.Dispatch(scr, 'D', []int{3}, false)
	if scr.CurCol != 7 {
		t.Fatalf("CUB 3: want col 7, got %d", scr.CurCol)
	}
}

func TestCompliance_CUB_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 5
	h := NewCSIHandler()
	h.Dispatch(scr, 'D', nil, false)
	if scr.CurCol != 4 {
		t.Fatalf("CUB default: want col 4, got %d", scr.CurCol)
	}
}

func TestCompliance_CUB_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'D', []int{9999}, false)
	if scr.CurCol != 0 {
		t.Fatalf("CUB overflow: want col 0, got %d", scr.CurCol)
	}
}

// ─── CNL (E) — Cursor Next Line ─────────────────────────────────────────

func TestCompliance_CNL_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'E', []int{3}, false)
	if scr.CurRow != 8 {
		t.Fatalf("CNL 3: want row 8, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CNL 3: want col 0, got %d", scr.CurCol)
	}
}

func TestCompliance_CNL_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'E', nil, false)
	if scr.CurRow != 6 {
		t.Fatalf("CNL default: want row 6, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CNL default: want col 0, got %d", scr.CurCol)
	}
}

func TestCompliance_CNL_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 22
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'E', []int{9999}, false)
	if scr.CurRow != 23 {
		t.Fatalf("CNL overflow: want row 23, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CNL overflow: want col 0, got %d", scr.CurCol)
	}
}

// ─── CPL (F) — Cursor Previous Line ─────────────────────────────────────

func TestCompliance_CPL_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'F', []int{3}, false)
	if scr.CurRow != 7 {
		t.Fatalf("CPL 3: want row 7, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CPL 3: want col 0, got %d", scr.CurCol)
	}
}

func TestCompliance_CPL_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'F', nil, false)
	if scr.CurRow != 9 {
		t.Fatalf("CPL default: want row 9, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CPL default: want col 0, got %d", scr.CurCol)
	}
}

func TestCompliance_CPL_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 2
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'F', []int{9999}, false)
	if scr.CurRow != 0 {
		t.Fatalf("CPL overflow: want row 0, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CPL overflow: want col 0, got %d", scr.CurCol)
	}
}

// ─── CHA (G) — Cursor Horizontal Absolute ────────────────────────────────

func TestCompliance_CHA_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 50
	h := NewCSIHandler()
	h.Dispatch(scr, 'G', []int{20}, false)
	if scr.CurCol != 19 {
		t.Fatalf("CHA 20: want col 19, got %d", scr.CurCol)
	}
}

func TestCompliance_CHA_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 50
	h := NewCSIHandler()
	h.Dispatch(scr, 'G', nil, false)
	if scr.CurCol != 0 {
		t.Fatalf("CHA default: want col 0, got %d", scr.CurCol)
	}
}

func TestCompliance_CHA_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'G', []int{9999}, false)
	if scr.CurCol != 79 {
		t.Fatalf("CHA overflow: want col 79, got %d", scr.CurCol)
	}
}

func TestCompliance_CHA_ZeroParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 50
	h := NewCSIHandler()
	h.Dispatch(scr, 'G', []int{0}, false)
	if scr.CurCol != 0 {
		t.Fatalf("CHA 0: want col 0 (default), got %d", scr.CurCol)
	}
}

// ─── CUP (H/f) — Cursor Position ────────────────────────────────────────

func TestCompliance_CUP_H_FinalByte(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'H', []int{5, 10}, false)
	if scr.CurRow != 4 || scr.CurCol != 9 {
		t.Fatalf("CUP H: want (4,9), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCompliance_CUP_f_FinalByte(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'f', []int{5, 10}, false)
	if scr.CurRow != 4 || scr.CurCol != 9 {
		t.Fatalf("CUP f: want (4,9), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCompliance_CUP_DefaultParams(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 20
	scr.CurCol = 60
	h := NewCSIHandler()
	h.Dispatch(scr, 'H', nil, false)
	if scr.CurRow != 0 || scr.CurCol != 0 {
		t.Fatalf("CUP default: want (0,0), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCompliance_CUP_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'H', []int{9999, 9999}, false)
	if scr.CurRow != 23 || scr.CurCol != 79 {
		t.Fatalf("CUP overflow: want (23,79), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCompliance_CUP_ZeroParams(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 20
	scr.CurCol = 60
	h := NewCSIHandler()
	h.Dispatch(scr, 'H', []int{0, 0}, false)
	if scr.CurRow != 0 || scr.CurCol != 0 {
		t.Fatalf("CUP zero: want (0,0), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCompliance_CUP_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.PendingWrap = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'H', []int{1, 1}, false)
	if scr.PendingWrap {
		t.Fatal("CUP should clear PendingWrap")
	}
}

// ─── ED (J) — Erase Display ─────────────────────────────────────────────

func TestCompliance_ED_Mode0_CursorToEnd(t *testing.T) {
	t.Parallel()
	scr := NewScreen(4, 4)
	for r := range 4 {
		for c := range 4 {
			scr.Cells[r][c].Ch = 'X'
		}
	}
	scr.CurRow = 1
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'J', []int{0}, false)
	// Row 0 untouched, row 1 cols 0-1 untouched
	if scr.Cells[0][0].Ch != 'X' {
		t.Fatal("ED 0: row 0 should be untouched")
	}
	if scr.Cells[1][0].Ch != 'X' {
		t.Fatal("ED 0: row 1 col 0 should be untouched")
	}
	if scr.Cells[1][2].Ch != ' ' {
		t.Fatal("ED 0: row 1 col 2 should be erased")
	}
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatal("ED 0: row 2 should be erased")
	}
}

func TestCompliance_ED_Mode1_StartToCursor(t *testing.T) {
	t.Parallel()
	scr := NewScreen(4, 4)
	for r := range 4 {
		for c := range 4 {
			scr.Cells[r][c].Ch = 'X'
		}
	}
	scr.CurRow = 2
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'J', []int{1}, false)
	// Rows 0-1 fully erased, row 2 cols 0-1 erased, row 3 untouched
	if scr.Cells[0][0].Ch != ' ' {
		t.Fatal("ED 1: row 0 should be erased")
	}
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatal("ED 1: row 2 col 0 should be erased")
	}
	if scr.Cells[2][2].Ch != 'X' {
		t.Fatal("ED 1: row 2 col 2 should be untouched")
	}
	if scr.Cells[3][0].Ch != 'X' {
		t.Fatal("ED 1: row 3 should be untouched")
	}
}

func TestCompliance_ED_Mode2_EntireDisplay(t *testing.T) {
	t.Parallel()
	scr := NewScreen(4, 4)
	for r := range 4 {
		for c := range 4 {
			scr.Cells[r][c].Ch = 'X'
		}
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'J', []int{2}, false)
	for r := range 4 {
		for c := range 4 {
			if scr.Cells[r][c].Ch != ' ' {
				t.Fatalf("ED 2: cell[%d][%d] should be blank", r, c)
			}
		}
	}
}

func TestCompliance_ED_Mode3_EraseScrollback(t *testing.T) {
	t.Parallel()
	scr := NewScreen(3, 5)
	// Fill screen and force scrollback
	for r := range 3 {
		for c := range 5 {
			scr.Cells[r][c].Ch = 'A' + rune(r)
		}
	}
	scr.ScrollUp(1)
	if scr.ScrollbackLen == 0 {
		t.Fatal("precondition: expected scrollback")
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'J', []int{3}, false)
	if scr.ScrollbackLen != 0 {
		t.Fatalf("ED 3: scrollback should be cleared, got %d lines", scr.ScrollbackLen)
	}
	// Screen content should NOT be erased
	if scr.Cells[0][0].Ch == ' ' {
		t.Fatal("ED 3: screen content should be preserved")
	}
}

func TestCompliance_ED_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(4, 4)
	for r := range 4 {
		for c := range 4 {
			scr.Cells[r][c].Ch = 'X'
		}
	}
	scr.CurRow = 2
	scr.CurCol = 0
	h := NewCSIHandler()
	h.Dispatch(scr, 'J', nil, false)
	// Default is mode 0 (cursor to end)
	if scr.Cells[0][0].Ch != 'X' {
		t.Fatal("ED default: row 0 should be untouched")
	}
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatal("ED default: row 2 should be erased")
	}
}

// ─── EL (K) — Erase Line ────────────────────────────────────────────────

func TestCompliance_EL_Mode0_CursorToEnd(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurRow = 0
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'K', []int{0}, false)
	got := rowString(scr, 0)
	if got != "AB   " {
		t.Fatalf("EL 0: want 'AB   ', got %q", got)
	}
}

func TestCompliance_EL_Mode1_StartToCursor(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurRow = 0
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'K', []int{1}, false)
	got := rowString(scr, 0)
	if got != "   DE" {
		t.Fatalf("EL 1: want '   DE', got %q", got)
	}
}

func TestCompliance_EL_Mode2_EntireLine(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurRow = 0
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'K', []int{2}, false)
	got := rowString(scr, 0)
	if got != "     " {
		t.Fatalf("EL 2: want '     ', got %q", got)
	}
}

func TestCompliance_EL_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurRow = 0
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'K', nil, false)
	got := rowString(scr, 0)
	if got != "AB   " {
		t.Fatalf("EL default: want 'AB   ', got %q", got)
	}
}

// ─── IL (L) — Insert Lines ──────────────────────────────────────────────

func TestCompliance_IL_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	scr.CurRow = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'L', []int{2}, false)
	if scr.Cells[1][0].Ch != ' ' {
		t.Fatalf("IL: row 1 should be blank, got %q", scr.Cells[1][0].Ch)
	}
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatalf("IL: row 2 should be blank, got %q", scr.Cells[2][0].Ch)
	}
	if scr.Cells[3][0].Ch != 'B' {
		t.Fatalf("IL: row 3 should have 'B', got %q", scr.Cells[3][0].Ch)
	}
}

func TestCompliance_IL_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	scr.CurRow = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'L', nil, false)
	if scr.Cells[1][0].Ch != ' ' {
		t.Fatalf("IL default: row 1 should be blank, got %q", scr.Cells[1][0].Ch)
	}
	if scr.Cells[2][0].Ch != 'B' {
		t.Fatalf("IL default: row 2 should have 'B', got %q", scr.Cells[2][0].Ch)
	}
}

func TestCompliance_IL_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	scr.CurRow = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'L', []int{9999}, false)
	// Should clamp to available lines from cursor to bottom
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatalf("IL overflow: row 2 should be blank, got %q", scr.Cells[2][0].Ch)
	}
}

func TestCompliance_IL_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	scr.PendingWrap = true
	scr.CurRow = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'L', []int{1}, false)
	if scr.PendingWrap {
		t.Fatal("IL should clear PendingWrap")
	}
}

// ─── DL (M) — Delete Lines ──────────────────────────────────────────────

func TestCompliance_DL_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	scr.CurRow = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'M', []int{2}, false)
	if scr.Cells[1][0].Ch != 'D' {
		t.Fatalf("DL: row 1 should have 'D', got %q", scr.Cells[1][0].Ch)
	}
	if scr.Cells[2][0].Ch != 'E' {
		t.Fatalf("DL: row 2 should have 'E', got %q", scr.Cells[2][0].Ch)
	}
}

func TestCompliance_DL_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	scr.CurRow = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'M', nil, false)
	if scr.Cells[1][0].Ch != 'C' {
		t.Fatalf("DL default: row 1 should have 'C', got %q", scr.Cells[1][0].Ch)
	}
}

func TestCompliance_DL_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	scr.CurRow = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'M', []int{9999}, false)
	// Should clamp to available lines from cursor to bottom
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatalf("DL overflow: row 2 should be blank, got %q", scr.Cells[2][0].Ch)
	}
}

func TestCompliance_DL_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	scr.PendingWrap = true
	scr.CurRow = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'M', []int{1}, false)
	if scr.PendingWrap {
		t.Fatal("DL should clear PendingWrap")
	}
}

// ─── DCH (P) — Delete Characters ─────────────────────────────────────────

func TestCompliance_DCH_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'P', []int{2}, false)
	got := rowString(scr, 0)
	if got != "ADE  " {
		t.Fatalf("DCH 2: want 'ADE  ', got %q", got)
	}
}

func TestCompliance_DCH_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'P', nil, false)
	got := rowString(scr, 0)
	if got != "ACDE " {
		t.Fatalf("DCH default: want 'ACDE ', got %q", got)
	}
}

func TestCompliance_DCH_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'P', []int{9999}, false)
	got := rowString(scr, 0)
	if got != "AB   " {
		t.Fatalf("DCH overflow: want 'AB   ', got %q", got)
	}
}

// ─── ECH (X) — Erase Characters ─────────────────────────────────────────

func TestCompliance_ECH_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'X', []int{2}, false)
	got := rowString(scr, 0)
	if got != "A  DE" {
		t.Fatalf("ECH 2: want 'A  DE', got %q", got)
	}
}

func TestCompliance_ECH_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, 'X', nil, false)
	got := rowString(scr, 0)
	// Default param is 1: erase 1 char at cursor position
	if got != "A CDE" {
		t.Fatalf("ECH default: want 'A CDE', got %q", got)
	}
}

func TestCompliance_ECH_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, 'X', []int{9999}, false)
	got := rowString(scr, 0)
	if got != "AB   " {
		t.Fatalf("ECH overflow: want 'AB   ', got %q", got)
	}
}

// ─── ICH (@) — Insert Characters ─────────────────────────────────────────

func TestCompliance_ICH_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, '@', []int{2}, false)
	got := rowString(scr, 0)
	if got != "A  BC" {
		t.Fatalf("ICH 2: want 'A  BC', got %q", got)
	}
}

func TestCompliance_ICH_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := NewCSIHandler()
	h.Dispatch(scr, '@', nil, false)
	got := rowString(scr, 0)
	// Default param is 1: insert 1 blank at cursor, 'E' pushed off
	if got != "A BCD" {
		t.Fatalf("ICH default: want 'A BCD', got %q", got)
	}
}

func TestCompliance_ICH_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2
	h := NewCSIHandler()
	h.Dispatch(scr, '@', []int{9999}, false)
	got := rowString(scr, 0)
	if got != "AB   " {
		t.Fatalf("ICH overflow: want 'AB   ', got %q", got)
	}
}

// ─── SU (S) — Scroll Up ─────────────────────────────────────────────────

func TestCompliance_SU_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'S', []int{2}, false)
	if scr.Cells[0][0].Ch != 'C' {
		t.Fatalf("SU 2: row 0 should have 'C', got %q", scr.Cells[0][0].Ch)
	}
	if scr.Cells[1][0].Ch != 'D' {
		t.Fatalf("SU 2: row 1 should have 'D', got %q", scr.Cells[1][0].Ch)
	}
	if scr.Cells[2][0].Ch != 'E' {
		t.Fatalf("SU 2: row 2 should have 'E', got %q", scr.Cells[2][0].Ch)
	}
	if scr.Cells[3][0].Ch != ' ' {
		t.Fatalf("SU 2: row 3 should be blank, got %q", scr.Cells[3][0].Ch)
	}
}

func TestCompliance_SU_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'S', nil, false)
	if scr.Cells[0][0].Ch != 'B' {
		t.Fatalf("SU default: row 0 should have 'B', got %q", scr.Cells[0][0].Ch)
	}
}

func TestCompliance_SU_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'S', []int{9999}, false)
	for r := range 5 {
		if scr.Cells[r][0].Ch != ' ' {
			t.Fatalf("SU overflow: row %d should be blank, got %q", r, scr.Cells[r][0].Ch)
		}
	}
}

// ─── SD (T) — Scroll Down ───────────────────────────────────────────────

func TestCompliance_SD_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'T', []int{2}, false)
	if scr.Cells[0][0].Ch != ' ' {
		t.Fatalf("SD 2: row 0 should be blank, got %q", scr.Cells[0][0].Ch)
	}
	if scr.Cells[1][0].Ch != ' ' {
		t.Fatalf("SD 2: row 1 should be blank, got %q", scr.Cells[1][0].Ch)
	}
	if scr.Cells[2][0].Ch != 'A' {
		t.Fatalf("SD 2: row 2 should have 'A', got %q", scr.Cells[2][0].Ch)
	}
}

func TestCompliance_SD_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'T', nil, false)
	if scr.Cells[0][0].Ch != ' ' {
		t.Fatalf("SD default: row 0 should be blank, got %q", scr.Cells[0][0].Ch)
	}
	if scr.Cells[1][0].Ch != 'A' {
		t.Fatalf("SD default: row 1 should have 'A', got %q", scr.Cells[1][0].Ch)
	}
}

func TestCompliance_SD_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(5, 5)
	for r := range 5 {
		scr.Cells[r][0].Ch = 'A' + rune(r)
	}
	h := NewCSIHandler()
	h.Dispatch(scr, 'T', []int{9999}, false)
	for r := range 5 {
		if scr.Cells[r][0].Ch != ' ' {
			t.Fatalf("SD overflow: row %d should be blank, got %q", r, scr.Cells[r][0].Ch)
		}
	}
}

// ─── VPA (d) — Vertical Position Absolute ────────────────────────────────

func TestCompliance_VPA_ValidParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'd', []int{12}, false)
	if scr.CurRow != 11 {
		t.Fatalf("VPA 12: want row 11, got %d", scr.CurRow)
	}
}

func TestCompliance_VPA_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 20
	h := NewCSIHandler()
	h.Dispatch(scr, 'd', nil, false)
	if scr.CurRow != 0 {
		t.Fatalf("VPA default: want row 0, got %d", scr.CurRow)
	}
}

func TestCompliance_VPA_OutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'd', []int{9999}, false)
	if scr.CurRow != 23 {
		t.Fatalf("VPA overflow: want row 23, got %d", scr.CurRow)
	}
}

func TestCompliance_VPA_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.PendingWrap = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'd', []int{1}, false)
	if scr.PendingWrap {
		t.Fatal("VPA should clear PendingWrap")
	}
}

// ─── CHT (I) — Cursor Horizontal Tab ────────────────────────────────────

func TestCompliance_CHT_ValidParam(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[3I"))
	_, col := v.CursorPosition()
	if col != 24 {
		t.Fatalf("CHT 3: want col 24, got %d", col)
	}
}

func TestCompliance_CHT_DefaultParam(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[I"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("CHT default: want col 8, got %d", col)
	}
}

func TestCompliance_CHT_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[9999I"))
	_, col := v.CursorPosition()
	if col != 79 {
		t.Fatalf("CHT overflow: want col 79, got %d", col)
	}
}

// ─── CBT (Z) — Cursor Backward Tab ──────────────────────────────────────

func TestCompliance_CBT_ValidParam(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1;25H"))
	v.Write([]byte("\x1b[2Z"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("CBT 2: want col 8, got %d", col)
	}
}

func TestCompliance_CBT_DefaultParam(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1;17H"))
	v.Write([]byte("\x1b[Z"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("CBT default: want col 8, got %d", col)
	}
}

func TestCompliance_CBT_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1;17H"))
	v.Write([]byte("\x1b[9999Z"))
	_, col := v.CursorPosition()
	if col != 0 {
		t.Fatalf("CBT overflow: want col 0, got %d", col)
	}
}

// ─── TBC (g) — Tab Clear ────────────────────────────────────────────────

func TestCompliance_TBC_Mode0_ClearCurrentTab(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 8
	h := NewCSIHandler()
	h.Dispatch(scr, 'g', []int{0}, false)
	if scr.TabStops[8] {
		t.Fatal("TBC 0: tab at col 8 should be cleared")
	}
	// Other tabs should remain
	if !scr.TabStops[0] {
		t.Fatal("TBC 0: tab at col 0 should remain")
	}
}

func TestCompliance_TBC_Mode3_ClearAllTabs(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'g', []int{3}, false)
	for i, ts := range scr.TabStops {
		if ts {
			t.Fatalf("TBC 3: tab at col %d should be cleared", i)
		}
	}
}

func TestCompliance_TBC_DefaultParam(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurCol = 16
	h := NewCSIHandler()
	h.Dispatch(scr, 'g', nil, false)
	if scr.TabStops[16] {
		t.Fatal("TBC default: tab at col 16 should be cleared")
	}
}

// ─── SGR (m) — Set Graphic Rendition ────────────────────────────────────

func TestCompliance_SGR_NoParams_Resets(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurAttr.Bold = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'm', nil, false)
	if scr.CurAttr.Bold {
		t.Fatal("SGR no params: expected reset")
	}
}

func TestCompliance_SGR_Reset(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurAttr.Bold = true
	scr.CurAttr.Italic = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'm', []int{0}, false)
	if scr.CurAttr.Bold || scr.CurAttr.Italic {
		t.Fatal("SGR 0: expected full reset")
	}
}

func TestCompliance_SGR_Bold(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'm', []int{1}, false)
	if !scr.CurAttr.Bold {
		t.Fatal("SGR 1: expected Bold=true")
	}
}

// ─── DECSTBM (r) — Set Scrolling Region ─────────────────────────────────

func TestCompliance_DECSTBM_ValidParams(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	h := NewCSIHandler()
	h.Dispatch(scr, 'r', []int{5, 20}, false)
	if scr.ScrollTop != 5 || scr.ScrollBot != 20 {
		t.Fatalf("DECSTBM: want (5,20), got (%d,%d)", scr.ScrollTop, scr.ScrollBot)
	}
}

func TestCompliance_DECSTBM_DefaultParams(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.ScrollTop = 5
	scr.ScrollBot = 20
	scr.CurRow = 10
	h := NewCSIHandler()
	h.Dispatch(scr, 'r', nil, false)
	if scr.ScrollTop != 1 || scr.ScrollBot != 24 {
		t.Fatalf("DECSTBM default: want (1,24), got (%d,%d)", scr.ScrollTop, scr.ScrollBot)
	}
}

func TestCompliance_DECSTBM_InvertedRegion(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'r', []int{20, 5}, false)
	top, bot := scr.ScrollRegion()
	if top != 0 || bot != 24 {
		t.Fatalf("DECSTBM inverted: want defaults (0,24), got (%d,%d)", top, bot)
	}
}

func TestCompliance_DECSTBM_HomesCursor(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	scr.CurCol = 30
	h := NewCSIHandler()
	h.Dispatch(scr, 'r', []int{5, 20}, false)
	if scr.CurRow != 0 || scr.CurCol != 0 {
		t.Fatalf("DECSTBM home: want (0,0), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCompliance_DECSTBM_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.PendingWrap = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'r', []int{5, 20}, false)
	if scr.PendingWrap {
		t.Fatal("DECSTBM should clear PendingWrap")
	}
}

// ─── SM/DECSET (h) — Set Mode ───────────────────────────────────────────

func TestCompliance_SM_InsertMode(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{4}, false)
	if !scr.InsertMode {
		t.Fatal("SM 4h: expected InsertMode=true")
	}
}

func TestCompliance_SM_LineFeedNewLine(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{20}, false)
	if !scr.LineFeedNewLine {
		t.Fatal("SM 20h: expected LineFeedNewLine=true")
	}
}

func TestCompliance_SM_PrivatePrefixNotAffected(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	// Private mode 4 should NOT set InsertMode
	h.Dispatch(scr, 'h', []int{4}, true)
	if scr.InsertMode {
		t.Fatal("SM ?4h: should not set InsertMode")
	}
}

// ─── RM/DECRST (l) — Reset Mode ─────────────────────────────────────────

func TestCompliance_RM_InsertMode(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.InsertMode = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{4}, false)
	if scr.InsertMode {
		t.Fatal("RM 4l: expected InsertMode=false")
	}
}

func TestCompliance_RM_LineFeedNewLine(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.LineFeedNewLine = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{20}, false)
	if scr.LineFeedNewLine {
		t.Fatal("RM 20l: expected LineFeedNewLine=false")
	}
}

// ─── DECSET/DECRST Private Modes ─────────────────────────────────────────

func TestCompliance_DECSET_OriginMode(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{6}, true)
	if !scr.OriginMode {
		t.Fatal("DECSET ?6h: expected OriginMode=true")
	}
}

func TestCompliance_DECRST_OriginMode(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.OriginMode = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{6}, true)
	if scr.OriginMode {
		t.Fatal("DECRST ?6l: expected OriginMode=false")
	}
}

func TestCompliance_DECSET_ApplicationCursor(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1}, true)
	if !scr.ApplicationCursor {
		t.Fatal("DECSET ?1h: expected ApplicationCursor=true")
	}
}

func TestCompliance_DECRST_ApplicationCursor(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.ApplicationCursor = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{1}, true)
	if scr.ApplicationCursor {
		t.Fatal("DECRST ?1l: expected ApplicationCursor=false")
	}
}

func TestCompliance_DECSET_KeypadApplication(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{66}, true)
	if !scr.KeypadApplication {
		t.Fatal("DECSET ?66h: expected KeypadApplication=true")
	}
}

func TestCompliance_DECRST_KeypadApplication(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.KeypadApplication = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{66}, true)
	if scr.KeypadApplication {
		t.Fatal("DECRST ?66l: expected KeypadApplication=false")
	}
}

func TestCompliance_DECSET_AutoWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.AutoWrap = false
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{7}, true)
	if !scr.AutoWrap {
		t.Fatal("DECSET ?7h: expected AutoWrap=true")
	}
}

func TestCompliance_DECRST_AutoWrap(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{7}, true)
	if scr.AutoWrap {
		t.Fatal("DECRST ?7l: expected AutoWrap=false")
	}
}

func TestCompliance_DECSET_CursorVisible(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CursorVisible = false
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{25}, true)
	if !scr.CursorVisible {
		t.Fatal("DECSET ?25h: expected CursorVisible=true")
	}
}

func TestCompliance_DECRST_CursorVisible(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{25}, true)
	if scr.CursorVisible {
		t.Fatal("DECRST ?25l: expected CursorVisible=false")
	}
}

func TestCompliance_DECSET_MouseTrackingBasic(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingBasic {
		t.Fatalf("DECSET ?1000h: want MouseTrackingBasic, got %d", scr.MouseTracking)
	}
}

func TestCompliance_DECSET_MouseTrackingButtonEvent(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1002}, true)
	if scr.MouseTracking != MouseTrackingButtonEvent {
		t.Fatalf("DECSET ?1002h: want MouseTrackingButtonEvent, got %d", scr.MouseTracking)
	}
}

func TestCompliance_DECSET_MouseTrackingAnyEvent(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1003}, true)
	if scr.MouseTracking != MouseTrackingAnyEvent {
		t.Fatalf("DECSET ?1003h: want MouseTrackingAnyEvent, got %d", scr.MouseTracking)
	}
}

func TestCompliance_DECSET_HighlightTracking(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1001}, true)
	if !scr.HighlightTracking {
		t.Fatal("DECSET ?1001h: expected HighlightTracking=true")
	}
}

func TestCompliance_DECRST_HighlightTracking(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.HighlightTracking = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{1001}, true)
	if scr.HighlightTracking {
		t.Fatal("DECRST ?1001l: expected HighlightTracking=false")
	}
}

func TestCompliance_DECSET_MouseSGR(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1006}, true)
	if !scr.MouseSGR {
		t.Fatal("DECSET ?1006h: expected MouseSGR=true")
	}
}

func TestCompliance_DECRST_MouseSGR(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.MouseSGR = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{1006}, true)
	if scr.MouseSGR {
		t.Fatal("DECRST ?1006l: expected MouseSGR=false")
	}
}

func TestCompliance_DECSET_FocusReporting(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{1004}, true)
	if !scr.FocusReporting {
		t.Fatal("DECSET ?1004h: expected FocusReporting=true")
	}
}

func TestCompliance_DECRST_FocusReporting(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.FocusReporting = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{1004}, true)
	if scr.FocusReporting {
		t.Fatal("DECRST ?1004l: expected FocusReporting=false")
	}
}

func TestCompliance_DECSET_BracketedPaste(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{2004}, true)
	if !scr.BracketedPaste {
		t.Fatal("DECSET ?2004h: expected BracketedPaste=true")
	}
}

func TestCompliance_DECRST_BracketedPaste(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.BracketedPaste = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{2004}, true)
	if scr.BracketedPaste {
		t.Fatal("DECRST ?2004l: expected BracketedPaste=false")
	}
}

func TestCompliance_DECSET_SynchronizedOutput(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	h.Dispatch(scr, 'h', []int{2026}, true)
	if !scr.SynchronizedOutput {
		t.Fatal("DECSET ?2026h: expected SynchronizedOutput=true")
	}
}

func TestCompliance_DECRST_SynchronizedOutput(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.SynchronizedOutput = true
	h := NewCSIHandler()
	h.Dispatch(scr, 'l', []int{2026}, true)
	if scr.SynchronizedOutput {
		t.Fatal("DECRST ?2026l: expected SynchronizedOutput=false")
	}
}

func TestCompliance_DECSET_AltScreen_Modes(t *testing.T) {
	t.Parallel()
	for _, mode := range []int{47, 1047, 1049} {
		scr := NewScreen(24, 80)
		var gotAlt bool
		h := NewCSIHandler(
			WithAltScreenFn(func(toAlt bool, _ int) { gotAlt = toAlt }),
		)
		h.Dispatch(scr, 'h', []int{mode}, true)
		if !gotAlt {
			t.Fatalf("DECSET ?%dh: expected AltScreenFn(true)", mode)
		}
		gotAlt = false
		h.Dispatch(scr, 'l', []int{mode}, true)
		if gotAlt {
			t.Fatalf("DECRST ?%dl: expected AltScreenFn(false)", mode)
		}
	}
}

func TestCompliance_DECSET_AltScreen_NilCallback(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	// Must not panic
	h.Dispatch(scr, 'h', []int{1049}, true)
	h.Dispatch(scr, 'l', []int{1049}, true)
}

// ─── SCP/RCP (s/u) — Save/Restore Cursor ────────────────────────────────

func TestCompliance_SCP_RCP_Basic(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 10
	scr.CurAttr.Bold = true
	h := NewCSIHandler()
	h.Dispatch(scr, 's', nil, false)
	scr.CurRow = 20
	scr.CurCol = 70
	scr.CurAttr.Bold = false
	h.Dispatch(scr, 'u', nil, false)
	if scr.CurRow != 5 || scr.CurCol != 10 || !scr.CurAttr.Bold {
		t.Fatalf("SCP/RCP: want (5,10,Bold), got (%d,%d,Bold=%v)",
			scr.CurRow, scr.CurCol, scr.CurAttr.Bold)
	}
}

func TestCompliance_SCP_SavesAllState(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 10
	scr.OriginMode = true
	scr.ApplicationCursor = true
	scr.BracketedPaste = true
	scr.CursorShape = 3
	scr.FocusReporting = true
	scr.AutoWrap = false
	scr.SynchronizedOutput = true
	scr.InsertMode = true
	scr.KeypadApplication = true
	scr.LineFeedNewLine = true
	scr.HighlightTracking = true
	scr.PendingWrap = true
	h := NewCSIHandler()
	h.Dispatch(scr, 's', nil, false)
	// Reset everything
	scr.CurRow = 0
	scr.CurCol = 0
	scr.OriginMode = false
	scr.ApplicationCursor = false
	scr.BracketedPaste = false
	scr.CursorShape = 0
	scr.FocusReporting = false
	scr.AutoWrap = true
	scr.SynchronizedOutput = false
	scr.InsertMode = false
	scr.KeypadApplication = false
	scr.LineFeedNewLine = false
	scr.HighlightTracking = false
	scr.PendingWrap = false
	h.Dispatch(scr, 'u', nil, false)
	if scr.CurRow != 5 || scr.CurCol != 10 {
		t.Fatalf("RCP cursor: want (5,10), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
	if !scr.OriginMode {
		t.Fatal("RCP: OriginMode not restored")
	}
	if !scr.ApplicationCursor {
		t.Fatal("RCP: ApplicationCursor not restored")
	}
	if !scr.BracketedPaste {
		t.Fatal("RCP: BracketedPaste not restored")
	}
	if scr.CursorShape != 3 {
		t.Fatalf("RCP: CursorShape = %d, want 3", scr.CursorShape)
	}
	if !scr.FocusReporting {
		t.Fatal("RCP: FocusReporting not restored")
	}
	if scr.AutoWrap {
		t.Fatal("RCP: AutoWrap not restored (should be false)")
	}
	if !scr.SynchronizedOutput {
		t.Fatal("RCP: SynchronizedOutput not restored")
	}
	if !scr.InsertMode {
		t.Fatal("RCP: InsertMode not restored")
	}
	if !scr.KeypadApplication {
		t.Fatal("RCP: KeypadApplication not restored")
	}
	if !scr.LineFeedNewLine {
		t.Fatal("RCP: LineFeedNewLine not restored")
	}
	if !scr.HighlightTracking {
		t.Fatal("RCP: HighlightTracking not restored")
	}
	if !scr.PendingWrap {
		t.Fatal("RCP: PendingWrap not restored")
	}
}

// ─── DA (c) — Device Attributes ─────────────────────────────────────────

func TestCompliance_DA1_Primary(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[c"))
	resp := getResp()
	if string(resp) != "\x1b[?64;22c" {
		t.Fatalf("DA1: want %q, got %q", "\x1b[?64;22c", string(resp))
	}
}

func TestCompliance_DA2_Secondary(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[>c"))
	resp := getResp()
	if string(resp) != "\x1b[>1;0;0c" {
		t.Fatalf("DA2: want %q, got %q", "\x1b[>1;0;0c", string(resp))
	}
}

func TestCompliance_DA1_PrivateMode_NoResponse(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[?c"))
	resp := getResp()
	if resp != nil {
		t.Fatalf("DA1 private: should not respond, got %q", string(resp))
	}
}

func TestCompliance_DA_NoResponseWithoutWriter(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Must not panic
	v.Write([]byte("\x1b[c"))
	v.Write([]byte("\x1b[>c"))
}

// ─── DSR (n) — Device Status Report ─────────────────────────────────────

func TestCompliance_DSR_OK(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[5n"))
	resp := getResp()
	if string(resp) != "\x1b[0n" {
		t.Fatalf("DSR-OK: want %q, got %q", "\x1b[0n", string(resp))
	}
}

func TestCompliance_DSR_CPR(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[6n"))
	resp := getResp()
	if string(resp) != "\x1b[1;1R" {
		t.Fatalf("DSR-CPR: want %q, got %q", "\x1b[1;1R", string(resp))
	}
}

func TestCompliance_DSR_NoParams_NoResponse(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[n"))
	resp := getResp()
	if resp != nil {
		t.Fatalf("DSR no params: should not respond, got %q", string(resp))
	}
}

// ─── DECSCUSR (q) — Cursor Style ────────────────────────────────────────

func TestCompliance_DECSCUSR_WithSpace(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[3 q"))
	if v.primary.CursorShape != 3 {
		t.Fatalf("DECSCUSR 3: want 3, got %d", v.primary.CursorShape)
	}
}

func TestCompliance_DECSCUSR_WithoutSpace_Ignored(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[3q"))
	if v.primary.CursorShape != 0 {
		t.Fatalf("DECSCUSR without space: want 0 (unchanged), got %d", v.primary.CursorShape)
	}
}

func TestCompliance_DECSCUSR_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[7 q"))
	if v.primary.CursorShape != 0 {
		t.Fatalf("DECSCUSR 7: want 0 (clamped), got %d", v.primary.CursorShape)
	}
}

func TestCompliance_DECSCUSR_NegativeOutOfRange(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler(
		WithHasInterSp(func() bool { return true }),
	)
	h.Dispatch(scr, 'q', []int{-1}, false)
	if scr.CursorShape != 0 {
		t.Fatalf("DECSCUSR -1: want 0 (clamped), got %d", scr.CursorShape)
	}
}

// ─── XTWINOPS (t) — Window Manipulation ─────────────────────────────────

func TestCompliance_XTWINOPS_Resize(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[8;30;120t"))
	scr := v.ActiveScreen()
	if scr.Rows != 30 || scr.Cols != 120 {
		t.Fatalf("XTWINOPS resize: want 30x120, got %dx%d", scr.Rows, scr.Cols)
	}
}

func TestCompliance_XTWINOPS_ReportSize(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[18t"))
	resp := getResp()
	if string(resp) != "\x1b[8;480;800t" {
		t.Fatalf("XTWINOPS report: want %q, got %q", "\x1b[8;480;800t", string(resp))
	}
}

func TestCompliance_XTWINOPS_UnknownSubcommand(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[22t"))
	resp := getResp()
	if resp != nil {
		t.Fatalf("XTWINOPS unknown: should not respond, got %q", string(resp))
	}
}

func TestCompliance_XTWINOPS_DefaultParams(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[8t"))
	scr := v.ActiveScreen()
	if scr.Rows != 24 || scr.Cols != 80 {
		t.Fatalf("XTWINOPS default: want 24x80, got %dx%d", scr.Rows, scr.Cols)
	}
}

// ─── DECSTR/DECRQM (p) ──────────────────────────────────────────────────

func TestCompliance_DECSTR_SoftReset(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[4h"))
	if !v.primary.InsertMode {
		t.Fatal("precondition: InsertMode should be true")
	}
	v.Write([]byte("\x1b[!p"))
	if v.primary.InsertMode {
		t.Fatal("DECSTR: InsertMode should be false after soft reset")
	}
}

func TestCompliance_DECRQM_QueryMode(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[?1h"))
	v.Write([]byte("\x1b[?1$p"))
	resp := getResp()
	if string(resp) != "\x1b[?1;2$y" {
		t.Fatalf("DECRQM: want %q, got %q", "\x1b[?1;2$y", string(resp))
	}
}

func TestCompliance_DECRQM_UnrecognizedMode(t *testing.T) {
	t.Parallel()
	v, getResp := csiComplianceHelper()
	v.Write([]byte("\x1b[?999$p"))
	resp := getResp()
	if string(resp) != "\x1b[?999;1$y" {
		t.Fatalf("DECRQM unknown: want %q, got %q", "\x1b[?999;1$y", string(resp))
	}
}

// ─── Unknown Final Byte ─────────────────────────────────────────────────

func TestCompliance_UnknownFinalByte_NoPanic(t *testing.T) {
	t.Parallel()
	scr := NewScreen(24, 80)
	h := NewCSIHandler()
	// Must not panic on unrecognised final bytes
	h.Dispatch(scr, '~', []int{42}, false)
	h.Dispatch(scr, '{', []int{1}, false)
	h.Dispatch(scr, 'W', []int{1}, false)
}

// ─── VTerm Integration: Full Sequence via Write ──────────────────────────

func TestCompliance_VTerm_CUU(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;1H"))
	v.Write([]byte("\x1b[3A"))
	row, _ := v.CursorPosition()
	if row != 6 {
		t.Fatalf("VTerm CUU: want row 6, got %d", row)
	}
}

func TestCompliance_VTerm_CUD(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;1H"))
	v.Write([]byte("\x1b[3B"))
	row, _ := v.CursorPosition()
	if row != 7 {
		t.Fatalf("VTerm CUD: want row 7, got %d", row)
	}
}

func TestCompliance_VTerm_CNL(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;30H"))
	v.Write([]byte("\x1b[3E"))
	row, col := v.CursorPosition()
	if row != 7 || col != 0 {
		t.Fatalf("VTerm CNL: want (7,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_CPL(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;30H"))
	v.Write([]byte("\x1b[3F"))
	row, col := v.CursorPosition()
	if row != 6 || col != 0 {
		t.Fatalf("VTerm CPL: want (6,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_CHA(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20G"))
	_, col := v.CursorPosition()
	if col != 19 {
		t.Fatalf("VTerm CHA: want col 19, got %d", col)
	}
}

func TestCompliance_VTerm_VPA(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[12d"))
	row, _ := v.CursorPosition()
	if row != 11 {
		t.Fatalf("VTerm VPA: want row 11, got %d", row)
	}
}

func TestCompliance_VTerm_ED_Mode2(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("ABCDE"))
	v.Write([]byte("\x1b[2J"))
	s := v.String()
	if s != "" {
		t.Fatalf("VTerm ED 2: screen should be empty, got %q", s)
	}
}

func TestCompliance_VTerm_EL_Mode2(t *testing.T) {
	t.Parallel()
	v := NewVTerm(1, 10)
	v.Write([]byte("ABCDEFGHIJ"))
	v.Write([]byte("\x1b[1;5H"))
	v.Write([]byte("\x1b[2K"))
	snap := v.ActiveScreen()
	// Entire line erased — all cells should be spaces
	for c := range 10 {
		if snap.Cells[0][c].Ch != ' ' {
			t.Fatalf("VTerm EL 2: col %d should be blank, got %q", c, snap.Cells[0][c].Ch)
		}
	}
}

func TestCompliance_VTerm_IL(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[2;1H"))
	v.Write([]byte("\x1b[1L"))
	snap := v.ActiveScreen()
	if snap.Cells[1][0].Ch != ' ' {
		t.Fatalf("VTerm IL: row 1 should be blank, got %q", snap.Cells[1][0].Ch)
	}
}

func TestCompliance_VTerm_DL(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[2;1H"))
	v.Write([]byte("\x1b[1M"))
	snap := v.ActiveScreen()
	if snap.Cells[1][0].Ch != ' ' {
		t.Fatalf("VTerm DL: row 1 should be blank (from row 2), got %q", snap.Cells[1][0].Ch)
	}
}

func TestCompliance_VTerm_SU(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[1S"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch == 'A' {
		t.Fatal("VTerm SU: row 0 should have scrolled up")
	}
}

func TestCompliance_VTerm_SD(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[1T"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch != ' ' {
		t.Fatalf("VTerm SD: row 0 should be blank, got %q", snap.Cells[0][0].Ch)
	}
}

func TestCompliance_VTerm_DECSTBM_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Set a scroll region first
	v.Write([]byte("\x1b[5;20r"))
	// Then reset with default params
	v.Write([]byte("\x1b[r"))
	scr := v.ActiveScreen()
	top, bot := scr.ScrollRegion()
	if top != 0 || bot != 24 {
		t.Fatalf("VTerm DECSTBM default: want (0,24), got (%d,%d)", top, bot)
	}
}

func TestCompliance_VTerm_SCP_RCP(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;30H"))
	v.Write([]byte("\x1b[s"))
	v.Write([]byte("\x1b[1;1H"))
	v.Write([]byte("\x1b[u"))
	row, col := v.CursorPosition()
	if row != 9 || col != 29 {
		t.Fatalf("VTerm SCP/RCP: want (9,29), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_TBC_Mode3(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[3g"))
	scr := v.ActiveScreen()
	for i, ts := range scr.TabStops {
		if ts {
			t.Fatalf("VTerm TBC 3: tab at col %d should be cleared", i)
		}
	}
}

func TestCompliance_VTerm_SGR_Reset(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1m"))
	if !v.primary.CurAttr.Bold {
		t.Fatal("VTerm SGR bold: expected Bold=true")
	}
	v.Write([]byte("\x1b[m"))
	if v.primary.CurAttr.Bold {
		t.Fatal("VTerm SGR reset: expected Bold=false")
	}
}

// ─── Edge Cases: Out-of-Range via VTerm Write ────────────────────────────

func TestCompliance_VTerm_CUU_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[2;1H"))
	v.Write([]byte("\x1b[9999A"))
	row, _ := v.CursorPosition()
	if row != 0 {
		t.Fatalf("VTerm CUU overflow: want row 0, got %d", row)
	}
}

func TestCompliance_VTerm_CUD_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[22;1H"))
	v.Write([]byte("\x1b[9999B"))
	row, _ := v.CursorPosition()
	if row != 23 {
		t.Fatalf("VTerm CUD overflow: want row 23, got %d", row)
	}
}

func TestCompliance_VTerm_CUF_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[9999C"))
	_, col := v.CursorPosition()
	if col != 79 {
		t.Fatalf("VTerm CUF overflow: want col 79, got %d", col)
	}
}

func TestCompliance_VTerm_CUB_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[9999D"))
	_, col := v.CursorPosition()
	if col != 0 {
		t.Fatalf("VTerm CUB overflow: want col 0, got %d", col)
	}
}

func TestCompliance_VTerm_CUP_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[9999;9999H"))
	row, col := v.CursorPosition()
	if row != 23 || col != 79 {
		t.Fatalf("VTerm CUP overflow: want (23,79), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_CHA_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[9999G"))
	_, col := v.CursorPosition()
	if col != 79 {
		t.Fatalf("VTerm CHA overflow: want col 79, got %d", col)
	}
}

func TestCompliance_VTerm_VPA_OutOfRange(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[9999d"))
	row, _ := v.CursorPosition()
	if row != 23 {
		t.Fatalf("VTerm VPA overflow: want row 23, got %d", row)
	}
}

// ─── Default Parameters via VTerm Write ──────────────────────────────────

func TestCompliance_VTerm_CUU_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;1H"))
	v.Write([]byte("\x1b[A"))
	row, _ := v.CursorPosition()
	if row != 3 {
		t.Fatalf("VTerm CUU default: want row 3, got %d", row)
	}
}

func TestCompliance_VTerm_CUD_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;1H"))
	v.Write([]byte("\x1b[B"))
	row, _ := v.CursorPosition()
	if row != 5 {
		t.Fatalf("VTerm CUD default: want row 5, got %d", row)
	}
}

func TestCompliance_VTerm_CUF_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[C"))
	_, col := v.CursorPosition()
	if col != 1 {
		t.Fatalf("VTerm CUF default: want col 1, got %d", col)
	}
}

func TestCompliance_VTerm_CUB_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;10H"))
	v.Write([]byte("\x1b[D"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("VTerm CUB default: want col 8, got %d", col)
	}
}

func TestCompliance_VTerm_CNL_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;30H"))
	v.Write([]byte("\x1b[E"))
	row, col := v.CursorPosition()
	if row != 5 || col != 0 {
		t.Fatalf("VTerm CNL default: want (5,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_CPL_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;30H"))
	v.Write([]byte("\x1b[F"))
	row, col := v.CursorPosition()
	if row != 3 || col != 0 {
		t.Fatalf("VTerm CPL default: want (3,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_CUP_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;30H"))
	v.Write([]byte("\x1b[H"))
	row, col := v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("VTerm CUP default: want (0,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_CHA_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;30H"))
	v.Write([]byte("\x1b[G"))
	_, col := v.CursorPosition()
	if col != 0 {
		t.Fatalf("VTerm CHA default: want col 0, got %d", col)
	}
}

func TestCompliance_VTerm_VPA_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[10;30H"))
	v.Write([]byte("\x1b[d"))
	row, _ := v.CursorPosition()
	if row != 0 {
		t.Fatalf("VTerm VPA default: want row 0, got %d", row)
	}
}

func TestCompliance_VTerm_ED_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[3;1H"))
	v.Write([]byte("\x1b[J"))
	// Default is mode 0 (cursor to end) — rows 2-4 should be erased
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch != 'A' {
		t.Fatal("VTerm ED default: row 0 should be untouched")
	}
	if snap.Cells[2][0].Ch != ' ' {
		t.Fatal("VTerm ED default: row 2 should be erased")
	}
}

func TestCompliance_VTerm_EL_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(1, 10)
	v.Write([]byte("ABCDEFGHIJ"))
	v.Write([]byte("\x1b[1;5H"))
	v.Write([]byte("\x1b[K"))
	snap := v.ActiveScreen()
	if snap.Cells[0][3].Ch != 'D' {
		t.Fatal("VTerm EL default: col 3 should be untouched")
	}
	if snap.Cells[0][4].Ch != ' ' {
		t.Fatal("VTerm EL default: col 4 should be erased")
	}
}

func TestCompliance_VTerm_IL_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[2;1H"))
	v.Write([]byte("\x1b[L"))
	snap := v.ActiveScreen()
	if snap.Cells[1][0].Ch != ' ' {
		t.Fatalf("VTerm IL default: row 1 should be blank, got %q", snap.Cells[1][0].Ch)
	}
}

func TestCompliance_VTerm_DL_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[1;1H"))
	v.Write([]byte("\x1b[M"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch == 'A' {
		t.Fatal("VTerm DL default: row 0 should have shifted up")
	}
}

func TestCompliance_VTerm_DCH_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(1, 5)
	v.Write([]byte("ABCDE"))
	v.Write([]byte("\x1b[1;2H"))
	v.Write([]byte("\x1b[P"))
	s := v.String()
	if s != "ACDE" {
		t.Fatalf("VTerm DCH default: want 'ACDE', got %q", s)
	}
}

func TestCompliance_VTerm_ECH_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(1, 5)
	v.Write([]byte("ABCDE"))
	v.Write([]byte("\x1b[1;2H"))
	v.Write([]byte("\x1b[X"))
	snap := v.ActiveScreen()
	if snap.Cells[0][1].Ch != ' ' {
		t.Fatalf("VTerm ECH default: col 1 should be blank, got %q", snap.Cells[0][1].Ch)
	}
	if snap.Cells[0][2].Ch != 'C' {
		t.Fatalf("VTerm ECH default: col 2 should be 'C', got %q", snap.Cells[0][2].Ch)
	}
}

func TestCompliance_VTerm_ICH_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(1, 5)
	v.Write([]byte("ABCDE"))
	v.Write([]byte("\x1b[1;2H"))
	v.Write([]byte("\x1b[@"))
	snap := v.ActiveScreen()
	if snap.Cells[0][1].Ch != ' ' {
		t.Fatalf("VTerm ICH default: col 1 should be blank, got %q", snap.Cells[0][1].Ch)
	}
	if snap.Cells[0][2].Ch != 'B' {
		t.Fatalf("VTerm ICH default: col 2 should be 'B', got %q", snap.Cells[0][2].Ch)
	}
}

func TestCompliance_VTerm_SU_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[S"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch == 'A' {
		t.Fatal("VTerm SU default: row 0 should have scrolled up")
	}
}

func TestCompliance_VTerm_SD_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(5, 5)
	v.Write([]byte("AAAAA"))
	v.Write([]byte("\x1b[T"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch != ' ' {
		t.Fatalf("VTerm SD default: row 0 should be blank, got %q", snap.Cells[0][0].Ch)
	}
}

func TestCompliance_VTerm_TBC_Default(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1;9H"))
	v.Write([]byte("\x1b[g"))
	scr := v.ActiveScreen()
	if scr.TabStops[8] {
		t.Fatal("VTerm TBC default: tab at col 8 should be cleared")
	}
}

// ─── Private Mode Prefix via VTerm Write ─────────────────────────────────

func TestCompliance_VTerm_DECSET_PrivatePrefix(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?25l"))
	if v.primary.CursorVisible {
		t.Fatal("VTerm DECSET ?25l: expected CursorVisible=false")
	}
	v.Write([]byte("\x1b[?25h"))
	if !v.primary.CursorVisible {
		t.Fatal("VTerm DECSET ?25h: expected CursorVisible=true")
	}
}

func TestCompliance_VTerm_SM_NonPrivate(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[4h"))
	if !v.primary.InsertMode {
		t.Fatal("VTerm SM 4h: expected InsertMode=true")
	}
	v.Write([]byte("\x1b[4l"))
	if v.primary.InsertMode {
		t.Fatal("VTerm RM 4l: expected InsertMode=false")
	}
}

func TestCompliance_VTerm_SM_PrivateNotAffected(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// CSI ?4h (private mode 4) should NOT set InsertMode
	v.Write([]byte("\x1b[?4h"))
	if v.primary.InsertMode {
		t.Fatal("VTerm SM ?4h: should not set InsertMode")
	}
}

func TestCompliance_VTerm_DECSET_OriginMode_HomesCursor(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;15r"))
	v.Write([]byte("\x1b[10;30H"))
	v.Write([]byte("\x1b[?6h"))
	row, col := v.CursorPosition()
	if row != 4 || col != 0 {
		t.Fatalf("VTerm DECSET ?6h: want (4,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_DECRST_OriginMode_HomesCursor(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;15r"))
	v.Write([]byte("\x1b[?6h"))
	v.Write([]byte("\x1b[?6l"))
	row, col := v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("VTerm DECRST ?6l: want (0,0), got (%d,%d)", row, col)
	}
}

func TestCompliance_VTerm_DECRST_AutoWrap_ClearsPendingWrap(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Write to last column to set PendingWrap
	v.Write([]byte("01234567890123456789012345678901234567890123456789012345678901234567890123456789"))
	if !v.primary.PendingWrap {
		t.Fatal("precondition: PendingWrap should be true")
	}
	// DECRST ?7l should clear PendingWrap
	v.Write([]byte("\x1b[?7l"))
	if v.primary.PendingWrap {
		t.Fatal("VTerm DECRST ?7l: should clear PendingWrap")
	}
}

// ─── Multi-param DECSET/DECRST ───────────────────────────────────────────

func TestCompliance_VTerm_DECSET_MultiParam(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1;25h"))
	if !v.primary.ApplicationCursor {
		t.Fatal("VTerm DECSET ?1;25h: expected ApplicationCursor=true")
	}
	if !v.primary.CursorVisible {
		t.Fatal("VTerm DECSET ?1;25h: expected CursorVisible=true")
	}
}

func TestCompliance_VTerm_DECRST_MultiParam(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1;25h"))
	v.Write([]byte("\x1b[?1;25l"))
	if v.primary.ApplicationCursor {
		t.Fatal("VTerm DECRST ?1;25l: expected ApplicationCursor=false")
	}
	if v.primary.CursorVisible {
		t.Fatal("VTerm DECRST ?1;25l: expected CursorVisible=false")
	}
}

// ─── CUP with 'f' final byte via VTerm ───────────────────────────────────

func TestCompliance_VTerm_CUP_f_FinalByte(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5;10f"))
	row, col := v.CursorPosition()
	if row != 4 || col != 9 {
		t.Fatalf("VTerm CUP f: want (4,9), got (%d,%d)", row, col)
	}
}
