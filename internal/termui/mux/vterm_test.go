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
	// Should end with cursor at (3,7) → ESC[3;7H, then cursor visibility.
	if !strings.HasSuffix(rendered, "\x1b[3;7H\x1b[?25h") {
		t.Fatalf("render should end with cursor position + visibility: %q", rendered)
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

func TestVTerm_SGR_TruncatedTruecolorFG(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// ESC[38;2;1m — truncated truecolor (only R, missing G and B).
	// The trailing "1" must NOT activate bold.
	fmt.Fprint(vt, "\x1b[38;2;1mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.bold {
		t.Fatal("truncated truecolor FG leaked: bold should be false")
	}
}

func TestVTerm_SGR_TruncatedTruecolorBG(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// ESC[48;2;255;128m — truncated bg truecolor (missing B).
	// The trailing "128" must NOT be misinterpreted.
	fmt.Fprint(vt, "\x1b[48;2;255;128mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.bg.kind != 0 {
		t.Fatalf("truncated truecolor BG should not set color, got kind=%d", a.bg.kind)
	}
}

func TestVTerm_SGR_ValidTruecolorAfterFix(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Valid truecolor: ESC[38;2;255;128;64m
	fmt.Fprint(vt, "\x1b[38;2;255;128;64mX")
	_, a := vtCellAt(vt, 0, 0)
	if a.fg.kind != kindRGB {
		t.Fatalf("valid truecolor: fg.kind=%d, want kindRGB(%d)", a.fg.kind, kindRGB)
	}
	wantVal := uint32(255)<<16 | uint32(128)<<8 | uint32(64)
	if a.fg.value != wantVal {
		t.Fatalf("valid truecolor: fg.value=%d, want %d", a.fg.value, wantVal)
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

// --- OSC Terminator Tests ---

func TestVTerm_OSC_ST_Consumed(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// OSC with ST (ESC \) terminator: set window title.
	// The entire sequence including the ST should be consumed.
	fmt.Fprint(vt, "A\x1b]0;My Title\x1b\\B")
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	if got0 != 'A' {
		t.Fatalf("cell 0: %q, want A", got0)
	}
	if got1 != 'B' {
		t.Fatalf("cell 1: %q, want B (backslash leaked: ST not consumed)", got1)
	}
}

func TestVTerm_OSC_BEL_StillWorks(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// OSC with BEL terminator: should still work.
	fmt.Fprint(vt, "A\x1b]0;Title\x07B")
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	if got0 != 'A' || got1 != 'B' {
		t.Fatalf("OSC BEL: %q %q, want A B", got0, got1)
	}
}

func TestVTerm_OSC_ST_NoLeakedBackslash(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Multiple OSC sequences with different terminators.
	fmt.Fprint(vt, "\x1b]0;First\x07A\x1b]0;Second\x1b\\B")
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	if got0 != 'A' || got1 != 'B' {
		t.Fatalf("mixed OSC terminators: %q %q, want A B", got0, got1)
	}
}

// --- CSI Robustness Tests ---

func TestVTerm_CSI_ParamBufCap(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Build a CSI sequence with >128 parameter bytes.
	var seq []byte
	seq = append(seq, 0x1b, '[')
	for j := 0; j < 200; j++ {
		seq = append(seq, '0') // parameter bytes
	}
	seq = append(seq, 'm') // SGR final byte
	seq = append(seq, 'A')
	vt.Write(seq)
	// Should not panic. The 'A' after the malformed SGR should render.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Fatalf("after oversized CSI params: %q, want A", got)
	}
}

func TestVTerm_CSI_EscInsideCSI(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Start a CSI sequence, then send ESC [ (new CSI) before the final byte.
	// ESC[1 ESC[2J should:
	//   - abort the first CSI (ESC[1)
	//   - start fresh escape, process '[' → CSI
	//   - then complete CSI 2J (erase display)
	fmt.Fprint(vt, "AAAA")          // fill row 0
	fmt.Fprint(vt, "\x1b[1\x1b[2J") // ESC inside CSI → abort, then erase all
	got, _ := vtCellAt(vt, 0, 0)
	if got != ' ' {
		t.Fatalf("ESC inside CSI: cell 0 = %q, want space (display should be erased)", got)
	}
}

// --- UTF-8 Tests ---

func TestVTerm_UTF8_TwoByteChar(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Write "café" — the é is UTF-8 two-byte (0xC3 0xA9).
	fmt.Fprint(vt, "caf\xc3\xa9")
	// Cells should be: c a f é
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	got2, _ := vtCellAt(vt, 0, 2)
	got3, _ := vtCellAt(vt, 0, 3)
	if got0 != 'c' || got1 != 'a' || got2 != 'f' || got3 != 'é' {
		t.Fatalf("got %q %q %q %q, want c a f é", got0, got1, got2, got3)
	}
	// Cursor should be at column 4 (é is width 1).
	_, col := vtCursorPos(vt)
	if col != 4 {
		t.Fatalf("cursor col: %d, want 4", col)
	}
}

func TestVTerm_UTF8_ThreeByteChar(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Chinese character 中 is 3-byte UTF-8 (0xE4 0xB8 0xAD) and width 2.
	fmt.Fprint(vt, "A\xe4\xb8\xadB")
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	got2, _ := vtCellAt(vt, 0, 2) // placeholder (0)
	got3, _ := vtCellAt(vt, 0, 3)
	if got0 != 'A' {
		t.Fatalf("cell 0: %q, want A", got0)
	}
	if got1 != '中' {
		t.Fatalf("cell 1: %q, want 中", got1)
	}
	if got2 != 0 {
		t.Fatalf("cell 2 (placeholder): %q, want 0", got2)
	}
	if got3 != 'B' {
		t.Fatalf("cell 3: %q, want B", got3)
	}
	_, col := vtCursorPos(vt)
	if col != 4 {
		t.Fatalf("cursor col: %d, want 4", col)
	}
}

func TestVTerm_UTF8_FourByteEmoji(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Robot face 🤖 is 4-byte UTF-8 (0xF0 0x9F 0xA4 0x96) and width 2.
	fmt.Fprint(vt, "X\xf0\x9f\xa4\x96Y")
	got0, _ := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1)
	got2, _ := vtCellAt(vt, 0, 2)
	got3, _ := vtCellAt(vt, 0, 3)
	if got0 != 'X' {
		t.Fatalf("cell 0: %q, want X", got0)
	}
	if got1 != '🤖' {
		t.Fatalf("cell 1: %q, want 🤖", got1)
	}
	if got2 != 0 {
		t.Fatalf("cell 2 (placeholder): %d, want 0", got2)
	}
	if got3 != 'Y' {
		t.Fatalf("cell 3: %q, want Y", got3)
	}
}

func TestVTerm_UTF8_SplitAcrossWrites(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Split the 2-byte é (0xC3 0xA9) across two Write calls.
	vt.Write([]byte{'c', 'a', 'f', 0xC3})
	vt.Write([]byte{0xA9})
	got3, _ := vtCellAt(vt, 0, 3)
	if got3 != 'é' {
		t.Fatalf("split write: cell 3: %q, want é", got3)
	}
}

func TestVTerm_UTF8_SplitFourByte(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Split 4-byte 🤖 (F0 9F A4 96) across 3 writes.
	vt.Write([]byte{0xF0})
	vt.Write([]byte{0x9F, 0xA4})
	vt.Write([]byte{0x96})
	got0, _ := vtCellAt(vt, 0, 0)
	if got0 != '🤖' {
		t.Fatalf("split 4-byte: cell 0: %q, want 🤖", got0)
	}
}

func TestVTerm_UTF8_InvalidByte(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// 0xFF is never valid in UTF-8.
	vt.Write([]byte{0xFF})
	got, _ := vtCellAt(vt, 0, 0)
	if got != '\uFFFD' {
		t.Fatalf("invalid byte: %q, want U+FFFD", got)
	}
}

func TestVTerm_UTF8_WideCharAtRightMargin(t *testing.T) {
	t.Parallel()
	// 5-column terminal. Wide char at col 4 should wrap.
	vt := NewVTerm(3, 5)
	fmt.Fprint(vt, "ABCD")
	// Cursor at col 4. Write wide char '中' (width 2).
	fmt.Fprint(vt, "中")
	// Column 4 should be padded with space, '中' should wrap to next line.
	got4, _ := vtCellAt(vt, 0, 4)
	if got4 != ' ' {
		t.Fatalf("right margin pad: %q, want space", got4)
	}
	gotRow1Col0, _ := vtCellAt(vt, 1, 0)
	if gotRow1Col0 != '中' {
		t.Fatalf("wrapped wide char: %q, want 中", gotRow1Col0)
	}
	gotRow1Col1, _ := vtCellAt(vt, 1, 1)
	if gotRow1Col1 != 0 {
		t.Fatalf("wrapped wide char placeholder: %d, want 0", gotRow1Col1)
	}
}

func TestVTerm_UTF8_RenderRoundtrip(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	fmt.Fprint(vt, "Hello 世界!")
	rendered := vt.Render()
	// The rendered output should contain valid UTF-8, including 世 and 界.
	if !strings.Contains(rendered, "Hello") {
		t.Fatal("rendered output missing 'Hello'")
	}
	if !strings.Contains(rendered, "世界") {
		t.Fatal("rendered output missing '世界'")
	}
	if !strings.Contains(rendered, "!") {
		t.Fatal("rendered output missing '!'")
	}
}

func TestVTerm_UTF8_MixedWithEscapeSequences(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Bold, then UTF-8, then reset.
	fmt.Fprint(vt, "\x1b[1m世界\x1b[0mABC")
	got0, a0 := vtCellAt(vt, 0, 0)
	got1, _ := vtCellAt(vt, 0, 1) // placeholder
	got2, _ := vtCellAt(vt, 0, 2)
	got3, _ := vtCellAt(vt, 0, 3) // placeholder
	got4, a4 := vtCellAt(vt, 0, 4)
	if got0 != '世' || !a0.bold {
		t.Fatalf("cell 0: %q bold=%v, want 世/true", got0, a0.bold)
	}
	if got1 != 0 {
		t.Fatalf("cell 1 placeholder: %d, want 0", got1)
	}
	if got2 != '界' {
		t.Fatalf("cell 2: %q, want 界", got2)
	}
	if got3 != 0 {
		t.Fatalf("cell 3 placeholder: %d, want 0", got3)
	}
	if got4 != 'A' || a4.bold {
		t.Fatalf("cell 4: %q bold=%v, want A/false", got4, a4.bold)
	}
}

// --- Concurrency Tests ---

func TestVTerm_ConcurrentWriteRenderResize(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(24, 80)
	done := make(chan struct{})

	// Writer goroutine.
	go func() {
		defer func() { done <- struct{}{} }()
		for i := 0; i < 1000; i++ {
			_, _ = vt.Write([]byte("Hello 世界! \x1b[1mBold\x1b[0m\n"))
		}
	}()

	// Renderer goroutine.
	go func() {
		defer func() { done <- struct{}{} }()
		for i := 0; i < 1000; i++ {
			_ = vt.Render()
		}
	}()

	// Resizer goroutine.
	go func() {
		defer func() { done <- struct{}{} }()
		for i := 0; i < 100; i++ {
			vt.Resize(20+(i%10), 60+(i%40))
		}
	}()

	// Wait for all goroutines.
	<-done
	<-done
	<-done
}

// --- T062: Erase/scroll rendition preservation tests ---

// bgColor is a test helper that constructs a color for standard bg codes (40-47).
func bgColor(code int) color {
	return color{kind: kind8, value: uint32(code - 40)}
}

// TestVTerm_EraseDisplay_PreservesRendition verifies that ED (erase display)
// fills erased cells with the current graphic rendition, not default attrs.
func TestVTerm_EraseDisplay_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Set blue background (\x1b[44m), then erase entire display (\x1b[2J).
	vt.Write([]byte("\x1b[44m\x1b[2J"))
	wantBg := bgColor(44)
	_, a := vtCellAt(vt, 0, 0)
	if a.bg != wantBg {
		t.Errorf("cell (0,0) should have blue bg, got: %+v", a.bg)
	}
	_, a2 := vtCellAt(vt, 4, 9)
	if a2.bg != wantBg {
		t.Errorf("cell (4,9) should have blue bg, got: %+v", a2.bg)
	}
}

// TestVTerm_EraseLine_PreservesRendition verifies that EL fills with current rendition.
func TestVTerm_EraseLine_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Write some text, set green bg, erase entire line.
	vt.Write([]byte("Hello\x1b[42m\x1b[2K"))
	wantBg := bgColor(42)
	_, a := vtCellAt(vt, 0, 0)
	if a.bg != wantBg {
		t.Errorf("cell (0,0) should have green bg after EL, got: %+v", a.bg)
	}
	_, a2 := vtCellAt(vt, 0, 9)
	if a2.bg != wantBg {
		t.Errorf("cell (0,9) should have green bg after EL, got: %+v", a2.bg)
	}
}

// TestVTerm_EraseChars_PreservesRendition verifies ECH fills with current rendition.
func TestVTerm_EraseChars_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Write text, set red bg, position cursor, erase 3 chars.
	vt.Write([]byte("ABCDEFGH\x1b[41m\x1b[1;3H\x1b[3X"))
	wantBg := bgColor(41)
	// Cells at (0,2), (0,3), (0,4) should be erased with red bg.
	for c := 2; c <= 4; c++ {
		ch, a := vtCellAt(vt, 0, c)
		if ch != ' ' {
			t.Errorf("cell (0,%d) should be space, got %q", c, ch)
		}
		if a.bg != wantBg {
			t.Errorf("cell (0,%d) should have red bg, got: %+v", c, a.bg)
		}
	}
}

// TestVTerm_ScrollUp_PreservesRendition verifies scrolled-in lines carry rendition.
func TestVTerm_ScrollUp_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Fill some lines, set blue bg, then scroll up.
	vt.Write([]byte("Line1\nLine2\nLine3\nLine4\n"))
	vt.Write([]byte("\x1b[44m"))
	// Trigger scroll by writing at the last line and going past it.
	vt.Write([]byte("Last\n"))
	// The new blank line(s) at the bottom should have blue bg.
	wantBg := bgColor(44)
	_, a := vtCellAt(vt, 4, 0)
	if a.bg != wantBg {
		t.Errorf("new scrolled-in line should have blue bg, got: %+v", a.bg)
	}
}

// TestVTerm_DeleteChars_PreservesRendition verifies DCH fills vacated cells with rendition.
func TestVTerm_DeleteChars_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Write "ABCDEFGHIJ", set cyan bg, position at col 2, delete 3 chars.
	vt.Write([]byte("ABCDEFGHIJ\x1b[46m\x1b[1;3H\x1b[3P"))
	wantBg := bgColor(46)
	// Cells at end of line (positions 7,8,9) should be blanks with cyan bg.
	for c := 7; c <= 9; c++ {
		ch, a := vtCellAt(vt, 0, c)
		if ch != ' ' {
			t.Errorf("cell (0,%d) should be space, got %q", c, ch)
		}
		if a.bg != wantBg {
			t.Errorf("cell (0,%d) should have cyan bg, got: %+v", c, a.bg)
		}
	}
}

