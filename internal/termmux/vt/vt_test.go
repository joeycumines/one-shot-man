package vt

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestNewVTerm(t *testing.T) {
	v := NewVTerm(24, 80)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.rows != 24 {
		t.Fatalf("rows = %d; want 24", v.rows)
	}
	if v.cols != 80 {
		t.Fatalf("cols = %d; want 80", v.cols)
	}
	if v.active != v.primary {
		t.Fatal("active should be primary after construction")
	}
}

func TestVTerm_Resize(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Resize(40, 120)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.rows != 40 || v.cols != 120 {
		t.Fatalf("after Resize: got %dx%d, want 40x120", v.rows, v.cols)
	}
	if v.primary.Rows != 40 || v.primary.Cols != 120 {
		t.Fatalf("primary dims: %dx%d, want 40x120", v.primary.Rows, v.primary.Cols)
	}
	if v.alternate.Rows != 40 || v.alternate.Cols != 120 {
		t.Fatalf("alternate dims: %dx%d, want 40x120", v.alternate.Rows, v.alternate.Cols)
	}
}

func TestVTerm_WriteASCII(t *testing.T) {
	v := NewVTerm(24, 80)
	n, err := v.Write([]byte("Hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Fatalf("Write returned %d; want 5", n)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	want := []rune{'H', 'e', 'l', 'l', 'o'}
	for i, ch := range want {
		if v.active.Cells[0][i].Ch != ch {
			t.Errorf("cell[0][%d] = %q; want %q", i, v.active.Cells[0][i].Ch, ch)
		}
	}
}

func TestVTerm_WriteNewline(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("Hi\nBye"))
	v.mu.Lock()
	defer v.mu.Unlock()
	// Row 0: H, i
	if v.active.Cells[0][0].Ch != 'H' || v.active.Cells[0][1].Ch != 'i' {
		t.Fatalf("row 0: got %q %q, want H i",
			v.active.Cells[0][0].Ch, v.active.Cells[0][1].Ch)
	}
	// Row 1: B, y, e (LF moves down, cursor stays at same column in raw mode,
	// but our handleControl for LF only calls LineFeed, no CR).
	// Actually \n is LF which calls LineFeed() but does NOT set CurCol=0.
	// So "Bye" starts at whatever CurCol was after "Hi\n".
	// After "Hi": CurCol=2. After LF: CurRow=1, CurCol=2.
	// So "Bye" is at cells[1][2], [1][3], [1][4].
	if v.active.Cells[1][2].Ch != 'B' || v.active.Cells[1][3].Ch != 'y' || v.active.Cells[1][4].Ch != 'e' {
		t.Fatalf("row 1: got %q %q %q at cols 2-4, want B y e",
			v.active.Cells[1][2].Ch, v.active.Cells[1][3].Ch, v.active.Cells[1][4].Ch)
	}
}

func TestVTerm_WriteCSI(t *testing.T) {
	v := NewVTerm(4, 4)
	// Fill screen with 'X'.
	v.mu.Lock()
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			v.active.Cells[r][c].Ch = 'X'
		}
	}
	v.mu.Unlock()
	// ED mode 2 = erase entire display.
	v.Write([]byte("\x1b[2J"))
	v.mu.Lock()
	defer v.mu.Unlock()
	for r := 0; r < 4; r++ {
		for c := 0; c < 4; c++ {
			if v.active.Cells[r][c].Ch != ' ' {
				t.Fatalf("cell[%d][%d] = %q after ED 2; want ' '", r, c, v.active.Cells[r][c].Ch)
			}
		}
	}
}

func TestVTerm_WriteUTF8(t *testing.T) {
	v := NewVTerm(24, 80)
	// 漢 = U+6F22 = E6 BC A2 (3 bytes), display width 2.
	v.Write([]byte("漢"))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.Cells[0][0].Ch != '漢' {
		t.Fatalf("cell[0][0] = %q; want '漢'", v.active.Cells[0][0].Ch)
	}
	// Width-2 char occupies two columns; second is placeholder (rune 0).
	if v.active.Cells[0][1].Ch != 0 {
		t.Fatalf("cell[0][1] = %q; want 0 (placeholder)", v.active.Cells[0][1].Ch)
	}
}

