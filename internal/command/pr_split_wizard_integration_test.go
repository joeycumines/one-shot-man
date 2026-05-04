package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/mcpcallbackmod"
)

// ---------------------------------------------------------------------------
// T21: Wizard E2E integration tests with full JS engine + mock MCP
// ---------------------------------------------------------------------------

// TestIntegration_WizardBaselineRetry verifies the complete wizard flow when
// baseline verification fails: auto-split → BASELINE_FAIL → override command
// → automatedSplit via mock MCP → DONE.
//
// This exercises:
//   - handleConfigState baseline verification (chunk 13 calling chunk 06)
//   - auto-split TUI command storing wizard in tuiState
//   - override TUI command resuming pipeline from BASELINE_FAIL
//   - Full automatedSplit pipeline (chunks 01-10) with mock MCP injection
//   - Wizard state transitions through to DONE
func TestIntegration_WizardBaselineRetry(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	// Set up repo where baseline verify FAILS on main (missing .verify-ok)
	// but PASSES on feature (has .verify-ok). Split branches also verify
	// because every split includes .verify-ok.
	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{".verify-ok", "marker"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "test -f .verify-ok",
			"strategy":      "directory",
			"_evalTimeout":  90 * time.Second,
		},
	})

	// Mock ClaudeCodeExecutor (pipeline requires it for classify+plan).
	mockSetup := `
		prSplit.SEND_TEXT_CHUNK_BYTES = 1000000;
		var _mockSentPrompts = [];
		ClaudeCodeExecutor = function(config) {
			this.config = config;
			this.resolved = { command: 'mock-claude' };
			this.handle = {
				send: function(text) { _mockSentPrompts.push(text); },
				isAlive: function() { return true; }
			};
		};
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: null }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: null }; };
		ClaudeCodeExecutor.prototype.spawn = function(sid, opts) {
			return { error: null, sessionId: 'mock-session-baseline-retry' };
		};
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`
	if _, err := tp.EvalJS(mockSetup); err != nil {
		t.Fatalf("inject mock ClaudeCodeExecutor: %v", err)
	}

	// --- Step 1: dispatch auto-split → should hit BASELINE_FAIL ---
	if err := tp.Dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split dispatch: %v", err)
	}

	// Verify stdout shows baseline failure. (tuiState is IIFE-scoped so
	// we can't inspect wizard state directly — verify via output.)
	stdout := tp.Stdout.String()
	if !strings.Contains(stdout, "Baseline verification failed") &&
		!strings.Contains(stdout, "baseline") {
		t.Errorf("stdout missing baseline failure message:\n%s", stdout)
	}

	// --- Step 2: Set up MCP injection for the pipeline after override ---
	classJSON, _ := json.Marshal(map[string]any{
		"categories": []map[string]any{
			{
				"name":        "pkg",
				"description": "Package additions",
				"files":       []string{"pkg/handler.go", ".verify-ok"},
			},
			{
				"name":        "cmd",
				"description": "Command additions",
				"files":       []string{"cmd/run.go", ".verify-ok"},
			},
		},
	})
	planJSON, _ := json.Marshal(map[string]any{
		"stages": []map[string]any{
			{
				"name":    "split/01-pkg",
				"files":   []string{"pkg/handler.go", ".verify-ok"},
				"message": "Add package handler and verify marker",
			},
			{
				"name":    "split/02-cmd",
				"files":   []string{"cmd/run.go", ".verify-ok"},
				"message": "Add run command and verify marker",
			},
		},
	})

	watchCh := mcpcallbackmod.WatchForInit()
	go func() {
		h := <-watchCh
		if err := h.InjectToolResult("reportClassification", classJSON); err != nil {
			t.Logf("inject classification: %v", err)
		}
		if err := h.InjectToolResult("reportSplitPlan", planJSON); err != nil {
			t.Logf("inject plan: %v", err)
		}
	}()

	// --- Step 3: dispatch override → triggers pipeline via automatedSplit ---
	if err := tp.Dispatch("override", nil); err != nil {
		t.Fatalf("override dispatch: %v", err)
	}

	// --- Step 4: Verify wizard ended in DONE ---
	// After override, tuiState._activeWizard is set to null and the wizard
	// local to the override handler should be in DONE. We can verify via
	// the output since the wizard was cleared. Let's check the last wizard
	// state from the output and verify DONE transitions happened.
	finalStdout := tp.Stdout.String()

	// The override handler should have printed "Overriding baseline failure".
	if !strings.Contains(finalStdout, "Overriding baseline failure") &&
		!strings.Contains(finalStdout, "overrid") {
		t.Errorf("stdout missing override message:\n%s", finalStdout)
	}

	// Verify the pipeline actually ran (should mention steps or produce report).
	// The pipeline sends prompts to the mock Claude, so there should be
	// classification+plan interaction.
	promptCount, err := tp.EvalJS(`_mockSentPrompts.length`)
	if err != nil {
		t.Fatalf("check prompt count: %v", err)
	}
	pCount, ok := promptCount.(int64)
	if !ok {
		// Goja may return float64 for numbers.
		if f, fok := promptCount.(float64); fok {
			pCount = int64(f)
		}
	}
	if pCount < 1 {
		t.Errorf("expected at least 1 prompt sent to mock Claude, got %d", pCount)
	}
}