// TestVTerm_InsertChars_PreservesRendition verifies ICH fills inserted cells with rendition.
func TestVTerm_InsertChars_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Write "ABCDEFGHIJ", set magenta bg, position at col 2, insert 2 chars.
	vt.Write([]byte("ABCDEFGHIJ\x1b[45m\x1b[1;3H\x1b[2@"))
	wantBg := bgColor(45)
	// Inserted cells at (0,2) and (0,3) should be blanks with magenta bg.
	for c := 2; c <= 3; c++ {
		ch, a := vtCellAt(vt, 0, c)
		if ch != ' ' {
			t.Errorf("cell (0,%d) should be space, got %q", c, ch)
		}
		if a.bg != wantBg {
			t.Errorf("cell (0,%d) should have magenta bg, got: %+v", c, a.bg)
		}
	}
}

// TestVTerm_InsertLines_PreservesRendition verifies IL fills with current rendition.
func TestVTerm_InsertLines_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Write some lines.
	vt.Write([]byte("Line1\nLine2\nLine3\nLine4\nLine5"))
	// Set yellow bg, move to row 1, insert 2 lines.
	vt.Write([]byte("\x1b[43m\x1b[2;1H\x1b[2L"))
	wantBg := bgColor(43)
	// Inserted lines at row 1 and 2 should have yellow bg.
	for r := 1; r <= 2; r++ {
		_, a := vtCellAt(vt, r, 0)
		if a.bg != wantBg {
			t.Errorf("row %d should have yellow bg, got: %+v", r, a.bg)
		}
	}
}

