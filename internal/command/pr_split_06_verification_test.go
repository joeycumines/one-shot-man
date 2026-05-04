package command

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Chunk 06: Verification — verifySplit, verifySplits, verifyEquivalence,
//            verifyEquivalenceDetailed, cleanupBranches
// ---------------------------------------------------------------------------

var verifyChunks = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning",
	"04_validation", "05_execution", "06_verification",
}

// setupExecRepoAndExecute creates a repo, makes changes, runs executeSplit,
// then returns the dir and plan JSON for verification tests.
func setupExecRepoAndExecute(t *testing.T, evalJS func(string) (any, error)) string {
	t.Helper()
	dir := initGitRepo(t)

	// Create files on main and commit.
	writeFile(t, filepath.Join(dir, "a.go"), "package a")
	writeFile(t, filepath.Join(dir, "b.go"), "package b")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base")

	// Feature branch with changes.
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "a.go"), "package a // modified")
	writeFile(t, filepath.Join(dir, "c.go"), "package c // new")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature")

	// Execute split.
	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: { 'a.go': 'M', 'c.go': 'A' },
				splits: [
					{ name: 'split/01-a', files: ['a.go'], message: 'modify a' },
					{ name: 'split/02-c', files: ['c.go'], message: 'add c' }
				]
			};
			var r = prSplit.executeSplit(plan);
			return r.error || 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatalf("executeSplit failed: %v", result)
	}

	return dir
}

func TestChunk06_VerifyEquivalence_Equivalent(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)
	dir := setupExecRepoAndExecute(t, evalJS)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.verifyEquivalence({
				dir: '` + escapeJSPath(dir) + `',
				sourceBranch: 'feature',
				splits: [
					{ name: 'split/01-a' },
					{ name: 'split/02-c' }
				]
			});
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var eq struct {
		Equivalent bool    `json:"equivalent"`
		SplitTree  string  `json:"splitTree"`
		SourceTree string  `json:"sourceTree"`
		Error      *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &eq); err != nil {
		t.Fatal(err)
	}

	if eq.Error != nil {
		t.Fatalf("verifyEquivalence error: %s", *eq.Error)
	}
	if !eq.Equivalent {
		t.Errorf("expected equivalent trees, splitTree=%s sourceTree=%s", eq.SplitTree, eq.SourceTree)
	}
}

func TestChunk06_VerifyEquivalence_Mismatch(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)
	dir := initGitRepo(t)

	writeFile(t, filepath.Join(dir, "a.go"), "package a")
	writeFile(t, filepath.Join(dir, "b.go"), "package b")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base")

	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "a.go"), "package a // v2")
	writeFile(t, filepath.Join(dir, "b.go"), "package b // v2")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature")

	// Execute split but only include a.go — b.go is missing from splits.
	_, err := evalJS(`
		(function() {
			var plan = {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: { 'a.go': 'M' },
				splits: [{ name: 'split/01-partial', files: ['a.go'], message: 'partial' }]
			};
			globalThis.prSplit.executeSplit(plan);
			return 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.verifyEquivalence({
				dir: '` + escapeJSPath(dir) + `',
				sourceBranch: 'feature',
				splits: [{ name: 'split/01-partial' }]
			});
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var eq struct {
		Equivalent bool `json:"equivalent"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &eq); err != nil {
		t.Fatal(err)
	}
	if eq.Equivalent {
		t.Error("expected non-equivalent when b.go is missing from splits")
	}
}

func TestChunk06_VerifyEquivalence_NullPlan(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.verifyEquivalence(null);
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.(string), "invalid plan") {
		t.Errorf("error = %q, want 'invalid plan'", result)
	}
}

func TestChunk06_VerifyEquivalenceDetailed_IncludesDiffFiles(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)
	dir := initGitRepo(t)

	writeFile(t, filepath.Join(dir, "a.go"), "package a")
	writeFile(t, filepath.Join(dir, "b.go"), "package b")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "base")

	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "a.go"), "package a // v2")
	writeFile(t, filepath.Join(dir, "b.go"), "package b // v2")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature")

	// Only include a.go.
	_, _ = evalJS(`
		(function() {
			globalThis.prSplit.executeSplit({
				baseBranch: 'main', sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: { 'a.go': 'M' },
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'a only' }]
			});
			return 'ok';
		})()
	`)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.verifyEquivalenceDetailed({
				dir: '` + escapeJSPath(dir) + `',
				sourceBranch: 'feature',
				splits: [{ name: 'split/01-a' }]
			});
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var detailed struct {
		Equivalent  bool     `json:"equivalent"`
		DiffFiles   []string `json:"diffFiles"`
		DiffSummary string   `json:"diffSummary"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &detailed); err != nil {
		t.Fatal(err)
	}
	if detailed.Equivalent {
		t.Error("expected non-equivalent")
	}
	if len(detailed.DiffFiles) == 0 {
		t.Error("expected diffFiles to include b.go")
	}
	found := false
	for _, f := range detailed.DiffFiles {
		if f == "b.go" {
			found = true
		}
	}
	if !found {
		t.Errorf("diffFiles = %v, want b.go present", detailed.DiffFiles)
	}
}

func TestChunk06_CleanupBranches(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)
	dir := setupExecRepoAndExecute(t, evalJS)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.cleanupBranches({
				baseBranch: 'main',
				dir: '` + escapeJSPath(dir) + `',
				splits: [
					{ name: 'split/01-a' },
					{ name: 'split/02-c' }
				]
			});
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var cleanup struct {
		Deleted []string `json:"deleted"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &cleanup); err != nil {
		t.Fatal(err)
	}

	if len(cleanup.Deleted) != 2 {
		t.Errorf("expected 2 deleted branches, got %d: %v", len(cleanup.Deleted), cleanup.Deleted)
	}
	if len(cleanup.Errors) != 0 {
		t.Errorf("unexpected errors: %v", cleanup.Errors)
	}

	// Verify branches are gone.
	for _, name := range []string{"split/01-a", "split/02-c"} {
		showRef := gitCmdAllowFail(t, dir, "rev-parse", "--verify", "refs/heads/"+name)
		if showRef.err == nil {
			t.Errorf("branch %s still exists after cleanup", name)
		}
	}
}

func TestChunk06_CleanupBranches_Idempotent(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)
	dir := setupExecRepoAndExecute(t, evalJS)

	// First cleanup.
	_, err := evalJS(`
		(function() {
			globalThis.prSplit.cleanupBranches({
				baseBranch: 'main',
				dir: '` + escapeJSPath(dir) + `',
				splits: [{ name: 'split/01-a' }, { name: 'split/02-c' }]
			});
			return 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Second cleanup — should not panic, branches just already gone.
	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.cleanupBranches({
				baseBranch: 'main',
				dir: '` + escapeJSPath(dir) + `',
				splits: [{ name: 'split/01-a' }, { name: 'split/02-c' }]
			});
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var cleanup struct {
		Deleted []string `json:"deleted"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &cleanup); err != nil {
		t.Fatal(err)
	}
	// Both should appear in errors (already deleted), zero in deleted.
	if len(cleanup.Deleted) != 0 {
		t.Errorf("expected 0 deleted on second run, got %d", len(cleanup.Deleted))
	}
}

func TestChunk06_CleanupBranches_NullPlan(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, verifyChunks...)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.cleanupBranches(null);
			return JSON.stringify(r);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var cleanup struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &cleanup); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.Errors) == 0 {
		t.Error("expected errors for null plan")
	}
}
