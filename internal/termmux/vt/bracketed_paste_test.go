package vt

import "testing"

func TestBracketedPaste_DECSET(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2004h"))
	if !v.primary.BracketedPaste {
		t.Fatal("after DECSET ?2004h: BracketedPaste = false, want true")
	}
}

func TestBracketedPaste_DECRST(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2004h"))
	v.Write([]byte("\x1b[?2004l"))
	if v.primary.BracketedPaste {
		t.Fatal("after DECRST ?2004l: BracketedPaste = true, want false")
	}
}

func TestBracketedPaste_Accessor(t *testing.T) {
	v := NewVTerm(24, 80)
	if v.BracketedPaste() {
		t.Fatal("default BracketedPaste = true, want false")
	}
	v.Write([]byte("\x1b[?2004h"))
	if !v.BracketedPaste() {
		t.Fatal("after DECSET ?2004h: BracketedPaste() = false, want true")
	}
}

func TestBracketedPaste_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2004h"))
	snap := v.ActiveScreen()
	if !snap.BracketedPaste {
		t.Fatal("Snapshot BracketedPaste = false, want true")
	}
	snap.BracketedPaste = false
	if !v.primary.BracketedPaste {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestBracketedPaste_DefaultFalse(t *testing.T) {
	s := NewScreen(24, 80)
	if s.BracketedPaste {
		t.Fatal("new screen BracketedPaste = true, want false")
	}
}

func TestBracketedPaste_AltScreenIndependent(t *testing.T) {
	v := NewVTerm(24, 80)
	// Enable on primary, switch to alt, verify alt doesn't have it.
	v.Write([]byte("\x1b[?2004h"))
	v.Write([]byte("\x1b[?1049h"))
	if v.alternate.BracketedPaste {
		t.Fatal("alternate screen should not inherit primary's BracketedPaste")
	}
	// Switch back, primary still has it.
	v.Write([]byte("\x1b[?1049l"))
	if !v.primary.BracketedPaste {
		t.Fatal("primary BracketedPaste lost after alt screen round-trip")
	}
}
