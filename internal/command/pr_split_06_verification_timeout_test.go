package command

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

const verifyTimeoutChildEnv = "OSM_TEST_VERIFY_TIMEOUT_CHILD"

// The verification command re-executes this test binary so the native timeout
// test does not depend on a platform-specific sleep command.
func TestVerifySplitAsyncTimeoutProcess(t *testing.T) {
	if os.Getenv(verifyTimeoutChildEnv) != "1" {
		t.Skip("subprocess helper")
	}
	skipSlow(t)
	time.Sleep(5 * time.Second)
}

func TestVerifySplitAsync_NativeTimeoutSetsTimedOut(t *testing.T) {
	skipSlow(t)
	// Worktrees and the helper's temporary files belong to this test. Git must
	// not inherit host configuration, templates, hooks, or signing settings.
	tempDir := t.TempDir()
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, tempDir)
	}
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_CONFIG_PARAMETERS", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_TEMPLATE_DIR", t.TempDir())
	t.Setenv(verifyTimeoutChildEnv, "1")
	dir := initGitRepo(t)
	gitCmd(t, dir, "commit", "--allow-empty", "-m", "verification fixture")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	quotedDir, _ := json.Marshal(dir)
	quotedExecutable, _ := json.Marshal(executable)
	evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
	raw, err := evalJS(fmt.Sprintf(`(async function() {
		var command = prSplit._shellQuote(%s) + ' -test.run=TestVerifySplitAsyncTimeoutProcess';
		return JSON.stringify(await prSplit.verifySplitAsync('main', {
			dir: %s, verifyCommand: command, verifyTimeoutMs: 1000
		}));
	})()`, quotedExecutable, quotedDir))
	if err != nil {
		t.Fatal(err)
	}
	var result asyncVerifyResult
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Passed || result.ExitCode == 0 || !result.TimedOut || result.ExecFailed {
		t.Fatalf("native timeout must carry a recorded failure and timedOut:true: %s", raw)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "verify timeout") {
		t.Fatalf("missing timeout diagnostic: %s", raw)
	}
	if result.CleanupFailed || result.WorktreeRetained {
		t.Fatalf("native timeout cleanup failed: %s", raw)
	}
	if got := gitCmd(t, dir, "worktree", "list", "--porcelain"); strings.Count(got, "worktree ") != 1 {
		t.Fatalf("verification worktree was not removed: %s", got)
	}
}

func TestVerifySplitAsync_TimeoutCaptureClassification(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, tc := range []struct {
		name        string
		code        int
		truncated   bool
		waitRejects bool
		wantTimeout bool
	}{
		{name: "success", code: 0},
		{name: "command failure", code: 7},
		{name: "incomplete capture", code: 7, truncated: true},
		{name: "timeout", code: 124, wantTimeout: true},
		{name: "timeout with incomplete capture", code: 124, truncated: true, wantTimeout: true},
		{name: "rejected wait after deadline", waitRejects: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			waitBody := fmt.Sprintf("return Promise.resolve({code: %d, signal: null});", tc.code)
			config := "verifyTimeoutMs: 600000"
			if tc.waitRejects {
				waitBody = `return new Promise(function(resolve, reject) {
					setTimeout(function() { reject(new Error('wait unavailable')); }, 10);
				});`
				config = "verifyTimeoutMs: 1"
			}
			result, pending, removes := runAsyncVerifyWithFactory(t, fmt.Sprintf(`function() {
				return {
					stdout: { read: function() { return Promise.resolve({done: true, truncated: %v}); } },
					stderr: { read: function() { return Promise.resolve({done: true}); } },
					wait: function() { %s },
					kill: function() { return Promise.resolve(); }
				};
			}`, tc.truncated, waitBody), config)
			if result.TimedOut != tc.wantTimeout || result.Truncated != tc.truncated || result.ExecFailed != tc.waitRejects {
				t.Fatalf("classification = %+v, want timedOut=%v truncated=%v execFailed=%v", result, tc.wantTimeout, tc.truncated, tc.waitRejects)
			}
			if wantPass := tc.code == 0 && !tc.waitRejects && !tc.truncated; result.Passed != wantPass {
				t.Errorf("passed = %v, want %v", result.Passed, wantPass)
			}
			if pending != 0 {
				t.Errorf("pending timers = %d, want 0", pending)
			}
			if tc.waitRejects {
				if removes != 0 || !result.CleanupFailed || !result.WorktreeRetained {
					t.Errorf("unconfirmed termination must retain worktree: removes=%d result=%+v", removes, result)
				}
			} else if removes != 1 || result.CleanupFailed || result.WorktreeRetained {
				t.Errorf("completed child must clean up once: removes=%d result=%+v", removes, result)
			}
		})
	}
}

func TestVerifySplitsAsync_TruncatedTimeoutPreservesDependencyGuard(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	for _, baselineTruncated := range []bool{false, true} {
		t.Run(fmt.Sprintf("baselineTruncated=%v", baselineTruncated), func(t *testing.T) {
			t.Parallel()
			evalJS := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
			quotedDir, _ := json.Marshal(t.TempDir())
			raw, err := evalJS(fmt.Sprintf(`(async function() {
				var exec = prSplit._modules.exec;
				var originalSpawn = exec.spawn;
				var originalGit = prSplit._gitExecAsync;
				var spawns = 0;
				prSplit._gitExecAsync = function() { return {code: 0, stdout: '', stderr: ''}; };
				exec.spawn = function() {
					var baseline = spawns++ === 0;
					var truncated = baseline === %v;
					var code = truncated ? 124 : (baseline ? 7 : 2);
					return {
						stdout: { read: function() { return Promise.resolve({done: true, truncated: truncated}); } },
						stderr: { read: function() { return Promise.resolve({done: true}); } },
						wait: function() { return Promise.resolve({code: code, signal: null}); },
						kill: function() { return Promise.resolve(); }
					};
				};
				try {
					var result = await prSplit.verifySplitsAsync({
						dir: %s, sourceBranch: 'source', verifyCommand: 'check',
						splits: [{name: 'split/one', files: ['one']}, {name: 'split/two', files: ['two'], dependencies: ['split/one']}]
					}, {verifyTimeoutMs: 600000});
					return JSON.stringify({result: result, spawns: spawns});
				} finally {
					exec.spawn = originalSpawn;
					prSplit._gitExecAsync = originalGit;
				}
			})()`, baselineTruncated, quotedDir))
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Result struct {
					AllPassed bool `json:"allPassed"`
					Results   []struct {
						PreExisting bool `json:"preExisting"`
						Skipped     bool `json:"skipped"`
						TimedOut    bool `json:"timedOut"`
						Truncated   bool `json:"truncated"`
					} `json:"results"`
				} `json:"result"`
				Spawns int `json:"spawns"`
			}
			if err := json.Unmarshal([]byte(raw.(string)), &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Result.Results) != 2 || report.Spawns != 2 || report.Result.AllPassed {
				t.Fatalf("verification must stop the dependent branch: %s", raw)
			}
			first, second := report.Result.Results[0], report.Result.Results[1]
			if first.PreExisting || !second.Skipped {
				t.Errorf("unknown verdict must not be forgiven: %s", raw)
			}
			if first.TimedOut != !baselineTruncated || first.Truncated != !baselineTruncated {
				t.Errorf("split must retain both timeout and capture attribution: %s", raw)
			}
		})
	}
}
