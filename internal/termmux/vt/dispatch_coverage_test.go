package vt

import "testing"

// ── CSI B: CUD — cursor down ───────────────────────────────────────

func TestCSI_CUD_CursorDown(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	h := &CSIHandler{}
	h.Dispatch(scr, 'B', []int{3}, false)
	if scr.CurRow != 8 {
		t.Fatalf("CUD: want row 8, got %d", scr.CurRow)
	}
}

func TestCSI_CUD_DefaultParam(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	h := &CSIHandler{}
	h.Dispatch(scr, 'B', nil, false) // default = 1
	if scr.CurRow != 6 {
		t.Fatalf("CUD default: want row 6, got %d", scr.CurRow)
	}
}

func TestCSI_CUD_ClampsToBottom(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 20
	h := &CSIHandler{}
	h.Dispatch(scr, 'B', []int{100}, false)
	if scr.CurRow != 23 {
		t.Fatalf("CUD clamp: want row 23, got %d", scr.CurRow)
	}
}

// ── CSI E: CNL — cursor next line ──────────────────────────────────

func TestCSI_CNL_CursorNextLine(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 5
	scr.CurCol = 40
	h := &CSIHandler{}
	h.Dispatch(scr, 'E', []int{3}, false)
	if scr.CurRow != 8 {
		t.Fatalf("CNL: want row 8, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CNL: want col 0, got %d", scr.CurCol)
	}
}

func TestCSI_CNL_ClampsToBottom(t *testing.T) {
	scr := NewScreen(10, 80)
	scr.CurRow = 7
	scr.CurCol = 50
	h := &CSIHandler{}
	h.Dispatch(scr, 'E', []int{100}, false)
	if scr.CurRow != 9 {
		t.Fatalf("CNL clamp: want row 9, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CNL clamp: want col 0, got %d", scr.CurCol)
	}
}

// ── CSI F: CPL — cursor previous line ──────────────────────────────

func TestCSI_CPL_CursorPrevLine(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 10
	scr.CurCol = 40
	h := &CSIHandler{}
	h.Dispatch(scr, 'F', []int{3}, false)
	if scr.CurRow != 7 {
		t.Fatalf("CPL: want row 7, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CPL: want col 0, got %d", scr.CurCol)
	}
}

func TestCSI_CPL_ClampsToTop(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CurRow = 3
	scr.CurCol = 20
	h := &CSIHandler{}
	h.Dispatch(scr, 'F', []int{100}, false)
	if scr.CurRow != 0 {
		t.Fatalf("CPL clamp: want row 0, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("CPL clamp: want col 0, got %d", scr.CurCol)
	}
}

// ── CSI K: EL — erase in line (via dispatch) ───────────────────────

func TestCSI_EL_Mode0_CursorToEnd(t *testing.T) {
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2
	h := &CSIHandler{}
	h.Dispatch(scr, 'K', []int{0}, false)
	// Cols 0-1 untouched, cols 2-4 erased
	if scr.Cells[0][0].Ch != 'A' || scr.Cells[0][1].Ch != 'B' {
		t.Fatal("EL mode 0: cells before cursor should be untouched")
	}
	for c := 2; c < 5; c++ {
		if scr.Cells[0][c].Ch != ' ' {
			t.Fatalf("EL mode 0: cell[0][%d] = %c, want space", c, scr.Cells[0][c].Ch)
		}
	}
}

func TestCSI_EL_Mode1_StartToCursor(t *testing.T) {
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 2
	h := &CSIHandler{}
	h.Dispatch(scr, 'K', []int{1}, false)
	// Cols 0-2 erased, cols 3-4 untouched
	for c := 0; c <= 2; c++ {
		if scr.Cells[0][c].Ch != ' ' {
			t.Fatalf("EL mode 1: cell[0][%d] = %c, want space", c, scr.Cells[0][c].Ch)
		}
	}
	if scr.Cells[0][3].Ch != 'D' || scr.Cells[0][4].Ch != 'E' {
		t.Fatal("EL mode 1: cells after cursor should be untouched")
	}
}

func TestCSI_EL_DefaultMode(t *testing.T) {
	scr := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		scr.Cells[0][i].Ch = ch
	}
	scr.CurCol = 3
	h := &CSIHandler{}
	h.Dispatch(scr, 'K', nil, false) // default mode = 0
	if scr.Cells[0][2].Ch != 'C' {
		t.Fatal("EL default: cell before cursor should be untouched")
	}
	if scr.Cells[0][3].Ch != ' ' || scr.Cells[0][4].Ch != ' ' {
		t.Fatal("EL default: cursor onward should be erased")
	}
}

