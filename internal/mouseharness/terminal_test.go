//go:build unix

package mouseharness

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTerminalBuffer_PlainText(t *testing.T) {
	buffer := "Hello World\nSecond Line\nThird Line"
	screen := parseTerminalBuffer(buffer)

	assert.Equal(t, "Hello World", screen[0])
	assert.Equal(t, "Second Line", screen[1])
	assert.Equal(t, "Third Line", screen[2])
}

func TestParseTerminalBuffer_CursorPositioning(t *testing.T) {
	tests := []struct {
		name     string
		buffer   string
		row      int
		expected string
	}{
		{
			name:     "move to position",
			buffer:   "\x1b[2;5HText",
			row:      1,
			expected: "Text",
		},
		{
			name:     "clear screen and write",
			buffer:   "\x1b[2JHello",
			row:      0,
			expected: "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen := parseTerminalBuffer(tt.buffer)
			assert.Contains(t, screen[tt.row], tt.expected)
		})
	}
}

func TestParseTerminalBuffer_ANSIColors(t *testing.T) {
	// ANSI colors should be stripped/ignored, text preserved
	buffer := "\x1b[31mRed\x1b[0m Normal \x1b[32mGreen\x1b[0m"
	screen := parseTerminalBuffer(buffer)

	assert.Contains(t, screen[0], "Red")
	assert.Contains(t, screen[0], "Normal")
	assert.Contains(t, screen[0], "Green")
}

func TestParseTerminalBuffer_AltScreen(t *testing.T) {
	// Alt screen switch should clear buffer
	buffer := "Before\x1b[?1049hAfter"
	screen := parseTerminalBuffer(buffer)

	// After alt screen switch, "Before" should be gone
	assert.Equal(t, "After", screen[0])
}

func TestParseTerminalBuffer_CursorMovement(t *testing.T) {
	tests := []struct {
		name   string
		buffer string
		check  func([]string)
	}{
		{
			name:   "cursor up",
			buffer: "Line1\nLine2\x1b[1AUp",
			check: func(s []string) {
				assert.Contains(t, s[0], "Up")
			},
		},
		{
			name:   "cursor down",
			buffer: "Line1\x1b[1BDown",
			check: func(s []string) {
				assert.Contains(t, s[1], "Down")
			},
		},
		{
			name:   "cursor forward",
			buffer: "X\x1b[5CY",
			check: func(s []string) {
				assert.True(t, len(s[0]) > 5)
			},
		},
		{
			name:   "cursor back",
			buffer: "ABCDEF\x1b[3DX",
			check: func(s []string) {
				assert.Contains(t, s[0], "X")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen := parseTerminalBuffer(tt.buffer)
			tt.check(screen)
		})
	}
}

func TestParseTerminalBuffer_EraseOperations(t *testing.T) {
	tests := []struct {
		name   string
		buffer string
		check  func([]string)
	}{
		{
			name:   "erase to end of line",
			buffer: "Hello World\x1b[6D\x1b[K",
			check: func(s []string) {
				assert.Equal(t, "Hello", s[0])
			},
		},
		{
			name:   "erase entire screen",
			buffer: "Content\x1b[2JNew",
			check: func(s []string) {
				assert.Equal(t, "       New", s[0])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen := parseTerminalBuffer(tt.buffer)
			tt.check(screen)
		})
	}
}

func TestParseTerminalBuffer_SpecialChars(t *testing.T) {
	tests := []struct {
		name   string
		buffer string
		check  func([]string)
	}{
		{
			name:   "carriage return",
			buffer: "Old\rNew",
			check: func(s []string) {
				assert.Equal(t, "New", s[0])
			},
		},
		{
			name:   "tab",
			buffer: "A\tB",
			check: func(s []string) {
				assert.Contains(t, s[0], "A")
				assert.Contains(t, s[0], "B")
			},
		},
		{
			name:   "backspace",
			buffer: "ABC\bX",
			check: func(s []string) {
				assert.Contains(t, s[0], "ABX")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			screen := parseTerminalBuffer(tt.buffer)
			tt.check(screen)
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no escape codes",
			input:    "Hello World",
			expected: "Hello World",
		},
		{
			name:     "simple CSI color",
			input:    "\x1b[31mRed\x1b[0m",
			expected: "Red",
		},
		{
			name:     "bold and color",
			input:    "\x1b[1;32mBold Green\x1b[0m",
			expected: "Bold Green",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2J\x1b[HHello",
			expected: "Hello",
		},
		{
			name:     "multiple sequences",
			input:    "\x1b[1m[\x1b[32mA\x1b[0m]\x1b[0mdd",
			expected: "[A]dd",
		},
		{
			name:     "OSC sequence with BEL",
			input:    "\x1b]0;Title\x07Text",
			expected: "Text",
		},
		{
			name:     "mixed content",
			input:    "Start \x1b[31mRed\x1b[0m Middle \x1b[34mBlue\x1b[0m End",
			expected: "Start Red Middle Blue End",
		},
		{
			name:     "lipgloss styled button",
			input:    "\x1b[38;5;15m\x1b[48;5;35m  [A]dd  \x1b[0m",
			expected: "  [A]dd  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCSITerminator(t *testing.T) {
	// Valid terminators: 0x40-0x7E (@ through ~)
	assert.True(t, isCSITerminator('@')) // 0x40
	assert.True(t, isCSITerminator('H')) // Cursor position
	assert.True(t, isCSITerminator('m')) // SGR
	assert.True(t, isCSITerminator('~')) // 0x7E

	// Invalid terminators
	assert.False(t, isCSITerminator('0'))    // Digit
	assert.False(t, isCSITerminator(';'))    // Separator
	assert.False(t, isCSITerminator('?'))    // Private
	assert.False(t, isCSITerminator('\x7F')) // DEL
}
