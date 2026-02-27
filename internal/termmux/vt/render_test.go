package vt

import (
	"strings"
	"testing"
)

func TestRender_Empty(t *testing.T) {
	scr := NewScreen(3, 5)
	out := Render(scr)
	// Should have reset, cursor position, and cursor visibility but NO cell-content CUP.
	if !strings.Contains(out, "\x1b[0m") {
		t.Error("missing SGR reset")
	}
	if !strings.Contains(out, "\x1b[1;1H") {
		// Cursor should be at 1,1 (0,0 -> 1-indexed).
		t.Error("missing cursor position")
	}
	if !strings.Contains(out, "\x1b[?25h") {
		t.Error("missing cursor show")
	}
	// Should NOT have any row-positioning CUPs since all rows are blank.
	// The only CUP should be the final cursor position.
	cupCount := strings.Count(out, "H")
	// We have \x1b[1;1H for cursor pos, \x1b[?25h also has 'h' not 'H'.
	if cupCount > 1 {
		t.Errorf("expected at most 1 CUP (cursor pos), got %d", cupCount)
	}
}

func TestRender_SimpleText(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.PutChar('A')
	scr.PutChar('B')
	out := Render(scr)
	if !strings.Contains(out, "\x1b[1;1H") {
		t.Error("missing CUP for row 1")
	}
	if !strings.Contains(out, "AB") {
		t.Errorf("missing text AB in output: %q", out)
	}
	if !strings.Contains(out, "\x1b[1;3H") {
		t.Errorf("missing cursor position \\x1b[1;3H in output: %q", out)
	}
}

func TestRender_BoldText(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.CurAttr = Attr{Bold: true}
	scr.PutChar('B')
	out := Render(scr)
	if !strings.Contains(out, "\x1b[") {
		t.Error("missing SGR sequence")
	}
	if !strings.Contains(out, "1") {
		t.Error("missing bold code in SGR")
	}
	if !strings.Contains(out, "B") {
		t.Error("missing character B")
	}
}

func TestRender_WideChar(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.PutChar('\u6F22') // 漢 - width 2
	out := Render(scr)
	count := strings.Count(out, "漢")
	if count != 1 {
		t.Errorf("漢 appears %d times, want 1", count)
	}
}

func TestRender_Idempotent(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.PutChar('X')
	scr.PutChar('Y')
	out1 := Render(scr)
	out2 := Render(scr)
	if out1 != out2 {
		t.Errorf("Render not idempotent:\n  1: %q\n  2: %q", out1, out2)
	}
}

// ── T122 and T089: Consecutive Render calls are idempotent ─────────

func TestRender_ConsecutiveCalls(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.CurAttr = Attr{Bold: true}
	scr.PutChar('H')
	scr.PutChar('i')
	scr.CurAttr = Attr{}
	scr.CurRow = 2
	scr.CurCol = 3
	scr.PutChar('!')

	out1 := Render(scr)
	out2 := Render(scr)
	out3 := Render(scr)
	if out1 != out2 || out2 != out3 {
		t.Error("Render() not idempotent across 3 calls")
	}
}

func TestRender_AfterModification(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.PutChar('A')
	out1 := Render(scr)

	scr.PutChar('B')
	out2 := Render(scr)

	if out1 == out2 {
		t.Error("Render should change after screen modification")
	}
	if !strings.Contains(out2, "AB") {
		t.Errorf("updated Render should contain AB, got %q", out2)
	}
}

func TestRender_HiddenCursor(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.CursorVisible = false
	scr.PutChar('T')
	out := Render(scr)
	if !strings.Contains(out, "\x1b[?25l") {
		t.Error("should contain cursor-hide sequence when CursorVisible=false")
	}
	if strings.Contains(out, "\x1b[?25h") {
		t.Error("should NOT contain cursor-show when CursorVisible=false")
	}
}
