package vt

import "testing"

func TestHasIntermediateDollar_CSI(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// CSI ? 1 $ p = DECRQM for DECCKM (mode 1).
	// The '$' intermediate must be detected for DECRQM to produce a response.
	v.Write([]byte("\x1b[?1$p"))

	if response == nil {
		t.Fatal("expected DECRQM response, got nil — '$' intermediate not detected")
	}
	want := "\x1b[?1;3$y"
	if string(response) != want {
		t.Errorf("DECRQM response = %q, want %q", response, want)
	}
}

func TestHasIntermediateDollar_DCS(t *testing.T) {
	v := NewVTerm(24, 80)
	var response []byte
	v.ResponseWriter = func(data []byte) {
		response = data
	}

	// DCS $ q m ST = DECRQSS for SGR.
	// The '$q' prefix in DCS data must be detected for DECRQSS to produce a response.
	v.Write([]byte("\x1bP$qm\x1b\\"))

	if response == nil {
		t.Fatal("expected DECRQSS response, got nil — '$q' prefix not detected in DCS data")
	}
	want := "\x1bP1$r0m\x1b\\"
	if string(response) != want {
		t.Errorf("DECRQSS response = %q, want %q", response, want)
	}
}
