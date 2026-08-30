package vt

import (
	"testing"
)

// TestDECSC_DECRC_SGRAllFields verifies that DECSC saves and DECRC restores
// every field of the current attribute (SGR) state, including colors, styles,
// and flags.
func TestDECSC_DECRC_SGRAllFields(t *testing.T) {
	v := NewVTerm(10, 40)
	scr := v.primary

	want := Attr{
		FG:      color{kind: kindRGB, value: 0x123456},
		BG:      color{kind: kind256, value: 200},
		Bold:    true,
		Dim:     true,
		Italic:  true,
		Under:   true,
		Blink:   true,
		Inverse: true,
		Hidden:  true,
		Strike:  true,
	}

	scr.CurAttr = want
	v.Write([]byte("\x1b7")) // DECSC

	// Mutate current attributes to non-default values different from want.
	scr.CurAttr = Attr{
		FG:      color{kind: kind8, value: 7},
		BG:      color{kind: kind8, value: 0},
		Bold:    false,
		Dim:     false,
		Italic:  false,
		Under:   false,
		Blink:   false,
		Inverse: false,
		Hidden:  false,
		Strike:  false,
	}

	v.Write([]byte("\x1b8")) // DECRC

	got := scr.CurAttr
	if got.FG != want.FG {
		t.Errorf("FG = %+v, want %+v", got.FG, want.FG)
	}
	if got.BG != want.BG {
		t.Errorf("BG = %+v, want %+v", got.BG, want.BG)
	}
	if got.Bold != want.Bold {
		t.Errorf("Bold = %v, want %v", got.Bold, want.Bold)
	}
	if got.Dim != want.Dim {
		t.Errorf("Dim = %v, want %v", got.Dim, want.Dim)
	}
	if got.Italic != want.Italic {
		t.Errorf("Italic = %v, want %v", got.Italic, want.Italic)
	}
	if got.Under != want.Under {
		t.Errorf("Under = %v, want %v", got.Under, want.Under)
	}
	if got.Blink != want.Blink {
		t.Errorf("Blink = %v, want %v", got.Blink, want.Blink)
	}
	if got.Inverse != want.Inverse {
		t.Errorf("Inverse = %v, want %v", got.Inverse, want.Inverse)
	}
	if got.Hidden != want.Hidden {
		t.Errorf("Hidden = %v, want %v", got.Hidden, want.Hidden)
	}
	if got.Strike != want.Strike {
		t.Errorf("Strike = %v, want %v", got.Strike, want.Strike)
	}
}

// TestDECSC_DECRC_CharsetGL verifies that DECSC saves and DECRC restores
// G0/G1 charset designations and the active GL selection.
// Note: this VT implementation currently supports only G0 and G1 charset
// designations; G2/G3 are not implemented.
func TestDECSC_DECRC_CharsetGL(t *testing.T) {
	v := NewVTerm(10, 40)
	scr := v.primary

	scr.G0Charset = 1 // VT100 line-drawing
	scr.G1Charset = 0 // ASCII
	scr.GL = 1        // G1 active

	v.Write([]byte("\x1b7")) // DECSC

	scr.G0Charset = 0
	scr.G1Charset = 1
	scr.GL = 0

	v.Write([]byte("\x1b8")) // DECRC

	if scr.G0Charset != 1 {
		t.Errorf("G0Charset = %d, want 1", scr.G0Charset)
	}
	if scr.G1Charset != 0 {
		t.Errorf("G1Charset = %d, want 0", scr.G1Charset)
	}
	if scr.GL != 1 {
		t.Errorf("GL = %d, want 1", scr.GL)
	}
}

// TestDECSC_DECRC_OriginMode verifies that DECSC saves and DECRC restores the
// origin mode (DECOM) state.
func TestDECSC_DECRC_OriginMode(t *testing.T) {
	v := NewVTerm(10, 40)
	scr := v.primary

	scr.SetScrollRegion(3, 8) // 1-indexed
	scr.OriginMode = true

	v.Write([]byte("\x1b7")) // DECSC

	scr.OriginMode = false

	v.Write([]byte("\x1b8")) // DECRC

	if !scr.OriginMode {
		t.Error("OriginMode = false, want true after DECRC")
	}
}

// TestDECSC_DECRC_SGRDoesNotSaveSearchMatch verifies that the virtual
// SearchMatch rendering hint is not part of the saved SGR state.
func TestDECSC_DECRC_SGRDoesNotSaveSearchMatch(t *testing.T) {
	v := NewVTerm(10, 40)
	scr := v.primary

	scr.CurAttr.SearchMatch = true
	v.Write([]byte("\x1b7")) // DECSC

	scr.CurAttr.SearchMatch = false
	v.Write([]byte("\x1b8")) // DECRC

	if scr.CurAttr.SearchMatch {
		t.Error("SearchMatch should not be restored by DECRC")
	}
}
