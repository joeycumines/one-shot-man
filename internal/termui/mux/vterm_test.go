package mux

import (
	"fmt"
	"strings"
	"testing"
)

// Test helpers that access unexported VTerm fields directly.
// These replace the formerly-exported accessor methods which were removed
// because they had no production callers (deadcode).

func vtRows(vt *VTerm) int { return vt.rows }
func vtCols(vt *VTerm) int { return vt.cols }

func vtCursorPos(vt *VTerm) (row, col int) {
	return vt.active.curRow, vt.active.curCol
}

func vtIsAltScreen(vt *VTerm) bool {
	return vt.active == vt.alternate
}

func vtCellAt(vt *VTerm, row, col int) (rune, attr) {
	if row < 0 || row >= vt.rows || col < 0 || col >= vt.cols {
		return ' ', attr{}
	}
	c := vt.active.cells[row][col]
	return c.ch, c.attr
}

func TestVTerm_New(t *testing.T) {
	vt := NewVTerm(24, 80)
	if vtRows(vt) != 24 || vtCols(vt) != 80 {
		t.Fatalf("expected 24x80, got %dx%d", vtRows(vt), vtCols(vt))
	}
	row, col := vtCursorPos(vt)
	if row != 0 || col != 0 {
		t.Fatalf("expected cursor at 0,0, got %d,%d", row, col)
	}
	// All cells should be spaces with default attrs.
	ch, a := vtCellAt(vt, 0, 0)
	if ch != ' ' {
		t.Fatalf("expected space, got %q", ch)
	}
	if a != (attr{}) {
		t.Fatalf("expected default attr, got %+v", a)
	}
}

func TestVTerm_New_MinDimensions(t *testing.T) {
	vt := NewVTerm(0, -5)
	if vtRows(vt) != 1 || vtCols(vt) != 1 {
		t.Fatalf("expected 1x1 minimum, got %dx%d", vtRows(vt), vtCols(vt))
	}
}

func TestVTerm_PlainText(t *testing.T) {
	vt := NewVTerm(24, 80)
	fmt.Fprint(vt, "Hello, World!")
	// Check characters were placed.
	expected := "Hello, World!"
	for i, ch := range expected {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("cell [0][%d] = %q, want %q", i, got, ch)
		}
	}
	// Cursor should be at (0, 13).
	row, col := vtCursorPos(vt)
	if row != 0 || col != 13 {
		t.Fatalf("cursor at %d,%d, want 0,13", row, col)
	}
}

func TestVTerm_LineWrap(t *testing.T) {
	vt := NewVTerm(3, 5)
	fmt.Fprint(vt, "ABCDEFGH")
	// "ABCDE" on row 0, "FGH" on row 1.
	for i, ch := range "ABCDE" {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("cell [0][%d] = %q, want %q", i, got, ch)
		}
	}
	for i, ch := range "FGH" {
		got, _ := vtCellAt(vt, 1, i)
		if got != ch {
			t.Fatalf("cell [1][%d] = %q, want %q", i, got, ch)
		}
	}
}

func TestVTerm_Newline(t *testing.T) {
	vt := NewVTerm(5, 10)
	// \n is pure LF — it moves down but does NOT reset column.
	// Use \r\n for full CR+LF behavior.
	fmt.Fprint(vt, "ABC\r\nDEF")
	// ABC on row 0, DEF on row 1 col 0.
	for i, ch := range "ABC" {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("cell [0][%d]: got %q, want %q", i, got, ch)
		}
	}
	for i, ch := range "DEF" {
		got, _ := vtCellAt(vt, 1, i)
		if got != ch {
			t.Fatalf("cell [1][%d]: got %q, want %q", i, got, ch)
		}
	}
}

func TestVTerm_CarriageReturn(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "AAAA\rBB")
	// BB should overwrite first two chars.
	expected := "BBAA"
	for i, ch := range expected {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("cell [0][%d]: got %q, want %q", i, got, ch)
		}
	}
}

