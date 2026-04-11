package vt

import "testing"

// TestCursorPosition_Initial verifies the cursor starts at (0,0).
func TestCursorPosition_Initial(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	row, col := v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("initial cursor = (%d,%d); want (0,0)", row, col)
	}
}

// TestCursorPosition_AfterCUP verifies cursor position after CUP (CSI H).
func TestCursorPosition_AfterCUP(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// CUP row 5, col 10 (1-based in ANSI → 4,9 in 0-based).
	v.Write([]byte("\x1b[5;10H"))
	row, col := v.CursorPosition()
	if row != 4 || col != 9 {
		t.Fatalf("cursor = (%d,%d); want (4,9)", row, col)
	}
}

// TestCursorPosition_AfterText verifies cursor advances with text output.
func TestCursorPosition_AfterText(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("Hello"))
	row, col := v.CursorPosition()
	if row != 0 || col != 5 {
		t.Fatalf("cursor = (%d,%d); want (0,5)", row, col)
	}
}

// TestCursorPosition_AfterNewline verifies cursor moves down on newline.
func TestCursorPosition_AfterNewline(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	v.Write([]byte("line1\r\nline2"))
	row, col := v.CursorPosition()
	if row != 1 || col != 5 {
		t.Fatalf("cursor = (%d,%d); want (1,5)", row, col)
	}
}

// TestCursorPosition_ThreadSafe verifies CursorPosition is safe under concurrent access.
func TestCursorPosition_ThreadSafe(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			v.Write([]byte("x"))
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		r, c := v.CursorPosition()
		_ = r
		_ = c
	}
	<-done
}
