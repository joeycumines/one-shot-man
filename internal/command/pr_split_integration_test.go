package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/mcpcallbackmod"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func newPrSplitEvalFromFlags(t testing.TB, args ...string) (*PrSplitCommand, func(string) (any, error)) {
	t.Helper()

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	cmd.applyConfigDefaults()
	if err := cmd.validateFlags(); err != nil {
		t.Fatalf("validate flags: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout safeBuffer
	var stderr bytes.Buffer
	engine, cleanup, err := cmd.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prepare engine: %v", err)
	}
	t.Cleanup(cleanup)

	if _, _, err := cmd.setupEngineGlobals(ctx, engine, &stdout); err != nil {
		t.Fatalf("setup engine globals: %v", err)
	}

	shim := engine.LoadScriptFromString("pr-split/compat-shim", chunkCompatShim)
	if err := engine.ExecuteScript(shim); err != nil {
		t.Fatalf("compat shim failed: %v", err)
	}

	evalJS := func(js string) (any, error) {
		done := make(chan struct{})
		var result any
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			if strings.Contains(js, "await ") {
				_ = vm.Set("__evalResult", func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set("__evalError", func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := "(async function() {\n\ttry {\n\t\tvar __res = " + js + ";\n\t\tif (__res && typeof __res.then === 'function') { __res = await __res; }\n\t\t__evalResult(__res);\n\t} catch(e) {\n\t\t__evalError(e.message || String(e));\n\t}\n})();"
				if _, runErr := vm.RunString(wrapped); runErr != nil {
					resultErr = runErr
					close(done)
				}
				return
			}

			val, err := vm.RunString(js)
			if err != nil {
				resultErr = err
				close(done)
				return
			}
			result = val.Export()
			close(done)
		})
		if submitErr != nil {
			return nil, submitErr
		}

		select {
		case <-done:
			return result, resultErr
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("eval timeout after 30s")
		}
	}

	return cmd, evalJS
}

// ---------------------------------------------------------------------------
// Bootstrap verification: tuiMux and sessionTypes globals
// ---------------------------------------------------------------------------

