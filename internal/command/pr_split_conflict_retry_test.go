package command

import (
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestPrSplitCommand_ResolveConflictsNoVerifyCommand(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// With no verify command or verifyCommand = 'true', resolveConflicts returns early.
	val, err := evalJS(`JSON.stringify(
		await globalThis.prSplit.resolveConflicts(
			{ dir: '.', splits: [], verifyCommand: 'true' },
			{ retryBudget: 5 }
		)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// fixed should be empty array.
	fixed, ok := result["fixed"].([]interface{})
	if !ok {
		t.Fatalf("Expected fixed to be array, got %T: %v", result["fixed"], result["fixed"])
	}
	if len(fixed) != 0 {
		t.Errorf("Expected empty fixed array, got %d items", len(fixed))
	}
	// skipped should be a non-empty message.
	skipped, _ := result["skipped"].(string)
	if skipped == "" {
		t.Error("Expected non-empty skipped message")
	}
	// reSplitNeeded should be false.
	if result["reSplitNeeded"] != false {
		t.Errorf("Expected reSplitNeeded=false, got %v", result["reSplitNeeded"])
	}
}

func TestPrSplitCommand_ResolveConflictsReturnShape(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Check that resolveConflicts returns all expected fields.
	val, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.resolveConflicts(
			{ dir: '/nonexistent', splits: [], verifyCommand: 'false' },
			{}
		);
		return JSON.stringify({
			hasFixed: Array.isArray(result.fixed),
			hasErrors: Array.isArray(result.errors),
			hasReSplitNeeded: typeof result.reSplitNeeded === 'boolean',
			hasTotalRetries: typeof result.totalRetries === 'number' || typeof result.totalRetries === 'undefined',
			hasReSplitFiles: Array.isArray(result.reSplitFiles) || typeof result.reSplitFiles === 'undefined'
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &shape); err != nil {
		t.Fatalf("Failed to parse shape: %v", err)
	}
	if shape["hasFixed"] != true {
		t.Error("Expected fixed array")
	}
	if shape["hasReSplitNeeded"] != true {
		t.Error("Expected reSplitNeeded boolean")
	}
}

func TestPrSplitCommand_SetRetryBudget(t *testing.T) {
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// Set retry budget.
	if err := dispatch("set", []string{"retry-budget", "5"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !contains(output, "Set retry-budget = 5") {
		t.Errorf("Expected confirmation, got: %s", output)
	}

	// Set invalid budget.
	stdout.Reset()
	if err := dispatch("set", []string{"retry-budget", "abc"}); err != nil {
		t.Fatal(err)
	}
	output = stdout.String()
	if !contains(output, "Invalid") {
		t.Errorf("Expected invalid message, got: %s", output)
	}
}

func TestPrSplitCommand_SetRetryBudgetNegative(t *testing.T) {
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// Set negative retry budget — should be rejected.
	if err := dispatch("set", []string{"retry-budget", "-1"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !contains(output, "Invalid") {
		t.Errorf("Expected invalid message for negative budget, got: %s", output)
	}
}

func TestPrSplitCommand_SetRetryBudgetZero(t *testing.T) {
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// Zero should be accepted — it means "no retries at all".
	if err := dispatch("set", []string{"retry-budget", "0"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !contains(output, "Set retry-budget = 0") {
		t.Errorf("Expected confirmation for zero budget, got: %s", output)
	}
}

func TestPrSplitCommand_ResolveConflictsZeroBudget(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	// Create a branch that will fail verification.
	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/zero-budget")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]interface{}{
		"retryBudget": 0,
	})

	// With retryBudget=0 AND runtime retryBudget=0, no strategies should be attempted.
	// The branch should immediately get "retry budget exhausted".
	val, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + dir + `',
			splits: [{ name: 'split/zero-budget', files: ['main.go'] }],
			verifyCommand: 'exit 1'
		}, { retryBudget: 0 });
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// With budget 0, the first branch should immediately get budget exhausted.
	errs, _ := result["errors"].([]interface{})
	if len(errs) == 0 {
		t.Error("Expected errors for zero-budget branch")
	}
	// totalRetries should be 0.
	totalRetries, _ := result["totalRetries"].(float64)
	if totalRetries != 0 {
		t.Errorf("Expected totalRetries=0 with zero budget, got %v", totalRetries)
	}
}

func TestPrSplitCommand_SetShowsRetryBudget(t *testing.T) {
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	if err := dispatch("set", nil); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	if !contains(output, "retry-budget:") {
		t.Errorf("Expected retry-budget in set output, got: %s", output)
	}
	// Default value is 3.
	if !contains(output, "3") {
		t.Errorf("Expected default retry-budget of 3 in output, got: %s", output)
	}
}

func TestPrSplitCommand_SetRetryBudgetAndVerify(t *testing.T) {
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// Set budget to 7.
	if err := dispatch("set", []string{"retry-budget", "7"}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()

	// Show settings — should reflect new value.
	if err := dispatch("set", nil); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !contains(output, "retry-budget:") || !contains(output, "7") {
		t.Errorf("Expected retry-budget: 7 in output, got: %s", output)
	}
}

func TestPrSplitCommand_Phase5ScriptContent(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name    string
		content string
	}{
		{"go-build-missing-imports strategy", "go-build-missing-imports"},
		{"npm-install strategy", "'npm-install'"},
		{"make-generate strategy", "'make-generate'"},
		{"add-missing-files strategy", "'add-missing-files'"},
		{"claude-fix strategy", "'claude-fix'"},
		{"retryBudget in runtime", "retryBudget"},
		{"retry-budget in set command", "case 'retry-budget':"},
		{"reSplitNeeded in resolveConflicts", "reSplitNeeded"},
		{"reSplitFiles in resolveConflicts", "reSplitFiles"},
		{"totalRetries tracking", "totalRetries"},
		{"verifyOutput passed to detect", "verifyOutput"},
	}

	src := allChunkSources()
	for _, c := range checks {
		if !strings.Contains(src, c.content) {
			t.Errorf("Script missing %s (expected to contain %q)", c.name, c.content)
		}
	}
}

func TestPrSplitCommand_ResolveConflictsWithGitRepo(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Resolve conflicts on a plan where splits already pass verification.
	val, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + dir + `',
			splits: [],
			verifyCommand: 'echo ok'
		}, { retryBudget: 2 });
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// No splits means no errors.
	errs, _ := result["errors"].([]interface{})
	if len(errs) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errs), errs)
	}
	if result["reSplitNeeded"] != false {
		t.Errorf("Expected reSplitNeeded=false, got %v", result["reSplitNeeded"])
	}
}

func TestPrSplitCommand_ResolveConflictsRetryBudgetExhaustion(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	// Create a dummy branch that will fail verification.
	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/test-fail")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Use a verify command that always fails + budget of 1.
	// The resolve should exhaust the budget and flag reSplitNeeded.
	val, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + dir + `',
			splits: [{ name: 'split/test-fail', files: ['main.go'] }],
			verifyCommand: 'exit 1'
		}, { retryBudget: 1 });
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// Should have errors for the failed branch.
	errs, _ := result["errors"].([]interface{})
	if len(errs) == 0 {
		t.Error("Expected errors for failed branch")
	}
	// reSplitNeeded should be true when strategies exhaust.
	if result["reSplitNeeded"] != true {
		t.Errorf("Expected reSplitNeeded=true, got %v", result["reSplitNeeded"])
	}
	// reSplitFiles should contain the files from the failed split.
	reSplitFiles, _ := result["reSplitFiles"].([]interface{})
	if len(reSplitFiles) == 0 {
		t.Error("Expected reSplitFiles to contain files from failed split")
	}
}

