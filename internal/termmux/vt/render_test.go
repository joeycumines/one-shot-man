package vt

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// scanRow writes s into the given screen row from column zero, leaving the
// rest of the row blank.
func scanRow(scr *Screen, row int, s string) {
	if row < 0 || row >= scr.Rows {
		return
	}
	for i, ch := range []rune(s) {
		if i >= scr.Cols {
			break
		}
		scr.Cells[row][i] = Cell{Ch: ch}
	}
}

// scanCells returns a row of cols cells containing s at the left.
func scanCells(s string, cols int) []Cell {
	out := make([]Cell, cols)
	for i := range out {
		out[i] = Cell{Ch: ' '}
	}
	for i, ch := range []rune(s) {
		if i >= cols {
			break
		}
		out[i] = Cell{Ch: ch}
	}
	return out
}

func fullTail(scr *Screen) string {
	vis := "\x1b[?25l"
	if scr.CursorVisible {
		vis = "\x1b[?25h"
	}
	return fmt.Sprintf("\x1b[%d;%dH%s", scr.CurRow+1, scr.CurCol+1, vis)
}

// ── RenderCapture: full-range semantics ───────────────────────────

func TestRenderCapture_ExactFullBytes(t *testing.T) {
	scr := NewScreen(3, 4)
	scanRow(scr, 0, "AB")
	scanRow(scr, 2, "CD")
	scr.CurRow, scr.CurCol = 2, 2
	scr.CursorVisible = true

	plain, ansi, full := RenderCapture(scr, 0, 0, false)

	if want := "AB\n\nCD"; plain != want {
		t.Errorf("plain = %q, want %q", plain, want)
	}
	if want := "AB\x1b[0m\n\nCD\x1b[0m"; ansi != want {
		t.Errorf("ansi = %q, want %q", ansi, want)
	}
	want := "\x1b[1;1HAB\x1b[0m\x1b[K" +
		"\x1b[2;1H\x1b[0m\x1b[K" +
		"\x1b[3;1HCD\x1b[0m\x1b[K" +
		"\x1b[3;3H\x1b[?25h"
	if full != want {
		t.Errorf("full = %q, want %q", full, want)
	}
}

func TestRenderCapture_EmitsAllRows(t *testing.T) {
	scr := NewScreen(4, 10)
	scanRow(scr, 0, "A")
	_, _, out := RenderCapture(scr, 0, 0, false)

	for r := 1; r <= 4; r++ {
		cup := fmt.Sprintf("\x1b[%d;1H", r)
		if !strings.Contains(out, cup) {
			t.Errorf("missing CUP for row %d: %q", r, cup)
		}
	}
	if elCount := strings.Count(out, "\x1b[K"); elCount < 4 {
		t.Errorf("expected at least 4 EL sequences (one per row), got %d", elCount)
	}
}

func TestRenderCapture_NoScreenClear(t *testing.T) {
	scr := NewScreen(3, 5)
	scanRow(scr, 0, "X")
	_, _, out := RenderCapture(scr, 0, 0, false)

	if strings.Contains(out, "\x1b[2J") {
		t.Error("full-screen capture must not emit ESC[2J (erase display)")
	}
}

func TestRenderCapture_CursorPositionAndVisibility(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.CurRow, scr.CurCol = 1, 4
	scr.CursorVisible = false
	_, _, out := RenderCapture(scr, 0, 0, false)

	if !strings.Contains(out, "\x1b[2;5H") {
		t.Errorf("expected cursor at \\x1b[2;5H (1-indexed), got %q", out)
	}
	if !strings.Contains(out, "\x1b[?25l") {
		t.Error("should contain cursor-hide for CursorVisible=false")
	}
	if strings.Contains(out, "\x1b[?25h") {
		t.Error("should NOT contain cursor-show for CursorVisible=false")
	}
}

func TestRenderCapture_Idempotent(t *testing.T) {
	scr := NewScreen(3, 10)
	scanRow(scr, 0, "AZ")
	p1, a1, f1 := RenderCapture(scr, 0, 0, false)
	p2, a2, f2 := RenderCapture(scr, 0, 0, false)
	if p1 != p2 || a1 != a2 || f1 != f2 {
		t.Error("RenderCapture not idempotent")
	}
}