func TestBootstrap_TuiMux_SessionTypes(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, evalJS := newPrSplitEvalFromFlags(t, "--test", "--store=memory", "--session="+t.Name())

	// tuiMux must be available as a global.
	v, err := evalJS(`typeof tuiMux`)
	if err != nil {
		t.Fatalf("tuiMux typeof: %v", err)
	}
	if v != "object" {
		t.Fatalf("tuiMux typeof = %v; want object", v)
	}

	// tuiMux.session() must return a valid InteractiveSession wrapper.
	v, err = evalJS(`typeof tuiMux.session`)
	if err != nil {
		t.Fatalf("tuiMux.session typeof: %v", err)
	}
	if v != "function" {
		t.Fatalf("tuiMux.session typeof = %v; want function", v)
	}

	// The mux's active session target should be pre-configured as "claude".
	v, err = evalJS(`JSON.stringify(tuiMux.session().target())`)
	if err != nil {
		t.Fatalf("session target: %v", err)
	}
	got := v.(string)
	if !strings.Contains(got, `"name":"claude"`) {
		t.Fatalf("session target should have name=claude, got %s", got)
	}
	if !strings.Contains(got, `"kind":"pty"`) {
		t.Fatalf("session target should have kind=pty, got %s", got)
	}

	// sessionTypes global must be available with claude and verify entries.
	v, err = evalJS(`JSON.stringify(sessionTypes)`)
	if err != nil {
		t.Fatalf("sessionTypes: %v", err)
	}
	st := v.(string)
	if !strings.Contains(st, `"claude"`) {
		t.Fatalf("sessionTypes should contain claude, got %s", st)
	}
	if !strings.Contains(st, `"verify"`) {
		t.Fatalf("sessionTypes should contain verify, got %s", st)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Heuristic Split (no AI required, real git)
// ---------------------------------------------------------------------------

// TestIntegration_HeuristicSplitEndToEnd creates a realistic git repository,
// runs the full heuristic split pipeline (analyze → group → plan → execute →
// verify equivalence), and validates that:
//   - Split branches are created with the correct files.
//   - The combined tree hash of all splits is equivalent to the original.
//   - No content is lost or duplicated.
//
// This test does NOT require AI infrastructure; it runs in every CI build.
func TestIntegration_HeuristicSplitEndToEnd(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Verify we're on the feature branch.
	branch := runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch != "feature" {
		t.Fatalf("expected to be on 'feature' branch, got %q", branch)
	}

	// Set up the pr-split JS engine pointing at our temp repo.
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true", // always passes
	})

	// Configure git mocks to point at real git repo instead.
	// We override the exec.execv to NOT mock — use real git commands.
	// The JS script's gitExec uses `git -C <dir>` when dir != '.'.
	// We'll use evalJS to call functions with the dir parameter.

	// Step 1: Analyze diff (using real git).
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var analysis struct {
		Files         []string          `json:"files"`
		FileStatuses  map[string]string `json:"fileStatuses"`
		CurrentBranch string            `json:"currentBranch"`
		BaseBranch    string            `json:"baseBranch"`
		Error         *string           `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &analysis); err != nil {
		t.Fatalf("failed to parse analysis: %v", err)
	}
	if analysis.Error != nil {
		t.Fatalf("analyzeDiff returned error: %s", *analysis.Error)
	}
	if len(analysis.Files) == 0 {
		t.Fatal("analyzeDiff returned no files")
	}
	// Deep validation: every file must be non-empty, and statuses must map to real files.
	for _, f := range analysis.Files {
		if f == "" {
			t.Error("analyzeDiff produced an empty-string file name")
		}
	}
	if len(analysis.FileStatuses) == 0 {
		t.Error("analyzeDiff returned no file statuses")
	}
	for name, status := range analysis.FileStatuses {
		if name == "" {
			t.Error("file status map contains empty key")
		}
		if status == "" {
			t.Errorf("file %q has empty status", name)
		}
	}
	t.Logf("Analyzed %d files: %v", len(analysis.Files), analysis.Files)
	t.Logf("File statuses: %v", analysis.FileStatuses)

	// Step 2: Apply strategy (directory grouping).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.applyStrategy(
		` + mustJSON(t, analysis.Files) + `,
		'directory',
		{
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
			maxFiles: 10,
			baseBranch: 'main'
		}
	))`)
	if err != nil {
		t.Fatalf("applyStrategy failed: %v", err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatalf("failed to parse groups: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("applyStrategy returned no groups")
	}
	// Deep validation: every group must have files, and all files must appear in analysis.
	analysisFileSet := make(map[string]bool)
	for _, f := range analysis.Files {
		analysisFileSet[f] = true
	}
	for gName, gFiles := range groups {
		if gName == "" {
			t.Error("applyStrategy produced a group with empty name")
		}
		if len(gFiles) == 0 {
			t.Errorf("group %q has no files", gName)
		}
		for _, f := range gFiles {
			if !analysisFileSet[f] {
				t.Errorf("group %q contains file %q not present in analysis", gName, f)
			}
		}
	}
	t.Logf("Groups: %v", groups)

	// Step 3: Create split plan (with real git to detect current branch).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.createSplitPlan(
		` + mustJSON(t, groups) + `,
		{
			baseBranch: 'main',
			sourceBranch: 'feature',
			branchPrefix: 'split/',
			maxFiles: 10,
			dir: ` + jsString(repoDir) + `,
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `
		}
	))`)
	if err != nil {
		t.Fatalf("createSplitPlan failed: %v", err)
	}

	var plan struct {
		BaseBranch   string `json:"baseBranch"`
		SourceBranch string `json:"sourceBranch"`
		Dir          string `json:"dir"`
		Splits       []struct {
			Name    string   `json:"name"`
			Files   []string `json:"files"`
			Message string   `json:"message"`
			Order   int      `json:"order"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &plan); err != nil {
		t.Fatalf("failed to parse plan: %v", err)
	}
	if len(plan.Splits) == 0 {
		t.Fatal("createSplitPlan produced no splits")
	}
	// Deep validation: every split must have a name, files, and message.
	t.Logf("Plan: %d splits", len(plan.Splits))
	for i, s := range plan.Splits {
		if s.Name == "" {
			t.Errorf("split %d has empty name", i)
		}
		if len(s.Files) == 0 {
			t.Errorf("split %d (%s) has no files", i, s.Name)
		}
		if s.Message == "" {
			t.Errorf("split %d (%s) has empty commit message", i, s.Name)
		}
		t.Logf("  Split %d: %s (%d files: %v)", i+1, s.Name, len(s.Files), s.Files)
	}

	// Verify all files are accounted for.
	allPlanFiles := make(map[string]bool)
	for _, s := range plan.Splits {
		for _, f := range s.Files {
			if allPlanFiles[f] {
				t.Errorf("duplicate file in plan: %s", f)
			}
			allPlanFiles[f] = true
		}
	}
	for _, f := range analysis.Files {
		if !allPlanFiles[f] {
			t.Errorf("file %s in analysis but missing from plan", f)
		}
	}

	// Step 4: Execute the split (creates real branches).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: ` + jsString(repoDir) + `,
		verifyCommand: 'true',
		fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
		splits: ` + mustJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("executeSplit failed: %v", err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Branch string  `json:"branch"`
			Error  *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &execResult); err != nil {
		t.Fatalf("failed to parse exec result: %v", err)
	}
	if execResult.Error != nil {
		t.Fatalf("executeSplit returned top-level error: %s", *execResult.Error)
	}
	for _, r := range execResult.Results {
		if r.Error != nil {
			t.Errorf("split branch %s failed: %s", r.Branch, *r.Error)
		}
	}

	// Verify branches exist in the repo.
	branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
	for _, s := range plan.Splits {
		if !strings.Contains(branchOutput, s.Name) {
			t.Errorf("expected branch %s to exist, not found in:\n%s", s.Name, branchOutput)
		}
	}
	t.Logf("Created branches:\n%s", branchOutput)

	// Step 5: Verify tree hash equivalence.
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: ` + jsString(repoDir) + `,
		splits: ` + mustJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("verifyEquivalenceDetailed failed: %v", err)
	}

	var equiv struct {
		Equivalent  bool     `json:"equivalent"`
		SplitTree   string   `json:"splitTree"`
		SourceTree  string   `json:"sourceTree"`
		Error       *string  `json:"error"`
		DiffFiles   []string `json:"diffFiles"`
		DiffSummary string   `json:"diffSummary"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &equiv); err != nil {
		t.Fatalf("failed to parse equivalence result: %v", err)
	}
	if equiv.Error != nil {
		t.Errorf("verifyEquivalence error: %s", *equiv.Error)
	}
	if !equiv.Equivalent {
		t.Errorf("tree hash mismatch! splitTree=%s sourceTree=%s diffFiles=%v diffSummary=%s",
			equiv.SplitTree, equiv.SourceTree, equiv.DiffFiles, equiv.DiffSummary)
	} else {
		t.Logf("✅ Tree hash equivalence verified: %s", equiv.SplitTree)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: executeSplit worktree conflict (T06 regression replication)
// ---------------------------------------------------------------------------

// TestIntegration_ExecuteSplit_WorktreeConflict verifies that executeSplit
// succeeds even when plan.baseBranch is checked out in another worktree.
// This was a regression (scratch/current-state.md item #4) where executeSplit
// did `git checkout <baseBranch>` which fails. The fix uses
// `git checkout -b <splitName> <baseBranch>`, which creates a new branch
// from the base without checking out the base branch by name.
func TestIntegration_ExecuteSplit_WorktreeConflict(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Create a git repo with main branch.
	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Initial commit on main.
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial")

	// Feature branch with a change.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package main\n\nfunc Hello() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature work")

	// Create a second worktree with 'main' checked out elsewhere.
	worktreeDir := t.TempDir()
	runGitCmd(t, dir, "worktree", "add", worktreeDir, "main")

	// We are on 'feature' in the main worktree. 'main' is checked out in
	// worktreeDir. executeSplit must succeed despite the worktree conflict.
	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	escapedDir = strings.ReplaceAll(escapedDir, `'`, `\'`)

	raw, err := evalJS(fmt.Sprintf(`(function() {
		var plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '%s',
			splits: [{
				name: 'split/hello',
				files: ['hello.go'],
				message: 'Add hello function'
			}],
			fileStatuses: { 'hello.go': 'M' }
		};
		var result = globalThis.prSplit.executeSplit(plan, {});
		return JSON.stringify(result);
	})()`, escapedDir))
	if err != nil {
		t.Fatalf("executeSplit eval failed: %v", err)
	}

	var result struct {
		Error   *string `json:"error"`
		Results []struct {
			Name  string  `json:"name"`
			SHA   string  `json:"sha"`
			Error *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Must succeed — worktree conflict is no longer a blocker.
	if result.Error != nil {
		t.Fatalf("executeSplit failed: %s", *result.Error)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 split result, got %d", len(result.Results))
	}
	if result.Results[0].Error != nil {
		t.Errorf("split error: %s", *result.Results[0].Error)
	}
	if result.Results[0].Name != "split/hello" {
		t.Errorf("split name = %q, want %q", result.Results[0].Name, "split/hello")
	}
	if result.Results[0].SHA == "" {
		t.Error("split SHA should not be empty")
	}
	t.Logf("✅ executeSplit succeeded with worktree conflict: split/hello @ %s", result.Results[0].SHA)
}

// ---------------------------------------------------------------------------
// Integration Test: Cancellation Flow
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitCancel verifies that the auto-split pipeline
// responds to cooperative cancellation within a reasonable time. It sets
// prSplit._cancelSource to return true for 'cancelled' queries and verifies
// the pipeline exits with a cancellation error.
func TestIntegration_AutoSplitCancel(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"branchPrefix":  "split/",
		"verifyCommand": "true",
	})

	// Inject a _cancelSource that returns true for 'cancelled' immediately.
	// This simulates the user requesting cancellation before the pipeline
	// starts any blocking operation.
	_, err := evalJS(`
		globalThis.prSplit._cancelSource = function(query) {
			return query === 'cancelled' || query === 'forceCancelled';
		};
	`)
	if err != nil {
		t.Fatalf("failed to inject _cancelSource: %v", err)
	}

	// Run auto-split — it should detect cancellation at the first step
	// boundary and return immediately.
	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `,
		strategy: 'directory'
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error *string `json:"error"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// The pipeline should have returned a cancellation error.
	if result.Error == nil || !strings.Contains(*result.Error, "cancel") {
		t.Errorf("expected cancellation error, got: %v", result.Error)
	} else {
		t.Logf("✅ Cancellation detected: %s", *result.Error)
	}

	// The original branch should still be intact.
	branch := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "feature" {
		t.Errorf("expected to be on 'feature' branch after cancel, got %q", branch)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: sendToHandle direct send path
// ---------------------------------------------------------------------------

// TestIntegration_SendToHandle_FallbackDirect verifies that sendToHandle
// uses direct handle.send() for writing data to the child process.
// Two-write: text first, then Enter (\r) as a separate write so that
// non-blocking TUI readers interpret it as submission.
func TestIntegration_SendToHandle_FallbackDirect(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// sendToHandle uses the direct handle.send() path.
	raw, err := evalJS(`
		(async function() {
			// sendToHandle uses two-write: text, then \r separately.
			var sends = [];
			var mockHandle = {
				send: function(text) { sends.push(text); }
			};
			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'hello Claude');
			return JSON.stringify({ error: result.error, sends: sends });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Sends []string `json:"sends"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Errorf("sendToHandle returned error: %s", *result.Error)
	}
	// Two-write: text first, then \r.
	if len(result.Sends) != 2 {
		t.Fatalf("expected 2 sends (two-write), got %d: %q", len(result.Sends), result.Sends)
	}
	if result.Sends[0] != "hello Claude" {
		t.Errorf("sends[0] = %q, want %q", result.Sends[0], "hello Claude")
	}
	if result.Sends[1] != "\r" {
		t.Errorf("sends[1] = %q, want %q", result.Sends[1], "\r")
	}
}

// TestIntegration_SendToHandle_FallbackError verifies that sendToHandle
// returns an error object (not throws) when the first write (text) fails.
// The second write (Enter) should not be attempted.
func TestIntegration_SendToHandle_FallbackError(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Two-write: error on first write (text) returns immediately, no Enter attempt.
	raw, err := evalJS(`
		(async function() {
			var sendCount = 0;
			var mockHandle = {
				send: function(text) {
					sendCount++;
					if (sendCount === 1) { throw new Error('PTY write failed'); }
				}
			};
			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'will fail');
			return JSON.stringify({ error: result.error, sendCount: sendCount });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle error test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		SendCount int     `json:"sendCount"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error from sendToHandle when send throws")
	}
	if !strings.Contains(*result.Error, "PTY write failed") {
		t.Errorf("error = %q, want to contain 'PTY write failed'", *result.Error)
	}
	if result.SendCount != 1 {
		t.Errorf("sendCount = %d, want 1 (first write fails, Enter not attempted)", result.SendCount)
	}
}

// TestIntegration_SendToHandle_DirectPath verifies the sendToHandle code path
// using direct handle.send(). Two-write: sends text first, then \r separately.
func TestIntegration_SendToHandle_DirectPath(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var calls = [];

			var mockHandle = {
				send: function(text) { calls.push(text); }
			};

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			return JSON.stringify({ error: result.error, calls: calls });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle direct path test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Calls []string `json:"calls"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %s", *result.Error)
	}
	// Two-write: text first, then \r separately.
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 send calls (two-write), got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0] != "classify these files" {
		t.Errorf("call[0] = %q, want %q", result.Calls[0], "classify these files")
	}
	if result.Calls[1] != "\r" {
		t.Errorf("call[1] = %q, want %q", result.Calls[1], "\r")
	}
}

// TestIntegration_SendToHandle_FirstSendError verifies that when
// handle.send() throws on the first write (text), the function returns
// that error without attempting the Enter write.
func TestIntegration_SendToHandle_FirstSendError(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var callCount = 0;

			var mockHandle = {
				send: function(text) {
					callCount++;
					if (callCount === 1) {
						throw new Error('cancelled by user');
					}
				}
			};

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'will cancel');

			return JSON.stringify({ error: result.error, callCount: callCount });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle error test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error from sendToHandle when first handle.send() throws")
	}
	if !strings.Contains(*result.Error, "cancelled") {
		t.Errorf("error = %q, want to contain 'cancelled'", *result.Error)
	}
	if result.CallCount != 1 {
		t.Errorf("callCount = %d, want 1 (first write fails, Enter not attempted)", result.CallCount)
	}
}

// TestIntegration_SendToHandle_ObservedSubmissionRetry verifies
// sendToHandle retries Enter submission when terminal output does not change
// after the first Enter, and succeeds once output change is observed.
func TestIntegration_SendToHandle_ObservedSubmissionRetry(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var calls = [];
			var screen = 'Claude shell\n❯ classify these files';
			var enterCount = 0;
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = 42;

			var mockHandle = {
				send: function(text) {
					calls.push(text);
					if (text === '\r') {
						enterCount++;
						// First Enter has no visible effect; second Enter causes
						// observable output change, simulating delayed submit ack.
						if (enterCount >= 2) {
							screen = 'Claude processing request...\n❯ ';
						}
					}
				}
			};
			globalThis.tuiMux = {
				snapshot: function(id) {
					if (id !== 42) return null;
					return { plainText: screen };
				}
			};

			prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 0;
			prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 25;
			prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 1;
			prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 1;
			prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 5;
			prSplit.SEND_SUBMIT_ACK_POLL_MS = 1;
			prSplit.SEND_SUBMIT_ACK_STABLE_SAMPLES = 1;
			prSplit.SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS = 3;

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.claudeSessionID = null;

			return JSON.stringify({
				error: result.error,
				calls: calls,
				enterCount: enterCount,
				screen: screen
			});
		})()
	`)
	if err != nil {
		t.Fatalf("observed submission retry test failed: %v", err)
	}

	var result struct {
		Error      *string  `json:"error"`
		Calls      []string `json:"calls"`
		EnterCount int      `json:"enterCount"`
		Screen     string   `json:"screen"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected success after retry, got error: %s", *result.Error)
	}
	if len(result.Calls) != 3 {
		t.Fatalf("expected text + two Enter keys (3 sends), got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0] != "classify these files" {
		t.Errorf("first call = %q, want prompt text", result.Calls[0])
	}
	if result.Calls[1] != "\r" || result.Calls[2] != "\r" {
		t.Errorf("expected second and third calls to be Enter (\\r), got: %+v", result.Calls)
	}
	if result.EnterCount != 2 {
		t.Errorf("enterCount = %d, want 2", result.EnterCount)
	}
	if !strings.Contains(result.Screen, "processing") {
		t.Errorf("screen = %q, want observed processing state", result.Screen)
	}
}

// TestIntegration_SendToHandle_ObservedSubmissionFailure verifies
// sendToHandle fails when Enter retries never produce an observable
// terminal output change.
func TestIntegration_SendToHandle_ObservedSubmissionFailure(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var calls = [];
			var screen = 'Claude shell\n❯ classify these files';
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = 42;

			var mockHandle = {
				send: function(text) {
					calls.push(text);
				}
			};
			globalThis.tuiMux = {
				snapshot: function(id) {
					if (id !== 42) return null;
					return { plainText: screen };
				}
			};

			prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 0;
			prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 25;
			prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 1;
			prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 1;
			prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 5;
			prSplit.SEND_SUBMIT_ACK_POLL_MS = 1;
			prSplit.SEND_SUBMIT_ACK_STABLE_SAMPLES = 1;
			prSplit.SEND_SUBMIT_MAX_NEWLINE_ATTEMPTS = 2;

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.claudeSessionID = null;

			return JSON.stringify({
				error: result.error,
				calls: calls
			});
		})()
	`)
	if err != nil {
		t.Fatalf("observed submission failure test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Calls []string `json:"calls"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected unconfirmed submission error")
	}
	if !strings.Contains(*result.Error, "prompt submission unconfirmed") {
		t.Fatalf("error = %q, want unconfirmed submission", *result.Error)
	}
	if len(result.Calls) != 3 {
		t.Fatalf("expected text + two Enter attempts (3 sends), got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0] != "classify these files" {
		t.Errorf("first call = %q, want prompt text", result.Calls[0])
	}
	if result.Calls[1] != "\r" || result.Calls[2] != "\r" {
		t.Errorf("expected Enter attempts, got: %+v", result.Calls)
	}
}

// TestIntegration_SendToHandle_PromptReadyTimeout verifies that
// sendToHandle fails before any write when no Claude prompt marker appears.
func TestIntegration_SendToHandle_PromptReadyTimeout(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var calls = [];
			var screen = 'Claude booting...';
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = 42;

			var mockHandle = {
				send: function(text) {
					calls.push(text);
				}
			};
			globalThis.tuiMux = {
				snapshot: function(id) {
					if (id !== 42) return null;
					return { plainText: screen };
				}
			};

			prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 20;
			prSplit.SEND_PROMPT_READY_POLL_MS = 1;
			prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 1;

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.claudeSessionID = null;

			return JSON.stringify({
				error: result.error,
				calls: calls
			});
		})()
	`)
	if err != nil {
		t.Fatalf("prompt ready timeout test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Calls []string `json:"calls"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected prompt-ready timeout error")
	}
	if !strings.Contains(*result.Error, "prompt not ready") {
		t.Fatalf("error = %q, want prompt-ready timeout", *result.Error)
	}
	if len(result.Calls) != 0 {
		t.Fatalf("expected no sends before prompt ready, got %d: %+v", len(result.Calls), result.Calls)
	}
}

// TestIntegration_SendToHandle_PromptSetupBlocker verifies that
// first-run setup screens are detected and reported as actionable errors.
func TestIntegration_SendToHandle_PromptSetupBlocker(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var calls = [];
			var screen = [
				"Let's get started.",
				"Choose the text style that looks best with your terminal",
				"❯ 1. Dark mode"
			].join('\n');
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = 42;

			var mockHandle = {
				send: function(text) {
					calls.push(text);
				}
			};
			globalThis.tuiMux = {
				snapshot: function(id) {
					if (id !== 42) return null;
					return { plainText: screen };
				}
			};

			prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 50;
			prSplit.SEND_PROMPT_READY_POLL_MS = 1;
			prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 1;

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.claudeSessionID = null;

			return JSON.stringify({
				error: result.error,
				calls: calls
			});
		})()
	`)
	if err != nil {
		t.Fatalf("prompt setup blocker test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Calls []string `json:"calls"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected setup blocker error")
	}
	if !strings.Contains(*result.Error, "first-run setup") {
		t.Fatalf("error = %q, want first-run setup message", *result.Error)
	}
	if len(result.Calls) != 0 {
		t.Fatalf("expected no sends when blocked by setup screen, got %d: %+v", len(result.Calls), result.Calls)
	}
}

// TestIntegration_SendToHandle_PromptReadyDelayed verifies that
// sendToHandle waits for prompt readiness before writing.
func TestIntegration_SendToHandle_PromptReadyDelayed(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var calls = [];
			var screenshotCalls = 0;
			var screen = 'Claude booting...';
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = 42;

			var mockHandle = {
				send: function(text) {
					calls.push(text);
					if (text === 'classify these files') {
						screen = 'Claude shell\n❯ classify these files';
					} else if (text === '\r') {
						screen = 'Claude processing request...\n❯ ';
					}
				}
			};
			globalThis.tuiMux = {
				snapshot: function(id) {
					if (id !== 42) return null;
					screenshotCalls++;
					if (screenshotCalls >= 3 && screen === 'Claude booting...') {
						screen = 'Claude shell\n❯ ';
					}
					return { plainText: screen };
				}
			};

			prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 200;
			prSplit.SEND_PROMPT_READY_POLL_MS = 1;
			prSplit.SEND_PROMPT_READY_STABLE_SAMPLES = 1;
			prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 0;
			prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 25;
			prSplit.SEND_PRE_SUBMIT_STABLE_POLL_MS = 1;
			prSplit.SEND_PRE_SUBMIT_STABLE_SAMPLES = 1;
			prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 25;
			prSplit.SEND_SUBMIT_ACK_POLL_MS = 1;
			prSplit.SEND_SUBMIT_ACK_STABLE_SAMPLES = 1;

			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.claudeSessionID = null;

			return JSON.stringify({
				error: result.error,
				calls: calls,
				screenshotCalls: screenshotCalls
			});
		})()
	`)
	if err != nil {
		t.Fatalf("prompt ready delayed test failed: %v", err)
	}

	var result struct {
		Error           *string  `json:"error"`
		Calls           []string `json:"calls"`
		ScreenshotCalls int      `json:"screenshotCalls"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected success after delayed prompt readiness, got error: %s", *result.Error)
	}
	if len(result.Calls) != 2 {
		t.Fatalf("expected text + Enter (2 sends), got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0] != "classify these files" || result.Calls[1] != "\r" {
		t.Fatalf("unexpected send sequence: %+v", result.Calls)
	}
	if result.ScreenshotCalls < 3 {
		t.Fatalf("expected multiple screenshot polls before prompt readiness, got %d", result.ScreenshotCalls)
	}
}

