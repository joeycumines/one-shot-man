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
