//go:build unix

package command

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Binary E2E Tests
//
// These tests compile the ACTUAL osm binary and exercise pr-split end-to-end
// as a subprocess. They prove the full stack works: CLI flag parsing → Go
// wiring → JS engine → TUI command dispatch → git operations → output.
//
// The binary is built once per test run via buildOSMBinary (cached with
// sync.Once). Tests create isolated temp git repos and verify git state
// after the binary exits.
//
// These tests do NOT require the -integration flag, Claude, or any external
// services. They test the heuristic (non-AI) flow exclusively via the
// batch command dispatch path (positional args after flags).
// ---------------------------------------------------------------------------

// setupBinaryTestRepo creates a test git repo with a realistic structure
// for binary E2E testing. Returns the repository path.
func setupBinaryTestRepo(t *testing.T) string {
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

	writeFile := func(relPath, content string) {
		t.Helper()
		full := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Initialize repo with base files.
	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("cmd/app/main.go", "package main\n\nfunc main() {}\n")
	writeFile("pkg/core/core.go", "package core\n\nfunc Version() string { return \"1.0\" }\n")
	writeFile("README.md", "# Test Project\n")
	writeFile(".gitignore", "*.exe\n/bin/\n")

	git("add", "-A")
	git("commit", "-m", "initial")

	// Create feature branch with changes across multiple directories.
	git("checkout", "-b", "feature")

	writeFile("cmd/app/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"v2\") }\n")
	writeFile("pkg/core/core.go", "package core\n\nfunc Version() string { return \"2.0\" }\n")
	writeFile("pkg/auth/auth.go", "package auth\n\nfunc Login() error { return nil }\n")
	writeFile("pkg/auth/auth_test.go", "package auth\n\nimport \"testing\"\n\nfunc TestLogin(t *testing.T) {\n\tif err := Login(); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n")
	writeFile("internal/util/helpers.go", "package util\n\nfunc Max(a, b int) int {\n\tif a > b { return a }\n\treturn b\n}\n")
	writeFile("docs/api.md", "# API Reference\n\nEndpoint documentation.\n")

	git("add", "-A")
	git("commit", "-m", "feat: add auth, update core, add docs")

	return dir
}

// runBinary executes the osm binary with the given arguments and returns
// stdout, stderr, and the exit error (nil if exit code 0).
func runBinary(t *testing.T, binPath, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"OSM_CONFIG=", // Prevent host config interference.
		"TERM=dumb",   // No color codes in output.
		"NO_COLOR=1",  // Belt and suspenders.
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// gitBranches returns the list of branches in a repo.
func gitBranches(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch failed: %v\n%s", err, out)
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// countSplitBranches returns the number of branches matching the split/
// prefix in the given repository directory.
func countSplitBranches(t *testing.T, dir string) int {
	t.Helper()
	count := 0
	for _, b := range gitBranches(t, dir) {
		if strings.HasPrefix(b, "split/") {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_HeuristicBatchRun
//
// The definitive test: builds the actual osm binary, creates a test git repo,
// and runs `osm pr-split -interactive=false -base=main -strategy=directory run`
// as a subprocess. Verifies:
//   - Exit code 0
//   - Output contains analysis, grouping, planning, execution markers
//   - Real git branches exist in the repo
//   - Tree hash equivalence verified
//
// The "run" command (from pr_split_14_tui_commands.js) prints:
//   "Running full PR split workflow..."
//   "✓ Analysis: N changed files"
//   "✓ Grouped into N groups (strategy)"
//   "✓ Plan created: N splits"
//   "✓ Split executed: N branches created"
//   "✅ Tree hash equivalence verified"  (or error/mismatch messages)
//   "Done in Ns"
// ---------------------------------------------------------------------------

func TestBinaryE2E_HeuristicBatchRun(t *testing.T) {
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("binary exited with error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Verify pipeline output markers (exact strings from JS run handler).
	assertContainsAny(t, stdout, "pipeline workflow header",
		"Running full PR split workflow", "PR split workflow")
	assertContainsAny(t, stdout, "analysis step",
		"Analysis:", "changed files")
	assertContainsAny(t, stdout, "execution step",
		"Split executed", "branches created", "Split completed")
	assertContainsAny(t, stdout, "equivalence check",
		"equivalence", "Equivalence", "Trees are equivalent")
	assertContainsAny(t, stdout, "pipeline completion",
		"Done in")

	// Verify actual git branches.
	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches found after binary execution, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("split branches created: %d (branches: %v)", splitCount, gitBranches(t, repoDir))

	// With 6 feature files across 4+ directories, expect multiple splits.
	if splitCount < 2 {
		t.Errorf("expected at least 2 split branches for multi-directory feature, got %d", splitCount)
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_IndividualCommandsBatch
//
// Dispatches analyze → group → plan → execute as individual batch commands
// in a SINGLE binary invocation. This proves the batch dispatch loop works:
// each positional arg is dispatched via TUIManager.ExecuteCommand, and
// in-process state (st.analysisCache → st.groupsCache → st.planCache →
// st.executionResultCache) carries through.
// ---------------------------------------------------------------------------

func TestBinaryE2E_IndividualCommandsBatch(t *testing.T) {
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		// Each positional arg dispatched as a TUI command in order.
		"analyze", "group", "plan", "execute",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("batch commands failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Each step should produce output.
	assertContainsAny(t, stdout, "analyze output",
		"Analyzing diff", "changed files", "Analysis")
	assertContainsAny(t, stdout, "group output",
		"Groups", "groups", "Grouped")
	assertContainsAny(t, stdout, "plan output",
		"Plan created", "splits", "plan")
	assertContainsAny(t, stdout, "execute output",
		"Split completed", "branches created", "Split executed")

	// Verify git branches were created.
	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches after batch execute, branches: %v",
			gitBranches(t, repoDir))
	}
	if splitCount < 2 {
		t.Errorf("expected at least 2 split branches, got %d", splitCount)
	}
	t.Logf("batch execute created %d split branches", splitCount)
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_FullPipelineWithCleanup
//
// Runs the entire pipeline INCLUDING cleanup in a single binary invocation:
// analyze → group → plan → execute → equivalence → cleanup.
// Verifies that after cleanup, NO split branches remain.
// ---------------------------------------------------------------------------

func TestBinaryE2E_FullPipelineWithCleanup(t *testing.T) {
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"analyze", "group", "plan", "execute", "equivalence", "cleanup",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("full pipeline failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Equivalence check should pass.
	assertContainsAny(t, stdout, "equivalence result",
		"Trees are equivalent", "equivalent")

	// Cleanup should report deleted branches.
	assertContainsAny(t, stdout, "cleanup output",
		"Deleted branches", "deleted", "Deleted")

	// After cleanup, NO split branches should remain.
	branches := gitBranches(t, repoDir)
	for _, b := range branches {
		if strings.HasPrefix(b, "split/") {
			t.Errorf("split branch %q still exists after cleanup", b)
		}
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_DryRunNoExecution
//
// Verifies --dry-run flag: the "run" command shows the plan preview but
// does NOT execute splits (no branches created). The JS run handler checks
// runtime.dryRun and returns early after printing "DRY RUN — plan preview:".
// ---------------------------------------------------------------------------

func TestBinaryE2E_DryRunNoExecution(t *testing.T) {
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-dry-run",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Dry run should show the dry-run-specific plan preview output.
	// Avoid matching generic "plan"/"Plan" which appear in normal execution too.
	assertContainsAny(t, stdout, "dry-run plan output",
		"DRY RUN", "dry run", "preview", "Preview")

	// Dry run should NOT create any branches.
	if count := countSplitBranches(t, repoDir); count > 0 {
		t.Errorf("dry-run should NOT create branches, found %d split/* branches: %v",
			count, gitBranches(t, repoDir))
	}

	// Should NOT contain completion markers from actual execution.
	if strings.Contains(stdout, "Split executed") || strings.Contains(stdout, "Split completed") {
		t.Error("dry-run output should NOT contain execution completion markers")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_EmptyDiff
//
// Verifies the binary handles an empty feature branch (no file changes)
// gracefully — no spurious split branches, no panic.
// ---------------------------------------------------------------------------

func TestBinaryE2E_EmptyDiff(t *testing.T) {
	osmBin := buildOSMBinary(t)

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
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Create repo with base but no feature changes.
	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-m", "initial")
	git("checkout", "-b", "feature")
	// No changes on feature — identical to main.
	git("commit", "--allow-empty", "-m", "empty feature")

	stdout, stderr, err := runBinary(t, osmBin, dir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	// May exit cleanly (no error) or with a descriptive error message.
	// However, panics/crashes are NOT acceptable — must be a graceful exit.
	if err != nil {
		if strings.Contains(stderr, "panic") || strings.Contains(stderr, "runtime error") ||
			strings.Contains(stderr, "goroutine ") {
			t.Fatalf("binary panicked (not a graceful exit): %v\nstderr:\n%s", err, stderr)
		}
		t.Logf("binary returned error (acceptable for empty diff): %v", err)
	}

	// Must NOT create any split branches.
	if count := countSplitBranches(t, dir); count > 0 {
		t.Errorf("no split branches expected for empty feature, found %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_InvalidRepo
//
// Verifies the binary produces a clear error message when run outside a
// git repo. The JS "run" handler catches the error gracefully and prints
// an "Analysis failed:" message rather than panicking.
// ---------------------------------------------------------------------------

func TestBinaryE2E_InvalidRepo(t *testing.T) {
	osmBin := buildOSMBinary(t)

	// Run in a temp dir that is NOT a git repo.
	dir := t.TempDir()

	stdout, stderr, err := runBinary(t, osmBin, dir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	// The binary may exit 0 (JS handler catches errors gracefully) or
	// non-zero (if error propagates). Either is acceptable.
	if err != nil {
		t.Logf("binary exited with error (expected for non-git repo): %v", err)
	}

	// Must report a meaningful error about the missing git repository.
	combined := strings.ToLower(stdout + stderr)
	if !strings.Contains(combined, "not a git repository") &&
		!strings.Contains(combined, "failed") &&
		!strings.Contains(combined, "fatal") {
		t.Errorf("expected git-related error message, got:\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// Must NOT report success.
	if strings.Contains(stdout, "Split executed") || strings.Contains(stdout, "Split completed") {
		t.Error("should not report successful execution in non-git directory")
	}
}

// ---------------------------------------------------------------------------
// TestBinaryE2E_HelpOutput
//
// Verifies `osm help` works and mentions the pr-split command.
// ---------------------------------------------------------------------------

func TestBinaryE2E_HelpOutput(t *testing.T) {
	osmBin := buildOSMBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(osmBin, "help")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "OSM_CONFIG=")
	out, err := cmd.CombinedOutput()

	output := string(out)
	t.Logf("help output:\n%s", output)

	// help should produce non-empty output.
	if len(output) == 0 {
		t.Error("help produced empty output")
	}

	// Should mention pr-split as an available command.
	if !strings.Contains(output, "pr-split") {
		t.Errorf("help should mention pr-split command")
	}

	_ = err // help may or may not return error depending on implementation
}

// ---------------------------------------------------------------------------
// assertContainsAny checks that stdout contains at least one of the given
// substrings (case-insensitive). On failure, logs the full output.
// ---------------------------------------------------------------------------

func assertContainsAny(t *testing.T, output, label string, substrs ...string) {
	t.Helper()
	lower := strings.ToLower(output)
	for _, s := range substrs {
		if strings.Contains(lower, strings.ToLower(s)) {
			return
		}
	}
	t.Errorf("%s: expected one of %v in output, got:\n%s", label, substrs, output)
}