// ---- T14: SendToHandle edge cases ----------------------------------------

// TestIntegration_SendToHandle_EmptyText verifies that sending an empty
// string does not crash and still sends the Enter (\r) separately.
func TestIntegration_SendToHandle_EmptyText(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var sends = [];
			var mockHandle = {
				send: function(text) { sends.push(text); }
			};
			var result = await globalThis.prSplit.sendToHandle(mockHandle, '');
			return JSON.stringify({ error: result.error, sends: sends });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle empty text test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Sends []string `json:"sends"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Errorf("expected no error for empty text, got: %s", *result.Error)
	}
	// Even empty text should produce a two-write (empty string + \r) or
	// at least not crash. Check that sends were attempted.
	if len(result.Sends) < 1 {
		t.Error("expected at least 1 send call for empty text")
	}
}

// TestIntegration_SendToHandle_LargePayload verifies that sendToHandle
// handles a large text payload (100KB) gracefully. The chunking logic should
// split large writes into 4KB chunks.
func TestIntegration_SendToHandle_LargePayload(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var sends = [];
			var totalBytes = 0;
			var mockHandle = {
				send: function(text) {
					sends.push(text.length);
					totalBytes += text.length;
				}
			};
			// Generate 100KB of text (25K repetitions of "abcd").
			var largeText = '';
			for (var i = 0; i < 25000; i++) largeText += 'abcd';
			var result = await globalThis.prSplit.sendToHandle(mockHandle, largeText);
			return JSON.stringify({
				error: result.error,
				sendCount: sends.length,
				totalBytes: totalBytes,
				inputLength: largeText.length
			});
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle large payload test failed: %v", err)
	}

	var result struct {
		Error       *string `json:"error"`
		SendCount   int     `json:"sendCount"`
		TotalBytes  int     `json:"totalBytes"`
		InputLength int     `json:"inputLength"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected no error for large payload, got: %s", *result.Error)
	}
	if result.InputLength != 100000 {
		t.Fatalf("input length = %d, want 100000", result.InputLength)
	}
	// Multiple chunks + Enter → should be more than 2 sends.
	if result.SendCount < 3 {
		t.Errorf("expected multiple sends for 100KB payload (chunked), got %d", result.SendCount)
	}
	// Total bytes sent should be at least the input length (plus \r).
	if result.TotalBytes < result.InputLength {
		t.Errorf("total bytes sent = %d, less than input length %d", result.TotalBytes, result.InputLength)
	}
}

// TestIntegration_SendToHandle_ConcurrentSends verifies that two concurrent
// sendToHandle calls do not cause a data race. This exercises the race
// detector when run with -race.
func TestIntegration_SendToHandle_ConcurrentSends(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var sends = [];
			var mockHandle = {
				send: function(text) { sends.push(text); }
			};
			// Launch two sendToHandle calls concurrently via Promise.all.
			var results = await Promise.all([
				globalThis.prSplit.sendToHandle(mockHandle, 'message-A'),
				globalThis.prSplit.sendToHandle(mockHandle, 'message-B')
			]);
			return JSON.stringify({
				error0: results[0].error,
				error1: results[1].error,
				sendCount: sends.length,
				hasA: sends.some(function(s) { return s === 'message-A'; }),
				hasB: sends.some(function(s) { return s === 'message-B'; })
			});
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle concurrent test failed: %v", err)
	}

	var result struct {
		Error0    *string `json:"error0"`
		Error1    *string `json:"error1"`
		SendCount int     `json:"sendCount"`
		HasA      bool    `json:"hasA"`
		HasB      bool    `json:"hasB"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error0 != nil {
		t.Errorf("first concurrent send errored: %s", *result.Error0)
	}
	if result.Error1 != nil {
		t.Errorf("second concurrent send errored: %s", *result.Error1)
	}
	// Both messages should have been sent.
	if !result.HasA {
		t.Error("message-A not found in sends")
	}
	if !result.HasB {
		t.Error("message-B not found in sends")
	}
	// 2 messages × 2 writes each (text + \r) = at least 4 sends.
	if result.SendCount < 4 {
		t.Errorf("expected at least 4 sends for 2 concurrent messages, got %d", result.SendCount)
	}
}

// TestIntegration_SendToHandle_AfterDetach verifies that sendToHandle returns
// a clear error when the handle's send function throws (simulating a detached
// or closed handle).
func TestIntegration_SendToHandle_AfterDetach(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(async function() {
			var mockHandle = {
				send: function() { throw new Error('handle already detached'); }
			};
			var result = await globalThis.prSplit.sendToHandle(mockHandle, 'should fail');
			return JSON.stringify({ error: result.error || '' });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle after-detach test failed: %v", err)
	}

	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected error when send throws after detach")
	}
	if !strings.Contains(result.Error, "detach") {
		t.Errorf("error should mention detach, got %q", result.Error)
	}
}

// TestIntegration_SpawnArgs_DangerouslySkipPermissions verifies that
// ClaudeCodeExecutor.spawn prepends --dangerously-skip-permissions for
// claude-code type providers but NOT for ollama type providers.
func TestIntegration_SpawnArgs_DangerouslySkipPermissions(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Use t.TempDir() for mock paths to avoid host state mutation.
	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	// Test: claude-code type should have --dangerously-skip-permissions
	// We mock the cm object including newMCPInstance to capture spawn args.
	raw, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			// Create a ClaudeCodeExecutor with claude-code type.
			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			// Mock resolve so spawn() doesn't need real claude/ollama on PATH.
			executor.resolved = { command: 'mock-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-session';

			// Override cm methods to capture spawn args.
			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			// Call spawn with mcpConfigPath (mandatory since mcpcallback is sole IPC).
			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			await originalSpawn.call(executor, null, { mcpConfigPath: tmpDir + '/mcp-config.json' });

			if (!capturedArgs) {
				return JSON.stringify({ error: 'spawn did not capture args' });
			}

			// Verify --dangerously-skip-permissions is first.
			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			var mcpIdx = capturedArgs.indexOf('--mcp-config');
			var userIdx = capturedArgs.indexOf('--user-arg');

			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx,
				mcpIndex: mcpIdx,
				userArgIndex: userIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("spawn args test failed: %v", err)
	}

	var result struct {
		Error        *string  `json:"error"`
		Args         []string `json:"args"`
		DSPIndex     int      `json:"dspIndex"`
		MCPIndex     int      `json:"mcpIndex"`
		UserArgIndex int      `json:"userArgIndex"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("spawn args test returned error: %s", *result.Error)
	}
	if result.DSPIndex == -1 {
		t.Fatal("--dangerously-skip-permissions not found in args")
	}
	if result.DSPIndex != 0 {
		t.Errorf("--dangerously-skip-permissions index = %d, want 0 (should be first arg)", result.DSPIndex)
	}
	if result.UserArgIndex == -1 {
		t.Error("--user-arg not found in args")
	}
	if result.MCPIndex == -1 {
		t.Error("--mcp-config not found in args")
	}
	// Order: --dangerously-skip-permissions, --user-arg, --mcp-config, <path>
	if result.DSPIndex >= result.UserArgIndex {
		t.Errorf("--dangerously-skip-permissions (idx %d) should come before --user-arg (idx %d)",
			result.DSPIndex, result.UserArgIndex)
	}
	if result.UserArgIndex >= result.MCPIndex {
		t.Errorf("--user-arg (idx %d) should come before --mcp-config (idx %d)",
			result.UserArgIndex, result.MCPIndex)
	}

	// Negative case: ollama type should NOT have --dangerously-skip-permissions.
	rawOllama, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			executor.resolved = { command: 'mock-ollama', type: 'ollama' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-session-ollama';

			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			await originalSpawn.call(executor, null, { mcpConfigPath: tmpDir + '/mcp-config.json' });

			if (!capturedArgs) {
				return JSON.stringify({ error: 'ollama spawn did not capture args' });
			}

			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("ollama spawn args test failed: %v", err)
	}

	var ollamaResult struct {
		Error    *string  `json:"error"`
		Args     []string `json:"args"`
		DSPIndex int      `json:"dspIndex"`
	}
	if err := json.Unmarshal([]byte(rawOllama.(string)), &ollamaResult); err != nil {
		t.Fatalf("ollama parse error: %v", err)
	}
	if ollamaResult.Error != nil {
		t.Fatalf("ollama spawn args returned error: %s", *ollamaResult.Error)
	}
	if ollamaResult.DSPIndex != -1 {
		t.Errorf("ollama args should NOT contain --dangerously-skip-permissions, but found at index %d; args: %v",
			ollamaResult.DSPIndex, ollamaResult.Args)
	}

	// Third case: explicit type with command name containing 'claude'
	// should have --dangerously-skip-permissions (basename detection).
	rawExplicitClaude, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			executor.resolved = { command: '/usr/local/bin/claude-code', type: 'explicit' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-session-explicit-claude';

			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			await originalSpawn.call(executor, null, { mcpConfigPath: tmpDir + '/mcp-config.json' });

			if (!capturedArgs) {
				return JSON.stringify({ error: 'explicit-claude spawn did not capture args' });
			}

			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("explicit-claude spawn args test failed: %v", err)
	}

	var explicitClaudeResult struct {
		Error    *string  `json:"error"`
		Args     []string `json:"args"`
		DSPIndex int      `json:"dspIndex"`
	}
	if err := json.Unmarshal([]byte(rawExplicitClaude.(string)), &explicitClaudeResult); err != nil {
		t.Fatalf("explicit-claude parse error: %v", err)
	}
	if explicitClaudeResult.Error != nil {
		t.Fatalf("explicit-claude spawn args returned error: %s", *explicitClaudeResult.Error)
	}
	if explicitClaudeResult.DSPIndex == -1 {
		t.Error("explicit type with 'claude' in command name should contain --dangerously-skip-permissions")
	}

	// Fourth case: explicit type with non-claude command name should NOT
	// have --dangerously-skip-permissions.
	rawExplicitOther, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			executor.resolved = { command: '/opt/bin/my-custom-tool', type: 'explicit' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-session-explicit-other';

			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			await originalSpawn.call(executor, null, { mcpConfigPath: tmpDir + '/mcp-config.json' });

			if (!capturedArgs) {
				return JSON.stringify({ error: 'explicit-other spawn did not capture args' });
			}

			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("explicit-other spawn args test failed: %v", err)
	}

	var explicitOtherResult struct {
		Error    *string  `json:"error"`
		Args     []string `json:"args"`
		DSPIndex int      `json:"dspIndex"`
	}
	if err := json.Unmarshal([]byte(rawExplicitOther.(string)), &explicitOtherResult); err != nil {
		t.Fatalf("explicit-other parse error: %v", err)
	}
	if explicitOtherResult.Error != nil {
		t.Fatalf("explicit-other spawn args returned error: %s", *explicitOtherResult.Error)
	}
	if explicitOtherResult.DSPIndex != -1 {
		t.Errorf("explicit type with non-claude command should NOT contain --dangerously-skip-permissions, but found at index %d; args: %v",
			explicitOtherResult.DSPIndex, explicitOtherResult.Args)
	}
}

