package termmux

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
)

// Default message overlay styling.
const (
	DefaultMessageFG = "#000000"
	DefaultMessageBG = "#f1c40f"
)

// MessageBarLine returns an ANSI sequence that draws a one-line highlighted
// message bar at the given 1-based terminal row. The text is truncated or padded
// to cols. An empty text or non-positive row/cols returns an empty string.
func MessageBarLine(text string, row, cols int) string {
	if text == "" || row <= 0 || cols <= 0 {
		return ""
	}
	truncated := text
	if len(truncated) > cols {
		truncated = truncated[:cols]
	}
	styled := lipgloss.NewStyle().
		Width(cols).
		MaxWidth(cols).
		Background(lipgloss.Color(DefaultMessageBG)).
		Foreground(lipgloss.Color(DefaultMessageFG)).
		Render(truncated)

	var b strings.Builder
	b.WriteString("\x1b7")
	fmt.Fprintf(&b, "\x1b[%d;1H", row)
	b.WriteString("\x1b[2K")
	b.WriteString(styled)
	b.WriteString("\x1b[0m")
	b.WriteString("\x1b8")
	return b.String()
}

// RenderMessageBar writes a one-line highlighted message bar to w.
func RenderMessageBar(w io.Writer, text string, row, cols int) error {
	line := MessageBarLine(text, row, cols)
	if line == "" {
		return nil
	}
	_, err := io.WriteString(w, line)
	return err
}
