//go:build unix

// This file contains a backward-compatibility shim for MouseTestAPI.
// DEPRECATED: New code should use mouseharness.Console directly.
package scripting

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/mouseharness"
)

// MouseTestAPI provides reusable mouse interaction utilities for integration tests.
// DEPRECATED: Use mouseharness.Console directly for new code.
type MouseTestAPI struct {
	console *mouseharness.Console
}

// NewMouseTestAPI creates a new MouseTestAPI for the given console.
// Uses default terminal height of 24 rows.
func NewMouseTestAPI(t *testing.T, cp *termtest.Console) *MouseTestAPI {
	console, err := mouseharness.New(
		mouseharness.WithTermtestConsole(cp),
		mouseharness.WithTestingTB(t),
		mouseharness.WithHeight(24),
	)
	if err != nil {
		t.Fatalf("failed to create mouseharness.Console: %v", err)
	}
	return &MouseTestAPI{console: console}
}

// SetHeight sets the terminal height for viewport calculations.
// Note: The new mouseharness API requires height at construction time.
// This method is a no-op in the shim for backward compatibility.
func (m *MouseTestAPI) SetHeight(_ int) {
	// No-op: height must be set at construction time with new API
}

// ElementLocation represents the location of a UI element in the terminal buffer.
type ElementLocation = mouseharness.ElementLocation

// FindElement searches the terminal buffer for the given content string.
func (m *MouseTestAPI) FindElement(content string) *ElementLocation {
	return m.console.FindElement(content)
}

// FindElementInBuffer searches a specific buffer for the given content string.
func (m *MouseTestAPI) FindElementInBuffer(buffer, content string) *ElementLocation {
	return m.console.FindElementInBuffer(buffer, content)
}

// ClickElement locates an element by its visible text content and clicks on it.
func (m *MouseTestAPI) ClickElement(ctx context.Context, content string, timeout time.Duration) error {
	return m.console.ClickElement(ctx, content, timeout)
}

// Click sends a mouse click at the specified viewport-relative coordinates.
func (m *MouseTestAPI) Click(x, y int) error {
	return m.console.Click(x, y)
}

// ClickViewport is an alias for Click.
func (m *MouseTestAPI) ClickViewport(x, y int) error {
	return m.console.ClickViewport(x, y)
}

// ClickAtBufferPosition sends a mouse click at buffer-absolute coordinates.
func (m *MouseTestAPI) ClickAtBufferPosition(x, bufferY int) error {
	return m.console.ClickAtBufferPosition(x, bufferY)
}

// ClickWithButton sends a mouse click with a specific button.
func (m *MouseTestAPI) ClickWithButton(x, y, button int) error {
	return m.console.ClickWithButton(x, y, button)
}

// ScrollWheel sends a mouse wheel event.
func (m *MouseTestAPI) ScrollWheel(x, y int, direction string) error {
	return m.console.ScrollWheel(x, y, direction)
}

// ScrollWheelOnElement finds an element and sends a scroll wheel event on it.
func (m *MouseTestAPI) ScrollWheelOnElement(_ context.Context, content string, direction string, _ time.Duration) error {
	return m.console.ScrollWheelOnElement(content, direction)
}

// ClickElementAndExpect clicks an element and waits for expected content.
func (m *MouseTestAPI) ClickElementAndExpect(ctx context.Context, clickTarget, expectContent string, timeout time.Duration) error {
	return m.console.ClickElementAndExpect(ctx, clickTarget, expectContent, timeout)
}

// RequireClickElement clicks an element and fails the test if not found.
func (m *MouseTestAPI) RequireClickElement(ctx context.Context, content string, timeout time.Duration) {
	m.console.RequireClickElement(ctx, content, timeout)
}

// RequireClick sends a click and fails the test if it cannot be sent.
func (m *MouseTestAPI) RequireClick(x, y int) {
	m.console.RequireClick(x, y)
}

// GetElementCenter returns the center coordinates of an element if found.
func (m *MouseTestAPI) GetElementCenter(content string) (x, y int, found bool) {
	return m.console.GetElementCenter(content)
}

// DebugBuffer prints the current buffer state for debugging.
func (m *MouseTestAPI) DebugBuffer() {
	m.console.DebugBuffer()
}

// Terminal buffer parsing functions - re-exported for backward compatibility

// parseTerminalBuffer is kept for backward compatibility with existing tests.
func parseTerminalBuffer(buffer string) []string {
	return mouseharness.ParseTerminalBuffer(buffer)
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	return mouseharness.StripANSI(s)
}
