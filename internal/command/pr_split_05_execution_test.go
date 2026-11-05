package command

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Chunk 05: Execution — executeSplit
//  Tests use real git repos to verify branch creation, file checkout, deletion.
// ---------------------------------------------------------------------------

// setupExecRepo creates a git repo with a branch containing modifications,
// additions, and deletions relative to main. Returns dir and a map of file→status.
func setupExecRepo(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := initGitRepo(t)

	// Create files on main and commit.
	writeFile(t, filepath.Join(dir, "keep.go"), "package keep")
	writeFile(t, filepath.Join(dir, "delete-me.go"), "package deleteme")
	writeFile(t, filepath.Join(dir, "modify.go"), "package modify // v1")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base commit")

	// Create feature branch with changes.
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "modify.go"), "package modify // v2")
	writeFile(t, filepath.Join(dir, "new-file.go"), "package newfile")
	gitCmd(t, dir, "rm", "delete-me.go")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature changes")

	statuses := map[string]string{
		"modify.go":    "M",
		"new-file.go":  "A",
		"delete-me.go": "D",
	}
	return dir, statuses
}

func TestChunk05_ExecuteSplit_BasicExecution(t *testing.T) {
	t.Parallel()
	dir, statuses := setupExecRepo(t)

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	// Build a plan with 2 splits: one with modifications, one with deletion.
	statusJSON, _ := json.Marshal(statuses)

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-mods', files: ['modify.go', 'new-file.go'], message: 'modifications' },
					{ name: 'split/02-dels', files: ['delete-me.go'], message: 'deletions' }
				]
			};
			var r = await prSplit.executeSplit(plan);
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Name  string   `json:"name"`
			Files []string `json:"files"`
			SHA   string   `json:"sha"`
			Error *string  `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &execResult); err != nil {
		t.Fatal(err)
	}

	if execResult.Error != nil {
		t.Fatalf("executeSplit error: %s", *execResult.Error)
	}
	if len(execResult.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(execResult.Results))
	}

	// Both branches should have a SHA.
	for i, r := range execResult.Results {
		if r.SHA == "" {
			t.Errorf("result %d (%s) has empty SHA (executeSplit returned: %s)", i, r.Name, result)
		}
		if r.Error != nil {
			t.Errorf("result %d (%s) has error: %s", i, r.Name, *r.Error)
		}
	}

	// Verify we're back on the feature branch.
	branch := strings.TrimSpace(gitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "feature" {
		t.Errorf("current branch = %q, want 'feature'", branch)
	}

	// Verify split/01-mods has modify.go with v2 content.
	gitCmd(t, dir, "checkout", "split/01-mods")
	content := gitCmd(t, dir, "show", "HEAD:modify.go")
	if !strings.Contains(content, "v2") {
		t.Error("split/01-mods: modify.go should contain 'v2'")
	}
	// new-file.go should exist on this branch.
	content = gitCmd(t, dir, "show", "HEAD:new-file.go")
	if !strings.Contains(content, "newfile") {
		t.Error("split/01-mods: new-file.go missing")
	}

	// Verify split/02-dels has delete-me.go removed.
	gitCmd(t, dir, "checkout", "split/02-dels")
	showResult := gitCmdAllowFail(t, dir, "show", "HEAD:delete-me.go")
	if showResult.err == nil {
		t.Error("split/02-dels: delete-me.go should not exist")
	}

	// Cleanup.
	gitCmd(t, dir, "checkout", "feature")
}

func TestChunk05_ExecuteSplit_RenameRemovesSourcePath(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "old.txt"), "renamed content\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base")
	gitCmd(t, dir, "checkout", "-b", "feature")
	gitCmd(t, dir, "mv", "old.txt", "new.txt")
	gitCmd(t, dir, "commit", "-m", "rename")

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")
	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.executeSplit({
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: { 'new.txt': 'R' },
				fileRenames: { 'new.txt': 'old.txt' },
				splits: [{ name: 'split/01-rename', files: ['new.txt'], message: 'rename' }]
			});
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var execResult struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &execResult); err != nil {
		t.Fatal(err)
	}
	if execResult.Error != nil {
		t.Fatalf("executeSplit error: %s", *execResult.Error)
	}

	gitCmd(t, dir, "checkout", "split/01-rename")
	if got := gitCmdAllowFail(t, dir, "show", "HEAD:old.txt"); got.err == nil {
		t.Fatal("old.txt should not exist on rename split")
	}
	if got := gitCmd(t, dir, "show", "HEAD:new.txt"); !strings.Contains(got, "renamed content") {
		t.Fatalf("new.txt has unexpected content: %q", got)
	}
}

func TestChunk05_ExecuteSplit_InvalidPlan(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.executeSplit({ splits: [] });
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected error for empty splits")
	}
	if !strings.Contains(result.(string), "invalid plan") {
		t.Errorf("error = %q, want 'invalid plan'", result)
	}
}

func TestChunk05_ExecuteSplit_MissingFileStatuses(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.executeSplit({
				splits: [{ name: 'test', files: ['a.go'] }]
			});
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected error for missing fileStatuses")
	}
	if !strings.Contains(result.(string), "fileStatuses is required") {
		t.Errorf("error = %q, want 'fileStatuses is required'", result)
	}
}

func TestChunk05_ExecuteSplit_ProgressCallback(t *testing.T) {
	t.Parallel()
	dir, statuses := setupExecRepo(t)

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statusJSON, _ := json.Marshal(statuses)

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var messages = [];
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-all', files: ['modify.go', 'new-file.go', 'delete-me.go'], message: 'all changes' }
				]
			};
			var r = await prSplit.executeSplit(plan, {
				progressFn: function(msg) { messages.push(msg); }
			});
			return JSON.stringify({ error: r.error, messageCount: messages.length, messages: messages });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error        *string  `json:"error"`
		MessageCount int      `json:"messageCount"`
		Messages     []string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Error != nil {
		t.Fatalf("executeSplit error: %s", *data.Error)
	}
	if data.MessageCount < 2 {
		t.Errorf("expected at least 2 progress messages, got %d: %v", data.MessageCount, data.Messages)
	}

	// Cleanup.
	gitCmd(t, dir, "checkout", "feature")
}

func TestChunk05_ExecuteSplit_ReRunDeletesOldBranches(t *testing.T) {
	t.Parallel()
	dir, statuses := setupExecRepo(t)

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statusJSON, _ := json.Marshal(statuses)
	planJS := `{
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapeJSPath(dir) + `',
		fileStatuses: ` + string(statusJSON) + `,
		splits: [
			{ name: 'split/01-all', files: ['modify.go', 'new-file.go', 'delete-me.go'], message: 'all' }
		]
	}`

	// Run once.
	_, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.executeSplit(` + planJS + `);
			return r.error || 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Run again — should succeed (old branch deleted and recreated).
	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.executeSplit(` + planJS + `);
			return r.error || 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatalf("re-run failed: %v", result)
	}
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

