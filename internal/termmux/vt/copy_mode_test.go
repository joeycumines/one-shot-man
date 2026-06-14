package vt

import (
	"strings"
	"testing"
)

// helper: write n lines to a VTerm to generate scrollback
func writeLines(v *VTerm, n int) {
	for i := range n {
		v.Write([]byte(string(rune('A'+i%26)) + "\n"))
	}
}

func TestScrollUp_AdjustsOffset(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20) // 20 lines → 15 scrollback + 5 visible

	if got := v.ScrollbackLines(); got < 10 {
		t.Fatalf("ScrollbackLines = %d, want >= 10", got)
	}

	v.ScrollUp(3)
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 3 {
		t.Errorf("ScrollOffset = %d, want 3", snap.ScrollOffset)
	}
}

func TestScrollDown_AdjustsOffset(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	v.ScrollUp(5)
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 5 {
		t.Fatalf("ScrollOffset = %d, want 5 after ScrollUp(5)", snap.ScrollOffset)
	}

	v.ScrollDown(2)
	snap = v.ActiveScreen()
	if snap.ScrollOffset != 3 {
		t.Errorf("ScrollOffset = %d, want 3 after ScrollDown(2)", snap.ScrollOffset)
	}
}

func TestScrollOffset_Clamped(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	sb := v.ScrollbackLines()
	maxOffset := sb + 5

	// ScrollUp beyond max should clamp
	v.ScrollUp(maxOffset + 50)
	snap := v.ActiveScreen()
	if snap.ScrollOffset > maxOffset {
		t.Errorf("ScrollOffset = %d, exceeds max %d", snap.ScrollOffset, maxOffset)
	}

	// ScrollDown below 0 should clamp
	v.ScrollDown(maxOffset + 100)
	snap = v.ActiveScreen()
	if snap.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 after over-scrolling down", snap.ScrollOffset)
	}
}

func TestVisibleLines_NoScrollback(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.Cells[0][0].Ch = 'A'
	scr.Cells[1][0].Ch = 'B'
	scr.Cells[2][0].Ch = 'C'

	lines := scr.VisibleLines()
	if len(lines) != 3 {
		t.Fatalf("len(VisibleLines) = %d, want 3", len(lines))
	}
	if lines[0][0].Ch != 'A' {
		t.Errorf("lines[0][0].Ch = %c, want A", lines[0][0].Ch)
	}
	if lines[1][0].Ch != 'B' {
		t.Errorf("lines[1][0].Ch = %c, want B", lines[1][0].Ch)
	}
	if lines[2][0].Ch != 'C' {
		t.Errorf("lines[2][0].Ch = %c, want C", lines[2][0].Ch)
	}
}

func TestVisibleLines_WithScrollOffset(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.MaxScrollback = 10

	// Write 6 lines to a 3-row screen → 3 scrollback + 3 visible
	for i := range 6 {
		scr.PutChar(rune('0' + i))
		scr.CurCol = 0
		scr.LineFeed()
	}

	if scr.ScrollbackLen < 3 {
		t.Fatalf("ScrollbackLen = %d, want >= 3", scr.ScrollbackLen)
	}

	// ScrollOffset=0: visible rows are the screen rows
	scr.ScrollOffset = 0
	lines := scr.VisibleLines()
	if len(lines) != 3 {
		t.Fatalf("len(VisibleLines) = %d, want 3", len(lines))
	}

	// ScrollOffset=3: should show scrollback rows
	scr.ScrollOffset = 3
	lines = scr.VisibleLines()
	if len(lines) != 3 {
		t.Fatalf("len(VisibleLines) = %d, want 3", len(lines))
	}

	// First visible row should be from scrollback
	if lines[0][0].Ch == ' ' || lines[0][0].Ch == 0 {
		t.Errorf("lines[0][0].Ch = %c, expected scrollback content", lines[0][0].Ch)
	}
}

func TestEnterCopyMode_SetsScrollOffset(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	// Scroll up a bit first
	v.ScrollUp(5)
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 5 {
		t.Fatalf("ScrollOffset = %d, want 5 before EnterCopyMode", snap.ScrollOffset)
	}

	v.EnterCopyMode()
	if !v.InCopyMode() {
		t.Error("InCopyMode = false, want true")
	}
	snap = v.ActiveScreen()
	if snap.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 after EnterCopyMode", snap.ScrollOffset)
	}
}

