package vt

import (
	"strings"
	"testing"
)

// TestUnderlineSpansErasedGaps pins the behaviour behind the "per-word
// underline" appearance.
//
// An application that underlines a phrase often writes each word and moves the
// cursor between them with an absolute column jump (HPA, ESC[<n>G) instead of
// writing the spaces. The cells in those gaps are never written, so they hold
// whatever was there: after a background-colour erase they are blanks carrying
// only the background, with NO underline.
//
// That is not a defect in the renderer — it is what a real terminal does, and
// it was verified against tmux directly: feeding tmux the same sequence
// produces the same per-word underline in `capture-pane -e`. A renderer that
// instead stamped the whole pen onto the erased gaps would show a continuous
// underline that the real terminal does not show, which is the divergence this
// test guards.
//
// So this test asserts BOTH halves of the contract:
//   - the gap cells carry no underline (matching a real terminal's BCE), and
//   - the words themselves keep theirs, contiguously within each word.
func TestUnderlineSpansErasedGaps(t *testing.T) {
	t.Parallel()

	v := NewVTerm(4, 40)
	// Underline on, erase the line (BCE), then write words at absolute columns.
	if _, err := v.Write([]byte("\x1b[4m\x1b[2K\x1b[1GNow\x1b[5Glet")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	scr := v.ActiveScreen()
	for _, tc := range []struct {
		col  int
		want bool
	}{{0, true}, {1, true}, {2, true}, {3, false}, {4, true}, {5, true}, {6, true}} {
		if got := scr.Cells[0][tc.col].Attr.Under; got != tc.want {
			t.Errorf("col %d underline = %v, want %v (gaps keep no rendition; words keep theirs)", tc.col, got, tc.want)
		}
	}

	// Within a word the underline must be continuous: no reset may land between
	// the first and last letter of "Now".
	_, ansi, _ := RenderCapture(scr, 0, 0, false)
	start := strings.Index(ansi, "N")
	end := strings.Index(ansi, "w")
	if start < 0 || end < start {
		t.Fatalf("could not locate the first word in %q", ansi)
	}
	if run := ansi[start:end]; strings.Contains(run, "\x1b[0m") {
		t.Errorf("a reset landed inside the word, breaking the underline: %q", run)
	}
}