// ── CSI L: IL — insert lines (via dispatch) ────────────────────────

func TestCSI_IL_InsertLines(t *testing.T) {
	scr := NewScreen(5, 3)
	for r := range 5 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	scr.CurRow = 2
	h := &CSIHandler{}
	h.Dispatch(scr, 'L', []int{1}, false)
	// Row 2 should be blank, row 3 should have what was row 2 ('C')
	if scr.Cells[2][0].Ch != ' ' {
		t.Fatalf("IL: row 2 = %c, want space (inserted)", scr.Cells[2][0].Ch)
	}
	if scr.Cells[3][0].Ch != 'C' {
		t.Fatalf("IL: row 3 = %c, want C (shifted)", scr.Cells[3][0].Ch)
	}
}

// ── CSI M: DL — delete lines (via dispatch) ────────────────────────

func TestCSI_DL_DeleteLines(t *testing.T) {
	scr := NewScreen(5, 3)
	for r := range 5 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	scr.CurRow = 1
	h := &CSIHandler{}
	h.Dispatch(scr, 'M', []int{2}, false)
	// Rows 1-2 deleted → row 1 should now have 'D', bottom rows blank
	if scr.Cells[1][0].Ch != 'D' {
		t.Fatalf("DL: row 1 = %c, want D", scr.Cells[1][0].Ch)
	}
	if scr.Cells[3][0].Ch != ' ' || scr.Cells[4][0].Ch != ' ' {
		t.Fatal("DL: bottom rows should be blank")
	}
}

// ── CSI S: SU — scroll up (via dispatch) ───────────────────────────

func TestCSI_SU_ScrollUp(t *testing.T) {
	scr := NewScreen(5, 3)
	for r := range 5 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	h := &CSIHandler{}
	h.Dispatch(scr, 'S', []int{2}, false)
	// Scrolled up 2: row 0 should now have 'C', bottom 2 rows blank
	if scr.Cells[0][0].Ch != 'C' {
		t.Fatalf("SU: row 0 = %c, want C", scr.Cells[0][0].Ch)
	}
	if scr.Cells[3][0].Ch != ' ' || scr.Cells[4][0].Ch != ' ' {
		t.Fatal("SU: bottom rows should be blank")
	}
}

// ── CSI T: SD — scroll down (via dispatch) ─────────────────────────

func TestCSI_SD_ScrollDown(t *testing.T) {
	scr := NewScreen(5, 3)
	for r := range 5 {
		scr.Cells[r][0].Ch = rune('A' + r)
	}
	h := &CSIHandler{}
	h.Dispatch(scr, 'T', []int{2}, false)
	// Scrolled down 2: top 2 rows blank, row 2 should have 'A'
	if scr.Cells[0][0].Ch != ' ' || scr.Cells[1][0].Ch != ' ' {
		t.Fatal("SD: top rows should be blank")
	}
	if scr.Cells[2][0].Ch != 'A' {
		t.Fatalf("SD: row 2 = %c, want A", scr.Cells[2][0].Ch)
	}
}

// ── CSI f: CUP alias ──────────────────────────────────────────────

