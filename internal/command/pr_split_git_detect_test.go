package command

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// ---------------------------------------------------------------------------
// T390: Early git-repo detection — unit and E2E tests
//
// Validates that non-git directories and missing base branches are caught
// by validateGitRepo BEFORE the scripting engine or TUI wizard starts.
// ---------------------------------------------------------------------------

// TestValidateGitRepo_NotAGitRepo verifies that running from a plain
// directory (not a git repo) produces an immediate, descriptive error.
func TestValidateGitRepo_NotAGitRepo(t *testing.T) {
	// Cannot use t.Parallel() — changes process working directory.
	dir := t.TempDir()
	pushd(t, dir)

	cmd := &PrSplitCommand{
		baseBranch: "main",
		strategy:   "directory",
		maxFiles:   10,
	}

	err := cmd.validateGitRepo()
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' in error, got: %v", err)
	}
}

// TestValidateGitRepo_MissingBaseBranch verifies that a non-existent base
// branch is rejected immediately, before any TUI startup.
func TestValidateGitRepo_MissingBaseBranch(t *testing.T) {
	// Cannot use t.Parallel() — changes process working directory.
	dir := setupMinimalGitRepo(t)
	pushd(t, dir)

	cmd := &PrSplitCommand{
		baseBranch: "nonexistent-branch-xyz",
		strategy:   "directory",
		maxFiles:   10,
	}

	err := cmd.validateGitRepo()
	if err == nil {
		t.Fatal("expected error for missing base branch, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-branch-xyz") {
		t.Errorf("expected branch name in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestValidateGitRepo_ValidRepo confirms that a valid repo with an existing
// base branch passes validation without error.
func TestValidateGitRepo_ValidRepo(t *testing.T) {
	// Cannot use t.Parallel() — changes process working directory.
	dir := setupMinimalGitRepo(t)
	pushd(t, dir)

	cmd := &PrSplitCommand{
		baseBranch: "main",
		strategy:   "directory",
		maxFiles:   10,
	}

	if err := cmd.validateGitRepo(); err != nil {
		t.Fatalf("unexpected error for valid repo: %v", err)
	}
}

// TestValidateGitRepo_EmptyBaseBranch confirms that an empty base-branch
// flag skips the branch existence check (it will be set interactively).
func TestValidateGitRepo_EmptyBaseBranch(t *testing.T) {
	dir := setupMinimalGitRepo(t)
	pushd(t, dir)

	cmd := &PrSplitCommand{
		baseBranch: "",
		strategy:   "directory",
		maxFiles:   10,
	}

	if err := cmd.validateGitRepo(); err != nil {
		t.Fatalf("unexpected error with empty base branch: %v", err)
	}
}

// TestValidateGitRepo_GitNotInstalled verifies a clear error when git
// is not on PATH (exec.ErrNotFound path).
func TestValidateGitRepo_GitNotInstalled(t *testing.T) {
	// Overwrite PATH to exclude git. t.Setenv restores on cleanup.
	t.Setenv("PATH", t.TempDir())

	cmd := &PrSplitCommand{
		baseBranch: "main",
		strategy:   "directory",
		maxFiles:   10,
	}

	err := cmd.validateGitRepo()
	if err == nil {
		t.Fatal("expected error when git is not in PATH, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") && !strings.Contains(err.Error(), "not in PATH") {
		t.Errorf("expected 'not installed' or 'not in PATH' in error, got: %v", err)
	}
}

// TestExecute_NotAGitRepo_NoWizard is an integration test that confirms
// Execute() returns immediately from a non-git directory without printing
// any wizard/TUI output (specifically, no "PR Split Wizard active" line).
func TestExecute_NotAGitRepo_NoWizard(t *testing.T) {
	dir := t.TempDir()
	pushd(t, dir)

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	cmd.strategy = "directory"
	cmd.maxFiles = 10
	cmd.baseBranch = "main"
	cmd.testMode = true
	cmd.scriptCommandBase.store = "memory"
	cmd.scriptCommandBase.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from Execute in non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' error, got: %v", err)
	}
	// Crucially: no TUI/wizard output should have been emitted.
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "Wizard") || strings.Contains(combined, "wizard") ||
		strings.Contains(combined, "PR Split") || strings.Contains(combined, "BubbleTea") {
		t.Errorf("wizard output was emitted before git validation:\n%s", combined)
	}
}

// TestExecute_MissingBaseBranch_NoWizard is an integration test that
// confirms Execute() returns immediately when the base branch doesn't exist.
func TestExecute_MissingBaseBranch_NoWizard(t *testing.T) {
	dir := setupMinimalGitRepo(t)
	pushd(t, dir)

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	cmd.strategy = "directory"
	cmd.maxFiles = 10
	cmd.baseBranch = "totally-bogus-branch"
	cmd.testMode = true
	cmd.scriptCommandBase.store = "memory"
	cmd.scriptCommandBase.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from Execute with missing branch, got nil")
	}
	if !strings.Contains(err.Error(), "totally-bogus-branch") {
		t.Errorf("expected branch name in error, got: %v", err)
	}
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "Wizard") || strings.Contains(combined, "wizard") ||
		strings.Contains(combined, "PR Split") || strings.Contains(combined, "BubbleTea") {
		t.Errorf("wizard output was emitted before git validation:\n%s", combined)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pushd changes the working directory to dir and restores it on cleanup.
func pushd(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// setupMinimalGitRepo creates a minimal git repository with one commit
// on the "main" branch and returns the repo directory path.
func setupMinimalGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	git := func(args ...string) {
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
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-m", "initial")

	return dir
}