// TestIntegration_WizardHandlerChain_PlanReject tests the wizard handler chain
// with all 14 chunks loaded: CONFIG → PLAN_REVIEW → regenerate → PLAN_REVIEW
// → approve → BRANCH_BUILDING (real git) → EQUIV_CHECK → FINALIZATION → DONE.
//
// This exercises the handler functions calling real chunk functions
// (executeSplit from chunk 05, verifyEquivalence from chunk 06).
func TestIntegration_WizardHandlerChain_PlanReject(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// Create wizard, transition through CONFIG.
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		var _cfgResult = prSplit._handleConfigState({
			baseBranch: 'main',
			strategy: 'directory',
			verifyCommand: 'true'
		});
		if (_cfgResult.error) throw new Error('config failed: ' + _cfgResult.error);
		// CONFIG → PLAN_GENERATION
		_tw.transition('PLAN_GENERATION');
	`)
	if err != nil {
		t.Fatalf("wizard setup: %v", err)
	}

	// Simulate plan arrival and transition to PLAN_REVIEW.
	_, err = tp.EvalJS(`
		// Simulate: plan generated, now ready for review.
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go'], message: 'Add run' }
			]
		};
		_tw.transition('PLAN_REVIEW');
	`)
	if err != nil {
		t.Fatalf("plan setup: %v", err)
	}

	// --- Reject plan (regenerate) ---
	rejectResult, err := tp.EvalJS(`JSON.stringify(prSplit._handlePlanReviewState(_tw, 'regenerate', {feedback: 'split by functionality instead'}))`)
	if err != nil {
		t.Fatalf("plan reject: %v", err)
	}
	var rr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(rejectResult.(string)), &rr); err != nil {
		t.Fatalf("parse reject result: %v", err)
	}
	if rr.Action != "regenerate" {
		t.Errorf("reject action = %q, want regenerate", rr.Action)
	}
	if rr.State != "PLAN_GENERATION" {
		t.Errorf("reject state = %q, want PLAN_GENERATION", rr.State)
	}

	// Verify feedback was stored in wizard data.
	feedback, err := tp.EvalJS(`_tw.data.feedback || ''`)
	if err != nil {
		t.Fatalf("read feedback: %v", err)
	}
	if fb, ok := feedback.(string); !ok || fb != "split by functionality instead" {
		t.Errorf("feedback = %v, want 'split by functionality instead'", feedback)
	}

	// --- New plan arrives, transition to PLAN_REVIEW again ---
	_, err = tp.EvalJS(`
		// Same plan structure, simulate "new" plan.
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go'], message: 'Add run' }
			]
		};
		_tw.transition('PLAN_REVIEW');
	`)
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}

	// --- Approve plan ---
	approveResult, err := tp.EvalJS(`JSON.stringify(prSplit._handlePlanReviewState(_tw, 'approve'))`)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	var ar struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(approveResult.(string)), &ar); err != nil {
		t.Fatalf("parse approve: %v", err)
	}
	if ar.Action != "approve" {
		t.Errorf("approve action = %q, want approve", ar.Action)
	}
	if ar.State != "BRANCH_BUILDING" {
		t.Errorf("approve state = %q, want BRANCH_BUILDING", ar.State)
	}

	// --- Execute branch building (real git operations) ---
	buildResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("branch building: %v", err)
	}
	var br struct {
		Action  string `json:"action"`
		State   string `json:"state"`
		Error   string `json:"error"`
		Results []struct {
			Name         string `json:"name"`
			VerifyPassed bool   `json:"verifyPassed"`
		} `json:"results"`
		FailedBranches []any `json:"failedBranches"`
	}
	if err := json.Unmarshal([]byte(buildResult.(string)), &br); err != nil {
		t.Fatalf("parse build result: %v", err)
	}
	if br.Error != "" {
		t.Fatalf("branch building error: %s", br.Error)
	}
	if br.Action != "success" {
		t.Errorf("build action = %q, want success", br.Action)
	}
	if br.State != "EQUIV_CHECK" {
		t.Errorf("build state = %q, want EQUIV_CHECK", br.State)
	}
	if len(br.Results) != 2 {
		t.Fatalf("expected 2 branch results, got %d", len(br.Results))
	}
	for _, r := range br.Results {
		if !r.VerifyPassed {
			t.Errorf("branch %q verify failed", r.Name)
		}
	}
	if len(br.FailedBranches) != 0 {
		t.Errorf("expected 0 failed branches, got %d", len(br.FailedBranches))
	}

	// --- Equivalence check ---
	equivResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleEquivCheckState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("equiv check: %v", err)
	}
	var er struct {
		Action      string `json:"action"`
		State       string `json:"state"`
		Error       string `json:"error"`
		Equivalence struct {
			TreeMatch bool `json:"treeMatch"`
		} `json:"equivalence"`
	}
	if err := json.Unmarshal([]byte(equivResult.(string)), &er); err != nil {
		t.Fatalf("parse equiv: %v", err)
	}
	if er.Error != "" {
		t.Fatalf("equiv check error: %s", er.Error)
	}
	if er.Action != "checked" {
		t.Errorf("equiv action = %q, want checked", er.Action)
	}
	if er.State != "FINALIZATION" {
		t.Errorf("equiv state = %q, want FINALIZATION", er.State)
	}

	// --- Finalization: done ---
	finalResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleFinalizationState(_tw, 'done'))`)
	if err != nil {
		t.Fatalf("finalization: %v", err)
	}
	var fr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(finalResult.(string)), &fr); err != nil {
		t.Fatalf("parse finalization: %v", err)
	}
	if fr.Action != "done" {
		t.Errorf("final action = %q, want done", fr.Action)
	}
	if fr.State != "DONE" {
		t.Errorf("final state = %q, want DONE", fr.State)
	}

	// --- Verify wizard history ---
	// CONFIG → PLAN_GEN → PLAN_REVIEW → PLAN_GEN (regenerate) → PLAN_REVIEW
	// → BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION → DONE = 9 transitions
	histLen, err := tp.EvalJS(`_tw.history.length`)
	if err != nil {
		t.Fatalf("history length: %v", err)
	}
	if hl, ok := histLen.(int64); ok {
		if hl != 9 {
			t.Errorf("wizard history length = %d, want 9", hl)
		}
	} else if f, ok := histLen.(float64); ok {
		if int(f) != 9 {
			t.Errorf("wizard history length = %v, want 9", f)
		}
	}
}