func TestRenderCapture_BoldTextAndWideChar(t *testing.T) {
	scr := NewScreen(3, 10)
	scr.CurRow, scr.CurCol = 0, 0
	scr.CurAttr = Attr{Bold: true}
	scr.PutChar('B')
	scr.CurAttr = Attr{}
	scr.PutChar('\u6F22') // 漢 - width 2

	_, ansi, _ := RenderCapture(scr, 0, 0, false)
	if !strings.Contains(ansi, "\x1b[") {
		t.Error("missing SGR sequence")
	}
	if strings.Count(ansi, "漢") != 1 {
		t.Error("wide char should appear exactly once")
	}
}

func TestRenderCapture_AfterModification(t *testing.T) {
	scr := NewScreen(5, 10)
	scanRow(scr, 0, "A")
	p1, _, _ := RenderCapture(scr, 0, 0, false)
	scr.Cells[0][1].Ch = 'B'
	p2, _, _ := RenderCapture(scr, 0, 0, false)

	if p1 == p2 {
		t.Error("RenderCapture should change after screen modification")
	}
	if p2 != "AB" {
		t.Errorf("updated capture = %q, want %q", p2, "AB")
	}
}

// ── RenderCapture: range normalization ────────────────────────────

func TestRenderCapture_RangeNormalization(t *testing.T) {
	scr := NewScreen(4, 4)
	for i := range 4 {
		scanRow(scr, i, fmt.Sprintf("R%d", i))
	}

	cases := []struct {
		name       string
		start, end int
		want       string
	}{
		{"negative start clamps", -5, 2, "R0\nR1"},
		{"zero end means all rows", 2, 0, "R2\nR3"},
		{"negative end means all rows", 1, -3, "R1\nR2\nR3"},
		{"end clamps to rows", 1, 99, "R1\nR2\nR3"},
		{"full range", 0, 4, "R0\nR1\nR2\nR3"},
		{"start equals end is empty", 3, 3, ""},
		{"inverted range is empty", 3, 1, ""},
		{"empty beyond rows", 9, 12, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plain, ansi, full := RenderCapture(scr, tc.start, tc.end, false)
			if plain != tc.want {
				t.Errorf("plain = %q, want %q", plain, tc.want)
			}
			if tc.want == "" {
				if ansi != "" || full != "" {
					t.Errorf("empty range must yield empty ansi/full, got %q / %q", ansi, full)
				}
				return
			}
		})
	}
}

func TestRenderCapture_RangeSelectsPhysicalRows(t *testing.T) {
	scr := NewScreen(4, 4)
	for i := range 4 {
		scanRow(scr, i, fmt.Sprintf("R%d", i))
	}
	scr.CurRow, scr.CurCol = 1, 1
	scr.CursorVisible = true

	plain, _, full := RenderCapture(scr, 2, 4, false)

	if want := "R2\nR3"; plain != want {
		t.Errorf("plain = %q, want %q", plain, want)
	}
	want := "\x1b[3;1HR2\x1b[0m\x1b[K" +
		"\x1b[4;1HR3\x1b[0m\x1b[K" +
		fullTail(scr)
	if full != want {
		t.Errorf("full = %q, want %q", full, want)
	}
	if strings.Contains(full, "\x1b[1;1H") || strings.Contains(full, "\x1b[2;1H") {
		t.Errorf("full-screen range must not emit CUP for unselected rows: %q", full)
	}
}

func TestRenderCapture_RangeKeepsCursorTail(t *testing.T) {
	scr := NewScreen(3, 4)
	scanRow(scr, 0, "A")
	scr.CurRow, scr.CurCol = 2, 3
	scr.CursorVisible = false

	_, _, fullRange := RenderCapture(scr, 0, 0, false)
	_, _, ranged := RenderCapture(scr, 0, 1, false)

	tail := "\x1b[3;4H\x1b[?25l"
	if !strings.HasSuffix(fullRange, tail) {
		t.Errorf("full capture must end with cursor tail %q: %q", tail, fullRange)
	}
	if !strings.HasSuffix(ranged, tail) {
		t.Errorf("ranged capture must keep the same cursor tail %q: %q", tail, ranged)
	}
}

