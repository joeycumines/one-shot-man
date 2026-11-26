package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestVerifySplitAsync_WaitRejectionIsReported pins the diagnostic for a
// verification child whose wait() REJECTS for a non-cancellation reason.
//
// The guard around child.wait() exists because an unhandled rejection is fatal
// to the event loop. Catching it is only half the job: the rejection used to
// collapse into `exitCode = -1`, so the operator saw
// `verify failed (exit -1):` with an empty explanation, which is exactly what a
// verify command that failed without writing to stderr also looks like. The
// rejection reason is now carried into the result.
func TestVerifySplitAsync_WaitRejectionIsReported(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}

	// A verify child whose streams end cleanly but whose wait() rejects, which
	// is the shape a cancelled or unrecorded exit produces.
	if _, err := evalJS(`
		globalThis.__origSpawn06c = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function() {
			function doneStream() {
				return { read: function() { return Promise.resolve({done: true}); } };
			}
			return {
				stdout: doneStream(),
				stderr: doneStream(),
				wait: function() { return Promise.reject(new Error('wait: context deadline exceeded')); },
				kill: function() {},
				isAlive: function() { return false; },
				close: function() {}
			};
		};
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(`globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawn06c; globalThis.__origSpawn06c = undefined;`); err != nil {
			t.Logf("restore spawn mock: %v", err)
		}
	})

	raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitAsync('split/01', {
		dir: '/tmp/repo',
		verifyCommand: 'make test'
	}))`)
	if err != nil {
		t.Fatalf("evalJS failed: %v", err)
	}

	var result struct {
		Passed    bool    `json:"passed"`
		ExitCode  float64 `json:"exitCode"`
		Cancelled bool    `json:"cancelled"`
		Truncated bool    `json:"truncated"`
		Error     *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("unmarshal %v: %v", raw, err)
	}

	if result.Passed {
		t.Fatal("a rejected wait must not pass verification")
	}
	if result.Cancelled {
		t.Error("a non-cancellation wait rejection must not be reported as cancelled")
	}
	if result.Truncated {
		t.Error("clean streams must not be reported as truncated")
	}
	if result.Error == nil {
		t.Fatal("a rejected wait must explain itself")
	}
	if !strings.Contains(*result.Error, "wait failed") {
		t.Errorf("error = %q, want it to name the wait failure", *result.Error)
	}
	if !strings.Contains(*result.Error, "context deadline exceeded") {
		t.Errorf("error = %q, want it to carry the rejection reason", *result.Error)
	}
	if strings.Contains(*result.Error, "verify failed (exit -1)") {
		t.Errorf("error = %q, must not degrade into the bare exit -1 message", *result.Error)
	}
}

// TestVerifySplitAsync_RejectingStreamReadDoesNotReject pins the other half of
// the event-loop-fatal contract: a stream read can reject when the engine
// context ends, and verifySplitAsync must not propagate that rejection.
//
// Callers await it without a catch (the TUI fallback path and the automated
// pipeline both do), so a rejection there is fatal to the loop. A stream that
// cannot be drained is a prefix capture, which is reported as truncated.
func TestVerifySplitAsync_RejectingStreamReadDoesNotReject(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}
	if _, err := evalJS(`
		globalThis.__origSpawn06e = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function() {
			function rejectingStream() {
				return { read: function() { return Promise.reject(new Error('read: context canceled')); } };
			}
			return {
				stdout: rejectingStream(),
				stderr: rejectingStream(),
				wait: function() { return Promise.resolve({code: 0}); },
				kill: function() {},
				isAlive: function() { return false; },
				close: function() {}
			};
		};
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(`globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawn06e; globalThis.__origSpawn06e = undefined;`); err != nil {
			t.Logf("restore spawn mock: %v", err)
		}
	})

	// If the rejection leaked, evalJS would report it instead of a result.
	raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitAsync('split/01', {
		dir: '/tmp/repo',
		verifyCommand: 'make test'
	}))`)
	if err != nil {
		t.Fatalf("verifySplitAsync rejected instead of returning a result: %v", err)
	}

	var result struct {
		Passed    bool    `json:"passed"`
		Truncated bool    `json:"truncated"`
		Error     *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("unmarshal %v: %v", raw, err)
	}
	if result.Passed {
		t.Error("an undrainable capture must not pass verification: the verdict is unknown")
	}
	if !result.Truncated {
		t.Error("an undrainable capture must be reported as truncated")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "cut short") {
		t.Errorf("error = %v, want the cut-short capture message", result.Error)
	}
}

// TestVerifySplitAsync_CancelledWaitIsNotAWaitFailure keeps the cancellation
// branch ahead of the new diagnostic: a cancelled verification is cancelled,
// not a broken wait.
func TestVerifySplitAsync_CancelledWaitIsNotAWaitFailure(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
		t.Fatalf("failed to install git mock: %v", err)
	}
	if _, err := evalJS(`
		globalThis.__origSpawn06d = globalThis.prSplit._modules.exec.spawn;
		globalThis.prSplit._modules.exec.spawn = function() {
			function doneStream() {
				return { read: function() { return Promise.resolve({done: true}); } };
			}
			return {
				stdout: doneStream(),
				stderr: doneStream(),
				wait: function() { return Promise.reject(new Error('wait: context canceled')); },
				kill: function() {},
				isAlive: function() { return false; },
				close: function() {}
			};
		};
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := evalJS(`globalThis.prSplit._modules.exec.spawn = globalThis.__origSpawn06d; globalThis.__origSpawn06d = undefined;`); err != nil {
			t.Logf("restore spawn mock: %v", err)
		}
	})

	raw, err := evalJS(`await JSON.stringify(await globalThis.prSplit.verifySplitAsync('split/01', {
		dir: '/tmp/repo',
		verifyCommand: 'make test',
		isCancelled: function() { return true; }
	}))`)
	if err != nil {
		t.Fatalf("evalJS failed: %v", err)
	}

	var result struct {
		Passed    bool    `json:"passed"`
		Cancelled bool    `json:"cancelled"`
		Error     *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("unmarshal %v: %v", raw, err)
	}
	if result.Passed {
		t.Error("a cancelled verification must not pass")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "cancelled by user") {
		t.Errorf("error = %v, want a cancellation message", result.Error)
	}
}
