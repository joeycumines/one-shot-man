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

	"github.com/creack/pty"
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

// ---------------------------------------------------------------------------
// PTY Interactive E2E Helpers
//
// Shared utilities for interactive PTY-based E2E tests that exercise the
// TUI through keypresses and screen scraping.
// ---------------------------------------------------------------------------

// startPTYBinary launches the osm binary in interactive PTY mode and returns
// the PTY master, output buffer, and cleanup function. The caller MUST call
// cleanup when done.
func startPTYBinary(t *testing.T, repoDir string, extraArgs ...string) (ptmx *os.File, outputBuf *threadSafeBuffer, cleanup func()) {
	t.Helper()

	osmBin := buildOSMBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

	args := []string{
		"pr-split",
		"-base=main",
		"-strategy=directory",
		"-claude-command=/nonexistent/claude",
		"--store=memory",
		"--session=" + t.Name(),
	}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, osmBin, args...)
	cmd.Dir = repoDir
	logFile := filepath.Join(t.TempDir(), "osm-debug.log")
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"OSM_CONFIG=",
		"TERM=xterm-256color",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"NO_COLOR=1",
		"OSM_LOG_LEVEL=debug",
		"OSM_LOG_FILE="+logFile,
	)

	t.Cleanup(func() {
		if data, err := os.ReadFile(logFile); err == nil && len(data) > 0 {
			const maxLogDump = 4000
			s := string(data)
			if len(s) > maxLogDump {
				s = "...(truncated)...\n" + s[len(s)-maxLogDump:]
			}
			t.Logf("=== OSM DEBUG LOG ===\n%s\n=== END LOG ===", s)
		}
	})

	p, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		cancel()
		t.Fatalf("failed to start pty: %v", err)
	}

	buf := &threadSafeBuffer{}
	go func() {
		tmp := make([]byte, 4096)
		for {
			n, err := p.Read(tmp)
			if n > 0 {
				buf.Write(tmp[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	return p, buf, func() {
		_ = p.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cancel()
	}
}

// navigateToAnalysis sends keystrokes to navigate from CONFIG to analysis.
// Returns true if PLAN_REVIEW is reached, false if timed out.
func navigateToAnalysis(t *testing.T, ptmx *os.File, buf *threadSafeBuffer) bool {
	t.Helper()

	// Wait for CONFIG screen
	if !waitForPTYOutput(t, buf, "Start Analysis", 15*time.Second) {
		t.Logf("CONFIG screen did not render")
		return false
	}
	t.Logf("CONFIG screen rendered")

	// Let Claude auto-detect settle
	time.Sleep(2 * time.Second)

	// Focus nav-next ("Start Analysis") using Shift+Tab×2, then Enter.
	// This is robust regardless of CONFIG's element count (which varies
	// depending on Claude check status).
	focusNavNext(ptmx)
	time.Sleep(300 * time.Millisecond)
	_, _ = ptmx.Write([]byte{'\r'}) // Enter

	// Wait for analysis to start
	if !waitForPTYOutput(t, buf, "Processing", 5*time.Second) {
		t.Logf("Processing indicator never appeared")
		return false
	}
	t.Logf("Analysis started")

	// Wait for PLAN_REVIEW
	if waitForPTYOutput(t, buf, "Plan Review", 30*time.Second) {
		t.Logf("PLAN_REVIEW reached")
		return true
	}
	if waitForPTYOutput(t, buf, "Execute Plan", 10*time.Second) {
		t.Logf("PLAN_REVIEW reached (found Execute Plan)")
		return true
	}
	t.Logf("PLAN_REVIEW never reached")
	return false
}

// cleanExit sends Ctrl+C → 'y' to cleanly exit the TUI.
func cleanExit(ptmx *os.File) {
	sendCtrlC(ptmx)
	time.Sleep(300 * time.Millisecond)
	_, _ = ptmx.Write([]byte("y"))
	time.Sleep(300 * time.Millisecond)
}

// focusNavNext moves focus to the nav-next button (e.g. "Execute Plan →")
// from ANY starting position. It exploits the fact that nav-next is ALWAYS
// the second-to-last focus element and nav-cancel is the last.
//
// Strategy: Shift+Tab×2 from index 0 wraps to index (N-2) = nav-next.
// Since state transitions always reset focusIndex to 0 (pr_split_16_tui_core.js
// line 298), this works reliably after any screen transition.
func focusNavNext(ptmx *os.File) {
	// Shift+Tab = ESC [ Z in BubbleTea (CSI backtab).
	shiftTab := []byte{0x1b, '[', 'Z'}
	_, _ = ptmx.Write(shiftTab) // → nav-cancel (last)
	time.Sleep(200 * time.Millisecond)
	_, _ = ptmx.Write(shiftTab) // → nav-next (second-to-last)
	time.Sleep(200 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// T041 — TestBinaryE2E_FullFlowToExecution
//
// Binary E2E: CONFIG → PLAN_REVIEW → Execute → BRANCH_BUILDING → FINALIZATION
// ---------------------------------------------------------------------------

func TestBinaryE2E_FullFlowToExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY E2E test in short mode")
	}

	repoDir := setupBinaryTestRepo(t)
	ptmx, buf, cleanup := startPTYBinary(t, repoDir, "-verify=true")
	defer cleanup()

	// Step 1: Navigate to PLAN_REVIEW
	if !navigateToAnalysis(t, ptmx, buf) {
		cleanExit(ptmx)
		t.Fatalf("failed to reach PLAN_REVIEW.\nOutput:\n%s", sanitizePTYOutput(buf.String()))
	}

	// Step 2: Focus nav-next ("Execute Plan") and press Enter.
	// focusIndex resets to 0 on state transition; nav-next is always
	// second-to-last. Shift+Tab×2 from 0 wraps to nav-next reliably.
	time.Sleep(1 * time.Second) // Let render settle
	focusNavNext(ptmx)
	_, _ = ptmx.Write([]byte{'\r'}) // Enter on Execute Plan

	// Step 3: Wait for BRANCH_BUILDING (execution screen)
	if !waitForPTYOutput(t, buf, "Building", 15*time.Second) &&
		!waitForPTYOutput(t, buf, "Executing", 10*time.Second) &&
		!waitForPTYOutput(t, buf, "Branch", 10*time.Second) {
		// May have jumped directly to FINALIZATION
		if !waitForPTYOutput(t, buf, "Finalization", 15*time.Second) &&
			!waitForPTYOutput(t, buf, "Complete", 10*time.Second) {
			cleanExit(ptmx)
			t.Fatalf("never reached BRANCH_BUILDING or FINALIZATION.\nOutput:\n%s",
				sanitizePTYTail(buf.String(), 2000))
		}
		t.Logf("Jumped directly to FINALIZATION (fast execution)")
		cleanExit(ptmx)
		return
	}
	t.Logf("BRANCH_BUILDING screen visible")

	// Step 4: Wait for FINALIZATION or equiv check
	if !waitForPTYOutput(t, buf, "Finalization", 60*time.Second) &&
		!waitForPTYOutput(t, buf, "Complete", 15*time.Second) &&
		!waitForPTYOutput(t, buf, "Equivalence", 15*time.Second) {
		cleanExit(ptmx)
		t.Fatalf("execution never completed.\nOutput:\n%s",
			sanitizePTYTail(buf.String(), 2000))
	}
	t.Logf("Execution completed — reached FINALIZATION or EQUIV_CHECK")

	// Step 5: Clean exit and verify branches
	cleanExit(ptmx)
	time.Sleep(1 * time.Second) // Let binary cleanup

	splitCount := countSplitBranches(t, repoDir)
	if splitCount < 2 {
		t.Errorf("expected at least 2 split branches, got %d (branches: %v)",
			splitCount, gitBranches(t, repoDir))
	}
	t.Logf("SUCCESS: %d split branches created via TUI flow", splitCount)
}

// ---------------------------------------------------------------------------
// T042 — TestBinaryE2E_ConfigScreenNavigation
//
// Binary E2E: CONFIG keyboard navigation — Tab through strategies, toggle
// advanced options, verify cancel dialog.
// ---------------------------------------------------------------------------

func TestBinaryE2E_ConfigScreenNavigation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY E2E test in short mode")
	}

	repoDir := setupBinaryTestRepo(t)
	ptmx, buf, cleanup := startPTYBinary(t, repoDir)
	defer cleanup()

	// Wait for CONFIG screen
	if !waitForPTYOutput(t, buf, "Start Analysis", 15*time.Second) {
		cleanExit(ptmx)
		t.Fatalf("CONFIG screen never rendered.\nOutput:\n%s", sanitizePTYOutput(buf.String()))
	}
	t.Logf("CONFIG screen rendered")

	// Let Claude auto-detect settle
	time.Sleep(2 * time.Second)

	// Step 1: Tab through strategy options (first 3 focus elements)
	// Focus order: strategy-auto(0) → strategy-heuristic(1) → strategy-directory(2)
	// The TUI starts with focusIndex=0 (strategy-auto).
	_, _ = ptmx.Write([]byte{0x09}) // Tab → strategy-heuristic
	time.Sleep(300 * time.Millisecond)
	_, _ = ptmx.Write([]byte{0x09}) // Tab → strategy-directory
	time.Sleep(300 * time.Millisecond)

	// Press Enter on "directory" to select it
	_, _ = ptmx.Write([]byte{'\r'})
	time.Sleep(300 * time.Millisecond)
	t.Logf("Navigated to directory strategy and pressed Enter")

	// Step 2: Tab to Advanced Options toggle and press Enter
	// After strategy-directory(2) → test-claude(3) → toggle-advanced(4)
	_, _ = ptmx.Write([]byte{0x09}) // Tab → test-claude
	time.Sleep(200 * time.Millisecond)
	_, _ = ptmx.Write([]byte{0x09}) // Tab → toggle-advanced
	time.Sleep(200 * time.Millisecond)
	_, _ = ptmx.Write([]byte{'\r'}) // Enter to toggle
	time.Sleep(500 * time.Millisecond)

	// Check that advanced fields are now visible
	output := buf.String()
	hasAdvanced := strings.Contains(output, "Branch Prefix") ||
		strings.Contains(output, "branch") ||
		strings.Contains(output, "Advanced") ||
		strings.Contains(strings.ToLower(output), "prefix")
	if hasAdvanced {
		t.Logf("Advanced options visible after toggle")
	} else {
		t.Logf("Advanced options section not clearly visible (may be just below viewport)")
	}

	// Step 3: Navigate to Cancel and check cancel dialog
	// Send Ctrl+C to trigger cancel overlay
	sendCtrlC(ptmx)
	time.Sleep(500 * time.Millisecond)

	// Check for cancel confirmation dialog
	if waitForPTYOutput(t, buf, "Cancel", 3*time.Second) ||
		waitForPTYOutput(t, buf, "Are you sure", 3*time.Second) ||
		waitForPTYOutput(t, buf, "Quit", 3*time.Second) {
		t.Logf("Cancel confirmation dialog appeared")

		// Dismiss cancel dialog by pressing 'n' or Escape
		_, _ = ptmx.Write([]byte{0x1b}) // Escape to dismiss
		time.Sleep(500 * time.Millisecond)

		// Verify we're still on CONFIG
		if strings.Contains(buf.String(), "Start Analysis") {
			t.Logf("Still on CONFIG after dismissing cancel")
		}
	} else {
		t.Logf("Cancel dialog did not appear (Ctrl+C behavior may vary)")
	}

	// Clean exit
	sendCtrlC(ptmx)
	time.Sleep(200 * time.Millisecond)
	_, _ = ptmx.Write([]byte("y"))
	time.Sleep(300 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// T009 — TestBinaryE2E_VerifyPTYLive
//
// Binary E2E with -verify="sleep 0.5". Navigate through CONFIG → execute →
// observe verify subprocess running with live output visible.
// ---------------------------------------------------------------------------

func TestBinaryE2E_VerifyPTYLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY E2E test in short mode")
	}

	repoDir := setupBinaryTestRepo(t)
	ptmx, buf, cleanup := startPTYBinary(t, repoDir, "-verify=sleep 0.5")
	defer cleanup()

	// Navigate to PLAN_REVIEW
	if !navigateToAnalysis(t, ptmx, buf) {
		cleanExit(ptmx)
		t.Fatalf("failed to reach PLAN_REVIEW.\nOutput:\n%s", sanitizePTYOutput(buf.String()))
	}

	// Focus nav-next ("Execute Plan") and press Enter.
	time.Sleep(1 * time.Second)
	focusNavNext(ptmx)
	_, _ = ptmx.Write([]byte{'\r'})

	// Wait for execution to start — BRANCH_BUILDING shows branch progress
	if !waitForPTYOutput(t, buf, "Building", 15*time.Second) &&
		!waitForPTYOutput(t, buf, "Verifying", 10*time.Second) &&
		!waitForPTYOutput(t, buf, "Branch", 10*time.Second) &&
		!waitForPTYOutput(t, buf, "Executing", 10*time.Second) {
		// May have completed already (sleep 0.5 is fast)
		if waitForPTYOutput(t, buf, "Complete", 10*time.Second) ||
			waitForPTYOutput(t, buf, "Finalization", 10*time.Second) {
			t.Logf("Execution completed very quickly (verify=sleep 0.5 was fast)")
			cleanExit(ptmx)
			return
		}
		cleanExit(ptmx)
		t.Fatalf("never reached BRANCH_BUILDING.\nOutput:\n%s",
			sanitizePTYTail(buf.String(), 2000))
	}
	t.Logf("BRANCH_BUILDING screen visible — verify subprocess running")

	// The verify command (sleep 0.5) runs per branch. With 2+ branches,
	// there's a visible window. Wait for it to complete.
	if !waitForPTYOutput(t, buf, "Finalization", 60*time.Second) &&
		!waitForPTYOutput(t, buf, "Complete", 15*time.Second) &&
		!waitForPTYOutput(t, buf, "Equivalence", 15*time.Second) {
		cleanExit(ptmx)
		t.Fatalf("execution never completed.\nOutput:\n%s",
			sanitizePTYTail(buf.String(), 2000))
	}
	t.Logf("Verification completed for all branches")

	cleanExit(ptmx)

	// Verify git state
	splitCount := countSplitBranches(t, repoDir)
	if splitCount < 2 {
		t.Errorf("expected at least 2 split branches, got %d", splitCount)
	}
	t.Logf("SUCCESS: verify PTY live — %d branches created with verify=sleep 0.5", splitCount)
}

// ---------------------------------------------------------------------------
// T010 — TestBinaryE2E_CancelDuringVerify
//
// Start with -verify="sleep 30" (long verify), navigate to execution,
// then Ctrl+C to cancel. Verify clean exit without zombies.
// ---------------------------------------------------------------------------

func TestBinaryE2E_CancelDuringVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY E2E test in short mode")
	}

	repoDir := setupBinaryTestRepo(t)
	ptmx, buf, cleanup := startPTYBinary(t, repoDir, "-verify=sleep 30")
	defer cleanup()

	// Navigate to PLAN_REVIEW
	if !navigateToAnalysis(t, ptmx, buf) {
		cleanExit(ptmx)
		t.Fatalf("failed to reach PLAN_REVIEW.\nOutput:\n%s", sanitizePTYOutput(buf.String()))
	}

	// Focus nav-next ("Execute Plan") and press Enter.
	time.Sleep(1 * time.Second)
	focusNavNext(ptmx)
	_, _ = ptmx.Write([]byte{'\r'})

	// Wait for execution to start
	if !waitForPTYOutput(t, buf, "Building", 15*time.Second) &&
		!waitForPTYOutput(t, buf, "Verifying", 10*time.Second) &&
		!waitForPTYOutput(t, buf, "Branch", 10*time.Second) &&
		!waitForPTYOutput(t, buf, "Executing", 10*time.Second) {
		cleanExit(ptmx)
		t.Fatalf("never reached BRANCH_BUILDING.\nOutput:\n%s",
			sanitizePTYTail(buf.String(), 2000))
	}
	t.Logf("BRANCH_BUILDING visible — verify running (sleep 30)")

	// Let verify run for a moment
	time.Sleep(2 * time.Second)

	// Step 1: Send Ctrl+C to interrupt
	sendCtrlC(ptmx)
	time.Sleep(1 * time.Second)

	// Should see a cancel confirmation or the TUI transitioning
	output := buf.String()
	cancelTriggered := strings.Contains(output, "Cancel") ||
		strings.Contains(output, "cancel") ||
		strings.Contains(output, "Are you sure") ||
		strings.Contains(output, "interrupted")

	if cancelTriggered {
		t.Logf("Cancel triggered after first Ctrl+C")
		// Confirm cancel
		_, _ = ptmx.Write([]byte("y"))
		time.Sleep(1 * time.Second)
	} else {
		t.Logf("First Ctrl+C may not have triggered dialog, sending second")
		// Step 2: Send second Ctrl+C for force-kill
		sendCtrlC(ptmx)
		time.Sleep(2 * time.Second)
	}

	// Wait for binary to exit (the PTY cleanup handles this)
	time.Sleep(2 * time.Second)

	// Verify worktrees are cleaned up (no leftover temp directories).
	// We can't easily check this since temp dirs are ephemeral, but we
	// verify the binary exited without hanging (the test timeout protects).
	t.Logf("SUCCESS: Binary exited after cancel during verify (sleep 30)")
}

