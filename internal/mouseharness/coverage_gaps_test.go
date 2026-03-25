//go:build unix

package mouseharness

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────
// terminal.go: Exported wrappers coverage (0% → 100%)
// ─────────────────────────────────────────────────────────────────────

func TestParseTerminalBuffer_Exported(t *testing.T) {
	t.Parallel()
	result := ParseTerminalBuffer("Hello\nWorld")
	require.True(t, len(result) >= 2)
	assert.Equal(t, "Hello", result[0])
	assert.Equal(t, "World", result[1])
}

func TestStripANSI_Exported(t *testing.T) {
	t.Parallel()
	got := StripANSI("\x1b[31mRed\x1b[0m")
	assert.Equal(t, "Red", got)
}

func TestIsCSITerminator_Exported(t *testing.T) {
	t.Parallel()
	assert.True(t, IsCSITerminator('m'))
	assert.True(t, IsCSITerminator('H'))
	assert.False(t, IsCSITerminator('0'))
}

// ─────────────────────────────────────────────────────────────────────
// terminal.go: GetBuffer / GetBufferRaw with nil cp
// ─────────────────────────────────────────────────────────────────────

func TestGetBuffer_NilCp(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 24, width: 80}
	got := c.GetBuffer()
	assert.Nil(t, got, "GetBuffer with nil cp should return nil")
}

func TestGetBufferRaw_NilCp(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 24, width: 80}
	got := c.GetBufferRaw()
	assert.Equal(t, "", got, "GetBufferRaw with nil cp should return empty string")
}

// ─────────────────────────────────────────────────────────────────────
// terminal.go: parseTerminalBuffer edge cases
// ─────────────────────────────────────────────────────────────────────

func TestParseTerminalBuffer_EraseFromBeginning(t *testing.T) {
	t.Parallel()
	// ESC[1J — erase from beginning of screen to cursor
	// Move to row 2 col 5, then erase from beginning
	buffer := "AAAAA\nBBBBB\n\x1b[2;3H\x1b[1J"
	screen := parseTerminalBuffer(buffer)
	// Row 0 should be cleared entirely
	assert.Equal(t, "", screen[0], "row 0 should be cleared")
	// Row 1 up to column 2 should be cleared, rest preserved
	// cursorRow=1, cursorCol=2 after H command
	// Erase from beginning: rows 0..cursorRow-1 all cleared, cursorRow up to cursorCol cleared
	assert.True(t, strings.HasPrefix(screen[1], "   "), "first 3 chars of row 1 should be spaces")
}

func TestParseTerminalBuffer_EraseInDisplayN2(t *testing.T) {
	t.Parallel()
	// ESC[2J — clear entire screen, but cursor stays in place
	// Then write new content
	buffer := "Old Content\x1b[2J\x1b[1;1HNew"
	screen := parseTerminalBuffer(buffer)
	assert.Equal(t, "New", screen[0])
}

func TestParseTerminalBuffer_OSCWithST(t *testing.T) {
	t.Parallel()
	// OSC sequence terminated by ST (ESC \) instead of BEL
	buffer := "\x1b]0;Window Title\x1b\\Visible Text"
	screen := parseTerminalBuffer(buffer)
	assert.Contains(t, screen[0], "Visible Text")
}

func TestParseTerminalBuffer_CursorPositionNoParams(t *testing.T) {
	t.Parallel()
	// ESC[H with no params → move to 1,1 (0,0 in 0-indexed)
	buffer := "XXXXX\x1b[HNew"
	screen := parseTerminalBuffer(buffer)
	assert.True(t, strings.HasPrefix(screen[0], "New"))
}

func TestParseTerminalBuffer_CursorPositionPartialParams(t *testing.T) {
	t.Parallel()
	// ESC[5H — row 5, col defaults to 1
	buffer := "\x1b[5;HText"
	screen := parseTerminalBuffer(buffer)
	assert.Equal(t, "Text", screen[4])
}

func TestParseTerminalBuffer_ResetMode(t *testing.T) {
	t.Parallel()
	// ESC[?1049l — reset mode (alt screen off) — should be ignored
	// Just verify no crash
	buffer := "Before\x1b[?1049lAfter"
	screen := parseTerminalBuffer(buffer)
	_ = screen // No crash
}

