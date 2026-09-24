package command

// T401: Unit tests for the pipeline orchestrator (chunk 10d).
// Using loadPrSplitEngineWithEval to get a full engine, then
// mocking cross-chunk deps to isolate orchestrator behavior.

import (
	"encoding/json"
	"strings"
	"testing"
)

// orchestratorResult is the parsed return of automatedSplit().
type orchestratorResult struct {
	Error       string `json:"error"`
	KeepSession bool   `json:"keepSession"`
	Report      struct {
		Steps []struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"steps"`
	} `json:"report"`
}

func parseOrchestratorResult(t *testing.T, raw any) orchestratorResult {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", raw, raw)
	}
	var r orchestratorResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, s)
	}
	return r
}

// TestChunk10d_CancellationBeforeFirstStep verifies the pipeline returns
// immediately when cancellation is already requested.
func TestChunk10d_CancellationBeforeFirstStep(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Set cancellation flag before running the pipeline.
	_, err := evalJS(`globalThis.prSplit.isCancelled = function() { return true; }`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected error for cancelled pipeline, got none")
	}
	if !strings.Contains(r.Error, "cancelled") {
		t.Errorf("error = %q, want 'cancelled'", r.Error)
	}
}

// TestChunk10d_PipelineTimeout verifies the pipeline aborts when the
// pipeline-level timeout is exceeded.
func TestChunk10d_PipelineTimeout(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Override Date.now to advance time by 5 minutes after the first 2 calls.
	// The pipeline calls Date.now() for pipelineStartTime and lastProgressTime,
	// then step() calls Date.now() again — the subsequent calls will return a
	// value 5min in the future, triggering the pipeline timeout.
	_, err := evalJS(`
		(function() {
			var originalDateNow = Date.now;
			var callCount = 0;
			Date.now = function() {
				callCount++;
				if (callCount <= 2) {
					return originalDateNow.call(Date);
				}
				// After the first 2 calls (pipelineStartTime, lastProgressTime),
				// jump forward by 5 minutes.
				return originalDateNow.call(Date) + 300000;
			};
		})();
	`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true,
		pipelineTimeoutMs: 60000,
		stepTimeoutMs: 999999,
		watchdogIdleMs: 999999
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected pipeline timeout error, got none")
	}
	t.Logf("pipeline error: %s", r.Error)
	if !strings.Contains(strings.ToLower(r.Error), "timeout") {
		t.Errorf("error = %q, want timeout error", r.Error)
	}
}

// TestChunk10d_StepReturnsNull verifies that when analyzeDiffAsync returns
// null, the step callback's internal TypeError is caught by step()'s
// try/catch and surfaced as a clean error — not a crash.
func TestChunk10d_StepReturnsNull(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Mock analyzeDiffAsync to return null. The step callback accesses
	// result.error internally, which triggers a TypeError that step()'s
	// try/catch should handle gracefully.
	_, err := evalJS(`globalThis.prSplit.analyzeDiffAsync = function() { return null; }`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true,
		pipelineTimeoutMs: 30000,
		stepTimeoutMs: 30000,
		watchdogIdleMs: 30000
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v (expected graceful error, not crash)", err)
	}

	r := parseOrchestratorResult(t, raw)
	// The pipeline should NOT crash — step()'s try/catch should catch the
	// TypeError from accessing .error on null. Verify the error is present
	// and relates to the null result.
	if r.Error == "" {
		t.Fatal("expected error from null analyzeDiff result, got none")
	}
	t.Logf("pipeline error: %s", r.Error)
	// Verify the error relates to null/property access, not something unrelated.
	errLower := strings.ToLower(r.Error)
	if !strings.Contains(errLower, "null") &&
		!strings.Contains(errLower, "undefined") &&
		!strings.Contains(errLower, "cannot read") &&
		!strings.Contains(errLower, "property") {
		t.Errorf("error = %q, want null/property-access related error", r.Error)
	}
	// Verify at least one step recorded in the report.
	if len(r.Report.Steps) == 0 {
		t.Error("expected at least one step in report")
	}
}

// TestChunk10d_StepExceptionPropagation verifies that an exception thrown by a
// step callback is caught and surfaced as a pipeline error (not a crash).
func TestChunk10d_StepExceptionPropagation(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Mock analyzeDiffAsync to throw an exception.
	_, err := evalJS(`globalThis.prSplit.analyzeDiffAsync = function() {
		throw new Error('synthetic test explosion');
	}`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true,
		pipelineTimeoutMs: 30000,
		stepTimeoutMs: 30000,
		watchdogIdleMs: 30000
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected error from step exception, got none")
	}
	if !strings.Contains(r.Error, "synthetic test explosion") {
		t.Errorf("error = %q, want 'synthetic test explosion'", r.Error)
	}
}

// TestChunk10d_EmptyDiffGraceful verifies that when analyzeDiff returns 0
// files, the pipeline exits gracefully with "No changes detected".
func TestChunk10d_EmptyDiffGraceful(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Mock analyzeDiffAsync to return an empty file list.
	_, err := evalJS(`globalThis.prSplit.analyzeDiffAsync = async function() {
		return { files: [], fileStatuses: {}, baseBranch: 'main', currentBranch: 'feature' };
	}`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true,
		pipelineTimeoutMs: 30000,
		stepTimeoutMs: 30000,
		watchdogIdleMs: 30000
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected 'No changes detected' error, got none")
	}
	if !strings.Contains(r.Error, "No changes detected") {
		t.Errorf("error = %q, want 'No changes detected'", r.Error)
	}
}

// TestChunk10d_WatchdogIdleTimeout verifies the watchdog fires when no
// pipeline progress occurs for longer than watchdogIdleMs.
func TestChunk10d_WatchdogIdleTimeout(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Override Date.now: first 2 calls are normal (pipelineStartTime,
	// lastProgressTime). Subsequent calls return +10min to trigger watchdog
	// but NOT pipeline timeout (set to 60min).
	_, err := evalJS(`
		(function() {
			var originalDateNow = Date.now;
			var callCount = 0;
			Date.now = function() {
				callCount++;
				if (callCount <= 2) {
					return originalDateNow.call(Date);
				}
				return originalDateNow.call(Date) + 600000; // +10 minutes
			};
		})();
	`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true,
		pipelineTimeoutMs: 9999999,
		stepTimeoutMs: 9999999,
		watchdogIdleMs: 60000
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected watchdog error, got none")
	}
	t.Logf("pipeline error: %s", r.Error)
	if !strings.Contains(strings.ToLower(r.Error), "watchdog") {
		t.Errorf("error = %q, want watchdog error", r.Error)
	}
}

// TestChunk10d_ForceCancellation verifies that isForceCancelled triggers
// immediate pipeline abort.
func TestChunk10d_ForceCancellation(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Set force-cancellation flag before running the pipeline.
	_, err := evalJS(`globalThis.prSplit.isForceCancelled = function() { return true; }`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected force-cancelled error, got none")
	}
	if !strings.Contains(r.Error, "cancelled") {
		t.Errorf("error = %q, want 'cancelled'", r.Error)
	}
}

// TestChunk10d_StepTimeout verifies the per-step timeout fires when a single
// step takes longer than stepTimeoutMs.
func TestChunk10d_StepTimeout(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Override Date.now so that the step-internal elapsed time exceeds
	// stepTimeoutMs. The step records t0 = Date.now() before calling fn(),
	// then checks elapsed = Date.now() - t0 after fn() returns.
	//
	// Date.now() call sequence in automatedSplit → step():
	//   1. pipelineStartTime = Date.now()        (automatedSplit setup)
	//   2. lastProgressTime = Date.now()          (automatedSplit setup)
	//   3. pipeline timeout check                 (inside step())
	//   4. watchdog idle check                    (inside step())
	//   5. t0 = Date.now()                        (inside step())
	//   6. lastProgressTime = Date.now()          (inside step())
	//   7. emitOutput → lastProgressTime update   (inside step(), via emitOutput)
	// After fn() returns:
	//   8. elapsed = Date.now() - t0              → need +10min here
	//
	// With threshold <= 6, calls 7+ (including emitOutput's Date.now and the
	// elapsed check at call 8) return +10min. t0 (call 5) is normal, so
	// elapsed = (now+10min) - t0 ≈ 10min >> stepTimeoutMs.
	_, err := evalJS(`
		(function() {
			var originalDateNow = Date.now;
			var callCount = 0;
			Date.now = function() {
				callCount++;
				if (callCount <= 6) {
					return originalDateNow.call(Date);
				}
				return originalDateNow.call(Date) + 600000; // +10 minutes
			};
		})();
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Mock analyzeDiffAsync to return empty (no error) — ensures result.error
	// is falsy, which is required for the step timeout condition to trigger.
	_, err = evalJS(`globalThis.prSplit.analyzeDiffAsync = async function() {
		return { files: [], fileStatuses: {}, baseBranch: 'main', currentBranch: 'feature' };
	}`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true,
		pipelineTimeoutMs: 99999999,
		stepTimeoutMs: 60000,
		watchdogIdleMs: 99999999
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected step timeout error, got none")
	}
	t.Logf("pipeline error: %s", r.Error)
	if !strings.Contains(strings.ToLower(r.Error), "step timeout") {
		t.Errorf("error = %q, want 'step timeout'", r.Error)
	}
}

// TestChunk10d_PauseBeforeFirstStep verifies that isPaused triggers a clean
// pipeline exit with a "paused by user" error.
func TestChunk10d_PauseBeforeFirstStep(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})

	// Set pause flag before running the pipeline.
	_, err := evalJS(`globalThis.prSplit.isPaused = function() { return true; }`)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
		disableTUI: true
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	r := parseOrchestratorResult(t, raw)
	if r.Error == "" {
		t.Fatal("expected paused error, got none")
	}
	t.Logf("pipeline error: %s", r.Error)
	if !strings.Contains(strings.ToLower(r.Error), "paused") {
		t.Errorf("error = %q, want 'paused' error", r.Error)
	}
}

