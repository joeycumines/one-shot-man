package termmux

import (
	"testing"
)

func TestPrefixKeyHandler_DefaultPrefix(t *testing.T) {
	h := NewPrefixKeyHandler("")
	if h.Prefix() != "ctrl+b" {
		t.Errorf("Prefix() = %q, want %q", h.Prefix(), "ctrl+b")
	}
}

func TestPrefixKeyHandler_CustomPrefix(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+a")
	if h.Prefix() != "ctrl+a" {
		t.Errorf("Prefix() = %q, want %q", h.Prefix(), "ctrl+a")
	}
}

func TestPrefixKeyHandler_NormalKeyPassesThrough(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	handled, action := h.HandleKey("a")
	if handled {
		t.Errorf("HandleKey('a') handled=true, want false (normal key should pass through)")
	}
	if action.Kind != PrefixActionNone {
		t.Errorf("action = %v, want None", action)
	}
}

func TestPrefixKeyHandler_PrefixActivates(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	handled, action := h.HandleKey("ctrl+b")
	if !handled {
		t.Error("HandleKey('ctrl+b') handled=false, want true")
	}
	if !h.Awaiting() {
		t.Error("Awaiting=false after prefix, want true")
	}
	if action.Kind != PrefixActionNone {
		t.Errorf("action = %v, want None (awaiting command)", action)
	}
}

func TestPrefixKeyHandler_PrefixThenCommand(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.HandleKey("ctrl+b")
	handled, action := h.HandleKey("c")

	if !handled {
		t.Error("HandleKey('c') after prefix handled=false, want true")
	}
	if action.Kind != PrefixActionNewWindow {
		t.Errorf("action = %v, want NewWindow", action)
	}
	if h.Awaiting() {
		t.Error("Awaiting=true after command dispatch, want false")
	}
}

func TestPrefixKeyHandler_PrefixThenEsc(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.HandleKey("ctrl+b")
	handled, action := h.HandleKey("esc")

	if !handled {
		t.Error("HandleKey('esc') after prefix handled=false, want true")
	}
	if action.Kind != PrefixActionCancel {
		t.Errorf("action = %v, want Cancel", action)
	}
}

func TestPrefixKeyHandler_PrefixThenUnknownKey(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.HandleKey("ctrl+b")
	handled, action := h.HandleKey("q")

	if !handled {
		t.Error("unknown key after prefix should be handled (consumed)")
	}
	if action.Kind != PrefixActionNone {
		t.Errorf("action = %v, want None for unknown key", action)
	}
}

func TestPrefixKeyHandler_AllDefaultCommands(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	cases := map[string]PrefixActionKind{
		"c":  PrefixActionNewWindow,
		"n":  PrefixActionNextWindow,
		"p":  PrefixActionPrevWindow,
		"d":  PrefixActionDetach,
		"z":  PrefixActionZoomPane,
		"x":  PrefixActionClosePane,
		"%":  PrefixActionSplitHorizontal,
		"\"": PrefixActionSplitVertical,
		"[":  PrefixActionCopyMode,
		"?":  PrefixActionListKeys,
		",":  PrefixActionRenameWindow,
	}

	for key, wantKind := range cases {
		h.Reset()
		h.HandleKey("ctrl+b")
		handled, action := h.HandleKey(key)
		if !handled {
			t.Errorf("key %q after prefix not handled", key)
		}
		if action.Kind != wantKind {
			t.Errorf("key %q: action = %v, want %v", key, action.Kind, wantKind)
		}
	}
}

func TestPrefixKeyHandler_SetCommand(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.SetCommand("r", PrefixActionRenameWindow)
	h.HandleKey("ctrl+b")
	handled, action := h.HandleKey("r")

	if !handled {
		t.Error("custom command key not handled")
	}
	if action.Kind != PrefixActionRenameWindow {
		t.Errorf("action = %v, want RenameWindow", action)
	}
}

func TestPrefixKeyHandler_RemoveCommand(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.RemoveCommand("c")
	h.HandleKey("ctrl+b")
	handled, action := h.HandleKey("c")

	if !handled {
		t.Error("removed command key should still be consumed")
	}
	if action.Kind != PrefixActionNone {
		t.Errorf("action = %v, want None for removed command", action)
	}
}

func TestPrefixKeyHandler_Reset(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.HandleKey("ctrl+b")
	if !h.Awaiting() {
		t.Error("Awaiting=false, want true after prefix")
	}

	h.Reset()
	if h.Awaiting() {
		t.Error("Awaiting=true after Reset, want false")
	}

	handled, action := h.HandleKey("a")
	if handled {
		t.Error("key after Reset should pass through")
	}
	if action.Kind != PrefixActionNone {
		t.Errorf("action = %v, want None", action)
	}
}

func TestPrefixKeyHandler_SetPrefix(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.SetPrefix("ctrl+a")
	if h.Prefix() != "ctrl+a" {
		t.Errorf("Prefix() = %q, want %q", h.Prefix(), "ctrl+a")
	}

	handled, _ := h.HandleKey("ctrl+a")
	if !handled {
		t.Error("new prefix key not handled")
	}
	if !h.Awaiting() {
		t.Error("Awaiting=false after new prefix, want true")
	}
}

func TestPrefixKeyHandler_Commands(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	cmds := h.Commands()
	if len(cmds) < 10 {
		t.Errorf("len(Commands) = %d, want >= 10", len(cmds))
	}

	cmds["c"] = PrefixActionDetach

	if h.Commands()["c"] != PrefixActionNewWindow {
		t.Error("modifying returned map should not affect handler")
	}
}

func TestPrefixKeyHandler_PrefixThenPrefix(t *testing.T) {
	h := NewPrefixKeyHandler("ctrl+b")

	h.HandleKey("ctrl+b")
	handled, action := h.HandleKey("ctrl+b")

	if !handled {
		t.Error("double prefix should be handled")
	}
	if action.Kind != PrefixActionCancel {
		t.Errorf("double prefix action = %v, want Cancel", action)
	}
}
