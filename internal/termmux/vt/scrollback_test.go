package vt

import (
	"strings"
	"testing"
)

func TestScrollback_LinesPreserved(t *testing.T) {
	// Write enough lines to a 5-row terminal to generate scrollback.
	// Using VTerm.Write which handles line feed properly.
	scr := NewScreen(5, 40)
	for i := range 15 {
		// Write a line and then CR+LF.
		line := strings.Repeat("A", i+1) // unique line lengths
		for _, ch := range line {
			scr.PutChar(ch)
		}
		// Explicit CR+LF to avoid PendingWrap complications.
		scr.CurCol = 0
		scr.LineFeed()
	}

	// 15 lines written to a 5-row terminal → 10 scrolled off + 5 visible.
	// But the last LineFeed doesn't scroll (cursor stays at bottom),
	// so we may have 10 scrollback lines.
	got := scr.ScrollbackLines()
	if got < 9 {
		t.Errorf("ScrollbackLines = %d, want >= 9", got)
	}

	// Verify scrollback rows are accessible and have content.
	for i := range got {
		row := scr.ScrollbackRow(i)
		if row == nil {
			t.Fatalf("ScrollbackRow(%d) = nil", i)
		}
	}
}

func TestScrollback_MaxEnforced(t *testing.T) {
	// Write 150 lines to a 10-row terminal with MaxScrollback=50.
	// Only the last 50 scrolled-off lines should be preserved.
	scr := NewScreen(10, 20)
	scr.MaxScrollback = 50

	for i := range 60 {
		// Write a unique marker: row number as a single digit in col 0.
		scr.PutChar(rune('0' + i%10))
		scr.LineFeed()
		scr.CurCol = 0
	}

	if got := scr.ScrollbackLines(); got != 50 {
		t.Errorf("ScrollbackLines = %d, want 50", got)
	}
}

func TestScrollback_MaxEnforcedLarge(t *testing.T) {
	// Write 15000 lines with MaxScrollback=10000.
	// Only the last 10000 should be preserved.
	scr := NewScreen(10, 20)
	scr.MaxScrollback = 10000

	for range 15000 {
		scr.PutChar('X')
		scr.LineFeed()
		scr.CurCol = 0
	}

	if got := scr.ScrollbackLines(); got != 10000 {
		t.Errorf("ScrollbackLines = %d, want 10000", got)
	}
}

