package vt

import (
	"strings"
	"testing"
)

// --- RowWrapped flag tests ---------------------------------------------------

func TestRowWrapped_NewScreen(t *testing.T) {
	s := NewScreen(10, 20)
	if len(s.RowWrapped) != 10 {
		t.Fatalf("len(RowWrapped) = %d, want 10", len(s.RowWrapped))
	}
	for i, w := range s.RowWrapped {
		if w {
			t.Fatalf("RowWrapped[%d] = true on new screen, want false", i)
		}
	}
}

func TestRowWrapped_PutCharWrap(t *testing.T) {
	// Write 20 characters on a 20-column terminal — the 20th triggers
	// PendingWrap. Then write 1 more to resolve the wrap, setting
	// RowWrapped on the new row.
	s := NewScreen(5, 20)
	for i := 0; i < 20; i++ {
		s.PutChar('A')
	}
	if !s.PendingWrap {
		t.Fatal("expected PendingWrap after writing 20 chars on 20-col screen")
	}
	// Write one more to resolve the wrap.
	s.PutChar('B')
	// Now cursor is on row 1 (wrapped continuation), RowWrapped[1] = true.
	if s.CurRow != 1 {
		t.Fatalf("CurRow = %d, want 1 after wrap", s.CurRow)
	}
	if !s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = false, want true (wrapped continuation)")
	}
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true, want false (first row of logical line)")
	}
}

func TestRowWrapped_WideCharWrap(t *testing.T) {
	// Place cursor at col 19 on a 20-col screen, then write a wide char.
	// The wide char can't fit (needs 2 cols, only 1 left), so it wraps.
	s := NewScreen(5, 20)
	s.CurCol = 19
	s.PutChar('中') // wide char (width 2)
	if s.CurRow != 1 {
		t.Fatalf("CurRow = %d, want 1 after wide-char wrap", s.CurRow)
	}
	if !s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = false after wide-char wrap, want true")
	}
}

func TestRowWrapped_ScrollUp(t *testing.T) {
	s := NewScreen(5, 20)
	for r := 1; r < 5; r++ {
		s.RowWrapped[r] = true
	}
	s.ScrollUp(1)
	for r := 0; r < 4; r++ {
		if !s.RowWrapped[r] {
			t.Fatalf("RowWrapped[%d] = false after ScrollUp, want true (shifted)", r)
		}
	}
	if s.RowWrapped[4] {
		t.Fatal("RowWrapped[4] = true after ScrollUp, want false (new blank row)")
	}
}

func TestRowWrapped_ScrollDown(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[2] = true
	s.RowWrapped[3] = true
	s.RowWrapped[4] = true
	s.ScrollDown(1)
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true after ScrollDown, want false (new blank)")
	}
	// Rows shifted down: old [2]→[3], old [3]→[4], old [4] pushed off.
	if !s.RowWrapped[3] {
		t.Fatal("RowWrapped[3] = false after ScrollDown, want true (shifted from [2])")
	}
	if !s.RowWrapped[4] {
		t.Fatal("RowWrapped[4] = false after ScrollDown, want true (shifted from [3])")
	}
}

func TestRowWrapped_EraseDisplay2(t *testing.T) {
	s := NewScreen(5, 20)
	for i := range s.RowWrapped {
		s.RowWrapped[i] = true
	}
	s.EraseDisplay(2)
	for i, w := range s.RowWrapped {
		if w {
			t.Fatalf("RowWrapped[%d] = true after EraseDisplay(2), want false", i)
		}
	}
}

func TestRowWrapped_EraseLine2(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[2] = true
	s.CurRow = 2
	s.EraseLine(2)
	if s.RowWrapped[2] {
		t.Fatal("RowWrapped[2] = true after EraseLine(2), want false")
	}
}

func TestRowWrapped_InsertLines(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[1] = true
	s.RowWrapped[2] = true
	s.RowWrapped[3] = true
	s.CurRow = 1
	s.InsertLines(1)
	if s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = true after InsertLines, want false (new blank)")
	}
	if !s.RowWrapped[2] {
		t.Fatal("RowWrapped[2] = false after InsertLines, want true (shifted from [1])")
	}
}