func TestVTerm_Tab(t *testing.T) {
	vt := NewVTerm(3, 20)
	fmt.Fprint(vt, "A\tB")
	// A at col 0, tab moves to col 8, B at col 8.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("expected A at 0, got %q", got)
	}
	got, _ = vtCellAt(vt, 0, 8)
	if got != 'B' {
		t.Fatalf("expected B at 8, got %q", got)
	}
}

func TestVTerm_Backspace(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "AB\bC")
	// AB, then backspace moves to col 1, C overwrites B.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("expected A at 0, got %q", got)
	}
	got, _ = vtCellAt(vt, 0, 1)
	if got != 'C' {
		t.Fatalf("expected C at 1, got %q", got)
	}
}

func TestVTerm_CursorPosition(t *testing.T) {
	vt := NewVTerm(10, 20)
	// CUP to row 5, col 10 (1-indexed: 5;10H).
	fmt.Fprint(vt, "\x1b[5;10H")
	row, col := vtCursorPos(vt)
	if row != 4 || col != 9 {
		t.Fatalf("cursor at %d,%d, want 4,9", row, col)
	}
	// CUP with no params → home (1,1 → 0,0).
	fmt.Fprint(vt, "\x1b[H")
	row, col = vtCursorPos(vt)
	if row != 0 || col != 0 {
		t.Fatalf("cursor at %d,%d, want 0,0", row, col)
	}
}

func TestVTerm_CursorMovement(t *testing.T) {
	vt := NewVTerm(10, 20)
	// Start at 5,5.
	fmt.Fprint(vt, "\x1b[6;6H") // 1-indexed
	// Move up 2.
	fmt.Fprint(vt, "\x1b[2A")
	row, col := vtCursorPos(vt)
	if row != 3 || col != 5 {
		t.Fatalf("after CUU: %d,%d, want 3,5", row, col)
	}
	// Move down 4.
	fmt.Fprint(vt, "\x1b[4B")
	row, _ = vtCursorPos(vt)
	if row != 7 {
		t.Fatalf("after CUD: row=%d, want 7", row)
	}
	// Move forward 3.
	fmt.Fprint(vt, "\x1b[3C")
	_, col = vtCursorPos(vt)
	if col != 8 {
		t.Fatalf("after CUF: col=%d, want 8", col)
	}
	// Move back 5.
	fmt.Fprint(vt, "\x1b[5D")
	_, col = vtCursorPos(vt)
	if col != 3 {
		t.Fatalf("after CUB: col=%d, want 3", col)
	}
}

func TestVTerm_CursorMovement_Clamp(t *testing.T) {
	vt := NewVTerm(5, 10)
	// Try to move up from row 0.
	fmt.Fprint(vt, "\x1b[99A")
	row, _ := vtCursorPos(vt)
	if row != 0 {
		t.Fatalf("CUU clamped: row=%d, want 0", row)
	}
	// Try to move down past last row.
	fmt.Fprint(vt, "\x1b[99B")
	row, _ = vtCursorPos(vt)
	if row != 4 {
		t.Fatalf("CUD clamped: row=%d, want 4", row)
	}
	// Try to move forward past last col.
	fmt.Fprint(vt, "\x1b[99C")
	_, col := vtCursorPos(vt)
	if col != 9 {
		t.Fatalf("CUF clamped: col=%d, want 9", col)
	}
	// Move back past col 0.
	fmt.Fprint(vt, "\x1b[99D")
	_, col = vtCursorPos(vt)
	if col != 0 {
		t.Fatalf("CUB clamped: col=%d, want 0", col)
	}
}