type cmdResult struct {
	output string
	err    error
}

// gitCmdAllowFail runs a git command and returns both output and error
// without calling t.Fatal on failure.
func gitCmdAllowFail(t *testing.T, dir string, args ...string) cmdResult {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return cmdResult{output: string(out), err: err}
}

func TestChunk05_ExecuteSplit_GitIgnoredFilesSkipped(t *testing.T) {
	t.Parallel()
	// Setup: create repo with .gitignore that ignores *.log files.
	dir := initGitRepo(t)

	writeFile(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base with gitignore")

	// Feature branch: add a normal file + a force-added ignored file.
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "feature.go"), "package feature")
	writeFile(t, filepath.Join(dir, "debug.log"), "debug output")
	gitCmd(t, dir, "add", "feature.go")
	gitCmd(t, dir, "add", "-f", "debug.log") // force-add ignored file
	gitCmd(t, dir, "commit", "-m", "feature with ignored file")

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statuses := map[string]string{
		"feature.go": "A",
		"debug.log":  "A",
	}
	statusJSON, _ := json.Marshal(statuses)

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-mixed', files: ['feature.go', 'debug.log'], message: 'mixed files' }
				]
			};
			var r = await prSplit.executeSplit(plan);
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Name         string   `json:"name"`
			Files        []string `json:"files"`
			SHA          string   `json:"sha"`
			Error        *string  `json:"error"`
			SkippedFiles []string `json:"skippedFiles"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &execResult); err != nil {
		t.Fatal(err)
	}

	if execResult.Error != nil {
		t.Fatalf("executeSplit error: %s", *execResult.Error)
	}
	if len(execResult.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(execResult.Results))
	}

	r := execResult.Results[0]
	if r.SHA == "" {
		t.Error("expected non-empty SHA (executeSplit returned: " + result.(string) + ")")
	}
	if r.Error != nil {
		t.Errorf("unexpected error: %s", *r.Error)
	}

	// debug.log should be in skippedFiles because it matches .gitignore.
	if len(r.SkippedFiles) != 1 || r.SkippedFiles[0] != "debug.log" {
		t.Errorf("skippedFiles = %v, want ['debug.log']", r.SkippedFiles)
	}

	// Verify the branch was created and feature.go is present.
	gitCmd(t, dir, "checkout", "split/01-mixed")
	content := gitCmd(t, dir, "show", "HEAD:feature.go")
	if !strings.Contains(content, "feature") {
		t.Error("feature.go should exist on split branch")
	}

	// debug.log should NOT be on the split branch (it was skipped).
	showResult := gitCmdAllowFail(t, dir, "show", "HEAD:debug.log")
	if showResult.err == nil {
		t.Error("debug.log should not exist on split branch (it was git-ignored and skipped)")
	}

	gitCmd(t, dir, "checkout", "feature")
}

func TestChunk05_ExecuteSplit_NoIgnoredFiles(t *testing.T) {
	t.Parallel()
	// When no files are ignored, skippedFiles should be empty.
	dir, statuses := setupExecRepo(t)

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statusJSON, _ := json.Marshal(statuses)

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-mods', files: ['modify.go', 'new-file.go'], message: 'modifications' }
				]
			};
			var r = await prSplit.executeSplit(plan);
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			SkippedFiles []string `json:"skippedFiles"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &execResult); err != nil {
		t.Fatal(err)
	}

	if execResult.Error != nil {
		t.Fatalf("executeSplit error: %s", *execResult.Error)
	}
	if len(execResult.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(execResult.Results))
	}
	if len(execResult.Results[0].SkippedFiles) != 0 {
		t.Errorf("skippedFiles = %v, want empty", execResult.Results[0].SkippedFiles)
	}

	gitCmd(t, dir, "checkout", "feature")
}