// TestIntegration_WizardHandlerChain_BranchFailSkip tests the wizard flow
// when branch verification fails and the user chooses to skip:
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING → ERROR_RESOLUTION
// (skip) → EQUIV_CHECK → FINALIZATION → DONE.
//
// Uses a verifyCommand that fails for branches containing a specific file.
func TestIntegration_WizardHandlerChain_BranchFailSkip(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	// verify-ok exists only in split/01-pkg. split/02-cmd doesn't have it → fails.
	// NOTE: executeSplit chains branches (split/02 branches from split/01),
	// so files from split/01 appear on split/02. Use a FAIL_MARKER file in
	// split/02 and a negative check to trigger failure only on split/02.
	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{"FAIL_MARKER", "this file triggers verify failure"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true", // baseline uses 'true'
			"strategy":      "directory",
		},
	})

	// Create wizard and go through CONFIG. Use verifyCommand 'true' for
	// baseline (passes on main). Override plan verifyCommand to trigger
	// failure for split/02-cmd.
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		var _cfgResult = prSplit._handleConfigState({
			baseBranch: 'main',
			strategy: 'directory',
			verifyCommand: 'true'
		});
		if (_cfgResult.error) throw new Error('config failed: ' + _cfgResult.error);
		_tw.transition('PLAN_GENERATION');

		// Set plan: verifyCommand fails if FAIL_MARKER exists.
		// split/01-pkg does NOT have FAIL_MARKER → passes.
		// split/02-cmd has FAIL_MARKER → fails.
		// NOTE: branches chain (02 from 01), but FAIL_MARKER is only added
		// in split/02-cmd's commit, not in split/01-pkg.
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'test ! -f FAIL_MARKER',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A',
				'FAIL_MARKER': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go', 'FAIL_MARKER'], message: 'Add run and fail marker' }
			]
		};
		_tw.transition('PLAN_REVIEW');
		prSplit._handlePlanReviewState(_tw, 'approve');
	`)
	if err != nil {
		t.Fatalf("wizard setup: %v", err)
	}

	// --- BRANCH_BUILDING: one branch should fail verification ---
	buildResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("branch building: %v", err)
	}
	var br struct {
		Action  string `json:"action"`
		State   string `json:"state"`
		Results []struct {
			Name         string `json:"name"`
			VerifyPassed bool   `json:"verifyPassed"`
		} `json:"results"`
		FailedBranches []struct {
			Name string `json:"name"`
		} `json:"failedBranches"`
	}
	if err := json.Unmarshal([]byte(buildResult.(string)), &br); err != nil {
		t.Fatalf("parse build: %v", err)
	}

	// Should have entered ERROR_RESOLUTION.
	if br.Action != "failed" {
		t.Errorf("build action = %q, want failed", br.Action)
	}
	if br.State != "ERROR_RESOLUTION" {
		t.Errorf("build state = %q, want ERROR_RESOLUTION", br.State)
	}
	if len(br.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(br.Results))
	}
	if len(br.FailedBranches) != 1 {
		t.Fatalf("expected 1 failed branch, got %d", len(br.FailedBranches))
	}
	if br.FailedBranches[0].Name != "split/02-cmd" {
		t.Errorf("failed branch = %q, want split/02-cmd", br.FailedBranches[0].Name)
	}

	// split/01-pkg should have passed.
	if !br.Results[0].VerifyPassed {
		t.Errorf("split/01-pkg should have passed verification")
	}
	// split/02-cmd should have failed.
	if br.Results[1].VerifyPassed {
		t.Errorf("split/02-cmd should have failed verification")
	}

	// --- ERROR_RESOLUTION: skip failed branches ---
	skipResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleErrorResolutionState(_tw, 'skip'))`)
	if err != nil {
		t.Fatalf("error resolution: %v", err)
	}
	var sr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(skipResult.(string)), &sr); err != nil {
		t.Fatalf("parse skip: %v", err)
	}
	if sr.Action != "skip" {
		t.Errorf("skip action = %q, want skip", sr.Action)
	}
	if sr.State != "EQUIV_CHECK" {
		t.Errorf("skip state = %q, want EQUIV_CHECK", sr.State)
	}

	// --- EQUIV_CHECK ---
	equivResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleEquivCheckState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("equiv check: %v", err)
	}
	var er struct {
		Action string `json:"action"`
		State  string `json:"state"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(equivResult.(string)), &er); err != nil {
		t.Fatalf("parse equiv: %v", err)
	}
	if er.Error != "" {
		t.Fatalf("equiv error: %s", er.Error)
	}
	if er.State != "FINALIZATION" {
		t.Errorf("equiv state = %q, want FINALIZATION", er.State)
	}

	// --- FINALIZATION: done ---
	finalResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleFinalizationState(_tw, 'done'))`)
	if err != nil {
		t.Fatalf("finalization: %v", err)
	}
	var fr struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(finalResult.(string)), &fr); err != nil {
		t.Fatalf("parse final: %v", err)
	}
	if fr.State != "DONE" {
		t.Errorf("final state = %q, want DONE", fr.State)
	}

	// --- Verify wizard history covers the full error-recovery flow ---
	// CONFIG → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → ERROR_RESOLUTION
	// → EQUIV_CHECK → FINALIZATION → DONE = 8 transitions
	histLen, err := tp.EvalJS(`_tw.history.length`)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if hl, ok := histLen.(int64); ok {
		if hl != 8 {
			t.Errorf("wizard history = %d, want 8", hl)
		}
	} else if f, ok := histLen.(float64); ok {
		if int(f) != 8 {
			t.Errorf("wizard history = %v, want 8", f)
		}
	}
}

// ---------------------------------------------------------------------------
// T21: Additional wizard flow-path integration tests
// ---------------------------------------------------------------------------

// assertHistoryLen is a test helper that checks wizard history length,
// accepting both int64 and float64 from Goja.
func assertHistoryLen(t *testing.T, evalJS func(string) (any, error), want int) {
	t.Helper()
	histLen, err := evalJS(`_tw.history.length`)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	switch v := histLen.(type) {
	case int64:
		if int(v) != want {
			t.Errorf("wizard history length = %d, want %d", v, want)
		}
	case float64:
		if int(v) != want {
			t.Errorf("wizard history length = %v, want %d", v, want)
		}
	default:
		t.Errorf("unexpected history type %T = %v, want %d", histLen, histLen, want)
	}
}

// TestIntegration_WizardHandlerChain_HappyPath tests the clean-path wizard
// flow with no errors, rejections, or edits:
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING → EQUIV_CHECK →
// FINALIZATION → DONE.
//
// All branches pass verification. Equivalence check passes.
func TestIntegration_WizardHandlerChain_HappyPath(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// --- CONFIG → PLAN_GENERATION ---
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		var _cfgResult = prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'true'
		});
		if (_cfgResult.error) throw new Error('config: ' + _cfgResult.error);
		_tw.transition('PLAN_GENERATION');
	`)
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	// --- PLAN_GENERATION → PLAN_REVIEW (inject plan) ---
	_, err = tp.EvalJS(`
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go'], message: 'Add run' }
			]
		};
		_tw.transition('PLAN_REVIEW');
	`)
	if err != nil {
		t.Fatalf("plan setup: %v", err)
	}

	// --- PLAN_REVIEW → BRANCH_BUILDING (approve) ---
	approveResult, err := tp.EvalJS(`JSON.stringify(prSplit._handlePlanReviewState(_tw, 'approve'))`)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	var ar struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(approveResult.(string)), &ar); err != nil {
		t.Fatalf("parse approve: %v", err)
	}
	if ar.Action != "approve" || ar.State != "BRANCH_BUILDING" {
		t.Fatalf("approve: action=%q state=%q, want approve/BRANCH_BUILDING", ar.Action, ar.State)
	}

	// --- BRANCH_BUILDING → EQUIV_CHECK (all pass) ---
	buildResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var br struct {
		Action         string `json:"action"`
		State          string `json:"state"`
		FailedBranches []any  `json:"failedBranches"`
		Results        []struct {
			Name         string `json:"name"`
			VerifyPassed bool   `json:"verifyPassed"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(buildResult.(string)), &br); err != nil {
		t.Fatalf("parse build: %v", err)
	}
	if br.Action != "success" {
		t.Errorf("build action = %q, want success", br.Action)
	}
	if br.State != "EQUIV_CHECK" {
		t.Errorf("build state = %q, want EQUIV_CHECK", br.State)
	}
	if len(br.FailedBranches) != 0 {
		t.Errorf("failed branches = %d, want 0", len(br.FailedBranches))
	}
	if len(br.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(br.Results))
	}
	for _, r := range br.Results {
		if !r.VerifyPassed {
			t.Errorf("branch %q verify failed, want pass", r.Name)
		}
	}

	// --- EQUIV_CHECK → FINALIZATION ---
	equivResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleEquivCheckState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("equiv: %v", err)
	}
	var er struct {
		Action string `json:"action"`
		State  string `json:"state"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(equivResult.(string)), &er); err != nil {
		t.Fatalf("parse equiv: %v", err)
	}
	if er.Error != "" {
		t.Fatalf("equiv error: %s", er.Error)
	}
	if er.State != "FINALIZATION" {
		t.Errorf("equiv state = %q, want FINALIZATION", er.State)
	}

	// --- FINALIZATION → DONE ---
	finalResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleFinalizationState(_tw, 'done'))`)
	if err != nil {
		t.Fatalf("finalization: %v", err)
	}
	var fr struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal([]byte(finalResult.(string)), &fr); err != nil {
		t.Fatalf("parse final: %v", err)
	}
	if fr.State != "DONE" {
		t.Errorf("final state = %q, want DONE", fr.State)
	}

	// CONFIG → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → EQUIV_CHECK
	// → FINALIZATION → DONE = 7 transitions.
	assertHistoryLen(t, tp.EvalJS, 7)
}

