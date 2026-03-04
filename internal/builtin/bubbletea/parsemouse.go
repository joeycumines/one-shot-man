package bubbletea

import (
	tea "github.com/charmbracelet/bubbletea"
)

// MouseEventToJS converts a tea.MouseEvent to a JavaScript-compatible map.
// This uses the generated MouseButtonDefs and MouseActionDefs for consistency
// with tea.MouseEvent.String().
func MouseEventToJS(msg tea.MouseMsg) map[string]interface{} {
	m := tea.MouseEvent(msg)

	// Get button string from generated defs
	buttonStr := "none"
	for str, def := range MouseButtonDefs {
		if def.Button == m.Button {
			buttonStr = str
			break
		}
	}

	// Get action string from generated defs
	actionStr := "press"
	for str, def := range MouseActionDefs {
		if def.Action == m.Action {
			actionStr = str
			break
		}
	}

	return map[string]interface{}{
		"type":    "Mouse",
		"x":       m.X,
		"y":       m.Y,
		"button":  buttonStr,
		"action":  actionStr,
		"alt":     m.Alt,
		"ctrl":    m.Ctrl,
		"shift":   m.Shift,
		"isWheel": m.IsWheel(),
		"string":  m.String(), // Full string representation for debugging
	}
}

// JSToMouseEvent converts a JavaScript mouse event object to a tea.MouseMsg.
// This is the inverse of MouseEventToJS.
func JSToMouseEvent(buttonStr, actionStr string, x, y int, alt, ctrl, shift bool) tea.MouseMsg {
	var m tea.MouseEvent
	m.X = x
	m.Y = y
	m.Alt = alt
	m.Ctrl = ctrl
	m.Shift = shift

	// Parse button from string using generated defs
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		m.Button = def.Button
	} else {
		m.Button = tea.MouseButtonNone
	}

	// Parse action from string using generated defs
	if def, ok := MouseActionDefs[actionStr]; ok {
		m.Action = def.Action
	} else {
		m.Action = tea.MouseActionPress
	}

	return tea.MouseMsg(m)
}
