package vt

import "testing"

func TestNewScreen(t *testing.T) {
	s := NewScreen(24, 80)
	if s.Rows != 24 || s.Cols != 80 {
		t.Fatalf("dims = %d×%d, want 24×80", s.Rows, s.Cols)
	}
	if len(s.Cells) != 24 {
		t.Fatalf("len(Cells) = %d, want 24", len(s.Cells))
	}
	for r, row := range s.Cells {
		if len(row) != 80 {
			t.Fatalf("row %d len = %d, want 80", r, len(row))
		}
		for c, cell := range row {
			if cell != DefaultCell {
				t.Fatalf("cell[%d][%d] not default", r, c)
			}
		}
	}
	if !s.CursorVisible {
		t.Error("CursorVisible should default true")
	}
	if s.CurRow != 0 || s.CurCol != 0 {
		t.Error("cursor should start at 0,0")
	}
}

func TestNewScreen_tabStops(t *testing.T) {
	s := NewScreen(24, 80)
	for i, stop := range s.TabStops {
		want := i%8 == 0
		if stop != want {
			t.Errorf("TabStop[%d] = %v, want %v", i, stop, want)
		}
	}
}

func TestNewScreen_minDimensions(t *testing.T) {
	s := NewScreen(0, -5)
	if s.Rows != 1 || s.Cols != 1 {
		t.Errorf("dims = %d×%d, want 1×1", s.Rows, s.Cols)
	}
}

func TestScreen_Resize_shrink(t *testing.T) {
	s := NewScreen(24, 80)
	s.CurRow = 20
	s.CurCol = 70
	s.Resize(12, 40)
	if s.Rows != 12 || s.Cols != 40 {
		t.Fatalf("dims = %d×%d, want 12×40", s.Rows, s.Cols)
	}
	if s.CurRow != 11 {
		t.Errorf("CurRow = %d, want 11 (clamped)", s.CurRow)
	}
	if s.CurCol != 39 {
		t.Errorf("CurCol = %d, want 39 (clamped)", s.CurCol)
	}
}

func TestScreen_Resize_grow(t *testing.T) {
	s := NewScreen(12, 40)
	s.Resize(24, 80)
	if len(s.Cells) != 24 {
		t.Fatalf("rows = %d, want 24", len(s.Cells))
	}
	for _, row := range s.Cells {
		if len(row) != 80 {
			t.Fatalf("cols = %d, want 80", len(row))
		}
	}
	if len(s.TabStops) != 80 {
		t.Fatalf("TabStops len = %d, want 80", len(s.TabStops))
	}
	if !s.TabStops[40] {
		t.Error("TabStops[40] should be set after grow")
	}
}

func TestScreen_Resize_resetsScrollRegion(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 5
	s.ScrollBot = 20
	s.Resize(24, 80)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("scroll region = %d-%d, want 0-0 (reset)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_ScrollUp(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.ScrollUp(1)
	if s.Cells[0][0].Ch != 'B' {
		t.Errorf("row 0 = %c, want B", s.Cells[0][0].Ch)
	}
	if s.Cells[3][0].Ch != 'E' {
		t.Errorf("row 3 = %c, want E", s.Cells[3][0].Ch)
	}
	if s.Cells[4][0].Ch != ' ' {
		t.Errorf("row 4 = %c, want space (new blank)", s.Cells[4][0].Ch)
	}
}

func TestScreen_ScrollDown(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.ScrollDown(1)
	if s.Cells[0][0].Ch != ' ' {
		t.Errorf("row 0 = %c, want space (new blank)", s.Cells[0][0].Ch)
	}
	if s.Cells[1][0].Ch != 'A' {
		t.Errorf("row 1 = %c, want A", s.Cells[1][0].Ch)
	}
}

func TestScreen_LineFeed_scrollsAtBottom(t *testing.T) {
	s := NewScreen(3, 5)
	s.Cells[0][0].Ch = 'A'
	s.CurRow = 2
	s.LineFeed()
	if s.Cells[0][0].Ch != ' ' {
		t.Error("should have scrolled: row 0 not blank")
	}
	if s.CurRow != 2 {
		t.Errorf("CurRow = %d, want 2", s.CurRow)
	}
}

func TestScreen_EraseDisplay_mode0(t *testing.T) {
	s := NewScreen(3, 5)
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c].Ch = 'X'
		}
	}
	s.CurRow = 1
	s.CurCol = 2
	s.EraseDisplay(0)
	if s.Cells[0][0].Ch != 'X' {
		t.Error("row 0 should be untouched")
	}
	if s.Cells[1][1].Ch != 'X' {
		t.Error("cell before cursor should be untouched")
	}
	if s.Cells[1][2].Ch != ' ' {
		t.Error("cursor cell should be erased")
	}
	if s.Cells[2][0].Ch != ' ' {
		t.Error("row below should be erased")
	}
}

