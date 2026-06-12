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

func TestScreen_Resize_preservesScrollRegion(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 5
	s.ScrollBot = 20
	s.Resize(24, 80)
	if s.ScrollTop != 5 || s.ScrollBot != 20 {
		t.Errorf("scroll region = %d-%d, want 5-20 (preserved)", s.ScrollTop, s.ScrollBot)
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

func TestScreen_InsertLines_OutsideScrollRegion(t *testing.T) {
	// Use VTerm + CSI handler for proper scroll region setup.
	v := NewVTerm(6, 3)
	scr := v.active
	for r := range 6 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	// Set scroll region to rows 3-5 (1-indexed: CSI 3;5 r).
	// ScrollRegion() converts to 0-indexed: top=2, bot=5.
	v.csi.Dispatch(scr, 'r', []int{3, 5}, false)
	top, bot := scr.ScrollRegion()
	if top != 2 || bot != 5 {
		t.Fatalf("scroll region = (%d,%d), want (2,5)", top, bot)
	}

	// Fill again (DECSTBM homes cursor and may clear).
	for r := range 6 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}

	// Cursor at row 0 (above scroll region which starts at row 2).
	scr.CurRow = 0
	scr.InsertLines(1)
	// Should be a no-op — nothing changes.
	for r := range 6 {
		if scr.Cells[r][0].Ch != rune('A'+r) {
			t.Errorf("row %d = %c, want %c (IL above scroll region should be no-op)", r, scr.Cells[r][0].Ch, rune('A'+r))
		}
	}
	// Cursor at row 5 (at bot, which is exclusive boundary).
	scr.CurRow = 5
	scr.InsertLines(1)
	for r := range 6 {
		if scr.Cells[r][0].Ch != rune('A'+r) {
			t.Errorf("row %d = %c, want %c (IL at ScrollBot should be no-op)", r, scr.Cells[r][0].Ch, rune('A'+r))
		}
	}
}

func TestScreen_DeleteLines_OutsideScrollRegion(t *testing.T) {
	// Use VTerm + CSI handler for proper scroll region setup.
	v := NewVTerm(6, 3)
	scr := v.active
	for r := range 6 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	// Set scroll region to rows 3-5 (1-indexed).
	v.csi.Dispatch(scr, 'r', []int{3, 5}, false)
	top, bot := scr.ScrollRegion()
	if top != 2 || bot != 5 {
		t.Fatalf("scroll region = (%d,%d), want (2,5)", top, bot)
	}

	// Fill again (DECSTBM homes cursor and may clear).
	for r := range 6 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}

	// Cursor at row 0 (above scroll region).
	scr.CurRow = 0
	scr.DeleteLines(1)
	// Should be a no-op.
	for r := range 6 {
		if scr.Cells[r][0].Ch != rune('A'+r) {
			t.Errorf("row %d = %c, want %c (DL above scroll region should be no-op)", r, scr.Cells[r][0].Ch, rune('A'+r))
		}
	}
	// Cursor at row 5 (at ScrollBot, which is exclusive).
	scr.CurRow = 5
	scr.DeleteLines(1)
	for r := range 6 {
		if scr.Cells[r][0].Ch != rune('A'+r) {
			t.Errorf("row %d = %c, want %c (DL at ScrollBot should be no-op)", r, scr.Cells[r][0].Ch, rune('A'+r))
		}
	}
}

func TestScreen_InsertLines_WithScrollRegion(t *testing.T) {
	// 8-row screen with scroll region rows 3-6 (1-indexed), i.e. rows 2-6 (0-indexed).
	v := NewVTerm(8, 3)
	scr := v.active
	for r := range 8 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	// Set scroll region: CSI 3;6 r => 0-indexed top=2, bot=6.
	v.csi.Dispatch(scr, 'r', []int{3, 6}, false)
	top, bot := scr.ScrollRegion()
	if top != 2 || bot != 6 {
		t.Fatalf("scroll region = (%d,%d), want (2,6)", top, bot)
	}

	// Fill again and set cursor inside scroll region.
	for r := range 8 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	scr.CurRow = 3

	scr.InsertLines(1)

	// Row 0-1: unchanged (A, B).
	if scr.Cells[0][0].Ch != 'A' || scr.Cells[1][0].Ch != 'B' {
		t.Error("rows above scroll region should be unchanged")
	}
	// Row 3: blank (inserted).
	if scr.Cells[3][0].Ch != ' ' {
		t.Errorf("inserted row = %c, want blank", scr.Cells[3][0].Ch)
	}
	// Row 4: old row 3 (D).
	if scr.Cells[4][0].Ch != 'D' {
		t.Errorf("shifted row 4 = %c, want D", scr.Cells[4][0].Ch)
	}
	// Row 5: old row 4 (E). Row 5 is the last in the scroll region.
	if scr.Cells[5][0].Ch != 'E' {
		t.Errorf("shifted row 5 = %c, want E", scr.Cells[5][0].Ch)
	}
	// Row 6-7: unchanged (G, H) — outside scroll region.
	if scr.Cells[6][0].Ch != 'G' || scr.Cells[7][0].Ch != 'H' {
		t.Error("rows below scroll region should be unchanged")
	}
	// Old row 5 (F) was pushed out of the scroll region (scrolled off).
}

