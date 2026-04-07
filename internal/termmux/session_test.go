package termmux

import "testing"

func TestSessionTarget_String(t *testing.T) {
	tests := []struct {
		name string
		got  SessionTarget
		want string
	}{
		{name: "zero", got: SessionTarget{}, want: "unknown"},
		{name: "kind", got: SessionTarget{Kind: SessionKindPTY}, want: "pty"},
		{name: "name", got: SessionTarget{Name: "claude"}, want: "claude"},
		{name: "name-kind", got: SessionTarget{Name: "verify", Kind: SessionKindCapture}, want: "verify[capture]"},
		{name: "name-kind-id", got: SessionTarget{Name: "shell", Kind: SessionKindCapture, ID: "s-1"}, want: "shell[capture:s-1]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got.String(); got != tc.want {
				t.Fatalf("SessionTarget.String() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestCaptureSession_Target(t *testing.T) {
	cs := NewCaptureSession(CaptureConfig{
		Name:    "verify",
		Kind:    SessionKindCapture,
		Command: "echo",
		Args:    []string{"hello"},
	})

	got := cs.Target()
	if got.Name != "verify" {
		t.Fatalf("Target().Name = %q; want %q", got.Name, "verify")
	}
	if got.Kind != SessionKindCapture {
		t.Fatalf("Target().Kind = %v; want %v", got.Kind, SessionKindCapture)
	}

	updated := SessionTarget{Name: "shell", Kind: SessionKindPTY, ID: "shell-1"}
	cs.SetTarget(updated)
	if got := cs.Target(); got != updated {
		t.Fatalf("Target() after SetTarget = %#v; want %#v", got, updated)
	}
}