// ── RenderCapture: wrap joining ───────────────────────────────────

func TestRenderCapture_JoinWrappedExactBytes(t *testing.T) {
	scr := NewScreen(3, 6)
	scanRow(scr, 0, "ABCD")
	scanRow(scr, 1, "EF")
	scanRow(scr, 2, "GH")
	scr.RowWrapped[1] = true

	plain, ansi, full := RenderCapture(scr, 0, 0, true)
	if want := "ABCDEF\nGH"; plain != want {
		t.Errorf("plain = %q, want %q", plain, want)
	}
	if want := "ABCD\x1b[0mEF\x1b[0m\nGH\x1b[0m"; ansi != want {
		t.Errorf("ansi = %q, want %q", ansi, want)
	}
	// Full-screen never joins: both rows keep their own CUP.
	if !strings.Contains(full, "\x1b[1;1H") || !strings.Contains(full, "\x1b[2;1H") {
		t.Errorf("full-screen must patch each row separately: %q", full)
	}
}

func TestRenderCapture_JoinWrappedOnlyAtTrueBoundaries(t *testing.T) {
	scr := NewScreen(3, 6)
	scanRow(scr, 0, "ABCD")
	scanRow(scr, 1, "EF")
	scanRow(scr, 2, "GH")
	scr.RowWrapped[1] = true
	scr.RowWrapped[2] = false

	plain, _, _ := RenderCapture(scr, 0, 0, false)
	if want := "ABCD\nEF\nGH"; plain != want {
		t.Errorf("joinWrapped=false plain = %q, want %q", plain, want)
	}

	plain, _, _ = RenderCapture(scr, 0, 0, true)
	if want := "ABCDEF\nGH"; plain != want {
		t.Errorf("joinWrapped=true plain = %q, want %q", plain, want)
	}
}

func TestRenderCapture_JoinWrappedRange(t *testing.T) {
	scr := NewScreen(3, 6)
	scanRow(scr, 0, "ABCD")
	scanRow(scr, 1, "EF")
	scanRow(scr, 2, "GH")
	scr.RowWrapped[1] = true

	// Selecting only the continuation row cannot join it to a row that is
	// outside the range.
	plain, _, _ := RenderCapture(scr, 1, 2, true)
	if want := "EF"; plain != want {
		t.Errorf("plain = %q, want %q", plain, want)
	}
	// Selecting both rows joins them.
	plain, _, _ = RenderCapture(scr, 0, 2, true)
	if want := "ABCDEF"; plain != want {
		t.Errorf("plain = %q, want %q", plain, want)
	}
}

// ── VisibleRowWrapped ─────────────────────────────────────────────

func TestVisibleRowWrapped_ScreenRows(t *testing.T) {
	scr := NewScreen(3, 4)
	scr.RowWrapped[1] = true

	if scr.VisibleRowWrapped(0) {
		t.Error("row 0 cannot be a continuation")
	}
	if !scr.VisibleRowWrapped(1) {
		t.Error("row 1 should report wrapped")
	}
	if scr.VisibleRowWrapped(2) {
		t.Error("row 2 should not report wrapped")
	}
	if scr.VisibleRowWrapped(-1) || scr.VisibleRowWrapped(3) {
		t.Error("out-of-range rows must report false")
	}
}

func TestVisibleRowWrapped_ScrollbackMapping(t *testing.T) {
	scr := NewScreen(2, 3)
	scr.Scrollback = [][]Cell{
		scanCells("A", 3),
		scanCells("B", 3),
		scanCells("C", 3),
	}
	scr.ScrollbackWrapped = []bool{false, true, false}
	scr.ScrollbackLen = 2
	scr.MaxScrollback = 3
	scr.ScrollbackHead = 2
	scanRow(scr, 0, "V")

	// ScrollOffset=1: the window slides one line toward older scrollback, so
	// visible row 0 is scrollback line 1 (wrapped=true) and visible row 1
	// is screen row 0 (unwrapped).
	scr.ScrollOffset = 1
	if !scr.VisibleRowWrapped(0) {
		t.Error("visible row 0 should map to wrapped scrollback line 1")
	}
	if scr.VisibleRowWrapped(1) {
		t.Error("visible row 1 should map to an unwrapped screen row")
	}

	// Without scrolling, the viewport shows the live tail: row 0 is screen
	// row 0 (unwrapped) and row 1 is screen row 1.
	scr.ScrollOffset = 0
	if scr.VisibleRowWrapped(0) {
		t.Error("visible row 0 should map to unwrapped screen row 0")
	}
}

