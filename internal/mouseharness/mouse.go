//go:build unix

package mouseharness

import (
	"fmt"
	"time"
)

// MouseButton represents a mouse button for click events.
// SGR encoding uses: 0=left, 1=middle, 2=right.
type MouseButton int

const (
	// MouseButtonLeft represents the left mouse button (SGR code 0).
	MouseButtonLeft MouseButton = 0
	// MouseButtonMiddle represents the middle mouse button/wheel (SGR code 1).
	MouseButtonMiddle MouseButton = 1
	// MouseButtonRight represents the right mouse button (SGR code 2).
	MouseButtonRight MouseButton = 2
)

// String returns a string representation of the mouse button.
// This implements the fmt.Stringer interface for logging.
func (b MouseButton) String() string {
	switch b {
	case MouseButtonLeft:
		return "left"
	case MouseButtonMiddle:
		return "middle"
	case MouseButtonRight:
		return "right"
	default:
		return fmt.Sprintf("MouseButton(%d)", b)
	}
}

// ScrollDirection represents the direction of a scroll wheel event.
// Use ScrollUp or ScrollDown constants.
type ScrollDirection int

const (
	// ScrollUp represents upward scroll wheel direction (toward user).
	ScrollUp ScrollDirection = 64 // SGR encoding for wheel up
	// ScrollDown represents downward scroll wheel direction (away from user).
	ScrollDown ScrollDirection = 65 // SGR encoding for wheel down
)

// String returns a string representation of the scroll direction.
// This implements the fmt.Stringer interface for logging.
func (d ScrollDirection) String() string {
	switch d {
	case ScrollUp:
		return "up"
	case ScrollDown:
		return "down"
	default:
		return fmt.Sprintf("ScrollDirection(%d)", d)
	}
}

// Click sends a mouse click at the specified viewport-relative coordinates (1-indexed).
// Use this when you know the exact viewport position.
// It sends both press and release events using SGR extended mouse mode.
func (c *Console) Click(x, y int) error {
	return c.ClickWithButton(x, y, MouseButtonLeft)
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
// Use MouseButtonLeft, MouseButtonMiddle, or MouseButtonRight constants.
func (c *Console) ClickWithButton(x, y int, button MouseButton) error {
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
// Coordinates are 1-indexed. Use ScrollUp or ScrollDown for direction.
// SGR mouse encoding: Button 64 = wheel up, Button 65 = wheel down.
//
// Deprecated: Use ScrollWheelWithDirection for type-safe direction.
func (c *Console) ScrollWheel(x, y int, direction string) error {
	var dir ScrollDirection
	switch direction {
	case "up":
		dir = ScrollUp
	case "down":
		dir = ScrollDown
	default:
		return fmt.Errorf("unknown scroll direction: %s (use 'up' or 'down')", direction)
	}
	return c.ScrollWheelWithDirection(x, y, dir)
}

// ScrollWheelWithDirection sends a mouse wheel event with a type-safe direction.
// Coordinates are 1-indexed. direction must be ScrollUp or ScrollDown.
// SGR mouse encoding: Button 64 = wheel up, Button 65 = wheel down.
func (c *Console) ScrollWheelWithDirection(x, y int, direction ScrollDirection) error {
	if direction != ScrollUp && direction != ScrollDown {
		return fmt.Errorf("invalid scroll direction: %v (use ScrollUp or ScrollDown)", direction)
	}

	// SGR mouse encoding: ESC [ < Cb ; Cx ; Cy M (wheel events are press-only)
	mouseEvent := fmt.Sprintf("\x1b[<%d;%d;%dM", direction, x, y)

	if _, err := c.cp.WriteString(mouseEvent); err != nil {
		return fmt.Errorf("failed to send scroll wheel event: %w", err)
	}

	return nil
}

// ScrollWheelOnElement finds an element and sends a scroll wheel event on it.
// Deprecated: Use ScrollWheelOnElementWithDirection for type-safe direction.
func (c *Console) ScrollWheelOnElement(content string, direction string) error {
	loc := c.FindElement(content)
	if loc == nil {
		return fmt.Errorf("element %q not found for scroll", content)
	}
	centerX := loc.Col + loc.Width/2
	viewportY := c.bufferRowToViewportRow(loc.Row)
	return c.ScrollWheel(centerX, viewportY, direction)
}

// ScrollWheelOnElementWithDirection finds an element and sends a scroll wheel event on it.
// Uses type-safe ScrollDirection (ScrollUp or ScrollDown).
func (c *Console) ScrollWheelOnElementWithDirection(content string, direction ScrollDirection) error {
	loc := c.FindElement(content)
	if loc == nil {
		return fmt.Errorf("element %q not found for scroll", content)
	}
	centerX := loc.Col + loc.Width/2
	viewportY := c.bufferRowToViewportRow(loc.Row)
	return c.ScrollWheelWithDirection(centerX, viewportY, direction)
}
