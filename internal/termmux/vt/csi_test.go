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
		AltScreenFn: func(toAlt bool, mode int) {
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

func TestCSI_AltScreen_Mode47_NoCursorSave(t *testing.T) {
	v := NewVTerm(24, 80)
	// Move cursor to a known position
	v.Write([]byte("\x1b[10;30H")) // row 10, col 30

	// Switch to alt screen with mode 47
	v.Write([]byte("\x1b[?47h"))

	// Move cursor on alt screen
	v.Write([]byte("\x1b[20;60H"))

	// Switch back to primary with mode 47
	v.Write([]byte("\x1b[?47l"))

	// Mode 47 does NOT restore cursor — the primary screen's cursor should
	// still be at its original position since switchToPrimary(47) doesn't
	// modify cursor. The primary screen cursor was at (9,29) when we left.
	row, col := v.CursorPosition()
	if row != 9 || col != 29 {
		t.Fatalf("cursor after mode 47 round-trip: row=%d col=%d; want 9,29 (no restore, stays where it was)", row, col)
	}
}

func TestCSI_AltScreen_Mode1047_ClearOnExit(t *testing.T) {
	v := NewVTerm(24, 80)
	// Write content to alt screen
	v.Write([]byte("\x1b[?1047h"))
	v.Write([]byte("AltScreen content"))

	// Switch back to primary with mode 1047 — should clear the alt screen on exit
	v.Write([]byte("\x1b[?1047l"))

	// Switch to alt again — content should be gone (cleared on exit)
	v.Write([]byte("\x1b[?1047h"))
	s := v.String()
	if s != "" {
		t.Fatalf("alt screen not cleared on mode 1047 exit: %q", s)
	}
}

func TestCSI_AltScreen_Mode1047_NoClearOnEntry(t *testing.T) {
	v := NewVTerm(24, 80)
	// Write content to alt screen
	v.Write([]byte("\x1b[?1047h"))
	v.Write([]byte("Preserved content"))
	v.Write([]byte("\x1b[?47l")) // exit with mode 47 (no clear)

	// Re-enter with mode 1047 — should NOT clear on entry
	v.Write([]byte("\x1b[?1047h"))
	s := v.String()
	if s != "Preserved content" {
		t.Fatalf("mode 1047 cleared alt screen on entry: %q; want %q", s, "Preserved content")
	}
}

func TestCSI_AltScreen_Mode1047_NoCursorRestore(t *testing.T) {
	v := NewVTerm(24, 80)
	// Move cursor to known position
	v.Write([]byte("\x1b[10;30H"))

	// Switch to alt with mode 1047
	v.Write([]byte("\x1b[?1047h"))
	// Move cursor on alt screen
	v.Write([]byte("\x1b[20;60H"))
	// Switch back with mode 1047 — no cursor restore
	v.Write([]byte("\x1b[?1047l"))

	// Mode 1047 does NOT restore cursor — primary cursor stays where it was (9,29)
	row, col := v.CursorPosition()
	if row != 9 || col != 29 {
		t.Fatalf("cursor after mode 1047 round-trip: row=%d col=%d; want 9,29 (no restore)", row, col)
	}
}

func TestCSI_AltScreen_Mode1049_SaveAndRestore(t *testing.T) {
	v := NewVTerm(24, 80)
	// Move cursor to known position
	v.Write([]byte("\x1b[10;30H"))
	row, col := v.CursorPosition()
	if row != 9 || col != 29 {
		t.Fatalf("cursor before alt: row=%d col=%d; want 9,29", row, col)
	}

	// Switch to alt with mode 1049 — should save cursor
	v.Write([]byte("\x1b[?1049h"))

	// Move cursor on alt screen
	v.Write([]byte("\x1b[20;60H"))

	// Switch back with mode 1049 — should restore cursor
	v.Write([]byte("\x1b[?1049l"))
	row, col = v.CursorPosition()
	if row != 9 || col != 29 {
		t.Fatalf("cursor after restore: row=%d col=%d; want 9,29", row, col)
	}
}

func TestCSI_AltScreen_Mode1049_ClearOnEntry(t *testing.T) {
	v := NewVTerm(24, 80)
	// Write to alt screen
	v.Write([]byte("\x1b[?1049h"))
	v.Write([]byte("Previous content"))
	v.Write([]byte("\x1b[?1049l"))

	// Switch again — alt screen should be cleared on entry
	v.Write([]byte("\x1b[?1049h"))
	s := v.String()
	if s != "" {
		t.Fatalf("alt screen not cleared on mode 1049 entry: %q", s)
	}
}

func TestCSI_AltScreen_Mode1049_SavesCharsetState(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set charset state
	v.Write([]byte("\x1b(0")) // line-drawing
	snap := v.ActiveScreen()
	if snap.G0Charset != 1 {
		t.Fatalf("G0Charset = %d; want 1 (line-drawing)", snap.G0Charset)
	}

	// Switch to alt screen with mode 1049
	v.Write([]byte("\x1b[?1049h"))

	// Switch back — charset should be restored
	v.Write([]byte("\x1b[?1049l"))
	snap = v.ActiveScreen()
	if snap.G0Charset != 1 {
		t.Fatalf("G0Charset after restore = %d; want 1 (line-drawing)", snap.G0Charset)
	}
}

func TestCSI_AltScreen_Mode1049_CursorHomesOnEntry(t *testing.T) {
	v := NewVTerm(24, 80)
	// Move cursor to known position
	v.Write([]byte("\x1b[10;30H"))

	// Switch to alt with mode 1049 — cursor should home to (0,0) on alt screen
	v.Write([]byte("\x1b[?1049h"))
	row, col := v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("cursor on alt screen: row=%d col=%d; want 0,0", row, col)
	}
}