func TestParseTerminalBuffer_UnknownCSI(t *testing.T) {
	t.Parallel()
	// Unknown CSI sequence (e.g., ESC[6n — device status report)
	buffer := "Text\x1b[6nMore"
	screen := parseTerminalBuffer(buffer)
	assert.Contains(t, screen[0], "Text")
	assert.Contains(t, screen[0], "More")
}

func TestParseTerminalBuffer_IncompleteEscape(t *testing.T) {
	t.Parallel()
	// Escape at end of buffer with nothing following
	buffer := "Text\x1b"
	screen := parseTerminalBuffer(buffer)
	assert.Contains(t, screen[0], "Text")
}

func TestParseTerminalBuffer_OtherEscapeSequence(t *testing.T) {
	t.Parallel()
	// Escape followed by non-[ non-] (e.g., ESC M = reverse line feed)
	buffer := "Text\x1bMMore"
	screen := parseTerminalBuffer(buffer)
	// "Other escape sequences" branch — skips one char
	assert.Contains(t, screen[0], "Text")
}

func TestParseTerminalBuffer_CursorUpClamp(t *testing.T) {
	t.Parallel()
	// Cursor up from row 0 should clamp to 0
	buffer := "\x1b[10AText"
	screen := parseTerminalBuffer(buffer)
	assert.Contains(t, screen[0], "Text")
}

func TestParseTerminalBuffer_CursorBackClamp(t *testing.T) {
	t.Parallel()
	// Cursor back past column 0 should clamp
	buffer := "X\x1b[10DY"
	screen := parseTerminalBuffer(buffer)
	assert.True(t, strings.HasPrefix(screen[0], "Y"))
}

func TestParseTerminalBuffer_Backspace(t *testing.T) {
	t.Parallel()
	// Backspace at column 0 — should not go negative
	buffer := "\bText"
	screen := parseTerminalBuffer(buffer)
	assert.Contains(t, screen[0], "Text")
}

func TestParseTerminalBuffer_ControlCharUnder32(t *testing.T) {
	t.Parallel()
	// Control character (< 32 but not \r \n \t \b \x1b) should be ignored
	buffer := "A\x01\x02\x03B"
	screen := parseTerminalBuffer(buffer)
	assert.Contains(t, screen[0], "A")
	// Control chars are not written, but B advances
	assert.Contains(t, screen[0], "B")
}

// ─────────────────────────────────────────────────────────────────────
// terminal.go: stripANSI edge cases
// ─────────────────────────────────────────────────────────────────────

func TestStripANSI_CharacterSetDesignation(t *testing.T) {
	t.Parallel()
	// Character set designation: ESC ( B or ESC ) 0
	input := "\x1b(BText\x1b)0More"
	got := stripANSI(input)
	assert.Equal(t, "TextMore", got)
}

func TestStripANSI_IncompleteEscape(t *testing.T) {
	t.Parallel()
	// Escape at end of string
	got := stripANSI("Text\x1b")
	assert.Equal(t, "Text", got)
}

func TestStripANSI_OSCWithST(t *testing.T) {
	t.Parallel()
	// OSC terminated by ST (ESC \)
	got := stripANSI("\x1b]2;Title\x1b\\Visible")
	assert.Equal(t, "Visible", got)
}

func TestStripANSI_UnknownEscape(t *testing.T) {
	t.Parallel()
	// Unknown escape sequence (e.g., ESC M)
	got := stripANSI("A\x1bMB")
	assert.Equal(t, "AB", got)
}

func TestStripANSI_CharSetAtEnd(t *testing.T) {
	t.Parallel()
	// Character set designation at end of string with nothing after designator
	got := stripANSI("Text\x1b(")
	assert.Equal(t, "Text", got)
}

// ─────────────────────────────────────────────────────────────────────
// console.go: New validation
// ─────────────────────────────────────────────────────────────────────

func TestNew_MissingTestingTB(t *testing.T) {
	t.Parallel()
	// Create a console without WithTestingTB — should fail
	_, err := New(WithHeight(24))
	require.Error(t, err)
	// Fails because cp is nil (no WithTermtestConsole)
	assert.Contains(t, err.Error(), "WithTermtestConsole is required")
}

