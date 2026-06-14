package termmux

import (
	"testing"
)

func TestCopyModeKeyHandler_MoveKeys(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	tests := []struct {
		key      string
		wantKind CopyModeActionKind
		wantN    int
	}{
		{"h", CopyModeActionMoveLeft, 1},
		{"l", CopyModeActionMoveRight, 1},
		{"j", CopyModeActionMoveDown, 1},
		{"k", CopyModeActionMoveUp, 1},
		{"left", CopyModeActionMoveLeft, 1},
		{"right", CopyModeActionMoveRight, 1},
		{"down", CopyModeActionMoveDown, 1},
		{"up", CopyModeActionMoveUp, 1},
	}

	for _, tc := range tests {
		a := h.HandleKey(tc.key)
		if a.Kind != tc.wantKind {
			t.Errorf("HandleKey(%q) kind = %v, want %v", tc.key, a.Kind, tc.wantKind)
		}
		if a.N != tc.wantN {
			t.Errorf("HandleKey(%q) N = %d, want %d", tc.key, a.N, tc.wantN)
		}
	}
}

func TestCopyModeKeyHandler_HalfPage(t *testing.T) {
	h := NewCopyModeKeyHandler(10)

	a := h.HandleKey("ctrl+u")
	if a.Kind != CopyModeActionHalfPageUp || a.N != 10 {
		t.Errorf("ctrl+u = %v, want HalfPageUp(10)", a)
	}

	a = h.HandleKey("ctrl+d")
	if a.Kind != CopyModeActionHalfPageDown || a.N != 10 {
		t.Errorf("ctrl+d = %v, want HalfPageDown(10)", a)
	}
}

func TestCopyModeKeyHandler_DefaultHalfPage(t *testing.T) {
	h := NewCopyModeKeyHandler(0)
	if h.halfPageRows != 12 {
		t.Errorf("default halfPageRows = %d, want 12", h.halfPageRows)
	}
}

func TestCopyModeKeyHandler_ScrollPositions(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("g")
	if a.Kind != CopyModeActionTopOfScrollback {
		t.Errorf("g = %v, want TopOfScrollback", a.Kind)
	}

	a = h.HandleKey("G")
	if a.Kind != CopyModeActionBottomOfScrollback {
		t.Errorf("G = %v, want BottomOfScrollback", a.Kind)
	}
}

func TestCopyModeKeyHandler_LineEnds(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("0")
	if a.Kind != CopyModeActionBeginningOfLine {
		t.Errorf("0 = %v, want BeginningOfLine", a.Kind)
	}

	a = h.HandleKey("$")
	if a.Kind != CopyModeActionEndOfLine {
		t.Errorf("$ = %v, want EndOfLine", a.Kind)
	}
}

func TestCopyModeKeyHandler_Words(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("w")
	if a.Kind != CopyModeActionNextWord || a.N != 1 {
		t.Errorf("w = %v, want NextWord(1)", a)
	}

	a = h.HandleKey("b")
	if a.Kind != CopyModeActionPrevWord || a.N != 1 {
		t.Errorf("b = %v, want PrevWord(1)", a)
	}
}

func TestCopyModeKeyHandler_ExitCopyMode(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("q")
	if a.Kind != CopyModeActionExitCopyMode {
		t.Errorf("q = %v, want ExitCopyMode", a.Kind)
	}

	a = h.HandleKey("esc")
	if a.Kind != CopyModeActionExitCopyMode {
		t.Errorf("esc = %v, want ExitCopyMode", a.Kind)
	}
}

func TestCopyModeKeyHandler_SelectAndCopy(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey(" ")
	if a.Kind != CopyModeActionSelectStart {
		t.Errorf("space = %v, want SelectStart", a.Kind)
	}

	a = h.HandleKey("enter")
	if a.Kind != CopyModeActionCopyAndExit {
		t.Errorf("enter = %v, want CopyAndExit", a.Kind)
	}
}

func TestCopyModeKeyHandler_ScrollPages(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("ctrl+b")
	if a.Kind != CopyModeActionScrollUpOne || a.N != 1 {
		t.Errorf("ctrl+b = %v, want ScrollUp(1)", a)
	}

	a = h.HandleKey("ctrl+f")
	if a.Kind != CopyModeActionScrollDownOne || a.N != 1 {
		t.Errorf("ctrl+f = %v, want ScrollDown(1)", a)
	}
}

func TestCopyModeKeyHandler_UnknownKey(t *testing.T) {
	h := NewCopyModeKeyHandler(12)

	a := h.HandleKey("x")
	if a.Kind != CopyModeActionNone {
		t.Errorf("unknown key = %v, want None", a.Kind)
	}

	a = h.HandleKey("ctrl+a")
	if a.Kind != CopyModeActionNone {
		t.Errorf("unknown ctrl key = %v, want None", a.Kind)
	}
}

func TestCopyModeKeyHandler_ActionString(t *testing.T) {
	tests := []struct {
		action CopyModeAction
		want   string
	}{
		{CopyModeAction{Kind: CopyModeActionMoveDown, N: 1}, "MoveDown(1)"},
		{CopyModeAction{Kind: CopyModeActionHalfPageUp, N: 12}, "HalfPageUp(12)"},
		{CopyModeAction{Kind: CopyModeActionTopOfScrollback}, "TopOfScrollback"},
		{CopyModeAction{Kind: CopyModeActionExitCopyMode}, "ExitCopyMode"},
		{CopyModeAction{Kind: CopyModeActionSelectStart}, "SelectStart"},
		{CopyModeAction{Kind: CopyModeActionCopyAndExit}, "CopyAndExit"},
		{CopyModeAction{Kind: CopyModeActionNone}, "None"},
	}

	for _, tc := range tests {
		got := tc.action.String()
		if got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

func TestCopyModeKeyHandler_AllViKeys(t *testing.T) {
	h := NewCopyModeKeyHandler(20)

	keys := map[string]CopyModeActionKind{
		"h":       CopyModeActionMoveLeft,
		"l":       CopyModeActionMoveRight,
		"j":       CopyModeActionMoveDown,
		"k":       CopyModeActionMoveUp,
		"ctrl+u":  CopyModeActionHalfPageUp,
		"ctrl+d":  CopyModeActionHalfPageDown,
		"g":       CopyModeActionTopOfScrollback,
		"G":       CopyModeActionBottomOfScrollback,
		"0":       CopyModeActionBeginningOfLine,
		"$":       CopyModeActionEndOfLine,
		"w":       CopyModeActionNextWord,
		"b":       CopyModeActionPrevWord,
		"q":       CopyModeActionExitCopyMode,
		" ":       CopyModeActionSelectStart,
		"enter":   CopyModeActionCopyAndExit,
		"ctrl+b":  CopyModeActionScrollUpOne,
		"ctrl+f":  CopyModeActionScrollDownOne,
	}

	for key, wantKind := range keys {
		a := h.HandleKey(key)
		if a.Kind != wantKind {
			t.Errorf("HandleKey(%q) = %v, want %v", key, a.Kind, wantKind)
		}
	}
}
