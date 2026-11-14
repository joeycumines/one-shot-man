package termmux

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// KeyToTermBytes converts a BubbleTea-style key name to terminal byte
// sequence for PTY forwarding. Key names match BubbleTea's KeyMsg.String()
// output (keys_gen.go). When appCursor is true, arrow keys and home/end
// use application cursor mode (SS3 sequences: ESC O{A-D/H/F instead of
// CSI A/B/C/D/H/F). When appKeypad is true, keypad keys use application
// mode (SS3 sequences: ESC O p–y for digits, ESC O M for enter, etc.)
// instead of their ASCII equivalents. Returns the byte sequence and true
// if the key was recognized, or an empty string and false otherwise.
func KeyToTermBytes(key string, appCursor bool, appKeypad bool) (string, bool) {
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
	case "space":
		return " ", true
	case "esc":
		return "\x1b", true
	case "delete":
		return "\x1b[3~", true
	case "up":
		if appCursor {
			return "\x1bOA", true
		}
		return "\x1b[A", true
	case "down":
		if appCursor {
			return "\x1bOB", true
		}
		return "\x1b[B", true
	case "right":
		if appCursor {
			return "\x1bOC", true
		}
		return "\x1b[C", true
	case "left":
		if appCursor {
			return "\x1bOD", true
		}
		return "\x1b[D", true
	case "home":
		if appCursor {
			return "\x1bOH", true
		}
		return "\x1b[H", true
	case "end":
		if appCursor {
			return "\x1bOF", true
		}
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

	// Keypad keys: application mode sends SS3 sequences, normal mode sends ASCII.
	switch key {
	case "kp0":
		if appKeypad {
			return "\x1bOp", true
		}
		return "0", true
	case "kp1":
		if appKeypad {
			return "\x1bOq", true
		}
		return "1", true
	case "kp2":
		if appKeypad {
			return "\x1bOr", true
		}
		return "2", true
	case "kp3":
		if appKeypad {
			return "\x1bOs", true
		}
		return "3", true
	case "kp4":
		if appKeypad {
			return "\x1bOt", true
		}
		return "4", true
	case "kp5":
		if appKeypad {
			return "\x1bOu", true
		}
		return "5", true
	case "kp6":
		if appKeypad {
			return "\x1bOv", true
		}
		return "6", true
	case "kp7":
		if appKeypad {
			return "\x1bOw", true
		}
		return "7", true
	case "kp8":
		if appKeypad {
			return "\x1bOx", true
		}
		return "8", true
	case "kp9":
		if appKeypad {
			return "\x1bOy", true
		}
		return "9", true
	case "kp_enter":
		if appKeypad {
			return "\x1bOM", true
		}
		return "\r", true
	case "kp_plus":
		if appKeypad {
			return "\x1bOk", true
		}
		return "+", true
	case "kp_minus":
		if appKeypad {
			return "\x1bOm", true
		}
		return "-", true
	case "kp_asterisk", "kp_star":
		if appKeypad {
			return "\x1bOj", true
		}
		return "*", true
	case "kp_slash":
		if appKeypad {
			return "\x1bOo", true
		}
		return "/", true
	case "kp_dot", "kp_period":
		if appKeypad {
			return "\x1bOn", true
		}
		return ".", true
	case "kp_comma":
		if appKeypad {
			return "\x1bOl", true
		}
		return ",", true
	case "kp_equal":
		if appKeypad {
			return "\x1bOX", true
		}
		return "=", true
	}

	// Ctrl+letter → control character (0x01–0x1A for a-z).
	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok && len(rest) == 1 {
		if b, ok := ctrlLetter(rest[0]); ok {
			return string(rune(b)), true
		}
	}

	// Modifier+navigation/tilde/function keys → xterm CSI sequences
	// (appCursor only affects UNMODIFIED keys, handled above). This accepts
	// modifiers in any order, including BubbleTea's canonical
	// "ctrl+alt+shift+meta+...".
	if s, ok := encodeModNav(key); ok {
		return s, true
	}

	// Ctrl+Alt+<single letter> (BubbleTea canonical order "ctrl+alt+a"):
	// xterm encodes this as ESC + the control byte (C-M-a = ESC 0x01).
	// Strip any combination of ctrl/alt modifiers (in any order) and check
	// for a single-letter stem.
	if rest, ok := stripCtrlAlt(key); ok && len(rest) == 1 {
		if b, ok := ctrlLetter(rest[0]); ok {
			return "\x1b" + string(rune(b)), true
		}
	}

	// Alt+key → ESC prefix + inner key bytes, except for keys that have
	// their own xterm modifier encoding (handled above), so alt+left emits
	// ESC[1;3D rather than a double-ESC sequence. A duplicated alt modifier
	// ("alt+alt+...") is malformed and must not produce a double ESC.
	if rest, ok := strings.CutPrefix(key, "alt+"); ok {
		if strings.HasPrefix(rest, "alt+") {
			return "", false
		}
		if inner, ok := KeyToTermBytes(rest, appCursor, appKeypad); ok {
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

	// Multi-character unknown keys containing non-ASCII runes (e.g., Unicode
	// text) → send as-is. ASCII-only multi-character strings that didn't match
	// any known key name are unrecognized → return false so the caller can
	// fall back to msg.text or other handling.
	if len(key) > 1 && !strings.Contains(key, "+") {
		for _, r := range key {
			if r > 0x7F {
				return key, true
			}
		}
		return "", false
	}

	return "", false
}

// encodeModNav handles modifier+navigation key combinations using the xterm
// modifier parameter: ESC[1;{mod}{letter} for cursor-style keys, ESC[{num};{mod}~ for
// tilde-style keys, and ESC[1;{mod}{P-S} for f1-f4, where mod = 1 + bitmask
// with shift=1, alt=2, ctrl=4, meta=8 (see xterm ctlseqs).
//
// Modified cursor/home/end keys ALWAYS use the CSI form, regardless of
// application-cursor mode (DECCKM): xterm's default modifyCursorKeys=2 (and
// every modern terminal + VT parser, e.g. ECMA-48 SS3 which takes no
// parameters) encodes modified keys as CSI 1;mod letter in both cursor
// modes. The SS3 form ESC O {letter} is only for UNMODIFIED application
// cursor keys (handled by KeyToTermBytes itself). Emitting ESC O {mod}{letter}
// would be parsed as an unknown SS3 sequence followed by a literal letter.
//
// Modifiers may appear in any order and each may appear at most once
// ("alt+alt+left" is rejected); the maps are package-level so the typing hot
// path allocates nothing.
var (
	modBits = map[string]int{
		"shift": 1,
		"alt":   2,
		"ctrl":  4,
		"meta":  8,
	}
	navMap = map[string]string{
		"up": "A", "down": "B", "right": "C", "left": "D",
		"home": "H", "end": "F",
	}
	tildeMap = map[string]string{
		"pgup": "5", "pgdown": "6", "delete": "3", "insert": "2",
	}
	// f1-f4 use the CSI 1;{mod}{P,Q,R,S} form; f5-f12 the CSI {num};{mod}~ form.
	fkeySS3Map = map[string]string{
		"f1": "P", "f2": "Q", "f3": "R", "f4": "S",
	}
	fkeyTildeMap = map[string]string{
		"f5": "15", "f6": "17", "f7": "18", "f8": "19",
		"f9": "20", "f10": "21", "f11": "23", "f12": "24",
	}
)

func encodeModNav(key string) (string, bool) {
	mod := 0
	rest := key
	for {
		progressed := false
		for name, bit := range modBits {
			if r, ok := strings.CutPrefix(rest, name+"+"); ok {
				if mod&bit != 0 {
					// Duplicate modifier ("alt+alt+left") is malformed.
					return "", false
				}
				mod |= bit
				rest = r
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	if mod == 0 {
		return "", false
	}
	xtermMod := strconv.Itoa(mod + 1)

	if letter, exists := navMap[rest]; exists {
		return "\x1b[1;" + xtermMod + letter, true
	}
	if num, exists := tildeMap[rest]; exists {
		return "\x1b[" + num + ";" + xtermMod + "~", true
	}
	if letter, exists := fkeySS3Map[rest]; exists {
		return "\x1b[1;" + xtermMod + letter, true
	}
	if num, exists := fkeyTildeMap[rest]; exists {
		return "\x1b[" + num + ";" + xtermMod + "~", true
	}
	return "", false
}

// stripCtrlAlt removes leading ctrl+/alt+ modifier prefixes (each at most
// once, in any order) and reports whether both ctrl AND alt were present.
// A duplicated modifier ("ctrl+alt+alt+a") is malformed and returns ok=false.
func stripCtrlAlt(key string) (rest string, ok bool) {
	rest = key
	sawCtrl, sawAlt := false, false
	for {
		if r, isCtrl := strings.CutPrefix(rest, "ctrl+"); isCtrl {
			if sawCtrl {
				return "", false
			}
			sawCtrl = true
			rest = r
			continue
		}
		if r, isAlt := strings.CutPrefix(rest, "alt+"); isAlt {
			if sawAlt {
				return "", false
			}
			sawAlt = true
			rest = r
			continue
		}
		break
	}
	return rest, sawCtrl && sawAlt
}

// ctrlLetter returns the control byte for a single ASCII letter (0x01–0x1A
// for a-z, case-insensitive), matching the ctrl+letter branch.
func ctrlLetter(ch byte) (byte, bool) {
	if ch >= 'a' && ch <= 'z' {
		return ch - 'a' + 1, true
	}
	if ch >= 'A' && ch <= 'Z' {
		return ch - 'A' + 1, true
	}
	return 0, false
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

// MouseButtonTea converts a bubbletea tea.MouseButton to a termmux MouseButton.
func MouseButtonTea(b tea.MouseButton) MouseButton {
	switch b {
	case tea.MouseLeft:
		return MouseLeft
	case tea.MouseMiddle:
		return MouseMiddle
	case tea.MouseRight:
		return MouseRight
	case tea.MouseWheelUp:
		return MouseWheelUp
	case tea.MouseWheelDown:
		return MouseWheelDown
	default:
		return MouseNone
	}
}

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
