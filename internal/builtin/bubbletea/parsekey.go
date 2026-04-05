package bubbletea

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/rivo/uniseg"
)

// ParseKey parses a string representation of a key event back into a KeyPressMsg.
func ParseKey(s string) (tea.KeyPressMsg, bool) {
	var k tea.Key

	if s == "" {
		return tea.KeyPressMsg(k), false
	}

	const altPrefix = "alt+"
	if strings.HasPrefix(s, altPrefix) {
		k.Mod |= tea.ModAlt
		s = s[len(altPrefix):]
	}

	const ctrlPrefix = "ctrl+"
	if strings.HasPrefix(s, ctrlPrefix) {
		k.Mod |= tea.ModCtrl
		s = s[len(ctrlPrefix):]
	}

	// Handle Bracketed paste
	if len(s) > 2 && strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		// Paste is no longer a flag on Key in v2, it uses PasteMsg, but let's just return the text
		k.Text = s[1 : len(s)-1]
		return tea.KeyPressMsg(k), true
	}

	// Try common named keys
	switch s {
	case "up":
		k.Code = tea.KeyUp
	case "down":
		k.Code = tea.KeyDown
	case "right":
		k.Code = tea.KeyRight
	case "left":
		k.Code = tea.KeyLeft
	case "enter":
		k.Code = tea.KeyEnter
	case "space":
		k.Code = tea.KeySpace
		k.Text = " "
	case "esc":
		k.Code = tea.KeyEscape
	case "backspace":
		k.Code = tea.KeyBackspace
	case "tab":
		k.Code = tea.KeyTab
	case "shift+tab":
		k.Code = tea.KeyTab
		k.Mod |= tea.ModShift
	case "home":
		k.Code = tea.KeyHome
	case "end":
		k.Code = tea.KeyEnd
	case "pgup":
		k.Code = tea.KeyPgUp
	case "pgdown":
		k.Code = tea.KeyPgDown
	case "delete":
		k.Code = tea.KeyDelete
	default:
		// Raw rune
		runes := []rune(s)
		if len(runes) == 1 {
			k.Code = runes[0]
			k.Text = s
		} else {
			k.Text = s
		}
	}

	return tea.KeyPressMsg(k), len(s) == 1 || uniseg.StringWidth(s) == 1 || k.Code != 0
}