func TestVTerm_EraseDisplay(t *testing.T) {
	vt := NewVTerm(3, 5)
	// Fill with X.
	for r := 0; r < 3; r++ {
		fmt.Fprintf(vt, "\x1b[%d;1H", r+1)
		fmt.Fprint(vt, "XXXXX")
	}
	// Position at (1,2) [0-indexed].
	fmt.Fprint(vt, "\x1b[2;3H")
	// ED 0: erase from cursor to end.
	fmt.Fprint(vt, "\x1b[0J")
	// Row 0 should be untouched.
	for c := 0; c < 5; c++ {
		got, _ := vtCellAt(vt, 0, c)
		if got != 'X' {
			t.Fatalf("row 0 col %d: got %q, want X", c, got)
		}
	}
	// Row 1, cols 0-1 should still be X, cols 2-4 should be space.
	for c := 0; c < 2; c++ {
		got, _ := vtCellAt(vt, 1, c)
		if got != 'X' {
			t.Fatalf("row 1 col %d: got %q, want X", c, got)
		}
	}
	for c := 2; c < 5; c++ {
		got, _ := vtCellAt(vt, 1, c)
		if got != ' ' {
			t.Fatalf("row 1 col %d: got %q, want space", c, got)
		}
	}
	// Row 2 should be all spaces.
	for c := 0; c < 5; c++ {
		got, _ := vtCellAt(vt, 2, c)
		if got != ' ' {
			t.Fatalf("row 2 col %d: got %q, want space", c, got)
		}
	}
}

func TestVTerm_EraseDisplay_Full(t *testing.T) {
	vt := NewVTerm(3, 5)
	fmt.Fprint(vt, "AAAAA\nBBBBB\nCCCCC")
	fmt.Fprint(vt, "\x1b[2J") // ED 2: erase all.
	for r := 0; r < 3; r++ {
		for c := 0; c < 5; c++ {
			got, _ := vtCellAt(vt, r, c)
			if got != ' ' {
				t.Fatalf("cell [%d][%d]: got %q, want space", r, c, got)
			}
		}
	}
}

func TestVTerm_EraseLine(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "ABCDEFGHIJ")
	// Position at col 5.
	fmt.Fprint(vt, "\x1b[1;6H")
	// EL 0: erase from cursor to end of line.
	fmt.Fprint(vt, "\x1b[0K")
	for i, ch := range "ABCDE" {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("col %d: got %q, want %q", i, got, ch)
		}
	}
	for c := 5; c < 10; c++ {
		got, _ := vtCellAt(vt, 0, c)
		if got != ' ' {
			t.Fatalf("col %d: got %q, want space", c, got)
		}
	}
}

func TestVTerm_EraseLine_Full(t *testing.T) {
	vt := NewVTerm(3, 5)
	fmt.Fprint(vt, "ABCDE")
	fmt.Fprint(vt, "\x1b[1;3H") // col 2
	fmt.Fprint(vt, "\x1b[2K")   // EL 2: erase entire line.
	for c := 0; c < 5; c++ {
		got, _ := vtCellAt(vt, 0, c)
		if got != ' ' {
			t.Fatalf("col %d: got %q, want space", c, got)
		}
	}
}

func TestVTerm_SGR_Bold(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "\x1b[1mBold\x1b[0m")
	// Check B has bold attr.
	_, a := vtCellAt(vt, 0, 0)
	if !a.bold {
		t.Fatal("expected bold for 'B'")
	}
	// After reset, next char should not be bold.
	fmt.Fprint(vt, "X")
	_, a = vtCellAt(vt, 0, 4)
	if a.bold {
		t.Fatal("expected non-bold for 'X'")
	}
}

func TestVTerm_SGR_ForegroundColors(t *testing.T) {
	vt := NewVTerm(3, 40)
	// Standard colors 30-37.
	for i := 0; i < 8; i++ {
		fmt.Fprintf(vt, "\x1b[%dmX", 30+i)
	}
	for i := 0; i < 8; i++ {
		_, a := vtCellAt(vt, 0, i)
		if a.fg.kind != kind8 || a.fg.value != uint32(i) {
			t.Fatalf("col %d: fg kind=%v value=%d, want kind8 value=%d", i, a.fg.kind, a.fg.value, i)
		}
	}
}

func TestVTerm_SGR_256Color(t *testing.T) {
	vt := NewVTerm(3, 10)
	// 256-color foreground: ESC[38;5;42m
	fmt.Fprint(vt, "\x1b[38;5;42mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.fg.kind != kind256 || a.fg.value != 42 {
		t.Fatalf("256 fg: kind=%v value=%d, want kind256 value=42", a.fg.kind, a.fg.value)
	}
}