func TestRowWrapped_DeleteLines(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[1] = true
	s.RowWrapped[2] = true
	s.RowWrapped[3] = true
	s.CurRow = 1
	s.DeleteLines(1)
	if !s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = false after DeleteLines, want true (shifted from [2])")
	}
	if s.RowWrapped[4] {
		t.Fatal("RowWrapped[4] = true after DeleteLines, want false (new blank)")
	}
}

func TestRowWrapped_Snapshot(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[2] = true
	snap := s.Snapshot()
	if !snap.RowWrapped[2] {
		t.Fatal("Snapshot RowWrapped[2] = false, want true")
	}
	snap.RowWrapped[2] = false
	if !s.RowWrapped[2] {
		t.Fatal("Modifying snapshot affected original RowWrapped")
	}
}

// --- Reflow on resize tests --------------------------------------------------

// helper: write n characters to a screen
func fillChars(s *Screen, ch rune, n int) {
	for i := 0; i < n; i++ {
		s.PutChar(ch)
	}
}

// cellRowStr returns the non-placeholder, non-NUL rune content of a row.
func cellRowStr(s *Screen, r int) string {
	if r < 0 || r >= s.Rows {
		return ""
	}
	var b strings.Builder
	for _, c := range s.Cells[r] {
		if c.SecondHalf {
			continue
		}
		if c.Ch != 0 && c.Ch != ' ' {
			b.WriteRune(c.Ch)
		}
	}
	return b.String()
}

func TestReflow_WiderUnwrap(t *testing.T) {
	// Write 30 chars on a 10-col screen, producing 3 wrapped rows.
	// Then resize to 15 cols — should unwrap to 2 rows.
	s := NewScreen(5, 10)
	s.ReflowOnResize = true
	for i := 0; i < 30; i++ {
		s.PutChar(rune('A' + i%26))
	}
	// Before resize: rows 1,2 are wrapped (RowWrapped=true).
	if !s.RowWrapped[1] || !s.RowWrapped[2] {
		t.Fatalf("pre-reflow: RowWrapped[1]=%v [2]=%v, want both true", s.RowWrapped[1], s.RowWrapped[2])
	}
	s.Resize(5, 15)
	// After reflow: 30 chars at 15 cols = 2 rows.
	// Row 0: first 15 chars, RowWrapped=false
	// Row 1: next 15 chars, RowWrapped=true (continuation)
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true, want false (first row of logical line)")
	}
	if !s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = false, want true (continuation)")
	}
}

func TestReflow_WiderFullUnwrap(t *testing.T) {
	// Write 15 chars on a 10-col screen (1.5 rows), then resize to 20.
	// All 15 chars should fit on a single row.
	s := NewScreen(5, 10)
	s.ReflowOnResize = true
	for i := 0; i < 15; i++ {
		s.PutChar(rune('A' + i))
	}
	s.Resize(5, 20)
	// Single row with 15 chars, no wrap.
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true, want false (fits on one row)")
	}
	content := cellRowStr(s, 0)
	if content != "ABCDEFGHIJKLMNO" {
		t.Fatalf("row 0 content = %q, want %q", content, "ABCDEFGHIJKLMNO")
	}
}

