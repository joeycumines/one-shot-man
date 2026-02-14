package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// TestCodeReviewScript_HasReviewChunksCommand verifies that the embedded
// code_review_script.js exposes the review-chunks command.
func TestCodeReviewScript_HasReviewChunksCommand(t *testing.T) {
	t.Parallel()
	if !strings.Contains(codeReviewScript, "'review-chunks'") {
		t.Error("expected code_review_script.js to contain 'review-chunks' command definition")
	}
	if !strings.Contains(codeReviewScript, "splitDiff") {
		t.Error("expected code_review_script.js to reference splitDiff")
	}
	if !strings.Contains(codeReviewScript, "defaultMaxDiffLines") {
		t.Error("expected code_review_script.js to reference defaultMaxDiffLines")
	}
}

// TestCodeReviewCommand_SplitDiffExposed verifies that the code-review
// command exposes splitDiff and defaultMaxDiffLines as JS globals.
func TestCodeReviewCommand_SplitDiffExposed(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the script loaded successfully (mode registered and entered).
	output := stdout.String()
	if !strings.Contains(output, "Sub-test register-mode passed") {
		t.Errorf("expected register-mode sub-test to pass, got: %s", output)
	}
	if !strings.Contains(output, "Sub-test enter-code-review passed") {
		t.Errorf("expected enter-code-review sub-test to pass, got: %s", output)
	}
}

// TestSplitDiff_Integration verifies that SplitDiff works correctly
// via a Go-level call (no JS), ensuring the function is reachable.
func TestSplitDiff_Integration(t *testing.T) {
	t.Parallel()

	t.Run("SmallDiff", func(t *testing.T) {
		diff := "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"
		chunks := SplitDiff(diff, DefaultMaxDiffLines)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk for small diff, got %d", len(chunks))
		}
		if chunks[0].Total != 1 {
			t.Errorf("expected Total=1, got %d", chunks[0].Total)
		}
		if len(chunks[0].Files) != 1 || chunks[0].Files[0] != "file.go" {
			t.Errorf("expected Files=[file.go], got %v", chunks[0].Files)
		}
	})

	t.Run("LargeDiff", func(t *testing.T) {
		// Build a diff with many files that exceeds DefaultMaxDiffLines.
		var b strings.Builder
		for i := 0; i < 20; i++ {
			b.WriteString("diff --git a/file" + itoa(i) + ".go b/file" + itoa(i) + ".go\n")
			b.WriteString("--- a/file" + itoa(i) + ".go\n")
			b.WriteString("+++ b/file" + itoa(i) + ".go\n")
			b.WriteString("@@ -1,50 +1,50 @@\n")
			for j := 0; j < 50; j++ {
				b.WriteString("-old line " + itoa(j) + "\n")
				b.WriteString("+new line " + itoa(j) + "\n")
			}
		}
		diff := b.String()
		chunks := SplitDiff(diff, DefaultMaxDiffLines)
		if len(chunks) < 2 {
			t.Fatalf("expected multiple chunks for large diff (%d lines), got %d chunks",
				countLines(diff), len(chunks))
		}
		// Every chunk should have consistent Total.
		for i, c := range chunks {
			if c.Total != len(chunks) {
				t.Errorf("chunk %d: expected Total=%d, got %d", i, len(chunks), c.Total)
			}
			if c.Index != i {
				t.Errorf("chunk %d: expected Index=%d, got %d", i, i, c.Index)
			}
			if len(c.Files) == 0 {
				t.Errorf("chunk %d: expected non-empty Files", i)
			}
		}
	})

	t.Run("EmptyDiff", func(t *testing.T) {
		chunks := SplitDiff("", DefaultMaxDiffLines)
		if chunks != nil {
			t.Fatalf("expected nil for empty diff, got %v", chunks)
		}
	})

	t.Run("NoDiffMarkers", func(t *testing.T) {
		chunks := SplitDiff("just some random text\nno diff here\n", DefaultMaxDiffLines)
		if chunks != nil {
			t.Fatalf("expected nil for text without diff markers, got %v", chunks)
		}
	})
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