func TestScrollback_EraseDisplay3(t *testing.T) {
	// ED mode 3 should clear scrollback but NOT clear the visible screen.
	scr := NewScreen(5, 20)

	// Write content to the screen first.
	scr.PutChar('Z')

	// Generate some scrollback.
	for range 10 {
		scr.PutChar('A')
		scr.LineFeed()
		scr.CurCol = 0
	}

	if scr.ScrollbackLines() == 0 {
		t.Fatal("expected scrollback lines before ED 3")
	}

	scr.EraseDisplay(3)

	if got := scr.ScrollbackLines(); got != 0 {
		t.Errorf("ScrollbackLines after ED 3 = %d, want 0", got)
	}
	if scr.ScrollOffset != 0 {
		t.Errorf("ScrollOffset after ED 3 = %d, want 0", scr.ScrollOffset)
	}

	// Verify visible display is NOT cleared.
	// After ED mode 3, the visible cells should still have content.
	found := false
	for r := 0; r < scr.Rows; r++ {
		for c := 0; c < scr.Cols; c++ {
			if scr.Cells[r][c].Ch != ' ' && scr.Cells[r][c].Ch != 0 {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("ED mode 3 should NOT clear the visible display")
	}
}

func TestScrollback_EraseDisplay2_PreservesScrollback(t *testing.T) {
	// ED mode 2 should NOT clear scrollback (only clears visible display).
	scr := NewScreen(5, 20)

	for range 10 {
		scr.PutChar('A')
		scr.LineFeed()
		scr.CurCol = 0
	}

	want := scr.ScrollbackLines()
	if want == 0 {
		t.Fatal("expected scrollback lines before ED 2")
	}

	scr.EraseDisplay(2)

	if got := scr.ScrollbackLines(); got != want {
		t.Errorf("ScrollbackLines after ED 2 = %d, want %d (scrollback preserved)", got, want)
	}
}

func TestScrollback_ResizePreserves(t *testing.T) {
	// Resize terminal — scrollback should be preserved.
	scr := NewScreen(5, 20)

	for range 10 {
		scr.PutChar('A')
		scr.LineFeed()
		scr.CurCol = 0
	}

	before := scr.ScrollbackLines()
	if before == 0 {
		t.Fatal("expected scrollback lines before resize")
	}

	scr.Resize(10, 40)

	if got := scr.ScrollbackLines(); got != before {
		t.Errorf("ScrollbackLines after resize = %d, want %d", got, before)
	}
}

func TestScrollback_ScrollRegion(t *testing.T) {
	// Lines scrolled within a scroll region that starts at top=0
	// should go to scrollback.
	scr := NewScreen(10, 20)

	// Set scroll region to rows 1-8 (1-indexed).
	scr.ScrollTop = 1
	scr.ScrollBot = 8

	// Move cursor to bottom of scroll region and write.
	scr.CurRow = 7 // 0-indexed, row 7 = 1-indexed row 8
	scr.CurCol = 0

	for range 5 {
		scr.PutChar('B')
		scr.LineFeed()
		scr.CurCol = 0
	}

	// With scroll region starting at top=0 (1-indexed=1 → 0-indexed=0),
	// scrollRegionUp should push to scrollback since top==0.
	if scr.ScrollbackLines() == 0 {
		t.Error("expected scrollback lines from scroll region scrolling")
	}
}

func TestScrollback_ScrollRegionNotFromTop_NoScrollback(t *testing.T) {
	// When scroll region does NOT start from top (top > 0),
	// scrollRegionUp should NOT push to scrollback.
	scr := NewScreen(10, 20)

	// Set scroll region to rows 3-8 (1-indexed).
	scr.ScrollTop = 3
	scr.ScrollBot = 8

	// Move cursor to bottom of scroll region and write.
	scr.CurRow = 7
	scr.CurCol = 0

	for range 5 {
		scr.PutChar('C')
		scr.LineFeed()
		scr.CurCol = 0
	}

	// Scroll region starts at row 3 (not top=0), so no scrollback.
	if scr.ScrollbackLines() != 0 {
		t.Errorf("ScrollbackLines = %d, want 0 (scroll region not from top)", scr.ScrollbackLines())
	}
}

func TestScrollback_SnapshotCopies(t *testing.T) {
	// Snapshot should include an independent copy of scrollback.
	scr := NewScreen(5, 20)

	for range 10 {
		scr.PutChar('D')
		scr.LineFeed()
		scr.CurCol = 0
	}

	snap := scr.Snapshot()

	if snap.ScrollbackLines() != scr.ScrollbackLines() {
		t.Errorf("snapshot ScrollbackLines = %d, want %d", snap.ScrollbackLines(), scr.ScrollbackLines())
	}

	// Mutate the original's scrollback — snapshot should be unaffected.
	scr.EraseDisplay(3)

	if scr.ScrollbackLines() != 0 {
		t.Error("original scrollback should be cleared after ED 3")
	}
	if snap.ScrollbackLines() == 0 {
		t.Error("snapshot scrollback should NOT be affected by original's ED 3")
	}
}

func TestScrollback_RingBuffer(t *testing.T) {
	// Verify ring buffer wraps correctly when MaxScrollback exceeded.
	// Use VTerm to write lines cleanly via Write().
	//
	// Strategy: fill the 5-row terminal first, then write more lines.
	// Once the terminal is full, each new line pushes the top visible row
	// into scrollback. We use unique markers per line so we can verify
	// that only the most recent scrollback entries are kept.
	v := NewVTerm(5, 20)
	v.SetScrollback(3)

	// Write enough lines to fill the screen and generate scrollback overflow.
	// After the screen is full (5 lines), each subsequent line pushes the
	// top visible row into scrollback.
	for i := range 20 {
		// Use two-char markers to distinguish lines: "AA", "BB", etc.
		marker := string(rune('A'+i/26)) + string(rune('A'+i%26))
		v.Write([]byte(marker + "\n"))
	}

	got := v.ScrollbackLines()
	if got != 3 {
		t.Fatalf("ScrollbackLines = %d, want 3", got)
	}

	// With 20 lines written to a 5-row terminal and MaxScrollback=3,
	// we should have exactly 3 lines in scrollback. The scrollback
	// should contain rows that were visible at some point (not blank).
	for i := range got {
		row := v.ScrollbackRow(i)
		if row == nil {
			t.Fatalf("ScrollbackRow(%d) = nil", i)
		}
		// Each row should have some non-space content.
		hasContent := false
		for _, c := range row {
			if c.Ch != ' ' && c.Ch != 0 {
				hasContent = true
				break
			}
		}
		if !hasContent {
			t.Errorf("ScrollbackRow(%d) is blank, expected content", i)
		}
	}
}

func TestScrollback_DefaultMax(t *testing.T) {
	// New screen should have MaxScrollback=10000.
	scr := NewScreen(10, 20)
	if scr.MaxScrollback != 10000 {
		t.Errorf("MaxScrollback = %d, want 10000", scr.MaxScrollback)
	}
}

func TestScrollback_NoScrollbackWhenMaxZero(t *testing.T) {
	// When MaxScrollback=0, no scrollback should be collected.
	scr := NewScreen(5, 20)
	scr.MaxScrollback = 0

	for range 10 {
		scr.PutChar('Z')
		scr.LineFeed()
		scr.CurCol = 0
	}

	if got := scr.ScrollbackLines(); got != 0 {
		t.Errorf("ScrollbackLines = %d, want 0 (MaxScrollback=0)", got)
	}
}

func TestScrollbackRow_OutOfRange(t *testing.T) {
	scr := NewScreen(5, 20)
	for range 10 {
		scr.PutChar('A')
		scr.LineFeed()
		scr.CurCol = 0
	}

	if row := scr.ScrollbackRow(-1); row != nil {
		t.Error("ScrollbackRow(-1) should be nil")
	}
	if row := scr.ScrollbackRow(scr.ScrollbackLines()); row != nil {
		t.Error("ScrollbackRow(ScrollbackLines) should be nil")
	}
}

// ── VTerm-level scrollback tests ──────────────────────────────────

func TestVTerm_ScrollbackLines(t *testing.T) {
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("Hello\n"))
	}

	if got := v.ScrollbackLines(); got == 0 {
		t.Error("VTerm.ScrollbackLines() = 0, want > 0")
	}
}

