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

// ── T28: RenderContentANSI tests ─────────────────────────────────

func TestRenderContentANSI_NoPositioningSequences(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.PutChar('H')
	scr.PutChar('i')

	out := RenderContentANSI(scr)
	// Must NOT have CUP (\x1b[row;colH) or EL (\x1b[K) or cursor sequences.
	if strings.Contains(out, "\x1b[K") {
		t.Errorf("RenderContentANSI should not contain EL (\\x1b[K), got %q", out)
	}
	// CUP sequence: \x1b[digits;digitsH
	if strings.Contains(out, ";1H") {
		t.Errorf("RenderContentANSI should not contain CUP (;1H), got %q", out)
	}
	if strings.Contains(out, "\x1b[?25") {
		t.Errorf("RenderContentANSI should not contain cursor visibility sequences, got %q", out)
	}
}

func TestRenderContentANSI_PreservesColors(t *testing.T) {
	scr := NewScreen(2, 10)
	scr.CurAttr = Attr{FG: color{kind: kind8, value: 1}} // red
	scr.PutChar('E')
	scr.PutChar('R')
	scr.PutChar('R')
	scr.CurAttr = Attr{}

	out := RenderContentANSI(scr)
	// Should contain SGR color escape for red.
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("RenderContentANSI should contain ANSI SGR, got %q", out)
	}
	// Characters should be present (possibly interleaved with SGR sequences).
	if !strings.ContainsRune(out, 'E') || !strings.ContainsRune(out, 'R') {
		t.Errorf("RenderContentANSI should contain characters E and R, got %q", out)
	}
	// Should end with SGR reset.
	if !strings.Contains(out, "\x1b[0m") {
		t.Errorf("RenderContentANSI should contain SGR reset, got %q", out)
	}
}

func TestRenderContentANSI_LinesJoinedByNewlines(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.PutChar('A')
	scr.CurRow = 1
	scr.CurCol = 0
	scr.PutChar('B')
	scr.CurRow = 2
	scr.CurCol = 0
	scr.PutChar('C')

	out := RenderContentANSI(scr)
	parts := strings.Split(out, "\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 lines, got %d (output: %q)", len(parts), out)
	}
	// First line should contain 'A', second 'B', third 'C'.
	if !strings.Contains(parts[0], "A") {
		t.Errorf("line 0 should contain 'A', got %q", parts[0])
	}
	if !strings.Contains(parts[1], "B") {
		t.Errorf("line 1 should contain 'B', got %q", parts[1])
	}
	if !strings.Contains(parts[2], "C") {
		t.Errorf("line 2 should contain 'C', got %q", parts[2])
	}
}

func TestRenderContentANSI_EmptyRowsAreBlank(t *testing.T) {
	scr := NewScreen(3, 10)
	// Only put content on row 0 — rows 1-2 should be empty.
	scr.PutChar('X')

	out := RenderContentANSI(scr)
	parts := strings.Split(out, "\n")
	if len(parts) != 3 {
		t.Fatalf("expected 3 lines, got %d (output: %q)", len(parts), out)
	}
	// Row 1 and 2 should be empty strings (no content, no SGR).
	if parts[1] != "" {
		t.Errorf("empty row should be blank, got %q", parts[1])
	}
	if parts[2] != "" {
		t.Errorf("empty row should be blank, got %q", parts[2])
	}
}

func TestRenderContentANSI_Idempotent(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.CurAttr = Attr{Bold: true, FG: color{kind: kind8, value: 2}}
	scr.PutChar('G')
	scr.PutChar('O')
	scr.CurAttr = Attr{}

	out1 := RenderContentANSI(scr)
	out2 := RenderContentANSI(scr)
	if out1 != out2 {
		t.Error("RenderContentANSI not idempotent")
	}
}