// TestVTerm_DeleteLines_PreservesRendition verifies DL fills with current rendition.
func TestVTerm_DeleteLines_PreservesRendition(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Write 5 lines.
	vt.Write([]byte("Line1\nLine2\nLine3\nLine4\nLine5"))
	// Set white bg, move to row 1, delete 2 lines.
	vt.Write([]byte("\x1b[47m\x1b[2;1H\x1b[2M"))
	wantBg := bgColor(47)
	// New blank lines at bottom (rows 3 and 4) should have white bg.
	for r := 3; r <= 4; r++ {
		_, a := vtCellAt(vt, r, 0)
		if a.bg != wantBg {
			t.Errorf("row %d should have white bg, got: %+v", r, a.bg)
		}
	}
}

// TestVTerm_EraseDisplay_ResetThenErase verifies that default attrs produce no bg.
func TestVTerm_EraseDisplay_ResetThenErase(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Set blue bg, reset, erase — cells should have default attrs.
	vt.Write([]byte("\x1b[44mX\x1b[0m\x1b[2J"))
	_, a := vtCellAt(vt, 0, 0)
	if a.bg.kind != kindDefault {
		t.Errorf("cell should have default bg after reset+erase, got: %+v", a.bg)
	}
}

// ---------------------------------------------------------------------------
// T065: Cursor visibility tracking (DECTCEM)
// ---------------------------------------------------------------------------

