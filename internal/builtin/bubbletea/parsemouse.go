package bubbletea

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ParseMouseEvent parses a string representation of a mouse event back into a MouseEvent.
//
// The string format matches tea.MouseEvent.String(), which is:
//   - [ctrl+][alt+][shift+]<button>[ <action>]
//
// Where:
//   - Modifiers (ctrl+, alt+, shift+) are optional prefixes
//   - Button is required (e.g., "left", "right", "wheel up")
//   - Action is optional for wheel buttons (they only press), required for regular buttons
//
// Examples:
//   - "left press" -> left button press
//   - "right release" -> right button release
//   - "wheel up" -> wheel up (action is always press for wheels)
//   - "ctrl+alt+left press" -> left button press with ctrl and alt modifiers
//   - "motion" -> motion event (button is none)
//   - "release" -> release event (button is none)
//
// The returned boolean indicates whether the parsing was successful.
func ParseMouseEvent(s string) (tea.MouseEvent, bool) {
	var m tea.MouseEvent

	if s == "" {
		return m, false
	}

	// Parse modifiers
	remaining := s
	for {
		if strings.HasPrefix(remaining, "ctrl+") {
			m.Ctrl = true
			remaining = remaining[5:]
		} else if strings.HasPrefix(remaining, "alt+") {
			m.Alt = true
			remaining = remaining[4:]
		} else if strings.HasPrefix(remaining, "shift+") {
			m.Shift = true
			remaining = remaining[6:]
		} else {
			break
		}
	}

	// Special case: "motion" or "release" without a button
	// (tea.MouseEvent.String() never returns "unknown", it returns "none")
	if remaining == "motion" {
		m.Button = tea.MouseButtonNone
		m.Action = tea.MouseActionMotion
		return m, true
	}
	if remaining == "release" {
		m.Button = tea.MouseButtonNone
		m.Action = tea.MouseActionRelease
		return m, true
	}

	// Find the longest matching button prefix.
	// This is critical because "wheel" would incorrectly match before "wheel up"
	// if we use the first match instead of the longest.
	var bestButtonDef *MouseButtonDef
	var bestButtonLen int
	for buttonStr, def := range MouseButtonDefs {
		if strings.HasPrefix(remaining, buttonStr) && len(buttonStr) > bestButtonLen {
			defCopy := def // Avoid capturing loop variable
			bestButtonDef = &defCopy
			bestButtonLen = len(buttonStr)
		}
	}

	if bestButtonDef == nil {
		// No button match found
		return m, false
	}

	m.Button = bestButtonDef.Button

	// Extract remainder after the button
	after := remaining[bestButtonLen:]
	after = strings.TrimPrefix(after, " ")

	// If no action specified
	if after == "" {
		// Wheel buttons are always press, others default to press
		m.Action = tea.MouseActionPress
		return m, true
	}

	// Try to parse the action
	if actionDef, ok := MouseActionDefs[after]; ok {
		m.Action = actionDef.Action
		return m, true
	}

	// Action not recognized - still return with default press action
	// This handles edge cases where bubbletea might add new actions
	m.Action = tea.MouseActionPress
	return m, true
}

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