func TestVisibleRowWrapped_RingHeadMapping(t *testing.T) {
	scr := NewScreen(2, 3)
	scr.Scrollback = [][]Cell{
		scanCells("A", 3),
		scanCells("B", 3),
		scanCells("C", 3),
	}
	// Ring is full; logical index 0 lives at physical head 1.
	scr.ScrollbackWrapped = []bool{false, true, true}
	scr.ScrollbackLen = 3
	scr.MaxScrollback = 3
	scr.ScrollbackHead = 1

	// Offset 0 shows the live tail (screen rows), so neither visible row
	// maps into scrollback.
	if scr.VisibleRowWrapped(0) {
		t.Error("live-tail row 0 must not report wrapped")
	}
	if scr.VisibleRowWrapped(1) {
		t.Error("live-tail row 1 must not report wrapped")
	}

	// Offset MaxScrollOffset shows the oldest window: visible rows 0 and 1
	// are logical scrollback 0 (physical 1, wrapped=true) and 1
	// (physical 2, wrapped=true).
	scr.ScrollOffset = scr.MaxScrollOffset()
	if !scr.VisibleRowWrapped(0) {
		t.Error("logical scrollback 0 maps to physical 1 (wrapped=true)")
	}
	if !scr.VisibleRowWrapped(1) {
		t.Error("logical scrollback 1 maps to physical 2 (wrapped=true)")
	}
}

func TestRenderCapture_ScrollbackWrapJoin(t *testing.T) {
	scr := NewScreen(2, 3)
	scr.Scrollback = [][]Cell{
		scanCells("S1", 3),
		scanCells("S2", 3),
	}
	scr.ScrollbackWrapped = []bool{false, true}
	scr.ScrollbackLen = 2
	scr.MaxScrollback = 4
	scanRow(scr, 0, "V1")
	scanRow(scr, 1, "V2")

	// Offset 0 shows the live tail.
	scr.ScrollOffset = 0
	plain, _, _ := RenderCapture(scr, 0, 2, true)
	if want := "V1\nV2"; plain != want {
		t.Errorf("live-tail plain = %q, want %q", plain, want)
	}

	// Offset ScrollbackLen shows the oldest window; S2 is a wrapped
	// continuation of S1, so joining yields one logical line.
	scr.ScrollOffset = scr.ScrollbackLen
	plain, _, _ = RenderCapture(scr, 0, 2, true)
	if want := "S1S2"; plain != want {
		t.Errorf("scrollback join plain = %q, want %q", plain, want)
	}
	plain, _, _ = RenderCapture(scr, 0, 2, false)
	if want := "S1\nS2"; plain != want {
		t.Errorf("scrollback split plain = %q, want %q", plain, want)
	}
}

func TestRenderCapture_ANSIHasNoPositioningSequences(t *testing.T) {
	scr := NewScreen(3, 10)
	scanRow(scr, 0, "AB")
	scanRow(scr, 2, "CD")
	cup := regexp.MustCompile("\x1b\\[[0-9]+;[0-9]+H")
	for _, join := range []bool{false, true} {
		for _, tc := range [][2]int{{0, 0}, {0, 1}, {1, 3}, {0, 3}} {
			_, ansi, _ := RenderCapture(scr, tc[0], tc[1], join)
			for _, seq := range []string{"\x1b[2J", "\x1b[K", "\x1b[?25h", "\x1b[?25l"} {
				if strings.Contains(ansi, seq) {
					t.Errorf("range %v join=%v: ANSI must not contain %q: %q", tc, join, seq, ansi)
				}
			}
			if cup.MatchString(ansi) {
				t.Errorf("range %v join=%v: ANSI must not contain CUP: %q", tc, join, ansi)
			}
		}
	}
}
