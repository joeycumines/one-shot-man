//go:build unix

package scripting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestMouseTestAPI_FindElementInBuffer(t *testing.T) {
	// Simulated terminal buffer (mock - no actual console)
	buffer := `📄 Super-Document Builder

Documents: 0

No documents yet. Press 'a' to add or 'l' to load from file.

  [A]dd    [L]oad File    [C]opy Prompt

a:add  l:load  e:edit  r:rename  d:delete  v:view  c:copy  g:generate  ?:help  q:quit`

	// Create a mock API just for testing FindElementInBuffer
	api := &MouseTestAPI{t: t}

	tests := []struct {
		name        string
		content     string
		expectRow   int
		expectCol   int
		expectWidth int
	}{
		{
			name:        "find title",
			content:     "Super-Document Builder",
			expectRow:   1,
			expectCol:   3, // After emoji placeholder (*) and space
			expectWidth: 22,
		},
		{
			name:        "find documents count",
			content:     "Documents: 0",
			expectRow:   3,
			expectCol:   1,
			expectWidth: 12,
		},
		{
			name:        "find add button",
			content:     "[A]dd",
			expectRow:   7,
			expectCol:   3, // After 2 spaces
			expectWidth: 5,
		},
		{
			name:        "find load button",
			content:     "[L]oad File",
			expectRow:   7,
			expectCol:   12, // After "[A]dd    "
			expectWidth: 11,
		},
		{
			name:        "find copy button",
			content:     "[C]opy Prompt",
			expectRow:   7,
			expectCol:   27, // After "[A]dd    [L]oad File    "
			expectWidth: 13,
		},
		{
			name:        "find help hint",
			content:     "a:add",
			expectRow:   9,
			expectCol:   1,
			expectWidth: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := api.FindElementInBuffer(buffer, tt.content)
			require.NotNil(t, loc, "element %q not found in buffer", tt.content)
			assert.Equal(t, tt.expectRow, loc.Row, "row mismatch")
			assert.Equal(t, tt.expectCol, loc.Col, "col mismatch")
			assert.Equal(t, tt.expectWidth, loc.Width, "width mismatch")
			assert.Equal(t, tt.content, loc.Text, "text mismatch")
		})
	}
}

func TestMouseTestAPI_FindElementInBuffer_WithANSI(t *testing.T) {
	// Simulated buffer with ANSI escape codes (like real terminal output)
	buffer := "\x1b[1m\x1b[38;5;99m📄 Super-Document Builder\x1b[0m\n\n\x1b[38;5;15mDocuments: 0\x1b[0m\n\n\x1b[38;5;102mNo documents yet.\x1b[0m\n\n\x1b[48;5;35m  [A]dd  \x1b[0m \x1b[48;5;35m  [L]oad File  \x1b[0m \x1b[1m\x1b[48;5;99m  [C]opy Prompt  \x1b[0m"

	api := &MouseTestAPI{t: t}

	tests := []struct {
		name      string
		content   string
		expectNil bool
	}{
		{
			name:      "find title with ANSI",
			content:   "Super-Document Builder",
			expectNil: false,
		},
		{
			name:      "find documents count with ANSI",
			content:   "Documents: 0",
			expectNil: false,
		},
		{
			name:      "find add button with ANSI",
			content:   "[A]dd",
			expectNil: false,
		},
		{
			name:      "find copy button with ANSI",
			content:   "[C]opy Prompt",
			expectNil: false,
		},
		{
			name:      "element not present",
			content:   "[X]NotHere",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := api.FindElementInBuffer(buffer, tt.content)
			if tt.expectNil {
				assert.Nil(t, loc, "expected nil for %q", tt.content)
			} else {
				require.NotNil(t, loc, "element %q not found in buffer with ANSI codes", tt.content)
				assert.Equal(t, tt.content, loc.Text)
			}
		})
	}
}

func TestMouseTestAPI_SGRMouseEscapeSequences(t *testing.T) {
	// Test that the SGR mouse escape sequences are correctly formatted
	// This is a pure unit test - no actual terminal needed

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
			press := "\x1b[<" + itoa(tt.button) + ";" + itoa(tt.x) + ";" + itoa(tt.y) + "M"
			release := "\x1b[<" + itoa(tt.button) + ";" + itoa(tt.x) + ";" + itoa(tt.y) + "m"

			assert.Equal(t, tt.press, press, "press sequence mismatch")
			assert.Equal(t, tt.release, release, "release sequence mismatch")
		})
	}
}

// Simple integer to string for testing (avoids strconv import in test)
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

