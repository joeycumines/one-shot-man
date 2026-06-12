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

// TestPendingWrap_ClearedByCursorMovement verifies that PendingWrap is cleared
// when an explicit cursor movement command is issued after writing to the last
// column. Without this fix, the next character would incorrectly wrap.
func TestPendingWrap_ClearedByCursorMovement(t *testing.T) {
	t.Parallel()

	t.Run("CUP", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("0123456789"))
		v.Write([]byte("\x1b[1;5H"))
		row, col := v.CursorPosition()
		if row != 0 || col != 4 {
			t.Fatalf("after CUP: (%d,%d), want (0,4)", row, col)
		}
		v.Write([]byte("X"))
		row, col = v.CursorPosition()
		if col != 5 {
			t.Fatalf("after write: col=%d, want 5 (not wrapped)", col)
		}
	})

	t.Run("CUU", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("\x1b[3;1H0123456789"))
		v.Write([]byte("\x1b[A"))
		row, _ := v.CursorPosition()
		if row != 1 {
			t.Fatalf("after CUU: row=%d, want 1", row)
		}
		v.Write([]byte("Y"))
		row, _ = v.CursorPosition()
		if row != 1 {
			t.Fatalf("after write: row=%d, want 1 (not wrapped)", row)
		}
	})

	t.Run("DECRC", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("\x1b7"))
		v.Write([]byte("0123456789"))
		v.Write([]byte("\x1b8"))
		row, col := v.CursorPosition()
		if row != 0 || col != 0 {
			t.Fatalf("after DECRC: (%d,%d), want (0,0)", row, col)
		}
		v.Write([]byte("W"))
		row, col = v.CursorPosition()
		if row != 0 || col != 1 {
			t.Fatalf("after write: (%d,%d), want (0,1) not wrapped", row, col)
		}
	})

	t.Run("RI", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("\x1b[3;1H0123456789"))
		v.Write([]byte("\x1bM"))
		row, _ := v.CursorPosition()
		if row != 1 {
			t.Fatalf("after RI: row=%d, want 1", row)
		}
		v.Write([]byte("V"))
		row, _ = v.CursorPosition()
		if row != 1 {
			t.Fatalf("after write: row=%d, want 1 (not wrapped)", row)
		}
	})

	t.Run("Resize", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("0123456789"))
		v.Resize(10, 20)
		row, col := v.CursorPosition()
		// With reflow, PendingWrap is accounted for: cursor is at
		// col 10 (one past the last char), valid on a 20-col screen.
		if row != 0 || col != 10 {
			t.Fatalf("after resize: (%d,%d), want (0,10)", row, col)
		}
		v.Write([]byte("R"))
		row, col = v.CursorPosition()
		if row != 0 || col != 11 {
			t.Fatalf("after write: (%d,%d), want (0,11) not wrapped", row, col)
		}
	})

	t.Run("CUP_no_wrap_then_write", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("0123456789"))
		v.Write([]byte("\x1b[2;1H"))
		v.Write([]byte("A"))
		row, col := v.CursorPosition()
		if row != 1 || col != 1 {
			t.Fatalf("after CUP+write: (%d,%d), want (1,1)", row, col)
		}
	})

	t.Run("DECSTBM", func(t *testing.T) {
		v := NewVTerm(10, 10)
		v.Write([]byte("0123456789"))
		// Set scroll region — moves cursor to (0,0) and must clear PendingWrap.
		v.Write([]byte("\x1b[1;5r"))
		row, col := v.CursorPosition()
		if row != 0 || col != 0 {
			t.Fatalf("after DECSTBM: (%d,%d), want (0,0)", row, col)
		}
		v.Write([]byte("X"))
		row, col = v.CursorPosition()
		if row != 0 || col != 1 {
			t.Fatalf("after write: (%d,%d), want (0,1) not wrapped", row, col)
		}
	})

	t.Run("IND", func(t *testing.T) {
		v := NewVTerm(5, 10)
		v.Write([]byte("\x1b[3;1H0123456789"))
		// ESC D (IND) is a line feed — must clear PendingWrap.
		v.Write([]byte("\x1bD"))
		row, _ := v.CursorPosition()
		if row != 3 {
			t.Fatalf("after IND: row=%d, want 3", row)
		}
		v.Write([]byte("Z"))
		row, _ = v.CursorPosition()
		if row != 3 {
			t.Fatalf("after write: row=%d, want 3 (not wrapped)", row)
		}
	})
}

// TestCursorRestore_BoundsChecking verifies that restoring a saved cursor
// position after a resize does not panic and clamps to valid screen bounds.
func TestCursorRestore_BoundsChecking(t *testing.T) {
	t.Parallel()

	t.Run("CSI_u_after_resize", func(t *testing.T) {
		v := NewVTerm(24, 80)
		// Move cursor to (20, 70) and save
		v.Write([]byte("\x1b[21;71H"))
		v.Write([]byte("\x1b[s"))
		// Resize smaller
		v.Resize(10, 40)
		// Restore cursor — should clamp to (9, 39), not panic
		v.Write([]byte("\x1b[u"))
		row, col := v.CursorPosition()
		if row != 9 {
			t.Fatalf("row=%d, want 9 (clamped)", row)
		}
		if col != 39 {
			t.Fatalf("col=%d, want 39 (clamped)", col)
		}
	})

	t.Run("DECRC_after_resize", func(t *testing.T) {
		v := NewVTerm(24, 80)
		// Move cursor and save via ESC 7
		v.Write([]byte("\x1b[21;71H"))
		v.Write([]byte("\x1b7"))
		// Resize smaller
		v.Resize(10, 40)
		// Restore via ESC 8 — should clamp, not panic
		v.Write([]byte("\x1b8"))
		row, col := v.CursorPosition()
		if row != 9 {
			t.Fatalf("row=%d, want 9 (clamped)", row)
		}
		if col != 39 {
			t.Fatalf("col=%d, want 39 (clamped)", col)
		}
	})

	t.Run("switchToPrimary_after_resize", func(t *testing.T) {
		v := NewVTerm(24, 80)
		// Switch to alt screen
		v.Write([]byte("\x1b[?1049h"))
		// The primary screen had cursor saved at switch time
		// Resize while on alt screen
		v.Resize(10, 40)
		// Switch back to primary — should not panic
		v.Write([]byte("\x1b[?1049l"))
		row, col := v.CursorPosition()
		// Cursor should be clamped to valid bounds
		if row < 0 || row >= 10 {
			t.Fatalf("row=%d, out of bounds [0,9]", row)
		}
		if col < 0 || col >= 40 {
			t.Fatalf("col=%d, out of bounds [0,39]", col)
		}
	})
}
