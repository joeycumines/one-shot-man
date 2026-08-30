package vt

import "testing"

func TestLNM_DefaultFalse(t *testing.T) {
	s := NewScreen(24, 80)
	if s.LineFeedNewLine {
		t.Fatal("new screen LineFeedNewLine = true, want false")
	}
}

func TestLNM_SetMode20(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20h"))
	if !v.LineFeedNewLine() {
		t.Fatal("after CSI 20h: LineFeedNewLine = false, want true")
	}
}

func TestLNM_ResetMode20(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20h"))
	if !v.LineFeedNewLine() {
		t.Fatal("after CSI 20h: expected LineFeedNewLine = true")
	}
	v.Write([]byte("\x1b[20l"))
	if v.LineFeedNewLine() {
		t.Fatal("after CSI 20l: LineFeedNewLine = true, want false")
	}
}

func TestLNM_LineFeedNewLineTrue(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20h")) // enable LNM
	v.Write([]byte("AB"))       // cursor at col 2
	v.Write([]byte("\x0a"))     // LF — should CR then move down
	if v.primary.CurRow != 1 {
		t.Fatalf("row = %d, want 1", v.primary.CurRow)
	}
	if v.primary.CurCol != 0 {
		t.Fatalf("col = %d, want 0 (CR before LF)", v.primary.CurCol)
	}
}

func TestLNM_LineFeedNewLineFalse(t *testing.T) {
	v := NewVTerm(24, 80)
	// LNM is false by default; just write LF
	v.Write([]byte("AB"))
	v.Write([]byte("\x0a")) // LF — should only move down
	if v.primary.CurRow != 1 {
		t.Fatalf("row = %d, want 1", v.primary.CurRow)
	}
	if v.primary.CurCol != 2 {
		t.Fatalf("col = %d, want 2 (no CR)", v.primary.CurCol)
	}
}

func TestLNM_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20h"))
	snap := v.ActiveScreen()
	if !snap.LineFeedNewLine {
		t.Fatal("Snapshot LineFeedNewLine = false, want true")
	}
	// Verify independence
	snap.LineFeedNewLine = false
	if !v.LineFeedNewLine() {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestLNM_RISReset(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[20h"))
	if !v.LineFeedNewLine() {
		t.Fatal("after CSI 20h: expected LineFeedNewLine = true")
	}
	// RIS (ESC c) resets all modes
	v.Write([]byte("\x1bc"))
	if v.LineFeedNewLine() {
		t.Fatal("after RIS: LineFeedNewLine = true, want false")
	}
}

func TestLNM_LineFeedAtScrollBottom(t *testing.T) {
	// Test that LNM still works correctly when at the scroll region bottom
	// (scrolling should happen, and cursor should end at col 0, row 4).
	v := NewVTerm(5, 10)
	// Set scroll region to rows 1-5 (0-indexed: 0-4)
	v.Write([]byte("\x1b[1;5r"))
	v.Write([]byte("\x1b[20h")) // LNM on
	// Write a character at bottom row, col 0
	v.Write([]byte("\x1b[5;1f")) // move to row 5, col 1 (1-indexed)
	v.Write([]byte("X"))
	if v.primary.Cells[4][0].Ch != 'X' {
		t.Fatal("character not placed correctly")
	}
	// LF should scroll up and move to col 0
	v.Write([]byte("\x0a"))
	if v.primary.CurCol != 0 {
		t.Fatalf("col = %d, want 0 (LNM with scroll)", v.primary.CurCol)
	}
	// Cursor should now be at row 4 (the new bottom of scroll region)
	if v.primary.CurRow != 4 {
		t.Fatalf("row = %d, want 4", v.primary.CurRow)
	}
}
