package command

import (
	"strings"
	"testing"
)

// ── splitIntoFileDiffs (unexported) ────────────────────────────────

func TestSplitIntoFileDiffs_SingleFile(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1,3 +1,3 @@\n-old\n+new\n"
	fds := splitIntoFileDiffs(diff)
	if len(fds) != 1 {
		t.Fatalf("got %d fileDiffs, want 1", len(fds))
	}
	if fds[0].name != "foo.go" {
		t.Errorf("name: got %q, want %q", fds[0].name, "foo.go")
	}
	if !strings.Contains(fds[0].content, "diff --git") {
		t.Error("content should contain the diff header")
	}
}

func TestSplitIntoFileDiffs_MultipleFiles(t *testing.T) {
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-x\n+y\n" +
		"diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n-p\n+q\n"
	fds := splitIntoFileDiffs(diff)
	if len(fds) != 2 {
		t.Fatalf("got %d fileDiffs, want 2", len(fds))
	}
	if fds[0].name != "a.go" {
		t.Errorf("file 0 name: got %q, want %q", fds[0].name, "a.go")
	}
	if fds[1].name != "b.go" {
		t.Errorf("file 1 name: got %q, want %q", fds[1].name, "b.go")
	}
}

func TestSplitIntoFileDiffs_ContentBeforeFirstMarker_Ignored(t *testing.T) {
	diff := "some preamble\nmore preamble\ndiff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n"
	fds := splitIntoFileDiffs(diff)
	if len(fds) != 1 {
		t.Fatalf("got %d fileDiffs, want 1", len(fds))
	}
	// Content before first marker should not appear in the file diff.
	if strings.Contains(fds[0].content, "preamble") {
		t.Error("pre-marker content should be ignored")
	}
}

func TestSplitIntoFileDiffs_Empty(t *testing.T) {
	fds := splitIntoFileDiffs("")
	if len(fds) != 0 {
		t.Errorf("got %d fileDiffs for empty input, want 0", len(fds))
	}
}

func TestSplitIntoFileDiffs_NoMarkers(t *testing.T) {
	diff := "this is not a diff\njust plain text\n"
	fds := splitIntoFileDiffs(diff)
	if len(fds) != 0 {
		t.Errorf("got %d fileDiffs for no-marker input, want 0", len(fds))
	}
}

func TestSplitIntoFileDiffs_PreservesHunkContent(t *testing.T) {
	diff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@\n-old line\n+new line\n context\n"
	fds := splitIntoFileDiffs(diff)
	if len(fds) != 1 {
		t.Fatalf("got %d fileDiffs, want 1", len(fds))
	}
	if !strings.Contains(fds[0].content, "@@ -1,2 +1,2 @@") {
		t.Error("hunk header should be preserved")
	}
	if !strings.Contains(fds[0].content, "+new line") {
		t.Error("hunk content should be preserved")
	}
}

// ── splitFileAtHunks (unexported) ──────────────────────────────────

func TestSplitFileAtHunks_NoHunkHeaders(t *testing.T) {
	fd := fileDiff{
		name:    "test.go",
		content: "diff --git a/test.go b/test.go\n--- a/test.go\n+++ b/test.go\n",
	}
	chunks := splitFileAtHunks(fd, 50)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if chunks[0] != fd.content {
		t.Error("single chunk should be the full content")
	}
}

func TestSplitFileAtHunks_SingleHunkWithinLimit(t *testing.T) {
	fd := fileDiff{
		name: "test.go",
		content: "diff --git a/test.go b/test.go\n" +
			"--- a/test.go\n" +
			"+++ b/test.go\n" +
			"@@ -1,3 +1,3 @@\n" +
			"-old\n" +
			"+new\n" +
			" ctx\n",
	}
	chunks := splitFileAtHunks(fd, 100)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if !strings.Contains(chunks[0], "diff --git") {
		t.Error("chunk should contain the diff header")
	}
	if !strings.Contains(chunks[0], "@@ -1,3") {
		t.Error("chunk should contain the hunk header")
	}
}

func TestSplitFileAtHunks_MultipleHunks_FitInOneChunk(t *testing.T) {
	fd := fileDiff{
		name: "test.go",
		content: "diff --git a/test.go b/test.go\n" +
			"--- a/test.go\n" +
			"+++ b/test.go\n" +
			"@@ -1,2 +1,2 @@\n" +
			"-a\n" +
			"+b\n" +
			"@@ -10,2 +10,2 @@\n" +
			"-c\n" +
			"+d\n",
	}
	// maxLines=100 — both hunks fit.
	chunks := splitFileAtHunks(fd, 100)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
}

func TestSplitFileAtHunks_MultipleHunks_EachInSeparateChunk(t *testing.T) {
	// Header: 3 lines. Each hunk: 3 lines. maxLines=6 → 3+3=6 fits one hunk,
	// but 3+3+3=9 exceeds. So two chunks.
	fd := fileDiff{
		name: "test.go",
		content: "diff --git a/test.go b/test.go\n" +
			"--- a/test.go\n" +
			"+++ b/test.go\n" +
			"@@ -1,2 +1,2 @@\n" +
			"-a\n" +
			"+b\n" +
			"@@ -10,2 +10,2 @@\n" +
			"-c\n" +
			"+d\n",
	}
	chunks := splitFileAtHunks(fd, 6)
	if len(chunks) < 2 {
		t.Fatalf("got %d chunks, want >= 2", len(chunks))
	}
	// Each chunk should contain the file header.
	for i, chunk := range chunks {
		if !strings.Contains(chunk, "diff --git") {
			t.Errorf("chunk %d missing diff header", i)
		}
	}
}

