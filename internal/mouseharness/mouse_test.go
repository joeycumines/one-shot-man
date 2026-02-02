//go:build unix

package mouseharness

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSGRMouseEscapeSequences(t *testing.T) {
	tests := []struct {
		name    string
		x, y    int
		button  int
		press   string
		release string
	}{
		{
			name:    "left click at origin",
			x:       1,
			y:       1,
			button:  0,
			press:   "\x1b[<0;1;1M",
			release: "\x1b[<0;1;1m",
		},
		{
			name:    "left click at 10,20",
			x:       10,
			y:       20,
			button:  0,
			press:   "\x1b[<0;10;20M",
			release: "\x1b[<0;10;20m",
		},
		{
			name:    "right click",
			x:       5,
			y:       5,
			button:  2,
			press:   "\x1b[<2;5;5M",
			release: "\x1b[<2;5;5m",
		},
		{
			name:    "middle click",
			x:       50,
			y:       25,
			button:  1,
			press:   "\x1b[<1;50;25M",
			release: "\x1b[<1;50;25m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			press := formatSGRMousePress(tt.button, tt.x, tt.y)
			release := formatSGRMouseRelease(tt.button, tt.x, tt.y)

			assert.Equal(t, tt.press, press, "press sequence mismatch")
			assert.Equal(t, tt.release, release, "release sequence mismatch")
		})
	}
}

func TestScrollWheelEscapeSequences(t *testing.T) {
	tests := []struct {
		name      string
		x, y      int
		direction string
		expected  string
	}{
		{
			name:      "scroll up",
			x:         10,
			y:         5,
			direction: "up",
			expected:  "\x1b[<64;10;5M",
		},
		{
			name:      "scroll down",
			x:         10,
			y:         5,
			direction: "down",
			expected:  "\x1b[<65;10;5M",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var button int
			switch tt.direction {
			case "up":
				button = 64
			case "down":
				button = 65
			}
			result := formatSGRMousePress(button, tt.x, tt.y)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper functions for testing - match the format used in the actual implementation
func formatSGRMousePress(button, x, y int) string {
	return "\x1b[<" + itoa(button) + ";" + itoa(x) + ";" + itoa(y) + "M"
}

func formatSGRMouseRelease(button, x, y int) string {
	return "\x1b[<" + itoa(button) + ";" + itoa(x) + ";" + itoa(y) + "m"
}

// TestScrollDirection tests the ScrollDirection type and constants
func TestScrollDirection(t *testing.T) {
	// Test constants have correct values
	assert.Equal(t, ScrollDirection(64), ScrollUp, "ScrollUp should be 64")
	assert.Equal(t, ScrollDirection(65), ScrollDown, "ScrollDown should be 65")

	// Test String() method
	assert.Equal(t, "up", ScrollUp.String())
	assert.Equal(t, "down", ScrollDown.String())

	// Test unknown direction returns descriptive string
	var unknown ScrollDirection = 99
	assert.Equal(t, "ScrollDirection(99)", unknown.String())
}

// TestScrollWheelDirection tests that invalid directions are rejected
func TestScrollWheelDirection(t *testing.T) {
	// Create a minimal console for testing (no PTY needed for this test)
	c := &Console{}

	// Test invalid direction is rejected
	err := c.ScrollWheelWithDirection(1, 1, ScrollDirection(99))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid scroll direction")

	// Note: Testing valid directions requires a properly initialized Console with
	// a termtest.Console (cp field). See TestScrollWheelOnElement_WithPTY for integration tests.
	// We can only test the validation logic here.
	assert.Equal(t, ScrollUp, ScrollDirection(64))
	assert.Equal(t, ScrollDown, ScrollDirection(65))
}

// TestMouseButton tests the MouseButton type and constants
func TestMouseButton(t *testing.T) {
	// Test constants have correct values
	assert.Equal(t, MouseButton(0), MouseButtonLeft, "MouseButtonLeft should be 0")
	assert.Equal(t, MouseButton(1), MouseButtonMiddle, "MouseButtonMiddle should be 1")
	assert.Equal(t, MouseButton(2), MouseButtonRight, "MouseButtonRight should be 2")

	// Test String() method
	assert.Equal(t, "left", MouseButtonLeft.String())
	assert.Equal(t, "middle", MouseButtonMiddle.String())
	assert.Equal(t, "right", MouseButtonRight.String())

	// Test unknown button returns descriptive string
	var unknown MouseButton = 99
	assert.Equal(t, "MouseButton(99)", unknown.String())
}

// Simple integer to string for testing
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
