package vt

import (
	"strings"
	"testing"
)

// TestEraseDoesNotInheritUnderline guards the "artifacts on every line"
// corruption.
//
// An application legitimately turns underline on for its prompt marker and for
// horizontal rules. A real terminal treats an ERASE as background-colour erase:
// the blank inherits the current BACKGROUND and nothing else, so a cleared row
// stays visually empty. If the erase instead stamps the whole pen onto the
// blank, every erased cell becomes underlined, and since a space with underline
// set renders as a visible line, the whole screen fills with stray rules.
func TestEraseDoesNotInheritUnderline(t *testing.T) {
	t.Parallel()

	v := NewVTerm(6, 20)
	// Underline on, then erase the line, exactly as a TUI does around its
	// prompt marker and rules.
	if _, err := v.Write([]byte("\x1b[4m\x1b[2K")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	scr := v.ActiveScreen()
	for c := range scr.Cols {
		if scr.Cells[0][c].Attr.Under {
			t.Fatalf("erased cell at col %d is underlined; an erased blank must not carry the rendition", c)
		}
	}

	// The rendered frame must not describe the blank row as underlined either:
	// a reset alone, with no underline re-asserted over empty cells.
	_, _, full := RenderCapture(scr, 0, 0, false)
	if strings.Contains(full, "\x1b[4m") {
		t.Errorf("rendered frame re-asserts underline over an erased row: %q", full)
	}
}

// TestEraseKeepsBackground is the complement: background-colour erase means the
// background MUST survive an erase, since that is how applications paint a
// full-width band.
func TestEraseKeepsBackground(t *testing.T) {
	t.Parallel()

	v := NewVTerm(6, 20)
	if _, err := v.Write([]byte("\x1b[48;5;16m\x1b[2K")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	scr := v.ActiveScreen()
	if got := scr.Cells[0][0].Attr.BG; got.kind == kindDefault {
		t.Errorf("erased cell lost its background: %+v", got)
	}
}

// TestEraseKeepsWideGraphemeSafety is a guard for the erase path when the pen
// also carries rendition flags: erasing must not smear them onto neighbouring
// cells either.
//
// A double-width character occupies two cells. Erasing only its first half
// orphans the placeholder second half, and repairWideBoundary blanks that
// orphaned half so no stale glyph remains. The repair goes through
// background-colour erase exactly like any other erase path: with the pen
// carrying underline at repair time, the blanked cells must still carry no
// rendition, or the boundary renders as a stray marked cell in an otherwise
// cleared region.
func TestEraseKeepsWideGraphemeSafety(t *testing.T) {
	t.Parallel()

	v := NewVTerm(4, 10)
	// Write the wide char (occupying zero-based cols 3 and 4) with a clean
	// pen, then turn underline on and erase exactly one cell starting at the
	// wide char's first half. ECH repairs the orphaned second half.
	if _, err := v.Write([]byte("\x1b[1;4H世\x1b[4m\x1b[1;4H\x1b[1X")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	scr := v.ActiveScreen()
	for _, c := range []int{3, 4} {
		cell := scr.Cells[0][c]
		if cell.Ch != ' ' || cell.SecondHalf {
			t.Errorf("repaired cell at col %d = %q (secondHalf=%v), want a blank placeholder-free cell", c, cell.Ch, cell.SecondHalf)
		}
		if cell.Attr.Under {
			t.Errorf("repaired cell at col %d is underlined; an erase-repaired boundary must not inherit the pen rendition", c)
		}
	}

	// The untouched cells on the row must be equally free of smearing.
	for c := range 3 {
		if scr.Cells[0][c].Attr.Under {
			t.Errorf("col %d is underlined; the repair must not smear the pen onto neighbours", c)
		}
	}
}