// TestIntegration_WizardHandlerChain_PlanEditRoundtrip tests the plan-editor
// flow path: user edits the plan before approving it:
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → PLAN_EDITOR → PLAN_REVIEW →
// BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION → DONE.
//
// Verifies that plan modifications made in the editor persist through to
// branch building.
func TestIntegration_WizardHandlerChain_PlanEditRoundtrip(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// --- CONFIG → PLAN_GENERATION → PLAN_REVIEW ---
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		var _cfgResult = prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'true'
		});
		if (_cfgResult.error) throw new Error('config: ' + _cfgResult.error);
		_tw.transition('PLAN_GENERATION');
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A'
			},
			splits: [
				{ name: 'split/01-all', files: ['pkg/handler.go', 'cmd/run.go'], message: 'All changes' }
			]
		};
		_tw.transition('PLAN_REVIEW');
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// --- PLAN_REVIEW → PLAN_EDITOR (edit) ---
	editResult, err := tp.EvalJS(`JSON.stringify(prSplit._handlePlanReviewState(_tw, 'edit'))`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	var edr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(editResult.(string)), &edr); err != nil {
		t.Fatalf("parse edit: %v", err)
	}
	if edr.Action != "edit" || edr.State != "PLAN_EDITOR" {
		t.Fatalf("edit: action=%q state=%q, want edit/PLAN_EDITOR", edr.Action, edr.State)
	}

	// --- PLAN_EDITOR → PLAN_REVIEW (done with modified plan) ---
	// The user splits the single chunk into two chunks in the editor.
	editorDone, err := tp.EvalJS(`
		var _editedPlan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go'], message: 'Add run command' }
			]
		};
		JSON.stringify(prSplit._handlePlanEditorState(_tw, 'done', _editedPlan))
	`)
	if err != nil {
		t.Fatalf("editor done: %v", err)
	}
	var edd struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(editorDone.(string)), &edd); err != nil {
		t.Fatalf("parse editor: %v", err)
	}
	if edd.Action != "done" || edd.State != "PLAN_REVIEW" {
		t.Fatalf("editor: action=%q state=%q, want done/PLAN_REVIEW", edd.Action, edd.State)
	}

	// Verify the plan was updated — should now have 2 splits (edited).
	splitCount, err := tp.EvalJS(`_tw.data.plan.splits.length`)
	if err != nil {
		t.Fatalf("split count: %v", err)
	}
	switch v := splitCount.(type) {
	case int64:
		if v != 2 {
			t.Errorf("plan splits = %d after editor, want 2", v)
		}
	case float64:
		if v != 2 {
			t.Errorf("plan splits = %v after editor, want 2", v)
		}
	}

	// --- PLAN_REVIEW → BRANCH_BUILDING (approve the edited plan) ---
	_, err = tp.EvalJS(`prSplit._handlePlanReviewState(_tw, 'approve')`)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	// --- BRANCH_BUILDING with the edited 2-split plan ---
	buildResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var br struct {
		Action  string `json:"action"`
		State   string `json:"state"`
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(buildResult.(string)), &br); err != nil {
		t.Fatalf("parse build: %v", err)
	}
	if br.Action != "success" || br.State != "EQUIV_CHECK" {
		t.Fatalf("build: action=%q state=%q, want success/EQUIV_CHECK", br.Action, br.State)
	}
	// The edited plan's 2 splits should have been built.
	if len(br.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(br.Results))
	}
	if br.Results[0].Name != "split/01-pkg" {
		t.Errorf("result[0] name = %q, want split/01-pkg", br.Results[0].Name)
	}
	if br.Results[1].Name != "split/02-cmd" {
		t.Errorf("result[1] name = %q, want split/02-cmd", br.Results[1].Name)
	}

	// --- EQUIV_CHECK → FINALIZATION ---
	equivResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleEquivCheckState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("equiv: %v", err)
	}
	var eqr struct {
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(equivResult.(string)), &eqr); err != nil {
		t.Fatalf("parse equiv: %v", err)
	}
	if eqr.Error != "" {
		t.Fatalf("equiv error: %s", eqr.Error)
	}
	if eqr.State != "FINALIZATION" {
		t.Errorf("equiv state = %q, want FINALIZATION", eqr.State)
	}

	// --- FINALIZATION → DONE ---
	_, err = tp.EvalJS(`prSplit._handleFinalizationState(_tw, 'done')`)
	if err != nil {
		t.Fatalf("finalization: %v", err)
	}

	// CONFIG → PLAN_GEN → PLAN_REVIEW → PLAN_EDITOR → PLAN_REVIEW →
	// BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION → DONE = 9 transitions.
	assertHistoryLen(t, tp.EvalJS, 9)
}

