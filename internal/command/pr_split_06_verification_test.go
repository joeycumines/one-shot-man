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
			var r = await prSplit.executeSplit(plan);
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
		(async function() {
			var r = await globalThis.prSplit.verifyEquivalence({
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
			await globalThis.prSplit.executeSplit(plan);
			return 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.verifyEquivalence({
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
		(async function() {
			var r = await globalThis.prSplit.verifyEquivalence(null);
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
			await globalThis.prSplit.executeSplit({
				baseBranch: 'main', sourceBranch: 'feature',
				dir: '` + escapeJSPath(dir) + `',
				fileStatuses: { 'a.go': 'M' },
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'a only' }]
			});
			return 'ok';
		})()
	`)

	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.verifyEquivalenceDetailed({
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
		(async function() {
			var r = await globalThis.prSplit.cleanupBranches({
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
			await globalThis.prSplit.cleanupBranches({
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
		(async function() {
			var r = await globalThis.prSplit.cleanupBranches({
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
		(async function() {
			var r = await globalThis.prSplit.cleanupBranches(null);
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

// ---------------------------------------------------------------------------
//  Chunk 06: truncation hardening — verifySplitsAsync must not forgive or
//  mislabel cut-short captures.
// ---------------------------------------------------------------------------

// installTruncatedSpawnMock replaces the git-mock spawn with one whose
// streams end with {done: true, truncated: true} when the mocked execv result
// carries error === true, and cleanly otherwise. Returns JS restoring it.
//
// The mock dispatches through the same execv override so '!sh' responses keep
// working; only the stream end marker changes.
const truncatedSpawnMockJS = `
	globalThis.__origSpawn06 = globalThis.prSplit._modules.exec.spawn;
	globalThis.prSplit._modules.exec.spawn = function(cmd, args) {
		var execMod = globalThis.prSplit._modules.exec;
		var fullArgv = [cmd].concat(args || []);
		var r = execMod.execv(fullArgv);
		var truncated = (r && r.error === true);
		var stdoutRead = false, stderrRead = false;
		return {
			stdout: {
				read: function() {
					if (!stdoutRead) {
						stdoutRead = true;
						if (r.stdout) return Promise.resolve({done: false, value: r.stdout});
					}
					return Promise.resolve(truncated ? {done: true, truncated: true} : {done: true});
				}
			},
			stderr: {
				read: function() {
					if (!stderrRead) {
						stderrRead = true;
						if (r.stderr) return Promise.resolve({done: false, value: r.stderr});
					}
					return Promise.resolve(truncated ? {done: true, truncated: true} : {done: true});
				}
			},
			wait: function() { return Promise.resolve({code: r.code}); },
			kill: function() {},
			isAlive: function() { return false; },
			close: function() {}
		};
	};
`

const restoreSpawn06JS = `
	globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawn06;
	globalThis.__origSpawn06 = undefined;
`

type verifySplitsTruncResult struct {
	AllPassed bool `json:"allPassed"`
	Results   []struct {
		Name        string  `json:"name"`
		Passed      bool    `json:"passed"`
		Skipped     bool    `json:"skipped"`
		PreExisting bool    `json:"preExisting"`
		Truncated   bool    `json:"truncated"`
		Error       *string `json:"error"`
	} `json:"results"`
	Error *string `json:"error"`
}

func parseVerifySplitsTruncResult(t *testing.T, raw any) verifySplitsTruncResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string from evalJS, got %T: %v", raw, raw)
	}
	var r verifySplitsTruncResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("failed to parse verifySplits result: %v\nraw: %s", err, s)
	}
	return r
}

func TestVerifySplitsAsync_TruncatedCaptures(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}
	if _, err := evalJS(truncatedSpawnMockJS); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(restoreSpawn06JS); err != nil {
			t.Logf("restore spawn mock: %v", err)
		}
	})

	runVerify := func(t *testing.T) verifySplitsTruncResult {
		t.Helper()
		raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitsAsync({
			dir: '/tmp/repo',
			sourceBranch: 'feature',
			verifyCommand: 'make test',
			splits: [
				{name: 'split/01', files: ['a.go']}
			]
		}))`)
		if err != nil {
			t.Fatalf("evalJS failed: %v", err)
		}
		return parseVerifySplitsTruncResult(t, raw)
	}

	t.Run("truncated_baseline_disables_preexisting_and_failing_branch_is_not_forgiven", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Both baseline and branch fail with error:true → both streams truncated.
		if _, err := evalJS(`_gitResponses['!sh'] = _gitFail('FAIL: legacy');`); err != nil {
			t.Fatal(err)
		}
		r := runVerify(t)
		if r.AllPassed {
			t.Error("allPassed must be false: a truncated branch is a failure")
		}
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if !r.Results[0].Truncated {
			t.Error("branch result should carry truncated=true")
		}
		if r.Results[0].PreExisting {
			t.Error("truncated branch must NEVER be marked preExisting, even with a failing baseline")
		}
		if r.Results[0].Error == nil || !strings.Contains(*r.Results[0].Error, "baseline verdict unknown") {
			t.Errorf("error should annotate the unknown baseline, got %v", r.Results[0].Error)
		}
	})

	t.Run("genuine_baseline_failure_truncated_branch_not_preexisting", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Baseline genuinely fails (error:false), branch capture truncated (error:true).
		if _, err := evalJS(`
			globalThis._shCallCount = 0;
			_gitResponses['!sh'] = function(argv) {
				globalThis._shCallCount++;
				if (globalThis._shCallCount === 1) return {code: 1, stdout: '', stderr: 'FAIL: pre-existing breakage', error: false};
				return {code: 1, stdout: 'partial output', stderr: 'cut', error: true};
			};
		`); err != nil {
			t.Fatal(err)
		}
		r := runVerify(t)
		if r.AllPassed {
			t.Error("allPassed must be false for a truncated branch")
		}
		if len(r.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(r.Results))
		}
		if !r.Results[0].Truncated {
			t.Error("branch result should carry truncated=true")
		}
		if r.Results[0].PreExisting {
			t.Error("truncated branch must NOT be preExisting even when the baseline genuinely failed")
		}
	})

	t.Run("genuine_failures_still_preexisting_without_truncation", func(t *testing.T) {
		if _, err := evalJS(resetGitMockJS); err != nil {
			t.Fatal(err)
		}
		// Both fail genuinely (error:false → clean streams) → pre-existing detection still works.
		if _, err := evalJS(`_gitResponses['!sh'] = {code: 1, stdout: '', stderr: 'FAIL: legacy', error: false, message: ''};`); err != nil {
			t.Fatal(err)
		}
		r := runVerify(t)
		if !r.AllPassed {
			t.Error("allPassed should be true when the only failure is pre-existing")
		}
		if len(r.Results) != 1 || !r.Results[0].PreExisting {
			t.Errorf("genuine baseline failure + genuine branch failure should be preExisting, got %+v", r.Results)
		}
		if r.Results[0].Truncated {
			t.Error("result should not be truncated")
		}
	})
}
