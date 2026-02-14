package command

import (
	"fmt"
	"strings"
	"testing"
)

// ── helpers ──────────────────────────────────────────────────────────────

// makeFileDiff generates a realistic unified diff section for a single file
// consisting of numHunks hunks each containing linesPerHunk body lines.
// The total line count for the returned string is:
//
//	header (4 lines) + numHunks * (1 hunk-header + linesPerHunk body)
func makeFileDiff(name string, numHunks, linesPerHunk int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", name, name)
	fmt.Fprintf(&b, "index 1234567..abcdefg 100644\n")
	fmt.Fprintf(&b, "--- a/%s\n", name)
	fmt.Fprintf(&b, "+++ b/%s\n", name)
	for h := 0; h < numHunks; h++ {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h*10+1, linesPerHunk, h*10+1, linesPerHunk)
		for l := 0; l < linesPerHunk; l++ {
			fmt.Fprintf(&b, "+added line %d in hunk %d of %s\n", l, h, name)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// diffLineCount returns the number of newline-delimited lines in s,
// matching the behavior of countLines (non-empty string without trailing
// newline still counts as one final line).
func diffLineCount(s string) int { return countLines(s) }

// ── tests ────────────────────────────────────────────────────────────────

func TestSplitDiffSingleSmallFile(t *testing.T) {
	t.Parallel()

	diff := makeFileDiff("main.go", 1, 5)
	chunks := SplitDiff(diff, 500)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != diff {
		t.Errorf("chunk content should match input diff")
	}
	if len(chunks[0].Files) != 1 || chunks[0].Files[0] != "main.go" {
		t.Errorf("expected Files=[main.go], got %v", chunks[0].Files)
	}
}

func TestSplitDiffMultipleFiles(t *testing.T) {
	t.Parallel()

	// Three small files (~10 lines each). With maxLines=25 the first two
	// should fit in one chunk and the third starts a second.
	f1 := makeFileDiff("a.go", 1, 5)
	f2 := makeFileDiff("b.go", 1, 5)
	f3 := makeFileDiff("c.go", 1, 5)
	diff := f1 + "\n" + f2 + "\n" + f3

	chunks := SplitDiff(diff, 25)

	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for 3 files at maxLines=25, got %d", len(chunks))
	}

	// All file names should appear exactly once across chunks.
	seen := make(map[string]bool)
	for _, ch := range chunks {
		for _, f := range ch.Files {
			if seen[f] {
				t.Errorf("file %s appears in more than one chunk", f)
			}
			seen[f] = true
		}
	}
	for _, want := range []string{"a.go", "b.go", "c.go"} {
		if !seen[want] {
			t.Errorf("file %s missing from chunks", want)
		}
	}
}

func TestSplitDiffLargeFile(t *testing.T) {
	t.Parallel()

	// One file with 10 hunks of 60 lines each → ~604 lines total
	// (4 header + 10*(1+60)). maxLines=200 should cause hunk-level splitting.
	diff := makeFileDiff("big.go", 10, 60)
	chunks := SplitDiff(diff, 200)

	if len(chunks) < 2 {
		t.Fatalf("expected large file to be split into >=2 chunks, got %d", len(chunks))
	}

	// Every chunk should reference only "big.go".
	for i, ch := range chunks {
		if len(ch.Files) != 1 || ch.Files[0] != "big.go" {
			t.Errorf("chunk %d: unexpected Files %v", i, ch.Files)
		}
		// Every chunk should contain the file header.
		if !strings.Contains(ch.Content, "diff --git a/big.go b/big.go") {
			t.Errorf("chunk %d missing file header", i)
		}
		// Every chunk should contain at least one hunk header.
		if !strings.Contains(ch.Content, "@@") {
			t.Errorf("chunk %d missing hunk header", i)
		}
	}
}

func TestSplitDiffEmptyDiff(t *testing.T) {
	t.Parallel()

	chunks := SplitDiff("", 500)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(chunks))
	}

	// Whitespace-only also yields empty.
	chunks = SplitDiff("\n\n\n", 500)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for whitespace-only input, got %d", len(chunks))
	}
}

func TestSplitDiffRespectFileBoundary(t *testing.T) {
	t.Parallel()

	// Two files of ~11 lines each. maxLines=15 means each must be in its own
	// chunk (they cannot share).
	f1 := makeFileDiff("foo.go", 1, 6)
	f2 := makeFileDiff("bar.go", 1, 6)
	diff := f1 + "\n" + f2

	chunks := SplitDiff(diff, 15)

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (one per file), got %d", len(chunks))
	}
	if chunks[0].Files[0] != "foo.go" {
		t.Errorf("chunk 0: expected foo.go, got %v", chunks[0].Files)
	}
	if chunks[1].Files[0] != "bar.go" {
		t.Errorf("chunk 1: expected bar.go, got %v", chunks[1].Files)
	}
}