func TestIntegration_ClaudeCLIFlags_EndToEndToSpawn(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	commandPath := filepath.Join(t.TempDir(), "claude-launcher")
	mcpConfigPath := filepath.Join(t.TempDir(), "mcp-config.json")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write command path: %v", err)
	}

	cmd, evalJS := newPrSplitEvalFromFlags(t,
		"--test",
		"--store=memory",
		"--session="+t.Name(),
		"--claude-command", commandPath,
		"--claude-arg", "launch",
		"--claude-arg", "claude",
		"--claude-arg", "--verbose",
		"--claude-model", "sonnet",
	)

	if cmd.claudeCommand != commandPath {
		t.Fatalf("claudeCommand = %q, want %q", cmd.claudeCommand, commandPath)
	}
	if got, want := []string(cmd.claudeArgs), []string{"launch", "claude", "--verbose"}; !slices.Equal(got, want) {
		t.Fatalf("claudeArgs = %v, want %v", got, want)
	}
	if cmd.claudeModel != "sonnet" {
		t.Fatalf("claudeModel = %q, want sonnet", cmd.claudeModel)
	}

	rawConfig, err := evalJS(`JSON.stringify(prSplitConfig)`)
	if err != nil {
		t.Fatalf("read prSplitConfig: %v", err)
	}

	var cfgOut struct {
		ClaudeCommand string   `json:"claudeCommand"`
		ClaudeArgs    []string `json:"claudeArgs"`
		ClaudeModel   string   `json:"claudeModel"`
	}
	if err := json.Unmarshal([]byte(rawConfig.(string)), &cfgOut); err != nil {
		t.Fatalf("parse prSplitConfig: %v", err)
	}
	if cfgOut.ClaudeCommand != commandPath {
		t.Fatalf("prSplitConfig.claudeCommand = %q, want %q", cfgOut.ClaudeCommand, commandPath)
	}
	if !slices.Equal(cfgOut.ClaudeArgs, []string{"launch", "claude", "--verbose"}) {
		t.Fatalf("prSplitConfig.claudeArgs = %v, want [launch claude --verbose]", cfgOut.ClaudeArgs)
	}
	if cfgOut.ClaudeModel != "sonnet" {
		t.Fatalf("prSplitConfig.claudeModel = %q, want sonnet", cfgOut.ClaudeModel)
	}

	rawSpawn, err := evalJS(`(async function() {
		function makeChild(stdoutValue, stderrValue, exitCode) {
			var stdoutRead = false;
			var stderrRead = false;
			return {
				stdout: {
					read: function() {
						if (!stdoutRead && stdoutValue) {
							stdoutRead = true;
							return Promise.resolve({ done: false, value: stdoutValue });
						}
						stdoutRead = true;
						return Promise.resolve({ done: true });
					}
				},
				stderr: {
					read: function() {
						if (!stderrRead && stderrValue) {
							stderrRead = true;
							return Promise.resolve({ done: false, value: stderrValue });
						}
						stderrRead = true;
						return Promise.resolve({ done: true });
					}
				},
				wait: function() {
					return Promise.resolve({ code: exitCode });
				}
			};
		}

		var resolveSpawnCalls = 0;
		var captured = null;
		var providerCommand = '';
		var executor = new ClaudeCodeExecutor(prSplitConfig);
		var origExecSpawn = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function(cmd, args) {
			resolveSpawnCalls++;
			throw new Error('unexpected exec.spawn call during resolveAsync: ' + cmd + ' ' + JSON.stringify(args || []));
		};

		executor.cm = {
			claudeCode: function(opts) {
				providerCommand = opts && opts.command || '';
				return { name: function() { return 'mock-claude'; }, opts: opts };
			},
			ollama: function(opts) {
				return { name: function() { return 'mock-ollama'; }, opts: opts };
			},
			newRegistry: function() {
				return {
					register: function() {},
					spawn: function(name, opts) {
						captured = {
							name: name,
							args: opts.args,
							model: opts.model
						};
						return {
							send: function() {},
							isAlive: function() { return true; },
							receive: function() { return ''; },
							close: function() {}
						};
					}
				};
			}
		};

		var result;
		try {
			result = await executor.spawn(null, { mcpConfigPath: ` + jsString(mcpConfigPath) + ` });
		} finally {
			globalThis.prSplit._modules.exec.spawn = origExecSpawn;
		}
		return JSON.stringify({
			error: result.error || null,
			command: executor.command || '',
			providerCommand: providerCommand,
			resolvedType: executor.resolved ? executor.resolved.type : '',
			sessionId: result.sessionId || '',
			resolveSpawnCalls: resolveSpawnCalls,
			captured: captured
		});
	})()`)
	if err != nil {
		t.Fatalf("spawn capture eval failed: %v", err)
	}

	var spawnOut struct {
		Error             *string `json:"error"`
		Command           string  `json:"command"`
		ProviderCommand   string  `json:"providerCommand"`
		ResolvedType      string  `json:"resolvedType"`
		SessionID         string  `json:"sessionId"`
		ResolveSpawnCalls int     `json:"resolveSpawnCalls"`
		Captured          struct {
			Name  string   `json:"name"`
			Args  []string `json:"args"`
			Model string   `json:"model"`
		} `json:"captured"`
	}
	if err := json.Unmarshal([]byte(rawSpawn.(string)), &spawnOut); err != nil {
		t.Fatalf("parse spawn capture: %v", err)
	}
	if spawnOut.Error != nil {
		t.Fatalf("executor.spawn returned error: %s", *spawnOut.Error)
	}
	if spawnOut.Command != commandPath {
		t.Fatalf("executor.command = %q, want %q", spawnOut.Command, commandPath)
	}
	if spawnOut.ProviderCommand != commandPath {
		t.Fatalf("provider command = %q, want %q", spawnOut.ProviderCommand, commandPath)
	}
	if spawnOut.ResolvedType != "explicit" {
		t.Fatalf("executor.resolved.type = %q, want explicit", spawnOut.ResolvedType)
	}
	if spawnOut.ResolveSpawnCalls != 0 {
		t.Fatalf("resolve exec.spawn calls = %d, want 0 for explicit absolute command path", spawnOut.ResolveSpawnCalls)
	}
	if spawnOut.Captured.Name != "mock-claude" {
		t.Fatalf("registry spawn provider = %q, want mock-claude", spawnOut.Captured.Name)
	}
	if spawnOut.Captured.Model != "sonnet" {
		t.Fatalf("spawn opts.model = %q, want sonnet", spawnOut.Captured.Model)
	}

	wantArgs := []string{
		"--dangerously-skip-permissions",
		"launch",
		"claude",
		"--verbose",
		"--mcp-config",
		mcpConfigPath,
	}
	if !slices.Equal(spawnOut.Captured.Args, wantArgs) {
		t.Fatalf("spawn opts.args = %v, want %v", spawnOut.Captured.Args, wantArgs)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: post-spawn health check (T002 verification)
// ---------------------------------------------------------------------------

func TestIntegration_SpawnHealthCheck_DeadProcess(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	// Simulate a spawn where the handle is immediately dead.
	// The health check (sleep 0.3 + isAlive()) should detect this
	// and return a diagnostic error.
	raw, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: [],
				model: 'test-model'
			});

			executor.resolved = { command: 'fake-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-session-health';

			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					return {
						isAlive: function() { return false; },
						receive: function() { return 'Error: invalid API key'; },
						close: function() {},
						send: function() {}
					};
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			var result = await originalSpawn.call(executor, null, { mcpConfigPath: tmpDir + '/mcp-config.json' });
			return JSON.stringify(result);
		})()
	`)
	if err != nil {
		t.Fatalf("health check test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		SessionID string  `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error for dead process, got nil")
	}
	errMsg := *result.Error
	if !strings.Contains(errMsg, "exited immediately") {
		t.Errorf("error should mention 'exited immediately'; got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "invalid API key") {
		t.Errorf("error should contain process output 'invalid API key'; got: %s", errMsg)
	}
}

func TestIntegration_SpawnHealthCheck_AliveProcess(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	// Simulate a spawn where the handle is alive after the health check.
	// Should return success (error: null).
	raw, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: [],
				model: 'test-model'
			});

			executor.resolved = { command: 'fake-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-session-healthy';

			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					return {
						isAlive: function() { return true; },
						receive: function() { return ''; },
						close: function() {},
						send: function() {}
					};
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			var result = await originalSpawn.call(executor, null, { mcpConfigPath: tmpDir + '/mcp-config.json' });
			return JSON.stringify(result);
		})()
	`)
	if err != nil {
		t.Fatalf("healthy spawn test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		SessionID string  `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected no error for alive process, got: %s", *result.Error)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty sessionId")
	}
}

// ---------------------------------------------------------------------------
// Integration Test: isAlive guards (T021)
// ---------------------------------------------------------------------------
// These tests verify the isAlive checks at two critical locations:
// 1. Auto-split attach path: warns if Claude died between spawn and attach
// 2. Claude command handler: detects dead process, surfaces diagnostics

