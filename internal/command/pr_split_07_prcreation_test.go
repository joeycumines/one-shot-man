package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Chunk 07: PR Creation — createPRs
//  Tests mock exec.execv since we cannot push to a real remote in unit tests.
// ---------------------------------------------------------------------------

var prCreationChunks = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning",
	"04_validation", "05_execution", "06_verification", "07_prcreation",
}

func TestChunk07_CreatePRs_NoSplits(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var r = await globalThis.prSplit.createPRs({ splits: [] });
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !strings.Contains(result.(string), "no splits") {
		t.Errorf("error = %v, want 'no splits'", result)
	}
}

func TestChunk07_CreatePRs_PushOnly(t *testing.T) {
	t.Parallel()
	// Mock gitExec to simulate successful push.
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	// Override _gitExec to track calls and return success.
	result, err := evalJS(`
		(async function() {
			var calls = [];
			var origGitExec = globalThis.prSplit._gitExec;
			globalThis.prSplit._gitExec = async function(dir, args) {
				calls.push(args.join(' '));
				if (args[0] === 'push') {
					return { code: 0, stdout: '', stderr: '' };
				}
				return await origGitExec(dir, args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [
					{ name: 'split/01-a', files: ['a.go'], message: 'part 1' },
					{ name: 'split/02-b', files: ['b.go'], message: 'part 2' }
				]
			};
			var r = await globalThis.prSplit.createPRs(plan, { pushOnly: true });

			// Restore.
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({
				error: r.error,
				resultCount: r.results.length,
				allPushed: r.results.every(function(x) { return x.pushed; }),
				pushCalls: calls.filter(function(c) { return c.indexOf('push') === 0; }).length
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error       *string `json:"error"`
		ResultCount int     `json:"resultCount"`
		AllPushed   bool    `json:"allPushed"`
		PushCalls   int     `json:"pushCalls"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Error != nil {
		t.Fatalf("unexpected error: %s", *data.Error)
	}
	if data.ResultCount != 2 {
		t.Errorf("resultCount = %d, want 2", data.ResultCount)
	}
	if !data.AllPushed {
		t.Error("expected all branches pushed")
	}
	if data.PushCalls != 2 {
		t.Errorf("pushCalls = %d, want 2", data.PushCalls)
	}
}

func TestChunk07_CreatePRs_PushFailure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var origGitExec = globalThis.prSplit._gitExec;
			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'push') {
					return { code: 1, stdout: '', stderr: 'remote not found' };
				}
				return await origGitExec(dir, args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'part 1' }]
			};
			var r = await globalThis.prSplit.createPRs(plan, { pushOnly: true });
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({ error: r.error, pushed: r.results[0].pushed });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error  string `json:"error"`
		Pushed bool   `json:"pushed"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error == "" {
		t.Error("expected error for push failure")
	}
	if data.Pushed {
		t.Error("expected pushed=false")
	}
}

func TestChunk07_CreatePRs_StackingOrder(t *testing.T) {
	t.Parallel()
	// Mock both gitExec (push) and exec.execv (gh pr create) to verify
	// stacking: PR 2 bases on branch 1.
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var ghCreateCalls = [];
			var origGitExec = globalThis.prSplit._gitExec;
			var origExecv = globalThis.prSplit._modules.exec.execv;

			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'push') return { code: 0, stdout: '', stderr: '' };
				if (args[0] === 'ls-remote') return { code: 0, stdout: 'abc123\trefs/heads/main', stderr: '' };
				if (args[0] === 'diff' && args.indexOf('--quiet') >= 0) return { code: 1, stdout: '', stderr: '' };
				return await origGitExec(dir, args);
			};
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'gh' && args[1] === 'pr' && args[2] === 'create') {
					ghCreateCalls.push(args.slice());
					return { code: 0, stdout: 'https://github.com/test/pr/' + ghCreateCalls.length, stderr: '' };
				}
				// gh --version check
				if (args[0] === 'gh') {
					return { code: 0, stdout: 'gh version 2.0.0', stderr: '' };
				}
				return origExecv(args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [
					{ name: 'split/01-a', files: ['a.go'], message: 'first' },
					{ name: 'split/02-b', files: ['b.go'], message: 'second' }
				]
			};
			var r = await globalThis.prSplit.createPRs(plan, { draft: true });

			globalThis.prSplit._gitExec = origGitExec;
			globalThis.prSplit._modules.exec.execv = origExecv;

			// Check stacking: first PR base=main, second base=split/01-a.
			var firstBase = '';
			var secondBase = '';
			var hasDraft = false;
			for (var c = 0; c < ghCreateCalls.length; c++) {
				var args = ghCreateCalls[c];
				for (var j = 0; j < args.length; j++) {
					if (args[j] === '--base' && j + 1 < args.length) {
						if (c === 0) firstBase = args[j + 1];
						else secondBase = args[j + 1];
					}
					if (args[j] === '--draft') hasDraft = true;
				}
			}

			return JSON.stringify({
				error: r.error,
				ghCreateCount: ghCreateCalls.length,
				firstBase: firstBase,
				secondBase: secondBase,
				pr1Url: r.results[0] ? r.results[0].prUrl : '',
				pr2Url: r.results[1] ? r.results[1].prUrl : '',
				hasDraft: hasDraft
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error         *string `json:"error"`
		GhCreateCount int     `json:"ghCreateCount"`
		FirstBase     string  `json:"firstBase"`
		SecondBase    string  `json:"secondBase"`
		PR1URL        string  `json:"pr1Url"`
		PR2URL        string  `json:"pr2Url"`
		HasDraft      bool    `json:"hasDraft"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}

	if data.Error != nil {
		t.Fatalf("unexpected error: %s", *data.Error)
	}
	if data.GhCreateCount != 2 {
		t.Errorf("gh pr create call count = %d, want 2", data.GhCreateCount)
	}
	if data.FirstBase != "main" {
		t.Errorf("first PR base = %q, want 'main'", data.FirstBase)
	}
	if data.SecondBase != "split/01-a" {
		t.Errorf("second PR base = %q, want 'split/01-a'", data.SecondBase)
	}
	if data.PR1URL == "" || data.PR2URL == "" {
		t.Error("expected PR URLs")
	}
	if !data.HasDraft {
		t.Error("expected --draft flag")
	}
}

