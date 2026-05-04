package command

import (
	"path"
	"strings"
)

// DefaultMaxDiffLines is the default maximum number of lines per chunk,
// chosen to fit comfortably within typical LLM context windows.
const DefaultMaxDiffLines = 500

// DiffChunk represents a coherent portion of a larger diff.
type DiffChunk struct {
	Index   int      // 0-based chunk index
	Total   int      // total number of chunks (set after all chunks are built)
	Files   []string // file names in this chunk
	Content string   // the actual diff text
	Lines   int      // number of lines in Content
}

// SplitDiff splits a unified diff into chunks of approximately maxLines each.
// It respects file and hunk boundaries for semantic coherence.
//
// When maxLines is <= 0, DefaultMaxDiffLines is used.
//
// The algorithm:
//  1. Parse the diff into per-file segments (splitting at "diff --git" lines).
//  2. Bin-pack file diffs into chunks that do not exceed maxLines.
//  3. If a single file diff exceeds maxLines, split it at hunk ("@@") boundaries.
//  4. Assign Index/Total metadata to every resulting chunk.
func SplitDiff(diff string, maxLines int) []DiffChunk {
	if maxLines <= 0 {
		maxLines = DefaultMaxDiffLines
	}

	diff = strings.TrimRight(diff, "\n")
	if diff == "" {
		return nil
	}

	fileDiffs := splitIntoFileDiffs(diff)
	if len(fileDiffs) == 0 {
		return nil
	}

	var chunks []DiffChunk
	var curFiles []string
	var curParts []string
	curLines := 0

	flush := func() {
		if len(curParts) == 0 {
			return
		}
		content := strings.Join(curParts, "\n")
		chunks = append(chunks, DiffChunk{
			Index:   len(chunks),
			Files:   curFiles,
			Content: content,
			Lines:   countLines(content),
		})
		curFiles = nil
		curParts = nil
		curLines = 0
	}

	for _, fd := range fileDiffs {
		fdLines := countLines(fd.content)
		fileName := fd.name

		// Case 1: file fits in the current chunk.
		if curLines+fdLines <= maxLines {
			curFiles = append(curFiles, fileName)
			curParts = append(curParts, fd.content)
			curLines += fdLines
			continue
		}

		// Case 2: file doesn't fit, but current chunk is non-empty → flush first,
		// then try again with the file in a fresh chunk.
		if curLines > 0 {
			flush()
		}

		// Case 3: file fits in an empty chunk.
		if fdLines <= maxLines {
			curFiles = append(curFiles, fileName)
			curParts = append(curParts, fd.content)
			curLines = fdLines
			continue
		}

		// Case 4: single file exceeds maxLines → split at hunk boundaries.
		hunkChunks := splitFileAtHunks(fd, maxLines)
		for _, hc := range hunkChunks {
			chunks = append(chunks, DiffChunk{
				Index:   len(chunks),
				Files:   []string{fileName},
				Content: hc,
				Lines:   countLines(hc),
			})
		}
	}

	flush()

	// Stamp Total on every chunk.
	for i := range chunks {
		chunks[i].Total = len(chunks)
	}

	return chunks
}

// fileDiff holds the parsed content and extracted file name from one
// "diff --git a/... b/..." section.
type fileDiff struct {
	name    string
	content string
}

// splitIntoFileDiffs splits a unified diff string into per-file segments.
// Each segment starts with a "diff --git" line.
func splitIntoFileDiffs(diff string) []fileDiff {
	const marker = "diff --git "
	var result []fileDiff
	lines := strings.Split(diff, "\n")

	var current []string
	started := false // have we seen the first "diff --git" line?
	for _, line := range lines {
		if strings.HasPrefix(line, marker) {
			if started && len(current) > 0 {
				text := strings.Join(current, "\n")
				result = append(result, fileDiff{
					name:    extractFileName(current[0]),
					content: text,
				})
			}
			started = true
			current = []string{line}
		} else if started {
			current = append(current, line)
		}
		// Lines before the first "diff --git" marker are ignored.
	}
	if started && len(current) > 0 {
		text := strings.Join(current, "\n")
		result = append(result, fileDiff{
			name:    extractFileName(current[0]),
			content: text,
		})
	}
	return result
}

// extractFileName extracts the file path from a "diff --git a/path b/path" line.
// It returns the b-side path. If parsing fails it returns the raw line.
func extractFileName(diffLine string) string {
	const prefix = "diff --git "
	if !strings.HasPrefix(diffLine, prefix) {
		return diffLine
	}
	rest := diffLine[len(prefix):]
	// Standard format: "a/foo b/foo"
	// Find the " b/" separator — we take the last occurrence in case the
	// path itself contains " b/".
	idx := strings.LastIndex(rest, " b/")
	if idx < 0 {
		return rest
	}
	return path.Clean(rest[idx+len(" b/"):])
}

// splitFileAtHunks splits a single file diff into chunks that stay at or
// under maxLines, cutting only at hunk headers ("@@"). Each chunk includes
// the file header (everything before the first hunk).
func splitFileAtHunks(fd fileDiff, maxLines int) []string {
	lines := strings.Split(fd.content, "\n")

	// Separate the file header from the hunks.
	var header []string
	var hunkStarts []int
	for i, line := range lines {
		if strings.HasPrefix(line, "@@") {
			hunkStarts = append(hunkStarts, i)
		}
	}

	if len(hunkStarts) == 0 {
		// No hunk headers — return the whole file as one chunk.
		return []string{fd.content}
	}

	header = lines[:hunkStarts[0]]

	// Build hunk slices.
	type hunk struct {
		lines []string
	}
	var hunks []hunk
	for i, start := range hunkStarts {
		end := len(lines)
		if i+1 < len(hunkStarts) {
			end = hunkStarts[i+1]
		}
		hunks = append(hunks, hunk{lines: lines[start:end]})
	}

	headerLen := len(header)
	var chunks []string
	var curHunkLines []string
	curLen := headerLen

	flushHunks := func() {
		if len(curHunkLines) == 0 {
			return
		}
		all := make([]string, 0, headerLen+len(curHunkLines))
		all = append(all, header...)
		all = append(all, curHunkLines...)
		chunks = append(chunks, strings.Join(all, "\n"))
		curHunkLines = nil
		curLen = headerLen
	}

	for _, h := range hunks {
		hLen := len(h.lines)

		if curLen+hLen <= maxLines {
			curHunkLines = append(curHunkLines, h.lines...)
			curLen += hLen
			continue
		}

		// Doesn't fit — flush what we have and start fresh.
		if len(curHunkLines) > 0 {
			flushHunks()
		}

		// If a single hunk + header exceeds maxLines, we still emit it as
		// one chunk rather than splitting mid-hunk. This preserves hunk
		// coherence at the cost of occasionally exceeding the limit.
		curHunkLines = append(curHunkLines, h.lines...)
		curLen += hLen
	}

	flushHunks()
	return chunks
}

// countLines returns the number of lines in s (the count of '\n'
// characters, plus 1 if s is non-empty and does not end with '\n').
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
