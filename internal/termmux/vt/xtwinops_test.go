package vt

import "testing"

func TestXTWINOPS_ReportSize(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[18t"))

	if response == nil {
		t.Fatal("expected XTWINOPS 18 response, got nil")
	}
	want := "\x1b[8;480;800t"
	if string(response) != want {
		t.Errorf("XTWINOPS 18 response = %q, want %q", response, want)
	}
}

func TestXTWINOPS_ResizeTerminal(t *testing.T) {
	v := NewVTerm(24, 80)

	v.Write([]byte("\x1b[8;30;120t"))

	scr := v.ActiveScreen()
	if scr.Rows != 30 {
		t.Errorf("rows after resize = %d, want 30", scr.Rows)
	}
	if scr.Cols != 120 {
		t.Errorf("cols after resize = %d, want 120", scr.Cols)
	}
}

func TestXTWINOPS_ResizeTerminalDefault(t *testing.T) {
	v := NewVTerm(24, 80)

	// CSI 8 t with no row/col params should use defaults (24, 80).
	v.Write([]byte("\x1b[8t"))

	scr := v.ActiveScreen()
	if scr.Rows != 24 {
		t.Errorf("rows after default resize = %d, want 24", scr.Rows)
	}
	if scr.Cols != 80 {
		t.Errorf("cols after default resize = %d, want 80", scr.Cols)
	}
}

func TestXTWINOPS_UnknownSubcommand(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// CSI 22 t is an unimplemented XTWINOPS subcommand — must be silently ignored.
	v.Write([]byte("\x1b[22t"))

	if response != nil {
		t.Errorf("unknown XTWINOPS should not respond, got %q", response)
	}
	scr := v.ActiveScreen()
	if scr.Rows != 24 || scr.Cols != 80 {
		t.Errorf("screen dimensions changed after unknown XTWINOPS: rows=%d cols=%d", scr.Rows, scr.Cols)
	}
}

func TestXTWINOPS_ReportSizeAfterResize(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[8;40;100t"))
	response = nil

	v.Write([]byte("\x1b[18t"))

	if response == nil {
		t.Fatal("expected XTWINOPS 18 response after resize, got nil")
	}
	want := "\x1b[8;800;1000t"
	if string(response) != want {
		t.Errorf("XTWINOPS 18 after resize = %q, want %q", response, want)
	}
	scr := v.ActiveScreen()
	if scr.Rows != 40 || scr.Cols != 100 {
		t.Errorf("screen dimensions after resize: rows=%d cols=%d, want 40x100", scr.Rows, scr.Cols)
	}
}