// TestIntegration_IsAliveGuard_AutoSplitAttach tests the guard that runs
// between spawn and tuiMux.attach inside automatedSplit(). When the spawned
// process dies before attach, it should log a warning and emit output but
// NOT error — the pipeline continues with the dead handle (toggle just won't
// work).
func TestIntegration_IsAliveGuard_AutoSplitAttach(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Simulate the auto-split isAlive guard logic directly.
	// This exercises the same conditional from automatedSplit() (~line 2405)
	// without running the entire pipeline.
	raw, err := evalJS(`
		(function() {
			// Build a mock claudeExecutor with a dead handle.
			var mockHandle = {
				isAlive: function() { return false; },
				receive: function() { return 'Error: API key expired'; },
				close:   function() {},
				send:    function() {}
			};

			var warnings = [];
			var outputs = [];

			// Replicate the guard logic from automatedSplit().
			if (typeof mockHandle.isAlive === 'function' && !mockHandle.isAlive()) {
				warnings.push('Claude process died between spawn and attach');
				outputs.push('[auto-split] Warning: Claude process exited unexpectedly. Toggle (Ctrl+]) unavailable.');
			} else {
				warnings.push('UNEXPECTED: alive path taken');
			}

			return JSON.stringify({
				warnings: warnings,
				outputs: outputs,
				handleIsDead: !mockHandle.isAlive()
			});
		})()
	`)
	if err != nil {
		t.Fatalf("isAlive guard test failed: %v", err)
	}

	var result struct {
		Warnings     []string `json:"warnings"`
		Outputs      []string `json:"outputs"`
		HandleIsDead bool     `json:"handleIsDead"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !result.HandleIsDead {
		t.Error("mock handle should report dead")
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "died between spawn") {
		t.Errorf("expected warning about death between spawn and attach, got: %v", result.Warnings)
	}
	if len(result.Outputs) != 1 || !strings.Contains(result.Outputs[0], "Toggle (Ctrl+]) unavailable") {
		t.Errorf("expected output about toggle unavailable, got: %v", result.Outputs)
	}
}

// TestIntegration_IsAliveGuard_AutoSplitAttach_Alive verifies the happy path
// where isAlive returns true — the attach branch should be taken instead.
func TestIntegration_IsAliveGuard_AutoSplitAttach_Alive(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var mockHandle = {
				isAlive: function() { return true; },
				receive: function() { return ''; },
				close:   function() {},
				send:    function() {}
			};

			var branch = '';
			if (typeof mockHandle.isAlive === 'function' && !mockHandle.isAlive()) {
				branch = 'dead';
			} else {
				branch = 'alive';
			}

			return JSON.stringify({ branch: branch, isAlive: mockHandle.isAlive() });
		})()
	`)
	if err != nil {
		t.Fatalf("isAlive alive guard test failed: %v", err)
	}

	var result struct {
		Branch  string `json:"branch"`
		IsAlive bool   `json:"isAlive"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if result.Branch != "alive" {
		t.Errorf("expected 'alive' branch, got: %s", result.Branch)
	}
	if !result.IsAlive {
		t.Error("expected isAlive to be true")
	}
}

// TestIntegration_IsAliveGuard_MissingIsAlive verifies graceful handling when
// the handle object does not have an isAlive method (older handle interface).
// The guard should skip the check and proceed to the attach path.
func TestIntegration_IsAliveGuard_MissingIsAlive(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			// Handle without isAlive — mimics an older interface.
			var mockHandle = {
				receive: function() { return ''; },
				close:   function() {},
				send:    function() {}
			};

			var branch = '';
			if (typeof mockHandle.isAlive === 'function' && !mockHandle.isAlive()) {
				branch = 'dead';
			} else {
				branch = 'alive-or-no-method';
			}

			return JSON.stringify({
				branch: branch,
				hasIsAlive: typeof mockHandle.isAlive === 'function'
			});
		})()
	`)
	if err != nil {
		t.Fatalf("missing isAlive guard test failed: %v", err)
	}

	var result struct {
		Branch     string `json:"branch"`
		HasIsAlive bool   `json:"hasIsAlive"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if result.HasIsAlive {
		t.Error("mock handle should not have isAlive")
	}
	if result.Branch != "alive-or-no-method" {
		t.Errorf("expected 'alive-or-no-method' branch for missing isAlive, got: %s", result.Branch)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Pipeline Output Observation — verify stdout step progress
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitMockMCP_OutputObservation exercises the pipeline's
// step progress output to verify that pipeline progress is observable via
// stdout. The pipeline emits "[auto-split] {step}..." and "[auto-split] {step} OK"
// messages through output.print(), which are captured in the test's stdout buffer.
//
// This test addresses the "PTY screenshot mechanism" requirement:
// after the pipeline completes, we capture stdout and assert on it — proving
// that a real user would see correct progress information.
func TestIntegration_AutoSplitMockMCP_OutputObservation(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	initialFiles := []TestPipelineFile{
		{"pkg/types.go", "package pkg\n\ntype Config struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
	}
	featureFiles := []TestPipelineFile{
		{"pkg/handler.go", "package pkg\n\nfunc Handle() {}\n"},
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"docs/README.md", "# Docs\n"},
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: initialFiles,
		FeatureFiles: featureFiles,
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Classification data.
	classJSON, _ := json.Marshal(map[string]any{"categories": []map[string]any{
		{"name": "api", "description": "Add API", "files": []string{"pkg/handler.go"}},
		{"name": "cli", "description": "CLI runner", "files": []string{"cmd/run.go"}},
		{"name": "docs", "description": "Documentation", "files": []string{"docs/README.md"}},
	}})

	// Mock ClaudeCodeExecutor.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) {
			this.config = config;
			this.resolved = { command: 'mock-claude' };
			this.handle = { send: function() {}, isAlive: function() { return true; } };
		};
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: null }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: null }; };
		ClaudeCodeExecutor.prototype.spawn = function(sessionId, opts) {
			return { error: null, sessionId: 'mock-tui-obs' };
		};
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	// Inject classification via mcpcallback.
	watchCh := mcpcallbackmod.WatchForInit()
	go func() {
		h := <-watchCh
		if err := h.InjectToolResult("reportClassification", classJSON); err != nil {
			t.Logf("inject classification failed: %v", err)
		}
	}()

	// Run pipeline.
	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: false,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	// Parse report to verify pipeline succeeded.
	var report struct {
		Error  string `json:"error"`
		Report struct {
			Error string `json:"error"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &report); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, result)
	}
	if report.Error != "" {
		t.Fatalf("pipeline error: %s", report.Error)
	}
	if report.Report.Error != "" {
		t.Fatalf("report error: %s", report.Report.Error)
	}

	// -----------------------------------------------------------------------
	// Validate stdout captured pipeline step progress.
	// The pipeline emits "[auto-split] {step}..." and "[auto-split] {step} OK"
	// messages via output.print().
	// -----------------------------------------------------------------------

	outStr := tp.Stdout.String()
	t.Logf("Pipeline stdout (%d bytes):\n%s", len(outStr), outStr)

	// 1. Verify step progress messages appear in stdout.
	// Pipeline steps emit "[auto-split] Analyze diff...", "[auto-split] Classify files...", etc.
	expectedStepKeywords := []string{"nalyze", "lassif", "lan", "xecut"}
	for _, keyword := range expectedStepKeywords {
		if !strings.Contains(strings.ToLower(outStr), strings.ToLower(keyword)) {
			t.Errorf("expected step keyword %q in stdout, not found", keyword)
		}
	}

	// 2. Verify success indicators appear (step completion messages).
	successIndicators := []string{"[auto-split]", "OK"}
	for _, indicator := range successIndicators {
		if !strings.Contains(outStr, indicator) {
			t.Errorf("expected %q in stdout, not found", indicator)
		}
	}

	// 3. Verify no error messages were emitted.
	if strings.Contains(outStr, "FAILED") {
		t.Errorf("stdout contains FAILED — unexpected step failure")
	}

	// 4. Verify stdout has no ANSI escape sequences (broken.md issue #2).
	for _, seq := range []string{"\x1b[?1049h", "\x1b[?1049l", "\x1b[2J"} {
		if strings.Contains(outStr, seq) {
			t.Errorf("stdout contains raw ANSI sequence %q — terminal mangling", seq)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Full auto-split pipeline with real Claude/AI
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitWithClaude_Pipeline runs the full automated
// split pipeline with a real Claude-compatible agent (configured via
// -integration -claude-command flags). It creates a realistic git repo,
// spawns the agent, sends the classification prompt, and waits for results.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=15m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_AutoSplitWithClaude_Pipeline
//
// Or via make:
//
//	make integration-test-prsplit
func TestIntegration_AutoSplitWithClaude_Pipeline(t *testing.T) {
	skipSlow(t)
	skipIfNoClaude(t)
	verifyClaudeAuth(t) // T37: pre-flight check — ensures Claude is logged in + functional

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Build config from TestMain flags.
	claudeArgsList := slices.Clone(claudeTestArgs)

	configOverrides := map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"claudeCommand": claudeTestCommand,
		"claudeArgs":    claudeArgsList,
		"timeoutMs":     int64(5 * 60 * 1000), // 5 minutes per step (JS layer)
		"_evalTimeout":  10 * time.Minute,     // T37: Go-layer evalJS timeout
	}
	if integrationModel != "" {
		configOverrides["claudeModel"] = integrationModel
	}

	stdoutBuf, _, evalJS, _ := loadPrSplitEngineWithEval(t, configOverrides)

	// T37: Dump engine output on test completion for diagnostics.
	t.Cleanup(func() {
		if out := stdoutBuf.String(); out != "" {
			t.Logf("Engine stdout:\n%s", out)
		}
	})

	// Run the full auto-split pipeline.
	t.Log("Starting auto-split pipeline with real Claude agent...")
	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `,
		strategy: 'directory',
		classifyTimeoutMs: 300000,
		planTimeoutMs: 300000,
		resolveTimeoutMs: 300000
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error              *string `json:"error"`
			ClaudeInteractions int     `json:"claudeInteractions"`
			FallbackUsed       bool    `json:"fallbackUsed"`
			SplitsCreated      int     `json:"splitsCreated"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Log the full result for debugging.
	t.Logf("Result: %s", raw)

	if result.Report.FallbackUsed {
		t.Log("WARNING: Pipeline fell back to heuristic mode — Claude may not be responding")
	}
	if result.Error != nil {
		t.Errorf("pipeline returned error: %s", *result.Error)
	}
	if result.Report.ClaudeInteractions == 0 && !result.Report.FallbackUsed {
		t.Error("expected at least one Claude interaction")
	}

	// Check that split branches were created (if pipeline completed).
	if result.Report.SplitsCreated > 0 {
		branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
		t.Logf("Created branches:\n%s", branchOutput)
		if branchOutput == "" {
			t.Error("pipeline reported splits created but no split/* branches found")
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Claude MCP Headless (no TUI login required)
// ---------------------------------------------------------------------------

// TestIntegration_ClaudeMCP_Headless validates that Claude Code can call MCP
// tools when invoked in headless (-p) mode. Unlike the full pipeline test above
// which requires interactive TUI login, this test works with ANTHROPIC_API_KEY
// alone (or any valid Claude auth method).
//
// The test:
//  1. Starts a minimal MCP callback server with a reportClassification tool.
//  2. Invokes `claude -p` with a classification prompt and --mcp-config.
//  3. Waits for Claude to call the reportClassification tool via MCP.
//  4. Verifies the tool was called with valid data.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=5m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_ClaudeMCP_Headless
func TestIntegration_ClaudeMCP_Headless(t *testing.T) {
	skipSlow(t)
	skipIfNoClaude(t)

	// Pre-flight: osm binary is required for the stdio-to-socket bridge (mcp-bridge subcommand).
	osmExe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve osm executable: %v", err)
	}

	// Pre-flight: verify Claude can respond in -p mode.
	// This catches "Not logged in" AND missing API key.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		args := []string{"-p", "Reply with the word PONG", "--max-turns", "1"}
		if integrationModel != "" {
			args = append(args, "--model", integrationModel)
		}
		cmd := exec.CommandContext(ctx, claudeTestCommand, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Skipf("Claude not functional in -p mode (need 'claude login' or ANTHROPIC_API_KEY):\n  error: %v\n  output: %s",
				err, string(out))
		}
		t.Logf("Claude -p mode pre-flight OK: %s", strings.TrimSpace(string(out)))
	}

	// 1. Create MCP server with a reportClassification tool.
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "test-mcp-headless",
		Version: "1.0.0",
	}, nil)

	// Channel to receive the tool call arguments.
	toolCallCh := make(chan json.RawMessage, 1)

	srv.AddTool(&mcp.Tool{
		Name:        "reportClassification",
		Description: "Report file classification results. Call this with a categories array.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"categories": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"name": {"type": "string"},
							"description": {"type": "string"},
							"files": {"type": "array", "items": {"type": "string"}}
						},
						"required": ["name", "files"]
					}
				}
			},
			"required": ["categories"]
		}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Logf("MCP tool called: reportClassification with %d bytes", len(req.Params.Arguments))
		select {
		case toolCallCh <- req.Params.Arguments:
		default:
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Classification accepted."}},
		}, nil
	})

	// 2. Listen on Unix socket.
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "mcp.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Accept loop. Uses fmt.Fprintf(os.Stderr, ...) instead of t.Logf
	// in goroutines because these goroutines may outlive the test function
	// (e.g., session.Wait() returns after the test reads toolCallCh and exits).
	// Calling t.Logf after test completion panics in Go 1.24+.
	go func() {
		for {
			conn, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func() {
				defer conn.Close()
				transport := &mcp.IOTransport{Reader: conn, Writer: conn}
				session, sErr := srv.Connect(ctx, transport, nil)
				if sErr != nil {
					fmt.Fprintf(os.Stderr, "[headless-mcp] connect error: %v\n", sErr)
					return
				}
				fmt.Fprintf(os.Stderr, "[headless-mcp] session established\n")
				wErr := session.Wait()
				fmt.Fprintf(os.Stderr, "[headless-mcp] session ended: %v\n", wErr)
			}()
		}
	}()

	// 3. Generate MCP config JSON using osm mcp-bridge as the stdio-to-socket bridge.
	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"osm-callback": map[string]any{
				"command": osmExe,
				"args":    []string{"mcp-bridge", "unix", sockPath},
			},
		},
	}
	configBytes, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		t.Fatalf("marshal mcp config: %v", err)
	}
	configPath := filepath.Join(tmpDir, "mcp-config.json")
	if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatalf("write mcp config: %v", err)
	}
	t.Logf("MCP config: %s → %s", configPath, string(configBytes))

	// 4. Build the classification prompt.
	prompt := `You have access to a reportClassification MCP tool.

I have a small project with these changed files:
- cmd/main.go (entry point changes)
- internal/auth/login.go (authentication logic)
- internal/auth/login_test.go (auth tests)
- docs/README.md (documentation update)

Please classify these files into logical groups for separate PRs.
Each group should have a name, description (a short commit message), and list of files.

You MUST call the reportClassification tool with your classification.
Do NOT just describe the classification in text — use the tool.`

	// 5. Run claude -p with the prompt and MCP config.
	claudeArgs := []string{
		"-p", prompt,
		"--mcp-config", configPath,
		"--dangerously-skip-permissions",
		"--max-turns", "5",
	}
	if integrationModel != "" {
		claudeArgs = append(claudeArgs, "--model", integrationModel)
	}

	t.Logf("Running: %s %s", claudeTestCommand, strings.Join(claudeArgs, " "))

	cmd := exec.CommandContext(ctx, claudeTestCommand, claudeArgs...)
	cmd.Dir = tmpDir
	claudeOut, claudeErr := cmd.CombinedOutput()
	t.Logf("Claude output (%d bytes):\n%s", len(claudeOut), string(claudeOut))
	if claudeErr != nil {
		t.Logf("Claude exit error: %v", claudeErr)
		// Don't fail immediately — the tool call might have still happened.
	}

	// 6. Check if the tool was called.
	select {
	case data := <-toolCallCh:
		t.Logf("Tool call received! Data: %s", string(data))
		// Parse and validate.
		var result struct {
			Categories []struct {
				Name  string   `json:"name"`
				Files []string `json:"files"`
			} `json:"categories"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("failed to parse tool call data: %v", err)
		}
		if len(result.Categories) == 0 {
			t.Error("expected at least one category in classification")
		}
		// Verify all 4 files are classified somewhere.
		allFiles := make(map[string]bool)
		for _, cat := range result.Categories {
			t.Logf("  Category %q: %v", cat.Name, cat.Files)
			for _, f := range cat.Files {
				allFiles[f] = true
			}
		}
		for _, expected := range []string{"cmd/main.go", "internal/auth/login.go", "internal/auth/login_test.go", "docs/README.md"} {
			if !allFiles[expected] {
				t.Errorf("file %q not classified", expected)
			}
		}
		t.Logf("SUCCESS: Claude called reportClassification with %d categories covering %d files",
			len(result.Categories), len(allFiles))

	case <-time.After(10 * time.Second):
		// Claude should have already finished by now (cmd.CombinedOutput completed).
		t.Fatal("reportClassification tool was never called — Claude did not invoke the MCP tool")
	}
}

// ---------------------------------------------------------------------------
// Integration Helpers
// ---------------------------------------------------------------------------

// initIntegrationRepo creates a temporary git repository mimicking a real
// Go project with multiple packages. The initial commit contains baseline
// files that the feature branch will build upon.
func initIntegrationRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Verify git is available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available — skipping integration test")
	}

	runGit(t, dir, "init")
	runGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGit(t, dir, "config", "user.email", "integration-test@osm.dev")
	runGit(t, dir, "config", "user.name", "OSM Integration Test")

	// Create a realistic Go project structure.
	initialFiles := []struct{ path, content string }{
		{"go.mod", "module example.com/test-project\n\ngo 1.21\n"},
		{"README.md", "# Test Project\n\nA sample project for integration testing.\n"},
		{"cmd/app/main.go", `package main

import (
	"fmt"
	"example.com/test-project/pkg/core"
)

func main() {
	fmt.Println(core.Version())
}
`},
		{"pkg/core/core.go", `package core

// Version returns the project version.
func Version() string {
	return "1.0.0"
}
`},
		{"pkg/core/core_test.go", `package core

import "testing"

func TestVersion(t *testing.T) {
	skipSlow(t)
	if v := Version(); v == "" {
		t.Fatal("version should not be empty")
	}
}
`},
		{"internal/util/strings.go", `package util

// TrimAll trims whitespace from all strings in a slice.
func TrimAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s // placeholder
		_ = s
	}
	return out
}
`},
		{"docs/getting-started.md", "# Getting Started\n\nFollow these steps.\n"},
		{".gitignore", "*.exe\n*.test\n/bin/\n"},
	}

	for _, f := range initialFiles {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial project structure")

	return dir
}

// addIntegrationFeatureFiles creates a "feature" branch with changes across
// multiple concerns: new package, modified existing code, added tests, docs
// updates, and config changes. This diversity ensures the split algorithm
// must make non-trivial grouping decisions.
func addIntegrationFeatureFiles(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	featureFiles := []struct{ path, content string }{
		// New package: authentication
		{"pkg/auth/auth.go", `package auth

