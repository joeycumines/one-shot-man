package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestVerifySplitAsync_GitExecRejectionDoesNotReject pins review-04 §2: the
// function documents that it must not reject (an unhandled rejection is fatal
// to the event loop), but the git worktree setup awaits before the try block.
// A rejecting gitExecAsync must become a structured failure result.
func TestVerifySplitAsync_GitExecRejectionDoesNotReject(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(`
		globalThis.__origGitAsync06 = globalThis.prSplit._gitExecAsync;
		globalThis.prSplit._gitExecAsync = function() { return Promise.reject(new Error('spawn failed: engine context ended')); };
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(`globalThis.prSplit._gitExecAsync = globalThis.__origGitAsync06; globalThis.__origGitAsync06 = undefined;`); err != nil {
			t.Logf("restore gitExecAsync mock: %v", err)
		}
	})

	raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitAsync('split/01', {
		dir: '/tmp/repo', verifyCommand: 'make test'
	}))`)
	if err != nil {
		t.Fatalf("verifySplitAsync rejected instead of returning a result: %v", err)
	}
	var result struct {
		Passed     bool    `json:"passed"`
		ExecFailed bool    `json:"execFailed"`
		Error      *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("unmarshal %v: %v", raw, err)
	}
	if result.Passed {
		t.Error("a rejected exec layer must not pass verification")
	}
	if !result.ExecFailed {
		t.Error("an uncaught exec-layer failure must be tagged execFailed so it cannot be forgiven as pre-existing")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "spawn failed") {
		t.Errorf("error = %v, want the original exec failure reason", result.Error)
	}
}

// TestVerifySplitsAsync_WaitFailureIsNotPreExisting pins review-04 §1.4: a
// wait-failure result (no exit status) is an unknown verdict, exactly like a
// truncated capture, so it must never be marked preExisting and must force
// allPassed=false even when the baseline also had a broken wait. Both the
// baseline and the split use a child whose wait() rejects.
func TestVerifySplitsAsync_WaitFailureIsNotPreExisting(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("git mock: %v", err)
	}
	if _, err := evalJS(`
		globalThis.__origSpawn06wait = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function() {
			function doneStream() {
				return { read: function() { return Promise.resolve({done: true}); } };
			}
			return {
				stdout: doneStream(), stderr: doneStream(),
				wait: function() { return Promise.reject(new Error('wait: no exit status')); },
				kill: function() {}, isAlive: function() { return false; }, close: function() {}
			};
		};
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(`globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawn06wait; globalThis.__origSpawn06wait = undefined;`); err != nil {
			t.Logf("restore spawn mock: %v", err)
		}
	})

	raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitsAsync({
		sourceBranch: 'feature',
		splits: [{ name: 'split/01' }]
	}, { verifyCommand: 'make test' }))`)
	if err != nil {
		t.Fatalf("evalJS: %v", err)
	}
	var result struct {
		AllPassed bool `json:"allPassed"`
		Results   []struct {
			PreExisting bool `json:"preExisting"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("unmarshal %v: %v", raw, err)
	}
	if result.AllPassed {
		t.Error("a wait failure with no exit status is an unknown verdict and must not be forgiven as allPassed")
	}
	if len(result.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(result.Results))
	}
	if result.Results[0].PreExisting {
		t.Error("a wait failure must never be marked preExisting")
	}
}