func TestCSI_CUP_AliasF(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// 'f' is an alias for 'H' (CUP)
	h.Dispatch(scr, 'f', []int{10, 20}, false)
	if scr.CurRow != 9 || scr.CurCol != 19 {
		t.Fatalf("CUP alias f: want (9,19), got (%d,%d)", scr.CurRow, scr.CurCol)
	}
}

// ── CSI h/l non-private: no-op ─────────────────────────────────────

func TestCSI_SM_RM_NonPrivate_NoOp(t *testing.T) {
	scr := NewScreen(24, 80)
	scr.CursorVisible = true
	h := &CSIHandler{}
	// SM/RM without '?' prefix should be no-op for mode 25
	h.Dispatch(scr, 'l', []int{25}, false)
	if !scr.CursorVisible {
		t.Fatal("non-private RM should not change CursorVisible")
	}
	h.Dispatch(scr, 'h', []int{25}, false)
	if !scr.CursorVisible {
		t.Fatal("non-private SM should not change CursorVisible")
	}
}

// ── ESC D: IND — index (line feed) ────────────────────────────────

func TestESC_IND_LineFeed(t *testing.T) {
	scr := NewScreen(5, 5)
	scr.CurRow = 2
	h := &ESCHandler{}
	h.Dispatch(scr, 'D') // IND = LineFeed
	if scr.CurRow != 3 {
		t.Fatalf("IND: want row 3, got %d", scr.CurRow)
	}
}

func TestESC_IND_AtBottomScrolls(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.Cells[0][0].Ch = 'X'
	scr.CurRow = 2 // at bottom
	h := &ESCHandler{}
	h.Dispatch(scr, 'D')
	// Should have scrolled: row 0 content pushed up (lost), row 0 now has old row 1
	if scr.CurRow != 2 {
		t.Fatalf("IND scroll: want row 2, got %d", scr.CurRow)
	}
	// Row 0 was 'X', but after scroll it should have what was row 1 (blank)
	if scr.Cells[0][0].Ch != ' ' {
		t.Fatalf("IND scroll: row 0 should be blank after scroll, got %c", scr.Cells[0][0].Ch)
	}
}

// ── ESC E: NEL — next line ────────────────────────────────────────