func TestScreen_EraseLine_mode2(t *testing.T) {
	s := NewScreen(3, 5)
	for c := range s.Cells[1] {
		s.Cells[1][c].Ch = 'X'
	}
	s.CurRow = 1
	s.EraseLine(2)
	for c, cell := range s.Cells[1] {
		if cell.Ch != ' ' {
			t.Errorf("cell[1][%d] = %c, want space", c, cell.Ch)
		}
	}
}

func TestScreen_InsertLines(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.CurRow = 1
	s.InsertLines(2)
	if s.Cells[1][0].Ch != ' ' || s.Cells[2][0].Ch != ' ' {
		t.Error("inserted lines should be blank")
	}
	if s.Cells[3][0].Ch != 'B' {
		t.Errorf("shifted line = %c, want B", s.Cells[3][0].Ch)
	}
}

func TestScreen_DeleteLines(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.CurRow = 1
	s.DeleteLines(2)
	if s.Cells[1][0].Ch != 'D' {
		t.Errorf("after delete, row 1 = %c, want D", s.Cells[1][0].Ch)
	}
	if s.Cells[3][0].Ch != ' ' || s.Cells[4][0].Ch != ' ' {
		t.Error("bottom rows should be blank after delete")
	}
}

func TestScreen_PutChar_ascii(t *testing.T) {
	s := NewScreen(3, 5)
	s.PutChar('A')
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("cell = %c, want A", s.Cells[0][0].Ch)
	}
	if s.CurCol != 1 {
		t.Errorf("CurCol = %d, want 1", s.CurCol)
	}
}

func TestScreen_PutChar_wide(t *testing.T) {
	s := NewScreen(3, 10)
	s.PutChar('\u6F22') // 漢 - width 2
	if s.Cells[0][0].Ch != '\u6F22' {
		t.Errorf("cell[0] = %c, want 漢", s.Cells[0][0].Ch)
	}
	if s.Cells[0][1].Ch != 0 {
		t.Errorf("cell[1] = %d, want 0 (placeholder)", s.Cells[0][1].Ch)
	}
	if s.CurCol != 2 {
		t.Errorf("CurCol = %d, want 2", s.CurCol)
	}
}

func TestScreen_PutChar_pendingWrap(t *testing.T) {
	s := NewScreen(3, 5)
	for i := range 5 {
		s.PutChar(rune('A' + i))
	}
	if !s.PendingWrap {
		t.Error("PendingWrap should be true at right margin")
	}
	// Next char should wrap to next line.
	s.PutChar('F')
	if s.CurRow != 1 {
		t.Errorf("CurRow = %d, want 1 (wrapped)", s.CurRow)
	}
	if s.Cells[1][0].Ch != 'F' {
		t.Errorf("wrapped cell = %c, want F", s.Cells[1][0].Ch)
	}
}

func TestScreen_PutChar_wideAtMargin(t *testing.T) {
	s := NewScreen(3, 5)
	// Fill up to col 4 (0-indexed), which is the last column.
	s.CurCol = 4
	s.PutChar('\u6F22') // 漢 needs 2 cols but only 1 left
	// Should pad with space at col 4, wrap, then place char on next line.
	if s.Cells[0][4].Ch != ' ' {
		t.Errorf("margin cell = %c, want space (pad)", s.Cells[0][4].Ch)
	}
	if s.Cells[1][0].Ch != '\u6F22' {
		t.Errorf("wrapped wide = %c, want 漢", s.Cells[1][0].Ch)
	}
	if s.CurCol != 2 {
		t.Errorf("CurCol = %d, want 2", s.CurCol)
	}
}

// ── T091: Screen.ScrollUp edge cases ───────────────────────────────

func TestScreen_ScrollUp_Zero(t *testing.T) {
	s := NewScreen(5, 3)
	s.Cells[0][0].Ch = 'A'
	s.ScrollUp(0)
	if s.Cells[0][0].Ch != 'A' {
		t.Error("ScrollUp(0) should be no-op")
	}
}

