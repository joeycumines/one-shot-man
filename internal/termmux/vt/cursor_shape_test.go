package vt

import "testing"

func TestDECSCUSR_Block(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1 q"))
	if v.primary.CursorShape != 1 {
		t.Fatalf("after CSI 1 SP q: CursorShape = %d, want 1", v.primary.CursorShape)
	}
}

func TestDECSCUSR_Underline(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[3 q"))
	if v.primary.CursorShape != 3 {
		t.Fatalf("after CSI 3 SP q: CursorShape = %d, want 3", v.primary.CursorShape)
	}
}

func TestDECSCUSR_Bar(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5 q"))
	if v.primary.CursorShape != 5 {
		t.Fatalf("after CSI 5 SP q: CursorShape = %d, want 5", v.primary.CursorShape)
	}
}

func TestDECSCUSR_Default(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5 q"))
	v.Write([]byte("\x1b[0 q"))
	if v.primary.CursorShape != 0 {
		t.Fatalf("after CSI 0 SP q: CursorShape = %d, want 0", v.primary.CursorShape)
	}
}

func TestDECSCUSR_SteadyVariants(t *testing.T) {
	v := NewVTerm(24, 80)

	v.Write([]byte("\x1b[2 q"))
	if v.primary.CursorShape != 2 {
		t.Fatalf("CSI 2 SP q: CursorShape = %d, want 2 (steady-block)", v.primary.CursorShape)
	}

	v.Write([]byte("\x1b[4 q"))
	if v.primary.CursorShape != 4 {
		t.Fatalf("CSI 4 SP q: CursorShape = %d, want 4 (steady-underline)", v.primary.CursorShape)
	}

	v.Write([]byte("\x1b[6 q"))
	if v.primary.CursorShape != 6 {
		t.Fatalf("CSI 6 SP q: CursorShape = %d, want 6 (steady-bar)", v.primary.CursorShape)
	}
}

func TestDECSCUSR_MissingParam(t *testing.T) {
	// CSI SP q with no numeric parameter should default to 0.
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5 q")) // set to non-zero first
	v.Write([]byte("\x1b[ q"))  // missing param → default 0
	if v.primary.CursorShape != 0 {
		t.Fatalf("after CSI SP q with missing param: CursorShape = %d, want 0", v.primary.CursorShape)
	}
}

func TestDECSCUSR_InvalidValue(t *testing.T) {
	// Values outside 0-6 should clamp to 0 (default).
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[7 q"))
	if v.primary.CursorShape != 0 {
		t.Fatalf("after CSI 7 SP q: CursorShape = %d, want 0 (clamped)", v.primary.CursorShape)
	}
}

func TestDECSCUSR_NoSpaceIgnored(t *testing.T) {
	// CSI q without intermediate space should NOT set CursorShape.
	// Without the space, this is not DECSCUSR.
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[1q"))
	if v.primary.CursorShape != 0 {
		t.Fatalf("after CSI 1q (no space): CursorShape = %d, want 0 (unchanged)", v.primary.CursorShape)
	}
}

func TestDECSCUSR_Accessor(t *testing.T) {
	v := NewVTerm(24, 80)
	if v.CursorShape() != 0 {
		t.Fatalf("default CursorShape = %d, want 0", v.CursorShape())
	}
	v.Write([]byte("\x1b[3 q"))
	if v.CursorShape() != 3 {
		t.Fatalf("after CSI 3 SP q: CursorShape() = %d, want 3", v.CursorShape())
	}
}

func TestDECSCUSR_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[5 q"))
	snap := v.ActiveScreen()
	if snap.CursorShape != 5 {
		t.Fatalf("Snapshot CursorShape = %d, want 5", snap.CursorShape)
	}
	snap.CursorShape = 0
	if v.primary.CursorShape != 5 {
		t.Fatalf("modifying snapshot affected original: CursorShape = %d, want 5", v.primary.CursorShape)
	}
}

func TestDECSCUSR_DefaultZero(t *testing.T) {
	s := NewScreen(24, 80)
	if s.CursorShape != 0 {
		t.Fatalf("new screen CursorShape = %d, want 0", s.CursorShape)
	}
}

func TestDECSCUSR_AltScreenIndependent(t *testing.T) {
	v := NewVTerm(24, 80)
	// Set cursor shape on primary, switch to alt, verify alt doesn't have it.
	v.Write([]byte("\x1b[5 q"))
	v.Write([]byte("\x1b[?1049h"))
	if v.alternate.CursorShape != 0 {
		t.Fatalf("alternate screen should not inherit primary's CursorShape, got %d", v.alternate.CursorShape)
	}
	// Switch back, primary still has it.
	v.Write([]byte("\x1b[?1049l"))
	if v.primary.CursorShape != 5 {
		t.Fatalf("primary CursorShape lost after alt screen round-trip: got %d, want 5", v.primary.CursorShape)
	}
}