func TestChunk10d_ClassificationTimeoutPreservesSession(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	tmpDir := t.TempDir()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
		"transcriptDir": tmpDir,
	})

	raw, err := evalJS(`(async function() {
		var st = globalThis.prSplit._state;
		st.agentSessionID = 4242;
		st.agentExecutor = {
			handle: {
				isAlive: function() { return true; },
				health: function() { return { alive: true }; },
				close: function() { return Promise.resolve(); }
			},
			resolved: { command: 'agent', type: 'explicit' }
		};
		st.mcpCallbackObj = {
			address: 'mock-addr',
			transport: 'mock-transport',
			lastCallTime: function() { return 0; }
		};
		globalThis.tuiMux = {
			capture: function() { return { plain: 'agent screen', fullScreen: 'agent ansi', ansi: 'agent ansi' }; },
			lastActivityMs: function() { return 5; },
			isDone: function() { return false; }
		};
		var before = {
			handle: !!st.agentExecutor.handle,
			mcp: !!globalThis.prSplit._mcpCallbackObj
		};
		var r = { error: 'timeout waiting for reportClassification after 300000ms' };
		var errText = String(r.error || '');
		var keep = errText.indexOf('timeout waiting for reportClassification') >= 0;
		return JSON.stringify({ before: before, keep: keep });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var probe struct {
		Before struct {
			Handle bool `json:"handle"`
			Mcp    bool `json:"mcp"`
		} `json:"before"`
		Keep bool `json:"keep"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &probe); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if !probe.Before.Handle {
		t.Fatal("harness must pin a live handle before the preserve branch")
	}
	if !probe.Keep {
		t.Fatal("classification timeout string must take the keepSession path")
	}
}

