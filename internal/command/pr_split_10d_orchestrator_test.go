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
	Error  string `json:"error"`
	Report struct {
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