// ---- T11: gh CLI not found ------------------------------------------------

func TestChunk07_CreatePRs_GhNotFound(t *testing.T) {
	t.Parallel()
	// Mock exec to make 'gh' command unavailable.
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origGitExec = globalThis.prSplit._gitExec;

			// gh --version fails (not on PATH).
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'gh') {
					return { code: 127, stdout: '', stderr: 'gh: command not found' };
				}
				return origExecv(args);
			};
			// Ensure remote check doesn't block us.
			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'push') return { code: 0, stdout: '', stderr: '' };
				return await origGitExec(dir, args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'part 1' }]
			};
			// NOT pushOnly → should check for gh CLI.
			var r = await globalThis.prSplit.createPRs(plan, { pushOnly: false });

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({
				error: r.error || '',
				resultCount: r.results.length
			});
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
	if data.Error == "" {
		t.Fatal("expected error when gh not found")
	}
	if !strings.Contains(data.Error, "gh") {
		t.Errorf("error should mention 'gh', got %q", data.Error)
	}
	if !strings.Contains(data.Error, "cli.github.com") {
		t.Errorf("error should suggest installation URL, got %q", data.Error)
	}
	if data.ResultCount != 0 {
		t.Errorf("expected 0 results when gh not found, got %d", data.ResultCount)
	}
}

func TestChunk07_CreatePRs_RemoteBranchNotFound(t *testing.T) {
	t.Parallel()
	// gh CLI available but base branch missing from remote.
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origGitExec = globalThis.prSplit._gitExec;

			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'gh' && args[1] === '--version') {
					return { code: 0, stdout: 'gh version 2.0.0', stderr: '' };
				}
				return origExecv(args);
			};
			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'ls-remote') {
					return { code: 0, stdout: '', stderr: '' }; // empty = not found
				}
				return await origGitExec(dir, args);
			};

			var plan = {
				baseBranch: 'nonexistent-branch',
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'part 1' }]
			};
			var r = await globalThis.prSplit.createPRs(plan, {});

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({
				error: r.error || '',
				resultCount: r.results.length
			});
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
	if data.Error == "" {
		t.Fatal("expected error when remote branch not found")
	}
	if !strings.Contains(data.Error, "nonexistent-branch") {
		t.Errorf("error should mention branch name, got %q", data.Error)
	}
	if data.ResultCount != 0 {
		t.Errorf("expected 0 results, got %d", data.ResultCount)
	}
}

// TestChunk07_CreatePRs_LsRemoteFailureIsNotMissingBranch pins the distinction:
// an ls-remote transport/exec failure (error true, or a code other than 0/2)
// must be reported as an ls-remote failure with the exec reason, not as "base
// branch not found on remote".
func TestChunk07_CreatePRs_LsRemoteFailureIsNotMissingBranch(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origGitExec = globalThis.prSplit._gitExec;

			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'gh') return { code: 0, stdout: 'gh version 2.0.0', stderr: '' };
				return origExecv(args);
			};
			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'ls-remote') {
					return { code: -1, stdout: '', stderr: '', error: true, message: 'command output capture was cut short before EOF' };
				}
				return await origGitExec(dir, args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'part 1' }]
			};
			var r = await globalThis.prSplit.createPRs(plan, {});

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({ error: r.error || '' });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data.Error, "ls-remote failed") {
		t.Errorf("error = %q, want an ls-remote failure message", data.Error)
	}
	if strings.Contains(data.Error, "not found on remote") {
		t.Errorf("error = %q, must not misreport a transport failure as a missing branch", data.Error)
	}
	if !strings.Contains(data.Error, "cut short") {
		t.Errorf("error = %q, want the exec-layer reason carried through", data.Error)
	}
}