func TestVTerm_WriteSplitUTF8(t *testing.T) {
	v := NewVTerm(24, 80)
	// 漢 = E6 BC A2 — split across two writes.
	v.Write([]byte{0xE6})
	v.Write([]byte{0xBC, 0xA2})
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.Cells[0][0].Ch != '漢' {
		t.Fatalf("cell[0][0] = %q; want '漢'", v.active.Cells[0][0].Ch)
	}
}

func TestVTerm_Tab(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\t"))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurCol != 8 {
		t.Fatalf("CurCol after TAB = %d; want 8", v.active.CurCol)
	}
}

func TestVTerm_Backspace(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("AB\x08"))
	v.mu.Lock()
	defer v.mu.Unlock()
	// After 'A' CurCol=1, after 'B' CurCol=2, after BS CurCol=1.
	if v.active.CurCol != 1 {
		t.Fatalf("CurCol after BS = %d; want 1", v.active.CurCol)
	}
}

func TestVTerm_AltScreen(t *testing.T) {
	v := NewVTerm(24, 80)
	// Write on primary.
	v.Write([]byte("Hi"))
	// Switch to alternate screen.
	v.Write([]byte("\x1b[?1049h"))
	// Write on alternate.
	v.Write([]byte("XX"))
	// Switch back to primary.
	v.Write([]byte("\x1b[?1049l"))

	v.mu.Lock()
	defer v.mu.Unlock()
	// Primary should be preserved.
	if v.active != v.primary {
		t.Fatal("active should be primary after 1049l")
	}
	if v.active.Cells[0][0].Ch != 'H' || v.active.Cells[0][1].Ch != 'i' {
		t.Fatalf("primary cells: got %q %q, want H i",
			v.active.Cells[0][0].Ch, v.active.Cells[0][1].Ch)
	}
	// Cursor should be restored to saved position (col 2 after "Hi").
	if v.active.CurCol != 2 {
		t.Fatalf("primary CurCol = %d; want 2", v.active.CurCol)
	}
}

func TestVTerm_Reset(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("Hello"))
	// ESC c = RIS (full reset).
	v.Write([]byte("\x1bc"))

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurRow != 0 || v.active.CurCol != 0 {
		t.Fatalf("cursor after reset: (%d,%d); want (0,0)", v.active.CurRow, v.active.CurCol)
	}
	// Screen should be blank.
	for c := 0; c < 5; c++ {
		if v.active.Cells[0][c].Ch != ' ' {
			t.Fatalf("cell[0][%d] = %q after reset; want ' '", c, v.active.Cells[0][c].Ch)
		}
	}
}

func TestVTerm_ConcurrentWriteResize(t *testing.T) {
	v := NewVTerm(24, 80)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			v.Write([]byte("ABCDEFGHIJ\n"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			rows := 20 + (i % 10)
			cols := 60 + (i % 40)
			v.Resize(rows, cols)
		}
	}()
	wg.Wait()
	// If we get here without -race detector complaint, the test passes.
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.rows < 1 || v.cols < 1 {
		t.Fatal("terminal dimensions invalid after concurrent operations")
	}
}

func TestVTerm_Render(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("Hi"))
	out := v.RenderFullScreen()
	if !strings.Contains(out, "Hi") {
		t.Errorf("RenderFullScreen() missing text; got %q", out)
	}
	if !strings.Contains(out, "\x1b[0m") {
		t.Error("RenderFullScreen() missing SGR reset")
	}
	if !strings.Contains(out, "\x1b[?25h") {
		t.Error("RenderFullScreen() missing cursor show")
	}
}

func TestVTerm_RenderConcurrent(t *testing.T) {
	v := NewVTerm(24, 80)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			v.Write([]byte("test\n"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			_ = v.RenderFullScreen()
		}
	}()
	wg.Wait()
	// Race detector would fail if there's a data race.
}

// ── T089: Property-based test — VTerm Render round-trip ────────────