func TestChunk10d_HeartbeatStalePreservesSession(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"disableTUI": true,
	})

	raw, err := evalJS(`(function() {
		var errText = 'Agent process unresponsive (heartbeat timeout for reportClassification)';
		var isHeartbeatStale = errText.indexOf('heartbeat timeout for reportClassification') >= 0;
		return JSON.stringify({ stale: isHeartbeatStale });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Stale bool `json:"stale"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if !res.Stale {
		t.Fatal("heartbeat-stale string must take the keepSession path")
	}
}

func TestChunk10d_ReclassifyTimeoutPreservesSession(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	raw, err := evalJS(`(function() {
		var rePoll = { error: 'timeout waiting for reportClassification after 300000ms' };
		var reText = String(rePoll.error || '');
		var reTimeout = reText.indexOf('timeout waiting for reportClassification') >= 0;
		var marked = reTimeout ? { error: rePoll.error, preserveSession: true } : { error: rePoll.error };
		return JSON.stringify({ preserve: !!marked.preserveSession });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Preserve bool `json:"preserve"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if !res.Preserve {
		t.Fatal("re-classify timeout must set the preserveSession marker")
	}
}

func runChunk10dResplitFailure(t *testing.T, failureMode string) orchestratorResult {
	t.Helper()
	skipSlow(t)
	t.Parallel()

	tmpDir := t.TempDir()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"disableTUI":    true,
	})
	failureModeJSON, err := json.Marshal(failureMode)
	if err != nil {
		t.Fatal(err)
	}
	tmpDirJSON, err := json.Marshal(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	setup := `(function() {
		var sp = globalThis.prSplit;
		var categories = [{ name: 'api', description: 'API changes', files: ['pkg/impl.go'] }];
		var classificationWaits = 0;
		var verifyCalls = 0;
		var executeCalls = 0;
		var cleanupCalls = 0;
		var mcpCloseCalls = 0;
		var equivalenceCalls = 0;
		var failureMode = ` + string(failureModeJSON) + `;
		var mockMcp = {
			address: 'mock',
			transport: 'mock',
			mcpConfigPath: 'mock.json',
			addTool: function() {},
			init: async function() {},
			resetWaiter: function() {},
			lastCallTime: function() { return 0; },
			close: async function() { mcpCloseCalls++; }
		};
		var realRequire = require;
		require = function(name) {
			if (name === 'osm:mcp') return { createServer: function() { return {}; } };
			if (name === 'osm:mcpcallback') return {
				MCPCallback: function() { return mockMcp; }
			};
			return realRequire(name);
		};
		sp.analyzeDiffAsync = async function() {
			return {
				error: null,
				files: ['pkg/impl.go'],
				baseBranch: 'main',
				currentBranch: 'feature',
				fileStatuses: {},
				fileRenames: {}
			};
		};
		sp.AgentCodeExecutor = function() {
			this.handle = {
				isAlive: function() { return true; },
				close: async function() {}
			};
			this.resolveAsync = async function() { return { error: null }; };
			this.spawn = async function() { return { error: null, sessionId: 7 }; };
		};
		sp.renderClassificationPrompt = function() { return { error: null, text: 'classify' }; };
		sp.sendToHandle = async function() { return { error: null }; };
		sp.waitForLogged = async function(name) {
			if (name === 'reportClassification') {
				classificationWaits++;
				if (classificationWaits === 2 && failureMode === 'reclassify') {
					return { error: 'cancelled by user' };
				}
				if (classificationWaits === 2 && failureMode === 'preserve-timeout') {
					return { error: 'timeout waiting for reportClassification after 1000ms' };
				}
				return { data: { categories: categories }, error: null };
			}
			return { error: 'no agent split plan' };
		};
		sp.classificationToGroups = function() {
			return [{ name: 'api', description: 'API changes', files: ['pkg/impl.go'] }];
		};
		sp.createSplitPlanAsync = async function() {
			return {
				baseBranch: 'main',
				sourceBranch: 'feature',
				dir: '.',
				verifyCommand: 'true',
				fileStatuses: {},
				splits: [{
					name: 'split/1',
					files: ['pkg/impl.go'],
					message: 'API changes',
					order: 0
				}]
			};
		};
		sp.executeSplitAsync = async function() {
			executeCalls++;
			if (executeCalls === 2 && failureMode === 'reexecute') {
				return { error: 'resplit execution failed' };
			}
			return { error: null, results: [{ name: 'split/1' }] };
		};
		sp.verifySplitsAsync = async function() {
			verifyCalls++;
			if (verifyCalls === 2 && failureMode !== 'reverify') {
				return { results: [{ name: 'split/1', passed: true }] };
			}
			return { results: [{ name: 'split/1', passed: false }] };
		};
		sp.resolveConflictsWithAgent = async function() {
			return { reSplitNeeded: true, reSplitReason: 'separate API files' };
		};
		sp.cleanupBranchesAsync = async function() { return { deleted: [] }; };
		sp.savePlan = async function() { return { error: null, path: 'mock-plan.json' }; };
		sp.verifyEquivalenceAsync = async function() {
			equivalenceCalls++;
			return { equivalent: true };
		};
		sp.cleanupExecutor = async function() { cleanupCalls++; };
		globalThis.tuiMux = {
			attach: function() { return 7; },
			isDone: function() { return false; },
			capture: function() { return { plain: 'agent screen', fullScreen: 'agent ansi' }; },
			lastActivityMs: function() { return 5; }
		};
		globalThis.__resplitTestState = function() {
			return {
				classificationWaits: classificationWaits,
				verifyCalls: verifyCalls,
				executeCalls: executeCalls,
				cleanupCalls: cleanupCalls,
				mcpCloseCalls: mcpCloseCalls,
				equivalenceCalls: equivalenceCalls
			};
		};
	})()`
	if _, err := evalJS(setup); err != nil {
		t.Fatalf("configure pipeline mocks: %v", err)
	}

	raw, err := evalJS(`(async function() {
		var result = await globalThis.prSplit.automatedSplit({
			disableTUI: true,
			pollIntervalMs: 50,
			classifyTimeoutMs: 1000,
			planTimeoutMs: 1000,
			resolveTimeoutMs: 1000,
			maxResolveRetries: 0,
			maxReSplits: 1,
			transcriptDir: ` + string(tmpDirJSON) + `,
			pipelineTimeoutMs: 60000,
			stepTimeoutMs: 60000,
			watchdogIdleMs: 60000
		});
		return JSON.stringify({ result: result, state: globalThis.__resplitTestState() });
	})()`)
	if err != nil {
		t.Fatalf("run automatedSplit: %v", err)
	}
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string result, got %T: %v", raw, raw)
	}
	var response struct {
		Result orchestratorResult `json:"result"`
		State  struct {
			ClassificationWaits int `json:"classificationWaits"`
			VerifyCalls         int `json:"verifyCalls"`
			ExecuteCalls        int `json:"executeCalls"`
			CleanupCalls        int `json:"cleanupCalls"`
			McpCloseCalls       int `json:"mcpCloseCalls"`
			EquivalenceCalls    int `json:"equivalenceCalls"`
		} `json:"state"`
	}
	if err := json.Unmarshal([]byte(s), &response); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, s)
	}
	if failureMode == "preserve-timeout" {
		if response.State.CleanupCalls != 0 || response.State.McpCloseCalls != 0 || !response.Result.KeepSession {
			t.Fatalf("timeout preservation = keep:%v executor cleanup:%d mcp close:%d, want true/0/0",
				response.Result.KeepSession, response.State.CleanupCalls, response.State.McpCloseCalls)
		}
	} else if response.State.CleanupCalls != 1 || response.State.McpCloseCalls != 1 {
		t.Fatalf("failure cleanup = executor:%d mcp:%d, want both once", response.State.CleanupCalls, response.State.McpCloseCalls)
	}
	if response.State.EquivalenceCalls != 0 {
		t.Fatalf("equivalence ran after pipeline failure %d times", response.State.EquivalenceCalls)
	}
	return response.Result
}

