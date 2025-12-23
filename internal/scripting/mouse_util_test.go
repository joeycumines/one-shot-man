//go:build unix

package scripting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/joeycumines/go-prompt/termtest"
)

// MouseTestAPI provides reusable mouse interaction utilities for integration tests.
// It dynamically parses terminal buffer state to locate UI elements and generates
// proper SGR mouse escape sequences for clicking.
//
// IMPORTANT: SGR mouse coordinates are viewport-relative (1-indexed from the
// visible top of the terminal window). When content is taller than the terminal
// and scrolled, this API converts absolute buffer rows to viewport-relative rows.
type MouseTestAPI struct {
	cp     *termtest.Console
	t      *testing.T
	height int // Terminal height in rows (default 24)
}

// NewMouseTestAPI creates a new MouseTestAPI for the given console.
// Uses default terminal height of 24 rows.
func NewMouseTestAPI(t *testing.T, cp *termtest.Console) *MouseTestAPI {
	return &MouseTestAPI{cp: cp, t: t, height: 24}
}

// NewMouseTestAPIWithSize creates a MouseTestAPI with explicit terminal dimensions.
// Height is used to convert buffer rows to viewport-relative rows for mouse events.
func NewMouseTestAPIWithSize(t *testing.T, cp *termtest.Console, height int) *MouseTestAPI {
	if height <= 0 {
		height = 24 // Default
	}
	return &MouseTestAPI{cp: cp, t: t, height: height}
}

// SetHeight sets the terminal height for viewport calculations.
func (m *MouseTestAPI) SetHeight(height int) {
	if height > 0 {
		m.height = height
	}
}

// ElementLocation represents the location of a UI element in the terminal buffer.
type ElementLocation struct {
	Row    int    // 1-indexed row
	Col    int    // 1-indexed column
	Width  int    // Width of the element
	Height int    // Height of the element (usually 1)
	Text   string // The matched text
}

// FindElement searches the terminal buffer for the given content string.
// Returns the location of the first occurrence IN THE CURRENT SCREEN STATE, or nil if not found.
// Strips ANSI escape codes before searching to match visible text.
func (m *MouseTestAPI) FindElement(content string) *ElementLocation {
	m.t.Helper()
	buffer := m.cp.String()
	return m.FindElementInBuffer(buffer, content)
}

