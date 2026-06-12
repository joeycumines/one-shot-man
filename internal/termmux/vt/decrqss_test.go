package vt

import "testing"

func TestDECRQSS_SGR(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Default attributes → SGR params "0".
	v.Write([]byte("\x1bP$qm\x1b\\"))

	want := "\x1bP1$r0m\x1b\\"
	if string(response) != want {
		t.Errorf("DECRQSS SGR default = %q, want %q", response, want)
	}
}

func TestDECRQSS_DECSTBM(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Default scroll region is full screen (1;24 for 24-row terminal).
	v.Write([]byte("\x1bP$qr\x1b\\"))

	want := "\x1bP1$r1;24r\x1b\\"
	if string(response) != want {
		t.Errorf("DECRQSS DECSTBM default = %q, want %q", response, want)
	}
}

func TestDECRQSS_Unrecognized(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	v.Write([]byte("\x1bP$qz\x1b\\"))

	want := "\x1bP0$r\x1b\\"
	if string(response) != want {
		t.Errorf("DECRQSS unrecognized = %q, want %q", response, want)
	}
}

func TestDECRQSS_SGRAfterSettingAttributes(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// Set bold + red foreground.
	v.Write([]byte("\x1b[1;31m"))
	v.Write([]byte("\x1bP$qm\x1b\\"))

	want := "\x1bP1$r1;31m\x1b\\"
	if string(response) != want {
		t.Errorf("DECRQSS SGR after setting = %q, want %q", response, want)
	}
}