func TestVTerm_SGR_Truecolor(t *testing.T) {
	vt := NewVTerm(3, 10)
	// Truecolor foreground: ESC[38;2;255;128;0m
	fmt.Fprint(vt, "\x1b[38;2;255;128;0mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.fg.kind != kindRGB {
		t.Fatalf("truecolor fg kind=%v, want kindRGB", a.fg.kind)
	}
	r := (a.fg.value >> 16) & 0xFF
	g := (a.fg.value >> 8) & 0xFF
	b := a.fg.value & 0xFF
	if r != 255 || g != 128 || b != 0 {
		t.Fatalf("truecolor: R=%d G=%d B=%d, want 255,128,0", r, g, b)
	}
}

func TestVTerm_SGR_Background(t *testing.T) {
	vt := NewVTerm(3, 10)
	// 256-color background: ESC[48;5;100m
	fmt.Fprint(vt, "\x1b[48;5;100mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.bg.kind != kind256 || a.bg.value != 100 {
		t.Fatalf("256 bg: kind=%v value=%d, want kind256 value=100", a.bg.kind, a.bg.value)
	}
}

func TestVTerm_AltScreen(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "PRIMARY")
	if vtIsAltScreen(vt) {
		t.Fatal("should not be in alt screen initially")
	}
	// Enter alt-screen.
	fmt.Fprint(vt, "\x1b[?1049h")
	if !vtIsAltScreen(vt) {
		t.Fatal("should be in alt screen after DECSET 1049")
	}
	// Alt-screen should be blank.
	got, _ := vtCellAt(vt, 0, 0)
	if got != ' ' {
		t.Fatalf("alt screen cell: %q, want space", got)
	}
	// Write something to alt-screen.
	fmt.Fprint(vt, "ALT")
	got, _ = vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("alt screen write: %q, want A", got)
	}
	// Exit alt-screen.
	fmt.Fprint(vt, "\x1b[?1049l")
	if vtIsAltScreen(vt) {
		t.Fatal("should not be in alt screen after DECRST 1049")
	}
	// Primary screen should still have "PRIMARY".
	got, _ = vtCellAt(vt, 0, 0)
	if got != 'P' {
		t.Fatalf("primary screen after alt: %q, want P", got)
	}
}

func TestVTerm_ScrollRegion(t *testing.T) {
	vt := NewVTerm(5, 10)
	// Fill rows.
	for r := 0; r < 5; r++ {
		fmt.Fprintf(vt, "\x1b[%d;1H%d%d%d%d%d", r+1, r, r, r, r, r)
	}
	// Set scroll region to rows 2-4 (1-indexed).
	fmt.Fprint(vt, "\x1b[2;4r")
	// Cursor should be homed to 0,0 after DECSTBM.
	row, col := vtCursorPos(vt)
	if row != 0 || col != 0 {
		t.Fatalf("cursor after DECSTBM: %d,%d, want 0,0", row, col)
	}
	// Move to bottom of scroll region and do line feed to trigger scroll.
	fmt.Fprint(vt, "\x1b[4;1H") // row 3 (0-indexed), which is bottom of region
	fmt.Fprint(vt, "\n")
	// Row 0 should be unchanged (outside scroll region above).
	got, _ := vtCellAt(vt, 0, 0)
	if got != '0' {
		t.Fatalf("row 0 after scroll: %q, want '0'", got)
	}
	// Row 4 should be unchanged (outside scroll region below).
	got, _ = vtCellAt(vt, 4, 0)
	if got != '4' {
		t.Fatalf("row 4 after scroll: %q, want '4'", got)
	}
	// Row 1 (top of scroll region) should now have what was row 2.
	got, _ = vtCellAt(vt, 1, 0)
	if got != '2' {
		t.Fatalf("row 1 after scroll: %q, want '2'", got)
	}
}

