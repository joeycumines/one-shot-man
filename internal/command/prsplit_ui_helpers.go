package command

import "strings"

// appendCapped appends lines and trims to max capacity.
func appendCapped(existing, newLines []string, max int) []string {
	existing = append(existing, newLines...)
	if len(existing) > max {
		existing = existing[len(existing)-max:]
	}
	return existing
}

// renderPane renders the last N lines of content to fill the pane height.
func renderPane(lines []string, height, width int) string {
	if len(lines) == 0 {
		return strings.Repeat("\n", height-1)
	}

	start := len(lines) - height
	if start < 0 {
		start = 0
	}
	visible := lines[start:]

	var b strings.Builder
	for i, line := range visible {
		if len(line) > width {
			line = line[:width]
		}
		b.WriteString(line)
		if i < len(visible)-1 {
			b.WriteByte('\n')
		}
	}

	for i := len(visible); i < height; i++ {
		b.WriteByte('\n')
	}
	return b.String()
}
