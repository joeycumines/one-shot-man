//go:build unix

package mouseharness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// ElementLocation represents the location of a UI element in the terminal buffer.
type ElementLocation struct {
	Row    int    // 1-indexed row
	Col    int    // 1-indexed column
	Width  int    // Width of the element
	Height int    // Height of the element (usually 1)
	Text   string // The matched text
}

// FindElement searches the terminal buffer for the given content string.
// Returns the location of the first occurrence, or nil if not found.
// Strips ANSI escape codes before searching to match visible text.
func (c *Console) FindElement(content string) *ElementLocation {
	buffer := c.cp.String()
	return c.FindElementInBuffer(buffer, content)
}

// FindElementInBuffer searches a specific buffer for the given content string.
// It uses a virtual terminal emulator to track cursor position and screen content.
func (c *Console) FindElementInBuffer(buffer, content string) *ElementLocation {
	// Parse buffer into virtual screen state
	screen := parseTerminalBuffer(buffer)

	// Search for content in the virtual screen
	for row, line := range screen {
		colIdx := strings.Index(line, content)
		if colIdx >= 0 {
			return &ElementLocation{
				Row:    row + 1,    // 1-indexed
				Col:    colIdx + 1, // 1-indexed
				Width:  len(content),
				Height: 1,
				Text:   content,
			}
		}
	}

	return nil
}

// ClickElement locates an element by its visible text content and clicks on it.
// It dynamically reads the terminal buffer, finds the element, calculates the
// center coordinates, and sends SGR mouse press/release events.
//
// Returns an error if the element cannot be found within the timeout.
func (c *Console) ClickElement(ctx context.Context, content string, timeout time.Duration) error {
	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Check immediately before waiting - if element is already visible, don't wait 50ms
	var loc *ElementLocation
	loc = c.FindElement(content)
	if loc != nil {
		goto found
	}

	// Poll for the element to appear
	{
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("element %q not found within timeout; buffer: %q", content, c.cp.String())
			case <-ticker.C:
				loc = c.FindElement(content)
				if loc != nil {
					goto found
				}
			}
		}
	}

found:
	// Calculate center of element (screen coordinates)
	centerX := loc.Col + loc.Width/2
	screenY := loc.Row

	// Check if row 0 is empty in the parsed buffer - this indicates a render issue
	screen := parseTerminalBuffer(c.cp.String())
	row0Empty := len(screen) > 0 && strings.TrimSpace(screen[0]) == ""
	if row0Empty {
		screenY++ // Compensate for missing title line
		c.tb.Logf("[CLICK DEBUG] Row 0 is empty, adjusting screenY from %d to %d", screenY-1, screenY)
	}

	c.tb.Logf("[CLICK DEBUG] ClickElement %q: loc.Row=%d (1-indexed), centerX=%d, screenY=%d", content, loc.Row, centerX, screenY)

	// Send mouse click using screen-relative coordinates
	return c.Click(centerX, screenY)
}

// ClickElementAndExpect clicks an element and waits for expected content to appear.
func (c *Console) ClickElementAndExpect(ctx context.Context, clickTarget, expectContent string, timeout time.Duration) error {
	snap := c.cp.Snapshot()

	if err := c.ClickElement(ctx, clickTarget, timeout/2); err != nil {
		return fmt.Errorf("failed to click %q: %w", clickTarget, err)
	}

	// Wait for expected content
	expectCtx, cancel := context.WithTimeout(ctx, timeout/2)
	defer cancel()

	if err := c.cp.Expect(expectCtx, snap, termtest.Contains(expectContent), fmt.Sprintf("wait for %q after clicking %q", expectContent, clickTarget)); err != nil {
		return fmt.Errorf("expected %q after clicking %q: %w\nBuffer: %q", expectContent, clickTarget, err, c.cp.String())
	}

	return nil
}

// RequireClickElement clicks an element and fails the test if it cannot be found.
func (c *Console) RequireClickElement(ctx context.Context, content string, timeout time.Duration) {
	c.tb.Helper()
	if err := c.ClickElement(ctx, content, timeout); err != nil {
		c.tb.Fatalf("RequireClickElement failed: %v", err)
	}
}

// RequireClick sends a click and fails the test if it cannot be sent.
func (c *Console) RequireClick(x, y int) {
	c.tb.Helper()
	if err := c.Click(x, y); err != nil {
		c.tb.Fatalf("RequireClick failed: %v", err)
	}
}

// GetElementCenter returns the center coordinates of an element if found.
func (c *Console) GetElementCenter(content string) (x, y int, found bool) {
	loc := c.FindElement(content)
	if loc == nil {
		return 0, 0, false
	}
	return loc.Col + loc.Width/2, loc.Row, true
}

// DebugBuffer prints the current buffer state with line numbers for debugging.
func (c *Console) DebugBuffer() {
	c.tb.Helper()
	buffer := c.cp.String()
	lines := strings.Split(buffer, "\n")
	c.tb.Log("=== Buffer State ===")
	for i, line := range lines {
		cleanLine := stripANSI(line)
		c.tb.Logf("Line %2d: %q (clean: %q)", i+1, line, cleanLine)
	}
	c.tb.Log("=== End Buffer ===")
}