// TestIntegration_WizardHandlerChain_ErrorRetry tests the error recovery
// flow via retry (regenerate plan):
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING (fail) →
// ERROR_RESOLUTION (retry) → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING
// (success) → EQUIV_CHECK → FINALIZATION → DONE.
//
// The first plan includes a file that causes verification to fail. After
// error-retry, a new plan without the problematic file succeeds.
func TestIntegration_WizardHandlerChain_ErrorRetry(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{"FAIL_MARKER", "this file triggers verify failure"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// --- CONFIG → PLAN_GENERATION → PLAN_REVIEW ---
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		var _cfgResult = prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'true'
		});
		if (_cfgResult.error) throw new Error('config: ' + _cfgResult.error);
		_tw.transition('PLAN_GENERATION');

		// First plan: split/02-cmd includes FAIL_MARKER, and verify
		// checks for its absence → failure.
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'test ! -f FAIL_MARKER',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A',
				'FAIL_MARKER': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go', 'FAIL_MARKER'], message: 'Add run + marker' }
			]
		};
		_tw.transition('PLAN_REVIEW');
		prSplit._handlePlanReviewState(_tw, 'approve');
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// --- BRANCH_BUILDING (fail) → ERROR_RESOLUTION ---
	buildResult1, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("build 1: %v", err)
	}
	var br1 struct {
		Action         string `json:"action"`
		State          string `json:"state"`
		FailedBranches []struct {
			Name string `json:"name"`
		} `json:"failedBranches"`
	}
	if err := json.Unmarshal([]byte(buildResult1.(string)), &br1); err != nil {
		t.Fatalf("parse build 1: %v", err)
	}
	if br1.Action != "failed" || br1.State != "ERROR_RESOLUTION" {
		t.Fatalf("build 1: action=%q state=%q, want failed/ERROR_RESOLUTION", br1.Action, br1.State)
	}
	if len(br1.FailedBranches) != 1 || br1.FailedBranches[0].Name != "split/02-cmd" {
		t.Fatalf("build 1 failed branches unexpected: %+v", br1.FailedBranches)
	}

	// --- ERROR_RESOLUTION (retry) → PLAN_GENERATION ---
	retryResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleErrorResolutionState(_tw, 'retry'))`)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	var rr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(retryResult.(string)), &rr); err != nil {
		t.Fatalf("parse retry: %v", err)
	}
	if rr.Action != "retry" || rr.State != "PLAN_GENERATION" {
		t.Fatalf("retry: action=%q state=%q, want retry/PLAN_GENERATION", rr.Action, rr.State)
	}

	// --- PLAN_GENERATION → PLAN_REVIEW with a fixed plan ---
	// Second plan: all files in one split, verify='true' (always passes).
	_, err = tp.EvalJS(`
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A',
				'FAIL_MARKER': 'A'
			},
			splits: [
				{ name: 'split/01-all', files: ['pkg/handler.go', 'cmd/run.go', 'FAIL_MARKER'], message: 'All changes' }
			]
		};
		_tw.transition('PLAN_REVIEW');
		prSplit._handlePlanReviewState(_tw, 'approve');
	`)
	if err != nil {
		t.Fatalf("second plan: %v", err)
	}

	// --- BRANCH_BUILDING (success) → EQUIV_CHECK ---
	buildResult2, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("build 2: %v", err)
	}
	var br2 struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(buildResult2.(string)), &br2); err != nil {
		t.Fatalf("parse build 2: %v", err)
	}
	if br2.Action != "success" || br2.State != "EQUIV_CHECK" {
		t.Fatalf("build 2: action=%q state=%q, want success/EQUIV_CHECK", br2.Action, br2.State)
	}

	// --- EQUIV_CHECK → FINALIZATION ---
	equivResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleEquivCheckState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("equiv: %v", err)
	}
	var eqr struct {
		State string `json:"state"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(equivResult.(string)), &eqr); err != nil {
		t.Fatalf("parse equiv: %v", err)
	}
	if eqr.Error != "" {
		t.Fatalf("equiv error: %s", eqr.Error)
	}
	if eqr.State != "FINALIZATION" {
		t.Errorf("equiv state = %q, want FINALIZATION", eqr.State)
	}

	// --- FINALIZATION → DONE ---
	_, err = tp.EvalJS(`prSplit._handleFinalizationState(_tw, 'done')`)
	if err != nil {
		t.Fatalf("finalization: %v", err)
	}

	// CONFIG → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → ERROR_RESOLUTION
	// → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → EQUIV_CHECK
	// → FINALIZATION → DONE = 11 transitions.
	assertHistoryLen(t, tp.EvalJS, 11)
}

