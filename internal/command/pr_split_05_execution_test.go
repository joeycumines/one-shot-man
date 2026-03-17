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
			var r = prSplit.executeSplit(plan);
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
			t.Errorf("result %d (%s) has empty SHA", i, r.Name)
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

func TestChunk05_ExecuteSplit_InvalidPlan(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.executeSplit({ splits: [] });
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
	evalJS := prsplittest.NewChunkEngine(t, nil,
		"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation", "05_execution")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.executeSplit({
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
			var r = prSplit.executeSplit(plan, {
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
		(function() {
			var r = globalThis.prSplit.executeSplit(` + planJS + `);
			return r.error || 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Run again — should succeed (old branch deleted and recreated).
	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.executeSplit(` + planJS + `);
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
			var r = prSplit.executeSplit(plan);
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
		t.Error("expected non-empty SHA")
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
			var r = prSplit.executeSplit(plan);
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