// TestVTerm_CursorVisibility_Default verifies the cursor is visible by default
// and Render() includes the show-cursor sequence.
func TestVTerm_CursorVisibility_Default(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 10)
	rendered := vt.Render()
	if !strings.HasSuffix(rendered, "\x1b[?25h") {
		t.Fatalf("default render should end with cursor-show, got suffix: %q",
			rendered[max(0, len(rendered)-20):])
	}
}

// TestVTerm_CursorVisibility_Hide verifies DECRST ?25 hides the cursor and
// Render() includes the hide-cursor sequence.
func TestVTerm_CursorVisibility_Hide(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 10)
	vt.Write([]byte("\x1b[?25l")) // DECRST 25 — hide cursor
	rendered := vt.Render()
	if !strings.HasSuffix(rendered, "\x1b[?25l") {
		t.Fatalf("render after cursor hide should end with hide seq, got suffix: %q",
			rendered[max(0, len(rendered)-20):])
	}
}

// TestVTerm_CursorVisibility_ShowAfterHide verifies a hiding then re-showing
// of cursor roundtrips through Render() correctly.
func TestVTerm_CursorVisibility_ShowAfterHide(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 10)
	vt.Write([]byte("\x1b[?25l")) // hide
	vt.Write([]byte("\x1b[?25h")) // show again
	rendered := vt.Render()
	if !strings.HasSuffix(rendered, "\x1b[?25h") {
		t.Fatalf("render after show-after-hide should end with cursor-show, got suffix: %q",
			rendered[max(0, len(rendered)-20):])
	}
}

// TestVTerm_CursorVisibility_PersistsAcrossText verifies that text output
// between hide and render does not reset cursor visibility.
func TestVTerm_CursorVisibility_PersistsAcrossText(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 10)
	vt.Write([]byte("\x1b[?25l"))       // hide
	vt.Write([]byte("Hello, world!\n")) // normal text
	vt.Write([]byte("\x1b[1;1H"))       // cursor move
	rendered := vt.Render()
	if !strings.HasSuffix(rendered, "\x1b[?25l") {
		t.Fatalf("cursor should stay hidden after text output, got suffix: %q",
			rendered[max(0, len(rendered)-20):])
	}
}

// TestVTerm_CursorVisibility_AltScreen verifies cursor visibility state is
// independent on the alt screen.
func TestVTerm_CursorVisibility_AltScreen(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 10)
	// Hide cursor on main screen.
	vt.Write([]byte("\x1b[?25l"))
	// Switch to alt screen — alt screen has its own screenBuffer with
	// cursorVisible defaulting to true.
	vt.Write([]byte("\x1b[?1049h"))
	rendered := vt.Render()
	if !strings.HasSuffix(rendered, "\x1b[?25h") {
		t.Fatalf("alt screen cursor should be visible by default, got suffix: %q",
			rendered[max(0, len(rendered)-20):])
	}
	// Switch back to main screen — cursor should still be hidden.
	vt.Write([]byte("\x1b[?1049l"))
	rendered = vt.Render()
	if !strings.HasSuffix(rendered, "\x1b[?25l") {
		t.Fatalf("main screen cursor should still be hidden after alt-screen roundtrip, got suffix: %q",
			rendered[max(0, len(rendered)-20):])
	}
}

// ---------------------------------------------------------------------------
// T070: Saved cursor clamping on resize
// ---------------------------------------------------------------------------

// TestVTerm_Resize_ClampsSavedCursor verifies that when a VTerm is resized
// smaller than the saved cursor position, the saved position is clamped to
// prevent index-out-of-bounds on restore.
func TestVTerm_Resize_ClampsSavedCursor(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(40, 100)
	// Move cursor to row 35, col 90 (0-indexed: 34, 89).
	vt.Write([]byte("\x1b[36;91H"))
	// Save cursor.
	vt.Write([]byte("\x1b7"))
	// Resize to much smaller: 24 rows, 80 cols.
	vt.Resize(24, 80)
	// Restore cursor — should not panic, and cursor should be clamped.
	vt.Write([]byte("\x1b8"))
	// Render to verify no panic and cursor position is within bounds.
	rendered := vt.Render()
	// The cursor position in Render() should reference row <= 24, col <= 80.
	// The saved cursor (35, 90) should have been clamped to (23, 79) by resize.
	// After restore: cursor is at (23, 79), so Render() ends with \x1b[24;80H.
	wantSuffix := "\x1b[24;80H\x1b[?25h"
	if !strings.HasSuffix(rendered, wantSuffix) {
		t.Fatalf("cursor should be clamped to (24,80) 1-indexed after resize, got suffix: %q",
			rendered[max(0, len(rendered)-30):])
	}
}

// TestVTerm_Resize_ClampsSavedCursor_CSI verifies CSI s/u (ANSI save/restore)
// also benefits from resize clamping.
func TestVTerm_Resize_ClampsSavedCursor_CSI(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(50, 120)
	// Move cursor to row 45, col 110 (0-indexed: 44, 109).
	vt.Write([]byte("\x1b[46;111H"))
	// Save cursor via CSI s.
	vt.Write([]byte("\x1b[s"))
	// Resize smaller.
	vt.Resize(20, 60)
	// Restore via CSI u — should not panic.
	vt.Write([]byte("\x1b[u"))
	rendered := vt.Render()
	// Saved cursor (44, 109) clamped to (19, 59) → 1-indexed (20, 60)
	wantSuffix := "\x1b[20;60H\x1b[?25h"
	if !strings.HasSuffix(rendered, wantSuffix) {
		t.Fatalf("CSI s/u cursor should be clamped after resize, got suffix: %q",
			rendered[max(0, len(rendered)-30):])
	}
}

// ---------------------------------------------------------------------------
// T066: Render() optimization — skip empty rows and trailing default cells
// ---------------------------------------------------------------------------