func TestVTerm_ScrollUp(t *testing.T) {
	vt := NewVTerm(3, 5)
	// Use CUP to position and write each row explicitly.
	fmt.Fprint(vt, "\x1b[1;1HAAAAA")
	fmt.Fprint(vt, "\x1b[2;1HBBBBB")
	fmt.Fprint(vt, "\x1b[3;1HCCCCC")
	// SU 1: scroll up 1 line.
	fmt.Fprint(vt, "\x1b[1S")
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'B' {
		t.Fatalf("row 0 after SU: %q, want B", got)
	}
	got, _ = vtCellAt(vt, 1, 0)
	if got != 'C' {
		t.Fatalf("row 1 after SU: %q, want C", got)
	}
	// Row 2 should be blank.
	got, _ = vtCellAt(vt, 2, 0)
	if got != ' ' {
		t.Fatalf("row 2 after SU: %q, want space", got)
	}
}

func TestVTerm_ScrollDown(t *testing.T) {
	vt := NewVTerm(3, 5)
	// Use CUP to position rows explicitly.
	fmt.Fprint(vt, "\x1b[1;1HAAAAA")
	fmt.Fprint(vt, "\x1b[2;1HBBBBB")
	fmt.Fprint(vt, "\x1b[3;1HCCCCC")
	// SD 1: scroll down 1 line.
	fmt.Fprint(vt, "\x1b[1T")
	// Row 0 should be blank.
	got, _ := vtCellAt(vt, 0, 0)
	if got != ' ' {
		t.Fatalf("row 0 after SD: %q, want space", got)
	}
	got, _ = vtCellAt(vt, 1, 0)
	if got != 'A' {
		t.Fatalf("row 1 after SD: %q, want A", got)
	}
	got, _ = vtCellAt(vt, 2, 0)
	if got != 'B' {
		t.Fatalf("row 2 after SD: %q, want B", got)
	}
}

func TestVTerm_InsertLines(t *testing.T) {
	vt := NewVTerm(4, 5)
	for r := 0; r < 4; r++ {
		fmt.Fprintf(vt, "\x1b[%d;1H%c%c%c%c%c", r+1, 'A'+rune(r), 'A'+rune(r), 'A'+rune(r), 'A'+rune(r), 'A'+rune(r))
	}
	// Position at row 1 (0-indexed), insert 1 line.
	fmt.Fprint(vt, "\x1b[2;1H")
	fmt.Fprint(vt, "\x1b[1L")
	// Row 0: AAAAA (unchanged).
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("row 0: %q, want A", got)
	}
	// Row 1: blank (inserted).
	got, _ = vtCellAt(vt, 1, 0)
	if got != ' ' {
		t.Fatalf("row 1: %q, want space", got)
	}
	// Row 2: BBBBB (was row 1).
	got, _ = vtCellAt(vt, 2, 0)
	if got != 'B' {
		t.Fatalf("row 2: %q, want B", got)
	}
	// Row 3: CCCCC (was row 2). DDDDD was pushed off.
	got, _ = vtCellAt(vt, 3, 0)
	if got != 'C' {
		t.Fatalf("row 3: %q, want C", got)
	}
}

func TestVTerm_DeleteLines(t *testing.T) {
	vt := NewVTerm(4, 5)
	for r := 0; r < 4; r++ {
		fmt.Fprintf(vt, "\x1b[%d;1H%c%c%c%c%c", r+1, 'A'+rune(r), 'A'+rune(r), 'A'+rune(r), 'A'+rune(r), 'A'+rune(r))
	}
	// Position at row 1, delete 1 line.
	fmt.Fprint(vt, "\x1b[2;1H")
	fmt.Fprint(vt, "\x1b[1M")
	// Row 0: AAAAA.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("row 0: %q, want A", got)
	}
	// Row 1: CCCCC (was row 2).
	got, _ = vtCellAt(vt, 1, 0)
	if got != 'C' {
		t.Fatalf("row 1: %q, want C", got)
	}
	// Row 2: DDDDD (was row 3).
	got, _ = vtCellAt(vt, 2, 0)
	if got != 'D' {
		t.Fatalf("row 2: %q, want D", got)
	}
	// Row 3: blank (new).
	got, _ = vtCellAt(vt, 3, 0)
	if got != ' ' {
		t.Fatalf("row 3: %q, want space", got)
	}
}