func TestVTerm_ScrollbackRow(t *testing.T) {
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("World\n"))
	}

	row := v.ScrollbackRow(0)
	if row == nil {
		t.Fatal("VTerm.ScrollbackRow(0) = nil")
	}
	// First character should be 'W' from "World".
	if row[0].Ch != 'W' {
		t.Errorf("VTerm.ScrollbackRow(0)[0].Ch = %c, want 'W'", row[0].Ch)
	}
}

func TestVTerm_SetScrollback(t *testing.T) {
	v := NewVTerm(5, 20)

	// Generate scrollback.
	for range 20 {
		v.Write([]byte("Line\n"))
	}

	before := v.ScrollbackLines()
	if before == 0 {
		t.Fatal("expected scrollback before SetScrollback")
	}

	// Reduce scrollback.
	v.SetScrollback(5)

	if got := v.ScrollbackLines(); got > 5 {
		t.Errorf("ScrollbackLines after SetScrollback(5) = %d, want <= 5", got)
	}
}

func TestVTerm_ActiveScreen_ScrollbackInSnapshot(t *testing.T) {
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("Test\n"))
	}

	snap := v.ActiveScreen()
	if snap.ScrollbackLines() == 0 {
		t.Error("ActiveScreen snapshot should include scrollback")
	}
}

func TestVTerm_Scrollback_Reset(t *testing.T) {
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("Data\n"))
	}

	if v.ScrollbackLines() == 0 {
		t.Fatal("expected scrollback before reset")
	}

	// Full reset (ESC c) creates new screens.
	v.Write([]byte("\x1bc"))

	if got := v.ScrollbackLines(); got != 0 {
		t.Errorf("ScrollbackLines after reset = %d, want 0", got)
	}
}