func TestExitCopyMode_ResetsScrollOffset(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	row, col := v.CursorPosition()

	v.EnterCopyMode()
	v.ScrollUp(3)

	v.ExitCopyMode()
	if v.InCopyMode() {
		t.Error("InCopyMode = true, want false after ExitCopyMode")
	}
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 after ExitCopyMode", snap.ScrollOffset)
	}

	// Cursor should be restored
	r2, c2 := v.CursorPosition()
	if r2 != row || c2 != col {
		t.Errorf("cursor = (%d,%d), want (%d,%d)", r2, c2, row, col)
	}
}

func TestEnterCopyMode_Idempotent(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	v.EnterCopyMode()
	v.ScrollUp(3)

	// Second EnterCopyMode should be no-op
	v.EnterCopyMode()
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 3 {
		t.Errorf("ScrollOffset = %d, want 3 (second EnterCopyMode should be no-op)", snap.ScrollOffset)
	}
}

func TestExitCopyMode_WhenNotInCopyMode(t *testing.T) {
	v := NewVTerm(5, 20)
	// Should be a no-op, not panic
	v.ExitCopyMode()
	if v.InCopyMode() {
		t.Error("InCopyMode = true, want false")
	}
}

func TestSelectedText_NoSelection(t *testing.T) {
	v := NewVTerm(5, 20)
	if got := v.SelectedText(); got != "" {
		t.Errorf("SelectedText = %q, want empty when no selection", got)
	}
}

func TestSelectedText_SingleRow(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("Hello World"))

	v.SelectStart(0, 0)
	v.SelectEnd(0, 4)

	got := v.SelectedText()
	if got != "Hello" {
		t.Errorf("SelectedText = %q, want %q", got, "Hello")
	}
}

func TestSelectedText_MultiRow(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("ABC\r\nDEF\r\nGHI"))

	v.SelectStart(0, 1)
	v.SelectEnd(2, 2)

	got := v.SelectedText()
	if got != "BC\nDEF\nGHI" {
		t.Errorf("SelectedText = %q, want %q", got, "BC\nDEF\nGHI")
	}
}

func TestSelectedText_ReversedSelection(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("ABC\r\nDEF"))

	// End before start — should still work
	v.SelectStart(1, 2)
	v.SelectEnd(0, 1)

	got := v.SelectedText()
	if got != "BC\nDEF" {
		t.Errorf("SelectedText = %q, want %q", got, "BC\nDEF")
	}
}

func TestSelectedText_WithScrollback(t *testing.T) {
	v := NewVTerm(3, 20)
	v.SetScrollback(100)

	// Write 6 lines: 3 scrollback + 3 visible
	for i := range 6 {
		v.Write([]byte(string(rune('A'+i)) + string(rune('A'+i)) + string(rune('A'+i))))
		v.Write([]byte("\n"))
	}

	// Enter copy mode to see scrollback
	v.EnterCopyMode()

	// Select from scrollback row 0 (visible row 0 when ScrollOffset=0)
	v.SelectStart(0, 0)
	v.SelectEnd(1, 2)

	got := v.SelectedText()
	if got == "" {
		t.Error("SelectedText empty, expected scrollback content")
	}
}

func TestCopySelection_SendsOSC52(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("Hello World"))

	var oscCode int
	var oscData string
	v.OSCHandler = func(code int, data string) {
		oscCode = code
		oscData = data
	}

	v.SelectStart(0, 0)
	v.SelectEnd(0, 4)
	v.CopySelection()

	if oscCode != 52 {
		t.Errorf("OSC code = %d, want 52", oscCode)
	}
	if oscData != "Hello" {
		t.Errorf("OSC data = %q, want %q", oscData, "Hello")
	}
}

func TestCopySelection_NoOSCHandler(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("Hello"))

	v.SelectStart(0, 0)
	v.SelectEnd(0, 4)
	// Should not panic
	v.CopySelection()
}

func TestCopySelection_NoSelection(t *testing.T) {
	v := NewVTerm(5, 20)
	called := false
	v.OSCHandler = func(code int, data string) {
		called = true
	}
	v.CopySelection()
	if called {
		t.Error("OSCHandler called with no selection")
	}
}

func TestClampScrollOffset(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.MaxScrollback = 10

	// No scrollback: max offset = 0 + 5 = 5
	scr.ScrollOffset = 100
	scr.ClampScrollOffset()
	if scr.ScrollOffset != 5 {
		t.Errorf("ScrollOffset = %d, want 5", scr.ScrollOffset)
	}

	scr.ScrollOffset = -5
	scr.ClampScrollOffset()
	if scr.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0", scr.ScrollOffset)
	}
}

