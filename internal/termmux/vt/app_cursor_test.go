package vt

import "testing"

func TestApplicationCursor_DECSET(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1h"))
	if !v.primary.ApplicationCursor {
		t.Fatal("after DECSET ?1h: ApplicationCursor = false, want true")
	}
}

func TestApplicationCursor_DECRST(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1h"))
	v.Write([]byte("\x1b[?1l"))
	if v.primary.ApplicationCursor {
		t.Fatal("after DECRST ?1l: ApplicationCursor = true, want false")
	}
}

func TestApplicationCursor_DefaultFalse(t *testing.T) {
	s := NewScreen(24, 80)
	if s.ApplicationCursor {
		t.Fatal("new screen ApplicationCursor = true, want false")
	}
}

func TestApplicationCursor_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1h"))
	snap := v.ActiveScreen()
	if !snap.ApplicationCursor {
		t.Fatal("Snapshot ApplicationCursor = false, want true")
	}
	snap.ApplicationCursor = false
	if !v.primary.ApplicationCursor {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestApplicationCursor_Accessor(t *testing.T) {
	v := NewVTerm(24, 80)
	if v.ApplicationCursor() {
		t.Fatal("default ApplicationCursor() = true, want false")
	}
	v.Write([]byte("\x1b[?1h"))
	if !v.ApplicationCursor() {
		t.Fatal("after DECSET ?1h: ApplicationCursor() = false, want true")
	}
}

func TestApplicationCursor_AltScreenIndependent(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1h"))
	v.Write([]byte("\x1b[?1049h"))
	if v.alternate.ApplicationCursor {
		t.Fatal("alternate screen should not inherit primary's ApplicationCursor")
	}
	v.Write([]byte("\x1b[?1049l"))
	if !v.primary.ApplicationCursor {
		t.Fatal("primary ApplicationCursor lost after alt screen round-trip")
	}
}