func TestReflow_Narrower(t *testing.T) {
	// Write 15 chars on a 3-row, 20-col screen, then resize to 10 cols.
	// The 15-char line wraps from 1 row to 2 rows. 2 blank rows stay as
	// 1 row each. Total: 2 + 2 = 4 rows. Screen is 3, so 1 overflow.
	// The first half of the content line (ABCDEFGHIJ) goes to scrollback;
	// the visible screen starts with KLMNO (continuation row).
	s := NewScreen(3, 20)
	s.ReflowOnResize = true
	for i := 0; i < 15; i++ {
		s.PutChar(rune('A' + i))
	}
	s.Resize(3, 10)
	// After reflow: empty rows produce no output, so just 2 content rows.
	// 2 rows fit in 3-row screen — no overflow.
	if s.ScrollbackLen != 0 {
		t.Fatalf("ScrollbackLen = %d, want 0", s.ScrollbackLen)
	}
	// Row 0 = ABCDEFGHIJ (first row of content), Row 1 = KLMNO (continuation)
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true, want false (first row)")
	}
	if !s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = false, want true (continuation)")
	}
	r0 := cellRowStr(s, 0)
	if r0 != "ABCDEFGHIJ" {
		t.Fatalf("row 0 = %q, want %q", r0, "ABCDEFGHIJ")
	}
	r1 := cellRowStr(s, 1)
	if r1 != "KLMNO" {
		t.Fatalf("row 1 = %q, want %q", r1, "KLMNO")
	}
}

func TestReflow_SameWidth(t *testing.T) {
	s := NewScreen(5, 20)
	s.ReflowOnResize = true
	for i := 0; i < 15; i++ {
		s.PutChar(rune('A' + i))
	}
	s.Resize(5, 20)
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true after same-width resize, want false")
	}
}

func TestReflow_AlternateScreenNoReflow(t *testing.T) {
	s := NewScreen(5, 20)
	s.ReflowOnResize = false
	for i := 0; i < 40; i++ {
		s.PutChar(rune('A' + i%26))
	}
	if !s.RowWrapped[1] {
		t.Fatal("pre-resize: RowWrapped[1] should be true")
	}
	s.Resize(5, 10)
	if len(s.RowWrapped) != 5 {
		t.Fatalf("len(RowWrapped) = %d, want 5", len(s.RowWrapped))
	}
	// Alternate screen should NOT reflow — content is truncated, not rewrapped.
}

func TestReflow_ScrollbackOverflow(t *testing.T) {
	// 5-row terminal, 20 cols. Fill 3 rows with 20-char content each.
	// Resize to 10 cols: each line becomes 2 rows = 6 rows total.
	// 1 row overflows to scrollback.
	s := NewScreen(5, 20)
	s.ReflowOnResize = true
	// Write 3 independent 20-char lines.
	for line := 0; line < 3; line++ {
		for i := 0; i < 20; i++ {
			s.PutChar(rune('A' + line))
		}
		s.CurCol = 0
		if s.CurRow < s.Rows-1 {
			s.CurRow++
		}
		s.PendingWrap = false
	}
	s.Resize(5, 10)
	// 3 content lines × 2 rows each = 6, plus 2 blank rows = 8 rows total.
	// Screen is 5, so 3 overflow.
	if s.ScrollbackLen != 2 {
		t.Fatalf("ScrollbackLen = %d, want 2", s.ScrollbackLen)
	}
}

func TestReflow_CursorTrackingWider(t *testing.T) {
	// Write 15 chars on 10-col screen, cursor at col 4 row 1, then
	// resize to 20 cols. Cursor should track to col 14 row 0.
	s := NewScreen(5, 10)
	s.ReflowOnResize = true
	for i := 0; i < 15; i++ {
		s.PutChar(rune('A' + i))
	}
	// Cursor at (1, 4) — the 'E' (0-indexed: A=0, B=1, ..., E=4, but
	// it's offset by 10 from the start of the logical line = position 14).
	s.Resize(10, 20)
	if s.CurRow != 0 {
		t.Fatalf("CurRow = %d, want 0", s.CurRow)
	}
	// Cursor was after 'O' (15th char, 0-indexed col 14), so offset=15 in logical line.
	if s.CurCol != 15 {
		t.Fatalf("CurCol = %d, want 15", s.CurCol)
	}
}