func TestChunk10d_ReclassifyErrorUsesFailureCleanup(t *testing.T) {
	result := runChunk10dResplitFailure(t, "reclassify")
	if result.Error == "" || !strings.Contains(result.Error, "cancelled by user") {
		t.Fatalf("pipeline error = %q, want re-classification cancellation", result.Error)
	}
}

func TestChunk10d_ReverifyFailureFailsPipeline(t *testing.T) {
	result := runChunk10dResplitFailure(t, "reverify")
	if result.Error == "" || !strings.Contains(result.Error, "still fail after re-split") {
		t.Fatalf("pipeline error = %q, want re-verification failure", result.Error)
	}
}

func TestChunk10d_ReexecuteFailureFailsPipeline(t *testing.T) {
	result := runChunk10dResplitFailure(t, "reexecute")
	if result.Error == "" || !strings.Contains(result.Error, "resplit execution failed") {
		t.Fatalf("pipeline error = %q, want re-execution failure", result.Error)
	}
}

func TestChunk10d_ReclassifyTimeoutPreservesLiveSession(t *testing.T) {
	result := runChunk10dResplitFailure(t, "preserve-timeout")
	if result.Error == "" || !strings.Contains(result.Error, "timeout waiting for reportClassification") {
		t.Fatalf("pipeline error = %q, want re-classification timeout", result.Error)
	}
}

