//go:build unix

package mouseharness

import (
	"fmt"
	"time"
)

// Click sends a mouse click at the specified viewport-relative coordinates (1-indexed).
// Use this when you know the exact viewport position.
// It sends both press and release events using SGR extended mouse mode.
func (c *Console) Click(x, y int) error {
	return c.ClickWithButton(x, y, 0) // 0 = left button
}

// ClickViewport is an alias for Click, emphasizing that coordinates are
// viewport-relative (not buffer-absolute). SGR mouse events always use
// viewport-relative coordinates where row 1 is the top visible row.
func (c *Console) ClickViewport(x, y int) error {
	return c.Click(x, y)
}

// ClickAtBufferPosition sends a mouse click at the specified buffer-absolute
// coordinates (1-indexed). The buffer row is converted to viewport-relative
// coordinates before sending the SGR mouse event.
func (c *Console) ClickAtBufferPosition(x, bufferY int) error {
	viewportY := c.bufferRowToViewportRow(bufferY)
	return c.Click(x, viewportY)
}

// ClickWithButton sends a mouse click with a specific button at the coordinates.
// Coordinates are viewport-relative (1-indexed).
// Button values: 0=left, 1=middle, 2=right
func (c *Console) ClickWithButton(x, y, button int) error {
	// SGR mouse encoding: ESC [ < Cb ; Cx ; Cy M (press) / m (release)
	// Cb = button number (0=left, 1=middle, 2=right)
	mousePress := fmt.Sprintf("\x1b[<%d;%d;%dM", button, x, y)
	mouseRelease := fmt.Sprintf("\x1b[<%d;%d;%dm", button, x, y)

	if _, err := c.cp.WriteString(mousePress); err != nil {
		return fmt.Errorf("failed to send mouse press: %w", err)
	}

	// Small delay between press and release for realism
	time.Sleep(30 * time.Millisecond)

	if _, err := c.cp.WriteString(mouseRelease); err != nil {
		return fmt.Errorf("failed to send mouse release: %w", err)
	}

	return nil
}

// ScrollWheel sends a mouse wheel event at the specified viewport-relative coordinates.
// Coordinates are 1-indexed. direction: "up" or "down".
// SGR mouse encoding: Button 64 = wheel up, Button 65 = wheel down.
func (c *Console) ScrollWheel(x, y int, direction string) error {
	var button int
	switch direction {
	case "up":
		button = 64 // SGR encoding for wheel up
	case "down":
		button = 65 // SGR encoding for wheel down
	default:
		return fmt.Errorf("unknown scroll direction: %s (use 'up' or 'down')", direction)
	}

	// SGR mouse encoding: ESC [ < Cb ; Cx ; Cy M (wheel events are press-only)
	mouseEvent := fmt.Sprintf("\x1b[<%d;%d;%dM", button, x, y)

	if _, err := c.cp.WriteString(mouseEvent); err != nil {
		return fmt.Errorf("failed to send scroll wheel event: %w", err)
	}

	return nil
}

// ScrollWheelOnElement finds an element and sends a scroll wheel event on it.
func (c *Console) ScrollWheelOnElement(content string, direction string) error {
	loc := c.FindElement(content)
	if loc == nil {
		return fmt.Errorf("element %q not found for scroll", content)
	}
	centerX := loc.Col + loc.Width/2
	viewportY := c.bufferRowToViewportRow(loc.Row)
	return c.ScrollWheel(centerX, viewportY, direction)
}
