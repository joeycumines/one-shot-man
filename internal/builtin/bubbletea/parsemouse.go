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
	}

	return map[string]any{
		"type":   eventType,
		"x":      mouse.X,
		"y":      mouse.Y,
		"button": buttonStr,
		"mod":    modToStrings(mouse.Mod),
		"string": msg.String(), // Full string representation for debugging
	}
}