func TestReflow_CursorTrackingNarrower(t *testing.T) {
	// Write 7 chars on a 3-row, 20-col screen, cursor at col 6 row 0,
	// then resize to 5 cols. Content fits: 7 chars at 5 cols = 2 rows.
	// 2 content rows + 2 blank rows = 4. Screen is 3, so 1 overflow.
	// The first row (ABCDE) overflows. Cursor at offset 6 ('G') maps to
	// row 1 col 1 in the re-broken line (offset 5-9). After overflow,
	// visible row 0 = "FG" (continuation), cursor at col 1.
	s := NewScreen(3, 20)
	s.ReflowOnResize = true
	for i := 0; i < 7; i++ {
		s.PutChar(rune('A' + i))
	}
	// Cursor at (0, 7) — one past 'G'. Offset in logical line = 7.
	s.Resize(3, 5)
	// Cursor offset 7 maps to row 1 col 2 in the re-broken line.
	// No overflow, so cursor is at row 1 col 2.
	if s.CurRow != 1 {
		t.Fatalf("CurRow = %d, want 1", s.CurRow)
	}
	if s.CurCol != 2 {
		t.Fatalf("CurCol = %d, want 2", s.CurCol)
	}
}

func TestReflow_MultipleLogicalLines(t *testing.T) {
	// Two separate logical lines, each fitting within the new width.
	s := NewScreen(5, 10)
	s.ReflowOnResize = true
	for _, ch := range "HELLO" {
		s.PutChar(ch)
	}
	s.CurCol = 0
	s.LineFeed()
	s.PendingWrap = false
	for _, ch := range "WORLD" {
		s.PutChar(ch)
	}
	s.Resize(5, 5)
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true, want false")
	}
	if s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = true, want false (separate logical line)")
	}
	r0 := cellRowStr(s, 0)
	r1 := cellRowStr(s, 1)
	if r0 != "HELLO" {
		t.Fatalf("row 0 = %q, want %q", r0, "HELLO")
	}
	if r1 != "WORLD" {
		t.Fatalf("row 1 = %q, want %q", r1, "WORLD")
	}
}

func TestReflow_PadWithBlankRows(t *testing.T) {
	// 5-row terminal, 4 cols, with 1 line of 4-char text "ABCD".
	// Resize to 2 cols — the line becomes 2 rows (AB, CD).
	// 2 content rows + 4 blank padding = 6. Screen is 5, so 1 overflow.
	// First content row (AB) overflows; visible starts with CD.
	s := NewScreen(5, 4)
	s.ReflowOnResize = true
	for _, ch := range "ABCD" {
		s.PutChar(ch)
	}
	s.Resize(5, 2)
	// 4 chars at 2 cols = 2 rows. No empty rows emitted. No overflow.
	if s.ScrollbackLen != 0 {
		t.Fatalf("ScrollbackLen = %d, want 0", s.ScrollbackLen)
	}
	// Row 0 = AB (first row), Row 1 = CD (continuation).
	if s.RowWrapped[0] {
		t.Fatal("RowWrapped[0] = true, want false (first row)")
	}
	if !s.RowWrapped[1] {
		t.Fatal("RowWrapped[1] = false, want true (continuation)")
	}
}

func TestReflow_VTermPrimaryReflows(t *testing.T) {
	v := NewVTerm(5, 20)
	for i := 0; i < 30; i++ {
		v.Write([]byte{byte('A' + i%26)})
	}
	v.Resize(5, 10)
	p := v.primary
	// Primary screen should have reflowed.
	if len(p.RowWrapped) != p.Rows {
		t.Fatalf("len(RowWrapped) = %d, Rows = %d", len(p.RowWrapped), p.Rows)
	}
	// There should be a wrapped row since 30 chars at 10 cols = 3 rows
	// for the first logical line.
	hasWrapped := false
	for _, w := range p.RowWrapped {
		if w {
			hasWrapped = true
		}
	}
	if !hasWrapped {
		t.Fatal("no wrapped rows found after VTerm resize with reflow")
	}
}

func TestReflow_RowWrappedLenMatchRows(t *testing.T) {
	s := NewScreen(5, 20)
	s.ReflowOnResize = true
	s.Resize(10, 40)
	if len(s.RowWrapped) != s.Rows {
		t.Fatalf("len(RowWrapped) = %d, Rows = %d after grow", len(s.RowWrapped), s.Rows)
	}
	s.Resize(3, 5)
	if len(s.RowWrapped) != s.Rows {
		t.Fatalf("len(RowWrapped) = %d, Rows = %d after shrink", len(s.RowWrapped), s.Rows)
	}
}