func TestCSI_AltScreen_MultiParam(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotMode int
	// Test that multi-param DECSET works with alt screen mode
	// This tests the `p` variable being passed to AltScreenFn
	origHandler := v.csi.AltScreenFn
	v.csi.AltScreenFn = func(toAlt bool, mode int) {
		gotMode = mode
		origHandler(toAlt, mode)
	}

	// DECSET ?47;1049h — each mode should be processed separately
	// Note: in multi-param, p iterates through params, so both 47 and 1049
	// get dispatched. The last one wins for the switchToAlt call.
	v.Write([]byte("\x1b[?47;1049h"))

	// The last mode processed should be 1049
	if gotMode != 1049 {
		t.Fatalf("gotMode = %d; want 1049", gotMode)
	}
}

func TestCSI_AltScreen_Mode47_SwitchWithoutClear(t *testing.T) {
	v := NewVTerm(24, 80)
	// Write content to alt screen
	v.Write([]byte("\x1b[?1049h"))
	v.Write([]byte("Preserved content"))
	v.Write([]byte("\x1b[?1049l"))

	// Switch to alt with mode 47 — should NOT clear the alt screen
	v.Write([]byte("\x1b[?47h"))
	s := v.String()
	if s != "Preserved content" {
		t.Fatalf("mode 47 cleared alt screen: %q; want %q", s, "Preserved content")
	}
}