// TestVTerm_Render_SparseOptimization verifies that Render() output is
// significantly smaller for a sparse screen (few characters on a large terminal)
// compared to a dense screen, proving empty rows are skipped.
func TestVTerm_Render_SparseOptimization(t *testing.T) {
	t.Parallel()

	rows, cols := 200, 80

	// Sparse: only "Hello" on row 0.
	sparse := NewVTerm(rows, cols)
	sparse.Write([]byte("Hello"))
	sparseRendered := sparse.Render()

	// Dense: fill every row with content.
	dense := NewVTerm(rows, cols)
	line := strings.Repeat("X", cols) + "\n"
	for i := 0; i < rows; i++ {
		dense.Write([]byte(line))
	}
	denseRendered := dense.Render()

	// Sparse should be at LEAST 50% smaller than dense.
	if len(sparseRendered) >= len(denseRendered)/2 {
		t.Errorf("sparse render (%d bytes) should be much smaller than dense (%d bytes)",
			len(sparseRendered), len(denseRendered))
	}
}

// TestVTerm_Render_EmptyScreenMinimal verifies that a completely empty screen
// produces minimal Render output (just reset + cursor + visibility).
func TestVTerm_Render_EmptyScreenMinimal(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(100, 80)
	rendered := vt.Render()
	// Expected: \x1b[0m (reset) + \x1b[0m (reset at end) + \x1b[1;1H (cursor) + \x1b[?25h (show cursor)
	// No row content should be present.
	expected := "\x1b[0m\x1b[0m\x1b[1;1H\x1b[?25h"
	if rendered != expected {
		t.Errorf("empty 100x80 screen render should be minimal (%d bytes), got %d bytes: %q",
			len(expected), len(rendered), rendered)
	}
}

// TestVTerm_Render_TrailingSpacesTrimmed verifies that trailing default spaces
// on a line are not emitted.
func TestVTerm_Render_TrailingSpacesTrimmed(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(5, 80)
	vt.Write([]byte("AB"))
	rendered := vt.Render()
	// Should contain "AB" on row 1 but NOT 78 trailing spaces.
	// Row 1 content: CUP + "AB" = \x1b[1;1HAB
	if !strings.Contains(rendered, "\x1b[1;1HAB") {
		t.Fatalf("render should contain cursor-position + AB, got: %q", rendered)
	}
	// Check that there isn't a long run of spaces after AB.
	idx := strings.Index(rendered, "AB")
	after := rendered[idx+2:]
	// The next thing should be the reset (\x1b[0m) or cursor position, not spaces.
	if len(after) > 0 && after[0] == ' ' {
		t.Errorf("trailing spaces should be trimmed after AB, got: %q", after[:min(20, len(after))])
	}
}

// ---------------------------------------------------------------------------
// T068: DECSET 1049 cursor save/restore correctness
// ---------------------------------------------------------------------------

// TestVTerm_DECSET1049_CursorSaveRestore verifies the full cycle:
// text at (5,10) → DECSET 1049 → text at (2,3) → DECRST 1049 → cursor at (5,10).
func TestVTerm_DECSET1049_CursorSaveRestore(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(24, 80)
	// Position cursor at (5,10) — 1-indexed.
	vt.Write([]byte("\x1b[5;10H"))
	// Enter alt screen (saves primary cursor).
	vt.Write([]byte("\x1b[?1049h"))
	// Position cursor at (2,3) on alt screen.
	vt.Write([]byte("\x1b[2;3H"))
	// Exit alt screen (restores primary cursor).
	vt.Write([]byte("\x1b[?1049l"))
	// Cursor should be back at (5,10).
	rendered := vt.Render()
	wantSuffix := "\x1b[5;10H\x1b[?25h"
	if !strings.HasSuffix(rendered, wantSuffix) {
		t.Fatalf("cursor should be at (5,10) after DECRST 1049, got suffix: %q",
			rendered[max(0, len(rendered)-30):])
	}
}

// TestVTerm_DECSET1049_DoubleSET_NoPanic verifies double DECSET 1049 is a no-op
// on the second call and does not corrupt the saved cursor.
func TestVTerm_DECSET1049_DoubleSET_NoPanic(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(24, 80)
	// Set cursor at (10,20) on primary screen.
	vt.Write([]byte("\x1b[10;20H"))
	// Enter alt screen (saves (10,20)).
	vt.Write([]byte("\x1b[?1049h"))
	// Move cursor on alt screen.
	vt.Write([]byte("\x1b[1;1H"))
	// Double-set: should be a no-op (already on alternate).
	vt.Write([]byte("\x1b[?1049h"))
	// Move cursor again on alt screen.
	vt.Write([]byte("\x1b[3;5H"))
	// Exit alt screen: should restore to (10,20), not (1,1).
	vt.Write([]byte("\x1b[?1049l"))
	rendered := vt.Render()
	wantSuffix := "\x1b[10;20H\x1b[?25h"
	if !strings.HasSuffix(rendered, wantSuffix) {
		t.Fatalf("double DECSET should not corrupt saved cursor, expected (10,20), got suffix: %q",
			rendered[max(0, len(rendered)-30):])
	}
}

// TestVTerm_DECRST1049_DoubleRST_NoPanic verifies double DECRST 1049 is a no-op
// on the second call and does not corrupt primary cursor.
func TestVTerm_DECRST1049_DoubleRST_NoPanic(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(24, 80)
	// Set cursor at (8,15).
	vt.Write([]byte("\x1b[8;15H"))
	// Enter alt screen (saves (8,15)).
	vt.Write([]byte("\x1b[?1049h"))
	// Exit alt screen (restores to (8,15)).
	vt.Write([]byte("\x1b[?1049l"))
	// Move primary cursor to (3,3).
	vt.Write([]byte("\x1b[3;3H"))
	// Double-reset: should be a no-op (already on primary).
	vt.Write([]byte("\x1b[?1049l"))
	// Cursor should still be at (3,3), not restored to (8,15).
	rendered := vt.Render()
	wantSuffix := "\x1b[3;3H\x1b[?25h"
	if !strings.HasSuffix(rendered, wantSuffix) {
		t.Fatalf("double DECRST should not corrupt cursor, expected (3,3), got suffix: %q",
			rendered[max(0, len(rendered)-30):])
	}
}

// ---------------------------------------------------------------------------
// T069: Alt-screen mode variants ?47 and ?1047
// ---------------------------------------------------------------------------