func TestSplitFileAtHunks_OversizedSingleHunk(t *testing.T) {
	// A single hunk that exceeds maxLines. Should still emit as one chunk
	// (never splits mid-hunk).
	lines := []string{
		"diff --git a/big.go b/big.go",
		"--- a/big.go",
		"+++ b/big.go",
		"@@ -1,50 +1,50 @@",
	}
	for i := 0; i < 50; i++ {
		lines = append(lines, "+line")
	}
	fd := fileDiff{name: "big.go", content: strings.Join(lines, "\n")}
	chunks := splitFileAtHunks(fd, 10) // maxLines=10 but hunk has 50+ lines
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1 (single oversized hunk)", len(chunks))
	}
	if !strings.Contains(chunks[0], "@@ -1,50") {
		t.Error("chunk should contain the oversized hunk")
	}
}

func TestSplitFileAtHunks_HeaderRepeated(t *testing.T) {
	// When hunks are split across chunks, each chunk must include the header.
	fd := fileDiff{
		name: "test.go",
		content: "diff --git a/test.go b/test.go\n" +
			"--- a/test.go\n" +
			"+++ b/test.go\n" +
			"@@ -1,1 +1,1 @@\n" +
			"-a\n" +
			"@@ -10,1 +10,1 @@\n" +
			"-b\n" +
			"@@ -20,1 +20,1 @@\n" +
			"-c\n",
	}
	// maxLines=5 → header(3) + hunk(2) = 5 fits exactly.
	chunks := splitFileAtHunks(fd, 5)
	for i, chunk := range chunks {
		if !strings.HasPrefix(chunk, "diff --git") {
			t.Errorf("chunk %d should start with diff header", i)
		}
	}
}

// ── countRelSegments (unexported) ──────────────────────────────────

func TestCountRelSegments_MixedSegments(t *testing.T) {
	up, down := countRelSegments([]string{"..", "..", "foo", "bar"})
	if up != 2 || down != 2 {
		t.Errorf("got up=%d down=%d, want up=2 down=2", up, down)
	}
}

func TestCountRelSegments_AllUp(t *testing.T) {
	up, down := countRelSegments([]string{"..", "..", ".."})
	if up != 3 || down != 0 {
		t.Errorf("got up=%d down=%d, want up=3 down=0", up, down)
	}
}

func TestCountRelSegments_AllDown(t *testing.T) {
	up, down := countRelSegments([]string{"src", "pkg", "main.go"})
	if up != 0 || down != 3 {
		t.Errorf("got up=%d down=%d, want up=0 down=3", up, down)
	}
}

func TestCountRelSegments_EmptyAndDot_Ignored(t *testing.T) {
	up, down := countRelSegments([]string{"", ".", "foo", "", "."})
	if up != 0 || down != 1 {
		t.Errorf("got up=%d down=%d, want up=0 down=1", up, down)
	}
}

func TestCountRelSegments_EmptySlice(t *testing.T) {
	up, down := countRelSegments(nil)
	if up != 0 || down != 0 {
		t.Errorf("got up=%d down=%d, want up=0 down=0", up, down)
	}
}

// ── extractFileName ────────────────────────────────────────────────

func TestExtractFileName_StandardFormat(t *testing.T) {
	name := extractFileName("diff --git a/src/main.go b/src/main.go")
	if name != "src/main.go" {
		t.Errorf("got %q, want %q", name, "src/main.go")
	}
}

func TestExtractFileName_NoPrefix_RawReturn(t *testing.T) {
	// If line doesn't start with "diff --git ", return raw line.
	name := extractFileName("some random line")
	if name != "some random line" {
		t.Errorf("got %q, want %q", name, "some random line")
	}
}

func TestExtractFileName_PathContainingBSlash(t *testing.T) {
	// Path with " b/" in it — extractFileName uses LastIndex, so for
	// ambiguous paths like "a b/c.go", the last " b/" becomes the
	// separator. This is an inherent limitation of the git diff format.
	name := extractFileName("diff --git a/a b/c.go b/a b/c.go")
	// LastIndex picks the last " b/", yielding just "c.go".
	if name != "c.go" {
		t.Errorf("got %q, want %q", name, "c.go")
	}
}

func TestExtractFileName_NoBSlash(t *testing.T) {
	// Malformed diff line without " b/" separator.
	name := extractFileName("diff --git foo bar")
	if name != "foo bar" {
		t.Errorf("got %q, want %q", name, "foo bar")
	}
}

// ── countLines ─────────────────────────────────────────────────────

func TestCountLines_Empty(t *testing.T) {
	if n := countLines(""); n != 0 {
		t.Errorf("countLines(\"\") = %d, want 0", n)
	}
}

func TestCountLines_SingleLineNoNewline(t *testing.T) {
	if n := countLines("hello"); n != 1 {
		t.Errorf("countLines(\"hello\") = %d, want 1", n)
	}
}

func TestCountLines_SingleLineWithNewline(t *testing.T) {
	if n := countLines("hello\n"); n != 1 {
		t.Errorf("countLines(\"hello\\n\") = %d, want 1", n)
	}
}

func TestCountLines_MultipleLines(t *testing.T) {
	if n := countLines("a\nb\nc\n"); n != 3 {
		t.Errorf("countLines(\"a\\nb\\nc\\n\") = %d, want 3", n)
	}
}

func TestCountLines_MultipleLinesNoTrailingNewline(t *testing.T) {
	if n := countLines("a\nb\nc"); n != 3 {
		t.Errorf("countLines(\"a\\nb\\nc\") = %d, want 3", n)
	}
}