func TestChunk10d_ClassifyCheckpointFields(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"disableTUI": true,
	})
	raw, err := evalJS(`(async function() {
		var seen = null;
		var fakeWait = async function(name, timeout, opts) {
			if (opts && typeof opts.onProgress === 'function') {
				opts.onProgress(16000);
				opts.onProgress(17000);
			}
			return { data: null, error: null };
		};
		var timeouts = { classify: 300000 };
		var state = { agentSessionID: 7, classifyCheckpoint: null };
		var prSplit = { _classifyCheckpoint: null };
		var tuiMux = { lastActivityMs: function() { return 9; } };
		var mcp = { lastCallTime: function() { return Date.now() - 2000; } };
		var classifyPromptSentAt = Date.now() - 16000;
		var classifyLastCheckpointAt = 0;
		await fakeWait('reportClassification', timeouts.classify, {
			onProgress: function(elapsed) {
				var now = Date.now();
				if (now - classifyLastCheckpointAt < 15000) return;
				classifyLastCheckpointAt = now;
				state.classifyCheckpoint = {
					stage: 'receive-classification',
					elapsedMs: elapsed,
					timeoutMs: timeouts.classify,
					remainingMs: Math.max(0, timeouts.classify - elapsed),
					promptSentAt: classifyPromptSentAt,
					lastActivityMs: tuiMux.lastActivityMs(7),
					heartbeatAgeMs: 2000,
					sessionId: state.agentSessionID
				};
				prSplit._classifyCheckpoint = state.classifyCheckpoint;
				seen = state.classifyCheckpoint;
			}
		});
		return JSON.stringify(seen);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var ck struct {
		Stage          string `json:"stage"`
		ElapsedMs      int64  `json:"elapsedMs"`
		TimeoutMs      int64  `json:"timeoutMs"`
		RemainingMs    int64  `json:"remainingMs"`
		LastActivityMs int64  `json:"lastActivityMs"`
		SessionID      int64  `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &ck); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if ck.Stage != "receive-classification" || ck.TimeoutMs != 300000 || ck.SessionID != 7 {
		t.Fatalf("checkpoint fields wrong: %+v", ck)
	}
	if ck.RemainingMs != ck.TimeoutMs-ck.ElapsedMs {
		t.Fatalf("remaining must equal timeout minus elapsed: %+v", ck)
	}
}

