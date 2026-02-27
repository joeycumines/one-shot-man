package vt

import (
	"testing"
)

// ── handleControl ──────────────────────────────────────────────────

func TestHandleControl_BS_DecrementsCol(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurCol = 5
	v.handleControl(0x08) // BS
	if v.active.CurCol != 4 {
		t.Errorf("CurCol after BS: got %d, want 4", v.active.CurCol)
	}
}

func TestHandleControl_BS_AtColZero_NoOp(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurCol = 0
	v.handleControl(0x08) // BS
	if v.active.CurCol != 0 {
		t.Errorf("CurCol after BS at 0: got %d, want 0", v.active.CurCol)
	}
}

func TestHandleControl_BS_ClearsPendingWrap(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.PendingWrap = true
	v.active.CurCol = 5
	v.handleControl(0x08) // BS
	if v.active.PendingWrap {
		t.Error("PendingWrap should be cleared after BS")
	}
	if v.active.CurCol != 4 {
		t.Errorf("CurCol after BS: got %d, want 4", v.active.CurCol)
	}
}

func TestHandleControl_TAB_AdvancesToNextTabStop(t *testing.T) {
	v := NewVTerm(4, 80)
	v.active.CurCol = 3
	v.handleControl(0x09) // TAB
	// Default tab stops at 0, 8, 16, 24, ...
	// From col 3, next tab stop is col 8.
	if v.active.CurCol != 8 {
		t.Errorf("CurCol after TAB from 3: got %d, want 8", v.active.CurCol)
	}
}

func TestHandleControl_TAB_AtLastCol_ClampsToEnd(t *testing.T) {
	v := NewVTerm(4, 10) // cols = 10, last col index = 9
	v.active.CurCol = 9
	v.handleControl(0x09) // TAB
	// No tab stop beyond col 9 (cols-1=9). Should clamp to cols-1.
	if v.active.CurCol != 9 {
		t.Errorf("CurCol after TAB at end: got %d, want 9", v.active.CurCol)
	}
}

func TestHandleControl_TAB_ClearsPendingWrap(t *testing.T) {
	v := NewVTerm(4, 80)
	v.active.PendingWrap = true
	v.active.CurCol = 3
	v.handleControl(0x09) // TAB
	if v.active.PendingWrap {
		t.Error("PendingWrap should be cleared after TAB")
	}
}

func TestHandleControl_TAB_NoTabStopAfter_ClampsToLastCol(t *testing.T) {
	// Create a 10-column screen. Tab stops at 0 and 8. CurCol=8 → next would be past end.
	v := NewVTerm(4, 10)
	v.active.CurCol = 8
	v.handleControl(0x09) // TAB
	// No tab stop at index >8 within 10 cols. Clamp to cols-1=9.
	if v.active.CurCol != 9 {
		t.Errorf("CurCol after TAB at 8 in 10-col: got %d, want 9", v.active.CurCol)
	}
}

func TestHandleControl_LF_AdvancesRow(t *testing.T) {
	v := NewVTerm(4, 10) // 4 rows
	v.active.CurRow = 1
	v.handleControl(0x0A) // LF
	if v.active.CurRow != 2 {
		t.Errorf("CurRow after LF: got %d, want 2", v.active.CurRow)
	}
}

func TestHandleControl_VT_SameAsLF(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurRow = 1
	v.handleControl(0x0B) // VT
	if v.active.CurRow != 2 {
		t.Errorf("CurRow after VT: got %d, want 2", v.active.CurRow)
	}
}

func TestHandleControl_FF_SameAsLF(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurRow = 1
	v.handleControl(0x0C) // FF
	if v.active.CurRow != 2 {
		t.Errorf("CurRow after FF: got %d, want 2", v.active.CurRow)
	}
}

func TestHandleControl_CR_ResetsCol(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurCol = 7
	v.handleControl(0x0D) // CR
	if v.active.CurCol != 0 {
		t.Errorf("CurCol after CR: got %d, want 0", v.active.CurCol)
	}
}

func TestHandleControl_CR_ClearsPendingWrap(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.PendingWrap = true
	v.active.CurCol = 5
	v.handleControl(0x0D) // CR
	if v.active.PendingWrap {
		t.Error("PendingWrap should be cleared after CR")
	}
}

func TestHandleControl_BEL_NoOp(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurCol = 5
	v.active.CurRow = 2
	v.handleControl(0x07) // BEL
	if v.active.CurCol != 5 || v.active.CurRow != 2 {
		t.Errorf("BEL should not move cursor: col=%d row=%d", v.active.CurCol, v.active.CurRow)
	}
}

func TestHandleControl_UnrecognizedByte_Ignored(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurCol = 5
	v.active.CurRow = 2
	v.handleControl(0x01) // SOH — not in the switch
	if v.active.CurCol != 5 || v.active.CurRow != 2 {
		t.Errorf("Unrecognized control byte should not move cursor: col=%d row=%d",
			v.active.CurCol, v.active.CurRow)
	}
}

// ── scrollRegionUp / scrollRegionDown (unexported) ─────────────────