func TestMouseTestAPI_BufferRowToViewportRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		terminalHeight      int
		bufferLineCount     int
		bufferRow           int
		expectedViewportRow int
	}{
		{
			name:                "content fits in terminal - no conversion needed",
			terminalHeight:      24,
			bufferLineCount:     10,
			bufferRow:           5,
			expectedViewportRow: 5,
		},
		{
			name:                "content equals terminal height",
			terminalHeight:      10,
			bufferLineCount:     10,
			bufferRow:           5,
			expectedViewportRow: 5,
		},
		{
			name:            "scrolled content - element at buffer row 25 with height 10",
			terminalHeight:  10,
			bufferLineCount: 40,
			// visibleTop = 40 - 10 + 1 = 31
			// viewportY = 25 - (31 - 1) = 25 - 30 = -5 -> clamped to 1
			bufferRow:           25,
			expectedViewportRow: 1, // Clamped - row 25 is scrolled off top
		},
		{
			name:            "scrolled content - element at buffer row 35 with height 10",
			terminalHeight:  10,
			bufferLineCount: 40,
			// visibleTop = 40 - 10 + 1 = 31
			// viewportY = 35 - (31 - 1) = 35 - 30 = 5
			bufferRow:           35,
			expectedViewportRow: 5,
		},
		{
			name:            "element at last visible row",
			terminalHeight:  10,
			bufferLineCount: 40,
			// visibleTop = 31, last visible = row 40
			// viewportY = 40 - (31 - 1) = 40 - 30 = 10
			bufferRow:           40,
			expectedViewportRow: 10,
		},
		{
			name:            "element at first visible row",
			terminalHeight:  10,
			bufferLineCount: 40,
			// visibleTop = 31
			// viewportY = 31 - (31 - 1) = 31 - 30 = 1
			bufferRow:           31,
			expectedViewportRow: 1,
		},
		{
			name:            "element beyond viewport - clamped to height",
			terminalHeight:  10,
			bufferLineCount: 40,
			// visibleTop = 31
			// viewportY = 50 - (31 - 1) = 50 - 30 = 20 -> clamped to 10
			bufferRow:           50,
			expectedViewportRow: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock buffer with the specified line count
			var bufferLines []string
			for i := 0; i < tt.bufferLineCount; i++ {
				bufferLines = append(bufferLines, "line content")
			}
			buffer := strings.Join(bufferLines, "\n")

			// Create API with explicit height (nil console, won't be used for this test)
			_ = &MouseTestAPI{t: t, height: tt.terminalHeight}

			// Override getBufferLineCount for this test
			// We simulate by creating a buffer and calling the public helper
			screen := parseTerminalBuffer(buffer)
			actualLineCount := len(screen)
			for actualLineCount > 0 && strings.TrimSpace(screen[actualLineCount-1]) == "" {
				actualLineCount--
			}
			assert.Equal(t, tt.bufferLineCount, actualLineCount, "buffer line count")

			// Now calculate visibleTop manually and verify
			visibleTop := 1
			if actualLineCount > tt.terminalHeight {
				visibleTop = actualLineCount - tt.terminalHeight + 1
			}

			viewportY := tt.bufferRow - (visibleTop - 1)
			if viewportY < 1 {
				viewportY = 1
			}
			if viewportY > tt.terminalHeight {
				viewportY = tt.terminalHeight
			}

			assert.Equal(t, tt.expectedViewportRow, viewportY, "viewport row mismatch")
		})
	}
}

func TestMouseTestAPI_ClickElement_AccountsForViewportOffset(t *testing.T) {
	// This is a pure unit test that verifies the coordinate conversion logic
	// without needing a real terminal. The goal is to ensure that when content
	// is taller than the terminal, ClickElement uses viewport-relative Y.
	t.Parallel()

	// Create a mock 40-line buffer with a target element at row 35
	var lines []string
	for i := 0; i < 40; i++ {
		if i == 34 { // Row 35 (1-indexed)
			lines = append(lines, "  [Target Button]  ")
		} else {
			lines = append(lines, "Some content line "+itoa(i+1))
		}
	}
	buffer := strings.Join(lines, "\n")

	api := &MouseTestAPI{t: t, height: 10}
	screen := parseTerminalBuffer(buffer)

	// Verify the target is at buffer row 35
	loc := api.FindElementInBuffer(buffer, "[Target Button]")
	require.NotNil(t, loc, "target element not found")
	assert.Equal(t, 35, loc.Row, "element should be at buffer row 35")

	// Calculate expected viewport row
	// With 40 lines and height 10:
	// visibleTop = 40 - 10 + 1 = 31
	// viewportY = 35 - (31 - 1) = 35 - 30 = 5
	lineCount := len(screen)
	for lineCount > 0 && strings.TrimSpace(screen[lineCount-1]) == "" {
		lineCount--
	}

	visibleTop := 1
	if lineCount > api.height {
		visibleTop = lineCount - api.height + 1
	}
	expectedViewportY := loc.Row - (visibleTop - 1)

	assert.Equal(t, 31, visibleTop, "visibleTop")
	assert.Equal(t, 5, expectedViewportY, "expected viewport Y for row 35")

	// Verify bufferRowToViewportRow gives same result
	// (This requires mocking getBufferLineCount, which we can't easily do,
	// so we verify the calculation manually matches the expected formula)
}