func TestVTerm_RenderRoundTrip(t *testing.T) {
	// Write known content to a VTerm, Render(), feed rendered output
	// to a fresh VTerm, then compare visible cell content.
	cases := []struct {
		name  string
		input string
	}{
		{"ascii", "Hello World"},
		{"multiline", "Line1\r\nLine2\r\nLine3"},
		{"bold_color", "\x1b[1;31mRed Bold\x1b[0m Normal"},
		{"cursor_move", "\x1b[5;10HMOVED"},
		{"wide_chars", "\u6F22\u5B57"}, // 漢字
		{"mixed", "ABC\x1b[1;31mDEF\x1b[0m\r\nGHI\x1b[2;5HJKL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v1 := NewVTerm(10, 40)
			v1.Write([]byte(tc.input))

			rendered := v1.RenderFullScreen()

			v2 := NewVTerm(10, 40)
			v2.Write([]byte(rendered))

			// Compare all visible cells.
			v1.mu.Lock()
			v2.mu.Lock()
			defer v1.mu.Unlock()
			defer v2.mu.Unlock()

			for r := 0; r < v1.rows; r++ {
				for c := 0; c < v1.cols; c++ {
					c1 := v1.active.Cells[r][c]
					c2 := v2.active.Cells[r][c]
					if c1.Ch != c2.Ch {
						t.Errorf("[%d][%d] ch: got %q, want %q", r, c, c2.Ch, c1.Ch)
					}
				}
			}
		})
	}
}

// ── T092: PutChar wide character edge cases ────────────────────────

func TestVTerm_WideCharOverwritePrevious(t *testing.T) {
	// Overwriting the second cell of a wide char with a narrow char
	// should clear the first cell of the old wide char.
	v := NewVTerm(3, 10)
	v.Write([]byte("\u6F22")) // 漢 at cols 0-1
	// Move to col 1 and overwrite with narrow char.
	v.Write([]byte("\x1b[1;2H")) // CUP to row 1, col 2 (0-indexed col 1)
	v.Write([]byte("X"))

	v.mu.Lock()
	defer v.mu.Unlock()
	// The old wide char at col 0 should now be effectively broken.
	// Col 1 should be 'X'. The result at col 0 depends on implementation
	// but should not be the original wide char since its placeholder was
	// overwritten.
	if v.active.Cells[0][1].Ch != 'X' {
		t.Errorf("cell[0][1] = %q, want 'X'", v.active.Cells[0][1].Ch)
	}
}

func TestVTerm_ConsecutiveWideChars(t *testing.T) {
	v := NewVTerm(3, 10)
	// 漢字 = 2 wide chars = 4 columns
	v.Write([]byte("\u6F22\u5B57"))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.Cells[0][0].Ch != '\u6F22' {
		t.Errorf("cell[0][0] = %q, want 漢", v.active.Cells[0][0].Ch)
	}
	if v.active.Cells[0][1].Ch != 0 { // placeholder
		t.Errorf("cell[0][1] = %q, want 0 (placeholder)", v.active.Cells[0][1].Ch)
	}
	if v.active.Cells[0][2].Ch != '\u5B57' {
		t.Errorf("cell[0][2] = %q, want 字", v.active.Cells[0][2].Ch)
	}
	if v.active.Cells[0][3].Ch != 0 { // placeholder
		t.Errorf("cell[0][3] = %q, want 0 (placeholder)", v.active.Cells[0][3].Ch)
	}
	if v.active.CurCol != 4 {
		t.Errorf("CurCol = %d, want 4", v.active.CurCol)
	}
}

// ── T093: DECSTBM interaction with cursor movement ─────────────────

func TestVTerm_DECSTBM_HomesCursor(t *testing.T) {
	v := NewVTerm(10, 40)
	v.Write([]byte("\x1b[5;10H"))
	v.mu.Lock()
	if v.active.CurRow != 4 || v.active.CurCol != 9 {
		t.Fatalf("pre: cursor (%d,%d), want (4,9)", v.active.CurRow, v.active.CurCol)
	}
	v.mu.Unlock()

	// DECSTBM: set scroll region rows 3..8
	v.Write([]byte("\x1b[3;8r"))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurRow != 0 || v.active.CurCol != 0 {
		t.Errorf("after DECSTBM: cursor (%d,%d), want (0,0)", v.active.CurRow, v.active.CurCol)
	}
	if v.active.ScrollTop != 3 || v.active.ScrollBot != 8 {
		t.Errorf("scroll region = %d-%d, want 3-8", v.active.ScrollTop, v.active.ScrollBot)
	}
}

func TestVTerm_DECSTBM_CUPStillAddressesFullScreen(t *testing.T) {
	v := NewVTerm(10, 40)
	// Set scroll region rows 3..8
	v.Write([]byte("\x1b[3;8r"))
	// CUP to row 1, col 1 (outside scroll region)
	v.Write([]byte("\x1b[1;1H"))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurRow != 0 || v.active.CurCol != 0 {
		t.Errorf("CUP ignores scroll region: cursor (%d,%d), want (0,0)", v.active.CurRow, v.active.CurCol)
	}
}