func TestScrollRegionUp_Normal(t *testing.T) {
	s := NewScreen(5, 3) // 5 rows × 3 cols
	// Fill rows with identifiable content.
	for r := 0; r < 5; r++ {
		for c := 0; c < 3; c++ {
			s.Cells[r][c].Ch = rune('A' + r)
		}
	}
	// Scroll rows 1..4 (top=1, bot=4) up by 1.
	s.scrollRegionUp(1, 4, 1)
	// Row 0 unchanged.
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0: got %c, want A", s.Cells[0][0].Ch)
	}
	// Row 1 should now have what was row 2: 'C'.
	if s.Cells[1][0].Ch != 'C' {
		t.Errorf("row 1: got %c, want C", s.Cells[1][0].Ch)
	}
	// Row 2 should now have what was row 3: 'D'.
	if s.Cells[2][0].Ch != 'D' {
		t.Errorf("row 2: got %c, want D", s.Cells[2][0].Ch)
	}
	// Row 3 should be blank (newly scrolled in).
	if s.Cells[3][0].Ch != ' ' {
		t.Errorf("row 3: got %c, want space", s.Cells[3][0].Ch)
	}
	// Row 4 unchanged (outside scroll region 1..4; 'E' = 'A'+4).
	if s.Cells[4][0].Ch != 'E' {
		t.Errorf("row 4: got %c, want E", s.Cells[4][0].Ch)
	}
}

func TestScrollRegionUp_NGreaterThanRange(t *testing.T) {
	s := NewScreen(4, 3)
	for r := 0; r < 4; r++ {
		for c := 0; c < 3; c++ {
			s.Cells[r][c].Ch = rune('A' + r)
		}
	}
	// n=10 should be clamped to bot-top=2 (top=1, bot=3).
	s.scrollRegionUp(1, 3, 10)
	// Row 0 unchanged.
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0: got %c, want A", s.Cells[0][0].Ch)
	}
	// Rows 1,2 should be blank (all scrolled out).
	for r := 1; r <= 2; r++ {
		if s.Cells[r][0].Ch != ' ' {
			t.Errorf("row %d: got %c, want space", r, s.Cells[r][0].Ch)
		}
	}
	// Row 3 unchanged.
	if s.Cells[3][0].Ch != 'D' {
		t.Errorf("row 3: got %c, want D", s.Cells[3][0].Ch)
	}
}

func TestScrollRegionUp_ZeroN_NoOp(t *testing.T) {
	s := NewScreen(3, 3)
	for r := 0; r < 3; r++ {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.scrollRegionUp(0, 3, 0)
	for r := 0; r < 3; r++ {
		want := rune('A' + r)
		if s.Cells[r][0].Ch != want {
			t.Errorf("row %d: got %c, want %c", r, s.Cells[r][0].Ch, want)
		}
	}
}

func TestScrollRegionDown_Normal(t *testing.T) {
	s := NewScreen(5, 3)
	for r := 0; r < 5; r++ {
		for c := 0; c < 3; c++ {
			s.Cells[r][c].Ch = rune('A' + r)
		}
	}
	// Scroll rows 1..4 (top=1, bot=4) down by 1.
	s.scrollRegionDown(1, 4, 1)
	// Row 0 unchanged.
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0: got %c, want A", s.Cells[0][0].Ch)
	}
	// Row 1 should be blank (newly scrolled in).
	if s.Cells[1][0].Ch != ' ' {
		t.Errorf("row 1: got %c, want space", s.Cells[1][0].Ch)
	}
	// Row 2 should have what was row 1: 'B'.
	if s.Cells[2][0].Ch != 'B' {
		t.Errorf("row 2: got %c, want B", s.Cells[2][0].Ch)
	}
	// Row 3 should have what was row 2: 'C'.
	if s.Cells[3][0].Ch != 'C' {
		t.Errorf("row 3: got %c, want C", s.Cells[3][0].Ch)
	}
	// Row 4 unchanged (outside scroll region 1..4; 'E' = 'A'+4).
	if s.Cells[4][0].Ch != 'E' {
		t.Errorf("row 4: got %c, want E", s.Cells[4][0].Ch)
	}
}

func TestScrollRegionDown_NGreaterThanRange(t *testing.T) {
	s := NewScreen(4, 3)
	for r := 0; r < 4; r++ {
		for c := 0; c < 3; c++ {
			s.Cells[r][c].Ch = rune('A' + r)
		}
	}
	// n=10 should be clamped to bot-top=2 (top=1, bot=3).
	s.scrollRegionDown(1, 3, 10)
	// Row 0 unchanged.
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0: got %c, want A", s.Cells[0][0].Ch)
	}
	// Rows 1,2 should be blank (all original content scrolled off bottom).
	for r := 1; r <= 2; r++ {
		if s.Cells[r][0].Ch != ' ' {
			t.Errorf("row %d: got %c, want space", r, s.Cells[r][0].Ch)
		}
	}
	// Row 3 unchanged.
	if s.Cells[3][0].Ch != 'D' {
		t.Errorf("row 3: got %c, want D", s.Cells[3][0].Ch)
	}
}

