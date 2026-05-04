package ctxutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitExec runs a git command in the specified directory and fails the test on error.
func gitExec(t *testing.T, dir string, args ...string) string {
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

// initGitRepo creates a minimal git repo with one committed file.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitExec(t, dir, "init")
	gitExec(t, dir, "config", "user.email", "test@test.com")
	gitExec(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("initial content\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	gitExec(t, dir, "add", "file.txt")
	gitExec(t, dir, "commit", "-m", "initial commit")
}

// chdirCleanup changes the working directory and restores it on cleanup.
// Tests using this must NOT be marked parallel.
func chdirCleanup(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Logf("warning: failed to restore cwd: %v", err)
		}
	})
}

// --- runGitDiff tests ---

func TestRunGitDiff_Success(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	chdirCleanup(t, dir)

	// Modify tracked file so diff has content.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified content\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	stdout, msg, hadErr := runGitDiff(context.Background(), []string{"HEAD"})
	if hadErr {
		t.Fatalf("expected success, got error: %s", msg)
	}
	if msg != "" {
		t.Fatalf("expected empty message on success, got: %q", msg)
	}
	if !strings.Contains(stdout, "modified content") {
		t.Fatalf("expected diff output to contain 'modified content', got:\n%s", stdout)
	}
}

func TestRunGitDiff_Error(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	chdirCleanup(t, dir)

	// Use an invalid ref to trigger a git error.
	_, msg, hadErr := runGitDiff(context.Background(), []string{"nonexistent_ref_xyz123"})
	if !hadErr {
		t.Fatal("expected error for invalid git ref")
	}
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestRunGitDiff_NoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	chdirCleanup(t, dir)

	// No modifications → stdout should be empty, no error.
	stdout, msg, hadErr := runGitDiff(context.Background(), []string{"HEAD"})
	if hadErr {
		t.Fatalf("expected no error for clean tree, got: %s", msg)
	}
	if stdout != "" {
		t.Fatalf("expected empty diff for clean tree, got:\n%s", stdout)
	}
}

// --- getDefaultGitDiffArgs tests ---

func TestGetDefaultGitDiffArgs_WithChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	chdirCleanup(t, dir)

	// Modify file so HEAD has uncommitted changes.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("uncommitted\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	args := getDefaultGitDiffArgs(context.Background())
	if len(args) != 1 || args[0] != "HEAD" {
		t.Fatalf("expected [HEAD] when changes exist, got %v", args)
	}
}

func TestGetDefaultGitDiffArgs_MultipleCommitsNoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	chdirCleanup(t, dir)

	// Create a second commit so HEAD~1 exists.
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("second file\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	gitExec(t, dir, "add", "file2.txt")
	gitExec(t, dir, "commit", "-m", "second commit")

	// No uncommitted changes, HEAD~1 exists → should return ["HEAD~1"].
	args := getDefaultGitDiffArgs(context.Background())
	if len(args) != 1 || args[0] != "HEAD~1" {
		t.Fatalf("expected [HEAD~1] when HEAD~1 exists and no changes, got %v", args)
	}
}

func TestGetDefaultGitDiffArgs_SingleCommitNoChanges(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	chdirCleanup(t, dir)

	// Single commit, no changes → empty-tree fallback.
	args := getDefaultGitDiffArgs(context.Background())
	if len(args) != 2 {
		t.Fatalf("expected 2-element fallback for single commit, got %v", args)
	}
	if args[0] != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Fatalf("expected empty-tree SHA, got %q", args[0])
	}
	if args[1] != "HEAD" {
		t.Fatalf("expected HEAD as second arg, got %q", args[1])
	}
}