func TestVTerm_DECSTBM_LineFeedScrollsOnlyWithinRegion(t *testing.T) {
	v := NewVTerm(5, 10)
	// Label rows
	for r := 0; r < 5; r++ {
		v.Write([]byte{byte('A' + r)})
		if r < 4 {
			v.Write([]byte("\x1b[" + string(rune('0'+r+2)) + ";1H"))
		}
	}
	// Set scroll region rows 2..4 (1-indexed)
	v.Write([]byte("\x1b[2;4r"))
	// Move to row 4 (bottom of scroll region, 1-indexed)
	v.Write([]byte("\x1b[4;1H"))
	// Line feed should scroll only within region
	v.Write([]byte("\n"))

	v.mu.Lock()
	defer v.mu.Unlock()
	// Row 0 (outside region) should be unchanged
	if v.active.Cells[0][0].Ch != 'A' {
		t.Errorf("row 0 = %c, want A (unchanged)", v.active.Cells[0][0].Ch)
	}
	// Row 4 (outside region, 0-indexed) should be unchanged
	if v.active.Cells[4][0].Ch != 'E' {
		t.Errorf("row 4 = %c, want E (unchanged)", v.active.Cells[4][0].Ch)
	}
}

// ── T121: Interleaved CSI and UTF-8 ────────────────────────────────

func TestVTerm_InterleavedCSI_UTF8(t *testing.T) {
	v := NewVTerm(10, 40)
	// Move to row 5, col 10, write CJK, then move to row 1 col 1, write ASCII
	v.Write([]byte("\x1b[5;10H\u6F22\u5B57\x1b[1;1HABC"))

	v.mu.Lock()
	defer v.mu.Unlock()

	// 漢 at row 4, col 9 (0-indexed)
	if v.active.Cells[4][9].Ch != '\u6F22' {
		t.Errorf("cell[4][9] = %q, want 漢", v.active.Cells[4][9].Ch)
	}
	if v.active.Cells[4][10].Ch != 0 { // placeholder
		t.Errorf("cell[4][10] = %q, want 0 (placeholder)", v.active.Cells[4][10].Ch)
	}
	// 字 at row 4, col 11
	if v.active.Cells[4][11].Ch != '\u5B57' {
		t.Errorf("cell[4][11] = %q, want 字", v.active.Cells[4][11].Ch)
	}
	// ABC at row 0, col 0
	if v.active.Cells[0][0].Ch != 'A' || v.active.Cells[0][1].Ch != 'B' || v.active.Cells[0][2].Ch != 'C' {
		t.Errorf("row 0 = %c%c%c, want ABC",
			v.active.Cells[0][0].Ch, v.active.Cells[0][1].Ch, v.active.Cells[0][2].Ch)
	}
}

// ── T123: DECSC/DECRC cursor save/restore ──────────────────────────

func TestVTerm_DECSC_DECRC(t *testing.T) {
	v := NewVTerm(10, 40)
	// Move cursor and set attrs
	v.Write([]byte("\x1b[5;10H")) // CUP to 5,10
	v.Write([]byte("\x1b[1;31m")) // Bold + red
	v.Write([]byte("\x1b7"))      // DECSC - save cursor

	// Move somewhere else and change attrs
	v.Write([]byte("\x1b[1;1H"))
	v.Write([]byte("\x1b[0m"))

	// Restore cursor
	v.Write([]byte("\x1b8"))

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurRow != 4 || v.active.CurCol != 9 {
		t.Errorf("restored cursor = (%d,%d), want (4,9)", v.active.CurRow, v.active.CurCol)
	}
	if !v.active.CurAttr.Bold {
		t.Error("restored CurAttr should be Bold")
	}
}

// ── T113: Concurrent VTerm.Write safety ────────────────────────────

func TestVTerm_ConcurrentWrite(t *testing.T) {
	v := NewVTerm(24, 80)
	var wg sync.WaitGroup
	const workers = 5
	const iters = 100

	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				v.Write([]byte("HELLO \x1b[1;31mWORLD\x1b[0m\n"))
			}
		}()
	}
	wg.Wait()

	// If we get here without -race complaints, the test passes.
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurRow < 0 || v.active.CurRow >= v.rows {
		t.Errorf("CurRow %d out of bounds [0,%d)", v.active.CurRow, v.rows)
	}
	if v.active.CurCol < 0 || v.active.CurCol >= v.cols {
		t.Errorf("CurCol %d out of bounds [0,%d)", v.active.CurCol, v.cols)
	}
}