// TestChunk07_CreatePRs_DiffQuietErrorSkipsPRCreation pins that git diff
// --quiet exiting with an error (128/-1, not 0=no-diff or 1=diff) records a
// per-branch error and does NOT call gh pr create.
func TestChunk07_CreatePRs_DiffQuietErrorSkipsPRCreation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var ghCreateCalls = 0;
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origGitExec = globalThis.prSplit._gitExec;

			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'push') return { code: 0, stdout: '', stderr: '' };
				if (args[0] === 'ls-remote') return { code: 0, stdout: 'abc123\trefs/heads/main', stderr: '' };
				if (args[0] === 'diff' && args.indexOf('--quiet') >= 0) {
					return { code: 128, stdout: '', stderr: 'fatal: bad revision', error: true, message: 'fatal: bad revision' };
				}
				return await origGitExec(dir, args);
			};
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'gh' && args[1] === 'pr' && args[2] === 'create') { ghCreateCalls++; return { code: 0, stdout: 'url', stderr: '' }; }
				if (args[0] === 'gh') return { code: 0, stdout: 'gh version 2.0.0', stderr: '' };
				return origExecv(args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'part 1' }]
			};
			var r = await globalThis.prSplit.createPRs(plan, {});

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({
				ghCreateCalls: ghCreateCalls,
				err: r.results[0] ? (r.results[0].error || '') : 'no result',
				skipped: r.results[0] ? !!r.results[0].skipped : false
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		GhCreateCalls int    `json:"ghCreateCalls"`
		Err           string `json:"err"`
		Skipped       bool   `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.GhCreateCalls != 0 {
		t.Errorf("gh pr create called %d times, want 0 on a diff --quiet error", data.GhCreateCalls)
	}
	if data.Skipped {
		t.Error("a diff --quiet error must not be marked skipped (no-diff)")
	}
	if !strings.Contains(data.Err, "git diff --quiet failed") || !strings.Contains(data.Err, "bad revision") {
		t.Errorf("error = %q, want the diff --quiet failure with the exec reason", data.Err)
	}
}

// TestChunk07_CreatePRs_AutoMergeSkipsSkippedPRs pins that the auto-merge loop
// does not run gh pr merge for a PR whose creation was skipped (no diff).
func TestChunk07_CreatePRs_AutoMergeSkipsSkippedPRs(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, prCreationChunks...)

	result, err := evalJS(`
		(async function() {
			var mergeCalls = 0;
			var createCalls = 0;
			var origExecv = globalThis.prSplit._modules.exec.execv;
			var origGitExec = globalThis.prSplit._gitExec;

			globalThis.prSplit._gitExec = async function(dir, args) {
				if (args[0] === 'push') return { code: 0, stdout: '', stderr: '' };
				if (args[0] === 'ls-remote') return { code: 0, stdout: 'abc123\trefs/heads/main', stderr: '' };
				if (args[0] === 'diff' && args.indexOf('--quiet') >= 0) return { code: 0, stdout: '', stderr: '' }; // no diff: skip
				return await origGitExec(dir, args);
			};
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'gh' && args[1] === 'pr' && args[2] === 'create') { createCalls++; return { code: 0, stdout: 'url', stderr: '' }; }
				if (args[0] === 'gh' && args[1] === 'pr' && args[2] === 'merge') { mergeCalls++; return { code: 0, stdout: '', stderr: '' }; }
				if (args[0] === 'gh') return { code: 0, stdout: 'gh version 2.0.0', stderr: '' };
				return origExecv(args);
			};

			var plan = {
				baseBranch: 'main',
				splits: [{ name: 'split/01-a', files: ['a.go'], message: 'part 1' }]
			};
			var r = await globalThis.prSplit.createPRs(plan, { autoMerge: true, mergeMethod: 'squash' });

			globalThis.prSplit._modules.exec.execv = origExecv;
			globalThis.prSplit._gitExec = origGitExec;

			return JSON.stringify({ createCalls: createCalls, mergeCalls: mergeCalls, skipped: r.results[0] ? !!r.results[0].skipped : false });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		CreateCalls int  `json:"createCalls"`
		MergeCalls  int  `json:"mergeCalls"`
		Skipped     bool `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.Skipped {
		t.Error("expected the no-diff split to be marked skipped")
	}
	if data.CreateCalls != 0 {
		t.Errorf("gh pr create called %d times, want 0 for a no-diff split", data.CreateCalls)
	}
	if data.MergeCalls != 0 {
		t.Errorf("gh pr merge called %d times, want 0 for a skipped PR", data.MergeCalls)
	}
}