func TestVTerm_DeleteChars(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "ABCDEFGHIJ")
	// Position at col 2, delete 3 chars.
	fmt.Fprint(vt, "\x1b[1;3H")
	fmt.Fprint(vt, "\x1b[3P")
	expected := "ABFGHIJ   "
	for i, ch := range expected {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("col %d: got %q, want %q", i, got, ch)
		}
	}
}

func TestVTerm_InsertChars(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "ABCDEFGHIJ")
	// Position at col 2, insert 2 chars.
	fmt.Fprint(vt, "\x1b[1;3H")
	fmt.Fprint(vt, "\x1b[2@")
	expected := "AB  CDEFGH"
	for i, ch := range expected {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("col %d: got %q, want %q", i, got, ch)
		}
	}
}

func TestVTerm_EraseChars(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "ABCDEFGHIJ")
	// Position at col 3, erase 4 chars.
	fmt.Fprint(vt, "\x1b[1;4H")
	fmt.Fprint(vt, "\x1b[4X")
	expected := "ABC    HIJ"
	for i, ch := range expected {
		got, _ := vtCellAt(vt, 0, i)
		if got != ch {
			t.Fatalf("col %d: got %q, want %q", i, got, ch)
		}
	}
}

func TestVTerm_ReverseIndex(t *testing.T) {
	vt := NewVTerm(3, 5)
	// Use CUP to position rows explicitly.
	fmt.Fprint(vt, "\x1b[1;1HAAAAA")
	fmt.Fprint(vt, "\x1b[2;1HBBBBB")
	fmt.Fprint(vt, "\x1b[3;1HCCCCC")
	// Move to top row.
	fmt.Fprint(vt, "\x1b[1;1H")
	// Reverse index at top → should scroll down.
	fmt.Fprint(vt, "\x1bM")
	// Row 0 should now be blank.
	got, _ := vtCellAt(vt, 0, 0)
	if got != ' ' {
		t.Fatalf("row 0 after RI: %q, want space", got)
	}
	// Row 1 should have what was row 0.
	got, _ = vtCellAt(vt, 1, 0)
	if got != 'A' {
		t.Fatalf("row 1 after RI: %q, want A", got)
	}
}

func TestVTerm_SaveRestoreCursor(t *testing.T) {
	vt := NewVTerm(10, 20)
	// Move to 3,7.
	fmt.Fprint(vt, "\x1b[4;8H")
	// Save cursor (DECSC).
	fmt.Fprint(vt, "\x1b7")
	// Move elsewhere.
	fmt.Fprint(vt, "\x1b[1;1H")
	// Restore cursor (DECRC).
	fmt.Fprint(vt, "\x1b8")
	row, col := vtCursorPos(vt)
	if row != 3 || col != 7 {
		t.Fatalf("cursor after restore: %d,%d, want 3,7", row, col)
	}
}

func TestVTerm_SaveRestoreCursor_CSI(t *testing.T) {
	vt := NewVTerm(10, 20)
	// Move to 5,12.
	fmt.Fprint(vt, "\x1b[6;13H")
	// Save cursor (CSI s).
	fmt.Fprint(vt, "\x1b[s")
	// Move elsewhere.
	fmt.Fprint(vt, "\x1b[1;1H")
	// Restore cursor (CSI u).
	fmt.Fprint(vt, "\x1b[u")
	row, col := vtCursorPos(vt)
	if row != 5 || col != 12 {
		t.Fatalf("cursor after CSI u: %d,%d, want 5,12", row, col)
	}
}

func TestVTerm_Resize(t *testing.T) {
	vt := NewVTerm(3, 5)
	fmt.Fprint(vt, "ABCDE")
	// Grow to 5x10.
	vt.Resize(5, 10)
	if vtRows(vt) != 5 || vtCols(vt) != 10 {
		t.Fatalf("after resize: %dx%d, want 5x10", vtRows(vt), vtCols(vt))
	}
	// Original content preserved.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("after resize: %q, want A", got)
	}
	got, _ = vtCellAt(vt, 0, 4)
	if got != 'E' {
		t.Fatalf("after resize: %q, want E", got)
	}
	// New columns should be space.
	got, _ = vtCellAt(vt, 0, 5)
	if got != ' ' {
		t.Fatalf("new col: %q, want space", got)
	}
}