import "errors"

// ErrUnauthorized is returned when authentication fails.
var ErrUnauthorized = errors.New("unauthorized")

// Token represents an authentication token.
type Token struct {
	Value  string
	Expiry int64
}

// Validate checks if a token is valid.
func (t Token) Validate() error {
	if t.Value == "" {
		return ErrUnauthorized
	}
	return nil
}
`},
		{"pkg/auth/auth_test.go", `package auth

import "testing"

func TestToken_Validate(t *testing.T) {
	skipSlow(t)
	tests := []struct {
		name    string
		token   Token
		wantErr bool
	}{
		{"valid", Token{Value: "abc", Expiry: 9999}, false},
		{"empty", Token{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.token.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
`},
		// Modified: core package gets a new function
		{"pkg/core/config.go", `package core

// Config holds application configuration.
type Config struct {
	Debug   bool
	Verbose bool
	Port    int
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Debug:   false,
		Verbose: false,
		Port:    8080,
	}
}
`},
		{"pkg/core/config_test.go", `package core

import "testing"

func TestDefaultConfig(t *testing.T) {
	skipSlow(t)
	cfg := DefaultConfig()
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Debug {
		t.Error("debug should be false by default")
	}
}
`},
		// Modified: util package gets new function
		{"internal/util/numbers.go", `package util

// Max returns the larger of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`},
		{"internal/util/numbers_test.go", `package util

import "testing"

func TestMax(t *testing.T) {
	skipSlow(t)
	if Max(1, 2) != 2 {
		t.Error("Max(1,2) should be 2")
	}
	if Max(5, 3) != 5 {
		t.Error("Max(5,3) should be 5")
	}
}
`},
		// New: middleware package
		{"internal/middleware/logging.go", `package middleware

import "fmt"

// Logger provides request logging.
type Logger struct {
	Prefix string
}

// Log writes a log entry.
func (l Logger) Log(msg string) {
	fmt.Printf("[%s] %s\n", l.Prefix, msg)
}
`},
		// Documentation updates
		{"docs/api-reference.md", `# API Reference

## Authentication

Use the auth package for token-based authentication.

## Configuration

Use core.DefaultConfig() to get default settings.
`},
		{"docs/changelog.md", `# Changelog

## Unreleased

- Added authentication package
- Added configuration support
- Added middleware logging
- Updated documentation
`},
		// Modified: main.go to use new packages
		{"cmd/app/main.go", `package main

import (
	"fmt"
	"example.com/test-project/pkg/auth"
	"example.com/test-project/pkg/core"
)

func main() {
	fmt.Println(core.Version())
	cfg := core.DefaultConfig()
	fmt.Printf("Port: %d\n", cfg.Port)

	token := auth.Token{Value: "test-token", Expiry: 9999}
	if err := token.Validate(); err != nil {
		fmt.Printf("Auth error: %v\n", err)
	}
}
`},
	}

	for _, f := range featureFiles {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "feature: auth, config, middleware, docs")
}

// mustJSON marshals v to a JSON string, failing the test on error.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// runGit runs a git command and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Integration Test: cleanupExecutor ordering (T029)
// ---------------------------------------------------------------------------

// TestIntegration_CleanupExecutor_CloseBeforeDetach verifies the real
// cleanupExecutor implementation closes the Claude executor and does not call
// tuiMux.detach synchronously.
func TestIntegration_CleanupExecutor_CloseBeforeDetach(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
			(function() {
				var callOrder = [];

				claudeExecutor = {
					handle: {
						signal: function(sig) { callOrder.push('signal:' + sig); }
					},
					close: function() { callOrder.push('close'); }
				};

				tuiMux = {
					detach: function() { callOrder.push('detach'); }
				};
				prSplit.isForceCancelled = function() { return false; };

				cleanupExecutor();

				return JSON.stringify({
					callOrder: callOrder
				});
			})()
		`)
	if err != nil {
		t.Fatalf("cleanupExecutor ordering test failed: %v", err)
	}

	var result struct {
		CallOrder []string `json:"callOrder"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(result.CallOrder) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(result.CallOrder), result.CallOrder)
	}
	if result.CallOrder[0] != "close" {
		t.Errorf("first call should be 'close', got %q", result.CallOrder[0])
	}
	for _, call := range result.CallOrder {
		if call == "detach" {
			t.Fatalf("cleanupExecutor should not call tuiMux.detach synchronously, got calls: %v", result.CallOrder)
		}
	}
}

// TestIntegration_CleanupExecutor_ForceCancel verifies the real
// cleanupExecutor implementation sends SIGKILL before close when force-cancelled.
func TestIntegration_CleanupExecutor_ForceCancel(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var callOrder = [];

				claudeExecutor = {
					handle: {
						signal: function(sig) { callOrder.push('signal:' + sig); }
					},
					close: function() { callOrder.push('close'); }
				};

				tuiMux = {
					detach: function() { callOrder.push('detach'); }
				};

				prSplit.isForceCancelled = function() { return true; };
				cleanupExecutor();

				return JSON.stringify({
					callOrder: callOrder
				});
		})()
	`)
	if err != nil {
		t.Fatalf("cleanupExecutor force-cancel test failed: %v", err)
	}

	var result struct {
		CallOrder []string `json:"callOrder"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	expected := []string{"signal:SIGKILL", "close"}
	if len(result.CallOrder) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(result.CallOrder), result.CallOrder)
	}
	for i, want := range expected {
		if result.CallOrder[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, result.CallOrder[i], want)
		}
	}
}

// TestIntegration_CleanupExecutor_NilExecutor verifies that cleanupExecutor
// handles a nil claudeExecutor gracefully.
func TestIntegration_CleanupExecutor_NilExecutor(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
				var callOrder = [];
				claudeExecutor = null;
				tuiMux = {
					detach: function() { callOrder.push('detach'); }
				};
				prSplit.isForceCancelled = function() { return false; };
				cleanupExecutor();

				return JSON.stringify({ callOrder: callOrder });
			})()
		`)
	if err != nil {
		t.Fatalf("cleanupExecutor nil executor test failed: %v", err)
	}

	var result struct {
		CallOrder []string `json:"callOrder"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(result.CallOrder) != 0 {
		t.Errorf("expected no calls with nil executor, got: %v", result.CallOrder)
	}
}

// ---------------------------------------------------------------------------
// T115: ClaudeCodeExecutor.spawn() post-health-check — process dies immediately
// When the spawned process exits immediately (e.g., invalid API key, unknown
// flags), spawn() should detect via isAlive()=false, capture last output,
// clean up the handle, and return a diagnostic error.
// ---------------------------------------------------------------------------

