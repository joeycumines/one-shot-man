package vt

import "testing"

func TestSynchronizedOutput_DECSET(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2026h"))
	if !v.primary.SynchronizedOutput {
		t.Fatal("after DECSET ?2026h: SynchronizedOutput = false, want true")
	}
}

func TestSynchronizedOutput_DECRST(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2026h"))
	v.Write([]byte("\x1b[?2026l"))
	if v.primary.SynchronizedOutput {
		t.Fatal("after DECRST ?2026l: SynchronizedOutput = true, want false")
	}
}

func TestSynchronizedOutput_DefaultFalse(t *testing.T) {
	s := NewScreen(24, 80)
	if s.SynchronizedOutput {
		t.Fatal("new screen SynchronizedOutput = true, want false")
	}
}

func TestSynchronizedOutput_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2026h"))
	snap := v.ActiveScreen()
	if !snap.SynchronizedOutput {
		t.Fatal("Snapshot SynchronizedOutput = false, want true")
	}
	snap.SynchronizedOutput = false
	if !v.primary.SynchronizedOutput {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestSynchronizedOutput_Accessor(t *testing.T) {
	v := NewVTerm(24, 80)
	if v.SynchronizedOutput() {
		t.Fatal("default SynchronizedOutput() = true, want false")
	}
	v.Write([]byte("\x1b[?2026h"))
	if !v.SynchronizedOutput() {
		t.Fatal("after DECSET ?2026h: SynchronizedOutput() = false, want true")
	}
}

func TestSynchronizedOutput_AltScreenIndependent(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?2026h"))
	v.Write([]byte("\x1b[?1049h"))
	if v.alternate.SynchronizedOutput {
		t.Fatal("alternate screen should not inherit primary's SynchronizedOutput")
	}
	v.Write([]byte("\x1b[?1049l"))
	if !v.primary.SynchronizedOutput {
		t.Fatal("primary SynchronizedOutput lost after alt screen round-trip")
	}
}

func TestSynchronizedOutput_VTermStillUpdated(t *testing.T) {
	// When synchronized output is on, VTerm should still process output.
	// The mode only affects snapshot publication, not VTerm state.
	v := NewVTerm(1, 10)
	v.Write([]byte("\x1b[?2026h"))
	v.Write([]byte("hello"))
	if v.String() != "hello" {
		t.Fatalf("VTerm content = %q, want %q", v.String(), "hello")
	}
}