func TestVTerm_Resize_Shrink(t *testing.T) {
	vt := NewVTerm(5, 10)
	// Put cursor at row 4.
	fmt.Fprint(vt, "\x1b[5;8H")
	// Shrink to 3x5.
	vt.Resize(3, 5)
	// Cursor should be clamped.
	row, col := vtCursorPos(vt)
	if row != 2 || col != 4 {
		t.Fatalf("cursor after shrink: %d,%d, want 2,4", row, col)
	}
}

func TestVTerm_CellAt_OutOfBounds(t *testing.T) {
	vt := NewVTerm(3, 5)
	ch, a := vtCellAt(vt, -1, 0)
	if ch != ' ' || a != (attr{}) {
		t.Fatal("out of bounds should return space/default")
	}
	ch, a = vtCellAt(vt, 0, 100)
	if ch != ' ' || a != (attr{}) {
		t.Fatal("out of bounds should return space/default")
	}
}

func TestVTerm_Render_BasicText(t *testing.T) {
	vt := NewVTerm(2, 5)
	fmt.Fprint(vt, "Hello")
	rendered := vt.Render()
	// Should contain "Hello" somewhere.
	if !strings.Contains(rendered, "Hello") {
		t.Fatalf("render missing 'Hello': %q", rendered)
	}
	// Should contain reset at start.
	if !strings.HasPrefix(rendered, "\x1b[0m") {
		t.Fatal("render should start with reset")
	}
}

func TestVTerm_Render_WithColor(t *testing.T) {
	vt := NewVTerm(1, 3)
	// Red foreground.
	fmt.Fprint(vt, "\x1b[31mABC")
	rendered := vt.Render()
	// Should contain the red SGR code.
	if !strings.Contains(rendered, "31") {
		t.Fatalf("render missing red color: %q", rendered)
	}
}

func TestVTerm_Render_CursorPosition(t *testing.T) {
	vt := NewVTerm(5, 10)
	fmt.Fprint(vt, "\x1b[3;7H")
	rendered := vt.Render()
	// Should end with cursor at (3,7) → ESC[3;7H.
	if !strings.HasSuffix(rendered, "\x1b[3;7H") {
		t.Fatalf("render should end with cursor position: %q", rendered)
	}
}

func TestVTerm_Write_IoWriter(t *testing.T) {
	vt := NewVTerm(3, 10)
	n, err := vt.Write([]byte("Test"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Write returned %d, want 4", n)
	}
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'T' {
		t.Fatalf("after Write: %q, want T", got)
	}
}

func TestVTerm_FullReset(t *testing.T) {
	vt := NewVTerm(3, 5)
	fmt.Fprint(vt, "XXXXX")
	fmt.Fprint(vt, "\x1b[?1049h") // alt-screen
	fmt.Fprint(vt, "YYYYY")
	// Full reset.
	fmt.Fprint(vt, "\x1bc")
	if vtIsAltScreen(vt) {
		t.Fatal("should not be in alt-screen after reset")
	}
	got, _ := vtCellAt(vt, 0, 0)
	if got != ' ' {
		t.Fatalf("after reset: %q, want space", got)
	}
}

func TestVTerm_OSC_Consumed(t *testing.T) {
	vt := NewVTerm(3, 10)
	// OSC terminated by BEL.
	fmt.Fprint(vt, "\x1b]0;Title\x07ABC")
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("after OSC: %q, want A", got)
	}
}

func TestVTerm_CNL_CPL(t *testing.T) {
	vt := NewVTerm(10, 20)
	// Position at row 3, col 10.
	fmt.Fprint(vt, "\x1b[4;11H")
	// CNL 2: cursor next line 2.
	fmt.Fprint(vt, "\x1b[2E")
	row, col := vtCursorPos(vt)
	if row != 5 || col != 0 {
		t.Fatalf("after CNL: %d,%d, want 5,0", row, col)
	}
	// Move to row 5, col 10.
	fmt.Fprint(vt, "\x1b[6;11H")
	// CPL 3: cursor previous line 3.
	fmt.Fprint(vt, "\x1b[3F")
	row, col = vtCursorPos(vt)
	if row != 2 || col != 0 {
		t.Fatalf("after CPL: %d,%d, want 2,0", row, col)
	}
}

