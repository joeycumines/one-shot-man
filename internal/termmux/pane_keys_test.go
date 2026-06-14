package termmux

import (
	"bytes"
	"testing"
)

func withPanes(has bool) *PaneKeyHandler {
	return &PaneKeyHandler{
		HasPanes: func() bool { return has },
	}
}

func TestPaneKeyHandler_HandleKey_CtrlH(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x08})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneFocusLeft {
		t.Errorf("action: got %v, want PaneFocusLeft", action)
	}
}

func TestPaneKeyHandler_HandleKey_CtrlL(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x0C})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneFocusRight {
		t.Errorf("action: got %v, want PaneFocusRight", action)
	}
}

func TestPaneKeyHandler_HandleKey_CtrlJ(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x0A})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneFocusDown {
		t.Errorf("action: got %v, want PaneFocusDown", action)
	}
}

func TestPaneKeyHandler_HandleKey_CtrlK(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x0B})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneFocusUp {
		t.Errorf("action: got %v, want PaneFocusUp", action)
	}
}

func TestPaneKeyHandler_HandleKey_AltH(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x1B, 'h'})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneSplitH {
		t.Errorf("action: got %v, want PaneSplitH", action)
	}
}

func TestPaneKeyHandler_HandleKey_AltV(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x1B, 'v'})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneSplitV {
		t.Errorf("action: got %v, want PaneSplitV", action)
	}
}

func TestPaneKeyHandler_HandleKey_CtrlX(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x18})
	if !consumed {
		t.Error("expected consumed=true")
	}
	if action != PaneClose {
		t.Errorf("action: got %v, want PaneClose", action)
	}
}

func TestPaneKeyHandler_HandleKey_NoPanes(t *testing.T) {
	h := withPanes(false)
	for _, input := range [][]byte{
		{0x08},
		{0x0C},
		{0x0A},
		{0x0B},
		{0x1B, 'h'},
		{0x1B, 'v'},
		{0x18},
	} {
		consumed, action := h.HandleKey(input)
		if consumed {
			t.Errorf("input=%x: expected consumed=false when no panes", input)
		}
		if action != PaneActionNone {
			t.Errorf("input=%x: action: got %v, want PaneActionNone", input, action)
		}
	}
}

func TestPaneKeyHandler_HandleKey_NilHasPanes(t *testing.T) {
	h := &PaneKeyHandler{}
	consumed, action := h.HandleKey([]byte{0x08})
	if consumed {
		t.Error("expected consumed=false with nil HasPanes")
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
}

func TestPaneKeyHandler_HandleKey_EmptyData(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{})
	if consumed {
		t.Error("expected consumed=false for empty data")
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
}

func TestPaneKeyHandler_HandleKey_RegularKey(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{'a'})
	if consumed {
		t.Error("expected consumed=false for regular key")
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
}

func TestPaneKeyHandler_HandleKey_AltOtherKey(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x1B, 'x'})
	if consumed {
		t.Error("expected consumed=false for Alt+X (not a pane key)")
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
}

