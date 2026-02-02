//go:build unix

package mouseharness

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// parseTerminalBuffer simulates terminal output and returns the final screen state.
// It handles cursor positioning, line feeds, carriage returns, and ANSI sequences.
func parseTerminalBuffer(buffer string) []string {
	// Initialize a virtual screen (grows as needed)
	const initialRows = 30
	const initialCols = 100
	screen := make([][]byte, initialRows)
	for i := range screen {
		screen[i] = make([]byte, initialCols)
		for j := range screen[i] {
			screen[i][j] = ' '
		}
	}

	cursorRow := 0
	cursorCol := 0

	i := 0
	for i < len(buffer) {
		switch buffer[i] {
		case '\x1b': // Escape sequence
			i++
			if i >= len(buffer) {
				break
			}
			if buffer[i] == '[' {
				// CSI sequence
				i++
				// Parse parameters
				params := ""
				for i < len(buffer) && (buffer[i] >= '0' && buffer[i] <= '9' || buffer[i] == ';' || buffer[i] == '?') {
					params += string(buffer[i])
					i++
				}
				if i < len(buffer) {
					cmd := buffer[i]
					i++
					switch cmd {
					case 'H', 'f': // Cursor position
						row, col := 1, 1
						if params != "" {
							parts := strings.Split(params, ";")
							if len(parts) >= 1 && parts[0] != "" {
								if n, err := strconv.Atoi(parts[0]); err == nil {
									row = n
								}
							}
							if len(parts) >= 2 && parts[1] != "" {
								if n, err := strconv.Atoi(parts[1]); err == nil {
									col = n
								}
							}
						}
						cursorRow = row - 1 // Convert to 0-indexed
						cursorCol = col - 1
					case 'J': // Erase in Display
						n := 0
						if params != "" {
							if v, err := strconv.Atoi(params); err == nil {
								n = v
							}
						}
						switch n {
						case 0: // Clear from cursor to end of screen
							if cursorRow < len(screen) {
								for c := cursorCol; c < len(screen[cursorRow]); c++ {
									screen[cursorRow][c] = ' '
								}
							}
							for r := cursorRow + 1; r < len(screen); r++ {
								for c := range screen[r] {
									screen[r][c] = ' '
								}
							}
						case 1: // Clear from beginning of screen to cursor
							for r := 0; r < cursorRow; r++ {
								for c := range screen[r] {
									screen[r][c] = ' '
								}
							}
							if cursorRow < len(screen) {
								for c := 0; c <= cursorCol && c < len(screen[cursorRow]); c++ {
									screen[cursorRow][c] = ' '
								}
							}
						case 2: // Clear entire screen
							for r := range screen {
								for c := range screen[r] {
									screen[r][c] = ' '
								}
							}
						}
					case 'K': // Erase in Line
						if cursorRow < len(screen) {
							for c := cursorCol; c < len(screen[cursorRow]); c++ {
								screen[cursorRow][c] = ' '
							}
						}
					case 'A': // Cursor Up
						n := 1
						if params != "" {
							if v, err := strconv.Atoi(params); err == nil {
								n = v
							}
						}
						cursorRow -= n
						if cursorRow < 0 {
							cursorRow = 0
						}
					case 'B': // Cursor Down
						n := 1
						if params != "" {
							if v, err := strconv.Atoi(params); err == nil {
								n = v
							}
						}
						cursorRow += n
					case 'C': // Cursor Forward
						n := 1
						if params != "" {
							if v, err := strconv.Atoi(params); err == nil {
								n = v
							}
						}
						cursorCol += n
					case 'D': // Cursor Back
						n := 1
						if params != "" {
							if v, err := strconv.Atoi(params); err == nil {
								n = v
							}
						}
						cursorCol -= n
						if cursorCol < 0 {
							cursorCol = 0
						}
					case 'm': // SGR (colors, styles) - ignore
					case 'h': // Set mode
						// Handle alt screen switch: ?1049h or ?47h
						if strings.Contains(params, "1049") || strings.Contains(params, "47") {
							// Alt screen: clear the buffer and reset cursor
							for r := range screen {
								for c := range screen[r] {
									screen[r][c] = ' '
								}
							}
							cursorRow = 0
							cursorCol = 0
						}
					case 'l': // Reset mode - ignore
					default:
						// Other sequences - ignore
					}
				}
			} else if buffer[i] == ']' {
				// OSC sequence - skip until BEL or ST
				i++
				for i < len(buffer) {
					if buffer[i] == '\x07' {
						i++
						break
					}
					if buffer[i] == '\x1b' && i+1 < len(buffer) && buffer[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			} else {
				// Other escape sequences
				i++
			}
		case '\r': // Carriage return
			cursorCol = 0
			i++
		case '\n': // Line feed
			cursorRow++
			cursorCol = 0 // Also reset column for text files without explicit \r
			i++
		case '\t': // Tab
			cursorCol = ((cursorCol / 8) + 1) * 8
			i++
		case '\b': // Backspace
			if cursorCol > 0 {
				cursorCol--
			}
			i++
		default:
			// Regular character - write to screen
			if buffer[i] >= 32 && buffer[i] < 127 {
				// Ensure screen is large enough
				for cursorRow >= len(screen) {
					newRow := make([]byte, initialCols)
					for j := range newRow {
						newRow[j] = ' '
					}
					screen = append(screen, newRow)
				}
				for cursorCol >= len(screen[cursorRow]) {
					screen[cursorRow] = append(screen[cursorRow], ' ')
				}
				screen[cursorRow][cursorCol] = buffer[i]
				cursorCol++
			} else if buffer[i] >= 0x80 {
				// UTF-8 multi-byte character - handle properly
				r, size := utf8.DecodeRuneInString(buffer[i:])
				if r != utf8.RuneError {
					for cursorRow >= len(screen) {
						newRow := make([]byte, initialCols)
						for j := range newRow {
							newRow[j] = ' '
						}
						screen = append(screen, newRow)
					}
					// Place a placeholder for multi-byte characters
					if cursorCol < len(screen[cursorRow]) {
						screen[cursorRow][cursorCol] = '*' // Placeholder
					}
					cursorCol++
					i += size - 1 // -1 because we'll i++ at the end
				}
			}
			i++
		}
	}

	// Convert screen to string slice, trimming trailing spaces
	result := make([]string, len(screen))
	for i, row := range screen {
		result[i] = strings.TrimRight(string(row), " ")
	}
	return result
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Start of escape sequence
			i++
			if i >= len(s) {
				break
			}
			switch s[i] {
			case '[': // CSI sequence
				i++
				// Skip until we find a terminating character (@ through ~)
				for i < len(s) && !isCSITerminator(s[i]) {
					i++
				}
				if i < len(s) {
					i++ // Skip the terminator
				}
			case ']': // OSC sequence
				i++
				// Skip until BEL (\x07) or ST (ESC \)
				for i < len(s) {
					if s[i] == '\x07' {
						i++
						break
					}
					if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			case '(', ')': // Character set designation
				i++
				if i < len(s) {
					i++ // Skip the character set designator
				}
			default:
				// Unknown escape, skip one character
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// isCSITerminator returns true if the byte is a CSI sequence terminator (@ through ~)
func isCSITerminator(b byte) bool {
	return b >= 0x40 && b <= 0x7E
}

// ParseTerminalBuffer is the exported version of parseTerminalBuffer.
func ParseTerminalBuffer(buffer string) []string {
	return parseTerminalBuffer(buffer)
}

// StripANSI is the exported version of stripANSI.
func StripANSI(s string) string {
	return stripANSI(s)
}

// IsCSITerminator is the exported version of isCSITerminator.
func IsCSITerminator(b byte) bool {
	return isCSITerminator(b)
}

// getBufferLineCount returns the number of non-empty lines in the terminal buffer.
func (c *Console) getBufferLineCount() int {
	buffer := c.cp.String()
	screen := parseTerminalBuffer(buffer)
	// Count non-empty lines from the end
	count := len(screen)
	for count > 0 && strings.TrimSpace(screen[count-1]) == "" {
		count--
	}
	return count
}

// getVisibleTop returns the first buffer row that is visible in the viewport.
// When content is taller than the terminal, this is bufferRows - height + 1.
// Returns 1 (1-indexed) if content fits within the terminal.
func (c *Console) getVisibleTop() int {
	totalRows := c.getBufferLineCount()
	if totalRows <= c.height {
		return 1 // All content visible
	}
	return totalRows - c.height + 1
}

// bufferRowToViewportRow converts an absolute buffer row (1-indexed) to a
// viewport-relative row (1-indexed). SGR mouse events use viewport-relative
// coordinates, not absolute buffer positions.
func (c *Console) bufferRowToViewportRow(bufferRow int) int {
	visibleTop := c.getVisibleTop()
	viewportY := bufferRow - (visibleTop - 1)

	// Clamp to valid viewport range
	if viewportY < 1 {
		viewportY = 1
	}
	if viewportY > c.height {
		viewportY = c.height
	}
	return viewportY
}

// GetBuffer returns the current terminal buffer as a parsed screen.
// This is useful for tests and verification to inspect the current state
// of the terminal after interactions.
//
// Returns a slice of strings representing each row of the terminal screen,
// with ANSI escape codes stripped. Rows are 1-indexed for consistency with
// other coordinate systems in this package.
//
// Example:
//
//	screen := console.GetBuffer()
//	if len(screen) == 0 {
//	    t.Error("expected screen content")
//	}
//	if !strings.Contains(screen[0], "expected text") {
//	    t.Error("expected text not found in first row")
//	}
func (c *Console) GetBuffer() []string {
	if c.cp == nil {
		return nil
	}
	return parseTerminalBuffer(c.cp.String())
}

// GetBufferRaw returns the raw terminal buffer string without parsing.
// This returns the raw escape sequences and text as received from the PTY.
func (c *Console) GetBufferRaw() string {
	if c.cp == nil {
		return ""
	}
	return c.cp.String()
}