func TestSplitDiffChunkMetadata(t *testing.T) {
	t.Parallel()

	// Three files, maxLines small enough that each is its own chunk.
	f1 := makeFileDiff("x.go", 1, 10)
	f2 := makeFileDiff("y.go", 1, 10)
	f3 := makeFileDiff("z.go", 1, 10)
	diff := f1 + "\n" + f2 + "\n" + f3

	chunks := SplitDiff(diff, 16)

	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
	total := len(chunks)

	for i, ch := range chunks {
		if ch.Index != i {
			t.Errorf("chunk %d: expected Index=%d, got %d", i, i, ch.Index)
		}
		if ch.Total != total {
			t.Errorf("chunk %d: expected Total=%d, got %d", i, total, ch.Total)
		}
		if ch.Lines <= 0 {
			t.Errorf("chunk %d: Lines should be >0, got %d", i, ch.Lines)
		}
		if ch.Lines != diffLineCount(ch.Content) {
			t.Errorf("chunk %d: Lines=%d but countLines(Content)=%d",
				i, ch.Lines, diffLineCount(ch.Content))
		}
		if len(ch.Files) == 0 {
			t.Errorf("chunk %d: Files is empty", i)
		}
	}
}

func TestSplitDiffDefaultMaxLines(t *testing.T) {
	t.Parallel()

	// Pass 0 → should use DefaultMaxDiffLines (500).
	// A small diff should remain as a single chunk.
	diff := makeFileDiff("tiny.go", 1, 3)
	chunks := SplitDiff(diff, 0)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk with default maxLines, got %d", len(chunks))
	}
	if chunks[0].Total != 1 {
		t.Errorf("expected Total=1, got %d", chunks[0].Total)
	}

	// Negative value should also use the default.
	chunks2 := SplitDiff(diff, -10)
	if len(chunks2) != 1 {
		t.Fatalf("expected 1 chunk with negative maxLines (default), got %d", len(chunks2))
	}
}

// ── additional edge-case tests ──────────────────────────────────────────

func TestSplitDiffSingleHugeHunk(t *testing.T) {
	t.Parallel()

	// A file with one hunk larger than maxLines. The hunk must not be split
	// mid-hunk; it should be emitted as a single (oversized) chunk.
	diff := makeFileDiff("huge.go", 1, 300)
	chunks := SplitDiff(diff, 100)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for a single oversized hunk, got %d", len(chunks))
	}
	if chunks[0].Lines > 0 && chunks[0].Files[0] != "huge.go" {
		t.Errorf("chunk file should be huge.go, got %v", chunks[0].Files)
	}
}

func TestSplitDiffFileNameExtraction(t *testing.T) {
	t.Parallel()

	// Paths with nested directories.
	diff := makeFileDiff("internal/command/diff_splitter.go", 1, 3)
	chunks := SplitDiff(diff, 500)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Files[0] != "internal/command/diff_splitter.go" {
		t.Errorf("expected full path, got %s", chunks[0].Files[0])
	}
}

func TestSplitDiffReassembly(t *testing.T) {
	t.Parallel()

	// Verify that the union of all chunk contents covers the same set of
	// diff-markers as the original input. We don't require exact string
	// equality (headers may be duplicated for hunk splits), but every hunk
	// line from the original must appear in exactly one chunk.
	f1 := makeFileDiff("a.go", 3, 20)
	f2 := makeFileDiff("b.go", 2, 15)
	diff := f1 + "\n" + f2
	chunks := SplitDiff(diff, 50)

	// Collect all "+added" lines.
	originalAdded := collectAdded(diff)
	var chunkAdded []string
	for _, ch := range chunks {
		chunkAdded = append(chunkAdded, collectAdded(ch.Content)...)
	}

	if len(chunkAdded) != len(originalAdded) {
		t.Fatalf("original has %d added lines, chunks have %d",
			len(originalAdded), len(chunkAdded))
	}
	for i, line := range originalAdded {
		if chunkAdded[i] != line {
			t.Errorf("mismatch at index %d: %q vs %q", i, line, chunkAdded[i])
			break
		}
	}
}

func collectAdded(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "+added ") {
			out = append(out, line)
		}
	}
	return out
}

func TestSplitDiffNoGitPrefix(t *testing.T) {
	t.Parallel()

	// Input that does not contain "diff --git" at all → nothing to split.
	chunks := SplitDiff("some random text\nwithout diff markers\n", 500)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for non-diff input, got %d", len(chunks))
	}
}

func TestSplitDiffExactFit(t *testing.T) {
	t.Parallel()

	// Two files whose combined line count exactly equals maxLines.
	// They should share a single chunk.
	f1 := makeFileDiff("x.go", 1, 3)
	f2 := makeFileDiff("y.go", 1, 3)
	single := f1 + "\n" + f2

	totalLines := countLines(single)
	chunks := SplitDiff(single, totalLines)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk when total exactly fits, got %d", len(chunks))
	}
	if len(chunks[0].Files) != 2 {
		t.Errorf("expected 2 files in chunk, got %d", len(chunks[0].Files))
	}
}
