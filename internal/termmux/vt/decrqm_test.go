package vt

import "testing"

func TestDECRQM_CursorKeyMode_Set(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[?1h")) // DECCKM set
	v.Write([]byte("\x1b[?1$p"))

	if string(response) != "\x1b[?1;2$y" {
		t.Errorf("DECRQM mode 1 set = %q, want %q", response, "\x1b[?1;2$y")
	}
}

func TestDECRQM_CursorKeyMode_Reset(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// DECCKM is reset by default.
	v.Write([]byte("\x1b[?1$p"))

	if string(response) != "\x1b[?1;3$y" {
		t.Errorf("DECRQM mode 1 reset = %q, want %q", response, "\x1b[?1;3$y")
	}
}

func TestDECRQM_OriginMode_Set(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[?6h")) // DECOM set
	v.Write([]byte("\x1b[?6$p"))

	if string(response) != "\x1b[?6;2$y" {
		t.Errorf("DECRQM mode 6 set = %q, want %q", response, "\x1b[?6;2$y")
	}
}

func TestDECRQM_AutoWrap_Set(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// DECAWM is set by default.
	v.Write([]byte("\x1b[?7$p"))

	if string(response) != "\x1b[?7;2$y" {
		t.Errorf("DECRQM mode 7 set = %q, want %q", response, "\x1b[?7;2$y")
	}
}

func TestDECRQM_CursorVisible_Set(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// DECTCEM is set by default (cursor visible).
	v.Write([]byte("\x1b[?25$p"))

	if string(response) != "\x1b[?25;2$y" {
		t.Errorf("DECRQM mode 25 set = %q, want %q", response, "\x1b[?25;2$y")
	}
}

func TestDECRQM_UnrecognizedMode(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[?999$p"))

	if string(response) != "\x1b[?999;1$y" {
		t.Errorf("DECRQM unrecognized mode = %q, want %q", response, "\x1b[?999;1$y")
	}
}

func TestDECRQM_BracketedPaste_Set(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1b[?2004h")) // bracketed paste set
	v.Write([]byte("\x1b[?2004$p"))

	if string(response) != "\x1b[?2004;2$y" {
		t.Errorf("DECRQM mode 2004 set = %q, want %q", response, "\x1b[?2004;2$y")
	}
}

func TestDECRQM_KeypadApplication_Reset(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// DECNKM is reset by default.
	v.Write([]byte("\x1b[?66$p"))

	if string(response) != "\x1b[?66;3$y" {
		t.Errorf("DECRQM mode 66 reset = %q, want %q", response, "\x1b[?66;3$y")
	}
}