// TestIntegration_WizardHandlerChain_ErrorAutoResolve tests the error
// recovery flow via auto-resolve (re-run branch building):
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING (fail) →
// ERROR_RESOLUTION (auto-resolve) → BRANCH_BUILDING (success) →
// EQUIV_CHECK → FINALIZATION → DONE.
//
// Uses an external flag file to make the verify command fail on the first
// build but succeed on the second (after the flag is removed).
func TestIntegration_WizardHandlerChain_ErrorAutoResolve(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// Create an external flag file that the verify command will check.
	// On first build: flag present → verify fails.
	// After auto-resolve: flag removed → verify passes.
	flagFile := filepath.Join(filepath.Dir(tp.Dir), "auto-resolve-fail-flag")
	if err := os.WriteFile(flagFile, []byte("fail"), 0o644); err != nil {
		t.Fatalf("write flag: %v", err)
	}
	t.Cleanup(func() { os.Remove(flagFile) })

	verifyCmd := `test ! -f ` + flagFile

	// --- CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING ---
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'true'
		});
		_tw.transition('PLAN_GENERATION');
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: '` + verifyCmd + `',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go'], message: 'Add run' }
			]
		};
		_tw.transition('PLAN_REVIEW');
		prSplit._handlePlanReviewState(_tw, 'approve');
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// --- BRANCH_BUILDING (fail — flag file exists) → ERROR_RESOLUTION ---
	buildResult1, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("build 1: %v", err)
	}
	var br1 struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(buildResult1.(string)), &br1); err != nil {
		t.Fatalf("parse build 1: %v", err)
	}
	if br1.Action != "failed" || br1.State != "ERROR_RESOLUTION" {
		t.Fatalf("build 1: action=%q state=%q, want failed/ERROR_RESOLUTION", br1.Action, br1.State)
	}

	// --- ERROR_RESOLUTION (auto-resolve) → BRANCH_BUILDING ---
	autoResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleErrorResolutionState(_tw, 'auto-resolve'))`)
	if err != nil {
		t.Fatalf("auto-resolve: %v", err)
	}
	var aor struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(autoResult.(string)), &aor); err != nil {
		t.Fatalf("parse auto-resolve: %v", err)
	}
	if aor.Action != "auto-resolve" || aor.State != "BRANCH_BUILDING" {
		t.Fatalf("auto-resolve: action=%q state=%q, want auto-resolve/BRANCH_BUILDING", aor.Action, aor.State)
	}

	// Remove the flag file so branches pass verification this time.
	if err := os.Remove(flagFile); err != nil {
		t.Fatalf("remove flag: %v", err)
	}

	// --- BRANCH_BUILDING (success) → EQUIV_CHECK ---
	buildResult2, err := tp.EvalJS(`JSON.stringify(prSplit._handleBranchBuildingState(_tw, _tw.data.plan))`)
	if err != nil {
		t.Fatalf("build 2: %v", err)
	}
	var br2 struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(buildResult2.(string)), &br2); err != nil {
		t.Fatalf("parse build 2: %v", err)
	}
	if br2.Action != "success" || br2.State != "EQUIV_CHECK" {
		t.Fatalf("build 2: action=%q state=%q, want success/EQUIV_CHECK", br2.Action, br2.State)
	}

	// --- EQUIV_CHECK → FINALIZATION → DONE ---
	_, err = tp.EvalJS(`
		prSplit._handleEquivCheckState(_tw, _tw.data.plan);
		prSplit._handleFinalizationState(_tw, 'done');
	`)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}

	// CONFIG → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → ERROR_RESOLUTION
	// → BRANCH_BUILDING → EQUIV_CHECK → FINALIZATION → DONE = 9 transitions.
	assertHistoryLen(t, tp.EvalJS, 9)
}

// TestIntegration_WizardCancelFromAllStates verifies that wizard.cancel()
// works from every applicable state and that the CANCELLED state is recorded
// correctly in the wizard history.
//
// States tested: CONFIG, BASELINE_FAIL, PLAN_GENERATION, PLAN_REVIEW,
// BRANCH_BUILDING, ERROR_RESOLUTION, EQUIV_CHECK.
func TestIntegration_WizardCancelFromAllStates(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	// Create a shared test pipeline for real git operations.
	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// Each subtest creates a fresh WizardState and cancels from its target.
	// All use the same JS engine but separate wizard instances.

	t.Run("CancelFromCONFIG", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		// IDLE → CONFIG → CANCELLED = 2 transitions.
		if r.HistLen != 2 {
			t.Errorf("history = %d, want 2", r.HistLen)
		}
	})

	t.Run("CancelFromBASELINE_FAIL", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('BASELINE_FAIL');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		// IDLE → CONFIG → BASELINE_FAIL → CANCELLED = 3 transitions.
		if r.HistLen != 3 {
			t.Errorf("history = %d, want 3", r.HistLen)
		}
	})

	t.Run("CancelFromPLAN_GENERATION", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		if r.HistLen != 3 {
			t.Errorf("history = %d, want 3", r.HistLen)
		}
	})

	t.Run("CancelFromPLAN_REVIEW", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		// IDLE → CONFIG → PLAN_GEN → PLAN_REVIEW → CANCELLED = 4.
		if r.HistLen != 4 {
			t.Errorf("history = %d, want 4", r.HistLen)
		}
	})

	t.Run("CancelFromBRANCH_BUILDING", func(t *testing.T) {
		// BRANCH_BUILDING is reachable from PLAN_REVIEW or CONFIG
		// (direct branch building). Use PLAN_REVIEW → approve.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.transition('BRANCH_BUILDING');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		// 5 transitions.
		if r.HistLen != 5 {
			t.Errorf("history = %d, want 5", r.HistLen)
		}
	})

	t.Run("CancelFromERROR_RESOLUTION", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.transition('BRANCH_BUILDING');
				w.transition('ERROR_RESOLUTION');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		// 6 transitions.
		if r.HistLen != 6 {
			t.Errorf("history = %d, want 6", r.HistLen)
		}
	})

	t.Run("CancelFromEQUIV_CHECK", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.transition('BRANCH_BUILDING');
				w.transition('EQUIV_CHECK');
				w.cancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		// 6 transitions.
		if r.HistLen != 6 {
			t.Errorf("history = %d, want 6", r.HistLen)
		}
	})

	t.Run("CancelIdempotentFromTerminal", func(t *testing.T) {
		// Calling cancel() from a terminal state should be a no-op.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.cancel();
				var lenBefore = w.history.length;
				w.cancel(); // second cancel — should be no-op
				return JSON.stringify({
					current: w.current,
					histLen: w.history.length,
					idempotent: w.history.length === lenBefore
				});
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current    string `json:"current"`
			HistLen    int    `json:"histLen"`
			Idempotent bool   `json:"idempotent"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CANCELLED" {
			t.Errorf("state = %q, want CANCELLED", r.Current)
		}
		if !r.Idempotent {
			t.Errorf("cancel from terminal added extra history (len=%d)", r.HistLen)
		}
	})
}

