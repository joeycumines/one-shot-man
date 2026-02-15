package command

import (
	"testing"
)

// FuzzSplitDiff ensures the diff splitter does not panic on arbitrary input
// and produces structurally valid output.
func FuzzSplitDiff(f *testing.F) {
	// Seed with valid diff patterns
	f.Add("", 500)
	f.Add("diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,3 @@\n-old\n+new\n context\n", 500)
	f.Add("diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-x\n+y\ndiff --git a/b.go b/b.go\n@@ -1 +1 @@\n-a\n+b\n", 10)
	// Edge cases
	f.Add("not a diff at all", 100)
	f.Add("diff --git a/file.go b/file.go\n", 1)
	f.Add("@@ -1 +1 @@\n-x\n+y\n", 50)                         // hunk without file header
	f.Add("diff --git a/file.go b/file.go\n@@ -1 +1 @@\n", 50) // hunk header only
	f.Add("\n\n\n", 10)
	f.Add("diff --git a/f b/f\n+added line\n", 1) // maxLines = 1

	f.Fuzz(func(t *testing.T, diff string, maxLines int) {
		// Must not panic
		chunks := SplitDiff(diff, maxLines)

		// Validate structural properties
		if diff == "" && len(chunks) != 0 {
			t.Fatal("empty diff should produce no chunks")
		}

		for i, chunk := range chunks {
			if chunk.Index != i {
				t.Fatalf("chunk %d has Index %d", i, chunk.Index)
			}
			if chunk.Total != len(chunks) {
				t.Fatalf("chunk %d has Total %d, expected %d", i, chunk.Total, len(chunks))
			}
			if chunk.Lines < 0 {
				t.Fatalf("chunk %d has negative Lines: %d", i, chunk.Lines)
			}
			if chunk.Content == "" && chunk.Lines != 0 {
				t.Fatalf("chunk %d has empty Content but Lines=%d", i, chunk.Lines)
			}
		}
	})
}
