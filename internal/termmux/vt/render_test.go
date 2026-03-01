package vt

import (
	"fmt"
	"strings"
	"testing"
)

// ── RenderFullScreen tests ─────────────────────────────────────────

func TestRenderFullScreen_EmitsAllRows(t *testing.T) {
	scr := NewScreen(4, 10)
	// Only put content on row 0; rows 1-3 are blank.
	scr.PutChar('A')
	out := RenderFullScreen(scr)

	// Every row should have a CUP and EL (clear-to-EOL = \x1b[K).
	for r := 1; r <= 4; r++ {
		cup := fmt.Sprintf("\x1b[%d;1H", r)
		if !strings.Contains(out, cup) {
			t.Errorf("missing CUP for row %d: %q", r, cup)
		}
	}
	// Check that EL appears for clearing.
	elCount := strings.Count(out, "\x1b[K")
	if elCount < 4 {
		t.Errorf("expected at least 4 EL sequences (one per row), got %d", elCount)
	}
}

func TestRenderFullScreen_NoScreenClear(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.PutChar('X')
	out := RenderFullScreen(scr)

	// Must NOT contain ESC[2J (erase entire display).
	if strings.Contains(out, "\x1b[2J") {
		t.Error("RenderFullScreen must not emit ESC[2J (erase display)")
	}
}

func TestRenderFullScreen_PreservesContent(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.PutChar('H')
	scr.PutChar('i')
	out := RenderFullScreen(scr)

	if !strings.Contains(out, "Hi") {
		t.Errorf("expected content 'Hi' in output, got %q", out)
	}
}

func TestRenderFullScreen_CursorPosition(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.CurRow = 1
	scr.CurCol = 4
	out := RenderFullScreen(scr)

	if !strings.Contains(out, "\x1b[2;5H") {
		t.Errorf("expected cursor at \\x1b[2;5H (1-indexed), got %q", out)
	}
}

func TestRenderFullScreen_CursorVisibility(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.CursorVisible = false
	out := RenderFullScreen(scr)

	if !strings.Contains(out, "\x1b[?25l") {
		t.Error("should contain cursor-hide for CursorVisible=false")
	}
	if strings.Contains(out, "\x1b[?25h") {
		t.Error("should NOT contain cursor-show for CursorVisible=false")
	}
}

func TestRenderFullScreen_Idempotent(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.PutChar('A')
	scr.CurRow = 2
	scr.CurCol = 0
	scr.PutChar('Z')
	out1 := RenderFullScreen(scr)
	out2 := RenderFullScreen(scr)
	if out1 != out2 {
		t.Error("RenderFullScreen not idempotent")
	}
}

func TestRenderFullScreen_BoldText(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.CurAttr = Attr{Bold: true}
	scr.PutChar('B')
	out := RenderFullScreen(scr)
	if !strings.Contains(out, "\x1b[") {
		t.Error("missing SGR sequence")
	}
	if !strings.Contains(out, "B") {
		t.Error("missing character B")
	}
}

func TestRenderFullScreen_WideChar(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.PutChar('\u6F22') // 漢 - width 2
	out := RenderFullScreen(scr)
	count := strings.Count(out, "漢")
	if count != 1 {
		t.Errorf("漢 appears %d times, want 1", count)
	}
}

func TestRenderFullScreen_AfterModification(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.PutChar('A')
	out1 := RenderFullScreen(scr)

	scr.PutChar('B')
	out2 := RenderFullScreen(scr)

	if out1 == out2 {
		t.Error("RenderFullScreen should change after screen modification")
	}
	if !strings.Contains(out2, "AB") {
		t.Errorf("updated RenderFullScreen should contain AB, got %q", out2)
	}
}

func TestRenderFullScreen_ConsecutiveCalls(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.CurAttr = Attr{Bold: true}
	scr.PutChar('H')
	scr.PutChar('i')
	scr.CurAttr = Attr{}
	scr.CurRow = 2
	scr.CurCol = 3
	scr.PutChar('!')

	out1 := RenderFullScreen(scr)
	out2 := RenderFullScreen(scr)
	out3 := RenderFullScreen(scr)
	if out1 != out2 || out2 != out3 {
		t.Error("RenderFullScreen not idempotent across 3 calls")
	}
}