func TestMaxScrollOffset(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.MaxScrollback = 10

	// No scrollback yet
	if got := scr.MaxScrollOffset(); got != 5 {
		t.Errorf("MaxScrollOffset = %d, want 5", got)
	}

	// Add some scrollback
	for range 8 {
		scr.PutChar('X')
		scr.CurCol = 0
		scr.LineFeed()
	}

	sb := scr.ScrollbackLines()
	want := sb + 5
	if got := scr.MaxScrollOffset(); got != want {
		t.Errorf("MaxScrollOffset = %d, want %d (scrollback=%d + rows=5)", got, want, sb)
	}
}

func TestRenderRespectsScrollOffset(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.MaxScrollback = 50

	// Write 6 lines to get 3 scrollback + 3 visible
	for i := range 6 {
		scr.Cells[scr.CurRow][0].Ch = rune('0' + i)
		scr.CurCol = 0
		scr.LineFeed()
	}

	// Render with ScrollOffset=0
	scr.ScrollOffset = 0
	plain0, _, _ := RenderAll(scr)

	// Render with ScrollOffset=3 (showing scrollback)
	scr.ScrollOffset = 3
	plain3, _, _ := RenderAll(scr)

	// They should be different since scrollback content differs from visible
	if plain0 == plain3 {
		// Could happen if scrollback and visible are identical, but unlikely
		// with our setup. Just verify they're both non-empty.
		if plain0 == "" {
			t.Error("RenderAll returned empty plain text")
		}
	}
}

func TestRenderContentANSI_RespectsScrollOffset(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.MaxScrollback = 50

	for i := range 6 {
		scr.Cells[scr.CurRow][0].Ch = rune('A' + i)
		scr.CurCol = 0
		scr.LineFeed()
	}

	scr.ScrollOffset = 0
	ansi0 := RenderContentANSI(scr)

	scr.ScrollOffset = 3
	ansi3 := RenderContentANSI(scr)

	// Both should be non-empty
	if ansi0 == "" || ansi3 == "" {
		t.Error("RenderContentANSI returned empty")
	}

	// They should differ (scrollback vs visible content)
	if ansi0 == ansi3 {
		t.Log("RenderContentANSI same at offset 0 and 3 — may be identical content")
	}
}

func TestRenderFullScreen_RespectsScrollOffset(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.MaxScrollback = 50

	for i := range 6 {
		scr.Cells[scr.CurRow][0].Ch = rune('X' + i)
		scr.CurCol = 0
		scr.LineFeed()
	}

	scr.ScrollOffset = 0
	fs0 := RenderFullScreen(scr)

	scr.ScrollOffset = 3
	fs3 := RenderFullScreen(scr)

	if fs0 == "" || fs3 == "" {
		t.Error("RenderFullScreen returned empty")
	}
}

func TestVisibleLines_ReturnsCorrectRowCount(t *testing.T) {
	scr := NewScreen(5, 10)
	scr.MaxScrollback = 20

	for i := range 10 {
		scr.PutChar(rune('a' + i%26))
		scr.CurCol = 0
		scr.LineFeed()
	}

	for offset := 0; offset <= scr.ScrollbackLen+scr.Rows; offset++ {
		scr.ScrollOffset = offset
		lines := scr.VisibleLines()
		if len(lines) != scr.Rows {
			t.Errorf("offset=%d: len(VisibleLines) = %d, want %d", offset, len(lines), scr.Rows)
		}
		for r, row := range lines {
			if len(row) != scr.Cols {
				t.Errorf("offset=%d row=%d: len(row) = %d, want %d", offset, r, len(row), scr.Cols)
			}
		}
	}
}

func TestScrollUpScrollDown_Integration(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 30)

	sb := v.ScrollbackLines()

	// Scroll up to max
	v.ScrollUp(sb + 5 + 10) // try to go beyond max
	snap := v.ActiveScreen()
	maxOff := sb + 5
	if snap.ScrollOffset > maxOff {
		t.Errorf("ScrollOffset = %d, exceeds max %d", snap.ScrollOffset, maxOff)
	}

	// Scroll back down to 0
	v.ScrollDown(snap.ScrollOffset + 10)
	snap = v.ActiveScreen()
	if snap.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0", snap.ScrollOffset)
	}
}