// TestIntegration_WizardForceCancel verifies forceCancel() bypasses the
// transition matrix and works from any non-terminal state, including states
// that cannot normally reach CANCELLED through regular transitions.
func TestIntegration_WizardForceCancel(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix": "split/",
			"strategy":     "directory",
		},
	})

	t.Run("ForceCancelFromPLAN_EDITOR", func(t *testing.T) {
		// PLAN_EDITOR has no normal transition to CANCELLED.
		// forceCancel should bypass the transition matrix.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.transition('PLAN_EDITOR');
				w.forceCancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "FORCE_CANCEL" {
			t.Errorf("state = %q, want FORCE_CANCEL", r.Current)
		}
		// IDLE → CONFIG → PLAN_GEN → PLAN_REVIEW → PLAN_EDITOR → FORCE_CANCEL = 5.
		if r.HistLen != 5 {
			t.Errorf("history = %d, want 5", r.HistLen)
		}
	})

	t.Run("ForceCancelFromFINALIZATION", func(t *testing.T) {
		// FINALIZATION has no transition to CANCELLED either.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.transition('BRANCH_BUILDING');
				w.transition('EQUIV_CHECK');
				w.transition('FINALIZATION');
				w.forceCancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "FORCE_CANCEL" {
			t.Errorf("state = %q, want FORCE_CANCEL", r.Current)
		}
		// 7 transitions including force cancel.
		if r.HistLen != 7 {
			t.Errorf("history = %d, want 7", r.HistLen)
		}
	})

	t.Run("ForceCancelFromCANCELLED", func(t *testing.T) {
		// forceCancel should work even from the CANCELLED state.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.cancel();
				w.forceCancel();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "FORCE_CANCEL" {
			t.Errorf("state = %q, want FORCE_CANCEL", r.Current)
		}
		// IDLE → CONFIG → CANCELLED → FORCE_CANCEL = 3.
		if r.HistLen != 3 {
			t.Errorf("history = %d, want 3", r.HistLen)
		}
	})

	t.Run("ForceCancelNoopFromDONE", func(t *testing.T) {
		// forceCancel from DONE should be a no-op.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.cancel();
				w.transition('DONE');
				var lenBefore = w.history.length;
				w.forceCancel();
				return JSON.stringify({
					current: w.current,
					histLen: w.history.length,
					noop: w.history.length === lenBefore
				});
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
			Noop    bool   `json:"noop"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "DONE" {
			t.Errorf("state = %q, want DONE", r.Current)
		}
		if !r.Noop {
			t.Errorf("forceCancel from DONE should be no-op (len=%d)", r.HistLen)
		}
	})
}

// TestIntegration_WizardHandlerChain_BaselineFailAbort tests the abort
// path from BASELINE_FAIL state using the handler function:
// CONFIG → BASELINE_FAIL → abort → CANCELLED.
func TestIntegration_WizardHandlerChain_BaselineFailAbort(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "test -f .nonexistent-file",
			"strategy":      "directory",
		},
	})

	// CONFIG: get baseline verify config (T090: verify deferred to async).
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		var _cfgResult = prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'test -f .nonexistent-file'
		});
		// T090: handleConfigState returns baselineVerifyConfig, not verify result.
		// Perform deferred baseline verify using the config.
		var _bvc = _cfgResult.baselineVerifyConfig;
		var _verifyResult = prSplit.verifySplit(prSplit.runtime.baseBranch, _bvc);
		// Baseline should fail — .nonexistent-file doesn't exist.
		if (_verifyResult.passed) throw new Error('expected baseline failure');
		_tw.transition('BASELINE_FAIL');
	`)
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	// BASELINE_FAIL → abort → CANCELLED.
	abortResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleBaselineFailState(_tw, 'abort'))`)
	if err != nil {
		t.Fatalf("abort: %v", err)
	}
	var ar struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(abortResult.(string)), &ar); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ar.Action != "abort" || ar.State != "CANCELLED" {
		t.Fatalf("abort: action=%q state=%q, want abort/CANCELLED", ar.Action, ar.State)
	}

	// CONFIG → BASELINE_FAIL → CANCELLED = 3 transitions.
	assertHistoryLen(t, tp.EvalJS, 3)
}

// TestIntegration_WizardHandlerChain_FinalizationReport tests the report
// and create-prs paths in FINALIZATION (self-transitions):
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING → EQUIV_CHECK →
// FINALIZATION → report (stays) → create-prs (self-transition) → done → DONE.
func TestIntegration_WizardHandlerChain_FinalizationReport(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// Drive through to FINALIZATION.
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'true'
		});
		_tw.transition('PLAN_GENERATION');
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'true',
			fileStatuses: { 'pkg/handler.go': 'A' },
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' }
			]
		};
		_tw.transition('PLAN_REVIEW');
		prSplit._handlePlanReviewState(_tw, 'approve');
		prSplit._handleBranchBuildingState(_tw, _tw.data.plan);
		prSplit._handleEquivCheckState(_tw, _tw.data.plan);
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify we're in FINALIZATION.
	state, err := tp.EvalJS(`_tw.current`)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if state != "FINALIZATION" {
		t.Fatalf("state = %q, want FINALIZATION", state)
	}

	// --- Report: stays in FINALIZATION, no history change ---
	reportResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleFinalizationState(_tw, 'report'))`)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	var rr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(reportResult.(string)), &rr); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if rr.Action != "report" || rr.State != "FINALIZATION" {
		t.Fatalf("report: action=%q state=%q, want report/FINALIZATION", rr.Action, rr.State)
	}

	histBefore, err := tp.EvalJS(`_tw.history.length`)
	if err != nil {
		t.Fatalf("history before: %v", err)
	}

	// --- Create PRs: FINALIZATION → FINALIZATION self-transition ---
	prsResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleFinalizationState(_tw, 'create-prs'))`)
	if err != nil {
		t.Fatalf("create-prs: %v", err)
	}
	var pr struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(prsResult.(string)), &pr); err != nil {
		t.Fatalf("parse prs: %v", err)
	}
	if pr.Action != "create-prs" || pr.State != "FINALIZATION" {
		t.Fatalf("create-prs: action=%q state=%q, want create-prs/FINALIZATION", pr.Action, pr.State)
	}

	// create-prs does a self-transition → history should increment by 1.
	histAfter, err := tp.EvalJS(`_tw.history.length`)
	if err != nil {
		t.Fatalf("history after: %v", err)
	}
	var before, after int
	switch v := histBefore.(type) {
	case int64:
		before = int(v)
	case float64:
		before = int(v)
	}
	switch v := histAfter.(type) {
	case int64:
		after = int(v)
	case float64:
		after = int(v)
	}
	if after != before+1 {
		t.Errorf("create-prs history: before=%d after=%d, want +1", before, after)
	}

	// Verify prsRequested flag was set.
	prsReq, err := tp.EvalJS(`_tw.data.prsRequested`)
	if err != nil {
		t.Fatalf("prsRequested: %v", err)
	}
	if prsReq != true {
		t.Errorf("prsRequested = %v, want true", prsReq)
	}

	// --- Done: FINALIZATION → DONE ---
	_, err = tp.EvalJS(`prSplit._handleFinalizationState(_tw, 'done')`)
	if err != nil {
		t.Fatalf("done: %v", err)
	}

	// CONFIG → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → EQUIV_CHECK →
	// FINALIZATION → FINALIZATION (self) → DONE = 8 transitions.
	// (report action does NOT create a transition.)
	assertHistoryLen(t, tp.EvalJS, 8)
}

// TestIntegration_WizardHandlerChain_ErrorAbort tests the abort path from
// ERROR_RESOLUTION via the handler function (not wizard.cancel() directly):
// CONFIG → PLAN_GENERATION → PLAN_REVIEW → BRANCH_BUILDING (fail) →
// ERROR_RESOLUTION (abort) → CANCELLED.
func TestIntegration_WizardHandlerChain_ErrorAbort(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{"FAIL_MARKER", "triggers failure"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	// Setup: drive to ERROR_RESOLUTION via failed verification.
	_, err := tp.EvalJS(`
		globalThis._tw = new prSplit.WizardState();
		_tw.transition('CONFIG');
		prSplit._handleConfigState({
			baseBranch: 'main',
			strategy:   'directory',
			verifyCommand: 'true'
		});
		_tw.transition('PLAN_GENERATION');
		_tw.data.plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '` + tp.Dir + `',
			verifyCommand: 'test ! -f FAIL_MARKER',
			fileStatuses: {
				'pkg/handler.go': 'A',
				'cmd/run.go': 'A',
				'FAIL_MARKER': 'A'
			},
			splits: [
				{ name: 'split/01-pkg', files: ['pkg/handler.go'], message: 'Add handler' },
				{ name: 'split/02-cmd', files: ['cmd/run.go', 'FAIL_MARKER'], message: 'Add run + marker' }
			]
		};
		_tw.transition('PLAN_REVIEW');
		prSplit._handlePlanReviewState(_tw, 'approve');
		prSplit._handleBranchBuildingState(_tw, _tw.data.plan);
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Verify we're in ERROR_RESOLUTION.
	state, err := tp.EvalJS(`_tw.current`)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if state != "ERROR_RESOLUTION" {
		t.Fatalf("state = %q, want ERROR_RESOLUTION", state)
	}

	// --- ERROR_RESOLUTION (abort) → CANCELLED ---
	abortResult, err := tp.EvalJS(`JSON.stringify(prSplit._handleErrorResolutionState(_tw, 'abort'))`)
	if err != nil {
		t.Fatalf("abort: %v", err)
	}
	var ar struct {
		Action string `json:"action"`
		State  string `json:"state"`
	}
	if err := json.Unmarshal([]byte(abortResult.(string)), &ar); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ar.Action != "abort" || ar.State != "CANCELLED" {
		t.Fatalf("abort: action=%q state=%q, want abort/CANCELLED", ar.Action, ar.State)
	}

	// CONFIG → PLAN_GEN → PLAN_REVIEW → BRANCH_BUILDING → ERROR_RESOLUTION
	// → CANCELLED = 6 transitions.
	assertHistoryLen(t, tp.EvalJS, 6)
}