func TestReflow_EraseDisplay0_ClearsWrapFlags(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[3] = true
	s.RowWrapped[4] = true
	s.CurRow = 2
	s.EraseDisplay(0)
	if s.RowWrapped[2] || s.RowWrapped[3] || s.RowWrapped[4] {
		t.Fatal("EraseDisplay(0) should clear RowWrapped for current and below")
	}
}

func TestReflow_EraseDisplay1_ClearsWrapFlags(t *testing.T) {
	s := NewScreen(5, 20)
	s.RowWrapped[0] = true
	s.RowWrapped[1] = true
	s.CurRow = 2
	s.EraseDisplay(1)
	if s.RowWrapped[0] || s.RowWrapped[1] || s.RowWrapped[2] {
		t.Fatal("EraseDisplay(1) should clear RowWrapped for above and current")
	}
}

func TestReflow_BlankRowsDontMultiply(t *testing.T) {
	// Blank rows should not expand into multiple blank rows on narrow resize.
	// This is the key behavior enabled by trimRightCells.
	s := NewScreen(5, 20)
	s.ReflowOnResize = true
	// Write 10 chars on row 0 only; rows 1-4 are blank.
	for i := 0; i < 10; i++ {
		s.PutChar(rune('A' + i))
	}
	s.Resize(5, 10)
	// 10 chars at 10 cols = 1 row. 4 blank rows stay as 1 row each.
	// Total: 5 rows, no overflow.
	if s.ScrollbackLen != 0 {
		t.Fatalf("ScrollbackLen = %d, want 0 (no overflow expected)", s.ScrollbackLen)
	}
	// Content on row 0.
	r0 := cellRowStr(s, 0)
	if r0 != "ABCDEFGHIJ" {
		t.Fatalf("row 0 = %q, want %q", r0, "ABCDEFGHIJ")
	}
}

func TestReflow_WideCharBoundaryRepair(t *testing.T) {
	// Wide char at the edge of a rebreak boundary should not produce
	// orphaned SecondHalf placeholders.
	s := NewScreen(5, 4)
	s.ReflowOnResize = true
	s.PutChar('A')
	s.PutChar('中') // width 2, fills cols 1-2
	s.PutChar('B')
	// Row 0: A 中 中 B (4 cols, all fit)
	s.Resize(5, 2)
	// At 2 cols: "A中" becomes 2 cells (A + first-half of 中), but
	// 中 straddles the boundary. Repair blanks A and moves 中 to next row.
	// Row 0 should be " " (blanked A) or "A " (A + space) depending on repair.
	// Row 1 should start with "中" (the wide char).
	// The key invariant: no orphaned SecondHalf cells.
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			if s.Cells[r][c].SecondHalf {
				// Verify preceding cell is a wide char first half.
				if c == 0 {
					t.Fatalf("row %d col 0 is orphaned SecondHalf", r)
				}
				if s.Cells[r][c-1].Ch == 0 || s.Cells[r][c-1].SecondHalf {
					t.Fatalf("row %d col %d: SecondHalf without valid first half", r, c)
				}
			}
		}
	}
}

func TestReflow_ResetPreservesReflowOnResize(t *testing.T) {
	v := NewVTerm(5, 20)
	// Trigger reset (ESC c).
	v.Write([]byte("\x1bc"))
	// Verify primary still has ReflowOnResize=true.
	if !v.primary.ReflowOnResize {
		t.Fatal("after reset: primary.ReflowOnResize = false, want true")
	}
	// Verify alternate still has MaxScrollback=0.
	if v.alternate.MaxScrollback != 0 {
		t.Fatalf("after reset: alternate.MaxScrollback = %d, want 0", v.alternate.MaxScrollback)
	}
}
