package ui

import (
	"strings"
	"testing"
)

// ── renderHelpBar (unexported, directly tested) ────────────────────

func TestRenderHelpBar_Default_ShowsAllKeys(t *testing.T) {
	m := NewAutoSplitModel()
	// Default state: not done, not cancelled
	out := m.renderHelpBar(80)
	// Should contain cancel, claude, scroll, jump key hints
	for _, want := range []string{"q", "ctrl+]", "claude", "scroll", "cancel"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in help bar: %q", want, out)
		}
	}
}

func TestRenderHelpBar_Done_ShowsDismiss(t *testing.T) {
	m := NewAutoSplitModel()
	m.done = true
	out := m.renderHelpBar(80)
	if !strings.Contains(out, "dismiss") {
		t.Errorf("done state: missing 'dismiss' in %q", out)
	}
	if !strings.Contains(out, "q") {
		t.Errorf("done state: missing 'q' in %q", out)
	}
	if !strings.Contains(out, "enter") {
		t.Errorf("done state: missing 'enter' in %q", out)
	}
}

func TestRenderHelpBar_Cancelled_ShowsForceKill(t *testing.T) {
	m := NewAutoSplitModel()
	m.cancelled = true
	out := m.renderHelpBar(80)
	if !strings.Contains(out, "force kill") {
		t.Errorf("cancelled state: missing 'force kill' in %q", out)
	}
}

func TestRenderHelpBar_ForceCancelled_ShowsKillingMessage(t *testing.T) {
	m := NewAutoSplitModel()
	m.cancelled = true
	m.forceCancel = true
	out := m.renderHelpBar(80)
	if !strings.Contains(out, "force killing") {
		t.Errorf("force-cancelled state: missing 'force killing' in %q", out)
	}
}

func TestRenderHelpBar_NarrowWidth_NoPanic(t *testing.T) {
	m := NewAutoSplitModel()
	// Width smaller than the help text — should not panic.
	out := m.renderHelpBar(10)
	if out == "" {
		t.Error("narrow width: expected non-empty output")
	}
}

func TestRenderHelpBar_ZeroWidth_NoPanic(t *testing.T) {
	m := NewAutoSplitModel()
	// Width 0 — edge case, should not panic.
	_ = m.renderHelpBar(0)
	// Just verifying no panic.
}

// ── tickCmd (unexported, directly tested) ──────────────────────────

func TestTickCmd_ReturnsNonNil(t *testing.T) {
	cmd := tickCmd()
	if cmd == nil {
		t.Error("tickCmd() returned nil")
	}
}
