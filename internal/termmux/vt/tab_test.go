package vt

import "testing"

// --- CBT (CSI Ps Z) — Cursor Backward Tabulation ---

func TestCBT_BackwardOneTabStop(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Move cursor to col 16, then CBT 1 → col 8
	v.Write([]byte("\x1b[1;17H")) // CUP row=1 col=17 → (0,16)
	v.Write([]byte("\x1b[Z"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("col = %d; want 8", col)
	}
}

func TestCBT_BackwardMultipleTabStops(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Move cursor to col 24, then CBT 3 → col 0
	v.Write([]byte("\x1b[1;25H")) // CUP row=1 col=25 → (0,24)
	v.Write([]byte("\x1b[3Z"))
	_, col := v.CursorPosition()
	if col != 0 {
		t.Fatalf("col = %d; want 0", col)
	}
}

func TestCBT_AtColumnZero(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Cursor at col 0, CBT 1 → stays at 0
	v.Write([]byte("\x1b[1;1H")) // CUP row=1 col=1 → (0,0)
	v.Write([]byte("\x1b[Z"))
	_, col := v.CursorPosition()
	if col != 0 {
		t.Fatalf("col = %d; want 0", col)
	}
}

func TestCBT_DefaultParameter(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Move to col 16, CSI Z (no param) → backward 1 tab stop → col 8
	v.Write([]byte("\x1b[1;17H"))
	v.Write([]byte("\x1b[Z"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("col = %d; want 8", col)
	}
}

func TestCBT_NonDefaultTabPosition(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Cursor at col 12, CBT 1 → col 8 (previous 8-column tab stop)
	v.Write([]byte("\x1b[1;13H")) // CUP row=1 col=13 → (0,12)
	v.Write([]byte("\x1b[Z"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("col = %d; want 8", col)
	}
}

// --- CHT (CSI Ps I) — Cursor Horizontal Tabulation ---

func TestCHT_ForwardOneTabStop(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Cursor at col 0, CHT 1 → col 8
	v.Write([]byte("\x1b[I"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("col = %d; want 8", col)
	}
}

func TestCHT_ForwardMultipleTabStops(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Cursor at col 0, CHT 3 → col 24
	v.Write([]byte("\x1b[3I"))
	_, col := v.CursorPosition()
	if col != 24 {
		t.Fatalf("col = %d; want 24", col)
	}
}

func TestCHT_AtLastColumn(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Move to last column (79), CHT 1 → stays at 79
	v.Write([]byte("\x1b[1;80H")) // CUP row=1 col=80 → (0,79)
	v.Write([]byte("\x1b[I"))
	_, col := v.CursorPosition()
	if col != 79 {
		t.Fatalf("col = %d; want 79", col)
	}
}

func TestCHT_DefaultParameter(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Cursor at col 0, CSI I (no param) → forward 1 tab stop → col 8
	v.Write([]byte("\x1b[I"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("col = %d; want 8", col)
	}
}

func TestCHT_NonDefaultTabPosition(t *testing.T) {
	t.Parallel()
	v := NewVTerm(24, 80)
	// Cursor at col 5, CHT 1 → col 8 (next 8-column tab stop)
	v.Write([]byte("\x1b[1;6H")) // CUP row=1 col=6 → (0,5)
	v.Write([]byte("\x1b[I"))
	_, col := v.CursorPosition()
	if col != 8 {
		t.Fatalf("col = %d; want 8", col)
	}
}