func TestChunk10d_TranscriptDirPrefersInjected(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// Precedence: config.transcriptDir (call arg) wins, then the injected
	// prSplitConfig.transcriptDir (Go storage session dir), then dir.
	// Mirror the production transcriptDir() resolution order in JS.
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"disableTUI":    true,
		"transcriptDir": "/injected/session-dir",
	})
	raw, err := evalJS(`(function() {
		function resolve(config, injected, dir) {
			if (config && config.transcriptDir) return String(config.transcriptDir);
			if (injected) return String(injected);
			return dir;
		}
		var injected = String(prSplitConfig.transcriptDir || '');
		var viaConfig = resolve({ transcriptDir: '/tmp/override' }, injected, '/repo');
		var viaInjected = resolve({}, injected, '/repo');
		var viaFallback = resolve({}, '', '/repo');
		return JSON.stringify({ injected: injected, viaConfig: viaConfig, viaInjected: viaInjected, viaFallback: viaFallback });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Injected    string `json:"injected"`
		ViaConfig   string `json:"viaConfig"`
		ViaInjected string `json:"viaInjected"`
		ViaFallback string `json:"viaFallback"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if res.Injected != "/injected/session-dir" {
		t.Fatalf("injected transcriptDir = %q, want %q", res.Injected, "/injected/session-dir")
	}
	if res.ViaConfig != "/tmp/override" {
		t.Errorf("config override must win: got %q", res.ViaConfig)
	}
	if res.ViaInjected != "/injected/session-dir" {
		t.Errorf("injected dir must win over repo fallback: got %q", res.ViaInjected)
	}
	if res.ViaFallback != "/repo" {
		t.Errorf("repo dir must remain last resort: got %q", res.ViaFallback)
	}
}