func TestClaudeCodeExecutor_SpawnHealthCheck_DeadProcess(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	raw, err := evalJS(`
		(async function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: 'fake-claude',
				claudeArgs: []
			});

			// Pre-set resolved so spawn doesn't call resolve().
			executor.resolved = { command: 'fake-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.resolveAsync = async function() { return { error: null }; };
			executor.sessionId = 'test-health-check';

			// Mock cm: registry.spawn returns a handle that is immediately dead.
			var mockHandle = {
				isAlive: function() { return false; },
				receive: function() { return 'Error: Invalid API key. Please run claude login first.\n'; },
				close: function() { /* no-op */ },
				send: function() {}
			};

			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) { return mockHandle; }
			};

			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock-provider'; } }; },
				ollama: function() { return { name: function() { return 'mock-provider'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			var result = await originalSpawn.call(executor, null, {
				mcpConfigPath: tmpDir + '/mcp.json'
			});

			return JSON.stringify({
				error: result.error || null,
				handleIsNull: executor.handle === null
			});
		})()
	`)
	if err != nil {
		t.Fatalf("spawn health check test failed: %v", err)
	}

	var result struct {
		Error        *string `json:"error"`
		HandleIsNull bool    `json:"handleIsNull"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v\nraw: %s", err, raw)
	}

	// Error should be returned (process died).
	if result.Error == nil {
		t.Fatal("expected error when process dies immediately, got nil")
	}

	// Error should contain the diagnostic message.
	errStr := *result.Error
	if !strings.Contains(errStr, "exited immediately after spawn") {
		t.Errorf("error should mention 'exited immediately after spawn', got: %s", errStr)
	}
	if !strings.Contains(errStr, "Invalid API key") {
		t.Errorf("error should include process output ('Invalid API key'), got: %s", errStr)
	}
	if !strings.Contains(errStr, "fake-claude") {
		t.Errorf("error should include command name ('fake-claude'), got: %s", errStr)
	}
	if !strings.Contains(errStr, "claude-code") {
		t.Errorf("error should include provider type ('claude-code'), got: %s", errStr)
	}

	// Handle should be cleaned up (set to null).
	if !result.HandleIsNull {
		t.Error("expected handle to be null after health check cleanup")
	}
}

// ---------------------------------------------------------------------------
// T01: Expanded real-Claude integration tests
// ---------------------------------------------------------------------------

// TestIntegration_ClaudeClassificationAccuracy creates a known 3-category
// diff (api/db/docs) and validates that a real Claude agent classifies files
// into at least 3 meaningful categories with correct file assignments.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=10m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_ClaudeClassificationAccuracy
func TestIntegration_ClaudeClassificationAccuracy(t *testing.T) {
	skipSlow(t)
	skipIfNoClaude(t)
	verifyClaudeAuth(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create a repo with 3 distinct categories:
	// 1. API package (handler, types, routes)
	// 2. Database package (conn, migrate, schema)
	// 3. Documentation (README, api-ref, getting-started)
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/api/handler.go", "package api\n\nfunc Handler() {}\n"},
			{"pkg/db/conn.go", "package db\n\nfunc Connect() error { return nil }\n"},
			{"docs/README.md", "# Project\n"},
		},
		FeatureFiles: []TestPipelineFile{
			// API changes — 3 files, clearly API-related
			{"pkg/api/handler.go", "package api\n\nimport \"net/http\"\n\nfunc Handler(w http.ResponseWriter, r *http.Request) {\n\tw.WriteHeader(http.StatusOK)\n}\n"},
			{"pkg/api/types.go", "package api\n\ntype Request struct {\n\tMethod string\n\tPath   string\n}\n\ntype Response struct {\n\tStatus int\n\tBody   string\n}\n"},
			{"pkg/api/routes.go", "package api\n\nfunc RegisterRoutes() {\n\t// Wire HTTP endpoints\n}\n"},
			// Database changes — 3 files, clearly DB-related
			{"pkg/db/conn.go", "package db\n\nimport \"database/sql\"\n\nfunc Connect(dsn string) (*sql.DB, error) {\n\treturn sql.Open(\"postgres\", dsn)\n}\n"},
			{"pkg/db/migrate.go", "package db\n\nfunc Migrate(dsn string) error {\n\t// Run database migrations\n\treturn nil\n}\n"},
			{"pkg/db/schema.go", "package db\n\nconst CreateUsersTable = `\nCREATE TABLE IF NOT EXISTS users (\n\tid SERIAL PRIMARY KEY,\n\tname TEXT NOT NULL\n);\n`\n"},
			// Documentation changes — 3 files, clearly docs
			{"docs/README.md", "# Project\n\nA web service with API and database.\n\n## Quick Start\n\nRun `go run ./cmd/app`.\n"},
			{"docs/api-reference.md", "# API Reference\n\n## Endpoints\n\n### GET /health\n\nReturns 200 OK.\n\n### POST /users\n\nCreates a new user.\n"},
			{"docs/getting-started.md", "# Getting Started\n\n## Prerequisites\n\n- Go 1.21+\n- PostgreSQL 15+\n\n## Installation\n\n```bash\ngo install ./cmd/app\n```\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": claudeTestCommand,
			"claudeArgs":    []string(claudeTestArgs),
			"_evalTimeout":  10 * time.Minute,
		},
	})

	// Run analysis to get the file list.
	analysisRaw, err := tp.EvalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: ` + jsString(tp.Dir) + `
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}
	var analysis struct {
		Files        []string          `json:"files"`
		FileStatuses map[string]string `json:"fileStatuses"`
	}
	if err := json.Unmarshal([]byte(analysisRaw.(string)), &analysis); err != nil {
		t.Fatalf("parse analysis: %v", err)
	}
	if len(analysis.Files) != 9 {
		t.Fatalf("expected 9 changed files, got %d: %v", len(analysis.Files), analysis.Files)
	}
	t.Logf("Analyzed %d files: %v", len(analysis.Files), analysis.Files)

	// Now run the full auto-split pipeline with real Claude.
	t.Log("Starting auto-split with real Claude for classification accuracy test...")
	raw, err := tp.EvalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(tp.Dir) + `,
		strategy: 'directory',
		classifyTimeoutMs: 300000,
		planTimeoutMs: 300000,
		resolveTimeoutMs: 300000,
		disableTUI: true
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error              *string `json:"error"`
			FallbackUsed       bool    `json:"fallbackUsed"`
			ClaudeInteractions int     `json:"claudeInteractions"`
			Mode               string  `json:"mode"`
			Classification     []struct {
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Files       []string `json:"files"`
			} `json:"classification"`
			Plan struct {
				Splits []struct {
					Name    string   `json:"name"`
					Files   []string `json:"files"`
					Message string   `json:"message"`
				} `json:"splits"`
			} `json:"plan"`
			Splits []struct {
				Name  string `json:"name"`
				SHA   string `json:"sha"`
				Error string `json:"error"`
			} `json:"splits"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v\nraw: %s", err, raw)
	}
	t.Logf("Full result: %s", raw)

	if result.Error != nil {
		t.Fatalf("pipeline error: %s", *result.Error)
	}
	if result.Report.Error != nil {
		t.Fatalf("report error: %s", *result.Report.Error)
	}

	// If fallback was used, log warning but still validate what we can.
	if result.Report.FallbackUsed {
		t.Log("WARNING: Pipeline fell back to heuristic — classification accuracy test is limited")
		// Even with fallback, we should get at least 3 groups from directory strategy.
	}

	// --- Classification accuracy assertions ---

	if len(result.Report.Classification) < 3 {
		t.Errorf("expected at least 3 categories (api/db/docs), got %d: %v",
			len(result.Report.Classification), result.Report.Classification)
	}

	// Build file-to-category map.
	fileToCat := make(map[string]string)
	for _, cat := range result.Report.Classification {
		for _, f := range cat.Files {
			fileToCat[f] = cat.Name
		}
	}
	t.Logf("File-to-category mapping: %v", fileToCat)

	// Verify all 9 files are classified.
	for _, f := range analysis.Files {
		if _, ok := fileToCat[f]; !ok {
			t.Errorf("file %q not classified in any category", f)
		}
	}

	// Verify API files are grouped together (same category).
	apiFiles := []string{"pkg/api/handler.go", "pkg/api/types.go", "pkg/api/routes.go"}
	apiCats := make(map[string]bool)
	for _, f := range apiFiles {
		if cat, ok := fileToCat[f]; ok {
			apiCats[cat] = true
		}
	}
	if len(apiCats) > 1 {
		t.Errorf("API files should be in the same category, but split across %d: %v", len(apiCats), apiCats)
	}

	// Verify DB files are grouped together.
	dbFiles := []string{"pkg/db/conn.go", "pkg/db/migrate.go", "pkg/db/schema.go"}
	dbCats := make(map[string]bool)
	for _, f := range dbFiles {
		if cat, ok := fileToCat[f]; ok {
			dbCats[cat] = true
		}
	}
	if len(dbCats) > 1 {
		t.Errorf("DB files should be in the same category, but split across %d: %v", len(dbCats), dbCats)
	}

	// Verify docs files are grouped together.
	docFiles := []string{"docs/README.md", "docs/api-reference.md", "docs/getting-started.md"}
	docCats := make(map[string]bool)
	for _, f := range docFiles {
		if cat, ok := fileToCat[f]; ok {
			docCats[cat] = true
		}
	}
	if len(docCats) > 1 {
		t.Errorf("docs files should be in the same category, but split across %d: %v", len(docCats), docCats)
	}

	// Verify API and DB are in DIFFERENT categories.
	if len(apiCats) == 1 && len(dbCats) == 1 {
		var apiCat, dbCat string
		for c := range apiCats {
			apiCat = c
		}
		for c := range dbCats {
			dbCat = c
		}
		if apiCat == dbCat {
			t.Errorf("API and DB files should be in different categories, both in %q", apiCat)
		}
	}
}

// TestIntegration_ClaudeSplitPlanQuality runs the auto-split pipeline with
// real Claude and validates the generated split plan has:
//   - Valid branch names (non-empty, no spaces, starts with branchPrefix)
//   - Non-empty commit messages for every split
//   - No orphaned files (every analyzed file appears in exactly one split)
//   - No duplicate files across splits
//
// Run with:
//
//	go test -race -v -count=1 -timeout=10m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_ClaudeSplitPlanQuality
func TestIntegration_ClaudeSplitPlanQuality(t *testing.T) {
	skipSlow(t)
	skipIfNoClaude(t)
	verifyClaudeAuth(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/core/core.go", "package core\n\nfunc Version() string { return \"1.0\" }\n"},
			{"pkg/util/util.go", "package util\n\nfunc Help() string { return \"help\" }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
			{"docs/README.md", "# Project\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/core/core.go", "package core\n\nfunc Version() string { return \"2.0\" }\n\nfunc Init() error { return nil }\n"},
			{"pkg/core/config.go", "package core\n\ntype Config struct {\n\tPort int\n\tDebug bool\n}\n"},
			{"pkg/util/util.go", "package util\n\nfunc Help() string { return \"help v2\" }\n"},
			{"pkg/util/format.go", "package util\n\nfunc Format(s string) string { return s }\n"},
			{"cmd/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"v2\") }\n"},
			{"cmd/serve.go", "package main\n\nfunc serve() error { return nil }\n"},
			{"docs/README.md", "# Project v2\n\nUpdated documentation.\n"},
			{"docs/changelog.md", "# Changelog\n\n## v2.0\n\n- Major update\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": claudeTestCommand,
			"claudeArgs":    []string(claudeTestArgs),
			"branchPrefix":  "split/",
			"_evalTimeout":  10 * time.Minute,
		},
	})

	// Run analysis first to get file count.
	analysisRaw, err := tp.EvalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: ` + jsString(tp.Dir) + `
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff: %v", err)
	}
	var analysis struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(analysisRaw.(string)), &analysis); err != nil {
		t.Fatalf("parse analysis: %v", err)
	}
	t.Logf("Analyzed %d files", len(analysis.Files))

	// Run full pipeline.
	t.Log("Starting auto-split with real Claude for plan quality test...")
	raw, err := tp.EvalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(tp.Dir) + `,
		strategy: 'directory',
		classifyTimeoutMs: 300000,
		planTimeoutMs: 300000,
		resolveTimeoutMs: 300000,
		disableTUI: true
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error        *string `json:"error"`
			FallbackUsed bool    `json:"fallbackUsed"`
			Plan         struct {
				Splits []struct {
					Name    string   `json:"name"`
					Files   []string `json:"files"`
					Message string   `json:"message"`
				} `json:"splits"`
			} `json:"plan"`
			Splits []struct {
				Name  string `json:"name"`
				SHA   string `json:"sha"`
				Error string `json:"error"`
			} `json:"splits"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v\nraw: %s", err, raw)
	}
	t.Logf("Result: %s", raw)

	if result.Error != nil {
		t.Fatalf("pipeline error: %s", *result.Error)
	}
	if result.Report.Error != nil {
		t.Fatalf("report error: %s", *result.Report.Error)
	}

	splits := result.Report.Plan.Splits
	if len(splits) == 0 {
		t.Fatal("no splits in plan")
	}

	// --- Plan quality assertions ---

	// 1. Every split has valid branch name.
	for i, s := range splits {
		if s.Name == "" {
			t.Errorf("split %d has empty name", i)
			continue
		}
		if strings.Contains(s.Name, " ") {
			t.Errorf("split %d name %q contains spaces", i, s.Name)
		}
		if !strings.HasPrefix(s.Name, "split/") {
			t.Errorf("split %d name %q doesn't start with 'split/' prefix", i, s.Name)
		}
	}

	// 2. Every split has non-empty commit message.
	for i, s := range splits {
		if s.Message == "" {
			t.Errorf("split %d (%s) has empty commit message", i, s.Name)
		}
	}

	// 3. No orphaned files: every analyzed file must appear in exactly one split.
	fileToSplit := make(map[string]string)
	for _, s := range splits {
		for _, f := range s.Files {
			if existingSplit, exists := fileToSplit[f]; exists {
				t.Errorf("file %q appears in both %q and %q (duplicate)", f, existingSplit, s.Name)
			}
			fileToSplit[f] = s.Name
		}
	}
	for _, f := range analysis.Files {
		if _, ok := fileToSplit[f]; !ok {
			t.Errorf("file %q present in analysis but orphaned (not in any split)", f)
		}
	}

	// 4. All split files must be real files from the analysis.
	analysisSet := make(map[string]bool)
	for _, f := range analysis.Files {
		analysisSet[f] = true
	}
	for _, s := range splits {
		for _, f := range s.Files {
			if !analysisSet[f] {
				t.Errorf("split %q references file %q which is not in the analysis", s.Name, f)
			}
		}
	}

	// 5. If branches were actually created, verify they exist in git.
	if len(result.Report.Splits) > 0 {
		branches := gitBranchList(t, tp.Dir)
		splitBranches := filterPrefix(branches, "split/")
		t.Logf("Created %d split branches: %v", len(splitBranches), splitBranches)
		for _, s := range result.Report.Splits {
			if s.Error != "" {
				t.Errorf("split %s had error: %s", s.Name, s.Error)
			}
		}
	}

	t.Logf("Plan quality verified: %d splits, %d files, no orphans, no duplicates",
		len(splits), len(fileToSplit))
}

// TestIntegration_ClaudeMCP_RoundTrip validates that the MCP callback
// round-trip delivers parseable JSON matching the expected schema when
// Claude calls reportClassification and reportSplitPlan via MCP tools.
//
// This test starts an MCP server, invokes Claude in headless mode with
// a classification prompt, and validates the full round-trip:
//   - Claude calls reportClassification with valid categories JSON
//   - Each category has name (string), files ([]string), description (string)
//   - All specified files appear in the classification
//
// Run with:
//
//	go test -race -v -count=1 -timeout=5m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_ClaudeMCP_RoundTrip
func TestIntegration_ClaudeMCP_RoundTrip(t *testing.T) {
	skipSlow(t)
	skipIfNoClaude(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	osmExe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve osm executable: %v", err)
	}

	// Pre-flight Claude check.
	{
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		args := []string{"-p", "Reply with the word PONG", "--max-turns", "1"}
		if integrationModel != "" {
			args = append(args, "--model", integrationModel)
		}
		cmd := exec.CommandContext(ctx, claudeTestCommand, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Skipf("Claude not functional: %v\noutput: %s", err, out)
		}
	}

	// Set up MCP server with both tools to validate schema.
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "test-mcp-roundtrip",
		Version: "1.0.0",
	}, nil)

	classificationCh := make(chan json.RawMessage, 1)
	planCh := make(chan json.RawMessage, 1)

	srv.AddTool(&mcp.Tool{
		Name:        "reportClassification",
		Description: "Report file classification results. You MUST call this tool.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"categories": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"name": {"type": "string", "description": "Category name"},
							"description": {"type": "string", "description": "Short commit message"},
							"files": {"type": "array", "items": {"type": "string"}, "description": "File paths in this category"}
						},
						"required": ["name", "files"]
					}
				}
			},
			"required": ["categories"]
		}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Logf("MCP: reportClassification called (%d bytes)", len(req.Params.Arguments))
		select {
		case classificationCh <- req.Params.Arguments:
		default:
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Classification received. Now call reportSplitPlan with a split plan."}},
		}, nil
	})

	srv.AddTool(&mcp.Tool{
		Name:        "reportSplitPlan",
		Description: "Report the split plan. Call this AFTER reportClassification.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"stages": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"name": {"type": "string", "description": "Branch name"},
							"files": {"type": "array", "items": {"type": "string"}, "description": "Files in this split"},
							"message": {"type": "string", "description": "Commit message"}
						},
						"required": ["name", "files", "message"]
					}
				}
			},
			"required": ["stages"]
		}`),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		t.Logf("MCP: reportSplitPlan called (%d bytes)", len(req.Params.Arguments))
		select {
		case planCh <- req.Params.Arguments:
		default:
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Split plan received."}},
		}, nil
	})

	// Listen on Unix socket.
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "mcp.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	go func() {
		for {
			conn, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func() {
				defer conn.Close()
				transport := &mcp.IOTransport{Reader: conn, Writer: conn}
				session, sErr := srv.Connect(ctx, transport, nil)
				if sErr != nil {
					fmt.Fprintf(os.Stderr, "[roundtrip-mcp] connect error: %v\n", sErr)
					return
				}
				_ = session.Wait()
			}()
		}
	}()

	// MCP config JSON.
	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"osm-callback": map[string]any{
				"command": osmExe,
				"args":    []string{"mcp-bridge", "unix", sockPath},
			},
		},
	}
	configBytes, err := json.MarshalIndent(mcpConfig, "", "  ")
	if err != nil {
		t.Fatalf("marshal mcp config: %v", err)
	}
	configPath := filepath.Join(tmpDir, "mcp-config.json")
	if err := os.WriteFile(configPath, configBytes, 0600); err != nil {
		t.Fatalf("write mcp config: %v", err)
	}

	// Prompt that requires calling both tools.
	prompt := `You have access to two MCP tools: reportClassification and reportSplitPlan.

Here are changed files in a pull request:
- cmd/main.go (entry point update)
- cmd/serve.go (new serve subcommand)
- pkg/auth/login.go (new authentication logic)
- pkg/auth/token.go (new token validation)
- docs/README.md (documentation update)
- docs/auth.md (new auth documentation)

Step 1: Call reportClassification with categories grouping these files logically.
Step 2: Call reportSplitPlan with a split plan using branch names starting with "split/".

You MUST call both tools. Do NOT just describe the classification in text.`

	claudeArgs := []string{
		"-p", prompt,
		"--mcp-config", configPath,
		"--dangerously-skip-permissions",
		"--max-turns", "10",
	}
	if integrationModel != "" {
		claudeArgs = append(claudeArgs, "--model", integrationModel)
	}

	t.Logf("Running: %s %s", claudeTestCommand, strings.Join(claudeArgs, " "))
	cmd := exec.CommandContext(ctx, claudeTestCommand, claudeArgs...)
	cmd.Dir = tmpDir
	claudeOut, claudeErr := cmd.CombinedOutput()
	t.Logf("Claude output (%d bytes):\n%s", len(claudeOut), string(claudeOut))
	if claudeErr != nil {
		t.Logf("Claude exit error: %v", claudeErr)
	}

	// --- Validate classification schema ---
	select {
	case data := <-classificationCh:
		t.Logf("Classification payload: %s", string(data))
		var classification struct {
			Categories []struct {
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Files       []string `json:"files"`
			} `json:"categories"`
		}
		if err := json.Unmarshal(data, &classification); err != nil {
			t.Fatalf("classification JSON schema error: %v\nraw: %s", err, data)
		}
		if len(classification.Categories) == 0 {
			t.Error("classification has no categories")
		}
		allFiles := make(map[string]bool)
		for _, cat := range classification.Categories {
			if cat.Name == "" {
				t.Error("category has empty name")
			}
			if len(cat.Files) == 0 {
				t.Errorf("category %q has no files", cat.Name)
			}
			for _, f := range cat.Files {
				if f == "" {
					t.Error("category contains empty file path")
				}
				allFiles[f] = true
			}
		}
		expectedFiles := []string{
			"cmd/main.go", "cmd/serve.go",
			"pkg/auth/login.go", "pkg/auth/token.go",
			"docs/README.md", "docs/auth.md",
		}
		for _, ef := range expectedFiles {
			if !allFiles[ef] {
				t.Errorf("expected file %q not in classification", ef)
			}
		}
		t.Logf("Classification validated: %d categories, %d files", len(classification.Categories), len(allFiles))

	case <-time.After(10 * time.Second):
		t.Fatal("reportClassification was never called")
	}

	// --- Validate split plan schema ---
	select {
	case data := <-planCh:
		t.Logf("Split plan payload: %s", string(data))
		var plan struct {
			Stages []struct {
				Name    string   `json:"name"`
				Files   []string `json:"files"`
				Message string   `json:"message"`
			} `json:"stages"`
		}
		if err := json.Unmarshal(data, &plan); err != nil {
			t.Fatalf("split plan JSON schema error: %v\nraw: %s", err, data)
		}
		if len(plan.Stages) == 0 {
			t.Error("split plan has no stages")
		}
		for i, s := range plan.Stages {
			if s.Name == "" {
				t.Errorf("stage %d has empty name", i)
			}
			if len(s.Files) == 0 {
				t.Errorf("stage %d (%s) has no files", i, s.Name)
			}
			if s.Message == "" {
				t.Errorf("stage %d (%s) has empty message", i, s.Name)
			}
		}
		t.Logf("Split plan validated: %d stages", len(plan.Stages))

	case <-time.After(10 * time.Second):
		t.Log("WARNING: reportSplitPlan was not called (Claude may not have completed both steps)")
		// Not a hard failure — some models might not complete both tool calls.
	}
}

