package vt

import "testing"

func TestFocusReporting_DECSET(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))
	if !v.primary.FocusReporting {
		t.Fatal("after DECSET ?1004h: FocusReporting = false, want true")
	}
}

func TestFocusReporting_DECRST(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))
	v.Write([]byte("\x1b[?1004l"))
	if v.primary.FocusReporting {
		t.Fatal("after DECRST ?1004l: FocusReporting = true, want false")
	}
}

func TestFocusReporting_DefaultFalse(t *testing.T) {
	s := NewScreen(24, 80)
	if s.FocusReporting {
		t.Fatal("new screen FocusReporting = true, want false")
	}
}

func TestFocusReporting_Snapshot(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))
	snap := v.ActiveScreen()
	if !snap.FocusReporting {
		t.Fatal("Snapshot FocusReporting = false, want true")
	}
	snap.FocusReporting = false
	if !v.primary.FocusReporting {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestFocusReporting_Accessor(t *testing.T) {
	v := NewVTerm(24, 80)
	if v.FocusReporting() {
		t.Fatal("default FocusReporting() = true, want false")
	}
	v.Write([]byte("\x1b[?1004h"))
	if !v.FocusReporting() {
		t.Fatal("after DECSET ?1004h: FocusReporting() = false, want true")
	}
}

func TestFocusReporting_AltScreenIndependent(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))
	v.Write([]byte("\x1b[?1049h"))
	if v.alternate.FocusReporting {
		t.Fatal("alternate screen should not inherit primary's FocusReporting")
	}
	v.Write([]byte("\x1b[?1049l"))
	if !v.primary.FocusReporting {
		t.Fatal("primary FocusReporting lost after alt screen round-trip")
	}
}

func TestFocusIn_ReportsWhenEnabled(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))

	var got []byte
	v.ResponseWriter = func(b []byte) { got = append(got, b...) }

	v.FocusIn()
	if string(got) != "\x1b[I" {
		t.Fatalf("FocusIn output = %q, want \\x1b[I", got)
	}
}

func TestFocusOut_ReportsWhenEnabled(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))

	var got []byte
	v.ResponseWriter = func(b []byte) { got = append(got, b...) }

	v.FocusOut()
	if string(got) != "\x1b[O" {
		t.Fatalf("FocusOut output = %q, want \\x1b[O", got)
	}
}

func TestFocusIn_NoOutputWhenDisabled(t *testing.T) {
	v := NewVTerm(24, 80)
	// FocusReporting is off by default.

	var got []byte
	v.ResponseWriter = func(b []byte) { got = append(got, b...) }

	v.FocusIn()
	if len(got) != 0 {
		t.Fatalf("FocusIn with reporting off: output = %q, want empty", got)
	}
}

func TestFocusOut_NoOutputWhenDisabled(t *testing.T) {
	v := NewVTerm(24, 80)

	var got []byte
	v.ResponseWriter = func(b []byte) { got = append(got, b...) }

	v.FocusOut()
	if len(got) != 0 {
		t.Fatalf("FocusOut with reporting off: output = %q, want empty", got)
	}
}

func TestFocusIn_NoPanicWhenResponseWriterNil(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))
	// ResponseWriter is nil by default.
	v.FocusIn() // should not panic
}

func TestFocusOut_NoPanicWhenResponseWriterNil(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))
	// ResponseWriter is nil by default.
	v.FocusOut() // should not panic
}

func TestFocusIn_FocusOut_Sequence(t *testing.T) {
	v := NewVTerm(24, 80)
	v.Write([]byte("\x1b[?1004h"))

	var got []byte
	v.ResponseWriter = func(b []byte) { got = append(got, b...) }

	v.FocusIn()
	v.FocusOut()
	if string(got) != "\x1b[I\x1b[O" {
		t.Fatalf("FocusIn+FocusOut output = %q, want \\x1b[I\\x1b[O", got)
	}
}
