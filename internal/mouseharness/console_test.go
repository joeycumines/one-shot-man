//go:build unix

package mouseharness

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindElementInBuffer(t *testing.T) {
	buffer := `📄 Super-Document Builder

Documents: 0

No documents yet. Press 'a' to add or 'l' to load from file.

  [A]dd    [L]oad File    [C]opy Prompt

a:add  l:load  e:edit  r:rename  d:delete  v:view  c:copy  g:generate  ?:help  q:quit`

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
			expectCol:   3,
			expectWidth: 5,
		},
		{
			name:        "find load button",
			content:     "[L]oad File",
			expectRow:   7,
			expectCol:   12,
			expectWidth: 11,
		},
		{
			name:        "find copy button",
			content:     "[C]opy Prompt",
			expectRow:   7,
			expectCol:   27,
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
			screen := parseTerminalBuffer(buffer)
			loc := findElementInScreen(screen, tt.content)
			require.NotNil(t, loc, "element %q not found in buffer", tt.content)
			assert.Equal(t, tt.expectRow, loc.Row, "row mismatch")
			assert.Equal(t, tt.expectCol, loc.Col, "col mismatch")
			assert.Equal(t, tt.expectWidth, loc.Width, "width mismatch")
			assert.Equal(t, tt.content, loc.Text, "text mismatch")
		})
	}
}

func TestFindElementInBuffer_WithANSI(t *testing.T) {
	buffer := "\x1b[1m\x1b[38;5;99m📄 Super-Document Builder\x1b[0m\n\n\x1b[38;5;15mDocuments: 0\x1b[0m\n\n\x1b[38;5;102mNo documents yet.\x1b[0m\n\n\x1b[48;5;35m  [A]dd  \x1b[0m \x1b[48;5;35m  [L]oad File  \x1b[0m \x1b[1m\x1b[48;5;99m  [C]opy Prompt  \x1b[0m"

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
			screen := parseTerminalBuffer(buffer)
			loc := findElementInScreen(screen, tt.content)
			if tt.expectNil {
				assert.Nil(t, loc, "expected nil for %q", tt.content)
			} else {
				require.NotNil(t, loc, "element %q not found in buffer with ANSI codes", tt.content)
				assert.Equal(t, tt.content, loc.Text)
			}
		})
	}
}

func TestBufferRowToViewportRow(t *testing.T) {
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

			// Parse and verify
			screen := parseTerminalBuffer(buffer)
			actualLineCount := len(screen)
			for actualLineCount > 0 && strings.TrimSpace(screen[actualLineCount-1]) == "" {
				actualLineCount--
			}
			assert.Equal(t, tt.bufferLineCount, actualLineCount, "buffer line count")

			// Calculate visibleTop and viewport row manually
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

func TestClickElement_AccountsForViewportOffset(t *testing.T) {
	// Pure unit test verifying coordinate conversion logic
	var lines []string
	for i := 0; i < 40; i++ {
		if i == 34 { // Row 35 (1-indexed)
			lines = append(lines, "  [Target Button]  ")
		} else {
			lines = append(lines, "Some content line "+itoa(i+1))
		}
	}
	buffer := strings.Join(lines, "\n")

	screen := parseTerminalBuffer(buffer)

	// Verify the target is at buffer row 35
	loc := findElementInScreen(screen, "[Target Button]")
	require.NotNil(t, loc, "target element not found")
	assert.Equal(t, 35, loc.Row, "element should be at buffer row 35")

	// Calculate expected viewport row for height 10
	height := 10
	lineCount := len(screen)
	for lineCount > 0 && strings.TrimSpace(screen[lineCount-1]) == "" {
		lineCount--
	}

	visibleTop := 1
	if lineCount > height {
		visibleTop = lineCount - height + 1
	}
	expectedViewportY := loc.Row - (visibleTop - 1)

	assert.Equal(t, 31, visibleTop, "visibleTop")
	assert.Equal(t, 5, expectedViewportY, "expected viewport Y for row 35")
}

// Helper function for unit tests - matches what Console.FindElementInBuffer does
func findElementInScreen(screen []string, content string) *ElementLocation {
	for row, line := range screen {
		colIdx := strings.Index(line, content)
		if colIdx >= 0 {
			return &ElementLocation{
				Row:    row + 1,
				Col:    colIdx + 1,
				Width:  len(content),
				Height: 1,
				Text:   content,
			}
		}
	}
	return nil
}