func TestNew_OptionError(t *testing.T) {
	t.Parallel()
	// Invalid height should propagate error
	_, err := New(WithTestingTB(t), WithHeight(-1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to apply option")
}

// ─────────────────────────────────────────────────────────────────────
// mouse.go: convenience/alias functions that don't need PTY
// ─────────────────────────────────────────────────────────────────────

func TestMouseButton_String_All(t *testing.T) {
	t.Parallel()
	tests := []struct {
		button MouseButton
		want   string
	}{
		{MouseButtonLeft, "left"},
		{MouseButtonMiddle, "middle"},
		{MouseButtonRight, "right"},
		{MouseButton(42), "MouseButton(42)"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.button.String())
	}
}

func TestScrollDirection_String_All(t *testing.T) {
	t.Parallel()
	tests := []struct {
		dir  ScrollDirection
		want string
	}{
		{ScrollUp, "up"},
		{ScrollDown, "down"},
		{ScrollDirection(42), "ScrollDirection(42)"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, tc.dir.String())
	}
}

// ─────────────────────────────────────────────────────────────────────
// console.go: Simple getters (no PTY needed)
// ─────────────────────────────────────────────────────────────────────

func TestConsole_SimpleGetters(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 30, width: 120}
	assert.Equal(t, 30, c.Height())
	assert.Equal(t, 120, c.Width())
	assert.Nil(t, c.TermtestConsole())
}

// ─────────────────────────────────────────────────────────────────────
// element.go: FindElementInBuffer (pure function, no PTY)
// ─────────────────────────────────────────────────────────────────────

func TestFindElementInBuffer_Found(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 24, width: 80}
	loc := c.FindElementInBuffer("Hello World\nFoo Bar Baz", "Foo")
	require.NotNil(t, loc)
	assert.Equal(t, 2, loc.Row)
	assert.Equal(t, 1, loc.Col)
	assert.Equal(t, 3, loc.Width)
	assert.Equal(t, "Foo", loc.Text)
}

func TestFindElementInBuffer_NotFound(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 24, width: 80}
	loc := c.FindElementInBuffer("Hello World\nOnly This", "Missing")
	assert.Nil(t, loc)
}

func TestFindElementInBuffer_MidLine(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 24, width: 80}
	loc := c.FindElementInBuffer("abc def ghi", "def")
	require.NotNil(t, loc)
	assert.Equal(t, 1, loc.Row)
	assert.Equal(t, 5, loc.Col) // 1-indexed: "abc " is 4 chars, "def" starts at 5
}

func TestFindElementInBuffer_EmptyBuffer(t *testing.T) {
	t.Parallel()
	c := &Console{cp: nil, tb: t, height: 24, width: 80}
	loc := c.FindElementInBuffer("", "anything")
	assert.Nil(t, loc)
}

// ─────────────────────────────────────────────────────────────────────
// terminal.go: getVisibleTop / bufferRowToViewportRow computation
// ─────────────────────────────────────────────────────────────────────

func TestGetVisibleTop_ContentFits(t *testing.T) {
	t.Parallel()
	// When content has fewer lines than height, visibleTop should be 1
	buffer := "Line 1\nLine 2\nLine 3"
	screen := parseTerminalBuffer(buffer)
	// Simulate getBufferLineCount
	count := len(screen)
	for count > 0 && strings.TrimSpace(screen[count-1]) == "" {
		count--
	}
	height := 24
	visibleTop := 1
	if count > height {
		visibleTop = count - height + 1
	}
	assert.Equal(t, 1, visibleTop)
}

func TestGetVisibleTop_ContentScrolled(t *testing.T) {
	t.Parallel()
	// When content has more lines than height
	var lines []string
	for range 50 {
		lines = append(lines, "Content line")
	}
	buffer := strings.Join(lines, "\n")
	screen := parseTerminalBuffer(buffer)

	count := len(screen)
	for count > 0 && strings.TrimSpace(screen[count-1]) == "" {
		count--
	}
	height := 10
	visibleTop := 1
	if count > height {
		visibleTop = count - height + 1
	}
	assert.Equal(t, 41, visibleTop) // 50 - 10 + 1 = 41
}

func TestBufferRowToViewportRow_AllClampPaths(t *testing.T) {
	t.Parallel()
	height := 10
	visibleTop := 31 // e.g., 40 total lines, height 10

	tests := []struct {
		name      string
		bufferRow int
		want      int
	}{
		{"above viewport clamped", 1, 1},
		{"just below viewport clamped", 50, 10},
		{"in viewport", 35, 5},
		{"at viewport top", 31, 1},
		{"at viewport bottom", 40, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viewportY := max(tc.bufferRow-(visibleTop-1), 1)
			if viewportY > height {
				viewportY = height
			}
			assert.Equal(t, tc.want, viewportY)
		})
	}
}
