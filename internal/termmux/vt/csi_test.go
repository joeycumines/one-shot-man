package vt

import "testing"

func TestCSI_CUP_MoveCursor(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// CUP row=5 col=10 (1-indexed) → 0-indexed 4,9
	h.Dispatch(scr, 'H', []int{5, 10}, false)
	if scr.CurRow != 4 || scr.CurCol != 9 {
		t.Fatalf("CUP: want (4,9), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCSI_CUP_DefaultHomePosition(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	scr.CurCol = 40
	h := &CSIHandler{}
	// CUP with no params defaults to (1,1) → 0-indexed (0,0)
	h.Dispatch(scr, 'H', nil, false)
	if scr.CurRow != 0 || scr.CurCol != 0 {
		t.Fatalf("CUP default: want (0,0), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCSI_CUU_CursorUp(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	h := &CSIHandler{}
	h.Dispatch(scr, 'A', []int{3}, false)
	if scr.CurRow != 7 {
		t.Fatalf("CUU: want row 7, got %d", scr.CurRow)
	}
}

func TestCSI_CUU_ClampsToZero(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 2
	h := &CSIHandler{}
	h.Dispatch(scr, 'A', []int{100}, false)
	if scr.CurRow != 0 {
		t.Fatalf("CUU clamp: want row 0, got %d", scr.CurRow)
	}
}

func TestCSI_SGR_ChangesAttributes(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// SGR bold
	h.Dispatch(scr, 'm', []int{1}, false)
	if !scr.CurAttr.Bold {
		t.Fatal("SGR: expected Bold=true")
	}
	// SGR reset
	h.Dispatch(scr, 'm', []int{0}, false)
	if scr.CurAttr.Bold {
		t.Fatal("SGR reset: expected Bold=false")
	}
}

func TestCSI_SGR_NoParams_Resets(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurAttr.Bold = true
	h := &CSIHandler{}
	// CSI m with empty params resets (our Dispatch injects [0])
	h.Dispatch(scr, 'm', nil, false)
	if scr.CurAttr.Bold {
		t.Fatal("SGR no params: expected reset")
	}
}

func TestCSI_DECSTBM_SetsScrollRegion(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	scr.CurCol = 5
	h := &CSIHandler{}
	h.Dispatch(scr, 'r', []int{5, 20}, false)
	if scr.ScrollTop != 5 || scr.ScrollBot != 20 {
		t.Fatalf("DECSTBM: want (5,20), got (%d,%d)", scr.ScrollTop, scr.ScrollBot)
	}
	// DECSTBM homes the cursor
	if scr.CurRow != 0 || scr.CurCol != 0 {
		t.Fatalf("DECSTBM home: want (0,0), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

func TestCSI_DECSET_CursorVisible(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// ?25l hides cursor
	h.Dispatch(scr, 'l', []int{25}, true)
	if scr.CursorVisible {
		t.Fatal("DECRST ?25l: expected CursorVisible=false")
	}
	// ?25h shows cursor
	h.Dispatch(scr, 'h', []int{25}, true)
	if !scr.CursorVisible {
		t.Fatal("DECSET ?25h: expected CursorVisible=true")
	}
}

func TestCSI_DECSET_AltScreen(t *testing.T) {
	scr := NewScreen(24, 80)
	var gotAlt *bool
	h := &CSIHandler{
		AltScreenFn: func(toAlt bool) {
			gotAlt = &toAlt
		},
	}
	// ?1049h activates alt screen
	h.Dispatch(scr, 'h', []int{1049}, true)
	if gotAlt == nil || !*gotAlt {
		t.Fatal("DECSET ?1049h: expected AltScreenFn(true)")
	}
	// ?1049l deactivates
	gotAlt = nil
	h.Dispatch(scr, 'l', []int{1049}, true)
	if gotAlt == nil || *gotAlt {
		t.Fatal("DECRST ?1049l: expected AltScreenFn(false)")
	}
}

func TestCSI_DECSET_NilCallback(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{} // no AltScreenFn
	// Must not panic
	h.Dispatch(scr, 'h', []int{1049}, true)
	h.Dispatch(scr, 'l', []int{1049}, true)
}

func TestCSI_ED_EraseDisplay(t *testing.T) {
	scr := NewScreen(4, 4)
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			scr.Cells[r][c].Ch = 'X'
		}
	}
	scr.CurRow = 2
	scr.CurCol = 0
	h := &CSIHandler{}
	// ED mode 0 = cursor to end
	h.Dispatch(scr, 'J', []int{0}, false)
	// Row 0-1 should be untouched
	if scr.Cells[0][0].Ch != 'X' {
		t.Fatal("ED 0: row 0 should be untouched")
	}
	// Row 2 col 0 onward should be blank
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatal("ED 0: row 2 should be erased")
	}
}

func TestCSI_SaveRestore_Cursor(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 10
	scr.CurAttr.Bold = true
	h := &CSIHandler{}
	// Save
	h.Dispatch(scr, 's', nil, false)
	// Move somewhere else
	scr.CurRow = 20
	scr.CurCol = 70
	scr.CurAttr.Bold = false
	// Restore
	h.Dispatch(scr, 'u', nil, false)
	if scr.CurRow != 5 || scr.CurCol != 10 || !scr.CurAttr.Bold {
		t.Fatalf("restore: want (5,10,Bold), got (%d,%d,Bold=%v)",
			scr.CurRow, scr.CurCol, scr.CurAttr.Bold)
	}
}

func TestCSI_UnknownFinal_NoPanic(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Must not panic on unrecognised byte
	h.Dispatch(scr, '~', []int{42}, false)
	h.Dispatch(scr, 'Z', nil, false)
	h.Dispatch(scr, 'q', []int{1, 2, 3}, true)
}

func TestParamDefault(t *testing.T) {
	tests := []struct {
		params []int
		idx    int
		def    int
		want   int
	}{
		{nil, 0, 1, 1},           // nil slice → default
		{[]int{}, 0, 1, 1},       // empty → default
		{[]int{0}, 0, 1, 1},      // explicit 0 → default
		{[]int{5}, 0, 1, 5},      // explicit value
		{[]int{5, 10}, 1, 1, 10}, // second param
		{[]int{5}, 3, 99, 99},    // index out of range → default
	}
	for _, tc := range tests {
		got := paramDefault(tc.params, tc.idx, tc.def)
		if got != tc.want {
			t.Errorf("paramDefault(%v, %d, %d) = %d, want %d",
				tc.params, tc.idx, tc.def, got, tc.want)
		}
	}
}

func TestCSI_CUF_CursorForward(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'C', []int{5}, false)
	if scr.CurCol != 5 {
		t.Fatalf("CUF: want col 5, got %d", scr.CurCol)
	}
}

func TestCSI_CUB_CursorBackward(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurCol = 10
	h := &CSIHandler{}
	h.Dispatch(scr, 'D', []int{3}, false)
	if scr.CurCol != 7 {
		t.Fatalf("CUB: want col 7, got %d", scr.CurCol)
	}
}

func TestCSI_VPA_VerticalPositionAbsolute(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'd', []int{12}, false)
	if scr.CurRow != 11 { // 1-indexed → 0-indexed
		t.Fatalf("VPA: want row 11, got %d", scr.CurRow)
	}
}

func TestCSI_CHA_CursorToColumn(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'G', []int{20}, false)
	if scr.CurCol != 19 { // 1-indexed → 0-indexed
		t.Fatalf("CHA: want col 19, got %d", scr.CurCol)
	}
}

func TestCSI_TBC_ClearTabStop(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Default tabs at 0, 8, 16...
	if !scr.TabStops[8] {
		t.Fatal("precondition: tab at col 8")
	}
	scr.CurCol = 8
	h.Dispatch(scr, 'g', []int{0}, false)
	if scr.TabStops[8] {
		t.Fatal("TBC mode 0: tab at col 8 should be cleared")
	}
	// Clear all
	h.Dispatch(scr, 'g', []int{3}, false)
	for i, ts := range scr.TabStops {
		if ts {
			t.Fatalf("TBC mode 3: tab at col %d should be cleared", i)
		}
	}
}

func TestCSI_DCH_DeleteChars(t *testing.T) {
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := &CSIHandler{}
	h.Dispatch(scr, 'P', []int{2}, false) // delete 2 chars at col 1
	got := rowString(scr, 0)
	if got != "ADE  " {
		t.Fatalf("DCH: want 'ADE  ', got %q", got)
	}
}

func TestCSI_ICH_InsertChars(t *testing.T) {
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := &CSIHandler{}
	h.Dispatch(scr, '@', []int{2}, false) // insert 2 blanks at col 1
	got := rowString(scr, 0)
	if got != "A  BC" {
		t.Fatalf("ICH: want 'A  BC', got %q", got)
	}
}

func TestCSI_ECH_EraseChars(t *testing.T) {
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	h := &CSIHandler{}
	h.Dispatch(scr, 'X', []int{2}, false)
	got := rowString(scr, 0)
	if got != "A  DE" {
		t.Fatalf("ECH: want 'A  DE', got %q", got)
	}
}

// rowString extracts a row from the screen as a string.
func rowString(scr *Screen, row int) string {
	runes := make([]rune, scr.Cols)
	for i, c := range scr.Cells[row] {
		runes[i] = c.Ch
	}
	return string(runes)
}

func TestESC_DECSC_DECRC(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 10
	scr.CurAttr.Italic = true
	h := &ESCHandler{}
	h.Dispatch(scr, '7') // save
	scr.CurRow = 20
	scr.CurCol = 70
	scr.CurAttr = Attr{}
	h.Dispatch(scr, '8') // restore
	if scr.CurRow != 5 || scr.CurCol != 10 || !scr.CurAttr.Italic {
		t.Fatalf("DECSC/RC: want (5,10,Italic), got (%d,%d,Italic=%v)",
			scr.CurRow, scr.CurCol, scr.CurAttr.Italic)
	}
}

func TestESC_RI_ReverseIndex(t *testing.T) {
	scr := NewScreen(5, 5)
	scr.CurRow = 0 // already at top
	h := &ESCHandler{}
	// Put content in row 0 to verify scroll
	scr.Cells[0][0].Ch = 'X'
	h.Dispatch(scr, 'M') // reverse index at top → scroll down
	// Row 0 should now be blank (scroll down pushed old row 0 to row 1)
	if scr.Cells[0][0].Ch != ' ' {
		t.Fatalf("RI scroll: row 0 should be blank, got %q", scr.Cells[0][0].Ch)
	}
	if scr.Cells[1][0].Ch != 'X' {
		t.Fatalf("RI scroll: row 1 should have 'X', got %q", scr.Cells[1][0].Ch)
	}
}

func TestESC_RIS_CallsResetFn(t *testing.T) {
	scr := NewScreen(24, 80)
	called := false
	h := &ESCHandler{ResetFn: func() { called = true }}
	h.Dispatch(scr, 'c')
	if !called {
		t.Fatal("RIS: expected ResetFn to be called")
	}
}

func TestESC_RIS_NilFn_NoPanic(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &ESCHandler{}
	h.Dispatch(scr, 'c') // must not panic
}

func TestESC_HTS_SetTabStop(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &ESCHandler{}
	scr.CurCol = 5
	// Col 5 is not a default tab stop
	if scr.TabStops[5] {
		t.Fatal("precondition: col 5 should not be a tab stop")
	}
	h.Dispatch(scr, 'H')
	if !scr.TabStops[5] {
		t.Fatal("HTS: col 5 should be a tab stop")
	}
}