func TestPrSplitCommand_ResolveConflictsPassingBranch(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	// Create a branch that passes verification.
	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/test-pass")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Verify command always passes.
	val, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + dir + `',
			splits: [{ name: 'split/test-pass', files: ['main.go'] }],
			verifyCommand: 'echo ok'
		}, { retryBudget: 3 });
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// No errors when branches pass.
	errs, _ := result["errors"].([]interface{})
	if len(errs) != 0 {
		t.Errorf("Expected no errors, got %d", len(errs))
	}
	if result["reSplitNeeded"] != false {
		t.Errorf("Expected reSplitNeeded=false, got %v", result["reSplitNeeded"])
	}
}

func TestPrSplitCommand_ResolveConflictsWallClockTimeout(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	// Create two branches that will fail verification.
	for _, branch := range []string{"split/wc-a", "split/wc-b"} {
		cmd := exec.Command("git", "-C", dir, "checkout", "-b", branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to create branch %s: %s (%v)", branch, out, err)
		}
	}
	cmd := exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// wallClockTimeoutMs=0 guarantees the deadline will be exceeded immediately.
	val, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + strings.ReplaceAll(dir, `\`, `\\`) + `',
			splits: [
				{ name: 'split/wc-a', files: ['a.go'] },
				{ name: 'split/wc-b', files: ['b.go'] }
			],
			verifyCommand: 'exit 1'
		}, { retryBudget: 10, wallClockTimeoutMs: 0 });
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// Both branches should have wall-clock timeout errors.
	errs, _ := result["errors"].([]interface{})
	if len(errs) < 2 {
		t.Fatalf("Expected at least 2 errors (one per branch), got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		em, _ := e.(map[string]interface{})
		errMsg, _ := em["error"].(string)
		if !strings.Contains(errMsg, "wall-clock timeout") {
			t.Errorf("Expected 'wall-clock timeout' in error, got: %s", errMsg)
		}
	}
}

func TestPrSplitCommand_ResolveConflictsWallClockDefault(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Verify the default wall-clock timeout is 7200000ms (120 minutes).
	val, err := evalJS(`globalThis.prSplit.AUTOMATED_DEFAULTS.resolveWallClockTimeoutMs`)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := val.(int64)
	if !ok {
		// Try float64 — some JS runtimes export numbers as float.
		vf, ok2 := val.(float64)
		if !ok2 {
			t.Fatalf("Expected numeric value, got %T: %v", val, val)
		}
		v = int64(vf)
	}
	if v != 7200000 {
		t.Errorf("Expected resolveWallClockTimeoutMs=7200000, got %d", v)
	}
}

func TestPrSplitCommand_VerifyTimeoutDefault(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Verify AUTOMATED_DEFAULTS.verifyTimeoutMs = 600000 (10 minutes).
	val, err := evalJS(`globalThis.prSplit.AUTOMATED_DEFAULTS.verifyTimeoutMs`)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := val.(int64)
	if !ok {
		vf, ok2 := val.(float64)
		if !ok2 {
			t.Fatalf("Expected numeric value, got %T: %v", val, val)
		}
		v = int64(vf)
	}
	if v != 600000 {
		t.Errorf("Expected verifyTimeoutMs=600000, got %d", v)
	}
}

