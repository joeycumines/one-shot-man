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

// TestVerifySplitsAsync_BaselineExecFailureIsNotBaselineFailure distinguishes
// the actual split row from an early-stop baseline row. A baseline execution
// failure with successful cleanup must still run the split, and must not
// forgive its deterministic exit 1. An unconfirmed child instead retains its
// baseline worktree and stops before spawning a split.
func TestVerifySplitsAsync_BaselineExecFailureIsNotBaselineFailure(t *testing.T) {
	t.Parallel()
	for _, scenario := range baselineExecFailureScenarios {
		t.Run(scenario, func(t *testing.T) {
			_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
			if _, err := evalJS(prsplittest.GitMockSetupJS()); err != nil {
				t.Fatalf("git mock: %v", err)
			}
			scenarioJSON, err := json.Marshal(scenario)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := evalJS(`(async function() {
				var scenario = ` + string(scenarioJSON) + `;
				` + baselineExecFailureMockJS + `
				var fixture = installBaselineExecFailureMock('feature', scenario);
				try {
					var batch = await prSplit.verifySplitsAsync({
						dir:'/tmp/repo', sourceBranch:'feature',
						splits:[{name:'split/01'}], verifyCommand:'make test'
					});
					return JSON.stringify({batch:batch, spawned:fixture.spawned,
						waits:fixture.baselineWaits, kills:fixture.baselineKills,
						removed:fixture.removed, baselineDir:fixture.worktrees.feature});
				} finally {
					fixture.restore();
				}
			})()`)
			if err != nil {
				t.Fatalf("evalJS: %v", err)
			}
			var got struct {
				Batch struct {
					AllPassed bool   `json:"allPassed"`
					Error     string `json:"error"`
					Results   []struct {
						Name             string `json:"name"`
						Passed           bool   `json:"passed"`
						PreExisting      bool   `json:"preExisting"`
						ExitCode         int    `json:"exitCode"`
						ExecFailed       bool   `json:"execFailed"`
						CleanupFailed    bool   `json:"cleanupFailed"`
						WorktreeRetained bool   `json:"worktreeRetained"`
						WorktreeDir      string `json:"worktreeDir"`
						TreeCleanupError string `json:"treeCleanupError"`
						Baseline         bool   `json:"baseline"`
						Error            string `json:"error"`
					} `json:"results"`
				} `json:"batch"`
				Spawned     []string `json:"spawned"`
				Waits       int      `json:"waits"`
				Kills       int      `json:"kills"`
				Removed     []string `json:"removed"`
				BaselineDir string   `json:"baselineDir"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &got); err != nil {
				t.Fatalf("unmarshal %v: %v", raw, err)
			}
			if got.Batch.AllPassed || len(got.Batch.Results) != 1 {
				t.Fatalf("expected one failed result: %s", raw)
			}
			r := got.Batch.Results[0]
			if r.Passed || r.PreExisting || r.TreeCleanupError != "" {
				t.Fatalf("failure was passed, forgiven, or mislabeled as a tree cleanup error: %s", raw)
			}
			wantWaits, wantKills := 2, 1
			if scenario == "spawn_rejection" {
				wantWaits, wantKills = 0, 0
			}
			if got.Waits != wantWaits || got.Kills != wantKills {
				t.Fatalf("baseline cleanup path was not exercised: %s", raw)
			}
			if got.BaselineDir == "" {
				t.Fatalf("baseline never created a worktree: %s", raw)
			}
			if scenario == "persistent_wait_rejection" {
				if len(got.Spawned) != 1 || got.Spawned[0] != "feature" || len(got.Removed) != 0 {
					t.Fatalf("unconfirmed baseline termination must stop before removal or split spawn: %s", raw)
				}
				if r.Name != "feature" || !r.Baseline || r.ExitCode != -1 || !r.ExecFailed || !r.CleanupFailed || !r.WorktreeRetained || r.WorktreeDir != got.BaselineDir {
					t.Fatalf("retained baseline result lost its identity or cleanup metadata: %s", raw)
				}
				if !strings.Contains(got.Batch.Error, got.BaselineDir) || !strings.Contains(r.Error, "termination was not confirmed") {
					t.Fatalf("retained baseline diagnostic is not actionable: %s", raw)
				}
			} else {
				if len(got.Spawned) != 2 || got.Spawned[0] != "feature" || got.Spawned[1] != "split/01" || len(got.Removed) != 2 {
					t.Fatalf("ordinary unknown baseline must run and clean the actual split: %s", raw)
				}
				if r.Name != "split/01" || r.Baseline || r.ExitCode != 1 || r.ExecFailed || r.CleanupFailed || r.WorktreeRetained {
					t.Fatalf("expected the deterministic split exit 1 with successful cleanup: %s", raw)
				}
				if !strings.Contains(r.Error, "baseline verdict unknown") {
					t.Fatalf("unknown baseline must disable pre-existing detection: %s", raw)
				}
			}
		})
	}
}