func TestVTerm_CHA_VPA(t *testing.T) {
	vt := NewVTerm(10, 20)
	fmt.Fprint(vt, "\x1b[5;10H") // row 4, col 9
	// CHA (Cursor Horizontal Absolute) → col 15 (1-indexed).
	fmt.Fprint(vt, "\x1b[15G")
	_, col := vtCursorPos(vt)
	if col != 14 {
		t.Fatalf("after CHA: col=%d, want 14", col)
	}
	// VPA (Line Position Absolute) → row 8 (1-indexed).
	fmt.Fprint(vt, "\x1b[8d")
	row, _ := vtCursorPos(vt)
	if row != 7 {
		t.Fatalf("after VPA: row=%d, want 7", row)
	}
}

func TestVTerm_SGR_BrightColors(t *testing.T) {
	vt := NewVTerm(3, 10)
	// Bright red foreground (90).
	fmt.Fprint(vt, "\x1b[91mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.fg.kind != kind8 || a.fg.value != 9 { // 91-90+8=9
		t.Fatalf("bright red: kind=%v value=%d, want kind8 value=9", a.fg.kind, a.fg.value)
	}
	// Bright green background (102).
	fmt.Fprint(vt, "\x1b[102mY")
	_, a = vtCellAt(vt, 0, 1)
	if a.bg.kind != kind8 || a.bg.value != 10 { // 102-100+8=10
		t.Fatalf("bright green bg: kind=%v value=%d, want kind8 value=10", a.bg.kind, a.bg.value)
	}
}

func TestVTerm_SGR_AllAttributes(t *testing.T) {
	vt := NewVTerm(3, 20)
	// Set all attributes: bold, dim, italic, underline, blink, inverse, hidden, strikethrough.
	fmt.Fprint(vt, "\x1b[1;2;3;4;5;7;8;9mX")
	_, a := vtCellAt(vt, 0, 0)
	if !a.bold || !a.dim || !a.italic || !a.under || !a.blink || !a.inverse || !a.hidden || !a.strike {
		t.Fatalf("expected all attrs, got %+v", a)
	}
	// Reset specific attributes.
	fmt.Fprint(vt, "\x1b[22;23;24;27mY")
	_, a = vtCellAt(vt, 0, 1)
	if a.bold || a.dim || a.italic || a.under || a.inverse {
		t.Fatalf("after specific reset: %+v (bold/dim/italic/under/inverse should be off)", a)
	}
	if !a.blink || !a.hidden || !a.strike {
		t.Fatalf("blink/hidden/strike should still be on: %+v", a)
	}
}

func TestVTerm_ScrollAtBottom(t *testing.T) {
	vt := NewVTerm(3, 5)
	// Fill all 3 rows using CUP for precise positioning.
	fmt.Fprint(vt, "\x1b[1;1HAAAAA")
	fmt.Fprint(vt, "\x1b[2;1HBBBBB")
	fmt.Fprint(vt, "\x1b[3;1HCCCCC")
	// Move to last row, col 0 and do a line feed to trigger scroll.
	fmt.Fprint(vt, "\x1b[3;1H")
	fmt.Fprint(vt, "\n")
	// Row 0 should now be BBBBB.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'B' {
		t.Fatalf("row 0 after scroll: %q, want B", got)
	}
	// Row 1 should be CCCCC.
	got, _ = vtCellAt(vt, 1, 0)
	if got != 'C' {
		t.Fatalf("row 1 after scroll: %q, want C", got)
	}
	// Row 2 should be blank.
	got, _ = vtCellAt(vt, 2, 0)
	if got != ' ' {
		t.Fatalf("row 2 after scroll: %q, want space", got)
	}
}

func TestVTerm_BEL_Ignored(t *testing.T) {
	vt := NewVTerm(3, 10)
	fmt.Fprint(vt, "A\x07B")
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	if got0 != 'A' || got1 != 'B' {
		t.Fatalf("BEL should not affect output: %q %q", got0, got1)
	}
}