func TestScrollRegionDown_ZeroN_NoOp(t *testing.T) {
	s := NewScreen(3, 3)
	for r := 0; r < 3; r++ {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.scrollRegionDown(0, 3, 0)
	for r := 0; r < 3; r++ {
		want := rune('A' + r)
		if s.Cells[r][0].Ch != want {
			t.Errorf("row %d: got %c, want %c", r, s.Cells[r][0].Ch, want)
		}
	}
}

func TestScrollRegionUp_TopGeBot_NoOp(t *testing.T) {
	s := NewScreen(3, 3)
	for r := 0; r < 3; r++ {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.scrollRegionUp(2, 2, 1) // top == bot
	for r := 0; r < 3; r++ {
		want := rune('A' + r)
		if s.Cells[r][0].Ch != want {
			t.Errorf("row %d: got %c, want %c", r, s.Cells[r][0].Ch, want)
		}
	}
}

func TestScrollRegionDown_TopGeBot_NoOp(t *testing.T) {
	s := NewScreen(3, 3)
	for r := 0; r < 3; r++ {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.scrollRegionDown(2, 2, 1) // top == bot
	for r := 0; r < 3; r++ {
		want := rune('A' + r)
		if s.Cells[r][0].Ch != want {
			t.Errorf("row %d: got %c, want %c", r, s.Cells[r][0].Ch, want)
		}
	}
}

// ── makeDefaultTabStops ────────────────────────────────────────────

func TestMakeDefaultTabStops_Normal80(t *testing.T) {
	ts := makeDefaultTabStops(80)
	if len(ts) != 80 {
		t.Fatalf("len: got %d, want 80", len(ts))
	}
	for i := 0; i < 80; i++ {
		want := (i % 8) == 0
		if ts[i] != want {
			t.Errorf("ts[%d] = %v, want %v", i, ts[i], want)
		}
	}
}

func TestMakeDefaultTabStops_SmallCols(t *testing.T) {
	ts := makeDefaultTabStops(5)
	if len(ts) != 5 {
		t.Fatalf("len: got %d, want 5", len(ts))
	}
	// Only ts[0] should be true.
	if !ts[0] {
		t.Error("ts[0] should be true")
	}
	for i := 1; i < 5; i++ {
		if ts[i] {
			t.Errorf("ts[%d] should be false", i)
		}
	}
}

func TestMakeDefaultTabStops_ZeroCols(t *testing.T) {
	ts := makeDefaultTabStops(0)
	if len(ts) != 0 {
		t.Errorf("len: got %d, want 0", len(ts))
	}
}

// ── switchToAlt / switchToPrimary ──────────────────────────────────

func TestSwitchToAlt_SavesCursor(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurRow = 2
	v.active.CurCol = 7
	v.active.CurAttr = Attr{Bold: true}
	v.switchToAlt()

	if v.active != v.alternate {
		t.Error("active should be alternate after switchToAlt")
	}
	// Primary should have saved cursor.
	if v.primary.SavedRow != 2 || v.primary.SavedCol != 7 {
		t.Errorf("saved cursor: row=%d col=%d, want 2,7",
			v.primary.SavedRow, v.primary.SavedCol)
	}
	if !v.primary.SavedAttr.Bold {
		t.Error("saved attr should have Bold")
	}
}

func TestSwitchToAlt_Idempotent(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurRow = 2
	v.active.CurCol = 7
	v.switchToAlt()
	// Move cursor on alt screen.
	v.active.CurRow = 0
	v.active.CurCol = 0
	// Switch to alt again — should be no-op (already on alt).
	v.switchToAlt()
	if v.active != v.alternate {
		t.Error("should still be on alternate")
	}
	// Cursor should NOT be re-saved (it was already on alt).
	if v.primary.SavedRow != 2 || v.primary.SavedCol != 7 {
		t.Errorf("second switchToAlt should not re-save: row=%d col=%d",
			v.primary.SavedRow, v.primary.SavedCol)
	}
}

func TestSwitchToPrimary_RestoresCursor(t *testing.T) {
	v := NewVTerm(4, 10)
	v.active.CurRow = 2
	v.active.CurCol = 7
	v.active.CurAttr = Attr{Italic: true}
	v.switchToAlt()
	// Switch back to primary.
	v.switchToPrimary()
	if v.active != v.primary {
		t.Error("active should be primary after switchToPrimary")
	}
	if v.active.CurRow != 2 || v.active.CurCol != 7 {
		t.Errorf("restored cursor: row=%d col=%d, want 2,7",
			v.active.CurRow, v.active.CurCol)
	}
	if !v.active.CurAttr.Italic {
		t.Error("restored attr should have Italic")
	}
}

func TestSwitchToPrimary_Idempotent(t *testing.T) {
	v := NewVTerm(4, 10)
	// Already on primary. Should be no-op.
	v.switchToPrimary()
	if v.active != v.primary {
		t.Error("should still be on primary")
	}
}
