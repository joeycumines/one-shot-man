package vt

import "testing"

// TestPutChar_AutoWrapFalse_OverwriteWideCharPlaceholder verifies that with
// AutoWrap=false, writing a narrow character onto the second-half placeholder
// of a wide character at the last two columns blanks the orphaned first half
// (column Cols-2) and writes the narrow char in place of the placeholder.
func TestPutChar_AutoWrapFalse_OverwriteWideCharPlaceholder(t *testing.T) {
	// Use a 10-column terminal. Write 8 ASCII chars then a wide char so the
	// wide char (width 2) occupies columns 8-9 (the last two columns).
	// Col 8 = first half, col 9 = placeholder.
	v := NewVTerm(1, 10)
	v.Write([]byte("abcdefgh\xe4\xb8\x96")) // 8 ASCII + 世 (U+4E16 = 3-byte UTF-8)

	scr := v.ActiveScreen()
	if scr.Cells[0][8].Ch != '世' || scr.Cells[0][9].SecondHalf != true {
		t.Fatalf("wide char not placed correctly at Cols-2/Cols-1: %c/%v",
			scr.Cells[0][8].Ch, scr.Cells[0][9].SecondHalf)
	}

	// Turn off AutoWrap and move cursor onto the placeholder at Cols-1 (= col 9).
	v.Write([]byte("\x1b[?7l")) // DECRST ?7l → AutoWrap=false
	v.Write([]byte("\x1b[1;10H")) // HVP row=1 col=10 (1-indexed) → 0-indexed col 9

	// Write a narrow char 'X' — it lands on the placeholder at col 9.
	v.Write([]byte("X"))

	// Re-read screen state after the Write
	scr = v.ActiveScreen()

	// Verify: placeholder at Cols-1 was overwritten with 'X'
	if scr.Cells[0][9].Ch != 'X' {
		t.Errorf("placeholder overwritten: cell[9] = %c, want X", scr.Cells[0][9].Ch)
	}
	// Verify: first half at Cols-2 was repaired to a space (not orphaned)
	if scr.Cells[0][8].Ch != ' ' {
		t.Errorf("orphaned first half not repaired: cell[8] = %c, want space", scr.Cells[0][8].Ch)
	}
}

// TestPutChar_AutoWrapFalse_OverwriteAtLastCol verifies that with AutoWrap=false,
// writing a narrow character when the cursor is already at the last column
// writes into the last column without wrapping or crashing, and the cursor
// stays at Cols-1.
func TestPutChar_AutoWrapFalse_OverwriteAtLastCol(t *testing.T) {
	v := NewVTerm(1, 10)

	// Turn off AutoWrap.
	v.Write([]byte("\x1b[?7l"))

	// Move cursor to last column (col 9, 0-indexed).
	v.Write([]byte("\x1b[1;10H"))

	// Write a narrow character.  With AutoWrap=false it should overwrite
	// col 9 in place without wrapping.
	v.Write([]byte("Z"))

	scr := v.ActiveScreen()

	// The last cell should now hold 'Z'.
	if scr.Cells[0][9].Ch != 'Z' {
		t.Errorf("last cell = %c, want Z", scr.Cells[0][9].Ch)
	}

	// Cursor must still be at Cols-1 (col 9).
	if scr.CurCol != 9 {
		t.Errorf("cursor col = %d, want 9 (no wrap)", scr.CurCol)
	}

	// Cursor must not have moved to the next row.
	if scr.CurRow != 0 {
		t.Errorf("cursor row = %d, want 0 (no wrap)", scr.CurRow)
	}

	// PendingWrap must be false.
	if scr.PendingWrap {
		t.Error("PendingWrap = true, want false")
	}
}