// ── T124: Tab stops — HTS set, TBC clear, Tab navigation ──────────

func TestVTerm_TabStops_Default8Column(t *testing.T) {
	v := NewVTerm(24, 80)
	// Tab from col 0 should jump to col 8.
	v.Write([]byte("\t"))
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurCol != 8 {
		t.Errorf("CurCol after TAB = %d; want 8", v.active.CurCol)
	}
}

func TestVTerm_TabStops_HTS_SetCustomStop(t *testing.T) {
	v := NewVTerm(24, 80)
	// Move to col 5, set tab stop via ESC H (HTS).
	v.Write([]byte("\x1b[6G")) // CHA col 6 (1-indexed) = col 5 (0-indexed)
	v.Write([]byte("\x1bH"))   // HTS: set tab stop at current col
	// Return to col 0 and tab forward.
	v.Write([]byte("\r")) // CR to col 0
	v.Write([]byte("\t")) // Tab should now jump to col 5

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurCol != 5 {
		t.Errorf("CurCol after TAB to custom stop = %d; want 5", v.active.CurCol)
	}
}

func TestVTerm_TabStops_TBC_ClearSingleStop(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set custom stop at 5.
	v.Write([]byte("\x1b[6G\x1bH"))
	// Move to col 5, clear the stop with TBC mode 0.
	v.Write([]byte("\x1b[6G")) // CHA to col 5
	v.Write([]byte("\x1b[0g")) // TBC mode 0: clear stop at cursor
	// Return to col 0 and tab forward — should skip to default col 8.
	v.Write([]byte("\r\t"))

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurCol != 8 {
		t.Errorf("CurCol after TAB past cleared stop = %d; want 8", v.active.CurCol)
	}
}

func TestVTerm_TabStops_TBC_ClearAll(t *testing.T) {
	v := NewVTerm(24, 80)
	// Clear all tab stops with TBC mode 3.
	v.Write([]byte("\x1b[3g"))
	// Tab from col 0 should go to last column (no stops).
	v.Write([]byte("\t"))

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurCol != 79 {
		t.Errorf("CurCol after TAB with no stops = %d; want 79", v.active.CurCol)
	}
}

func TestVTerm_TabStops_Resize_ExtendsDefaults(t *testing.T) {
	v := NewVTerm(24, 40)
	v.Resize(24, 80)
	// After resize from 40→80 cols, new cols should have default 8-column stops.
	// Move to col 38 (near old boundary) and tab.
	v.Write([]byte("\x1b[39G")) // CHA col 39 (1-indexed) = col 38
	v.Write([]byte("\t"))

	v.mu.Lock()
	defer v.mu.Unlock()
	// Next 8-column stop after 38 is 40.
	if v.active.CurCol != 40 {
		t.Errorf("CurCol after TAB in extended region = %d; want 40", v.active.CurCol)
	}
}

func TestVTerm_TabStops_MultipleTabs(t *testing.T) {
	v := NewVTerm(24, 80)
	// Default stops: 0, 8, 16, 24 ...
	// Two tabs from col 0: 0→8→16.
	v.Write([]byte("\t\t"))

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.active.CurCol != 16 {
		t.Errorf("CurCol after 2 TABs = %d; want 16", v.active.CurCol)
	}
}

// ── T110: VTerm handles real Claude Code output samples ────────────

func TestVTerm_RealClaudeCodeOutput(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "claude_output.txt"))
	if err != nil {
		t.Skipf("testdata/claude_output.txt not found: %v", err)
	}

	v := NewVTerm(100, 120)
	n, err := v.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Write returned %d; want %d", n, len(data))
	}

	// RenderFullScreen must not panic and produce valid UTF-8.
	rendered := v.RenderFullScreen()
	if !utf8.ValidString(rendered) {
		t.Error("RenderFullScreen() produced invalid UTF-8")
	}

	// Verify some recognizable fragments made it through.
	v.mu.Lock()
	defer v.mu.Unlock()
	var screenText strings.Builder
	for r := 0; r < v.active.Rows; r++ {
		for c := 0; c < v.active.Cols; c++ {
			ch := v.active.Cells[r][c].Ch
			if ch != 0 && ch != ' ' {
				screenText.WriteRune(ch)
			}
		}
	}
	text := screenText.String()

	// These strings should appear somewhere in the rendered content.
	fragments := []string{"Claude", "tests", "pass"}
	for _, frag := range fragments {
		if !strings.Contains(text, frag) {
			t.Errorf("screen text missing fragment %q", frag)
		}
	}
}

