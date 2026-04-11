package termmux

import "testing"

func TestDefaultToggleKeyName_isCtrlRBracket(t *testing.T) {
	if DefaultToggleKeyName != "Ctrl+]" {
		t.Errorf("DefaultToggleKeyName = %q, want %q", DefaultToggleKeyName, "Ctrl+]")
	}
}
