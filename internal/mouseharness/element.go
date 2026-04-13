//go:build unix

package mouseharness

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	// IMPORTANT: Capture the buffer exactly once and derive ALL coordinates
	// from it. Using multiple c.cp.String() calls creates a TOCTOU race
	// where the buffer changes between reads, producing inconsistent
	// viewport calculations and wrong click targets.
	{
		buffer := c.cp.String()
		screen := parseTerminalBuffer(buffer)

		// Re-find the element in this specific buffer snapshot so the
		// row/col are consistent with the viewport calculation below.
		loc = c.FindElementInBuffer(buffer, content)
		if loc == nil {
			return fmt.Errorf("element %q disappeared between find and click; buffer: %q", content, buffer)
		}

		centerX := loc.Col + loc.Width/2

		// Inline viewport calculation from the SAME screen snapshot.
		totalRows := len(screen)
		for totalRows > 0 && strings.TrimSpace(screen[totalRows-1]) == "" {
			totalRows--
		}
		visibleTop := 1
		if totalRows > c.height {
			visibleTop = totalRows - c.height + 1
		}
		viewportY := min(max(loc.Row-(visibleTop-1), 1), c.height)

		c.tb.Logf("[CLICK DEBUG] ClickElement %q: loc.Row=%d (buffer-absolute), viewportY=%d, centerX=%d, totalRows=%d, visibleTop=%d",
			content, loc.Row, viewportY, centerX, totalRows, visibleTop)

		// Send mouse click using viewport-relative coordinates
		return c.Click(centerX, viewportY)
	}
}

// WaitForContent polls the VT-parsed terminal screen for a substring.
// BubbleTea v2 uses differential rendering (cursor movement + only changed
// characters), so raw byte checking (termtest.Contains) fails for state
// changes after the initial render. This method re-parses the full terminal
// buffer on each poll to get the actual rendered screen content.
func (c *Console) WaitForContent(ctx context.Context, content string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Immediate check
	if c.screenContains(content) {
		return nil
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			buffer := c.cp.String()
			screen := parseTerminalBuffer(buffer)
			return fmt.Errorf("content %q not found on rendered screen within timeout\nScreen lines:\n%s",
				content, strings.Join(screen, "\n"))
		case <-ticker.C:
			if c.screenContains(content) {
				return nil
			}
		}
	}
}

// screenContains checks whether the VT-parsed screen contains a substring.
func (c *Console) screenContains(content string) bool {
	buffer := c.cp.String()
	screen := parseTerminalBuffer(buffer)
	for _, line := range screen {
		if strings.Contains(line, content) {
			return true
		}
	}
	return false
}

// ClickElementAndExpect clicks an element and waits for expected content to appear.
// Uses VT-parsed screen polling (WaitForContent) instead of raw byte checking,
// which is required for BubbleTea v2's differential rendering.
func (c *Console) ClickElementAndExpect(ctx context.Context, clickTarget, expectContent string, timeout time.Duration) error {
	if err := c.ClickElement(ctx, clickTarget, timeout/2); err != nil {
		return fmt.Errorf("failed to click %q: %w", clickTarget, err)
	}

	if err := c.WaitForContent(ctx, expectContent, timeout/2); err != nil {
		return fmt.Errorf("expected %q after clicking %q: %w", expectContent, clickTarget, err)
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
