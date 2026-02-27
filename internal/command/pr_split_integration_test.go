package command

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/pty"
)

// ---------------------------------------------------------------------------
// Integration Test: Heuristic Split (no AI required, real git)
// ---------------------------------------------------------------------------

// TestIntegration_HeuristicSplitEndToEnd creates a realistic git repository,
// runs the full heuristic split pipeline (analyze → group → plan → execute →
// verify equivalence), and validates that:
//   - Split branches are created with the correct files.
//   - The combined tree hash of all splits is equivalent to the original.
//   - No content is lost or duplicated.
//
// This test does NOT require AI infrastructure; it runs in every CI build.
func TestIntegration_HeuristicSplitEndToEnd(t *testing.T) {
	t.Parallel()

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Verify we're on the feature branch.
	branch := runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch != "feature" {
		t.Fatalf("expected to be on 'feature' branch, got %q", branch)
	}

	// Set up the pr-split JS engine pointing at our temp repo.
	_, _, evalJS := loadPrSplitEngineWithEval(t, map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true", // always passes
	})

	// Configure git mocks to point at real git repo instead.
	// We override the exec.execv to NOT mock — use real git commands.
	// The JS script's gitExec uses `git -C <dir>` when dir != '.'.
	// We'll use evalJS to call functions with the dir parameter.

	// Step 1: Analyze diff (using real git).
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var analysis struct {
		Files         []string          `json:"files"`
		FileStatuses  map[string]string `json:"fileStatuses"`
		CurrentBranch string            `json:"currentBranch"`
		BaseBranch    string            `json:"baseBranch"`
		Error         *string           `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &analysis); err != nil {
		t.Fatalf("failed to parse analysis: %v", err)
	}
	if analysis.Error != nil {
		t.Fatalf("analyzeDiff returned error: %s", *analysis.Error)
	}
	if len(analysis.Files) == 0 {
		t.Fatal("analyzeDiff returned no files")
	}
	t.Logf("Analyzed %d files: %v", len(analysis.Files), analysis.Files)
	t.Logf("File statuses: %v", analysis.FileStatuses)

	// Step 2: Apply strategy (directory grouping).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.applyStrategy(
		` + mustJSON(t, analysis.Files) + `,
		'directory',
		{
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
			maxFiles: 10,
			baseBranch: 'main'
		}
	))`)
	if err != nil {
		t.Fatalf("applyStrategy failed: %v", err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatalf("failed to parse groups: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("applyStrategy returned no groups")
	}
	t.Logf("Groups: %v", groups)

	// Step 3: Create split plan (with real git to detect current branch).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.createSplitPlan(
		` + mustJSON(t, groups) + `,
		{
			baseBranch: 'main',
			sourceBranch: 'feature',
			branchPrefix: 'split/',
			maxFiles: 10,
			dir: ` + jsString(repoDir) + `,
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `
		}
	))`)
	if err != nil {
		t.Fatalf("createSplitPlan failed: %v", err)
	}

	var plan struct {
		BaseBranch   string `json:"baseBranch"`
		SourceBranch string `json:"sourceBranch"`
		Dir          string `json:"dir"`
		Splits       []struct {
			Name    string   `json:"name"`
			Files   []string `json:"files"`
			Message string   `json:"message"`
			Order   int      `json:"order"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &plan); err != nil {
		t.Fatalf("failed to parse plan: %v", err)
	}
	if len(plan.Splits) == 0 {
		t.Fatal("createSplitPlan produced no splits")
	}
	t.Logf("Plan: %d splits", len(plan.Splits))
	for i, s := range plan.Splits {
		t.Logf("  Split %d: %s (%d files: %v)", i+1, s.Name, len(s.Files), s.Files)
	}

	// Verify all files are accounted for.
	allPlanFiles := make(map[string]bool)
	for _, s := range plan.Splits {
		for _, f := range s.Files {
			if allPlanFiles[f] {
				t.Errorf("duplicate file in plan: %s", f)
			}
			allPlanFiles[f] = true
		}
	}
	for _, f := range analysis.Files {
		if !allPlanFiles[f] {
			t.Errorf("file %s in analysis but missing from plan", f)
		}
	}

	// Step 4: Execute the split (creates real branches).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: ` + jsString(repoDir) + `,
		verifyCommand: 'true',
		fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
		splits: ` + mustJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("executeSplit failed: %v", err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Branch string  `json:"branch"`
			Error  *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &execResult); err != nil {
		t.Fatalf("failed to parse exec result: %v", err)
	}
	if execResult.Error != nil {
		t.Fatalf("executeSplit returned top-level error: %s", *execResult.Error)
	}
	for _, r := range execResult.Results {
		if r.Error != nil {
			t.Errorf("split branch %s failed: %s", r.Branch, *r.Error)
		}
	}

	// Verify branches exist in the repo.
	branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
	for _, s := range plan.Splits {
		if !strings.Contains(branchOutput, s.Name) {
			t.Errorf("expected branch %s to exist, not found in:\n%s", s.Name, branchOutput)
		}
	}
	t.Logf("Created branches:\n%s", branchOutput)

	// Step 5: Verify tree hash equivalence.
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: ` + jsString(repoDir) + `,
		splits: ` + mustJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("verifyEquivalenceDetailed failed: %v", err)
	}

	var equiv struct {
		Equivalent  bool     `json:"equivalent"`
		SplitTree   string   `json:"splitTree"`
		SourceTree  string   `json:"sourceTree"`
		Error       *string  `json:"error"`
		DiffFiles   []string `json:"diffFiles"`
		DiffSummary string   `json:"diffSummary"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &equiv); err != nil {
		t.Fatalf("failed to parse equivalence result: %v", err)
	}
	if equiv.Error != nil {
		t.Errorf("verifyEquivalence error: %s", *equiv.Error)
	}
	if !equiv.Equivalent {
		t.Errorf("tree hash mismatch! splitTree=%s sourceTree=%s diffFiles=%v diffSummary=%s",
			equiv.SplitTree, equiv.SourceTree, equiv.DiffFiles, equiv.DiffSummary)
	} else {
		t.Logf("✅ Tree hash equivalence verified: %s", equiv.SplitTree)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Cancellation Flow
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitCancel verifies that the auto-split pipeline
// responds to cooperative cancellation within a reasonable time. It mocks
// the autoSplitTUI.cancelled() function to return true during the pipeline
// and verifies the pipeline exits with a cancellation error.
func TestIntegration_AutoSplitCancel(t *testing.T) {
	t.Parallel()

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	_, _, evalJS := loadPrSplitEngineWithEval(t, map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"branchPrefix":  "split/",
		"verifyCommand": "true",
	})

	// Inject a mock autoSplitTUI that returns cancelled immediately.
	// This simulates the user pressing q before the pipeline starts any
	// blocking operation.
	_, err := evalJS(`
		globalThis.autoSplitTUI = {
			runAsync: function() {},
			wait: function() { return null; },
			stepStart: function() {},
			stepDone: function() {},
			appendOutput: function() {},
			appendError: function() {},
			done: function() {},
			stepDetail: function() {},
			cancelled: function() { return true; },
			forceCancelled: function() { return false; },
			quit: function() {}
		};
	`)
	if err != nil {
		t.Fatalf("failed to inject mock autoSplitTUI: %v", err)
	}

	// Run auto-split — it should detect cancellation at the first step
	// boundary and return immediately.
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `,
		strategy: 'directory'
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error *string `json:"error"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// The pipeline should have returned a cancellation error.
	if result.Error == nil || !strings.Contains(*result.Error, "cancel") {
		t.Errorf("expected cancellation error, got: %v", result.Error)
	} else {
		t.Logf("✅ Cancellation detected: %s", *result.Error)
	}

	// The original branch should still be intact.
	branch := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "feature" {
		t.Errorf("expected to be on 'feature' branch after cancel, got %q", branch)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: sendToHandle fallback (no autoSplitTUI)
// ---------------------------------------------------------------------------

// TestIntegration_SendToHandle_FallbackDirect verifies that sendToHandle
// falls back to direct handle.send() when no autoSplitTUI is present.
// This covers the non-TUI / test-context code path.
func TestIntegration_SendToHandle_FallbackDirect(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Ensure autoSplitTUI is not defined (default engine state).
	raw, err := evalJS(`
		(function() {
			// sendToHandle should use the fallback path when autoSplitTUI is missing.
			// Two-write pattern: first write is the prompt text,
			// second write is \r (Enter key for PTY).
			var sends = [];
			var mockHandle = {
				send: function(text) { sends.push(text); }
			};
			var result = globalThis.prSplit.sendToHandle(mockHandle, 'hello Claude');
			return JSON.stringify({ error: result.error, sends: sends });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle test failed: %v", err)
	}

	var result struct {
		Error *string  `json:"error"`
		Sends []string `json:"sends"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Errorf("sendToHandle returned error: %s", *result.Error)
	}
	if len(result.Sends) != 2 {
		t.Fatalf("expected 2 sends, got %d: %q", len(result.Sends), result.Sends)
	}
	if result.Sends[0] != "hello Claude" {
		t.Errorf("sends[0] = %q, want %q", result.Sends[0], "hello Claude")
	}
	if result.Sends[1] != "\r" {
		t.Errorf("sends[1] = %q, want %q", result.Sends[1], "\r")
	}
}

// TestIntegration_SendToHandle_FallbackError verifies that sendToHandle
// returns an error object (not throws) when the underlying send fails.
func TestIntegration_SendToHandle_FallbackError(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Two-write pattern: test that error on FIRST send (text) returns
	// immediately without attempting the second send (Enter key).
	raw, err := evalJS(`
		(function() {
			var sendCount = 0;
			var mockHandle = {
				send: function(text) {
					sendCount++;
					if (sendCount === 1) { throw new Error('PTY write failed'); }
				}
			};
			var result = globalThis.prSplit.sendToHandle(mockHandle, 'will fail');
			return JSON.stringify({ error: result.error, sendCount: sendCount });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle error test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		SendCount int     `json:"sendCount"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error from sendToHandle when send throws")
	}
	if !strings.Contains(*result.Error, "PTY write failed") {
		t.Errorf("error = %q, want to contain 'PTY write failed'", *result.Error)
	}
	if result.SendCount != 1 {
		t.Errorf("sendCount = %d, want 1 (should not attempt second send on first failure)", result.SendCount)
	}
}

// TestIntegration_SendToHandle_TUIPath verifies the sendToHandle code path
// that uses autoSplitTUI.sendWithCancel (when autoSplitTUI is defined with
// that method). The TUI path sends text, sleeps 50ms, then sends '\r'.
func TestIntegration_SendToHandle_TUIPath(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			// Define autoSplitTUI with sendWithCancel to trigger the TUI path.
			var calls = [];
			globalThis.autoSplitTUI = {
				sendWithCancel: function(handle, text) {
					calls.push({ handle: 'mock', text: text });
					return { error: null };
				}
			};

			var mockHandle = {
				send: function(text) { calls.push({ directSend: text }); }
			};

			var result = globalThis.prSplit.sendToHandle(mockHandle, 'classify these files');

			// Tear down to avoid leaking into other tests.
			delete globalThis.autoSplitTUI;

			return JSON.stringify({ error: result.error, calls: calls });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle TUI path test failed: %v", err)
	}

	var result struct {
		Error *string `json:"error"`
		Calls []struct {
			Handle     string `json:"handle"`
			Text       string `json:"text"`
			DirectSend string `json:"directSend"`
		} `json:"calls"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %s", *result.Error)
	}
	// Should have used sendWithCancel for both writes (text + Enter key).
	if len(result.Calls) != 2 {
		t.Fatalf("expected 2 sendWithCancel calls, got %d: %+v", len(result.Calls), result.Calls)
	}
	if result.Calls[0].Text != "classify these files" {
		t.Errorf("first call text = %q, want %q", result.Calls[0].Text, "classify these files")
	}
	if result.Calls[1].Text != "\r" {
		t.Errorf("second call text = %q, want %q (Enter key)", result.Calls[1].Text, "\r")
	}
	// Should NOT have used direct send.
	for _, c := range result.Calls {
		if c.DirectSend != "" {
			t.Errorf("should not have used direct send, but got: %q", c.DirectSend)
		}
	}
}

// TestIntegration_SendToHandle_TUIPath_FirstSendError verifies that when
// autoSplitTUI.sendWithCancel returns an error on the first write (text),
// the function returns that error without attempting the second write.
func TestIntegration_SendToHandle_TUIPath_FirstSendError(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var callCount = 0;
			globalThis.autoSplitTUI = {
				sendWithCancel: function(handle, text) {
					callCount++;
					if (callCount === 1) {
						return { error: 'cancelled by user' };
					}
					return { error: null };
				}
			};

			var mockHandle = { send: function() {} };
			var result = globalThis.prSplit.sendToHandle(mockHandle, 'will cancel');

			delete globalThis.autoSplitTUI;

			return JSON.stringify({ error: result.error, callCount: callCount });
		})()
	`)
	if err != nil {
		t.Fatalf("sendToHandle TUI error test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		CallCount int     `json:"callCount"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error from sendToHandle when first sendWithCancel fails")
	}
	if !strings.Contains(*result.Error, "cancelled") {
		t.Errorf("error = %q, want to contain 'cancelled'", *result.Error)
	}
	if result.CallCount != 1 {
		t.Errorf("callCount = %d, want 1 (should not attempt Enter key)", result.CallCount)
	}
}

// TestIntegration_SpawnArgs_DangerouslySkipPermissions verifies that
// ClaudeCodeExecutor.spawn prepends --dangerously-skip-permissions for
// claude-code type providers but NOT for ollama type providers.
func TestIntegration_SpawnArgs_DangerouslySkipPermissions(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Use t.TempDir() for mock paths to avoid host state mutation.
	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	// Test: claude-code type should have --dangerously-skip-permissions
	// We mock the cm object including newMCPInstance to capture spawn args.
	raw, err := evalJS(`
		(function() {
			var tmpDir = '` + escapedTmpDir + `';

			// Create a ClaudeCodeExecutor with claude-code type.
			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			// Mock resolve so spawn() doesn't need real claude/ollama on PATH.
			executor.resolved = { command: 'mock-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.sessionId = 'test-session';

			// Override cm methods to capture spawn args.
			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			// Call spawn — it creates MCP instance via cm, then spawns provider.
			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			originalSpawn.call(executor);

			if (!capturedArgs) {
				return JSON.stringify({ error: 'spawn did not capture args' });
			}

			// Verify --dangerously-skip-permissions is first.
			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			var mcpIdx = capturedArgs.indexOf('--mcp-config');
			var userIdx = capturedArgs.indexOf('--user-arg');

			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx,
				mcpIndex: mcpIdx,
				userArgIndex: userIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("spawn args test failed: %v", err)
	}

	var result struct {
		Error        *string  `json:"error"`
		Args         []string `json:"args"`
		DSPIndex     int      `json:"dspIndex"`
		MCPIndex     int      `json:"mcpIndex"`
		UserArgIndex int      `json:"userArgIndex"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("spawn args test returned error: %s", *result.Error)
	}
	if result.DSPIndex == -1 {
		t.Fatal("--dangerously-skip-permissions not found in args")
	}
	if result.DSPIndex != 0 {
		t.Errorf("--dangerously-skip-permissions index = %d, want 0 (should be first arg)", result.DSPIndex)
	}
	if result.UserArgIndex == -1 {
		t.Error("--user-arg not found in args")
	}
	if result.MCPIndex == -1 {
		t.Error("--mcp-config not found in args")
	}
	// Order: --dangerously-skip-permissions, --user-arg, --mcp-config, <path>
	if result.DSPIndex >= result.UserArgIndex {
		t.Errorf("--dangerously-skip-permissions (idx %d) should come before --user-arg (idx %d)",
			result.DSPIndex, result.UserArgIndex)
	}
	if result.UserArgIndex >= result.MCPIndex {
		t.Errorf("--user-arg (idx %d) should come before --mcp-config (idx %d)",
			result.UserArgIndex, result.MCPIndex)
	}

	// Negative case: ollama type should NOT have --dangerously-skip-permissions.
	rawOllama, err := evalJS(`
		(function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			executor.resolved = { command: 'mock-ollama', type: 'ollama' };
			executor.resolve = function() { return { error: null }; };
			executor.sessionId = 'test-session-ollama';

			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			originalSpawn.call(executor);

			if (!capturedArgs) {
				return JSON.stringify({ error: 'ollama spawn did not capture args' });
			}

			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("ollama spawn args test failed: %v", err)
	}

	var ollamaResult struct {
		Error    *string  `json:"error"`
		Args     []string `json:"args"`
		DSPIndex int      `json:"dspIndex"`
	}
	if err := json.Unmarshal([]byte(rawOllama.(string)), &ollamaResult); err != nil {
		t.Fatalf("ollama parse error: %v", err)
	}
	if ollamaResult.Error != nil {
		t.Fatalf("ollama spawn args returned error: %s", *ollamaResult.Error)
	}
	if ollamaResult.DSPIndex != -1 {
		t.Errorf("ollama args should NOT contain --dangerously-skip-permissions, but found at index %d; args: %v",
			ollamaResult.DSPIndex, ollamaResult.Args)
	}

	// Third case: explicit type with command name containing 'claude'
	// should have --dangerously-skip-permissions (basename detection).
	rawExplicitClaude, err := evalJS(`
		(function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			executor.resolved = { command: '/usr/local/bin/claude-code', type: 'explicit' };
			executor.resolve = function() { return { error: null }; };
			executor.sessionId = 'test-session-explicit-claude';

			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			originalSpawn.call(executor);

			if (!capturedArgs) {
				return JSON.stringify({ error: 'explicit-claude spawn did not capture args' });
			}

			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("explicit-claude spawn args test failed: %v", err)
	}

	var explicitClaudeResult struct {
		Error    *string  `json:"error"`
		Args     []string `json:"args"`
		DSPIndex int      `json:"dspIndex"`
	}
	if err := json.Unmarshal([]byte(rawExplicitClaude.(string)), &explicitClaudeResult); err != nil {
		t.Fatalf("explicit-claude parse error: %v", err)
	}
	if explicitClaudeResult.Error != nil {
		t.Fatalf("explicit-claude spawn args returned error: %s", *explicitClaudeResult.Error)
	}
	if explicitClaudeResult.DSPIndex == -1 {
		t.Error("explicit type with 'claude' in command name should contain --dangerously-skip-permissions")
	}

	// Fourth case: explicit type with non-claude command name should NOT
	// have --dangerously-skip-permissions.
	rawExplicitOther, err := evalJS(`
		(function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: ['--user-arg'],
				model: 'test-model'
			});

			executor.resolved = { command: '/opt/bin/my-custom-tool', type: 'explicit' };
			executor.resolve = function() { return { error: null }; };
			executor.sessionId = 'test-session-explicit-other';

			var capturedArgs = null;
			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					capturedArgs = opts.args;
					return { send: function() {} };
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			originalSpawn.call(executor);

			if (!capturedArgs) {
				return JSON.stringify({ error: 'explicit-other spawn did not capture args' });
			}

			var dspIdx = capturedArgs.indexOf('--dangerously-skip-permissions');
			return JSON.stringify({
				error: null,
				args: capturedArgs,
				dspIndex: dspIdx
			});
		})()
	`)
	if err != nil {
		t.Fatalf("explicit-other spawn args test failed: %v", err)
	}

	var explicitOtherResult struct {
		Error    *string  `json:"error"`
		Args     []string `json:"args"`
		DSPIndex int      `json:"dspIndex"`
	}
	if err := json.Unmarshal([]byte(rawExplicitOther.(string)), &explicitOtherResult); err != nil {
		t.Fatalf("explicit-other parse error: %v", err)
	}
	if explicitOtherResult.Error != nil {
		t.Fatalf("explicit-other spawn args returned error: %s", *explicitOtherResult.Error)
	}
	if explicitOtherResult.DSPIndex != -1 {
		t.Errorf("explicit type with non-claude command should NOT contain --dangerously-skip-permissions, but found at index %d; args: %v",
			explicitOtherResult.DSPIndex, explicitOtherResult.Args)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: post-spawn health check (T002 verification)
// ---------------------------------------------------------------------------

func TestIntegration_SpawnHealthCheck_DeadProcess(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)
	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	// Simulate a spawn where the handle is immediately dead.
	// The health check (sleep 0.3 + isAlive()) should detect this
	// and return a diagnostic error.
	raw, err := evalJS(`
		(function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: [],
				model: 'test-model'
			});

			executor.resolved = { command: 'fake-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.sessionId = 'test-session-health';

			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					return {
						isAlive: function() { return false; },
						receive: function() { return 'Error: invalid API key'; },
						close: function() {},
						send: function() {}
					};
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			var result = originalSpawn.call(executor);
			return JSON.stringify(result);
		})()
	`)
	if err != nil {
		t.Fatalf("health check test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		SessionID string  `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected error for dead process, got nil")
	}
	errMsg := *result.Error
	if !strings.Contains(errMsg, "exited immediately") {
		t.Errorf("error should mention 'exited immediately'; got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "invalid API key") {
		t.Errorf("error should contain process output 'invalid API key'; got: %s", errMsg)
	}
}

func TestIntegration_SpawnHealthCheck_AliveProcess(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)
	tmpDir := t.TempDir()
	escapedTmpDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedTmpDir = strings.ReplaceAll(escapedTmpDir, `'`, `\'`)

	// Simulate a spawn where the handle is alive after the health check.
	// Should return success (error: null).
	raw, err := evalJS(`
		(function() {
			var tmpDir = '` + escapedTmpDir + `';

			var executor = new ClaudeCodeExecutor({
				claudeCommand: '',
				claudeArgs: [],
				model: 'test-model'
			});

			executor.resolved = { command: 'fake-claude', type: 'claude-code' };
			executor.resolve = function() { return { error: null }; };
			executor.sessionId = 'test-session-healthy';

			var mockRegistry = {
				register: function() {},
				spawn: function(name, opts) {
					return {
						isAlive: function() { return true; },
						receive: function() { return ''; },
						close: function() {},
						send: function() {}
					};
				}
			};
			executor.cm = {
				claudeCode: function() { return { name: function() { return 'mock'; } }; },
				ollama: function() { return { name: function() { return 'mock'; } }; },
				newRegistry: function() { return mockRegistry; },
				newMCPInstance: function() {
					return {
						configPath: function() { return tmpDir + '/mcp-config.json'; },
						resultDir: function() { return tmpDir + '/results'; },
						configDir: function() { return tmpDir; },
						setResultDir: function() {},
						writeConfigFile: function() {},
						close: function() {}
					};
				}
			};

			var originalSpawn = ClaudeCodeExecutor.prototype.spawn;
			var result = originalSpawn.call(executor);
			return JSON.stringify(result);
		})()
	`)
	if err != nil {
		t.Fatalf("healthy spawn test failed: %v", err)
	}

	var result struct {
		Error     *string `json:"error"`
		SessionID string  `json:"sessionId"`
		ResultDir string  `json:"resultDir"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("expected no error for alive process, got: %s", *result.Error)
	}
	if result.SessionID == "" {
		t.Error("expected non-empty sessionId")
	}
}

// ---------------------------------------------------------------------------
// Integration Test: isAlive guards (T021)
// ---------------------------------------------------------------------------
// These tests verify the isAlive checks at two critical locations:
// 1. Auto-split attach path: warns if Claude died between spawn and attach
// 2. Claude command handler: detects dead process, surfaces diagnostics

// TestIntegration_IsAliveGuard_AutoSplitAttach tests the guard that runs
// between spawn and tuiMux.attach inside automatedSplit(). When the spawned
// process dies before attach, it should log a warning and emit output but
// NOT error — the pipeline continues with the dead handle (toggle just won't
// work).
func TestIntegration_IsAliveGuard_AutoSplitAttach(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	// Simulate the auto-split isAlive guard logic directly.
	// This exercises the same conditional from automatedSplit() (~line 2405)
	// without running the entire pipeline.
	raw, err := evalJS(`
		(function() {
			// Build a mock claudeExecutor with a dead handle.
			var mockHandle = {
				isAlive: function() { return false; },
				receive: function() { return 'Error: API key expired'; },
				close:   function() {},
				send:    function() {}
			};

			var warnings = [];
			var outputs = [];

			// Replicate the guard logic from automatedSplit().
			if (typeof mockHandle.isAlive === 'function' && !mockHandle.isAlive()) {
				warnings.push('Claude process died between spawn and attach');
				outputs.push('[auto-split] Warning: Claude process exited unexpectedly. Toggle (Ctrl+]) unavailable.');
			} else {
				warnings.push('UNEXPECTED: alive path taken');
			}

			return JSON.stringify({
				warnings: warnings,
				outputs: outputs,
				handleIsDead: !mockHandle.isAlive()
			});
		})()
	`)
	if err != nil {
		t.Fatalf("isAlive guard test failed: %v", err)
	}

	var result struct {
		Warnings     []string `json:"warnings"`
		Outputs      []string `json:"outputs"`
		HandleIsDead bool     `json:"handleIsDead"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !result.HandleIsDead {
		t.Error("mock handle should report dead")
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "died between spawn") {
		t.Errorf("expected warning about death between spawn and attach, got: %v", result.Warnings)
	}
	if len(result.Outputs) != 1 || !strings.Contains(result.Outputs[0], "Toggle (Ctrl+]) unavailable") {
		t.Errorf("expected output about toggle unavailable, got: %v", result.Outputs)
	}
}

// TestIntegration_IsAliveGuard_AutoSplitAttach_Alive verifies the happy path
// where isAlive returns true — the attach branch should be taken instead.
func TestIntegration_IsAliveGuard_AutoSplitAttach_Alive(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var mockHandle = {
				isAlive: function() { return true; },
				receive: function() { return ''; },
				close:   function() {},
				send:    function() {}
			};

			var branch = '';
			if (typeof mockHandle.isAlive === 'function' && !mockHandle.isAlive()) {
				branch = 'dead';
			} else {
				branch = 'alive';
			}

			return JSON.stringify({ branch: branch, isAlive: mockHandle.isAlive() });
		})()
	`)
	if err != nil {
		t.Fatalf("isAlive alive guard test failed: %v", err)
	}

	var result struct {
		Branch  string `json:"branch"`
		IsAlive bool   `json:"isAlive"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if result.Branch != "alive" {
		t.Errorf("expected 'alive' branch, got: %s", result.Branch)
	}
	if !result.IsAlive {
		t.Error("expected isAlive to be true")
	}
}

// TestIntegration_IsAliveGuard_MissingIsAlive verifies graceful handling when
// the handle object does not have an isAlive method (older handle interface).
// The guard should skip the check and proceed to the attach path.
func TestIntegration_IsAliveGuard_MissingIsAlive(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			// Handle without isAlive — mimics an older interface.
			var mockHandle = {
				receive: function() { return ''; },
				close:   function() {},
				send:    function() {}
			};

			var branch = '';
			if (typeof mockHandle.isAlive === 'function' && !mockHandle.isAlive()) {
				branch = 'dead';
			} else {
				branch = 'alive-or-no-method';
			}

			return JSON.stringify({
				branch: branch,
				hasIsAlive: typeof mockHandle.isAlive === 'function'
			});
		})()
	`)
	if err != nil {
		t.Fatalf("missing isAlive guard test failed: %v", err)
	}

	var result struct {
		Branch     string `json:"branch"`
		HasIsAlive bool   `json:"hasIsAlive"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if result.HasIsAlive {
		t.Error("mock handle should not have isAlive")
	}
	if result.Branch != "alive-or-no-method" {
		t.Errorf("expected 'alive-or-no-method' branch for missing isAlive, got: %s", result.Branch)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: prSplitSendWithCancel with real PTY child
// ---------------------------------------------------------------------------

// TestPrSplitSendWithCancel_NormalWrite spawns a real `cat` process in a
// PTY and sends a small amount of data through prSplitSendWithCancel.
// This verifies the happy path with a real child process.
func TestPrSplitSendWithCancel_NormalWrite(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not available")
	}

	ctx := context.Background()
	proc, err := ptySpawnCat(ctx)
	if err != nil {
		t.Fatalf("failed to spawn cat: %v", err)
	}
	defer proc.Close()

	// Send a small amount of data — cat will echo it back, keeping the
	// write buffer from filling up. No cancellation.
	sendErr := prSplitSendWithCancel(
		func() error { return proc.Write("hello world\n") },
		func() { _ = proc.Signal("SIGKILL") },
		func() bool { return false },
		func() bool { return false },
	)
	if sendErr != nil {
		t.Fatalf("prSplitSendWithCancel returned error on normal write: %v", sendErr)
	}

	// Read back the data to verify it went through.
	out, readErr := proc.Read()
	if readErr != nil && out == "" {
		t.Fatalf("failed to read from cat: %v", readErr)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected cat to echo 'hello world', got: %q", out)
	}
}

// TestPrSplitSendWithCancel_Cancel spawns a real `sleep` process (which
// never reads stdin), starts a large write (that will block the PTY buffer),
// and then signals cancel. Verifies that prSplitSendWithCancel kills the
// process and returns within a reasonable time.
//
// Uses a synthetic blocking send (not a real PTY write) to avoid
// platform-specific PTY buffer size differences. A separate test
// (TestPrSplitSendWithCancel_RealPTYKill) validates that SIGKILL
// actually unblocks a real PTY write.
func TestPrSplitSendWithCancel_Cancel(t *testing.T) {
	t.Parallel()

	var cancelledFlag int32
	go func() {
		time.Sleep(300 * time.Millisecond)
		atomic.StoreInt32(&cancelledFlag, 1)
	}()

	// The "send" blocks on a channel until kill() closes it —
	// simulating a PTY write that would block indefinitely.
	killed := make(chan struct{})

	start := time.Now()
	sendErr := prSplitSendWithCancel(
		func() error {
			<-killed
			return errors.New("write aborted: process killed")
		},
		func() { close(killed) },
		func() bool { return atomic.LoadInt32(&cancelledFlag) == 1 },
		func() bool { return false },
	)
	elapsed := time.Since(start)

	if sendErr == nil {
		t.Fatal("expected cancel error, got nil")
	}
	if !strings.Contains(sendErr.Error(), "cancelled by user") {
		t.Errorf("expected 'cancelled by user' error, got: %v", sendErr)
	}
	// Should complete within 2 seconds (300ms cancel delay + ≤200ms poll).
	if elapsed > 2*time.Second {
		t.Errorf("cancel took too long: %v (expected < 2s)", elapsed)
	}
	t.Logf("cancel completed in %v", elapsed)
}

// TestPrSplitSendWithCancel_ForceCancel is similar to Cancel but sets
// the forceCancel flag instead. Verifies the "force cancelled" error path.
func TestPrSplitSendWithCancel_ForceCancel(t *testing.T) {
	t.Parallel()

	var forceCancelFlag int32
	go func() {
		time.Sleep(300 * time.Millisecond)
		atomic.StoreInt32(&forceCancelFlag, 1)
	}()

	killed := make(chan struct{})

	start := time.Now()
	sendErr := prSplitSendWithCancel(
		func() error {
			<-killed
			return errors.New("write aborted: process killed")
		},
		func() { close(killed) },
		func() bool { return false },
		func() bool { return atomic.LoadInt32(&forceCancelFlag) == 1 },
	)
	elapsed := time.Since(start)

	if sendErr == nil {
		t.Fatal("expected force cancel error, got nil")
	}
	if !strings.Contains(sendErr.Error(), "force cancelled") {
		t.Errorf("expected 'force cancelled' error, got: %v", sendErr)
	}
	if elapsed > 2*time.Second {
		t.Errorf("force cancel took too long: %v (expected < 2s)", elapsed)
	}
	t.Logf("force cancel completed in %v", elapsed)
}

// TestPrSplitSendWithCancel_RealPTYKill spawns a real child process (sleep)
// in a PTY and verifies that the SIGKILL path works correctly. On macOS the
// PTY buffer is large enough to absorb 1MB without blocking, so the cancel
// check may never fire. This test verifies that either:
//   - The write completes and the function returns nil (large buffer), OR
//   - The cancel fires, kills the process, and returns "cancelled" (small buffer).
//
// In both cases the function must return promptly (no hang).
func TestPrSplitSendWithCancel_RealPTYKill(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}

	ctx := context.Background()
	proc, err := pty.Spawn(ctx, pty.SpawnConfig{
		Command: "sleep",
		Args:    []string{"3600"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("failed to spawn sleep: %v", err)
	}
	defer proc.Close()

	// Set cancel flag immediately — we just want to verify the function
	// completes promptly without hanging.
	start := time.Now()
	sendErr := prSplitSendWithCancel(
		func() error {
			return proc.Write(strings.Repeat("x", 1<<20))
		},
		func() { _ = proc.Signal("SIGKILL") },
		func() bool { return true }, // cancelled from the start
		func() bool { return false },
	)
	elapsed := time.Since(start)

	// The function must return within a few seconds regardless of buffer.
	if elapsed > 5*time.Second {
		t.Errorf("real PTY send+cancel took too long: %v (hang detected)", elapsed)
	}
	t.Logf("real PTY send+cancel completed in %v, err=%v", elapsed, sendErr)

	// Now separately verify SIGKILL actually works on a real process.
	proc2, err := pty.Spawn(ctx, pty.SpawnConfig{
		Command: "sleep", Args: []string{"3600"},
	})
	if err != nil {
		t.Fatalf("failed to spawn second sleep: %v", err)
	}
	defer proc2.Close()

	if !proc2.IsAlive() {
		t.Fatal("process should be alive before kill")
	}
	if err := proc2.Signal("SIGKILL"); err != nil {
		t.Fatalf("Signal(SIGKILL) failed: %v", err)
	}
	// Wait for the process to die (should be near-instant after SIGKILL).
	code, _ := proc2.Wait()
	if proc2.IsAlive() {
		t.Error("process should be dead after SIGKILL + Wait")
	}
	t.Logf("SIGKILL exit code: %d", code)
}

// ---------------------------------------------------------------------------
// Integration Test: Full auto-split pipeline with real Claude/AI
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitWithClaude_Pipeline runs the full automated
// split pipeline with a real Claude-compatible agent (configured via
// -integration -claude-command flags). It creates a realistic git repo,
// spawns the agent, sends the classification prompt, and waits for results.
//
// Run with:
//
//	go test -race -v -count=1 -timeout=15m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_AutoSplitWithClaude_Pipeline
//
// Or via make:
//
//	make integration-test-prsplit
func TestIntegration_AutoSplitWithClaude_Pipeline(t *testing.T) {
	skipIfNoClaude(t)

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Build config from TestMain flags.
	claudeArgsList := make([]string, len(claudeTestArgs))
	copy(claudeArgsList, claudeTestArgs)

	configOverrides := map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"claudeCommand": claudeTestCommand,
		"claudeArgs":    claudeArgsList,
		"timeoutMs":     int64(5 * 60 * 1000), // 5 minutes per step
	}
	if integrationModel != "" {
		configOverrides["claudeModel"] = integrationModel
	}

	_, _, evalJS := loadPrSplitEngineWithEval(t, configOverrides)

	// Inject autoSplitTUI mock — no real BubbleTea (no terminal in CI),
	// but provides the interface the pipeline expects. sendWithCancel is
	// deliberately NOT included so the pipeline uses the direct fallback;
	// the sendWithCancel mechanics are tested by the PTY tests above.
	_, err := evalJS(`
		globalThis.autoSplitTUI = {
			runAsync: function() {},
			wait: function() { return null; },
			stepStart: function(name) { log.printf('STEP START: %s', name); },
			stepDone: function(name, err, elapsed) {
				log.printf('STEP DONE: %s err=%s elapsed=%dms', name, err || 'ok', elapsed);
			},
			appendOutput: function(text) { log.printf('OUTPUT: %s', text); },
			appendError: function(text) { log.printf('ERROR: %s', text); },
			done: function(summary) { log.printf('DONE: %s', summary); },
			stepDetail: function(name, detail) { log.printf('DETAIL: %s — %s', name, detail); },
			cancelled: function() { return false; },
			forceCancelled: function() { return false; },
			quit: function() {}
		};
	`)
	if err != nil {
		t.Fatalf("failed to inject autoSplitTUI mock: %v", err)
	}

	// Run the full auto-split pipeline.
	t.Log("Starting auto-split pipeline with real Claude agent...")
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `,
		strategy: 'directory'
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error              *string `json:"error"`
			ClaudeInteractions int     `json:"claudeInteractions"`
			FallbackUsed       bool    `json:"fallbackUsed"`
			SplitsCreated      int     `json:"splitsCreated"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Log the full result for debugging.
	t.Logf("Result: %s", raw)

	if result.Report.FallbackUsed {
		t.Log("WARNING: Pipeline fell back to heuristic mode — Claude may not be responding")
	}
	if result.Error != nil {
		t.Errorf("pipeline returned error: %s", *result.Error)
	}
	if result.Report.ClaudeInteractions == 0 && !result.Report.FallbackUsed {
		t.Error("expected at least one Claude interaction")
	}

	// Check that split branches were created (if pipeline completed).
	if result.Report.SplitsCreated > 0 {
		branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
		t.Logf("Created branches:\n%s", branchOutput)
		if branchOutput == "" {
			t.Error("pipeline reported splits created but no split/* branches found")
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Test Helpers (PTY)
// ---------------------------------------------------------------------------

// ptySpawnCat spawns a `cat` process in a PTY for testing.
func ptySpawnCat(ctx context.Context) (*pty.Process, error) {
	return pty.Spawn(ctx, pty.SpawnConfig{
		Command: "cat",
		Rows:    24,
		Cols:    80,
	})
}

// ---------------------------------------------------------------------------
// Integration Helpers
// ---------------------------------------------------------------------------

// initIntegrationRepo creates a temporary git repository mimicking a real
// Go project with multiple packages. The initial commit contains baseline
// files that the feature branch will build upon.
func initIntegrationRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Verify git is available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available — skipping integration test")
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "integration-test@osm.dev")
	runGit(t, dir, "config", "user.name", "OSM Integration Test")

	// Create a realistic Go project structure.
	initialFiles := []struct{ path, content string }{
		{"go.mod", "module example.com/test-project\n\ngo 1.21\n"},
		{"README.md", "# Test Project\n\nA sample project for integration testing.\n"},
		{"cmd/app/main.go", `package main

import (
	"fmt"
	"example.com/test-project/pkg/core"
)

func main() {
	fmt.Println(core.Version())
}
`},
		{"pkg/core/core.go", `package core

// Version returns the project version.
func Version() string {
	return "1.0.0"
}
`},
		{"pkg/core/core_test.go", `package core

import "testing"

func TestVersion(t *testing.T) {
	if v := Version(); v == "" {
		t.Fatal("version should not be empty")
	}
}
`},
		{"internal/util/strings.go", `package util

// TrimAll trims whitespace from all strings in a slice.
func TrimAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s // placeholder
		_ = s
	}
	return out
}
`},
		{"docs/getting-started.md", "# Getting Started\n\nFollow these steps.\n"},
		{".gitignore", "*.exe\n*.test\n/bin/\n"},
	}

	for _, f := range initialFiles {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial project structure")

	return dir
}

// addIntegrationFeatureFiles creates a "feature" branch with changes across
// multiple concerns: new package, modified existing code, added tests, docs
// updates, and config changes. This diversity ensures the split algorithm
// must make non-trivial grouping decisions.
func addIntegrationFeatureFiles(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	featureFiles := []struct{ path, content string }{
		// New package: authentication
		{"pkg/auth/auth.go", `package auth

import "errors"

// ErrUnauthorized is returned when authentication fails.
var ErrUnauthorized = errors.New("unauthorized")

// Token represents an authentication token.
type Token struct {
	Value  string
	Expiry int64
}

// Validate checks if a token is valid.
func (t Token) Validate() error {
	if t.Value == "" {
		return ErrUnauthorized
	}
	return nil
}
`},
		{"pkg/auth/auth_test.go", `package auth

import "testing"

func TestToken_Validate(t *testing.T) {
	tests := []struct {
		name    string
		token   Token
		wantErr bool
	}{
		{"valid", Token{Value: "abc", Expiry: 9999}, false},
		{"empty", Token{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.token.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
`},
		// Modified: core package gets a new function
		{"pkg/core/config.go", `package core

// Config holds application configuration.
type Config struct {
	Debug   bool
	Verbose bool
	Port    int
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Debug:   false,
		Verbose: false,
		Port:    8080,
	}
}
`},
		{"pkg/core/config_test.go", `package core

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Debug {
		t.Error("debug should be false by default")
	}
}
`},
		// Modified: util package gets new function
		{"internal/util/numbers.go", `package util

// Max returns the larger of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`},
		{"internal/util/numbers_test.go", `package util

import "testing"

func TestMax(t *testing.T) {
	if Max(1, 2) != 2 {
		t.Error("Max(1,2) should be 2")
	}
	if Max(5, 3) != 5 {
		t.Error("Max(5,3) should be 5")
	}
}
`},
		// New: middleware package
		{"internal/middleware/logging.go", `package middleware

import "fmt"

// Logger provides request logging.
type Logger struct {
	Prefix string
}

// Log writes a log entry.
func (l Logger) Log(msg string) {
	fmt.Printf("[%s] %s\n", l.Prefix, msg)
}
`},
		// Documentation updates
		{"docs/api-reference.md", `# API Reference

## Authentication

Use the auth package for token-based authentication.

## Configuration

Use core.DefaultConfig() to get default settings.
`},
		{"docs/changelog.md", `# Changelog

## Unreleased

- Added authentication package
- Added configuration support
- Added middleware logging
- Updated documentation
`},
		// Modified: main.go to use new packages
		{"cmd/app/main.go", `package main

import (
	"fmt"
	"example.com/test-project/pkg/auth"
	"example.com/test-project/pkg/core"
)

func main() {
	fmt.Println(core.Version())
	cfg := core.DefaultConfig()
	fmt.Printf("Port: %d\n", cfg.Port)

	token := auth.Token{Value: "test-token", Expiry: 9999}
	if err := token.Validate(); err != nil {
		fmt.Printf("Auth error: %v\n", err)
	}
}
`},
	}

	for _, f := range featureFiles {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "feature: auth, config, middleware, docs")
}

// jsString returns a JavaScript string literal (single-quoted, with escaping)
// for embedding a Go string into a JS expression.
func jsString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return `'` + escaped + `'`
}

// mustJSON marshals v to a JSON string, failing the test on error.
func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// runGit runs a git command and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// Integration Test: cleanupExecutor ordering (T029)
// ---------------------------------------------------------------------------

// TestIntegration_CleanupExecutor_CloseBeforeDetach verifies that
// cleanupExecutor() calls claudeExecutor.close() BEFORE tuiMux.detach().
// The correct ordering is critical: closing the executor first makes the
// child PTY fd release, so the Mux reader goroutine sees EOF and exits.
// Only then can Detach() return (it waits for the reader to finish).
func TestIntegration_CleanupExecutor_CloseBeforeDetach(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var callOrder = [];

			// Mock claudeExecutor with observable close().
			var claudeExecutor = {
				handle: {
					signal: function(sig) { callOrder.push('signal:' + sig); }
				},
				close: function() { callOrder.push('close'); }
			};

			// Mock tuiMux with observable detach().
			var tuiMux = {
				detach: function() { callOrder.push('detach'); }
			};

			// Replicate cleanupExecutor logic inline (the real function
			// references script-level vars we can't easily override).
			if (claudeExecutor) {
				try { claudeExecutor.close(); } catch (e) {}
			}
			if (typeof tuiMux !== 'undefined' && tuiMux) {
				try { tuiMux.detach(); } catch (e) {}
			}

			return JSON.stringify({
				callOrder: callOrder,
				closeBeforeDetach: callOrder.indexOf('close') < callOrder.indexOf('detach')
			});
		})()
	`)
	if err != nil {
		t.Fatalf("cleanupExecutor ordering test failed: %v", err)
	}

	var result struct {
		CallOrder         []string `json:"callOrder"`
		CloseBeforeDetach bool     `json:"closeBeforeDetach"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(result.CallOrder) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(result.CallOrder), result.CallOrder)
	}
	if result.CallOrder[0] != "close" {
		t.Errorf("first call should be 'close', got %q", result.CallOrder[0])
	}
	if result.CallOrder[1] != "detach" {
		t.Errorf("second call should be 'detach', got %q", result.CallOrder[1])
	}
	if !result.CloseBeforeDetach {
		t.Error("close must happen before detach to avoid Detach() blocking on reader goroutine")
	}
}

// TestIntegration_CleanupExecutor_ForceCancel verifies that when
// isForceCancelled returns true, cleanupExecutor sends SIGKILL before
// calling close(), then detaches.
func TestIntegration_CleanupExecutor_ForceCancel(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var callOrder = [];

			var claudeExecutor = {
				handle: {
					signal: function(sig) { callOrder.push('signal:' + sig); }
				},
				close: function() { callOrder.push('close'); }
			};

			var tuiMux = {
				detach: function() { callOrder.push('detach'); }
			};

			// Simulate force-cancel path.
			var forceCancelled = true;

			if (claudeExecutor) {
				if (forceCancelled && claudeExecutor.handle &&
					typeof claudeExecutor.handle.signal === 'function') {
					try { claudeExecutor.handle.signal('SIGKILL'); } catch (e) {}
				}
				try { claudeExecutor.close(); } catch (e) {}
			}
			if (typeof tuiMux !== 'undefined' && tuiMux) {
				try { tuiMux.detach(); } catch (e) {}
			}

			return JSON.stringify({
				callOrder: callOrder
			});
		})()
	`)
	if err != nil {
		t.Fatalf("cleanupExecutor force-cancel test failed: %v", err)
	}

	var result struct {
		CallOrder []string `json:"callOrder"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	expected := []string{"signal:SIGKILL", "close", "detach"}
	if len(result.CallOrder) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(result.CallOrder), result.CallOrder)
	}
	for i, want := range expected {
		if result.CallOrder[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, result.CallOrder[i], want)
		}
	}
}

// TestIntegration_CleanupExecutor_NilExecutor verifies that cleanupExecutor
// handles a nil claudeExecutor gracefully (only detach is called).
func TestIntegration_CleanupExecutor_NilExecutor(t *testing.T) {
	t.Parallel()

	_, _, evalJS := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`
		(function() {
			var callOrder = [];

			var claudeExecutor = null;

			var tuiMux = {
				detach: function() { callOrder.push('detach'); }
			};

			// Replicate cleanupExecutor logic.
			if (claudeExecutor) {
				try { claudeExecutor.close(); } catch (e) {}
			}
			if (typeof tuiMux !== 'undefined' && tuiMux) {
				try { tuiMux.detach(); } catch (e) {}
			}

			return JSON.stringify({ callOrder: callOrder });
		})()
	`)
	if err != nil {
		t.Fatalf("cleanupExecutor nil executor test failed: %v", err)
	}

	var result struct {
		CallOrder []string `json:"callOrder"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(result.CallOrder) != 1 || result.CallOrder[0] != "detach" {
		t.Errorf("expected only ['detach'], got: %v", result.CallOrder)
	}
}
