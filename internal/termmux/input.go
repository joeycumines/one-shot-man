package termmux

import (
	"strconv"
	"strings"
)

// KeyToTermBytes converts a BubbleTea-style key name to terminal byte
// sequence for PTY forwarding. Key names match BubbleTea's KeyMsg.String()
// output (keys_gen.go). Returns the byte sequence and true if the key was
// recognized, or an empty string and false otherwise.
func KeyToTermBytes(key string) (string, bool) {
	// Named special keys → terminal escape sequences.
	switch key {
	case "enter":
		return "\r", true
	case "tab":
		return "\t", true
	case "shift+tab":
		return "\x1b[Z", true
	case "backspace":
		return "\x7f", true
	case "esc":
		return "\x1b", true
	case "delete":
		return "\x1b[3~", true
	case "up":
		return "\x1b[A", true
	case "down":
		return "\x1b[B", true
	case "right":
		return "\x1b[C", true
	case "left":
		return "\x1b[D", true
	case "home":
		return "\x1b[H", true
	case "end":
		return "\x1b[F", true
	case "pgup":
		return "\x1b[5~", true
	case "pgdown":
		return "\x1b[6~", true
	case "insert":
		return "\x1b[2~", true
	case "f1":
		return "\x1bOP", true
	case "f2":
		return "\x1bOQ", true
	case "f3":
		return "\x1bOR", true
	case "f4":
		return "\x1bOS", true
	case "f5":
		return "\x1b[15~", true
	case "f6":
		return "\x1b[17~", true
	case "f7":
		return "\x1b[18~", true
	case "f8":
		return "\x1b[19~", true
	case "f9":
		return "\x1b[20~", true
	case "f10":
		return "\x1b[21~", true
	case "f11":
		return "\x1b[23~", true
	case "f12":
		return "\x1b[24~", true
	}

	// Ctrl+letter → control character (0x01–0x1A for a-z).
	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok && len(rest) == 1 {
		ch := rest[0]
		if ch >= 'a' && ch <= 'z' {
			return string(rune(ch - 'a' + 1)), true
		}
		if ch >= 'A' && ch <= 'Z' {
			return string(rune(ch - 'A' + 1)), true
		}
	}

	// Modifier+navigation keys → xterm CSI sequences.
	if s, ok := encodeModNav(key); ok {
		return s, true
	}

	// Alt+key → ESC prefix + inner key bytes.
	if rest, ok := strings.CutPrefix(key, "alt+"); ok {
		if inner, ok := KeyToTermBytes(rest); ok {
			return "\x1b" + inner, true
		}
	}

	// Bracketed paste: "[content]" → content.
	if len(key) > 2 && key[0] == '[' && key[len(key)-1] == ']' {
		return key[1 : len(key)-1], true
	}

	// Single printable character → send as-is.
	if len(key) == 1 {
		return key, true
	}

	// Multi-character unknown keys (e.g., Unicode) → send as-is.
	if len(key) > 1 && !strings.Contains(key, "+") {
		return key, true
	}

	return "", false
}

// encodeModNav handles modifier+navigation key combinations:
// Format: ESC[1;{mod}{letter} or ESC[{num};{mod}~ for tilde-style keys.
func encodeModNav(key string) (string, bool) {
	type modPrefix struct {
		prefix string
		mod    string
	}
	prefixes := []modPrefix{
		{"ctrl+shift+", "6"},
		{"shift+", "2"},
		{"ctrl+", "5"},
	}
	navMap := map[string]string{
		"up": "A", "down": "B", "right": "C", "left": "D",
		"home": "H", "end": "F",
	}
	tildeMap := map[string]string{
		"pgup": "5", "pgdown": "6", "delete": "3", "insert": "2",
	}

	for _, mp := range prefixes {
		rest, ok := strings.CutPrefix(key, mp.prefix)
		if !ok {
			continue
		}
		if letter, exists := navMap[rest]; exists {
			return "\x1b[1;" + mp.mod + letter, true
		}
		if num, exists := tildeMap[rest]; exists {
			return "\x1b[" + num + ";" + mp.mod + "~", true
		}
	}
	return "", false
}

// MouseButton identifies a mouse button for SGR encoding.
type MouseButton string

// Mouse button constants matching BubbleTea's button naming.
const (
	MouseLeft       MouseButton = "left"
	MouseMiddle     MouseButton = "middle"
	MouseRight      MouseButton = "right"
	MouseWheelUp    MouseButton = "wheel up"
	MouseWheelDown  MouseButton = "wheel down"
	MouseWheelLeft  MouseButton = "wheel left"
	MouseWheelRight MouseButton = "wheel right"
	MouseBackward   MouseButton = "backward"
	MouseForward    MouseButton = "forward"
	MouseNone       MouseButton = "none"
)

// MouseEventType identifies the type of mouse event.
type MouseEventType string

// Mouse event type constants matching BubbleTea's message types.
const (
	MouseClick   MouseEventType = "MouseClick"
	MouseRelease MouseEventType = "MouseRelease"
	MouseMotion  MouseEventType = "MouseMotion"
	MouseWheel   MouseEventType = "MouseWheel"
)

// MouseEvent holds the fields needed to encode a mouse event as SGR bytes.
type MouseEvent struct {
	Type   MouseEventType
	Button MouseButton
	X      int // 0-based column in screen coordinates
	Y      int // 0-based row in screen coordinates
	Shift  bool
	Alt    bool
	Ctrl   bool
}

// MouseToSGR converts a mouse event to an SGR mouse escape sequence,
// applying the given coordinate offsets to transform screen coordinates
// to pane-local coordinates. Returns the escape sequence and true if
// the event was recognized, or an empty string and false otherwise.
//
// The offset parameters subtract from the screen coordinates so that
// (0,0) maps to the pane origin. Negative resulting coordinates cause
// a false return since the event is outside the pane.
func MouseToSGR(ev MouseEvent, offsetRow, offsetCol int) (string, bool) {
	x := ev.X - offsetCol
	y := ev.Y - offsetRow
	if x < 0 || y < 0 {
		return "", false
	}

	btn, ok := mouseButtonCode(ev.Button)
	if !ok {
		return "", false
	}

	if ev.Shift {
		btn += 4
	}
	if ev.Alt {
		btn += 8
	}
	if ev.Ctrl {
		btn += 16
	}
	if ev.Type == MouseMotion {
		btn += 32
	}

	// SGR uses 1-based coordinates.
	cx := x + 1
	cy := y + 1

	suffix := "M"
	if ev.Type == MouseRelease {
		suffix = "m"
	}

	return "\x1b[<" + strconv.Itoa(btn) + ";" + strconv.Itoa(cx) + ";" + strconv.Itoa(cy) + suffix, true
}

func mouseButtonCode(b MouseButton) (int, bool) {
	switch b {
	case MouseLeft:
		return 0, true
	case MouseMiddle:
		return 1, true
	case MouseRight:
		return 2, true
	case MouseWheelUp:
		return 64, true
	case MouseWheelDown:
		return 65, true
	case MouseWheelLeft:
		return 66, true
	case MouseWheelRight:
		return 67, true
	case MouseBackward:
		return 128, true
	case MouseForward:
		return 129, true
	case MouseNone:
		return 3, true
	default:
		return 0, false
	}
}
