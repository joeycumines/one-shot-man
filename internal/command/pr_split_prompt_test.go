package command

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Tests for isCancelled, renderClassificationPrompt, renderSplitPlanPrompt,
// renderConflictPrompt, and heuristicFallback.
// These functions had zero or insufficient dedicated test coverage.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// isCancelled — returns true when prSplit._cancelSource() reports cancelled
// ---------------------------------------------------------------------------

func TestIsCancelled_NoCancelSource(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Without _cancelSource defined, isCancelled should return false.
	val, err := evalJS(`globalThis.prSplit.isCancelled()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("expected false when _cancelSource is not defined, got %v", val)
	}
}

func TestIsCancelled_NotCancelled(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Set _cancelSource returning false for all queries.
	_, err := evalJS(`globalThis.prSplit._cancelSource = function(q) { return false; }`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`globalThis.prSplit.isCancelled()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("expected false when _cancelSource reports not cancelled, got %v", val)
	}
}

func TestIsCancelled_Cancelled(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Set _cancelSource returning true for 'cancelled' query.
	_, err := evalJS(`globalThis.prSplit._cancelSource = function(q) { return q === 'cancelled'; }`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`globalThis.prSplit.isCancelled()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("expected true when _cancelSource reports cancelled, got %v", val)
	}
}

func TestIsCancelled_CancelSourceNotFunction(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Set _cancelSource to a non-function value.
	_, err := evalJS(`globalThis.prSplit._cancelSource = 'not-a-function'`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`globalThis.prSplit.isCancelled()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("expected false when _cancelSource is not a function, got %v", val)
	}
}

// ---------------------------------------------------------------------------
// renderClassificationPrompt — renders the classification prompt template
// ---------------------------------------------------------------------------

func TestRenderClassificationPrompt_BasicGo(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderClassificationPrompt(
		{
			baseBranch: 'main',
			currentBranch: 'feat',
			files: ['internal/foo/foo.go', 'internal/bar/bar.go'],
			fileStatuses: { 'internal/foo/foo.go': 'M', 'internal/bar/bar.go': 'A' }
		},
		{ maxGroups: 5 }
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	// Check that the rendered prompt contains key elements.
	if !strings.Contains(result.Text, "reportClassification") {
		t.Error("prompt should mention reportClassification MCP tool")
	}
	// T34: Session IDs removed from MCP payloads — routing by socket identity.
	if strings.Contains(result.Text, "session") {
		t.Error("prompt must NOT contain session ID references (routing is by socket)")
	}
	if !strings.Contains(result.Text, "internal/foo/foo.go") {
		t.Error("prompt should list the files")
	}
	// T33: Prompt must request new categories array format.
	if !strings.Contains(result.Text, "categories") {
		t.Error("prompt should mention 'categories' parameter for new array format")
	}
	if !strings.Contains(result.Text, "description") {
		t.Error("prompt should mention 'description' field (commit message)")
	}
	if !strings.Contains(result.Text, "Git commit message") {
		t.Error("prompt should describe description as 'Git commit message'")
	}
	// T34: Anti-slop directives.
	if !strings.Contains(result.Text, "Commit Message Requirements") {
		t.Error("prompt should include 'Commit Message Requirements' section")
	}
	if !strings.Contains(result.Text, "No placeholder messages") {
		t.Error("prompt should include anti-slop directive against placeholder messages")
	}
	if !strings.Contains(result.Text, "Be specific") {
		t.Error("prompt should include anti-slop directive to be specific")
	}
}

func TestRenderClassificationPrompt_EmptyFiles(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderClassificationPrompt(
		{ baseBranch: 'main', files: [], fileStatuses: {} },
		{}
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	// Should still render without error (template handles empty files).
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Text, "reportClassification") {
		t.Error("prompt should still mention reportClassification")
	}
}

// ---------------------------------------------------------------------------
// renderSplitPlanPrompt — renders the split plan prompt template
// ---------------------------------------------------------------------------

func TestRenderSplitPlanPrompt_Basic(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderSplitPlanPrompt(
		{ 'internal/foo/foo.go': 'infrastructure', 'internal/bar/bar.go': 'feature' },
		{ branchPrefix: 'split/', maxFilesPerSplit: 10 }
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Text, "reportSplitPlan") {
		t.Error("prompt should mention reportSplitPlan MCP tool")
	}
	// T34: Session IDs removed from MCP payloads.
	if strings.Contains(result.Text, "session") {
		t.Error("prompt must NOT contain session ID references")
	}
}

func TestRenderSplitPlanPrompt_DefaultConfig(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderSplitPlanPrompt(
		{ 'a.go': 'core' },
		{}
	))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Text, "reportSplitPlan") {
		t.Error("prompt should mention reportSplitPlan")
	}
}

