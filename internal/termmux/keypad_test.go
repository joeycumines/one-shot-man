package termmux

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func TestDECKPAM_SetMode66(t *testing.T) {
	v := vt.NewVTerm(24, 80)
	v.Write([]byte("\x1b[?66h"))
	if !v.KeypadApplication() {
		t.Fatal("after DECSET ?66h: KeypadApplication = false, want true")
	}
}

func TestDECKPAM_ResetMode66(t *testing.T) {
	v := vt.NewVTerm(24, 80)
	v.Write([]byte("\x1b[?66h"))
	v.Write([]byte("\x1b[?66l"))
	if v.KeypadApplication() {
		t.Fatal("after DECRST ?66l: KeypadApplication = true, want false")
	}
}

func TestDECKPAM_Snapshot(t *testing.T) {
	v := vt.NewVTerm(24, 80)
	v.Write([]byte("\x1b[?66h"))
	snap := v.ActiveScreen()
	if !snap.KeypadApplication {
		t.Fatal("Snapshot KeypadApplication = false, want true")
	}
	snap.KeypadApplication = false
	if !v.KeypadApplication() {
		t.Fatal("modifying snapshot affected original")
	}
}

func TestDECKPAM_Accessor(t *testing.T) {
	v := vt.NewVTerm(24, 80)
	if v.KeypadApplication() {
		t.Fatal("default KeypadApplication() = true, want false")
	}
	v.Write([]byte("\x1b[?66h"))
	if !v.KeypadApplication() {
		t.Fatal("after DECSET ?66h: KeypadApplication() = false, want true")
	}
}

func TestKeyToTermBytes_KeypadAppMode(t *testing.T) {
	seq, ok := KeyToTermBytes("kp0", false, true)
	if !ok {
		t.Fatal("kp0 not recognized")
	}
	if seq != "\x1bOp" {
		t.Fatalf("kp0 app mode = %q, want %q", seq, "\x1bOp")
	}
}

func TestKeyToTermBytes_KeypadNormalMode(t *testing.T) {
	seq, ok := KeyToTermBytes("kp0", false, false)
	if !ok {
		t.Fatal("kp0 not recognized")
	}
	if seq != "0" {
		t.Fatalf("kp0 normal mode = %q, want %q", seq, "0")
	}
}

func TestKeyToTermBytes_KeypadAllKeys(t *testing.T) {
	tests := []struct {
		key string
		seq string
	}{
		{"kp0", "\x1bOp"},
		{"kp1", "\x1bOq"},
		{"kp2", "\x1bOr"},
		{"kp3", "\x1bOs"},
		{"kp4", "\x1bOt"},
		{"kp5", "\x1bOu"},
		{"kp6", "\x1bOv"},
		{"kp7", "\x1bOw"},
		{"kp8", "\x1bOx"},
		{"kp9", "\x1bOy"},
		{"kp_enter", "\x1bOM"},
		{"kp_plus", "\x1bOk"},
		{"kp_minus", "\x1bOm"},
		{"kp_asterisk", "\x1bOj"},
		{"kp_star", "\x1bOj"},
		{"kp_slash", "\x1bOo"},
		{"kp_dot", "\x1bOn"},
		{"kp_period", "\x1bOn"},
		{"kp_comma", "\x1bOl"},
		{"kp_equal", "\x1bOX"},
	}
	for _, tt := range tests {
		seq, ok := KeyToTermBytes(tt.key, false, true)
		if !ok {
			t.Errorf("%s not recognized in app mode", tt.key)
			continue
		}
		if seq != tt.seq {
			t.Errorf("%s app mode = %q, want %q", tt.key, seq, tt.seq)
		}
	}
}

func TestKeyToTermBytes_KeypadNormalAllKeys(t *testing.T) {
	tests := []struct {
		key string
		seq string
	}{
		{"kp0", "0"},
		{"kp1", "1"},
		{"kp2", "2"},
		{"kp3", "3"},
		{"kp4", "4"},
		{"kp5", "5"},
		{"kp6", "6"},
		{"kp7", "7"},
		{"kp8", "8"},
		{"kp9", "9"},
		{"kp_enter", "\r"},
		{"kp_plus", "+"},
		{"kp_minus", "-"},
		{"kp_asterisk", "*"},
		{"kp_star", "*"},
		{"kp_slash", "/"},
		{"kp_dot", "."},
		{"kp_period", "."},
		{"kp_comma", ","},
		{"kp_equal", "="},
	}
	for _, tt := range tests {
		seq, ok := KeyToTermBytes(tt.key, false, false)
		if !ok {
			t.Errorf("%s not recognized in normal mode", tt.key)
			continue
		}
		if seq != tt.seq {
			t.Errorf("%s normal mode = %q, want %q", tt.key, seq, tt.seq)
		}
	}
}