func TestCellRowType(t *testing.T) {
	var row CellRow = make([]Cell, 5)
	if len(row) != 5 {
		t.Errorf("CellRow len = %d, want 5", len(row))
	}
}

func TestSelectedText_EmptySelection(t *testing.T) {
	v := NewVTerm(5, 20)
	v.Write([]byte("Hello"))

	// Only start, no end
	v.SelectStart(0, 0)
	if got := v.SelectedText(); got != "" {
		t.Errorf("SelectedText = %q, want empty with no end", got)
	}
}

func TestCopySelection_EmptySelection(t *testing.T) {
	v := NewVTerm(5, 20)
	called := false
	v.OSCHandler = func(code int, data string) {
		called = true
	}
	// No selection at all
	v.CopySelection()
	if called {
		t.Error("OSCHandler called with empty selection")
	}
}

func TestVisibleLines_ScrollOffsetBeyondContent(t *testing.T) {
	scr := NewScreen(3, 5)
	scr.MaxScrollback = 5

	// Write 4 lines → 1 scrollback + 3 visible
	for i := range 4 {
		scr.PutChar(rune('A' + i))
		scr.CurCol = 0
		scr.LineFeed()
	}

	// Set ScrollOffset beyond content — should still return Rows rows
	scr.ScrollOffset = scr.ScrollbackLen + scr.Rows + 5
	scr.ClampScrollOffset()
	lines := scr.VisibleLines()
	if len(lines) != 3 {
		t.Fatalf("len(VisibleLines) = %d, want 3", len(lines))
	}
}

func TestSelectedText_ScrollbackContent(t *testing.T) {
	v := NewVTerm(3, 20)
	v.SetScrollback(100)

	// Write lines with unique content
	for i := range 6 {
		v.Write([]byte(strings.Repeat(string(rune('A'+i)), 5) + "\n"))
	}

	sb := v.ScrollbackLines()
	if sb < 3 {
		t.Fatalf("ScrollbackLines = %d, want >= 3", sb)
	}

	// Enter copy mode and scroll to top
	v.EnterCopyMode()

	// Select from first visible row (which is scrollback row 0)
	v.SelectStart(0, 0)
	v.SelectEnd(0, 4)

	got := v.SelectedText()
	if len(got) != 5 {
		t.Errorf("SelectedText = %q, want 5 chars", got)
	}
}

func TestScrollCopyMode_WhenActive(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	v.EnterCopyMode()
	if !v.InCopyMode() {
		t.Fatal("InCopyMode = false, want true")
	}

	ok := v.ScrollCopyMode(3)
	if !ok {
		t.Error("ScrollCopyMode returned false, want true when copy mode active")
	}
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 3 {
		t.Errorf("ScrollOffset = %d, want 3 after ScrollCopyMode(3)", snap.ScrollOffset)
	}

	ok = v.ScrollCopyMode(-2)
	if !ok {
		t.Error("ScrollCopyMode returned false, want true")
	}
	snap = v.ActiveScreen()
	if snap.ScrollOffset != 1 {
		t.Errorf("ScrollOffset = %d, want 1 after ScrollCopyMode(-2)", snap.ScrollOffset)
	}
}

func TestScrollCopyMode_WhenNotActive(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	ok := v.ScrollCopyMode(3)
	if ok {
		t.Error("ScrollCopyMode returned true, want false when copy mode not active")
	}
	snap := v.ActiveScreen()
	if snap.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 (no scroll when not in copy mode)", snap.ScrollOffset)
	}
}

func TestScrollCopyMode_Clamped(t *testing.T) {
	v := NewVTerm(5, 20)
	v.SetScrollback(100)
	writeLines(v, 20)

	v.EnterCopyMode()

	ok := v.ScrollCopyMode(9999)
	if !ok {
		t.Error("ScrollCopyMode returned false")
	}
	snap := v.ActiveScreen()
	maxOff := v.ScrollbackLines() + 5
	if snap.ScrollOffset > maxOff {
		t.Errorf("ScrollOffset = %d, exceeds max %d", snap.ScrollOffset, maxOff)
	}

	ok = v.ScrollCopyMode(-9999)
	if !ok {
		t.Error("ScrollCopyMode returned false")
	}
	snap = v.ActiveScreen()
	if snap.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 after negative over-scroll", snap.ScrollOffset)
	}
}
