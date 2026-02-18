package command

import (
	"strings"
	"testing"
)

// FuzzMCPBacktickFence fuzzes mcpBacktickFence to verify that the returned
// fence string is always safe for wrapping arbitrary content in a Markdown
// code block. Seeds include empty strings, various backtick runs, and mixed
// content.
func FuzzMCPBacktickFence(f *testing.F) {
	seeds := []string{
		"",
		"hello world",
		"`",
		"``",
		"```",
		"````",
		"`````",
		"```code```",
		"some ``` embedded ``` fences",
		"````code````",
		strings.Repeat("`", 100),
		"mixed ` content `` here ``` and ```` more",
		"no backticks at all",
		"\x00\xff`\n`",
		"日本語`テスト",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, content string) {
		fence := mcpBacktickFence(content)

		// Invariant 1: Result contains only backtick characters.
		for i, r := range fence {
			if r != '`' {
				t.Fatalf("fence contains non-backtick rune %q at position %d; fence=%q content=%q",
					string(r), i, fence, content)
			}
		}

		// Invariant 2: Fence length is at least 3.
		if len(fence) < 3 {
			t.Fatalf("fence length %d < 3; fence=%q content=%q", len(fence), fence, content)
		}

		// Invariant 3: Content does NOT contain a consecutive run of backticks
		// of length >= len(fence). This ensures the fence safely wraps the content.
		fenceLen := len(fence)
		run := 0
		for _, r := range content {
			if r == '`' {
				run++
				if run >= fenceLen {
					t.Fatalf("content contains backtick run of length >= %d; fence=%q content=%q",
						fenceLen, fence, content)
				}
			} else {
				run = 0
			}
		}
	})
}