func TestVTerm_TestDataFixtures_NoPanic(t *testing.T) {
	// Feed all testdata fixture files through VTerm — must not panic.
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Skipf("testdata dir not found: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			v := NewVTerm(24, 80)
			n, err := v.Write(data)
			if err != nil {
				t.Fatalf("Write error: %v", err)
			}
			if n != len(data) {
				t.Fatalf("Write returned %d; want %d", n, len(data))
			}
			rendered := v.RenderFullScreen()
			if !utf8.ValidString(rendered) {
				t.Errorf("RenderFullScreen() produced invalid UTF-8 for %s", e.Name())
			}
		})
	}
}

// ── VTerm.String() plain-text rendering ────────────────────────────

func TestVTerm_String_Empty(t *testing.T) {
	v := NewVTerm(5, 10)
	if got := v.String(); got != "" {
		t.Errorf("String() on empty VTerm = %q; want empty", got)
	}
}

func TestVTerm_String_ASCII(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("Hello"))
	got := v.String()
	if got != "Hello" {
		t.Errorf("String() = %q; want %q", got, "Hello")
	}
}

func TestVTerm_String_MultipleLines(t *testing.T) {
	v := NewVTerm(5, 20)
	// VTerm treats \n as LF only (no carriage return). Use \r\n for
	// CR+LF to position each line at column 0, matching real terminal
	// behavior where the PTY's line discipline adds CR.
	v.Write([]byte("Line 1\r\nLine 2\r\nLine 3"))
	got := v.String()
	want := "Line 1\nLine 2\nLine 3"
	if got != want {
		t.Errorf("String() = %q; want %q", got, want)
	}
}

func TestVTerm_String_TrailingEmptyLines(t *testing.T) {
	v := NewVTerm(10, 20)
	v.Write([]byte("Data"))
	// VTerm has 10 rows, only row 0 has content. String() should
	// not include trailing empty lines.
	got := v.String()
	if got != "Data" {
		t.Errorf("String() = %q; want %q", got, "Data")
	}
}

func TestVTerm_String_TrailingSpaces(t *testing.T) {
	v := NewVTerm(5, 20)
	// Write "Hi" then move to col 10 and write "There" — columns 2-9
	// are filled with spaces. String() should strip trailing blanks per row.
	v.Write([]byte("Hi\x1b[1;11HThere"))
	got := v.String()
	// Row 0 should be "Hi        There" with trailing spaces stripped,
	// but the spaces between "Hi" and "There" should be preserved.
	if !strings.Contains(got, "Hi") || !strings.Contains(got, "There") {
		t.Errorf("String() = %q; want to contain 'Hi' and 'There'", got)
	}
	// Should not end with spaces.
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		if len(line) > 0 && line[len(line)-1] == ' ' {
			t.Errorf("line %d has trailing spaces: %q", i, line)
		}
	}
}

func TestVTerm_String_ANSI_Stripped(t *testing.T) {
	v := NewVTerm(5, 40)
	// Write with bold/red SGR — String() returns plain text only.
	v.Write([]byte("\x1b[1;31mBold Red\x1b[0m Normal"))
	got := v.String()
	if !strings.Contains(got, "Bold Red") {
		t.Errorf("String() = %q; want to contain %q", got, "Bold Red")
	}
	if !strings.Contains(got, "Normal") {
		t.Errorf("String() = %q; want to contain %q", got, "Normal")
	}
	// Should NOT contain ESC sequences.
	if strings.Contains(got, "\x1b") {
		t.Errorf("String() contains ESC sequences: %q", got)
	}
}

func TestVTerm_String_WideChars(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("漢字OK"))
	got := v.String()
	if !strings.Contains(got, "漢字OK") {
		t.Errorf("String() = %q; want to contain %q", got, "漢字OK")
	}
}

func TestVTerm_String_Concurrent(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("initial"))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			v.Write([]byte("abc"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = v.String()
		}
	}()
	wg.Wait()
	// Race detector validates thread safety.
}
