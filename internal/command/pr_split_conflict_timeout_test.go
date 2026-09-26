package command

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestPrSplitCommand_ResolveConflicts_TimeoutPropagatedToStrategy(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	// Create a branch that will fail verification.
	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/timeout-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Use a custom strategy that captures the options parameter passed by resolveConflicts.
	// This proves the timeout chain: resolveConflicts options → strategy.fix() options.
	val, err := evalJS(`(async function() {
		var capturedOptions = null;
		var shellTimeouts = [];
		var originalShellExecAsync = globalThis.prSplit._shellExecAsync;
		globalThis.prSplit._shellExecAsync = function(command, options) {
			shellTimeouts.push(options && options.timeoutMs);
			return Promise.resolve({ code: 1, stdout: '', stderr: 'intentional verify failure' });
		};
		var customStrategy = {
			name: 'capture-timeout',
			detect: function() { return true; },
			fix: function(dir, branch, plan, verifyOutput, options) {
				capturedOptions = options;
				return { fixed: false, error: 'intentional fail to capture options' };
			}
		};

		try {
			var result = await globalThis.prSplit.resolveConflicts({
				dir: '` + strings.ReplaceAll(dir, `\`, `\\`) + `',
				splits: [
					{ name: 'split/timeout-test', files: ['a.go'] }
				],
				verifyCommand: 'exit 1'
			}, {
				retryBudget: 1,
				strategies: [customStrategy],
				resolveTimeoutMs: 60000,
				pollIntervalMs: 250,
				verifyTimeoutMs: 12345
			});
			return JSON.stringify({
				options: capturedOptions,
				shellTimeouts: shellTimeouts,
				errors: result.errors
			});
		} finally {
			globalThis.prSplit._shellExecAsync = originalShellExecAsync;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Options struct {
			VerifyTimeoutMs  float64 `json:"verifyTimeoutMs"`
			ResolveTimeoutMs float64 `json:"resolveTimeoutMs"`
			PollIntervalMs   float64 `json:"pollIntervalMs"`
		} `json:"options"`
		ShellTimeouts []float64 `json:"shellTimeouts"`
		Errors        []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Verify the custom timeout was propagated to the strategy.
	if output.Options.VerifyTimeoutMs != 12345 {
		t.Errorf("Expected verifyTimeoutMs=12345 in strategy options, got %v", output.Options.VerifyTimeoutMs)
	}
	if len(output.ShellTimeouts) == 0 || output.ShellTimeouts[0] != 12345 {
		t.Errorf("Expected shell verification timeout=12345, got %v", output.ShellTimeouts)
	}
	if output.Options.ResolveTimeoutMs != 60000 {
		t.Errorf("Expected resolveTimeoutMs=60000 in strategy options, got %v", output.Options.ResolveTimeoutMs)
	}
	if output.Options.PollIntervalMs != 250 {
		t.Errorf("Expected pollIntervalMs=250 in strategy options, got %v", output.Options.PollIntervalMs)
	}
}