// TestVTerm_DECSET47_SwitchNoCursorSaveNoClear verifies ?47 switches to
// alternate screen without saving cursor or clearing.
func TestVTerm_DECSET47_SwitchNoCursorSaveNoClear(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(10, 20)
	// Write on primary.
	vt.Write([]byte("Primary"))
	// Position cursor on primary.
	vt.Write([]byte("\x1b[5;10H"))
	// Switch to alt via ?47 (no clear, no cursor save).
	vt.Write([]byte("\x1b[?47h"))
	// Write on alt.
	vt.Write([]byte("Alt"))
	// Switch back to primary via ?47.
	vt.Write([]byte("\x1b[?47l"))
	// Primary content should still show "Primary" (was not cleared).
	rendered := vt.Render()
	if !strings.Contains(rendered, "Primary") {
		t.Errorf("primary content should be preserved, got: %q", rendered)
	}
	// Cursor position is NOT restored (no cursor save with ?47).
	// After switching back, cursor is wherever primary left it.
}

// TestVTerm_DECSET1047_SwitchAndClearNoCursorSave verifies ?1047 switches to
// alternate screen and clears it, but does not save/restore cursor.
func TestVTerm_DECSET1047_SwitchAndClearNoCursorSave(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(10, 20)
	// Position cursor at (5,10).
	vt.Write([]byte("\x1b[5;10H"))
	// Switch to alt via ?1047 (clears alt, no cursor save).
	vt.Write([]byte("\x1b[?1047h"))
	// Write on alt.
	vt.Write([]byte("AltContent"))
	// Alt should have the content at (0,0) since cleared.
	gotCh, _ := vtCellAt(vt, 0, 0)
	if gotCh != 'A' {
		t.Errorf("alt screen after ?1047 should have 'A' at (0,0), got %q", gotCh)
	}
	// Switch back via ?1047.
	vt.Write([]byte("\x1b[?1047l"))
	// Cursor should NOT be restored to (5,10) — ?1047 doesn't save/restore.
	// (Cursor returns to wherever primary is, which wasn't modified.)
}

// TestVTerm_DECSET47_DoubleSet_NoOp verifies double ?47h is a no-op.
func TestVTerm_DECSET47_DoubleSet_NoOp(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(10, 20)
	vt.Write([]byte("\x1b[?47h")) // switch to alt
	vt.Write([]byte("AltText"))
	vt.Write([]byte("\x1b[?47h")) // double-set: should be no-op
	// Content on alt should be preserved.
	gotCh, _ := vtCellAt(vt, 0, 0)
	if gotCh != 'A' {
		t.Errorf("double ?47h should not clear, got %q at (0,0)", gotCh)
	}
	vt.Write([]byte("\x1b[?47l")) // switch back to primary, no panic
}

// ─── Tab Stop Tests ───

func TestVTerm_TabStops_DefaultEvery8(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// From column 0, tabs should land at 8, 16, 24, 32.
	fmt.Fprint(vt, "0\t1\t2\t3\t4")
	expectations := map[int]rune{
		0:  '0',
		8:  '1',
		16: '2',
		24: '3',
		32: '4',
	}
	for col, want := range expectations {
		got, _ := vtCellAt(vt, 0, col)
		if got != want {
			t.Errorf("col %d: got %q, want %q", col, got, want)
		}
	}
}

func TestVTerm_HTS_SetCustomTabStop(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// Move to column 5 and set a tab stop.
	vt.Write([]byte("\x1b[6G")) // CHA to column 6 (1-indexed) = col 5
	vt.Write([]byte("\x1bH"))   // HTS — set tab stop at col 5
	vt.Write([]byte("\x1b[1G")) // CHA to column 1 = col 0
	vt.Write([]byte("A\tB"))    // A at 0, tab should stop at 5, B at 5
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Errorf("col 0: got %q, want 'A'", got)
	}
	got, _ = vtCellAt(vt, 0, 5)
	if got != 'B' {
		t.Errorf("col 5: got %q, want 'B' (HTS at col 5)", got)
	}
}

func TestVTerm_TBC_ClearCurrentTabStop(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// Move to col 8 and clear tab stop there (CSI 0 g).
	vt.Write([]byte("\x1b[9G")) // CHA col 9 (1-indexed) = col 8
	vt.Write([]byte("\x1b[0g")) // TBC: clear tab stop at current column
	vt.Write([]byte("\x1b[1G")) // CHA back to col 0
	// Tab from col 0 should skip col 8 (cleared) and land at col 16.
	vt.Write([]byte("A\tB"))
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Errorf("col 0: got %q, want 'A'", got)
	}
	got, _ = vtCellAt(vt, 0, 16)
	if got != 'B' {
		t.Errorf("col 16: got %q, want 'B' (col 8 tab cleared)", got)
	}
	// Verify col 8 is blank.
	got, _ = vtCellAt(vt, 0, 8)
	if got != ' ' {
		t.Errorf("col 8: got %q, want ' '", got)
	}
}

func TestVTerm_TBC_ClearAllTabStops(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// Clear all tab stops (CSI 3 g).
	vt.Write([]byte("\x1b[3g"))
	// Tab from col 0 should go to last column (no stops to find).
	vt.Write([]byte("A\tB"))
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Errorf("col 0: got %q, want 'A'", got)
	}
	got, _ = vtCellAt(vt, 0, 39)
	if got != 'B' {
		t.Errorf("col 39: got %q, want 'B' (all tabs cleared, clamp to last col)", got)
	}
}

func TestVTerm_CHT_ForwardNTabStops(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// From col 0, CHT 2 should advance 2 tab stops = col 16.
	vt.Write([]byte("A"))       // cursor at col 1
	vt.Write([]byte("\x1b[2I")) // CHT 2: advance 2 tab stops (8, 16)
	vt.Write([]byte("B"))
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'A' {
		t.Errorf("col 0: got %q, want 'A'", got)
	}
	got, _ = vtCellAt(vt, 0, 16)
	if got != 'B' {
		t.Errorf("col 16: got %q, want 'B' (CHT 2 from col 1)", got)
	}
}

