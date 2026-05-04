package termmux

import (
	"testing"
)

// ── KeyToTermBytes tests ──────────────────────────────────────────

func TestKeyToTermBytes_SpecialKeys(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"enter", "\r"},
		{"tab", "\t"},
		{"shift+tab", "\x1b[Z"},
		{"backspace", "\x7f"},
		{"esc", "\x1b"},
		{"delete", "\x1b[3~"},
		{"up", "\x1b[A"},
		{"down", "\x1b[B"},
		{"right", "\x1b[C"},
		{"left", "\x1b[D"},
		{"home", "\x1b[H"},
		{"end", "\x1b[F"},
		{"pgup", "\x1b[5~"},
		{"pgdown", "\x1b[6~"},
		{"insert", "\x1b[2~"},
		{"f1", "\x1bOP"},
		{"f2", "\x1bOQ"},
		{"f3", "\x1bOR"},
		{"f4", "\x1bOS"},
		{"f5", "\x1b[15~"},
		{"f6", "\x1b[17~"},
		{"f7", "\x1b[18~"},
		{"f8", "\x1b[19~"},
		{"f9", "\x1b[20~"},
		{"f10", "\x1b[21~"},
		{"f11", "\x1b[23~"},
		{"f12", "\x1b[24~"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := KeyToTermBytes(tt.key)
			if !ok {
				t.Fatalf("KeyToTermBytes(%q): expected ok", tt.key)
			}
			if got != tt.want {
				t.Errorf("KeyToTermBytes(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_CtrlLetters(t *testing.T) {
	// ctrl+a → 0x01, ctrl+z → 0x1A
	got, ok := KeyToTermBytes("ctrl+a")
	if !ok || got != "\x01" {
		t.Errorf("ctrl+a: got %q ok=%v", got, ok)
	}
	got, ok = KeyToTermBytes("ctrl+z")
	if !ok || got != "\x1a" {
		t.Errorf("ctrl+z: got %q ok=%v", got, ok)
	}
	got, ok = KeyToTermBytes("ctrl+c")
	if !ok || got != "\x03" {
		t.Errorf("ctrl+c: got %q ok=%v", got, ok)
	}
	// Uppercase should also work.
	got, ok = KeyToTermBytes("ctrl+A")
	if !ok || got != "\x01" {
		t.Errorf("ctrl+A: got %q ok=%v", got, ok)
	}
}

func TestKeyToTermBytes_ModNav(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"shift+up", "\x1b[1;2A"},
		{"shift+down", "\x1b[1;2B"},
		{"shift+right", "\x1b[1;2C"},
		{"shift+left", "\x1b[1;2D"},
		{"shift+home", "\x1b[1;2H"},
		{"shift+end", "\x1b[1;2F"},
		{"ctrl+up", "\x1b[1;5A"},
		{"ctrl+down", "\x1b[1;5B"},
		{"ctrl+right", "\x1b[1;5C"},
		{"ctrl+left", "\x1b[1;5D"},
		{"ctrl+shift+up", "\x1b[1;6A"},
		{"ctrl+shift+right", "\x1b[1;6C"},
		// Tilde-style.
		{"shift+pgup", "\x1b[5;2~"},
		{"shift+pgdown", "\x1b[6;2~"},
		{"ctrl+delete", "\x1b[3;5~"},
		{"shift+insert", "\x1b[2;2~"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := KeyToTermBytes(tt.key)
			if !ok {
				t.Fatalf("KeyToTermBytes(%q): expected ok", tt.key)
			}
			if got != tt.want {
				t.Errorf("KeyToTermBytes(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestKeyToTermBytes_Alt(t *testing.T) {
	// alt+a → ESC + 'a'
	got, ok := KeyToTermBytes("alt+a")
	if !ok || got != "\x1ba" {
		t.Errorf("alt+a: got %q ok=%v", got, ok)
	}
	// alt+up → ESC + ESC[A
	got, ok = KeyToTermBytes("alt+up")
	if !ok || got != "\x1b\x1b[A" {
		t.Errorf("alt+up: got %q ok=%v", got, ok)
	}
}

func TestKeyToTermBytes_Paste(t *testing.T) {
	got, ok := KeyToTermBytes("[hello world]")
	if !ok || got != "hello world" {
		t.Errorf("paste: got %q ok=%v", got, ok)
	}
}

func TestKeyToTermBytes_SingleChar(t *testing.T) {
	got, ok := KeyToTermBytes("a")
	if !ok || got != "a" {
		t.Errorf("single char: got %q ok=%v", got, ok)
	}
	got, ok = KeyToTermBytes(" ")
	if !ok || got != " " {
		t.Errorf("space: got %q ok=%v", got, ok)
	}
}

func TestKeyToTermBytes_Unicode(t *testing.T) {
	got, ok := KeyToTermBytes("日本語")
	if !ok || got != "日本語" {
		t.Errorf("unicode: got %q ok=%v", got, ok)
	}
}

func TestKeyToTermBytes_Unknown(t *testing.T) {
	_, ok := KeyToTermBytes("ctrl+shift+alt+x")
	if ok {
		t.Error("expected !ok for unrecognized combo")
	}
}

// ── MouseToSGR tests ──────────────────────────────────────────────

func TestMouseToSGR_LeftClick(t *testing.T) {
	ev := MouseEvent{Type: MouseClick, Button: MouseLeft, X: 10, Y: 5}
	got, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	// 1-based: cx=11, cy=6
	want := "\x1b[<0;11;6M"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMouseToSGR_RightRelease(t *testing.T) {
	ev := MouseEvent{Type: MouseRelease, Button: MouseRight, X: 3, Y: 7}
	got, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	want := "\x1b[<2;4;8m"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMouseToSGR_WheelUp(t *testing.T) {
	ev := MouseEvent{Type: MouseWheel, Button: MouseWheelUp, X: 0, Y: 0}
	got, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	want := "\x1b[<64;1;1M"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMouseToSGR_Motion(t *testing.T) {
	ev := MouseEvent{Type: MouseMotion, Button: MouseLeft, X: 5, Y: 5}
	got, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	// Motion adds 32 to button code: 0+32=32
	want := "\x1b[<32;6;6M"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMouseToSGR_Modifiers(t *testing.T) {
	ev := MouseEvent{
		Type: MouseClick, Button: MouseLeft,
		X: 1, Y: 1, Shift: true, Alt: true, Ctrl: true,
	}
	got, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("expected ok")
	}
	// 0 + 4(shift) + 8(alt) + 16(ctrl) = 28
	want := "\x1b[<28;2;2M"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMouseToSGR_Offset(t *testing.T) {
	ev := MouseEvent{Type: MouseClick, Button: MouseLeft, X: 15, Y: 20}
	got, ok := MouseToSGR(ev, 10, 5)
	if !ok {
		t.Fatal("expected ok")
	}
	// Offset: x=15-5=10, y=20-10=10 → 1-based: 11,11
	want := "\x1b[<0;11;11M"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMouseToSGR_NegativeCoord(t *testing.T) {
	ev := MouseEvent{Type: MouseClick, Button: MouseLeft, X: 3, Y: 2}
	_, ok := MouseToSGR(ev, 10, 0) // y would be negative
	if ok {
		t.Error("expected !ok for negative coordinate")
	}
}

func TestMouseToSGR_UnknownButton(t *testing.T) {
	ev := MouseEvent{Type: MouseClick, Button: "magic", X: 0, Y: 0}
	_, ok := MouseToSGR(ev, 0, 0)
	if ok {
		t.Error("expected !ok for unknown button")
	}
}

func TestMouseToSGR_AllButtons(t *testing.T) {
	buttons := []struct {
		btn  MouseButton
		code int
	}{
		{MouseLeft, 0},
		{MouseMiddle, 1},
		{MouseRight, 2},
		{MouseWheelUp, 64},
		{MouseWheelDown, 65},
		{MouseWheelLeft, 66},
		{MouseWheelRight, 67},
		{MouseBackward, 128},
		{MouseForward, 129},
		{MouseNone, 3},
	}
	for _, tt := range buttons {
		t.Run(string(tt.btn), func(t *testing.T) {
			ev := MouseEvent{Type: MouseClick, Button: tt.btn, X: 0, Y: 0}
			_, ok := MouseToSGR(ev, 0, 0)
			if !ok {
				t.Errorf("expected ok for button %q", tt.btn)
			}
		})
	}
}

// ── Round-trip: MouseToSGR → parseSGRMouse ────────────────────────

func TestMouseRoundTrip(t *testing.T) {
	ev := MouseEvent{Type: MouseClick, Button: MouseLeft, X: 9, Y: 4}
	encoded, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("encode failed")
	}
	parsed, consumed, pOk := parseSGRMouse([]byte(encoded), 0)
	if !pOk {
		t.Fatal("parse failed")
	}
	if consumed != len(encoded) {
		t.Errorf("consumed=%d, want %d", consumed, len(encoded))
	}
	// 1-based: x=10, y=5
	if parsed.X != 10 || parsed.Y != 5 || parsed.Button != 0 || parsed.Release {
		t.Errorf("round-trip mismatch: %+v", parsed)
	}
}

func TestMouseRoundTrip_Release(t *testing.T) {
	ev := MouseEvent{Type: MouseRelease, Button: MouseRight, X: 20, Y: 15}
	encoded, ok := MouseToSGR(ev, 0, 0)
	if !ok {
		t.Fatal("encode failed")
	}
	parsed, _, pOk := parseSGRMouse([]byte(encoded), 0)
	if !pOk {
		t.Fatal("parse failed")
	}
	if !parsed.Release || parsed.X != 21 || parsed.Y != 16 || parsed.Button != 2 {
		t.Errorf("round-trip release mismatch: %+v", parsed)
	}
}

func TestMouseRoundTrip_WithOffset(t *testing.T) {
	ev := MouseEvent{Type: MouseClick, Button: MouseLeft, X: 15, Y: 20}
	encoded, ok := MouseToSGR(ev, 10, 5)
	if !ok {
		t.Fatal("encode failed")
	}
	parsed, _, pOk := parseSGRMouse([]byte(encoded), 0)
	if !pOk {
		t.Fatal("parse failed")
	}
	// After offset: x=10, y=10 → 1-based: 11, 11
	if parsed.X != 11 || parsed.Y != 11 {
		t.Errorf("round-trip offset: X=%d Y=%d, want 11,11", parsed.X, parsed.Y)
	}
}
