package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ===========================================================================
//  Chunk 01: Analysis — Tests
//
//  Tests for analyzeDiff and analyzeDiffStats, loaded via loadChunkEngine
//  with chunks 00_core + 01_analysis. Uses real git repos (t.TempDir)
//  to verify diff parsing handles edge cases.
// ===========================================================================

// gitInit creates a fresh git repo in dir with an initial commit.
func gitInit(t testing.TB, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-b", "main", dir)
	// Create initial commit so main exists.
	initial := filepath.Join(dir, ".gitkeep")
	if err := os.WriteFile(initial, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	run("-C", dir, "add", ".")
	run("-C", dir, "commit", "-m", "initial")
}

// gitRun runs a git command in dir.
func gitRun(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestChunk01_AnalyzeDiff_MultiFileChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows due to git path differences")
	}

	dir := t.TempDir()
	gitInit(t, dir)

	// Create files on main.
	for _, f := range []struct{ name, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"docs/README.md", "# Docs\n"},
	} {
		p := filepath.Join(dir, f.name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "add base files")

	// Create feature branch with changes.
	gitRun(t, dir, "checkout", "-b", "feature")
	// Modify existing file.
	os.WriteFile(filepath.Join(dir, "pkg/types.go"),
		[]byte("package pkg\n\ntype Foo struct{ Name string }\n"), 0644)
	// Add new file.
	os.MkdirAll(filepath.Join(dir, "pkg"), 0755)
	os.WriteFile(filepath.Join(dir, "pkg/handler.go"),
		[]byte("package pkg\n\nfunc Handler() {}\n"), 0644)
	// Delete file.
	os.Remove(filepath.Join(dir, "docs/README.md"))
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "feature changes")

	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch": "main",
	}, "00_core", "01_analysis")

	raw, err := evalJS(fmt.Sprintf(
		`JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: %q}))`, dir))
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var result struct {
		Files         []string          `json:"files"`
		FileStatuses  map[string]string `json:"fileStatuses"`
		Error         *string           `json:"error"`
		BaseBranch    string            `json:"baseBranch"`
		CurrentBranch string            `json:"currentBranch"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("JSON parse failed: %v\nraw: %s", err, raw)
	}

	if result.Error != nil {
		t.Fatalf("analyzeDiff returned error: %s", *result.Error)
	}
	if result.BaseBranch != "main" {
		t.Errorf("baseBranch = %q, want 'main'", result.BaseBranch)
	}
	if result.CurrentBranch != "feature" {
		t.Errorf("currentBranch = %q, want 'feature'", result.CurrentBranch)
	}

	// Should have 3 files: modified, added, deleted.
	if len(result.Files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(result.Files), result.Files)
	}

	// Verify statuses.
	expectations := map[string]string{
		"pkg/types.go":   "M",
		"pkg/handler.go": "A",
		"docs/README.md": "D",
	}
	for file, wantStatus := range expectations {
		got, ok := result.FileStatuses[file]
		if !ok {
			t.Errorf("file %q not in fileStatuses", file)
			continue
		}
		if got != wantStatus {
			t.Errorf("fileStatuses[%q] = %q, want %q", file, got, wantStatus)
		}
	}
}

func TestChunk01_AnalyzeDiff_EmptyDiff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows due to git path differences")
	}

	dir := t.TempDir()
	gitInit(t, dir)

	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "add file")

	// Feature branch with NO changes.
	gitRun(t, dir, "checkout", "-b", "feature")

	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch": "main",
	}, "00_core", "01_analysis")

	raw, err := evalJS(fmt.Sprintf(
		`JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: %q}))`, dir))
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var result struct {
		Files []string `json:"files"`
		Error *string  `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != nil {
		t.Fatalf("analyzeDiff returned error: %s", *result.Error)
	}
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files for empty diff, got %d: %v", len(result.Files), result.Files)
	}
}