func TestChunk05_ExecuteSplit_ResumesCompletedPrefix(t *testing.T) {
	t.Parallel()
	dir, statuses := setupExecRepo(t)
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")
	statusJSON, _ := json.Marshal(statuses)

	result, err := evalJS(`
		(async function() {
			var firstPlan = {
				baseBranch: 'main', sourceBranch: 'feature', dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [{ name: 'split/01-mods', files: ['modify.go', 'new-file.go'], message: 'modifications' }]
			};
			var first = await prSplit.executeSplitAsync(firstPlan);
			var fullPlan = {
				baseBranch: 'main', sourceBranch: 'feature', dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-mods', files: ['modify.go', 'new-file.go'], message: 'modifications' },
					{ name: 'split/02-dels', files: ['delete-me.go'], message: 'deletions' }
				]
			};
			var resumed = await prSplit.executeSplitAsync(fullPlan, { completedResults: first.results });
			return JSON.stringify({ first: first, resumed: resumed });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		First struct {
			Error   *string `json:"error"`
			Results []struct {
				Name string `json:"name"`
				SHA  string `json:"sha"`
			} `json:"results"`
		} `json:"first"`
		Resumed struct {
			Error   *string `json:"error"`
			Results []struct {
				Name string `json:"name"`
				SHA  string `json:"sha"`
			} `json:"results"`
		} `json:"resumed"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.First.Error != nil || payload.Resumed.Error != nil {
		t.Fatalf("resume execution failed: first=%v resumed=%v (executeSplitAsync returned: %s)", payload.First.Error, payload.Resumed.Error, result)
	}
	if len(payload.First.Results) != 1 || len(payload.Resumed.Results) != 2 {
		t.Fatalf("unexpected result counts: first=%d resumed=%d", len(payload.First.Results), len(payload.Resumed.Results))
	}
	if payload.Resumed.Results[0].SHA != payload.First.Results[0].SHA {
		t.Errorf("completed prefix was recreated: first=%s resumed=%s", payload.First.Results[0].SHA, payload.Resumed.Results[0].SHA)
	}
	if payload.Resumed.Results[1].Name != "split/02-dels" || payload.Resumed.Results[1].SHA == "" {
		t.Errorf("pending split was not executed: %+v", payload.Resumed.Results[1])
	}
}

// TestChunk05_ExecuteSplitAsync_RevParseFailureSurfaces pins the async
// execution contract: when rev-parse HEAD fails inside the just-committed
// worktree, the error must surface with the exec-layer reason (not an empty
// SHA, not a swallowed failure) and the worktree must be cleaned up.
func TestChunk05_ExecuteSplitAsync_RevParseFailureSurfaces(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	dir, statuses := setupExecRepo(t)

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statusJSON, _ := json.Marshal(statuses)

	// Intercept the async git path so that `rev-parse HEAD` inside the
	// temporary worktree fails after the commit, simulating a broken
	// worktree HEAD. Everything else passes through to the real git binary.
	// (Mocking _gitExecAsync keeps this cross-platform — no PATH shim.)
	if _, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var orig = prSplit._gitExecAsync;
			prSplit._gitExecAsync = async function(d, args) {
				if (args && args[0] === 'rev-parse' && args[1] === 'HEAD' && d && d !== '` + escapeJSPath(dir) + `') {
					return {stdout: '', stderr: 'fatal: bad revision\n', code: 128, error: true, message: 'exit status 128'};
				}
				return orig(d, args);
			};
		})()
	`); err != nil {
		t.Fatal(err)
	}

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-mods', files: ['modify.go', 'new-file.go'], message: 'modifications' }
				]
			};
			var r = await prSplit.executeSplitAsync(plan);
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Name  string  `json:"name"`
			SHA   string  `json:"sha"`
			Error *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &execResult); err != nil {
		t.Fatal(err)
	}

	if execResult.Error == nil {
		t.Fatal("expected top-level error for rev-parse failure")
	}
	if !strings.Contains(*execResult.Error, "rev-parse HEAD after commit failed") {
		t.Errorf("error should name the rev-parse failure, got %q", *execResult.Error)
	}
	// The exec-layer reason must survive: stderr content and/or exec message.
	if !strings.Contains(*execResult.Error, "bad revision") && !strings.Contains(*execResult.Error, "exit status") {
		t.Errorf("error should carry the exec-layer reason, got %q", *execResult.Error)
	}
	if len(execResult.Results) != 1 || execResult.Results[0].SHA != "" {
		t.Errorf("failed split must not carry a SHA, got %+v", execResult.Results)
	}

	// The worktree for the failed split must be gone.
	wtList := gitCmd(t, dir, "worktree", "list", "--porcelain")
	if strings.Contains(wtList, "split/01-mods") {
		t.Error("worktree for failed split should have been cleaned up")
	}
}

// TestChunk05_ExecuteSplit_CheckIgnoreErrorIsFatal pins that a git
// check-ignore failure other than 0 (ignored) / 1 (none ignored) — e.g. exit
// 128 or an aborted capture (error true, code -1) — aborts executeSplit with
// the exec reason instead of silently proceeding with an empty ignore list.
func TestChunk05_ExecuteSplit_CheckIgnoreErrorIsFatal(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "main.go"), "package main")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "feature.go"), "package feature")
	gitCmd(t, dir, "add", "feature.go")
	gitCmd(t, dir, "commit", "-m", "feature")

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statuses := map[string]string{"feature.go": "A"}
	statusJSON, _ := json.Marshal(statuses)

	result, err := evalJS(`
		(async function() {
			var prSplit = globalThis.prSplit;
			// Chunk 05 captures gitExec (which calls exec.execv) in a closure at
			// load time, so patch the exec binding underneath.
			var execMod = prSplit._modules.exec;
			var origExecv = execMod.execv;
			execMod.execv = function(argv) {
				if (argv && argv.indexOf('check-ignore') >= 0) {
					return { stdout: '', stderr: 'fatal: unable to access', code: 128, error: true, message: 'fatal: unable to access' };
				}
				return origExecv(argv);
			};
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [{ name: 'split/01-a', files: ['feature.go'], message: 'part 1' }]
			};
			var r = await prSplit.executeSplit(plan);
			execMod.execv = origExecv;
			return JSON.stringify({ error: r.error || '', resultCount: r.results.length });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Error       string `json:"error"`
		ResultCount int    `json:"resultCount"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.Error, "git check-ignore failed") {
		t.Errorf("error = %q, want a check-ignore failure", data.Error)
	}
	if !strings.Contains(data.Error, "unable to access") {
		t.Errorf("error = %q, want the exec-layer reason carried through", data.Error)
	}
	if data.ResultCount != 0 {
		t.Errorf("no split should have been executed, got %d results", data.ResultCount)
	}
}

// TestChunk05_ExecuteSplitAsync_EmptyShaGuard pins that a rev-parse HEAD that
// exits 0 with error true and empty stdout (an exec-layer anomaly) fails the
// split instead of publishing an empty SHA.
func TestChunk05_ExecuteSplitAsync_EmptyShaGuard(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	dir, statuses := setupExecRepo(t)

	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	statusJSON, _ := json.Marshal(statuses)

	if _, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var orig = prSplit._gitExecAsync;
			prSplit._gitExecAsync = async function(d, args) {
				if (args && args[0] === 'rev-parse' && args[1] === 'HEAD' && d && d !== '` + escapeJSPath(dir) + `') {
					return { stdout: '', stderr: '', code: 0, error: true, message: 'command output capture was cut short before EOF' };
				}
				return orig(d, args);
			};
		})()
	`); err != nil {
		t.Fatal(err)
	}

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: ` + string(statusJSON) + `,
				splits: [
					{ name: 'split/01-mods', files: ['modify.go', 'new-file.go'], message: 'modifications' }
				]
			};
			var r = await prSplit.executeSplitAsync(plan);
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Name  string  `json:"name"`
			SHA   string  `json:"sha"`
			Error *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &execResult); err != nil {
		t.Fatal(err)
	}
	if execResult.Error == nil || !strings.Contains(*execResult.Error, "rev-parse HEAD after commit failed") {
		t.Fatalf("error = %v, want the rev-parse guard to fire", execResult.Error)
	}
	if !strings.Contains(*execResult.Error, "cut short") {
		t.Errorf("error = %q, want the exec-layer reason carried through", *execResult.Error)
	}
	if len(execResult.Results) != 1 || execResult.Results[0].SHA != "" {
		t.Errorf("failed split must not carry a SHA, got %+v", execResult.Results)
	}
}