// TestIntegration_WizardTransitionListener verifies that wizard state
// transition listeners fire correctly and receive the expected arguments.
func TestIntegration_WizardTransitionListener(t *testing.T) {
	skipSlow(t)
	// NOT parallel — uses chdirTestPipeline (os.Chdir is process-global).

	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix": "split/",
			"strategy":     "directory",
		},
	})

	result, err := tp.EvalJS(`
		(function() {
			var transitions = [];
			var w = new prSplit.WizardState();
			w.onTransition(function(from, to, data) {
				transitions.push({ from: from, to: to });
			});
			w.transition('CONFIG');
			w.transition('PLAN_GENERATION');
			w.transition('PLAN_REVIEW');
			w.cancel();
			return JSON.stringify(transitions);
		})()
	`)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}

	var transitions []struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &transitions); err != nil {
		t.Fatalf("parse: %v", err)
	}

	expected := []struct {
		From string
		To   string
	}{
		{"IDLE", "CONFIG"},
		{"CONFIG", "PLAN_GENERATION"},
		{"PLAN_GENERATION", "PLAN_REVIEW"},
		{"PLAN_REVIEW", "CANCELLED"},
	}

	if len(transitions) != len(expected) {
		t.Fatalf("listener received %d transitions, want %d:\n%+v", len(transitions), len(expected), transitions)
	}
	for i, e := range expected {
		if transitions[i].From != e.From || transitions[i].To != e.To {
			t.Errorf("transition[%d] = %s→%s, want %s→%s", i, transitions[i].From, transitions[i].To, e.From, e.To)
		}
	}
}

// TestIntegration_WizardPauseResume verifies the pause() and resume mechanics.
// pause() only applies from PAUSABLE_STATES (PLAN_GENERATION, BRANCH_BUILDING).
func TestIntegration_WizardPauseResume(t *testing.T) {
	skipSlow(t)
	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix": "split/",
			"strategy":     "directory",
		},
	})

	t.Run("PauseFromPLAN_GENERATION", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.pause();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "PAUSED" {
			t.Errorf("state = %q, want PAUSED", r.Current)
		}
		// IDLE → CONFIG → PLAN_GEN → PAUSED = 3.
		if r.HistLen != 3 {
			t.Errorf("history = %d, want 3", r.HistLen)
		}
	})

	t.Run("PauseFromBRANCH_BUILDING", func(t *testing.T) {
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				w.transition('PLAN_GENERATION');
				w.transition('PLAN_REVIEW');
				w.transition('BRANCH_BUILDING');
				w.pause();
				return JSON.stringify({ current: w.current, histLen: w.history.length });
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			HistLen int    `json:"histLen"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "PAUSED" {
			t.Errorf("state = %q, want PAUSED", r.Current)
		}
	})

	t.Run("PauseNoopFromNonPausable", func(t *testing.T) {
		// pause() from a non-pausable state should be a no-op.
		result, err := tp.EvalJS(`
			(function() {
				var w = new prSplit.WizardState();
				w.transition('CONFIG');
				var before = w.history.length;
				w.pause();
				return JSON.stringify({
					current: w.current,
					noop: w.history.length === before
				});
			})()
		`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		var r struct {
			Current string `json:"current"`
			Noop    bool   `json:"noop"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &r); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if r.Current != "CONFIG" {
			t.Errorf("state = %q, want CONFIG (unchanged)", r.Current)
		}
		if !r.Noop {
			t.Error("pause from non-pausable state should be no-op")
		}
	})
}

// TestIntegration_WizardInvalidTransitions verifies that invalid state
// transitions throw errors and leave the wizard state unchanged.
func TestIntegration_WizardInvalidTransitions(t *testing.T) {
	skipSlow(t)
	tp := chdirTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Config struct{ Name string }\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/handler.go", "package pkg\n\nfunc Handle() string { return \"ok\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"branchPrefix": "split/",
			"strategy":     "directory",
		},
	})

	// Each case attempts an invalid transition and verifies the error.
	cases := []struct {
		name string
		js   string
	}{
		{
			name: "IDLE_to_PLAN_REVIEW",
			js:   `var w = new prSplit.WizardState(); w.transition('PLAN_REVIEW');`,
		},
		{
			name: "CONFIG_to_DONE",
			js:   `var w = new prSplit.WizardState(); w.transition('CONFIG'); w.transition('DONE');`,
		},
		{
			name: "PLAN_EDITOR_to_DONE",
			js: `var w = new prSplit.WizardState(); w.transition('CONFIG');
				w.transition('PLAN_GENERATION'); w.transition('PLAN_REVIEW');
				w.transition('PLAN_EDITOR'); w.transition('DONE');`,
		},
		{
			name: "DONE_to_CONFIG",
			js: `var w = new prSplit.WizardState(); w.transition('CONFIG');
				w.cancel(); w.transition('DONE'); w.transition('CONFIG');`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tp.EvalJS(tc.js)
			if err == nil {
				t.Error("expected error for invalid transition, got nil")
			}
			if !strings.Contains(err.Error(), "Invalid transition") {
				t.Errorf("error should mention 'Invalid transition': %v", err)
			}
		})
	}
}