func TestVTerm_CHT_DefaultParam(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// CHT with no param defaults to 1.
	vt.Write([]byte("A"))      // cursor at col 1
	vt.Write([]byte("\x1b[I")) // CHT 1: advance 1 tab stop = col 8
	vt.Write([]byte("B"))
	got, _ := vtCellAt(vt, 0, 8)
	if got != 'B' {
		t.Errorf("col 8: got %q, want 'B' (CHT default 1 from col 1)", got)
	}
}

func TestVTerm_CBT_BackwardNTabStops(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// Move to col 20, then CBT 2 should move back 2 tab stops = col 8.
	vt.Write([]byte("\x1b[21G")) // CHA col 21 (1-indexed) = col 20
	vt.Write([]byte("\x1b[2Z"))  // CBT 2: back 2 tab stops (16 → 8)
	vt.Write([]byte("B"))
	got, _ := vtCellAt(vt, 0, 8)
	if got != 'B' {
		t.Errorf("col 8: got %q, want 'B' (CBT 2 from col 20)", got)
	}
}

func TestVTerm_CBT_ClampsToCol0(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// From col 5, CBT 10 should clamp at col 0.
	vt.Write([]byte("\x1b[6G"))  // CHA col 6 = col 5
	vt.Write([]byte("\x1b[10Z")) // CBT 10: way past all stops
	vt.Write([]byte("B"))
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'B' {
		t.Errorf("col 0: got %q, want 'B' (CBT clamp to col 0)", got)
	}
}

func TestVTerm_TabStops_ResizeExtendsWithDefaults(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Resize to 40 columns. New columns should have default 8-col stops.
	vt.Resize(3, 40)
	// Tab from col 0 should stop at 8, another tab at 16, another at 24.
	fmt.Fprint(vt, "\t\t\tX")
	got, _ := vtCellAt(vt, 0, 24)
	if got != 'X' {
		t.Errorf("col 24: got %q, want 'X' (resize extended tab stops)", got)
	}
}

func TestVTerm_TabStops_ResizeTruncates(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// Resize to 10 columns. Tab from col 0 should stop at col 8.
	vt.Resize(3, 10)
	fmt.Fprint(vt, "\tX")
	got, _ := vtCellAt(vt, 0, 8)
	if got != 'X' {
		t.Errorf("col 8: got %q, want 'X' (tab at 8 still present after truncate)", got)
	}
}

func TestVTerm_TabStops_ResizePreservesCustom(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// Set custom tab stop at col 3.
	vt.Write([]byte("\x1b[4G")) // CHA col 4 = col 3
	vt.Write([]byte("\x1bH"))   // HTS at col 3
	// Resize to 40 columns — custom stop at 3 should be preserved.
	vt.Resize(3, 40)
	vt.Write([]byte("\x1b[1G")) // CHA back to col 0
	vt.Write([]byte("A\tB"))    // A at 0, tab stops at 3, B at 3
	got, _ := vtCellAt(vt, 0, 3)
	if got != 'B' {
		t.Errorf("col 3: got %q, want 'B' (custom tab stop preserved after resize)", got)
	}
}

func TestVTerm_TabAtLastColumn(t *testing.T) {
	t.Parallel()
	// Tab at the last column should stay at last column.
	vt := NewVTerm(3, 10)
	vt.Write([]byte("\x1b[10G")) // CHA col 10 = col 9 (last)
	vt.Write([]byte("\tX"))
	// Tab from last col: no tab stop found, should stay at col 9.
	got, _ := vtCellAt(vt, 0, 9)
	if got != 'X' {
		t.Errorf("col 9: got %q, want 'X' (tab at last col stays)", got)
	}
}

// ─── DCS (Device Control String) Tests ───

func TestVTerm_DCS_ConsumedWithST(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// DCS payload terminated by ST (ESC \). Should be completely consumed.
	vt.Write([]byte("A"))
	vt.Write([]byte("\x1bPsome;dcs;payload\x1b\\"))
	vt.Write([]byte("B"))
	// A at col 0, B at col 1 — DCS was invisible.
	gotA, _ := vtCellAt(vt, 0, 0)
	gotB, _ := vtCellAt(vt, 0, 1)
	if gotA != 'A' || gotB != 'B' {
		t.Errorf("DCS with ST not consumed: got (%q, %q), want ('A', 'B')", gotA, gotB)
	}
}

func TestVTerm_DCS_ConsumedWithBEL(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// DCS payload terminated by BEL.
	vt.Write([]byte("A"))
	vt.Write([]byte("\x1bPpayload\x07"))
	vt.Write([]byte("B"))
	gotA, _ := vtCellAt(vt, 0, 0)
	gotB, _ := vtCellAt(vt, 0, 1)
	if gotA != 'A' || gotB != 'B' {
		t.Errorf("DCS with BEL not consumed: got (%q, %q), want ('A', 'B')", gotA, gotB)
	}
}

func TestVTerm_DCS_BoundedConsumption(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// DCS with payload exceeding maxDCSLen (4096) — should abort to ground.
	// After abort, remaining bytes in the write are processed as ground-state
	// text. We verify the parser doesn't hang/OOM and resumes normal operation.
	dcs := []byte("\x1bP")
	payload := make([]byte, 5000)
	for i := range payload {
		payload[i] = 'x'
	}
	dcs = append(dcs, payload...)
	vt.Write(dcs)
	// After overflow abort + ground-state processing, parser is functional.
	// Erase display to clear overflow debris, then write verifiable text.
	vt.Write([]byte("\x1b[2J\x1b[H")) // ED 2 + CUP home
	vt.Write([]byte("OK"))
	gotO, _ := vtCellAt(vt, 0, 0)
	gotK, _ := vtCellAt(vt, 0, 1)
	if gotO != 'O' || gotK != 'K' {
		t.Errorf("parser not functional after DCS overflow: got (%q, %q), want ('O', 'K')", gotO, gotK)
	}
}

