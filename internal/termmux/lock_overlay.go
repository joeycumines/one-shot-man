package termmux

import (
	"fmt"
	"io"
	"strings"
)

const (
	unlockPrompt   = "Session locked — enter password:"
	unlockMaskChar = '*'
)

// UnlockPromptString returns the lock prompt content as centered lines
// suitable for overlaying on a terminal of the given dimensions.
func UnlockPromptString(maskLen int, message string, rows, cols int) string {
	var prompt strings.Builder
	prompt.WriteString(unlockPrompt)
	if maskLen > 0 {
		prompt.WriteString(" ")
		prompt.WriteString(strings.Repeat(string(unlockMaskChar), maskLen))
	}

	lines := []string{prompt.String()}
	if message != "" {
		msg := message
		if len(msg) > cols-2 {
			msg = msg[:max(0, cols-2)]
		}
		lines = append(lines, msg)
	}

	return padCenter(lines, rows, cols)
}

// RenderUnlockPrompt clears the screen and draws the centered lock prompt
// with a masked password and optional status message.
func RenderUnlockPrompt(w io.Writer, maskLen int, message string, rows, cols int) error {
	content := UnlockPromptString(maskLen, message, rows, cols)
	_, err := fmt.Fprintf(w, "\x1b[2J\x1b[H\x1b[7m%s\x1b[0m", content)
	return err
}

func padCenter(lines []string, rows, cols int) string {
	var b strings.Builder
	top := max((rows-len(lines))/2, 0)
	for range top {
		b.WriteByte('\n')
	}
	for _, line := range lines {
		if len(line) > cols {
			line = line[:cols]
		}
		pad := max((cols-len(line))/2, 0)
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
