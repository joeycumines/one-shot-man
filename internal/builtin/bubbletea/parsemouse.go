package bubbletea

import tea "charm.land/bubbletea/v2"

// MouseEventToJS converts any mouse message to a JS representation.
func MouseEventToJS(msg tea.Msg) map[string]any {
    res := map[string]any{
        "type": "Mouse",
        "alt": false,
        "ctrl": false,
    }
    
    switch m := msg.(type) {
    case tea.MouseClickMsg:
        res["type"] = "MouseClick"
        res["x"] = m.X
        res["y"] = m.Y
        res["button"] = "left" // Simplify for now
    case tea.MouseReleaseMsg:
        res["type"] = "MouseRelease"
        res["x"] = m.X
        res["y"] = m.Y
    case tea.MouseMotionMsg:
        res["type"] = "MouseMotion"
        res["x"] = m.X
        res["y"] = m.Y
    case tea.MouseWheelMsg:
        res["type"] = "MouseWheel"
        res["x"] = m.X
        res["y"] = m.Y
    }
    return res
}