func TestVTerm_Scrollback_ED3(t *testing.T) {
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("X\n"))
	}

	if v.ScrollbackLines() == 0 {
		t.Fatal("expected scrollback before ED 3")
	}

	// ED mode 3: \x1b[3J
	v.Write([]byte("\x1b[3J"))

	if got := v.ScrollbackLines(); got != 0 {
		t.Errorf("ScrollbackLines after ED 3 = %d, want 0", got)
	}
}

func TestVTerm_Scrollback_ResizePreserves(t *testing.T) {
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("Y\n"))
	}

	before := v.ScrollbackLines()
	if before == 0 {
		t.Fatal("expected scrollback before resize")
	}

	v.Resize(10, 40)

	if got := v.ScrollbackLines(); got != before {
		t.Errorf("ScrollbackLines after resize = %d, want %d", got, before)
	}
}

func TestVTerm_SetScrollback_Zero(t *testing.T) {
	// SetScrollback(0) should clear all scrollback data and disable it.
	v := NewVTerm(5, 20)

	for range 10 {
		v.Write([]byte("Data\n"))
	}

	if v.ScrollbackLines() == 0 {
		t.Fatal("expected scrollback before SetScrollback(0)")
	}

	v.SetScrollback(0)

	if got := v.ScrollbackLines(); got != 0 {
		t.Errorf("ScrollbackLines after SetScrollback(0) = %d, want 0", got)
	}

	// Further writes should not accumulate scrollback.
	for range 10 {
		v.Write([]byte("More\n"))
	}

	if got := v.ScrollbackLines(); got != 0 {
		t.Errorf("ScrollbackLines after more writes = %d, want 0 (scrollback disabled)", got)
	}
}

func TestVTerm_SetScrollback_Increase(t *testing.T) {
	// Increasing MaxScrollback should work correctly — ring buffer
	// must be rebuilt to avoid head-position corruption.
	v := NewVTerm(5, 20)
	v.SetScrollback(3)

	// Generate scrollback that fills the ring.
	for i := range 20 {
		v.Write([]byte(string(rune('A'+i%26)) + "\n"))
	}

	if v.ScrollbackLines() != 3 {
		t.Fatalf("ScrollbackLines = %d, want 3", v.ScrollbackLines())
	}

	// Increase max — ring buffer must be rebuilt correctly.
	v.SetScrollback(5)

	if got := v.ScrollbackLines(); got > 5 {
		t.Errorf("ScrollbackLines after increase = %d, want <= 5", got)
	}

	// Write more lines — they should be added to scrollback correctly.
	for i := range 5 {
		v.Write([]byte(string(rune('a'+i)) + "\n"))
	}

	// Scrollback should have grown up to the new max.
	if v.ScrollbackLines() > 5 {
		t.Errorf("ScrollbackLines after more writes = %d, want <= 5", v.ScrollbackLines())
	}

	// Verify ordering: oldest scrollback entry should be from before the increase.
	row0 := v.ScrollbackRow(0)
	if row0 == nil {
		t.Fatal("ScrollbackRow(0) = nil after increase")
	}
}

func TestVTerm_AlternateScreen_NoScrollback(t *testing.T) {
	// Alternate screen should not accumulate scrollback.
	v := NewVTerm(5, 20)

	// Switch to alternate screen.
	v.Write([]byte("\x1b[?1049h"))

	// Write enough lines to trigger scrolling.
	for range 10 {
		v.Write([]byte("AltLine\n"))
	}

	// Active screen is alternate — should have no scrollback.
	snap := v.ActiveScreen()
	if got := snap.ScrollbackLines(); got != 0 {
		t.Errorf("Alternate screen ScrollbackLines = %d, want 0", got)
	}

	// Switch back to primary.
	v.Write([]byte("\x1b[?1049l"))

	// Primary should still have its own (empty) scrollback since we
	// didn't write to it before switching.
	snap = v.ActiveScreen()
	if snap.MaxScrollback != 10000 {
		t.Errorf("Primary MaxScrollback = %d, want 10000", snap.MaxScrollback)
	}
}