// ---------------------------------------------------------------------------
// renderConflictPrompt — renders the conflict resolution prompt template
// ---------------------------------------------------------------------------

func TestRenderConflictPrompt_Basic(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderConflictPrompt({
		branchName: 'split/01-infrastructure',
		files: ['go.mod', 'go.sum'],
		exitCode: 2,
		errorOutput: 'missing module: github.com/example/lib',
		goModContent: 'module github.com/foo\n\ngo 1.21'
	}))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Text, "reportResolution") {
		t.Error("prompt should mention reportResolution MCP tool")
	}
	if !strings.Contains(result.Text, "preExistingFailure") {
		t.Error("prompt should mention preExistingFailure option")
	}
	if !strings.Contains(result.Text, "split/01-infrastructure") {
		t.Error("prompt should include branch name")
	}
	if !strings.Contains(result.Text, "missing module") {
		t.Error("prompt should include error output")
	}
}

func TestRenderConflictPrompt_MinimalInput(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(globalThis.prSplit.renderConflictPrompt({}))`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// Should render with empty defaults.
	if !strings.Contains(result.Text, "reportResolution") {
		t.Error("prompt should mention reportResolution even with minimal input")
	}
}

// ---------------------------------------------------------------------------
// heuristicFallback — local splitting without Claude
// ---------------------------------------------------------------------------

func TestHeuristicFallback_DirectoryStrategy(t *testing.T) {
	t.Parallel()

	// dryRun: true avoids any git operations — heuristicFallback only
	// builds groups+plan and returns immediately.
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]any{
		"dryRun": true,
	})

	val, err := evalJS(`
		(async function() {
			var analysis = {
				baseBranch: 'main',
				currentBranch: 'feature',
				files: [
					'internal/foo/foo.go',
					'internal/bar/bar.go',
					'cmd/main.go'
				],
				fileStatuses: {
					'internal/foo/foo.go': 'M',
					'internal/bar/bar.go': 'M',
					'cmd/main.go': 'A'
				}
			};
			var report = { plan: null, splits: [], error: null };
			var result = await globalThis.prSplit.heuristicFallback(analysis, {
				strategy: 'directory'
			}, report);

			return JSON.stringify(result);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Plan struct {
				Splits []struct {
					Name  string   `json:"name"`
					Files []string `json:"files"`
				} `json:"splits"`
			} `json:"plan"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.Error != nil {
		t.Fatalf("unexpected error: %s", *result.Error)
	}

	// With directory strategy, files in different top-level dirs should
	// produce separate splits (internal vs cmd).
	if len(result.Report.Plan.Splits) < 2 {
		t.Errorf("expected at least 2 splits for files in different directories, got %d",
			len(result.Report.Plan.Splits))
	}
}

// ---------------------------------------------------------------------------
// T93: heuristicFallback tree-hash-mismatch error path
// ---------------------------------------------------------------------------

func TestHeuristicFallback_TreeHashMismatch(t *testing.T) {
	// NOT parallel — OS state (chdir) is shared.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Override verifyEquivalenceAsync to always return a mismatch.
	if _, err := tp.EvalJS(`
		verifyEquivalenceAsync = function(plan) {
			return { equivalent: false, splitTree: 'aaa', sourceTree: 'bbb', error: null };
		};
	`); err != nil {
		t.Fatal(err)
	}

	val, err := tp.EvalJS(`(async function() {
		var analysis = globalThis.prSplit.analyzeDiff();
		var report = { plan: null, splits: [], error: null };
		var result = await globalThis.prSplit.heuristicFallback(analysis, {
			strategy: 'directory'
		}, report);
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatalf("heuristicFallback: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error string `json:"error"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	// The error should mention tree hash mismatch.
	if result.Error == nil {
		t.Fatal("expected error from tree hash mismatch")
	}
	if !strings.Contains(*result.Error, "tree hash mismatch") {
		t.Errorf("expected error containing 'tree hash mismatch', got: %s", *result.Error)
	}
}