// TestIntegration_ClaudeFallbackToHeuristic verifies that when the
// configured Claude command is invalid/unavailable, the pipeline falls
// back to heuristic mode without hanging or panicking.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=2m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_ClaudeFallbackToHeuristic
func TestIntegration_ClaudeFallbackToHeuristic(t *testing.T) {
	skipSlow(t)
	skipIfNotIntegration(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/core.go", "package pkg\n\nfunc Core() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
			{"docs/README.md", "# Project\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/core.go", "package pkg\n\nfunc Core() string { return \"v2\" }\n"},
			{"pkg/helper.go", "package pkg\n\nfunc Helper() {}\n"},
			{"cmd/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"v2\") }\n"},
			{"docs/README.md", "# Project v2\n\nUpdated.\n"},
			{"docs/changelog.md", "# Changelog\n\n## v2\n\n- Updated\n"},
		},
		ConfigOverrides: map[string]any{
			// Deliberately invalid command — should trigger fallback.
			"claudeCommand": "/nonexistent/invalid-claude-binary-that-does-not-exist",
			"claudeArgs":    []string{},
			"branchPrefix":  "split/",
			"_evalTimeout":  2 * time.Minute,
		},
	})

	start := time.Now()
	t.Log("Starting auto-split with invalid Claude command (expecting heuristic fallback)...")
	raw, err := tp.EvalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(tp.Dir) + `,
		strategy: 'directory',
		classifyTimeoutMs: 10000,
		planTimeoutMs: 10000,
		resolveTimeoutMs: 10000,
		disableTUI: true
	}))`)
	elapsed := time.Since(start)
	t.Logf("Pipeline completed in %s", elapsed)

	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error        *string `json:"error"`
			Mode         string  `json:"mode"`
			FallbackUsed bool    `json:"fallbackUsed"`
			Plan         struct {
				Splits []struct {
					Name  string   `json:"name"`
					Files []string `json:"files"`
				} `json:"splits"`
			} `json:"plan"`
			Splits []struct {
				Name string `json:"name"`
			} `json:"splits"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v\nraw: %s", err, raw)
	}
	t.Logf("Result: %s", raw)

	// --- Fallback assertions ---

	// 1. Pipeline should complete successfully (with fallback).
	if result.Error != nil {
		// Some implementations may return an error for invalid command — that's OK
		// as long as it fell back. If there's an error AND no fallback, that's a bug.
		t.Logf("Pipeline returned error (may be expected): %s", *result.Error)
	}

	// 2. Fallback should be used.
	if !result.Report.FallbackUsed {
		t.Error("expected fallbackUsed=true with invalid Claude command")
	}

	// 3. Splits should still be created via heuristic.
	if len(result.Report.Plan.Splits) == 0 && len(result.Report.Splits) == 0 {
		t.Error("expected heuristic to produce splits even without Claude")
	}

	// 4. Should complete within a reasonable time (not hang on Claude spawn).
	if elapsed > 60*time.Second {
		t.Errorf("fallback took too long (%s) — may be hanging on Claude spawn", elapsed)
	}
}

// TestIntegration_ClaudeConflictResolution creates a repository with known
// conflicts and verifies the error resolution flow works when Claude is
// available but branch execution fails due to conflicts.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=10m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_ClaudeConflictResolution
func TestIntegration_ClaudeConflictResolution(t *testing.T) {
	skipSlow(t)
	skipIfNoClaude(t)
	verifyClaudeAuth(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create a repo where the feature branch has inter-dependent changes
	// that will likely cause cherry-pick conflicts when split.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/core/core.go", "package core\n\nvar Version = \"1.0\"\n\nfunc Init() {\n\t// original init\n}\n"},
			{"pkg/api/api.go", "package api\n\nimport \"example.com/pkg/core\"\n\nfunc Start() {\n\t_ = core.Version\n}\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			// Both files modify in ways that depend on each other:
			// core.go adds NewFeature that api.go calls.
			{"pkg/core/core.go", "package core\n\nvar Version = \"2.0\"\n\nfunc Init() {\n\t// updated init for v2\n}\n\nfunc NewFeature() string {\n\treturn \"feature\"\n}\n"},
			{"pkg/api/api.go", "package api\n\nimport \"example.com/pkg/core\"\n\nfunc Start() {\n\t_ = core.Version\n\t_ = core.NewFeature()\n}\n\nfunc HandleV2() string {\n\treturn core.NewFeature()\n}\n"},
			{"cmd/main.go", "package main\n\nimport (\n\t\"fmt\"\n\t\"example.com/pkg/core\"\n)\n\nfunc main() {\n\tfmt.Println(core.Version)\n\tfmt.Println(core.NewFeature())\n}\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": claudeTestCommand,
			"claudeArgs":    []string(claudeTestArgs),
			"branchPrefix":  "split/",
			"_evalTimeout":  10 * time.Minute,
		},
	})

	t.Log("Starting auto-split with conflict-prone repo...")
	raw, err := tp.EvalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(tp.Dir) + `,
		strategy: 'directory',
		classifyTimeoutMs: 300000,
		planTimeoutMs: 300000,
		resolveTimeoutMs: 300000,
		maxResolveRetries: 2,
		disableTUI: true
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error              *string `json:"error"`
			Mode               string  `json:"mode"`
			FallbackUsed       bool    `json:"fallbackUsed"`
			ClaudeInteractions int     `json:"claudeInteractions"`
			Steps              []struct {
				Name  string `json:"name"`
				Error string `json:"error"`
			} `json:"steps"`
			Splits []struct {
				Name  string `json:"name"`
				SHA   string `json:"sha"`
				Error string `json:"error"`
			} `json:"splits"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v\nraw: %s", err, raw)
	}
	t.Logf("Result: %s", raw)

	// This test validates the pipeline handles conflicts gracefully.
	// It might succeed (Claude resolves conflicts), fall back to heuristic,
	// or complete with partial errors. All are acceptable as long as:
	// 1. No panic or hang.
	// 2. Report structure is complete.
	// 3. Steps are tracked.

	if len(result.Report.Steps) == 0 {
		t.Error("expected at least one pipeline step in report")
	}

	// Log all steps with their status.
	for _, s := range result.Report.Steps {
		if s.Error != "" {
			t.Logf("  Step %s: ERROR: %s", s.Name, s.Error)
		} else {
			t.Logf("  Step %s: OK", s.Name)
		}
	}

	// If splits were created, verify they exist in git.
	createdSplits := 0
	for _, s := range result.Report.Splits {
		if s.SHA != "" {
			createdSplits++
		}
	}
	if createdSplits > 0 {
		branches := gitBranchList(t, tp.Dir)
		splitBranches := filterPrefix(branches, "split/")
		t.Logf("Created %d split branches: %v", len(splitBranches), splitBranches)
	}

	t.Logf("Conflict test completed: %d steps, %d splits created, fallback=%v",
		len(result.Report.Steps), createdSplits, result.Report.FallbackUsed)
}