func TestCSI_ED_EraseDisplay(t *testing.T) {
	scr := NewScreen(4, 4)
	for r := range 4 {
		for c := range 4 {
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

// -- Mouse tracking mode tests -------------------------------------------------

func TestCSI_DECSET_MouseTrackingBasic(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'h', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingBasic {
		t.Fatalf("DECSET ?1000h: want MouseTrackingBasic, got %d", scr.MouseTracking)
	}
}

func TestCSI_DECSET_MouseTrackingButtonEvent(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'h', []int{1002}, true)
	if scr.MouseTracking != MouseTrackingButtonEvent {
		t.Fatalf("DECSET ?1002h: want MouseTrackingButtonEvent, got %d", scr.MouseTracking)
	}
}

func TestCSI_DECSET_MouseTrackingAnyEvent(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'h', []int{1003}, true)
	if scr.MouseTracking != MouseTrackingAnyEvent {
		t.Fatalf("DECSET ?1003h: want MouseTrackingAnyEvent, got %d", scr.MouseTracking)
	}
}

func TestCSI_DECSET_MouseSGR(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	h.Dispatch(scr, 'h', []int{1006}, true)
	if !scr.MouseSGR {
		t.Fatal("DECSET ?1006h: expected MouseSGR=true")
	}
}

func TestCSI_DECRST_MouseTrackingBasic_ClearsOnlyBasic(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Enable button-event tracking first
	h.Dispatch(scr, 'h', []int{1002}, true)
	if scr.MouseTracking != MouseTrackingButtonEvent {
		t.Fatalf("precondition: want ButtonEvent, got %d", scr.MouseTracking)
	}
	// DECRST ?1000l should NOT clear ButtonEvent (only clears Basic)
	h.Dispatch(scr, 'l', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingButtonEvent {
		t.Fatalf("DECRST ?1000l with ButtonEvent: should remain ButtonEvent, got %d", scr.MouseTracking)
	}
	// DECRST ?1002l should clear ButtonEvent
	h.Dispatch(scr, 'l', []int{1002}, true)
	if scr.MouseTracking != MouseTrackingNone {
		t.Fatalf("DECRST ?1002l: want None, got %d", scr.MouseTracking)
	}
}

func TestCSI_DECRST_MouseTrackingAnyEvent_ClearsOnlyAny(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Enable any-event tracking
	h.Dispatch(scr, 'h', []int{1003}, true)
	if scr.MouseTracking != MouseTrackingAnyEvent {
		t.Fatalf("precondition: want AnyEvent, got %d", scr.MouseTracking)
	}
	// DECRST ?1000l should NOT clear AnyEvent
	h.Dispatch(scr, 'l', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingAnyEvent {
		t.Fatalf("DECRST ?1000l with AnyEvent: should remain AnyEvent, got %d", scr.MouseTracking)
	}
	// DECRST ?1003l should clear AnyEvent
	h.Dispatch(scr, 'l', []int{1003}, true)
	if scr.MouseTracking != MouseTrackingNone {
		t.Fatalf("DECRST ?1003l: want None, got %d", scr.MouseTracking)
	}
}

func TestCSI_DECRST_MouseSGR(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Enable SGR
	h.Dispatch(scr, 'h', []int{1006}, true)
	if !scr.MouseSGR {
		t.Fatal("precondition: expected MouseSGR=true")
	}
	// Disable SGR
	h.Dispatch(scr, 'l', []int{1006}, true)
	if scr.MouseSGR {
		t.Fatal("DECRST ?1006l: expected MouseSGR=false")
	}
}

func TestCSI_DECRST_MouseTrackingBasic_ClearsBasic(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Enable basic tracking
	h.Dispatch(scr, 'h', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingBasic {
		t.Fatalf("precondition: want Basic, got %d", scr.MouseTracking)
	}
	// DECRST ?1000l should clear Basic
	h.Dispatch(scr, 'l', []int{1000}, true)
	if scr.MouseTracking != MouseTrackingNone {
		t.Fatalf("DECRST ?1000l: want None, got %d", scr.MouseTracking)
	}
}

func TestVTerm_MouseTrackingAccessor(t *testing.T) {
	v := NewVTerm(24, 80)
	// Initially no tracking
	if mode := v.MouseTracking(); mode != MouseTrackingNone {
		t.Fatalf("initial: want None, got %d", mode)
	}
	// Write DECSET ?1002h
	v.Write([]byte("\x1b[?1002h"))
	if mode := v.MouseTracking(); mode != MouseTrackingButtonEvent {
		t.Fatalf("after DECSET ?1002h: want ButtonEvent, got %d", mode)
	}
	// Write DECRST ?1002l
	v.Write([]byte("\x1b[?1002l"))
	if mode := v.MouseTracking(); mode != MouseTrackingNone {
		t.Fatalf("after DECRST ?1002l: want None, got %d", mode)
	}
}

func TestVTerm_MouseSGRAccessor(t *testing.T) {
	v := NewVTerm(24, 80)
	// Initially no SGR
	if v.MouseSGR() {
		t.Fatal("initial: expected MouseSGR=false")
	}
	// Write DECSET ?1006h
	v.Write([]byte("\x1b[?1006h"))
	if !v.MouseSGR() {
		t.Fatal("after DECSET ?1006h: expected MouseSGR=true")
	}
	// Write DECRST ?1006l
	v.Write([]byte("\x1b[?1006l"))
	if v.MouseSGR() {
		t.Fatal("after DECRST ?1006l: expected MouseSGR=false")
	}
}

// -- Insert Mode (IRM, ANSI mode 4) tests ----------------------------------------

func TestCSI_SM_IRM_Enable(t *testing.T) {
	scr := NewScreen(24, 80)
	if scr.InsertMode {
		t.Fatal("precondition: InsertMode should be false")
	}
	h := &CSIHandler{}
	// CSI 4h = SM (set mode) with mode 4 (IRM)
	h.Dispatch(scr, 'h', []int{4}, false)
	if !scr.InsertMode {
		t.Fatal("SM 4h: expected InsertMode=true")
	}
}

func TestCSI_RM_IRM_Disable(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.InsertMode = true
	h := &CSIHandler{}
	// CSI 4l = RM (reset mode) with mode 4 (IRM)
	h.Dispatch(scr, 'l', []int{4}, false)
	if scr.InsertMode {
		t.Fatal("RM 4l: expected InsertMode=false")
	}
}

func TestCSI_SM_IRM_PrivateModeNotAffected(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// CSI ?4h (private mode 4) should NOT set InsertMode
	h.Dispatch(scr, 'h', []int{4}, true)
	if scr.InsertMode {
		t.Fatal("private ?4h: should not set InsertMode (IRM is non-private mode 4)")
	}
}

func TestCSI_SM_IRM_MultipleParams(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// CSI 4;7h — mode 4 should be set, mode 7 (unknown) silently ignored
	h.Dispatch(scr, 'h', []int{4, 7}, false)
	if !scr.InsertMode {
		t.Fatal("SM 4;7h: expected InsertMode=true (mode 4 set)")
	}
}

func TestCSI_PutChar_InsertMode_ShiftsRight(t *testing.T) {
	scr := NewScreen(1, 10)
	// Pre-fill: "ABCDE     "
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2 // cursor at 'C'
	scr.InsertMode = true
	scr.PutChar('X')
	got := rowString(scr, 0)
	// 'X' inserted at col 2, shifting C/D/E right; 'E' pushed off
	if got != "ABXCDE    " {
		t.Fatalf("insert mode PutChar: want 'ABXCDE    ', got %q", got)
	}
}

func TestCSI_PutChar_InsertMode_OverwriteWhenOff(t *testing.T) {
	scr := NewScreen(1, 10)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2
	scr.InsertMode = false
	scr.PutChar('X')
	got := rowString(scr, 0)
	// Overwrite mode: 'X' replaces 'C'
	if got != "ABXDE     " {
		t.Fatalf("overwrite PutChar: want 'ABXDE     ', got %q", got)
	}
}

func TestCSI_PutChar_InsertMode_WideChar(t *testing.T) {
	scr := NewScreen(1, 10)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 1
	scr.InsertMode = true
	// Wide char (width=2) in insert mode should insert 2 cells (shift right by 2)
	scr.PutChar('世')
	// After insert: '世' at col 1, SecondHalf placeholder at col 2 (Ch=0),
	// then B/C/D/E shifted right by 2 (cols 3-6), E was at col 4 before,
	// now at col 6 — still fits in 10-col line.
	row := scr.Cells[0]
	if row[0].Ch != 'A' {
		t.Fatalf("col 0: want 'A', got %q", row[0].Ch)
	}
	if row[1].Ch != '世' {
		t.Fatalf("col 1: want '世', got %q", row[1].Ch)
	}
	if !row[2].SecondHalf {
		t.Fatal("col 2: expected SecondHalf placeholder")
	}
	if row[3].Ch != 'B' {
		t.Fatalf("col 3: want 'B', got %q", row[3].Ch)
	}
	if row[4].Ch != 'C' {
		t.Fatalf("col 4: want 'C', got %q", row[4].Ch)
	}
	if row[5].Ch != 'D' {
		t.Fatalf("col 5: want 'D', got %q", row[5].Ch)
	}
	if row[6].Ch != 'E' {
		t.Fatalf("col 6: want 'E', got %q", row[6].Ch)
	}
}

func TestVTerm_InsertModeAccessor(t *testing.T) {
	v := NewVTerm(24, 80)
	// Initially insert mode is off
	if v.InsertMode() {
		t.Fatal("initial: expected InsertMode=false")
	}
	// Enable via CSI 4h
	v.Write([]byte("\x1b[4h"))
	if !v.InsertMode() {
		t.Fatal("after CSI 4h: expected InsertMode=true")
	}
	// Disable via CSI 4l
	v.Write([]byte("\x1b[4l"))
	if v.InsertMode() {
		t.Fatal("after CSI 4l: expected InsertMode=false")
	}
}

func TestVTerm_InsertMode_PutCharViaWrite(t *testing.T) {
	v := NewVTerm(1, 10)
	// Write "ABCDE" first
	v.Write([]byte("ABCDE"))
	// Move cursor to column 2 (0-indexed)
	v.Write([]byte("\x1b[1;3H"))
	// Enable insert mode
	v.Write([]byte("\x1b[4h"))
	// Write 'X' — should insert, not overwrite
	v.Write([]byte("X"))
	snap := v.ActiveScreen()
	got := rowString(snap, 0)
	if got != "ABXCDE    " {
		t.Fatalf("insert mode via Write: want 'ABXCDE    ', got %q", got)
	}
}

func TestVTerm_InsertMode_RISResets(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[4h"))
	if !v.InsertMode() {
		t.Fatal("after CSI 4h: expected InsertMode=true")
	}
	// RIS (ESC c) should reset insert mode
	v.Write([]byte("\x1bc"))
	if v.InsertMode() {
		t.Fatal("after RIS: expected InsertMode=false")
	}
}

func TestScreen_Snapshot_InsertMode(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.InsertMode = true
	snap := scr.Snapshot()
	if !snap.InsertMode {
		t.Fatal("snapshot: expected InsertMode=true")
	}
	// Mutating snapshot should not affect original
	snap.InsertMode = false
	if !scr.InsertMode {
		t.Fatal("snapshot isolation: original InsertMode should still be true")
	}
}

func TestVTerm_MouseTrackingModeUpgrade(t *testing.T) {
	v := NewVTerm(24, 80)
	// Upgrade: Basic → ButtonEvent
	v.Write([]byte("\x1b[?1000h"))
	if mode := v.MouseTracking(); mode != MouseTrackingBasic {
		t.Fatalf("after DECSET ?1000h: want Basic, got %d", mode)
	}
	v.Write([]byte("\x1b[?1002h"))
	if mode := v.MouseTracking(); mode != MouseTrackingButtonEvent {
		t.Fatalf("after DECSET ?1002h upgrade: want ButtonEvent, got %d", mode)
	}
	// Upgrade: ButtonEvent → AnyEvent
	v.Write([]byte("\x1b[?1003h"))
	if mode := v.MouseTracking(); mode != MouseTrackingAnyEvent {
		t.Fatalf("after DECSET ?1003h upgrade: want AnyEvent, got %d", mode)
	}
	// Downgrade: DECRST ?1002l should NOT clear AnyEvent
	v.Write([]byte("\x1b[?1002l"))
	if mode := v.MouseTracking(); mode != MouseTrackingAnyEvent {
		t.Fatalf("after DECRST ?1002l with AnyEvent: should remain AnyEvent, got %d", mode)
	}
	// Only DECRST ?1003l clears AnyEvent
	v.Write([]byte("\x1b[?1003l"))
	if mode := v.MouseTracking(); mode != MouseTrackingNone {
		t.Fatalf("after DECRST ?1003l: want None, got %d", mode)
	}
}

// -- Character Set (VT100 line-drawing) tests -----------------------------------

func TestVTerm_CharsetDesignation_G0_LineDrawing(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b(0"))
	snap := v.ActiveScreen()
	if snap.G0Charset != 1 {
		t.Fatalf("after ESC(0: want G0Charset=1 (line-drawing), got %d", snap.G0Charset)
	}
	if snap.GL != 0 {
		t.Fatalf("after ESC(0: want GL=0, got %d", snap.GL)
	}
}

func TestVTerm_CharsetDesignation_G0_ASCII(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b(0"))
	v.Write([]byte("\x1b(B"))
	snap := v.ActiveScreen()
	if snap.G0Charset != 0 {
		t.Fatalf("after ESC(B: want G0Charset=0 (ASCII), got %d", snap.G0Charset)
	}
}

func TestVTerm_CharsetDesignation_G1_LineDrawing(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b)0"))
	snap := v.ActiveScreen()
	if snap.G1Charset != 1 {
		t.Fatalf("after ESC)0: want G1Charset=1 (line-drawing), got %d", snap.G1Charset)
	}
	if snap.G0Charset != 0 {
		t.Fatalf("after ESC)0: want G0Charset=0 (ASCII), got %d", snap.G0Charset)
	}
}

func TestVTerm_LineDrawing_PutChar(t *testing.T) {
	v := NewVTerm(1, 40)
	v.Write([]byte("\x1b(0"))
	v.Write([]byte("qx"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch != '─' {
		t.Fatalf("line-drawing 'q': want '─', got %q", snap.Cells[0][0].Ch)
	}
	if snap.Cells[0][1].Ch != '│' {
		t.Fatalf("line-drawing 'x': want '│', got %q", snap.Cells[0][1].Ch)
	}
}

func TestVTerm_LineDrawing_RestoreASCII(t *testing.T) {
	v := NewVTerm(1, 40)
	v.Write([]byte("\x1b(0q\x1b(Bq"))
	snap := v.ActiveScreen()
	if snap.Cells[0][0].Ch != '─' {
		t.Fatalf("first 'q' in line-drawing: want '─', got %q", snap.Cells[0][0].Ch)
	}
	if snap.Cells[0][1].Ch != 'q' {
		t.Fatalf("second 'q' in ASCII: want 'q', got %q", snap.Cells[0][1].Ch)
	}
}

func TestVTerm_ShiftOut_ShiftIn(t *testing.T) {
	v := NewVTerm(1, 40)
	v.Write([]byte("\x1b(B\x1b)0"))
	v.Write([]byte{0x0E}) // SO → GL=1
	snap := v.ActiveScreen()
	if snap.GL != 1 {
		t.Fatalf("after SO: want GL=1, got %d", snap.GL)
	}
	v.Write([]byte("q"))
	if v.ActiveScreen().Cells[0][0].Ch != '─' {
		t.Fatalf("G1 line-drawing 'q': want '─', got %q", v.ActiveScreen().Cells[0][0].Ch)
	}
	v.Write([]byte{0x0F}) // SI → GL=0
	snap = v.ActiveScreen()
	if snap.GL != 0 {
		t.Fatalf("after SI: want GL=0, got %d", snap.GL)
	}
	v.Write([]byte("q"))
	if v.ActiveScreen().Cells[0][1].Ch != 'q' {
		t.Fatalf("G0 ASCII 'q': want 'q', got %q", v.ActiveScreen().Cells[0][1].Ch)
	}
}

func TestVTerm_LineDrawing_AllMappings(t *testing.T) {
	v := NewVTerm(1, 80)
	v.Write([]byte("\x1b(0"))
	tests := []struct {
		input  byte
		expect rune
	}{
		{'q', '─'}, {'x', '│'}, {'l', '┌'}, {'m', '└'},
		{'k', '┐'}, {'j', '┘'}, {'t', '├'}, {'u', '┤'},
		{'w', '┬'}, {'v', '┴'}, {'n', '┼'},
	}
	for i, tc := range tests {
		v.Write([]byte{tc.input})
		got := v.ActiveScreen().Cells[0][i].Ch
		if got != tc.expect {
			t.Fatalf("line-drawing %c: want %q, got %q", tc.input, tc.expect, got)
		}
	}
}

func TestVTerm_CharsetDesignation_RISReset(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b(0"))
	if v.ActiveScreen().G0Charset != 1 {
		t.Fatal("after ESC(0: expected G0Charset=1")
	}
	v.Write([]byte("\x1bc"))
	snap := v.ActiveScreen()
	if snap.G0Charset != 0 {
		t.Fatalf("after RIS: want G0Charset=0, got %d", snap.G0Charset)
	}
	if snap.G1Charset != 0 {
		t.Fatalf("after RIS: want G1Charset=0, got %d", snap.G1Charset)
	}
	if snap.GL != 0 {
		t.Fatalf("after RIS: want GL=0, got %d", snap.GL)
	}
}

func TestVTerm_Charset_SnapshotIsolation(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b(0"))
	snap := v.ActiveScreen()
	if snap.G0Charset != 1 {
		t.Fatal("snapshot: expected G0Charset=1")
	}
	snap.G0Charset = 99
	if v.ActiveScreen().G0Charset != 1 {
		t.Fatal("snapshot isolation: VTerm G0Charset should still be 1")
	}
}

func TestVTerm_Charset_LineDrawingUnknownCharPassthrough(t *testing.T) {
	v := NewVTerm(1, 40)
	v.Write([]byte("\x1b(0"))
	v.Write([]byte("A"))
	if v.ActiveScreen().Cells[0][0].Ch != 'A' {
		t.Fatalf("unknown line-drawing char: want 'A', got %q", v.ActiveScreen().Cells[0][0].Ch)
	}
}

func TestVTerm_LineDrawing_ControlPictureChars(t *testing.T) {
	// Verify that control-picture characters produce visible symbols, not
	// literal control characters. These are the most commonly broken entries.
	v := NewVTerm(1, 40)
	v.Write([]byte("\x1b(0"))
	tests := []struct {
		input  byte
		expect rune
	}{
		{'b', '␉'}, // HT symbol, not \t
		{'c', '␌'}, // FF symbol, not \x0c
		{'d', '␍'}, // CR symbol, not \r
		{'e', '␊'}, // LF symbol, not \n
		{'h', '␤'}, // NL symbol, not \n
		{'i', '␋'}, // VT symbol, not \x0b
	}
	for i, tc := range tests {
		v.Write([]byte{tc.input})
		got := v.ActiveScreen().Cells[0][i].Ch
		if got != tc.expect {
			t.Fatalf("control-picture %c: want %q (U+%04X), got %q (U+%04X)",
				tc.input, tc.expect, tc.expect, got, got)
		}
	}
}

func TestESC_DECSC_DECRC_CharsetSaveRestore(t *testing.T) {
	v := NewVTerm(1, 40)
	// Set G0 to line-drawing and GL to G0
	v.Write([]byte("\x1b(0"))
	// Save cursor
	v.Write([]byte("\x1b7"))
	// Switch back to ASCII
	v.Write([]byte("\x1b(B"))
	// Verify G0 is now ASCII
	if v.ActiveScreen().G0Charset != 0 {
		t.Fatalf("after ESC(B: want G0Charset=0, got %d", v.ActiveScreen().G0Charset)
	}
	// Restore cursor — should restore charset state
	v.Write([]byte("\x1b8"))
	snap := v.ActiveScreen()
	if snap.G0Charset != 1 {
		t.Fatalf("after DECRC: want G0Charset=1 (line-drawing restored), got %d", snap.G0Charset)
	}
	if snap.G1Charset != 0 {
		t.Fatalf("after DECRC: want G1Charset=0, got %d", snap.G1Charset)
	}
	if snap.GL != 0 {
		t.Fatalf("after DECRC: want GL=0, got %d", snap.GL)
	}
}

func TestESC_DECSC_DECRC_CharsetWithGL1(t *testing.T) {
	v := NewVTerm(1, 40)
	// G0=ASCII, G1=line-drawing, GL=G1
	v.Write([]byte("\x1b(B\x1b)0\x0e")) // SO shifts to G1
	if v.ActiveScreen().GL != 1 {
		t.Fatal("precondition: GL should be 1")
	}
	// Save cursor
	v.Write([]byte("\x1b7"))
	// Shift back to G0
	v.Write([]byte("\x0f")) // SI
	if v.ActiveScreen().GL != 0 {
		t.Fatal("after SI: GL should be 0")
	}
	// Restore cursor — should restore GL to 1
	v.Write([]byte("\x1b8"))
	snap := v.ActiveScreen()
	if snap.GL != 1 {
		t.Fatalf("after DECRC: want GL=1, got %d", snap.GL)
	}
	if snap.G1Charset != 1 {
		t.Fatalf("after DECRC: want G1Charset=1, got %d", snap.G1Charset)
	}
}

func TestCSI_SaveRestoreCursor_CharsetState(t *testing.T) {
	v := NewVTerm(1, 40)
	// Set G0 to line-drawing
	v.Write([]byte("\x1b(0"))
	// Save cursor via CSI s
	v.Write([]byte("\x1b[s"))
	// Switch G0 to ASCII
	v.Write([]byte("\x1b(B"))
	if v.ActiveScreen().G0Charset != 0 {
		t.Fatal("after ESC(B: G0 should be ASCII")
	}
	// Restore cursor via CSI u
	v.Write([]byte("\x1b[u"))
	if v.ActiveScreen().G0Charset != 1 {
		t.Fatalf("after CSI u: want G0Charset=1 (line-drawing restored), got %d", v.ActiveScreen().G0Charset)
	}
}

func TestCSI_DECSET_OriginMode(t *testing.T) {
	v := NewVTerm(24, 80)
	// Enable origin mode
	v.Write([]byte("\x1b[?6h"))
	if !v.ActiveScreen().OriginMode {
		t.Fatal("after DECSET ?6h: expected OriginMode=true")
	}
	// Disable origin mode
	v.Write([]byte("\x1b[?6l"))
	if v.ActiveScreen().OriginMode {
		t.Fatal("after DECRST ?6l: expected OriginMode=false")
	}
}

func TestCSI_CUP_OriginMode_RelativeToScrollRegion(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Enable origin mode
	v.Write([]byte("\x1b[?6h"))
	// CUP row 1, col 1 — should go to scroll region top (row 4, 0-indexed)
	v.Write([]byte("\x1b[1;1H"))
	row, col := v.CursorPosition()
	if row != 4 || col != 0 {
		t.Fatalf("CUP in origin mode: want (4,0), got (%d,%d)", row, col)
	}
	// CUP row 3, col 5 — should be scroll region top + 2 (row 6), col 4
	v.Write([]byte("\x1b[3;5H"))
	row, col = v.CursorPosition()
	if row != 6 || col != 4 {
		t.Fatalf("CUP in origin mode: want (6,4), got (%d,%d)", row, col)
	}
}

func TestCSI_CUP_OriginMode_ClampedToScrollRegion(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Enable origin mode
	v.Write([]byte("\x1b[?6h"))
	// CUP row 100 — should clamp to scroll region bottom (row 14, 0-indexed)
	v.Write([]byte("\x1b[100;1H"))
	row, _ := v.CursorPosition()
	if row != 14 {
		t.Fatalf("CUP clamped in origin mode: want row 14, got %d", row)
	}
}

func TestCSI_CUP_NoOriginMode_Absolute(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Without origin mode, CUP should be absolute
	v.Write([]byte("\x1b[1;1H"))
	row, col := v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("CUP no origin mode: want (0,0), got (%d,%d)", row, col)
	}
}

func TestCSI_VPA_OriginMode(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Enable origin mode
	v.Write([]byte("\x1b[?6h"))
	// VPA row 1 — should go to scroll region top (row 4)
	v.Write([]byte("\x1b[1d"))
	row, _ := v.CursorPosition()
	if row != 4 {
		t.Fatalf("VPA in origin mode: want row 4, got %d", row)
	}
}

func TestCSI_DECSET_OriginMode_RISReset(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?6h"))
	if !v.ActiveScreen().OriginMode {
		t.Fatal("after DECSET ?6h: expected OriginMode=true")
	}
	// RIS resets origin mode
	v.Write([]byte("\x1bc"))
	if v.ActiveScreen().OriginMode {
		t.Fatal("after RIS: expected OriginMode=false")
	}
}

func TestCSI_DECSET_OriginMode_HomesCursor(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Move cursor somewhere
	v.Write([]byte("\x1b[10;30H"))
	// Enable origin mode — should home cursor to scroll region top
	v.Write([]byte("\x1b[?6h"))
	row, col := v.CursorPosition()
	if row != 4 || col != 0 {
		t.Fatalf("after DECSET ?6h: want cursor at (4,0), got (%d,%d)", row, col)
	}
	// Disable origin mode — should home cursor to screen top
	v.Write([]byte("\x1b[?6l"))
	row, col = v.CursorPosition()
	if row != 0 || col != 0 {
		t.Fatalf("after DECRST ?6l: want cursor at (0,0), got (%d,%d)", row, col)
	}
}

func TestCSI_DECSTBM_OriginMode_HomesToScrollRegion(t *testing.T) {
	v := NewVTerm(24, 80)
	// Enable origin mode first
	v.Write([]byte("\x1b[?6h"))
	// Set scroll region — should home cursor to scroll region top
	v.Write([]byte("\x1b[3;20r"))
	row, _ := v.CursorPosition()
	if row != 2 {
		t.Fatalf("after DECSTBM with origin mode: want cursor at row 2, got %d", row)
	}
}

func TestCSI_DSR_CPR_OriginMode(t *testing.T) {
	var resp []byte
	v := NewVTerm(24, 80)
	v.ResponseWriter = func(data []byte) { resp = append(resp, data...) }
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Enable origin mode
	v.Write([]byte("\x1b[?6h"))
	// Move cursor to scroll region row 3 (absolute row 6)
	v.Write([]byte("\x1b[3;1H"))
	// Request CPR
	v.Write([]byte("\x1b[6n"))
	// In origin mode, CPR should report row 3 (relative to scroll region top)
	got := string(resp)
	if got != "\x1b[3;1R" {
		t.Fatalf("DSR-CPR in origin mode: want %q, got %q", "\x1b[3;1R", got)
	}
}

func TestESC_DECSC_DECRC_OriginModeRestore(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set scroll region rows 5-15
	v.Write([]byte("\x1b[5;15r"))
	// Enable origin mode
	v.Write([]byte("\x1b[?6h"))
	// Save cursor (with origin mode on, cursor at scroll region top)
	v.Write([]byte("\x1b7"))
	// Disable origin mode
	v.Write([]byte("\x1b[?6l"))
	if v.ActiveScreen().OriginMode {
		t.Fatal("after DECRST ?6l: OriginMode should be false")
	}
	// Restore cursor — should restore origin mode
	v.Write([]byte("\x1b8"))
	if !v.ActiveScreen().OriginMode {
		t.Fatal("after DECRC: OriginMode should be restored to true")
	}
	// Cursor should be clamped to scroll region (not absolute row 0)
	row, _ := v.CursorPosition()
	if row < 4 {
		t.Fatalf("after DECRC with origin mode: cursor should be in scroll region, got row %d", row)
	}
}