func TestESC_NEL_NextLine(t *testing.T) {
	scr := NewScreen(5, 5)
	scr.CurRow = 2
	scr.CurCol = 3
	h := &ESCHandler{}
	h.Dispatch(scr, 'E') // NEL = col=0 + LineFeed
	if scr.CurRow != 3 {
		t.Fatalf("NEL: want row 3, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("NEL: want col 0, got %d", scr.CurCol)
	}
}

func TestESC_NEL_AtBottomScrolls(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.Cells[0][0].Ch = 'Y'
	scr.CurRow = 2
	scr.CurCol = 4
	h := &ESCHandler{}
	h.Dispatch(scr, 'E')
	if scr.CurRow != 2 {
		t.Fatalf("NEL scroll: want row 2, got %d", scr.CurRow)
	}
	if scr.CurCol != 0 {
		t.Fatalf("NEL scroll: want col 0, got %d", scr.CurCol)
	}
	if scr.Cells[0][0].Ch != ' ' {
		t.Fatalf("NEL scroll: row 0 should be blank, got %c", scr.Cells[0][0].Ch)
	}
}

// ── Screen.EraseLine modes 0 and 1 ────────────────────────────────

func TestScreen_EraseLine_Mode0_CursorToEnd(t *testing.T) {
	s := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		s.Cells[0][i].Ch = ch
	}
	s.CurCol = 2
	s.EraseLine(0)
	if s.Cells[0][0].Ch != 'A' || s.Cells[0][1].Ch != 'B' {
		t.Fatal("mode 0: cells before cursor should be preserved")
	}
	for c := 2; c < 5; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Fatalf("mode 0: cell[0][%d] = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
}

func TestScreen_EraseLine_Mode1_StartToCursor(t *testing.T) {
	s := NewScreen(1, 5)
	for i, ch := range "ABCDE" {
		s.Cells[0][i].Ch = ch
	}
	s.CurCol = 2
	s.EraseLine(1)
	for c := 0; c <= 2; c++ {
		if s.Cells[0][c].Ch != ' ' {
			t.Fatalf("mode 1: cell[0][%d] = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
	if s.Cells[0][3].Ch != 'D' || s.Cells[0][4].Ch != 'E' {
		t.Fatal("mode 1: cells after cursor should be preserved")
	}
}

// ── Screen.EraseDisplay modes 1, 2, 3 ─────────────────────────────

func TestScreen_EraseDisplay_Mode1_StartToCursor(t *testing.T) {
	s := NewScreen(3, 5)
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c].Ch = 'X'
		}
	}
	s.CurRow = 1
	s.CurCol = 2
	s.EraseDisplay(1)
	// Row 0 should be fully erased
	for c := range 5 {
		if s.Cells[0][c].Ch != ' ' {
			t.Fatalf("ED mode 1: row 0 cell %d = %c, want space", c, s.Cells[0][c].Ch)
		}
	}
	// Row 1, cols 0-2 erased
	for c := 0; c <= 2; c++ {
		if s.Cells[1][c].Ch != ' ' {
			t.Fatalf("ED mode 1: row 1 cell %d = %c, want space", c, s.Cells[1][c].Ch)
		}
	}
	// Row 1, col 3 onward untouched
	if s.Cells[1][3].Ch != 'X' || s.Cells[1][4].Ch != 'X' {
		t.Fatal("ED mode 1: cells after cursor should be untouched")
	}
	// Row 2 untouched
	if s.Cells[2][0].Ch != 'X' {
		t.Fatal("ED mode 1: row 2 should be untouched")
	}
}

func TestScreen_EraseDisplay_Mode2_EntireDisplay(t *testing.T) {
	s := NewScreen(3, 5)
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c].Ch = 'X'
		}
	}
	s.EraseDisplay(2)
	for r := range 3 {
		for c := range 5 {
			if s.Cells[r][c].Ch != ' ' {
				t.Fatalf("ED mode 2: cell[%d][%d] = %c, want space", r, c, s.Cells[r][c].Ch)
			}
		}
	}
}

func TestScreen_EraseDisplay_Mode3_Scrollback(t *testing.T) {
	s := NewScreen(3, 5)
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c].Ch = 'X'
		}
	}
	s.EraseDisplay(3) // mode 3 treated same as mode 2
	for r := range 3 {
		for c := range 5 {
			if s.Cells[r][c].Ch != ' ' {
				t.Fatalf("ED mode 3: cell[%d][%d] = %c, want space", r, c, s.Cells[r][c].Ch)
			}
		}
	}
}

// ── DECRST explicit test ──────────────────────────────────────────

func TestCSI_DECRST_CursorHide(t *testing.T) {
	scr := NewScreen(24, 80)
	h := &CSIHandler{}
	// Cursor visible by default
	if !scr.CursorVisible {
		t.Fatal("precondition: CursorVisible should be true")
	}
	h.Dispatch(scr, 'l', []int{25}, true) // DECRST ?25l
	if scr.CursorVisible {
		t.Fatal("DECRST ?25l: CursorVisible should be false")
	}
}

func TestCSI_DECRST_AltScreen(t *testing.T) {
	scr := NewScreen(24, 80)
	var gotAlt *bool
	h := &CSIHandler{
		AltScreenFn: func(toAlt bool) {
			gotAlt = &toAlt
		},
	}
	h.Dispatch(scr, 'l', []int{1049}, true) // DECRST ?1049l
	if gotAlt == nil || *gotAlt {
		t.Fatal("DECRST ?1049l: expected AltScreenFn(false)")
	}
}