func TestScreen_DeleteLines_WithScrollRegion(t *testing.T) {
	// 8-row screen with scroll region rows 3-6 (1-indexed), i.e. rows 2-6 (0-indexed).
	v := NewVTerm(8, 3)
	scr := v.active
	for r := range 8 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	// Set scroll region: CSI 3;6 r => 0-indexed top=2, bot=6.
	v.csi.Dispatch(scr, 'r', []int{3, 6}, false)
	top, bot := scr.ScrollRegion()
	if top != 2 || bot != 6 {
		t.Fatalf("scroll region = (%d,%d), want (2,6)", top, bot)
	}

	// Fill again and set cursor inside scroll region.
	for r := range 8 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	scr.CurRow = 3

	scr.DeleteLines(1)

	// Row 0-1: unchanged.
	if scr.Cells[0][0].Ch != 'A' || scr.Cells[1][0].Ch != 'B' {
		t.Error("rows above scroll region should be unchanged")
	}
	// Row 3: old row 4 (E).
	if scr.Cells[3][0].Ch != 'E' {
		t.Errorf("after delete, row 3 = %c, want E", scr.Cells[3][0].Ch)
	}
	// Row 4: old row 5 (F).
	if scr.Cells[4][0].Ch != 'F' {
		t.Errorf("after delete, row 4 = %c, want F", scr.Cells[4][0].Ch)
	}
	// Row 5: blank (bottom of scroll region fills with blank).
	if scr.Cells[5][0].Ch != ' ' {
		t.Errorf("bottom row = %c, want blank", scr.Cells[5][0].Ch)
	}
	// Row 6-7: unchanged.
	if scr.Cells[6][0].Ch != 'G' || scr.Cells[7][0].Ch != 'H' {
		t.Error("rows below scroll region should be unchanged")
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

func TestScreen_Resize_ClampsScrollBotOnShrink(t *testing.T) {
	s := NewScreen(10, 40)
	s.ScrollTop = 3
	s.ScrollBot = 8
	// Shrink to 5 rows — ScrollBot clamps to 5, ScrollTop=3 < 5, so region
	// is preserved as 3-5.
	s.Resize(5, 40)
	if s.ScrollTop != 3 || s.ScrollBot != 5 {
		t.Errorf("after shrink: scroll=%d-%d, want 3-5 (clamped bot)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_Resize_ClampsScrollBot(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 5
	s.ScrollBot = 20
	// Shrink to 15 rows — ScrollBot should be clamped to 15.
	s.Resize(15, 80)
	if s.ScrollTop != 5 || s.ScrollBot != 15 {
		t.Errorf("scroll region = %d-%d, want 5-15 (clamped bot)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_Resize_PreservesOnGrow(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 5
	s.ScrollBot = 20
	// Grow to 48 rows — scroll region should be unchanged.
	s.Resize(48, 80)
	if s.ScrollTop != 5 || s.ScrollBot != 20 {
		t.Errorf("scroll region = %d-%d, want 5-20 (preserved on grow)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_Resize_ResetsWhenRegionInverted(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 15
	s.ScrollBot = 10
	// Region is already inverted — resize should reset to defaults.
	s.Resize(24, 80)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("scroll region = %d-%d, want 0-0 (inverted)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_Resize_ResetsWhenTopExceedsBotAfterClamp(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 20
	s.ScrollBot = 22
	// Shrink to 15 rows — ScrollBot clamps to 15, but ScrollTop=20 > 15, so reset.
	s.Resize(15, 80)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("scroll region = %d-%d, want 0-0 (top > clamped bot)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_Resize_ScrollRegionAtBottomEdge(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 5
	s.ScrollBot = 24 // exactly at bottom edge
	// Shrink to 20 rows — ScrollBot clamps to 20, ScrollTop=5 < 20, preserved.
	s.Resize(20, 80)
	if s.ScrollTop != 5 || s.ScrollBot != 20 {
		t.Errorf("scroll region = %d-%d, want 5-20 (clamped to edge)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_Resize_ScrollRegionCollapseToZero(t *testing.T) {
	s := NewScreen(24, 80)
	s.ScrollTop = 5
	s.ScrollBot = 6 // very small region
	// Shrink to 3 rows — ScrollBot clamps to 3, ScrollTop=5 > 3, reset.
	s.Resize(3, 80)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("scroll region = %d-%d, want 0-0 (region collapsed)", s.ScrollTop, s.ScrollBot)
	}
}