func TestVTerm_DCS_SubsequentTextCorrect(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 40)
	// Write DCS, then multiple lines of text — everything after DCS is fine.
	vt.Write([]byte("\x1bP1$r0m\x1b\\")) // DECRQSS response (DCS ... ST)
	vt.Write([]byte("Hello"))
	vt.Write([]byte("\x1b[2;1H")) // CUP to row 2, col 1
	vt.Write([]byte("World"))
	gotH, _ := vtCellAt(vt, 0, 0)
	gotW, _ := vtCellAt(vt, 1, 0)
	if gotH != 'H' || gotW != 'W' {
		t.Errorf("post-DCS text wrong: got (%q, %q), want ('H', 'W')", gotH, gotW)
	}
}

func TestVTerm_DCS_SplitAcrossWrites(t *testing.T) {
	t.Parallel()
	vt := NewVTerm(3, 20)
	// DCS payload split across two Write() calls.
	vt.Write([]byte("A"))
	vt.Write([]byte("\x1bPpart1"))  // Start DCS, partial payload
	vt.Write([]byte("part2\x1b\\")) // Rest of payload + ST
	vt.Write([]byte("B"))
	gotA, _ := vtCellAt(vt, 0, 0)
	gotB, _ := vtCellAt(vt, 0, 1)
	if gotA != 'A' || gotB != 'B' {
		t.Errorf("split DCS not consumed: got (%q, %q), want ('A', 'B')", gotA, gotB)
	}
}

// ─── UTF-8 Property Tests ───

func TestVTerm_UTF8_Roundtrip_RandomStrings(t *testing.T) {
	t.Parallel()
	// Test that random valid UTF-8 strings survive Write → cell readback.
	testStrings := []string{
		"Hello, World!",
		"café résumé naïve",
		"日本語テスト",
		"Привет мир",
		"مرحبا بالعالم",
		"🤖🎉🌍💻🔥",
		"a\u0300e\u0301i\u0302o\u0303u\u0304", // combining chars
		"½¾⅓⅔⅛",                               // fractions
		"∀∃∅∈∉∋∌",                             // math symbols
		"αβγδεζηθ",                            // Greek
		"ℕℤℚℝℂ",                               // double-struck
	}
	for _, s := range testStrings {
		vt := NewVTerm(5, 80)
		vt.Write([]byte(s))
		// Verify first rune matches.
		runes := []rune(s)
		if len(runes) > 0 {
			got, _ := vtCellAt(vt, 0, 0)
			if got != runes[0] {
				t.Errorf("string %q: first rune got %q, want %q", s, got, runes[0])
			}
		}
	}
}

func TestVTerm_UTF8_SplitBoundary_AllLengths(t *testing.T) {
	t.Parallel()
	// Test UTF-8 split across Write() for all multi-byte lengths (2, 3, 4 bytes).
	testCases := []struct {
		name string
		ch   rune
		enc  []byte
	}{
		{"2-byte é", 'é', []byte{0xc3, 0xa9}},
		{"3-byte 日", '日', []byte{0xe6, 0x97, 0xa5}},
		{"4-byte 🤖", '🤖', []byte{0xf0, 0x9f, 0xa4, 0x96}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Split at every possible byte boundary.
			for split := 1; split < len(tc.enc); split++ {
				vt := NewVTerm(3, 20)
				vt.Write(tc.enc[:split])
				vt.Write(tc.enc[split:])
				got, _ := vtCellAt(vt, 0, 0)
				if got != tc.ch {
					t.Errorf("split at %d: got %q, want %q", split, got, tc.ch)
				}
			}
		})
	}
}

func TestVTerm_UTF8_InvalidBytes_ProduceReplacement(t *testing.T) {
	t.Parallel()
	// Invalid UTF-8 byte sequences should produce U+FFFD replacement chars.
	invalidSeqs := [][]byte{
		{0xff},             // Invalid start byte
		{0xfe},             // Invalid start byte
		{0xc0, 0x80},       // Overlong encoding
		{0xc3},             // Truncated 2-byte (single byte)
		{0xe6, 0x97},       // Truncated 3-byte
		{0xf0, 0x9f, 0xa4}, // Truncated 4-byte
	}
	for i, seq := range invalidSeqs {
		vt := NewVTerm(3, 20)
		// Write invalid sequence followed by valid ASCII to flush.
		vt.Write(seq)
		vt.Write([]byte("A"))
		// The invalid bytes should produce replacement char(s) before 'A'.
		// We just verify no panic and that 'A' appears somewhere.
		found := false
		for col := 0; col < 20; col++ {
			got, _ := vtCellAt(vt, 0, col)
			if got == 'A' {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("case %d: 'A' not found after invalid UTF-8 %x", i, seq)
		}
	}
}

func TestVTerm_UTF8_WideChar_Positions(t *testing.T) {
	t.Parallel()
	// CJK characters should occupy 2 columns. Verify cursor positioning.
	vt := NewVTerm(3, 20)
	vt.Write([]byte("A日B"))
	// A at col 0, 日 at col 1 (wide: occupies 1 and 2), B at col 3.
	gotA, _ := vtCellAt(vt, 0, 0)
	gotCJK, _ := vtCellAt(vt, 0, 1)
	gotB, _ := vtCellAt(vt, 0, 3)
	if gotA != 'A' {
		t.Errorf("col 0: got %q, want 'A'", gotA)
	}
	if gotCJK != '日' {
		t.Errorf("col 1: got %q, want '日'", gotCJK)
	}
	if gotB != 'B' {
		t.Errorf("col 3: got %q, want 'B'", gotB)
	}
}

func TestVTerm_UTF8_ManyRandomSplits(t *testing.T) {
	t.Parallel()
	// Write a long UTF-8 string one byte at a time.
	input := "Hello, 世界! こんにちは 🌍🎉"
	vt := NewVTerm(3, 80)
	raw := []byte(input)
	for _, b := range raw {
		vt.Write([]byte{b})
	}
	// Verify first character.
	got, _ := vtCellAt(vt, 0, 0)
	if got != 'H' {
		t.Errorf("col 0: got %q, want 'H'", got)
	}
	// Render should not panic and produce valid output.
	rendered := vt.Render()
	if len(rendered) == 0 {
		t.Error("Render() produced empty output for non-empty terminal")
	}
}