func TestScreen_ScrollUp_HugeN(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.ScrollUp(999)
	// All rows should be blank
	for r := range 5 {
		if s.Cells[r][0].Ch != ' ' {
			t.Errorf("row %d = %c, want space after huge ScrollUp", r, s.Cells[r][0].Ch)
		}
	}
}

func TestScreen_ScrollUp_OneRowRegion(t *testing.T) {
	s := NewScreen(5, 3)
	s.Cells[2][0].Ch = 'X'
	s.ScrollTop = 3 // 1-indexed: row 2 to row 2 (single row)
	s.ScrollBot = 3
	s.ScrollUp(1)
	// Single-row region: scroll up clears that one row
	if s.Cells[2][0].Ch != ' ' {
		t.Errorf("row 2 = %c, want space (single-row region scrolled)", s.Cells[2][0].Ch)
	}
}

func TestScreen_ScrollUp_NonDefaultRegion(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.ScrollTop = 2 // 1-indexed rows 2..4
	s.ScrollBot = 4
	s.ScrollUp(1)
	// Row 0 unchanged (outside region)
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0 = %c, want A", s.Cells[0][0].Ch)
	}
	// Row 1 (inside region, was 'C') shifts up to become row 1
	if s.Cells[1][0].Ch != 'C' {
		t.Errorf("row 1 = %c, want C", s.Cells[1][0].Ch)
	}
	// Row 3 (bottom of region) should be blank
	if s.Cells[3][0].Ch != ' ' {
		t.Errorf("row 3 = %c, want space", s.Cells[3][0].Ch)
	}
	// Row 4 unchanged (outside region)
	if s.Cells[4][0].Ch != 'E' {
		t.Errorf("row 4 = %c, want E", s.Cells[4][0].Ch)
	}
}

func TestScreen_ScrollUp_WithCurrentAttr(t *testing.T) {
	s := NewScreen(3, 3)
	for r := range 3 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.CurAttr = Attr{Bold: true}
	s.ScrollUp(1)
	// New blank row at bottom should carry CurAttr
	if !s.Cells[2][0].Attr.Bold {
		t.Error("new blank row should have Bold attr from CurAttr")
	}
	if s.Cells[2][0].Ch != ' ' {
		t.Errorf("new blank row ch = %c, want space", s.Cells[2][0].Ch)
	}
}

// ── T091: Erase edge cases ─────────────────────────────────────────

func TestScreen_EraseChars_Huge(t *testing.T) {
	s := NewScreen(3, 5)
	for c := range 5 {
		s.Cells[0][c].Ch = 'X'
	}
	s.CurRow = 0
	s.CurCol = 2
	// EraseChars(999) should not panic on 5-col screen
	s.EraseChars(999)
	if s.Cells[0][0].Ch != 'X' || s.Cells[0][1].Ch != 'X' {
		t.Error("chars before cursor should be untouched")
	}
	for c := 2; c < 5; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Errorf("cell[0][%d] = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
}

func TestScreen_ReverseIndex_AtTopOfRegion(t *testing.T) {
	s := NewScreen(5, 3)
	for r := range 5 {
		s.Cells[r][0].Ch = rune('A' + r)
	}
	s.ScrollTop = 2 // 1-indexed rows 2..4
	s.ScrollBot = 4
	s.CurRow = 1 // top of region (0-indexed)
	s.ReverseIndex()
	// Should scroll down within region
	if s.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0 = %c, want A (outside region)", s.Cells[0][0].Ch)
	}
	if s.Cells[1][0].Ch != ' ' {
		t.Errorf("row 1 = %c, want space (new blank from scroll down)", s.Cells[1][0].Ch)
	}
	if s.Cells[4][0].Ch != 'E' {
		t.Errorf("row 4 = %c, want E (outside region)", s.Cells[4][0].Ch)
	}
}

// ── T122: Render idempotent (already tested, add consecutive test) ──

func TestScreen_Resize_ResetsScrollRegion(t *testing.T) {
	s := NewScreen(10, 40)
	s.ScrollTop = 3
	s.ScrollBot = 8
	s.Resize(10, 40)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("after resize: scroll=%d-%d, want 0-0", s.ScrollTop, s.ScrollBot)
	}
}
