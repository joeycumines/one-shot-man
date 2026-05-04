package termmux

import (
	"fmt"
	"testing"
)

func TestExitReason_String(t *testing.T) {
	tests := []struct {
		reason ExitReason
		want   string
	}{
		{ExitToggle, "toggle"},
		{ExitChildExit, "child-exit"},
		{ExitContext, "context"},
		{ExitError, "error"},
		{ExitReason(99), "unknown(99)"},
		{ExitReason(-1), "unknown(-1)"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("ExitReason(%d)", int(tt.reason)), func(t *testing.T) {
			if got := tt.reason.String(); got != tt.want {
				t.Errorf("ExitReason(%d).String() = %q, want %q", int(tt.reason), got, tt.want)
			}
		})
	}
}

func TestExitReason_iota_values(t *testing.T) {
	if ExitToggle != 0 {
		t.Errorf("ExitToggle = %d, want 0", ExitToggle)
	}
	if ExitChildExit != 1 {
		t.Errorf("ExitChildExit = %d, want 1", ExitChildExit)
	}
	if ExitContext != 2 {
		t.Errorf("ExitContext = %d, want 2", ExitContext)
	}
	if ExitError != 3 {
		t.Errorf("ExitError = %d, want 3", ExitError)
	}
}