func TestPaneKeyHandler_HandleKey_EscOnly(t *testing.T) {
	h := withPanes(true)
	consumed, action := h.HandleKey([]byte{0x1B})
	if consumed {
		t.Error("expected consumed=false for lone ESC")
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_CtrlH(t *testing.T) {
	h := withPanes(true)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{0x08, 'x', 'y'})
	if prefixLen != 1 {
		t.Errorf("prefixLen: got %d, want 1", prefixLen)
	}
	if action != PaneFocusLeft {
		t.Errorf("action: got %v, want PaneFocusLeft", action)
	}
	if !bytes.Equal(remaining, []byte{'x', 'y'}) {
		t.Errorf("remaining: got %v, want [x y]", remaining)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_AltH(t *testing.T) {
	h := withPanes(true)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{0x1B, 'h', 'z'})
	if prefixLen != 2 {
		t.Errorf("prefixLen: got %d, want 2", prefixLen)
	}
	if action != PaneSplitH {
		t.Errorf("action: got %v, want PaneSplitH", action)
	}
	if !bytes.Equal(remaining, []byte{'z'}) {
		t.Errorf("remaining: got %v, want [z]", remaining)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_AltV(t *testing.T) {
	h := withPanes(true)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{0x1B, 'v', 'a', 'b'})
	if prefixLen != 2 {
		t.Errorf("prefixLen: got %d, want 2", prefixLen)
	}
	if action != PaneSplitV {
		t.Errorf("action: got %v, want PaneSplitV", action)
	}
	if !bytes.Equal(remaining, []byte{'a', 'b'}) {
		t.Errorf("remaining: got %v, want [a b]", remaining)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_NoMatch(t *testing.T) {
	h := withPanes(true)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{'h', 'e', 'l', 'l', 'o'})
	if prefixLen != 0 {
		t.Errorf("prefixLen: got %d, want 0", prefixLen)
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
	if !bytes.Equal(remaining, []byte{'h', 'e', 'l', 'l', 'o'}) {
		t.Errorf("remaining: got %v, want original data", remaining)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_NoPanes(t *testing.T) {
	h := withPanes(false)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{0x08, 'x'})
	if prefixLen != 0 {
		t.Errorf("prefixLen: got %d, want 0 (no panes)", prefixLen)
	}
	if action != PaneActionNone {
		t.Errorf("action: got %v, want PaneActionNone", action)
	}
	if !bytes.Equal(remaining, []byte{0x08, 'x'}) {
		t.Errorf("remaining: got %v, want original data", remaining)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_ExactKey(t *testing.T) {
	h := withPanes(true)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{0x18})
	if prefixLen != 1 {
		t.Errorf("prefixLen: got %d, want 1", prefixLen)
	}
	if action != PaneClose {
		t.Errorf("action: got %v, want PaneClose", action)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining: got %v, want empty", remaining)
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_AltHExact(t *testing.T) {
	h := withPanes(true)
	prefixLen, action, remaining := h.HandleKeyInBuffer([]byte{0x1B, 'h'})
	if prefixLen != 2 {
		t.Errorf("prefixLen: got %d, want 2", prefixLen)
	}
	if action != PaneSplitH {
		t.Errorf("action: got %v, want PaneSplitH", action)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining: got %v, want empty", remaining)
	}
}

func TestPaneKeyHandler_AllActions(t *testing.T) {
	tests := []struct {
		input  []byte
		action PaneAction
	}{
		{[]byte{0x08}, PaneFocusLeft},
		{[]byte{0x0C}, PaneFocusRight},
		{[]byte{0x0A}, PaneFocusDown},
		{[]byte{0x0B}, PaneFocusUp},
		{[]byte{0x1B, 'h'}, PaneSplitH},
		{[]byte{0x1B, 'v'}, PaneSplitV},
		{[]byte{0x18}, PaneClose},
	}
	for _, tt := range tests {
		h := withPanes(true)
		consumed, action := h.HandleKey(tt.input)
		if !consumed {
			t.Errorf("input=%x: expected consumed=true", tt.input)
		}
		if action != tt.action {
			t.Errorf("input=%x: action: got %v, want %v", tt.input, action, tt.action)
		}
	}
}

func TestPaneKeyHandler_HandleKeyInBuffer_AllActionsWithTrailing(t *testing.T) {
	tests := []struct {
		input    []byte
		action   PaneAction
		consumed int
	}{
		{[]byte{0x08, 'Z'}, PaneFocusLeft, 1},
		{[]byte{0x0C, 'Z'}, PaneFocusRight, 1},
		{[]byte{0x0A, 'Z'}, PaneFocusDown, 1},
		{[]byte{0x0B, 'Z'}, PaneFocusUp, 1},
		{[]byte{0x1B, 'h', 'Z'}, PaneSplitH, 2},
		{[]byte{0x1B, 'v', 'Z'}, PaneSplitV, 2},
		{[]byte{0x18, 'Z'}, PaneClose, 1},
	}
	for _, tt := range tests {
		h := withPanes(true)
		prefixLen, action, remaining := h.HandleKeyInBuffer(tt.input)
		if prefixLen != tt.consumed {
			t.Errorf("input=%x: prefixLen: got %d, want %d", tt.input, prefixLen, tt.consumed)
		}
		if action != tt.action {
			t.Errorf("input=%x: action: got %v, want %v", tt.input, action, tt.action)
		}
		if !bytes.Equal(remaining, []byte{'Z'}) {
			t.Errorf("input=%x: remaining: got %v, want [Z]", tt.input, remaining)
		}
	}
}