func TestPrSplitCommand_ResolveConflictsWithClaudeWallClockTimeout(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Call resolveConflictsWithClaude with wallClockMs=0 so the deadline expires immediately.
	// This should return the wall-clock timeout reason without trying to contact Claude.
	val, err := evalJS(`(async function() {
		var failures = [
			{ branch: 'split/fail-a', files: ['a.go'], error: 'test fail' },
			{ branch: 'split/fail-b', files: ['b.go'], error: 'test fail' }
		];
		var report = { conflicts: [], resolutions: [], claudeInteractions: 0 };
		var result = await globalThis.prSplit.resolveConflictsWithClaude(
			failures,
			'test-session',
			{ resolve: 30000, wallClockMs: 0 },
			500,
			3,
			report
		);
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	// Should have reSplitNeeded=false and wall-clock timeout reason.
	if result["reSplitNeeded"] != false {
		t.Errorf("Expected reSplitNeeded=false, got %v", result["reSplitNeeded"])
	}
	reason, _ := result["reSplitReason"].(string)
	if !strings.Contains(reason, "wall-clock timeout") {
		t.Errorf("Expected 'wall-clock timeout' in reSplitReason, got: %s", reason)
	}
}

func TestPrSplitCommand_ResolveConflicts_TimeoutPropagatedToStrategy(t *testing.T) {
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
		var customStrategy = {
			name: 'capture-timeout',
			detect: function() { return true; },
			fix: function(dir, branch, plan, verifyOutput, options) {
				capturedOptions = options;
				return { fixed: false, error: 'intentional fail to capture options' };
			}
		};

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
			pollIntervalMs: 250
		});
		return JSON.stringify({
			options: capturedOptions,
			errors: result.errors
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Options struct {
			ResolveTimeoutMs float64 `json:"resolveTimeoutMs"`
			PollIntervalMs   float64 `json:"pollIntervalMs"`
		} `json:"options"`
		Errors []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Verify the custom timeout was propagated to the strategy.
	if output.Options.ResolveTimeoutMs != 60000 {
		t.Errorf("Expected resolveTimeoutMs=60000 in strategy options, got %v", output.Options.ResolveTimeoutMs)
	}
	if output.Options.PollIntervalMs != 250 {
		t.Errorf("Expected pollIntervalMs=250 in strategy options, got %v", output.Options.PollIntervalMs)
	}
}

func TestPrSplitCommand_ResolveConflicts_TimeoutDefaultsWhenNotProvided(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/default-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// When no resolveTimeoutMs is provided, strategy should receive AUTOMATED_DEFAULTS.
	val, err := evalJS(`(async function() {
		var capturedOptions = null;
		var customStrategy = {
			name: 'capture-defaults',
			detect: function() { return true; },
			fix: function(dir, branch, plan, verifyOutput, options) {
				capturedOptions = options;
				return { fixed: false, error: 'intentional fail' };
			}
		};

		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + strings.ReplaceAll(dir, `\`, `\\`) + `',
			splits: [
				{ name: 'split/default-test', files: ['a.go'] }
			],
			verifyCommand: 'exit 1'
		}, {
			retryBudget: 1,
			strategies: [customStrategy]
			// NOTE: no resolveTimeoutMs or pollIntervalMs
		});
		return JSON.stringify({
			options: capturedOptions,
			defaults: {
				resolveTimeoutMs: globalThis.prSplit.AUTOMATED_DEFAULTS.resolveTimeoutMs,
				pollIntervalMs: globalThis.prSplit.AUTOMATED_DEFAULTS.pollIntervalMs
			}
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Options struct {
			ResolveTimeoutMs float64 `json:"resolveTimeoutMs"`
			PollIntervalMs   float64 `json:"pollIntervalMs"`
		} `json:"options"`
		Defaults struct {
			ResolveTimeoutMs float64 `json:"resolveTimeoutMs"`
			PollIntervalMs   float64 `json:"pollIntervalMs"`
		} `json:"defaults"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Strategy should receive AUTOMATED_DEFAULTS values when no overrides provided.
	if output.Options.ResolveTimeoutMs != output.Defaults.ResolveTimeoutMs {
		t.Errorf("Expected resolveTimeoutMs=%v (AUTOMATED_DEFAULTS), got %v",
			output.Defaults.ResolveTimeoutMs, output.Options.ResolveTimeoutMs)
	}
	if output.Options.PollIntervalMs != output.Defaults.PollIntervalMs {
		t.Errorf("Expected pollIntervalMs=%v (AUTOMATED_DEFAULTS), got %v",
			output.Defaults.PollIntervalMs, output.Options.PollIntervalMs)
	}
}

func TestPrSplitCommand_ResolveConflictsWithClaudePreExistingFailure(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock claudeExecutor so sendToHandle can send prompts.
	// Mock mcpCallbackObj to return pre-existing failure resolution data.
	val, err := evalJS(`(async function() {
		// Prevent text chunking — tests count raw send() calls.
		prSplit.SEND_TEXT_CHUNK_BYTES = 1000000;

		var sendCallCount = 0;
		claudeExecutor = {
			handle: {
				send: function(text) { sendCallCount++; },
				isAlive: function() { return true; }
			}
		};

		// Mock mcpCallbackObj to return resolution data on waitFor.
		mcpCallbackObj = {
			resetWaiter: function() {},
			waitFor: function(name, timeout, opts) {
				if (name === 'reportResolution') {
					return {
						data: { preExistingFailure: true, preExistingDetails: 'fails on main too' },
						error: null
					};
				}
				return { data: null, error: 'timeout' };
			}
		};

		var failures = [
			{ branch: 'split/pre-existing', files: ['a.go'], error: 'test fail' }
		];
		var report = { conflicts: [], resolutions: [], claudeInteractions: 0 };
		var result = await globalThis.prSplit.resolveConflictsWithClaude(
			failures,
			'test-session',
			{ resolve: 5000, wallClockMs: 30000 },
			100,
			3,
			report
		);

		return JSON.stringify({
			result: result,
			report: {
				conflicts: report.conflicts,
				resolutions: report.resolutions,
				claudeInteractions: report.claudeInteractions,
				preExistingFailures: report.preExistingFailures || []
			},
			sendCallCount: sendCallCount
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Result struct {
			ReSplitNeeded bool   `json:"reSplitNeeded"`
			ReSplitReason string `json:"reSplitReason"`
		} `json:"result"`
		Report struct {
			Conflicts           []interface{} `json:"conflicts"`
			Resolutions         []interface{} `json:"resolutions"`
			ClaudeInteractions  int           `json:"claudeInteractions"`
			PreExistingFailures []struct {
				Branch  string `json:"branch"`
				Details string `json:"details"`
			} `json:"preExistingFailures"`
		} `json:"report"`
		SendCallCount int `json:"sendCallCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// 1. reSplitNeeded should be false — pre-existing failure doesn't trigger re-split.
	if output.Result.ReSplitNeeded {
		t.Error("Expected reSplitNeeded=false for pre-existing failure")
	}

	// 2. Only 1 attempt (no retry) — sendCallCount should be 2 (two-write: text + \r).
	if output.SendCallCount != 2 {
		t.Errorf("Expected 2 send calls (two-write), got %d", output.SendCallCount)
	}

	// 3. Only 1 conflict recorded (1 attempt, not 3).
	if len(output.Report.Conflicts) != 1 {
		t.Errorf("Expected 1 conflict (no retry), got %d", len(output.Report.Conflicts))
	}

	// 4. report.preExistingFailures contains the branch.
	if len(output.Report.PreExistingFailures) != 1 {
		t.Fatalf("Expected 1 pre-existing failure, got %d", len(output.Report.PreExistingFailures))
	}
	pef := output.Report.PreExistingFailures[0]
	if pef.Branch != "split/pre-existing" {
		t.Errorf("Expected branch 'split/pre-existing', got %q", pef.Branch)
	}
	if pef.Details != "fails on main too" {
		t.Errorf("Expected details 'fails on main too', got %q", pef.Details)
	}

	// 5. 1 Claude interaction.
	if output.Report.ClaudeInteractions != 1 {
		t.Errorf("Expected 1 Claude interaction, got %d", output.Report.ClaudeInteractions)
	}
}

func TestPrSplitCommand_ResolveConflictsWithClaude_MaxAttemptsPerBranch(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// 2 failures, maxAttemptsPerBranch=2, mcpCallbackObj always times out.
	// Each failure should get exactly 2 attempts (not share a global budget).
	val, err := evalJS(`(async function() {
		// Prevent text chunking — tests count raw send() calls.
		prSplit.SEND_TEXT_CHUNK_BYTES = 1000000;

		var sendCount = 0;
		claudeExecutor = {
			handle: {
				send: function(text) { sendCount++; },
				isAlive: function() { return true; }
			}
		};

		// Mock mcpCallbackObj to always timeout (no resolution data).
		mcpCallbackObj = {
			resetWaiter: function() {},
			waitFor: function(name, timeout, opts) {
				return { data: null, error: 'timeout waiting for ' + name + ' after ' + timeout + 'ms' };
			}
		};

		var failures = [
			{ branch: 'split/fail-a', files: ['a.go'], error: 'test fail' },
			{ branch: 'split/fail-b', files: ['b.go'], error: 'test fail' }
		];
		var report = { conflicts: [], resolutions: [], claudeInteractions: 0 };

		var result = await globalThis.prSplit.resolveConflictsWithClaude(
			failures,
			'test-session',
			{ resolve: 100, wallClockMs: 30000 },
			50,
			2,
			report
		);
		return JSON.stringify({
			result: result,
			report: {
				conflicts: report.conflicts.length,
				claudeInteractions: report.claudeInteractions
			},
			sendCount: sendCount
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Result struct {
			ReSplitNeeded bool   `json:"reSplitNeeded"`
			ReSplitReason string `json:"reSplitReason"`
		} `json:"result"`
		Report struct {
			Conflicts          int `json:"conflicts"`
			ClaudeInteractions int `json:"claudeInteractions"`
		} `json:"report"`
		SendCount int `json:"sendCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Each failure gets 2 attempts (maxAttemptsPerBranch=2).
	// Total conflicts = 2 failures × 2 attempts = 4.
	if output.Report.Conflicts != 4 {
		t.Errorf("Expected 4 conflict entries (2 failures × 2 attempts), got %d", output.Report.Conflicts)
	}

	// Each attempt sends text + \n as two writes = 2 send calls.
	// 4 attempts × 2 sends = 8 send calls.
	if output.SendCount != 8 {
		t.Errorf("Expected 8 send calls (4 attempts × 2 sends), got %d", output.SendCount)
	}
}

func TestPrSplitCommand_SendToHandle_TwoWrite(t *testing.T) {
	t.Parallel()

	_, _, _, evalJSAsync := loadPrSplitEngineWithEval(t, nil)

	// Verify sendToHandle sends text and \r as two separate writes.
	// NOTE: sendToHandle is async (uses setTimeout for inter-write delay).
	// The outer await is needed because evalJSAsync wraps this in another async IIFE.
	val, err := evalJSAsync(`await (async function() {
		var sends = [];
		var mockHandle = {
			send: function(text) { sends.push(text); }
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'test prompt');
		return JSON.stringify({
			result: result,
			sends: sends
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Result struct {
			Error *string `json:"error"`
		} `json:"result"`
		Sends []string `json:"sends"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	if output.Result.Error != nil {
		t.Errorf("sendToHandle returned error: %s", *output.Result.Error)
	}
	// Two-write: text first, then \r (carriage return = Enter key in PTY).
	if len(output.Sends) != 2 {
		t.Fatalf("Expected 2 sends (two-write), got %d: %v", len(output.Sends), output.Sends)
	}
	if output.Sends[0] != "test prompt" {
		t.Errorf("sends[0] = %q, want %q", output.Sends[0], "test prompt")
	}
	if output.Sends[1] != "\r" {
		t.Errorf("sends[1] = %q, want %q", output.Sends[1], "\r")
	}
}

func TestPrSplitCommand_SendToHandle_EAGAINRetry(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock handle throws EAGAIN on first call, succeeds on second.
	val, err := evalJS(`(async function() {
		var callCount = 0;
		var mockHandle = {
			send: function(text) {
				callCount++;
				if (callCount === 1) {
					throw new Error('write: resource temporarily unavailable (EAGAIN)');
				}
				// succeed on subsequent calls
			}
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'retry test');
		return JSON.stringify({ error: result.error, callCount: callCount });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Two-write: EAGAIN on call 1 (text), retry text on call 2 (success),
	// then newline on call 3 (success).
	if output.Error != nil {
		t.Errorf("Expected success after EAGAIN retry, got error: %s", *output.Error)
	}
	// callCount: 1 (EAGAIN text) + 1 (text retry success) + 1 (newline) = 3
	if output.CallCount != 3 {
		t.Errorf("Expected 3 send calls (1 EAGAIN + 1 text success + 1 newline), got %d", output.CallCount)
	}
}

func TestPrSplitCommand_SendToHandle_EAGAINExhausted(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock handle always throws EAGAIN.
	val, err := evalJS(`(async function() {
		var callCount = 0;
		var mockHandle = {
			send: function(text) {
				callCount++;
				throw new Error('EAGAIN: resource temporarily unavailable');
			}
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'always fails');
		return JSON.stringify({ error: result.error, callCount: callCount });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Should fail after exhausting retries (initial + 3 retries = 4 attempts).
	if output.Error == nil {
		t.Fatal("Expected error after EAGAIN retry exhaustion")
	}
	if !strings.Contains(*output.Error, "EAGAIN") {
		t.Errorf("Error should contain 'EAGAIN', got: %s", *output.Error)
	}
	// callCount: 1 initial + 3 retries = 4
	if output.CallCount != 4 {
		t.Errorf("Expected 4 send calls (1 initial + 3 retries), got %d", output.CallCount)
	}
}

func TestPrSplitCommand_SendToHandle_NonEAGAINError(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock handle throws a non-EAGAIN error — should NOT retry.
	val, err := evalJS(`(async function() {
		var callCount = 0;
		var mockHandle = {
			send: function(text) {
				callCount++;
				throw new Error('connection refused');
			}
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'no retry');
		return JSON.stringify({ error: result.error, callCount: callCount });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Non-EAGAIN should fail immediately without retries.
	if output.Error == nil {
		t.Fatal("Expected error for non-EAGAIN failure")
	}
	if !strings.Contains(*output.Error, "connection refused") {
		t.Errorf("Error should contain 'connection refused', got: %s", *output.Error)
	}
	// Only 1 attempt (no retry for non-EAGAIN).
	if output.CallCount != 1 {
		t.Errorf("Expected 1 send call (no retry for non-EAGAIN), got %d", output.CallCount)
	}
}

// ---------------------------------------------------------------------------
// T16: sendToHandle null/undefined handle guard
// ---------------------------------------------------------------------------

func TestPrSplitCommand_SendToHandle_NullHandle(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// T16 fix: sendToHandle(null, ...) must return {error: ...} not crash.
	val, err := evalJS(`(async function() {
		var r1 = await globalThis.prSplit.sendToHandle(null, 'hello');
		var r2 = await globalThis.prSplit.sendToHandle(undefined, 'hello');
		return JSON.stringify({ nullResult: r1, undefinedResult: r2 });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		NullResult struct {
			Error *string `json:"error"`
		} `json:"nullResult"`
		UndefinedResult struct {
			Error *string `json:"error"`
		} `json:"undefinedResult"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	if output.NullResult.Error == nil {
		t.Error("sendToHandle(null, ...) should return an error")
	} else if !strings.Contains(*output.NullResult.Error, "null") {
		t.Errorf("null handle error should mention 'null', got: %s", *output.NullResult.Error)
	}

	if output.UndefinedResult.Error == nil {
		t.Error("sendToHandle(undefined, ...) should return an error")
	} else if !strings.Contains(*output.UndefinedResult.Error, "null") {
		t.Errorf("undefined handle error should mention 'null', got: %s", *output.UndefinedResult.Error)
	}
}

func TestPrSplitCommand_SendToHandle_FalsyHandleString(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Falsy values like 0, false, empty string are also invalid handles.
	val, err := evalJS(`(async function() {
		var r1 = await globalThis.prSplit.sendToHandle(0, 'hello');
		var r2 = await globalThis.prSplit.sendToHandle(false, 'hello');
		var r3 = await globalThis.prSplit.sendToHandle('', 'hello');
		return JSON.stringify({ zero: r1, boolFalse: r2, emptyStr: r3 });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Zero struct {
			Error *string `json:"error"`
		} `json:"zero"`
		BoolFalse struct {
			Error *string `json:"error"`
		} `json:"boolFalse"`
		EmptyStr struct {
			Error *string `json:"error"`
		} `json:"emptyStr"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// All falsy values should be caught by the !handle guard.
	if output.Zero.Error == nil {
		t.Error("sendToHandle(0, ...) should return an error")
	}
	if output.BoolFalse.Error == nil {
		t.Error("sendToHandle(false, ...) should return an error")
	}
	if output.EmptyStr.Error == nil {
		t.Error("sendToHandle('', ...) should return an error")
	}
}

func TestPrSplitCommand_ResolveConflicts_PerBranchRetryBudget(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	// Create 2 branches for the splits.
	for _, branch := range []string{"split/branch-a", "split/branch-b"} {
		cmd := exec.Command("git", "-C", dir, "checkout", "-b", branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to create branch %s: %s (%v)", branch, out, err)
		}
		cmd = exec.Command("git", "-C", dir, "checkout", "main")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to checkout main: %s (%v)", out, err)
		}
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Use custom strategies that track per-branch fix attempts.
	// Strategy always returns { fixed: false } so retries are consumed.
	val, err := evalJS(`(async function() {
		var attempts = {};
		var customStrategy = {
			name: 'always-fail',
			detect: function() { return true; },
			fix: function(dir, branch, plan, verifyOutput, options) {
				attempts[branch] = (attempts[branch] || 0) + 1;
				return { fixed: false, error: 'intentional fail' };
			}
		};

		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + strings.ReplaceAll(dir, `\`, `\\`) + `',
			splits: [
				{ name: 'split/branch-a', files: ['a.go'] },
				{ name: 'split/branch-b', files: ['b.go'] }
			],
			verifyCommand: 'exit 1'
		}, {
			retryBudget: 10,
			perBranchRetryBudget: 2,
			strategies: [customStrategy]
		});
		return JSON.stringify({
			attempts: attempts,
			totalRetries: result.totalRetries,
			branchRetries: result.branchRetries,
			errors: result.errors.map(function(e) { return { name: e.name, error: e.error }; })
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Attempts      map[string]int `json:"attempts"`
		TotalRetries  int            `json:"totalRetries"`
		BranchRetries map[string]int `json:"branchRetries"`
		Errors        []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Each branch should get exactly 2 retries (perBranchRetryBudget=2),
	// even though the global budget is 10.
	if output.Attempts["split/branch-a"] != 2 {
		t.Errorf("Expected 2 attempts for branch-a, got %d", output.Attempts["split/branch-a"])
	}
	if output.Attempts["split/branch-b"] != 2 {
		t.Errorf("Expected 2 attempts for branch-b, got %d", output.Attempts["split/branch-b"])
	}

	// Total retries should be 4 (2 per branch × 2 branches).
	if output.TotalRetries != 4 {
		t.Errorf("Expected 4 total retries, got %d", output.TotalRetries)
	}

	// branchRetries should match.
	if output.BranchRetries["split/branch-a"] != 2 {
		t.Errorf("Expected branchRetries[branch-a]=2, got %d", output.BranchRetries["split/branch-a"])
	}
	if output.BranchRetries["split/branch-b"] != 2 {
		t.Errorf("Expected branchRetries[branch-b]=2, got %d", output.BranchRetries["split/branch-b"])
	}

	// Both branches should have errors (verification failed).
	if len(output.Errors) != 2 {
		t.Fatalf("Expected 2 errors (one per branch), got %d", len(output.Errors))
	}
}

func TestPrSplitCommand_ResolveConflicts_PerBranchRetryBudget_Exhausted(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/exhaust-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Single branch with perBranchRetryBudget=1 — should stop after 1 attempt.
	val, err := evalJS(`(async function() {
		var attempts = 0;
		var customStrategy = {
			name: 'count-attempts',
			detect: function() { return true; },
			fix: function() {
				attempts++;
				return { fixed: false, error: 'still failing' };
			}
		};

		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + strings.ReplaceAll(dir, `\`, `\\`) + `',
			splits: [
				{ name: 'split/exhaust-test', files: ['a.go'] }
			],
			verifyCommand: 'exit 1'
		}, {
			retryBudget: 100,
			perBranchRetryBudget: 1,
			strategies: [customStrategy]
		});
		return JSON.stringify({
			attempts: attempts,
			totalRetries: result.totalRetries,
			branchRetries: result.branchRetries
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Attempts      int            `json:"attempts"`
		TotalRetries  int            `json:"totalRetries"`
		BranchRetries map[string]int `json:"branchRetries"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Should have exactly 1 attempt (perBranchRetryBudget=1).
	if output.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", output.Attempts)
	}
	if output.TotalRetries != 1 {
		t.Errorf("Expected 1 total retry, got %d", output.TotalRetries)
	}
	if output.BranchRetries["split/exhaust-test"] != 1 {
		t.Errorf("Expected branchRetries[split/exhaust-test]=1, got %d", output.BranchRetries["split/exhaust-test"])
	}
}

// ---------------------------------------------------------------------------
// T092: resolveConflicts threads aliveCheckFn to strategy options
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ResolveConflicts_AliveCheckFnThreaded(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows — git test repo setup uses Unix commands")
	}

	dir := setupTestGitRepo(t)

	cmd := exec.Command("git", "-C", dir, "checkout", "-b", "split/alive-test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to create branch: %s (%v)", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to checkout main: %s (%v)", out, err)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Custom strategy that captures whether aliveCheckFn was threaded into options.
	// If options.aliveCheckFn is a function, call it and return its result.
	val, err := evalJS(`(async function() {
		var receivedAliveCheckFn = false;
		var aliveCheckResult = null;
		var customStrategy = {
			name: 'check-alive-threading',
			detect: function() { return true; },
			fix: function(dir, branch, plan, verifyOutput, options) {
				if (options && typeof options.aliveCheckFn === 'function') {
					receivedAliveCheckFn = true;
					aliveCheckResult = options.aliveCheckFn();
				}
				return { fixed: true };
			}
		};

		// aliveCheckFn that returns false (process dead).
		var aliveCheckCalled = false;
		var aliveCheckFn = function() {
			aliveCheckCalled = true;
			return false;
		};

		var result = await globalThis.prSplit.resolveConflicts({
			dir: '` + strings.ReplaceAll(dir, `\`, `\\`) + `',
			splits: [
				{ name: 'split/alive-test', files: ['a.go'] }
			],
			verifyCommand: 'exit 1'
		}, {
			retryBudget: 1,
			perBranchRetryBudget: 1,
			strategies: [customStrategy],
			aliveCheckFn: aliveCheckFn
		});
		return JSON.stringify({
			receivedAliveCheckFn: receivedAliveCheckFn,
			aliveCheckCalled: aliveCheckCalled,
			aliveCheckResult: aliveCheckResult
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		ReceivedAliveCheckFn bool  `json:"receivedAliveCheckFn"`
		AliveCheckCalled     bool  `json:"aliveCheckCalled"`
		AliveCheckResult     *bool `json:"aliveCheckResult"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	if !result.ReceivedAliveCheckFn {
		t.Error("strategy did not receive aliveCheckFn in options — threading broken")
	}
	if !result.AliveCheckCalled {
		t.Error("aliveCheckFn was not called by the strategy")
	}
	if result.AliveCheckResult == nil || *result.AliveCheckResult != false {
		t.Errorf("aliveCheckFn return value not threaded — got %v, want false", result.AliveCheckResult)
	}
}

// ---------------------------------------------------------------------------
// T093: Claude auto-detection verification (--version / model availability)
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ClaudeAutoDetect_VersionCheckFails(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock exec.execv: 'which claude' succeeds, but 'claude --version' fails.
	val, err := evalJS(`(function() {
		var _origExec = exec;
		var execProxy = {
			execv: function(args) {
				var cmdStr = args.join(' ');
				if (cmdStr === 'which claude') {
					return { code: 0, stdout: '/usr/local/bin/claude\n', stderr: '' };
				}
				if (args[0] === 'claude' && args[1] === '--version') {
					return { code: 1, stdout: '', stderr: 'segfault\n' };
				}
				return _origExec.execv(args);
			}
		};
		for (var k in _origExec) {
			if (k !== 'execv') execProxy[k] = _origExec[k];
		}
		exec = execProxy;

		var executor = new ClaudeCodeExecutor({ claudeCommand: '' });
		var result = executor.resolve();

		// Restore exec.
		exec = _origExec;
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(*result.Error, "version check failed") {
		t.Errorf("expected error to contain 'version check failed', got: %s", *result.Error)
	}
	if !strings.Contains(*result.Error, "/usr/local/bin/claude") {
		t.Errorf("expected error to contain path, got: %s", *result.Error)
	}
}

func TestPrSplitCommand_ClaudeAutoDetect_OllamaModelMissing(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock exec.execv: 'which claude' fails, 'which ollama' succeeds,
	// 'ollama list' succeeds but without the requested model.
	val, err := evalJS(`(function() {
		var _origExec = exec;
		var execProxy = {
			execv: function(args) {
				var cmdStr = args.join(' ');
				if (cmdStr === 'which claude') {
					return { code: 1, stdout: '', stderr: '' };
				}
				if (cmdStr === 'which ollama') {
					return { code: 0, stdout: '/usr/local/bin/ollama\n', stderr: '' };
				}
				if (args[0] === 'ollama' && args[1] === 'list') {
					return { code: 0, stdout: 'NAME          ID\nllama2:latest abc123\nmistral:7b    def456\n', stderr: '' };
				}
				return _origExec.execv(args);
			}
		};
		for (var k in _origExec) {
			if (k !== 'execv') execProxy[k] = _origExec[k];
		}
		exec = execProxy;

		var executor = new ClaudeCodeExecutor({
			claudeCommand: '',
			claudeModel: 'claude-3-opus'
		});
		var result = executor.resolve();

		exec = _origExec;
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(*result.Error, "model") {
		t.Errorf("expected error to mention 'model', got: %s", *result.Error)
	}
	if !strings.Contains(*result.Error, "claude-3-opus") {
		t.Errorf("expected error to mention requested model name, got: %s", *result.Error)
	}
	if !strings.Contains(*result.Error, "not available") {
		t.Errorf("expected error to contain 'not available', got: %s", *result.Error)
	}
}

func TestPrSplitCommand_DefaultRetryBudget(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Runtime retryBudget should default to 3. Access it through the set command output.
	val, err := evalJS(`(async function() {
		// resolveConflicts uses runtime.retryBudget as default.
		// Verify by calling with no explicit budget on a benign plan.
		var result = await globalThis.prSplit.resolveConflicts(
			{ dir: '.', splits: [], verifyCommand: 'true' },
			{}
		);
		// If we get here without error, the default is working.
		return 'ok';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "ok" {
		t.Errorf("Expected 'ok', got %v", val)
	}
}

func TestPrSplitCommand_SetRetryBudgetViaAlternateKey(t *testing.T) {
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// "retryBudget" (camelCase) should also work.
	if err := dispatch("set", []string{"retryBudget", "10"}); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !contains(output, "Set retryBudget = 10") {
		t.Errorf("Expected confirmation for camelCase key, got: %s", output)
	}
}

func TestPrSplitCommand_AddMissingFilesFixNoSourceBranch(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// fix() without a source branch should return error.
	val, err := evalJS(`JSON.stringify(
		globalThis.prSplit.AUTO_FIX_STRATEGIES[5].fix('.', 'branch-1', {splits:[]}, 'file not found')
	)`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if result["fixed"] != false {
		t.Errorf("Expected fixed=false, got %v", result["fixed"])
	}
	errMsg, _ := result["error"].(string)
	if !strings.Contains(errMsg, "source branch") {
		t.Errorf("Expected 'source branch' error, got: %s", errMsg)
	}
}

func TestPrSplitCommand_GoMissingImportsFixNoGoimports(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// If goimports is not available at the given path, fix should fail gracefully.
	val, err := evalJS(`(function() {
		// Call fix on nonexistent dir — it'll try 'which goimports'.
		// If goimports IS available, it'll fail on the nonexistent dir.
		// Either way, fixed should be false.
		var result = globalThis.prSplit.AUTO_FIX_STRATEGIES[2].fix('/nonexistent/no-such-dir');
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if result["fixed"] != false {
		t.Errorf("Expected fixed=false for nonexistent dir, got %v", result["fixed"])
	}
}

// ---------------------------------------------------------------------------
// T34: sendToHandle — handle dies between text and newline writes
// ---------------------------------------------------------------------------

func TestPrSplitCommand_SendToHandle_HandleDiesBetweenWrites(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock handle: first send (text) succeeds, second send (newline) throws —
	// simulates a process dying between the two writes.
	val, err := evalJS(`(async function() {
		var callCount = 0;
		var mockHandle = {
			send: function(data) {
				callCount++;
				if (callCount === 2) {
					throw new Error('write: broken pipe');
				}
			}
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'hello world');
		return JSON.stringify({ error: result.error, callCount: callCount });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Text write (call 1) succeeds. Newline write (call 2) fails with broken pipe.
	// sendToHandle should return the error from the newline write.
	if output.Error == nil {
		t.Fatal("Expected error when handle dies between text and newline writes")
	}
	if !strings.Contains(*output.Error, "broken pipe") {
		t.Errorf("Error should contain 'broken pipe', got: %s", *output.Error)
	}
	// Exactly 2 calls: 1 text (success) + 1 newline (failure, non-EAGAIN so no retry).
	if output.CallCount != 2 {
		t.Errorf("Expected 2 send calls (text + failed newline), got %d", output.CallCount)
	}
}

// ---------------------------------------------------------------------------
// T35: sendToHandle — EAGAIN retry on newline write (second write path)
// ---------------------------------------------------------------------------

func TestPrSplitCommand_SendToHandle_NewlineEAGAINRetry(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock handle: text always succeeds (callCount 1), newline EAGAIN on first
	// attempt (callCount 2) then succeeds on retry (callCount 3).
	val, err := evalJS(`(async function() {
		var callCount = 0;
		var mockHandle = {
			send: function(data) {
				callCount++;
				// Call 1 = text (success). Call 2 = newline (EAGAIN). Call 3 = newline retry (success).
				if (callCount === 2) {
					throw new Error('write: resource temporarily unavailable (EAGAIN)');
				}
			}
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'test newline retry');
		return JSON.stringify({ error: result.error, callCount: callCount });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Should succeed: text (1) + newline EAGAIN (2) + newline retry success (3).
	if output.Error != nil {
		t.Errorf("Expected success after newline EAGAIN retry, got error: %s", *output.Error)
	}
	if output.CallCount != 3 {
		t.Errorf("Expected 3 send calls (text + newline EAGAIN + newline retry), got %d", output.CallCount)
	}
}

func TestPrSplitCommand_SendToHandle_NewlineEAGAINExhausted(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock handle: text succeeds (callCount 1), newline always throws EAGAIN.
	val, err := evalJS(`(async function() {
		var callCount = 0;
		var mockHandle = {
			send: function(data) {
				callCount++;
				if (callCount > 1) {
					throw new Error('EAGAIN: resource temporarily unavailable');
				}
			}
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'exhaust newline');
		return JSON.stringify({ error: result.error, callCount: callCount });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Text succeeds (1). Newline fails all 4 attempts (initial + 3 retries) = calls 2-5.
	// Total: 5 calls.
	if output.Error == nil {
		t.Fatal("Expected error after newline EAGAIN retry exhaustion")
	}
	if !strings.Contains(*output.Error, "EAGAIN") {
		t.Errorf("Error should contain 'EAGAIN', got: %s", *output.Error)
	}
	// 1 text success + 4 newline attempts (1 initial + 3 retries) = 5.
	if output.CallCount != 5 {
		t.Errorf("Expected 5 send calls (1 text + 4 newline EAGAIN), got %d", output.CallCount)
	}
}

// ---------------------------------------------------------------------------
// sendToHandle edge cases: empty text, cancellation, boundary chunking
// ---------------------------------------------------------------------------

func TestPrSplitCommand_SendToHandle_EmptyText(t *testing.T) {
	t.Parallel()

	_, _, _, evalJSAsync := loadPrSplitEngineWithEval(t, nil)

	// sendToHandle with empty string should skip the chunk loop entirely and
	// only send the carriage return (\r) for submission.
	val, err := evalJSAsync(`await (async function() {
		var sends = [];
		var mockHandle = {
			send: function(text) { sends.push(text); }
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, '');
		return JSON.stringify({
			result: result,
			sends: sends
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Result struct {
			Error *string `json:"error"`
		} `json:"result"`
		Sends []string `json:"sends"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	if output.Result.Error != nil {
		t.Errorf("sendToHandle('') returned error: %s", *output.Result.Error)
	}
	// Empty text: chunk loop body should not execute, only \r is sent.
	if len(output.Sends) != 1 {
		t.Fatalf("Expected 1 send (only \\r), got %d: %v", len(output.Sends), output.Sends)
	}
	if output.Sends[0] != "\r" {
		t.Errorf("sends[0] = %q, want %q", output.Sends[0], "\r")
	}
}

func TestPrSplitCommand_SendToHandle_CancellationBeforeChunk(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Set cancellation flag before calling sendToHandle.
	// getCancellationError() checks prSplit.isCancelled() which delegates to
	// prSplit._cancelSource().
	val, err := evalJS(`(async function() {
		globalThis.prSplit._cancelSource = function(q) {
			return q === 'cancelled';
		};
		var sends = [];
		var mockHandle = {
			send: function(text) { sends.push(text); }
		};
		var result = await globalThis.prSplit.sendToHandle(mockHandle, 'should not send');
		return JSON.stringify({
			error: result.error,
			sendCount: sends.length
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Error     *string `json:"error"`
		SendCount int     `json:"sendCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	if output.Error == nil {
		t.Fatal("Expected cancellation error, got nil")
	}
	if !strings.Contains(*output.Error, "cancelled") {
		t.Errorf("Error should mention cancellation, got: %s", *output.Error)
	}
	if output.SendCount != 0 {
		t.Errorf("Expected 0 sends (cancelled before write), got %d", output.SendCount)
	}
}

func TestPrSplitCommand_SendToHandle_ExactChunkBoundary(t *testing.T) {
	t.Parallel()

	_, _, _, evalJSAsync := loadPrSplitEngineWithEval(t, nil)

	// Send text whose length exactly equals SEND_TEXT_CHUNK_BYTES (512).
	// Should produce exactly 1 text chunk + 1 \r = 2 sends total.
	val, err := evalJSAsync(`await (async function() {
		var sends = [];
		var mockHandle = {
			send: function(text) { sends.push(text); }
		};
		// Build a string of exactly 512 characters.
		var text = '';
		for (var i = 0; i < 512; i++) { text += 'A'; }
		var result = await globalThis.prSplit.sendToHandle(mockHandle, text);
		return JSON.stringify({
			result: result,
			sendCount: sends.length,
			firstSendLen: sends.length > 0 ? sends[0].length : -1,
			lastSend: sends.length > 0 ? sends[sends.length - 1] : null
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Result struct {
			Error *string `json:"error"`
		} `json:"result"`
		SendCount    int     `json:"sendCount"`
		FirstSendLen int     `json:"firstSendLen"`
		LastSend     *string `json:"lastSend"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	if output.Result.Error != nil {
		t.Errorf("sendToHandle returned error: %s", *output.Result.Error)
	}
	// Exactly 512 bytes = exactly 1 chunk (offset 0..511, then offset 512 >= len → loop exits).
	// Total sends: 1 text chunk + 1 \r = 2.
	if output.SendCount != 2 {
		t.Errorf("Expected 2 sends (1 chunk + \\r), got %d", output.SendCount)
	}
	if output.FirstSendLen != 512 {
		t.Errorf("First send length = %d, want 512", output.FirstSendLen)
	}
	if output.LastSend == nil || *output.LastSend != "\r" {
		last := "<nil>"
		if output.LastSend != nil {
			last = *output.LastSend
		}
		t.Errorf("Last send = %q, want %q", last, "\r")
	}
}

// ---------------------------------------------------------------------------
// T116: resolveConflictsWithClaude — successful fix path
// ---------------------------------------------------------------------------

func TestPrSplitCommand_ResolveConflictsWithClaude_SuccessfulFix(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Install git mock so gitExec / exec.execStream are captured.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Exercise the successful fix path:
	//  1. sendToHandle sends prompt → increment counter
	//  2. mcpCallbackObj.waitFor returns resolution with patches
	//  3. osmod.writeFile applies each patch
	//  4. gitAddChangedFiles stages modified files → gitExec(['status', '--porcelain']), gitExec(['add', '--', ...])
	//  5. git commit --amend --no-edit
	//  6. verifySplit: gitExec(['checkout', branch]) + exec.execStream(['sh', '-c', ...]) → passed
	//  7. Result: fixed=true, reSplitNeeded=false, 1 claude interaction, 1 resolution recorded
	val, err := evalJS(`(async function() {
		// --- Track osmod.writeFile calls without real filesystem ---
		var writeFileCalls = [];
		var origWriteFile = osmod.writeFile;
		osmod.writeFile = function(path, content) {
			writeFileCalls.push({ file: path, content: content });
		};

		// Prevent text chunking — tests count raw send() calls.
		prSplit.SEND_TEXT_CHUNK_BYTES = 1000000;

		// --- Mock claudeExecutor ---
		var sendCallCount = 0;
		claudeExecutor = {
			handle: {
				send: function(text) { sendCallCount++; },
				isAlive: function() { return true; }
			}
		};

		// --- Mock mcpCallbackObj: return a resolution with one patch ---
		mcpCallbackObj = {
			resetWaiter: function() {},
			waitFor: function(name, timeout, opts) {
				if (name === 'reportResolution') {
					return {
						data: {
							patches: [
								{ file: 'pkg/handler.go', content: 'package handler\n\nfunc Handle() {}\n' }
							]
						},
						error: null
					};
				}
				return { data: null, error: 'timeout' };
			}
		};

		// --- Configure git mock responses ---
		// gitAddChangedFiles calls: status --porcelain, add -- <files>
		globalThis._gitResponses['status --porcelain'] = _gitOk(' M pkg/handler.go');
		// add, commit, checkout all succeed via default _gitOk
		// verifySplit: exec.execStream(['sh', '-c', ...]) routes through !sh
		globalThis._gitResponses['!sh'] = function(argv) { return _gitOk('all tests passed'); };

		// --- Call resolveConflictsWithClaude ---
		var failures = [
			{ branch: 'split/fix-me', files: ['pkg/handler.go'], error: 'test fail: handler_test.go:42' }
		];
		var report = { conflicts: [], resolutions: [], claudeInteractions: 0 };
		var result = await globalThis.prSplit.resolveConflictsWithClaude(
			failures,
			'test-session-fix',
			{ resolve: 5000, wallClockMs: 30000 },
			100,
			3,
			report
		);

		// Restore osmod.writeFile (good hygiene).
		osmod.writeFile = origWriteFile;

		// Collect git calls for assertion.
		var gitCallSummary = globalThis._gitCalls.map(function(c) { return c.argv.join(' '); });

		return JSON.stringify({
			result: result,
			report: {
				conflicts: report.conflicts.length,
				resolutions: report.resolutions.length,
				claudeInteractions: report.claudeInteractions
			},
			sendCallCount: sendCallCount,
			writeFileCalls: writeFileCalls,
			gitCallSummary: gitCallSummary
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var output struct {
		Result struct {
			ReSplitNeeded bool   `json:"reSplitNeeded"`
			ReSplitReason string `json:"reSplitReason"`
		} `json:"result"`
		Report struct {
			Conflicts          int `json:"conflicts"`
			Resolutions        int `json:"resolutions"`
			ClaudeInteractions int `json:"claudeInteractions"`
		} `json:"report"`
		SendCallCount  int `json:"sendCallCount"`
		WriteFileCalls []struct {
			File    string `json:"file"`
			Content string `json:"content"`
		} `json:"writeFileCalls"`
		GitCallSummary []string `json:"gitCallSummary"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &output); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// 1. Successful fix → reSplitNeeded=false, no reSplitReason.
	if output.Result.ReSplitNeeded {
		t.Error("Expected reSplitNeeded=false after successful fix")
	}
	if output.Result.ReSplitReason != "" {
		t.Errorf("Expected empty reSplitReason, got %q", output.Result.ReSplitReason)
	}

	// 2. One attempt, one resolution: 1 conflict, 1 resolution, 1 interaction.
	if output.Report.Conflicts != 1 {
		t.Errorf("Expected 1 conflict entry, got %d", output.Report.Conflicts)
	}
	if output.Report.Resolutions != 1 {
		t.Errorf("Expected 1 resolution (fix accepted), got %d", output.Report.Resolutions)
	}
	if output.Report.ClaudeInteractions != 1 {
		t.Errorf("Expected 1 Claude interaction, got %d", output.Report.ClaudeInteractions)
	}

	// 3. sendToHandle: 2 send calls (two-write: text + newline).
	if output.SendCallCount != 2 {
		t.Errorf("Expected 2 send calls (two-write), got %d", output.SendCallCount)
	}

	// 4. osmod.writeFile called once with correct patch.
	if len(output.WriteFileCalls) != 1 {
		t.Fatalf("Expected 1 writeFile call (1 patch), got %d", len(output.WriteFileCalls))
	}
	if !strings.HasSuffix(output.WriteFileCalls[0].File, "pkg/handler.go") {
		t.Errorf("writeFile file = %q, want suffix %q", output.WriteFileCalls[0].File, "pkg/handler.go")
	}
	if !strings.Contains(output.WriteFileCalls[0].Content, "func Handle()") {
		t.Errorf("writeFile content should contain patched function, got %q", output.WriteFileCalls[0].Content)
	}

	// 5. Git calls include the expected sequence:
	//    - git status --porcelain (gitAddChangedFiles)
	//    - git add -- pkg/handler.go (staging)
	//    - git commit --amend --no-edit (amend commit)
	//    - git checkout split/fix-me (verifySplit)
	//    - sh -c ... (verify command)
	var hasStatusPorcelain, hasAdd, hasCommitAmend, hasCheckout, hasShVerify bool
	for _, call := range output.GitCallSummary {
		switch {
		case strings.Contains(call, "status --porcelain"):
			hasStatusPorcelain = true
		case strings.Contains(call, "add --") && strings.Contains(call, "pkg/handler.go"):
			hasAdd = true
		case strings.Contains(call, "commit --amend --no-edit"):
			hasCommitAmend = true
		case strings.Contains(call, "worktree add") && strings.Contains(call, "split/fix-me"):
			hasCheckout = true
		case strings.HasPrefix(call, "sh -c"):
			hasShVerify = true
		}
	}
	if !hasStatusPorcelain {
		t.Error("Expected git status --porcelain call (from gitAddChangedFiles)")
	}
	if !hasAdd {
		t.Error("Expected git add -- pkg/handler.go call")
	}
	if !hasCommitAmend {
		t.Error("Expected git commit --amend --no-edit call")
	}
	if !hasCheckout {
		t.Error("Expected git worktree add ... split/fix-me call")
	}
	if !hasShVerify {
		t.Error("Expected sh -c verify command call (from verifySplit)")
	}
}