// FindElementInBuffer searches a specific buffer for the given content string.
// It uses a simple virtual terminal emulator to accurately track cursor position
// and screen content, handling ANSI escape sequences properly.
func (m *MouseTestAPI) FindElementInBuffer(buffer, content string) *ElementLocation {
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

// parseTerminalBuffer simulates terminal output and returns the final screen state.
// It handles cursor positioning, line feeds, carriage returns, and ANSI sequences.
func parseTerminalBuffer(buffer string) []string {
	// Initialize a virtual screen (24 rows x 80 cols default, but grows as needed)
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

	// Debug: track first few character writes
	// DISABLED: Debug logging causes excessive output
	charWrites := 0
	debugLog := func(_ string, _ ...interface{}) {
		// DISABLED
	}
	// Always print for important mode changes
	alwaysLog := func(_ string, _ ...interface{}) {
		// DISABLED
	}

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
							// Clear rest of current line
							if cursorRow < len(screen) {
								for c := cursorCol; c < len(screen[cursorRow]); c++ {
									screen[cursorRow][c] = ' '
								}
							}
							// Clear all lines below
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
						// 0K or K = clear from cursor to end of line
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
						alwaysLog("CSI h with params=%q", params)
						// Handle alt screen switch: ?1049h or ?47h
						if strings.Contains(params, "1049") || strings.Contains(params, "47") {
							// Alt screen: clear the buffer and reset cursor
							alwaysLog("ALT SCREEN ENABLE: clearing screen and resetting cursor")
							for r := range screen {
								for c := range screen[r] {
									screen[r][c] = ' '
								}
							}
							cursorRow = 0
							cursorCol = 0
							charWrites = 0 // Reset debug counter to trace post-alt-screen writes
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
				charWrites++
				if charWrites <= 10 {
					debugLog("WRITE char %q at row=%d col=%d", buffer[i], cursorRow, cursorCol)
				}
				screen[cursorRow][cursorCol] = buffer[i]
				cursorCol++
			} else if buffer[i] >= 0x80 {
				// UTF-8 multi-byte character - handle properly
				r, size := utf8.DecodeRuneInString(buffer[i:])
				if r != utf8.RuneError {
					// For simplicity, just advance cursor by 1 for emoji/unicode
					// (terminal width is actually variable, but this is good enough for testing)
					for cursorRow >= len(screen) {
						newRow := make([]byte, initialCols)
						for j := range newRow {
							newRow[j] = ' '
						}
						screen = append(screen, newRow)
					}
					// Place a placeholder for multi-byte characters
					// The actual emoji takes 1-2 columns depending on terminal
					charWrites++
					if charWrites <= 10 {
						debugLog("WRITE utf8 rune %q (size=%d) at row=%d col=%d", r, size, cursorRow, cursorCol)
					}
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
// This is necessary because the terminal buffer contains raw escape codes.
func stripANSI(s string) string {
	// Simple state machine to strip ANSI escape sequences
	// Handles CSI (ESC [) and OSC (ESC ]) sequences
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

// getBufferLineCount returns the number of lines in the terminal buffer.
func (m *MouseTestAPI) getBufferLineCount() int {
	buffer := m.cp.String()
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
func (m *MouseTestAPI) getVisibleTop() int {
	totalRows := m.getBufferLineCount()
	if totalRows <= m.height {
		return 1 // All content visible
	}
	return totalRows - m.height + 1
}

// bufferRowToViewportRow converts an absolute buffer row (1-indexed) to a
// viewport-relative row (1-indexed). SGR mouse events use viewport-relative
// coordinates, not absolute buffer positions.
//
// When content is scrolled (buffer taller than terminal), the conversion is:
//
//	viewportY = bufferRow - (visibleTop - 1)
//
// The result is clamped to [1, height] to ensure valid SGR coordinates.
func (m *MouseTestAPI) bufferRowToViewportRow(bufferRow int) int {
	visibleTop := m.getVisibleTop()
	viewportY := bufferRow - (visibleTop - 1)

	// Clamp to valid viewport range
	if viewportY < 1 {
		viewportY = 1
	}
	if viewportY > m.height {
		viewportY = m.height
	}
	return viewportY
}

// ClickElement locates an element by its visible text content and clicks on it.
// It dynamically reads the terminal buffer, finds the element, calculates the
// center coordinates, and sends SGR mouse press/release events.
//
// IMPORTANT: This method converts absolute buffer rows to viewport-relative
// rows for the SGR mouse event. This is required because terminals report
// mouse coordinates relative to the visible viewport, not the full buffer.
//
// Returns an error if the element cannot be found within the timeout.
func (m *MouseTestAPI) ClickElement(ctx context.Context, content string, timeout time.Duration) error {
	m.t.Helper()

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for the element to appear
	var loc *ElementLocation
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("element %q not found within timeout; buffer: %q", content, m.cp.String())
		case <-ticker.C:
			loc = m.FindElement(content)
			if loc != nil {
				goto found
			}
		}
	}

found:
	// Calculate center of element (buffer coordinates)
	centerX := loc.Col + loc.Width/2
	bufferY := loc.Row

	// Check if row 0 is empty in the parsed buffer - this indicates a render issue
	// where the title line was not sent to the terminal but exists in the zone system.
	// When this happens, we need to add 1 to the click Y coordinate to compensate.
	screen := parseTerminalBuffer(m.cp.String())
	row0Empty := len(screen) > 0 && strings.TrimSpace(screen[0]) == ""
	if row0Empty {
		bufferY++ // Compensate for missing title line
		m.t.Logf("[CLICK DEBUG] Row 0 is empty, adjusting bufferY from %d to %d", bufferY-1, bufferY)
	}

	// Convert buffer row to viewport-relative row for SGR mouse event
	viewportY := m.bufferRowToViewportRow(bufferY)

	m.t.Logf("[CLICK DEBUG] ClickElement %q: loc.Row=%d (1-indexed), centerX=%d, viewportY=%d", content, loc.Row, centerX, viewportY)

	// Send mouse click using viewport-relative coordinates
	return m.ClickViewport(centerX, viewportY)
}

// Click sends a mouse click at the specified viewport-relative coordinates (1-indexed).
// Use this when you know the exact viewport position. For clicking UI elements
// by their text content, use ClickElement instead.
// It sends both press and release events using SGR extended mouse mode.
func (m *MouseTestAPI) Click(x, y int) error {
	m.t.Helper()
	return m.ClickWithButton(x, y, 0) // 0 = left button
}

// ClickViewport is an alias for Click, emphasizing that coordinates are
// viewport-relative (not buffer-absolute). SGR mouse events always use
// viewport-relative coordinates where row 1 is the top visible row.
func (m *MouseTestAPI) ClickViewport(x, y int) error {
	m.t.Helper()
	return m.Click(x, y)
}

// ClickAtBufferPosition sends a mouse click at the specified buffer-absolute
// coordinates (1-indexed). The buffer row is converted to viewport-relative
// coordinates before sending the SGR mouse event.
// This is useful when you have buffer coordinates from FindElementInBuffer.
func (m *MouseTestAPI) ClickAtBufferPosition(x, bufferY int) error {
	m.t.Helper()
	viewportY := m.bufferRowToViewportRow(bufferY)
	return m.Click(x, viewportY)
}

// ClickWithButton sends a mouse click with a specific button at the coordinates.
// Coordinates are viewport-relative (1-indexed).
// Button values: 0=left, 1=middle, 2=right
func (m *MouseTestAPI) ClickWithButton(x, y, button int) error {
	m.t.Helper()

	// SGR mouse encoding: ESC [ < Cb ; Cx ; Cy M (press) / m (release)
	// Cb = button number (0=left, 1=middle, 2=right)
	mousePress := fmt.Sprintf("\x1b[<%d;%d;%dM", button, x, y)
	mouseRelease := fmt.Sprintf("\x1b[<%d;%d;%dm", button, x, y)

	// Use WriteString for raw escape sequences (Send expects bubbletea key names)
	if _, err := m.cp.WriteString(mousePress); err != nil {
		return fmt.Errorf("failed to send mouse press: %w", err)
	}

	// Small delay between press and release for realism
	time.Sleep(30 * time.Millisecond)

	if _, err := m.cp.WriteString(mouseRelease); err != nil {
		return fmt.Errorf("failed to send mouse release: %w", err)
	}

	return nil
}

// ScrollWheel sends a mouse wheel event at the specified viewport-relative coordinates.
// Coordinates are 1-indexed. direction: "up" or "down".
// SGR mouse encoding: Button 64 = wheel up, Button 65 = wheel down.
func (m *MouseTestAPI) ScrollWheel(x, y int, direction string) error {
	m.t.Helper()

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

	if _, err := m.cp.WriteString(mouseEvent); err != nil {
		return fmt.Errorf("failed to send scroll wheel event: %w", err)
	}

	return nil
}

// ScrollWheelOnElement finds an element and sends a scroll wheel event on it.
// This is useful for testing that wheel events don't accidentally trigger button actions.
func (m *MouseTestAPI) ScrollWheelOnElement(ctx context.Context, content string, direction string, timeout time.Duration) error {
	m.t.Helper()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var loc *ElementLocation
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("element %q not found within timeout for scroll", content)
		case <-ticker.C:
			loc = m.FindElement(content)
			if loc != nil {
				goto found
			}
		}
	}

found:
	centerX := loc.Col + loc.Width/2
	viewportY := m.bufferRowToViewportRow(loc.Row)
	return m.ScrollWheel(centerX, viewportY, direction)
}

// ClickElementAndExpect clicks an element and waits for expected content to appear.
func (m *MouseTestAPI) ClickElementAndExpect(ctx context.Context, clickTarget, expectContent string, timeout time.Duration) error {
	m.t.Helper()

	snap := m.cp.Snapshot()

	if err := m.ClickElement(ctx, clickTarget, timeout/2); err != nil {
		return fmt.Errorf("failed to click %q: %w", clickTarget, err)
	}

	// Wait for expected content
	expectCtx, cancel := context.WithTimeout(ctx, timeout/2)
	defer cancel()

	if err := m.cp.Expect(expectCtx, snap, termtest.Contains(expectContent), fmt.Sprintf("wait for %q after clicking %q", expectContent, clickTarget)); err != nil {
		return fmt.Errorf("expected %q after clicking %q: %w\nBuffer: %q", expectContent, clickTarget, err, m.cp.String())
	}

	return nil
}

// RequireClickElement clicks an element and fails the test if it cannot be found.
func (m *MouseTestAPI) RequireClickElement(ctx context.Context, content string, timeout time.Duration) {
	m.t.Helper()
	if err := m.ClickElement(ctx, content, timeout); err != nil {
		m.t.Fatalf("RequireClickElement failed: %v", err)
	}
}

// RequireClick sends a click and fails the test if it cannot be sent.
func (m *MouseTestAPI) RequireClick(x, y int) {
	m.t.Helper()
	if err := m.Click(x, y); err != nil {
		m.t.Fatalf("RequireClick failed: %v", err)
	}
}

// GetElementCenter returns the center coordinates of an element if found.
func (m *MouseTestAPI) GetElementCenter(content string) (x, y int, found bool) {
	m.t.Helper()
	loc := m.FindElement(content)
	if loc == nil {
		return 0, 0, false
	}
	return loc.Col + loc.Width/2, loc.Row, true
}

// DebugBuffer prints the current buffer state with line numbers for debugging.
func (m *MouseTestAPI) DebugBuffer() {
	m.t.Helper()
	buffer := m.cp.String()
	lines := strings.Split(buffer, "\n")
	m.t.Log("=== Buffer State ===")
	for i, line := range lines {
		cleanLine := stripANSI(line)
		m.t.Logf("Line %2d: %q (clean: %q)", i+1, line, cleanLine)
	}
	m.t.Log("=== End Buffer ===")
}