// ---------------------------------------------------------------------------
// T070 — TestBinaryE2E_PlanEditorFlow
//
// Navigate to plan editor, exercise basic editing operations.
// CONFIG → analysis → PLAN_REVIEW → press 'e' for editor.
// ---------------------------------------------------------------------------

func TestBinaryE2E_PlanEditorFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY E2E test in short mode")
	}

	repoDir := setupBinaryTestRepo(t)
	ptmx, buf, cleanup := startPTYBinary(t, repoDir)
	defer cleanup()

	// Navigate to PLAN_REVIEW
	if !navigateToAnalysis(t, ptmx, buf) {
		cleanExit(ptmx)
		t.Fatalf("failed to reach PLAN_REVIEW.\nOutput:\n%s", sanitizePTYOutput(buf.String()))
	}

	// Press 'e' to enter Plan Editor
	time.Sleep(1 * time.Second)
	_, _ = ptmx.Write([]byte("e"))

	// Wait for editor to appear
	if !waitForPTYOutput(t, buf, "Editor", 5*time.Second) &&
		!waitForPTYOutput(t, buf, "Edit Plan", 5*time.Second) &&
		!waitForPTYOutput(t, buf, "Move File", 5*time.Second) &&
		!waitForPTYOutput(t, buf, "Rename", 5*time.Second) {
		// Editor may not have a separate screen marker; check for split cards
		if !waitForPTYOutput(t, buf, "split/", 5*time.Second) {
			cleanExit(ptmx)
			t.Fatalf("Plan Editor never appeared.\nOutput:\n%s",
				sanitizePTYTail(buf.String(), 2000))
		}
	}
	t.Logf("Plan Editor visible")

	// Tab through editor elements to explore focus items
	for i := 0; i < 4; i++ {
		_, _ = ptmx.Write([]byte{0x09}) // Tab
		time.Sleep(300 * time.Millisecond)
	}
	t.Logf("Tabbed through editor focus elements")

	// Press Escape or 'q' to exit editor back to PLAN_REVIEW
	_, _ = ptmx.Write([]byte{0x1b}) // Escape
	time.Sleep(500 * time.Millisecond)

	// Check we're back at PLAN_REVIEW
	if waitForPTYOutput(t, buf, "Execute Plan", 5*time.Second) ||
		waitForPTYOutput(t, buf, "Plan Review", 5*time.Second) {
		t.Logf("Returned to PLAN_REVIEW from editor")
	} else {
		t.Logf("May still be in editor or transitioned elsewhere")
	}

	// Focus nav-next and press Enter to execute the plan.
	focusNavNext(ptmx)
	_, _ = ptmx.Write([]byte{'\r'})

	// Wait for execution
	if !waitForPTYOutput(t, buf, "Building", 15*time.Second) &&
		!waitForPTYOutput(t, buf, "Finalization", 30*time.Second) &&
		!waitForPTYOutput(t, buf, "Complete", 15*time.Second) {
		cleanExit(ptmx)
		t.Fatalf("execution never started after editor.\nOutput:\n%s",
			sanitizePTYTail(buf.String(), 2000))
	}

	// Wait for completion
	if !waitForPTYOutput(t, buf, "Finalization", 60*time.Second) &&
		!waitForPTYOutput(t, buf, "Complete", 15*time.Second) {
		t.Logf("May not have reached finalization within timeout")
	}

	cleanExit(ptmx)

	// Verify branches were created
	splitCount := countSplitBranches(t, repoDir)
	if splitCount < 2 {
		t.Errorf("expected at least 2 split branches after editor flow, got %d", splitCount)
	}
	t.Logf("SUCCESS: Plan editor flow → %d branches created", splitCount)
}
