package termmux

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.ToggleKey != DefaultToggleKey {
		t.Errorf("ToggleKey = 0x%02X, want 0x%02X", cfg.ToggleKey, DefaultToggleKey)
	}
	if cfg.ToggleKey != 0x1D {
		t.Errorf("ToggleKey = 0x%02X, want 0x1D", cfg.ToggleKey)
	}
	if !cfg.StatusEnabled {
		t.Error("StatusEnabled = false, want true")
	}
	if cfg.InitialStatus != "idle" {
		t.Errorf("InitialStatus = %q, want %q", cfg.InitialStatus, "idle")
	}
	if cfg.ResizeFn != nil {
		t.Error("ResizeFn = non-nil, want nil")
	}
}

func TestApplyOptions_noOps(t *testing.T) {
	cfg := defaultConfig()
	applyOptions(&cfg, nil)
	if cfg.ToggleKey != DefaultToggleKey {
		t.Errorf("ToggleKey changed after empty options")
	}
	if !cfg.StatusEnabled {
		t.Error("StatusEnabled changed after empty options")
	}
}

func TestDefaultToggleKeyName_isCtrlRBracket(t *testing.T) {
	if DefaultToggleKeyName != "Ctrl+]" {
		t.Errorf("DefaultToggleKeyName = %q, want %q", DefaultToggleKeyName, "Ctrl+]")
	}
}
