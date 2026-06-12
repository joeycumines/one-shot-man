package vt

import "testing"

func TestAutoWrap_DefaultTrue(t *testing.T) {
	s := NewScreen(24, 80)
	if !s.AutoWrap {
		t.Fatal("new screen AutoWrap = false, want true")
	}
}

func TestAutoWrap_DECSET(t *testing.T) {
	v := NewVTerm(24, 80)
	// Turn it off first
	v.Write([]byte("\x1b[?7l"))
	if v.primary.AutoWrap {
		t.Fatal("after DECRST ?7l: AutoWrap = true, want false")
	}
	// Turn it back on
	v.Write([]byte("\x1b[?7h"))
	if !v.primary.AutoWrap {
		t.Fatal("after DECSET ?7h: AutoWrap = false, want true")
	}
}

func TestAutoWrap_DECRST(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?7l"))
	if v.primary.AutoWrap {
		t.Fatal("after DECRST ?7l: AutoWrap = true, want false")
	}
}

func TestAutoWrap_Accessor(t *testing.T) {
	v := NewVTerm(24, 80)
	if !v.AutoWrap() {
		t.Fatal("default AutoWrap() = false, want true")
	}
	v.Write([]byte("\x1b[?7l"))
	if v.AutoWrap() {
		t.Fatal("after DECRST ?7l: AutoWrap() = true, want false")
	}
}

func TestAutoWrap_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?7l"))
	snap := v.ActiveScreen()
	if snap.AutoWrap {
		t.Fatal("Snapshot AutoWrap = true, want false")
	}
	snap.AutoWrap = true
	if v.primary.AutoWrap {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestAutoWrap_NoWrapAtMargin(t *testing.T) {
	// When AutoWrap is off, characters at the right margin overwrite the last column.
	v := NewVTerm(1, 5)
	v.Write([]byte("\x1b[?7l")) // turn off auto-wrap
	v.Write([]byte("ABCDE"))    // fills all 5 columns, no wrap
	if v.primary.CurCol != 4 {
		t.Fatalf("cursor at col %d, want 4 (last column)", v.primary.CurCol)
	}
	if v.primary.PendingWrap {
		t.Fatal("PendingWrap = true, want false (no wrap)")
	}
	// Write one more character — should overwrite col 4 without wrapping
	v.Write([]byte("F"))
	if v.primary.CurCol != 4 {
		t.Fatalf("after overwrite: cursor at col %d, want 4", v.primary.CurCol)
	}
	// The last cell should now be 'F'
	if v.primary.Cells[0][4].Ch != 'F' {
		t.Fatalf("last cell = %c, want F", v.primary.Cells[0][4].Ch)
	}
	// Verify no wrapping occurred (still on row 0)
	if v.primary.CurRow != 0 {
		t.Fatalf("cursor on row %d, want 0 (no wrap)", v.primary.CurRow)
	}
}

func TestAutoWrap_OverwriteMultipleChars(t *testing.T) {
	// Write 100 characters on a 10-column line with AutoWrap off.
	// All characters after the 10th should overwrite column 9.
	v := NewVTerm(1, 10)
	v.Write([]byte("\x1b[?7l"))
	v.Write([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")) // 26 chars
	// Cursor should be at column 9 (last column)
	if v.primary.CurCol != 9 {
		t.Fatalf("cursor at col %d, want 9", v.primary.CurCol)
	}
	// Last cell should be 'Z' (26th letter)
	if v.primary.Cells[0][9].Ch != 'Z' {
		t.Fatalf("last cell = %c, want Z", v.primary.Cells[0][9].Ch)
	}
	// First 9 cells should be A-I
	for i, want := range "ABCDEFGHI" {
		if v.primary.Cells[0][i].Ch != want {
			t.Errorf("cell[0][%d] = %c, want %c", i, v.primary.Cells[0][i].Ch, want)
		}
	}
}

func TestAutoWrap_WrapWhenEnabled(t *testing.T) {
	// Verify that AutoWrap=true (default) still wraps correctly.
	v := NewVTerm(2, 5)
	v.Write([]byte("ABCDE")) // fills row 0, sets PendingWrap
	if !v.primary.PendingWrap {
		t.Fatal("after filling row: PendingWrap = false, want true")
	}
	v.Write([]byte("F")) // triggers wrap to row 1
	if v.primary.CurRow != 1 {
		t.Fatalf("after wrap: CurRow = %d, want 1", v.primary.CurRow)
	}
	if v.primary.CurCol != 1 {
		t.Fatalf("after wrap: CurCol = %d, want 1", v.primary.CurCol)
	}
}

func TestAutoWrap_TurnOffClearsPendingWrap(t *testing.T) {
	// When DECAWM is turned off, PendingWrap should be cleared.
	v := NewVTerm(2, 5)
	v.Write([]byte("ABCDE")) // fills row, sets PendingWrap
	if !v.primary.PendingWrap {
		t.Fatal("PendingWrap should be true after filling row")
	}
	v.Write([]byte("\x1b[?7l")) // turn off auto-wrap
	if v.primary.PendingWrap {
		t.Fatal("after DECRST ?7l: PendingWrap = true, want false")
	}
}

func TestAutoWrap_NoWideCharWrapWhenOff(t *testing.T) {
	// When AutoWrap is off, wide characters at the margin should not wrap.
	v := NewVTerm(1, 5)
	v.Write([]byte("\x1b[?7l"))
	v.Write([]byte("AB")) // cursor at col 2
	v.Write([]byte("世"))  // wide char, needs 2 cols, but only cols 3-4 available
	// The wide char should be written starting at col 3
	if v.primary.CurCol != 4 {
		t.Fatalf("cursor at col %d, want 4", v.primary.CurCol)
	}
	if v.primary.PendingWrap {
		t.Fatal("PendingWrap = true, want false (no wrap)")
	}
}
