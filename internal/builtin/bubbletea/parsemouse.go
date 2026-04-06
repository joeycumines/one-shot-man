package bubbletea

import (
	tea "charm.land/bubbletea/v2"
)

// MouseEventToJS converts a tea.MouseMsg to a JavaScript-compatible map.
// In v2, MouseMsg is an interface. We handle each concrete type.
func MouseEventToJS(msg tea.MouseMsg) map[string]any {
	mouse := msg.Mouse()

	// Get button string from generated defs
	buttonStr := "none"
	for str, def := range MouseButtonDefs {
		if def.Button == mouse.Button {
			buttonStr = str
			break
		}
	}

	// Determine the event type based on the concrete message type
	var eventType string
	switch msg.(type) {
	case tea.MouseClickMsg:
		eventType = "MouseClick"
	case tea.MouseReleaseMsg:
		eventType = "MouseRelease"
	case tea.MouseMotionMsg:
		eventType = "MouseMotion"
	case tea.MouseWheelMsg:
		eventType = "MouseWheel"
	default:
		eventType = "Mouse"
	}

	return map[string]any{
		"type":    eventType,
		"x":       mouse.X,
		"y":       mouse.Y,
		"button":  buttonStr,
		"mod":     modToStrings(mouse.Mod),
		"isWheel": IsWheelButton(mouse.Button),
		"string":  msg.String(), // Full string representation for debugging
	}
}

// JSToMouseEvent converts a JavaScript mouse event object to a tea.Msg.
// In v2, mouse events are split into separate message types.
func JSToMouseEvent(buttonStr, actionStr string, x, y int, alt, ctrl, shift bool) tea.Msg {
	// Build the mouse data
	var mod tea.KeyMod
	if alt {
		mod |= tea.ModAlt
	}
	if ctrl {
		mod |= tea.ModCtrl
	}
	if shift {
		mod |= tea.ModShift
	}

	// Parse button from string using generated defs
	var button tea.MouseButton
	if def, ok := MouseButtonDefs[buttonStr]; ok {
		button = def.Button
	} else {
		button = tea.MouseNone
	}

	// In v2, the action and button determine the message type.
	// Wheel buttons always produce MouseWheelMsg.
	if IsWheelButton(button) {
		return tea.MouseWheelMsg{Button: button, Mod: mod, X: x, Y: y}
	}

	switch actionStr {
	case "press", "click":
		return tea.MouseClickMsg{Button: button, Mod: mod, X: x, Y: y}
	case "release":
		return tea.MouseReleaseMsg{Mod: mod, X: x, Y: y}
	case "motion":
		return tea.MouseMotionMsg{Button: button, Mod: mod, X: x, Y: y}
	default:
		// Default to click
		return tea.MouseClickMsg{Button: button, Mod: mod, X: x, Y: y}
	}
}