func TestChunk01_AnalyzeDiff_BadBaseBranch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows due to git path differences")
	}

	dir := t.TempDir()
	gitInit(t, dir)

	gitRun(t, dir, "checkout", "-b", "feature")
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x\n"), 0644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "x")

	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch": "nonexistent-branch",
	}, "00_core", "01_analysis")

	raw, err := evalJS(fmt.Sprintf(
		`JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'nonexistent-branch', dir: %q}))`, dir))
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var result struct {
		Files []string `json:"files"`
		Error *string  `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error == nil {
		t.Fatal("expected error for nonexistent base branch, got nil")
	}
	if !strings.Contains(*result.Error, "merge-base failed") {
		t.Errorf("error should mention merge-base, got: %s", *result.Error)
	}
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files on error, got %d", len(result.Files))
	}
}

func TestChunk01_AnalyzeDiff_Rename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows due to git path differences")
	}

	dir := t.TempDir()
	gitInit(t, dir)

	os.WriteFile(filepath.Join(dir, "old.txt"), []byte("content\n"), 0644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "add old.txt")

	gitRun(t, dir, "checkout", "-b", "feature")
	gitRun(t, dir, "mv", "old.txt", "new.txt")
	gitRun(t, dir, "commit", "-m", "rename")

	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch": "main",
	}, "00_core", "01_analysis")

	raw, err := evalJS(fmt.Sprintf(
		`JSON.stringify(globalThis.prSplit.analyzeDiff({baseBranch: 'main', dir: %q}))`, dir))
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var result struct {
		Files        []string          `json:"files"`
		FileStatuses map[string]string `json:"fileStatuses"`
		Error        *string           `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %s", *result.Error)
	}

	// Renames should track only the NEW path.
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file (renamed), got %d: %v", len(result.Files), result.Files)
	}
	if result.Files[0] != "new.txt" {
		t.Errorf("expected renamed file 'new.txt', got %q", result.Files[0])
	}
	if status, ok := result.FileStatuses["new.txt"]; !ok {
		t.Error("new.txt missing from fileStatuses")
	} else if status != "R" {
		t.Errorf("expected status 'R', got %q", status)
	}
}

func TestChunk01_AnalyzeDiffStats_BasicCounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows due to git path differences")
	}

	dir := t.TempDir()
	gitInit(t, dir)

	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nline2\nline3\n"), 0644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial file")

	gitRun(t, dir, "checkout", "-b", "feature")
	// Add 2 lines, remove 1.
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nmodified\nnew1\nnew2\n"), 0644)
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "modify file")

	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch": "main",
	}, "00_core", "01_analysis")

	raw, err := evalJS(fmt.Sprintf(
		`JSON.stringify(globalThis.prSplit.analyzeDiffStats({baseBranch: 'main', dir: %q}))`, dir))
	if err != nil {
		t.Fatalf("analyzeDiffStats failed: %v", err)
	}

	var result struct {
		Files []struct {
			Name      string `json:"name"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %s", *result.Error)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[0].Name != "file.txt" {
		t.Errorf("expected file.txt, got %q", result.Files[0].Name)
	}
	if result.Files[0].Additions == 0 {
		t.Error("expected additions > 0")
	}
	if result.Files[0].Deletions == 0 {
		t.Error("expected deletions > 0")
	}
}

func TestChunk01_AnalyzeDiffStats_EmptyDiff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows due to git path differences")
	}

	dir := t.TempDir()
	gitInit(t, dir)

	gitRun(t, dir, "checkout", "-b", "feature")

	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch": "main",
	}, "00_core", "01_analysis")

	raw, err := evalJS(fmt.Sprintf(
		`JSON.stringify(globalThis.prSplit.analyzeDiffStats({baseBranch: 'main', dir: %q}))`, dir))
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Files []interface{} `json:"files"`
		Error *string       `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %s", *result.Error)
	}
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files for empty diff, got %d", len(result.Files))
	}
}
