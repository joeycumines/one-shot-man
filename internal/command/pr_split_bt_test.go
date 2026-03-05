package command

// Tests for visualization, diff display, and report functions from the
// pr-split chunk files.
//
// T44: renderColorizedDiff, getSplitDiff, buildReport behavioral tests

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  T44: Behavioral Tests for renderColorizedDiff, getSplitDiff, buildReport
// ---------------------------------------------------------------------------

// TestRenderColorizedDiff_ContentPreserved verifies renderColorizedDiff
// produces output containing all input lines in order. Styling depends on
// terminal capability (lipgloss), so we verify content, not ANSI sequences.
func TestRenderColorizedDiff_ContentPreserved(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var diff = [
				'diff --git a/foo.go b/foo.go',
				'index abc123..def456 100644',
				'--- a/foo.go',
				'+++ b/foo.go',
				'@@ -1,3 +1,4 @@',
				' package foo',
				'-func old() {}',
				'+func new() {}',
				'+func extra() {}',
				' // end'
			].join('\n');
			var result = prSplit.renderColorizedDiff(diff);
			return JSON.stringify({
				hasAddLine: result.indexOf('+func new() {}') >= 0,
				hasRemoveLine: result.indexOf('-func old() {}') >= 0,
				hasHunkHeader: result.indexOf('@@ -1,3 +1,4 @@') >= 0,
				hasDiffHeader: result.indexOf('diff --git a/foo.go b/foo.go') >= 0,
				hasContext: result.indexOf('package foo') >= 0,
				lineCount: result.split('\n').length,
				notEmpty: result.length > 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		HasAddLine    bool `json:"hasAddLine"`
		HasRemoveLine bool `json:"hasRemoveLine"`
		HasHunkHeader bool `json:"hasHunkHeader"`
		HasDiffHeader bool `json:"hasDiffHeader"`
		HasContext     bool `json:"hasContext"`
		LineCount     int  `json:"lineCount"`
		NotEmpty      bool `json:"notEmpty"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasAddLine {
		t.Error("expected addition line content preserved")
	}
	if !result.HasRemoveLine {
		t.Error("expected removal line content preserved")
	}
	if !result.HasHunkHeader {
		t.Error("expected hunk header content preserved")
	}
	if !result.HasDiffHeader {
		t.Error("expected diff header content preserved")
	}
	if !result.HasContext {
		t.Error("expected context line content preserved")
	}
	if result.LineCount != 10 {
		t.Errorf("expected 10 lines, got %d", result.LineCount)
	}
}

// TestRenderColorizedDiff_EmptyInput verifies empty input returns empty string.
func TestRenderColorizedDiff_EmptyInput(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`prSplit.renderColorizedDiff('')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty string for empty input, got %q", val)
	}
}

// TestGetSplitDiff_InvalidIndex verifies getSplitDiff returns error for
// out-of-bounds split index.
func TestGetSplitDiff_InvalidIndex(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({ splits: [{ name: 'a', files: ['f.go'] }] }, 5))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for invalid split index")
	}
	if !strings.Contains(result.Error, "invalid split index") {
		t.Errorf("expected 'invalid split index' error, got %q", result.Error)
	}
}

// TestGetSplitDiff_EmptyFiles verifies getSplitDiff handles split with no files.
func TestGetSplitDiff_EmptyFiles(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({ splits: [{ name: 'a', files: [] }] }, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for empty files")
	}
	if !strings.Contains(result.Error, "no files") {
		t.Errorf("expected 'no files' error, got %q", result.Error)
	}
}

// TestGetSplitDiff_NullPlan verifies getSplitDiff handles null/undefined plan.
func TestGetSplitDiff_NullPlan(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff(null, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for null plan")
	}
}

// TestGetSplitDiff_NegativeIndex verifies getSplitDiff handles negative index.
func TestGetSplitDiff_NegativeIndex(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({ splits: [{ name: 'a', files: ['f.go'] }] }, -1))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for negative index")
	}
}

// ---------------------------------------------------------------------------
// T80: getSplitDiff success + fallback path tests
// ---------------------------------------------------------------------------

// TestGetSplitDiff_SuccessWithDiff verifies getSplitDiff returns diff content
// when the primary git diff (baseBranch...splitName) succeeds.
func TestGetSplitDiff_SuccessWithDiff(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock: primary diff returns content.
	if _, err := evalJS(`
		globalThis._gitResponses['diff'] = _gitOk('diff --git a/f.go b/f.go\n+new line\n');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({
		baseBranch: 'main',
		splits: [{name: 'split/01', files: ['f.go']}]
	}, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error *string `json:"error"`
		Diff  string  `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %s", *result.Error)
	}
	if !strings.Contains(result.Diff, "+new line") {
		t.Errorf("expected diff content, got: %q", result.Diff)
	}
}

// TestGetSplitDiff_FallbackOnThreeDotFailure verifies that when the primary
// git diff (baseBranch...splitName) fails, getSplitDiff falls back to
// git diff baseBranch -- files.
func TestGetSplitDiff_FallbackOnThreeDotFailure(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock: primary diff (baseBranch...splitName) fails.
	// Fallback diff (baseBranch -- files) succeeds.
	if _, err := evalJS(`
		var diffCallCount = 0;
		globalThis._gitResponses['diff'] = function(argv) {
			diffCallCount++;
			// First call: three-dot diff — fail.
			if (diffCallCount === 1) {
				return _gitFail('unknown revision');
			}
			// Second call: fallback — succeed.
			return _gitOk('diff --git a/f.go b/f.go\n+fallback content\n');
		};
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({
		baseBranch: 'main',
		splits: [{name: 'split/01', files: ['f.go']}]
	}, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error *string `json:"error"`
		Diff  string  `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != nil {
		t.Errorf("expected no error (fallback success), got: %s", *result.Error)
	}
	if !strings.Contains(result.Diff, "+fallback content") {
		t.Errorf("expected fallback diff content, got: %q", result.Diff)
	}

	// Verify fallback was used (2 diff calls).
	countVal, err := evalJS(`diffCallCount`)
	if err != nil {
		t.Fatal(err)
	}
	if countVal.(int64) != 2 {
		t.Errorf("expected 2 diff calls (primary + fallback), got %d", countVal.(int64))
	}
}

// TestGetSplitDiff_BothDiffsFail verifies getSplitDiff returns error when
// both primary and fallback diffs fail.
func TestGetSplitDiff_BothDiffsFail(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock: both diffs fail.
	if _, err := evalJS(`
		globalThis._gitResponses['diff'] = _gitFail('fatal: bad object');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({
		baseBranch: 'main',
		splits: [{name: 'split/01', files: ['f.go']}]
	}, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Error, "git diff failed") {
		t.Errorf("expected 'git diff failed' error, got: %q", result.Error)
	}
}

// TestBuildReport_WithNullCaches verifies the 'report' command outputs valid
// JSON when no caches are populated (no analyze/group/plan has been run).
func TestBuildReport_WithNullCaches(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{Path: "src/main.go", Content: "package main\n"},
		},
	})

	// Dispatch 'report' without running analyze first.
	if err := tp.Dispatch("report", nil); err != nil {
		t.Fatal(err)
	}

	out := tp.Stdout.String()
	// Find the JSON object in the output (may have other prints).
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("expected JSON in output, got: %s", out)
	}
	jsonStr := out[idx:]
	// Find matching closing brace.
	depth := 0
	end := -1
	for i, c := range jsonStr {
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		t.Fatalf("incomplete JSON in output: %s", jsonStr)
	}
	jsonStr = jsonStr[:end]

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		t.Fatalf("failed to parse report JSON: %v\nraw: %s", err, jsonStr)
	}
	if _, ok := report["version"]; !ok {
		t.Error("expected report to have 'version' field")
	}
	if report["analysis"] != nil {
		t.Error("expected report.analysis to be null when no analyze has been run")
	}
	if report["groups"] != nil {
		t.Error("expected report.groups to be null when no group has been run")
	}
	if report["plan"] != nil {
		t.Error("expected report.plan to be null when no plan has been run")
	}
}

// TestBuildReport_WithPopulatedCaches verifies the 'report' command produces
// complete JSON when analyze, group, and plan have been run.
func TestBuildReport_WithPopulatedCaches(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{Path: "cmd/run.go", Content: "package main\n\nfunc run() {}\n"},
			{Path: "pkg/lib.go", Content: "package pkg\n\nfunc Lib() {}\n"},
		},
	})

	// Run analyze → group → plan → report through dispatch.
	for _, cmd := range []string{"analyze", "group", "plan"} {
		if err := tp.Dispatch(cmd, nil); err != nil {
			t.Fatalf("dispatch %q failed: %v", cmd, err)
		}
	}

	// Clear stdout before report capture.
	tp.Stdout.Reset()
	if err := tp.Dispatch("report", nil); err != nil {
		t.Fatal(err)
	}

	out := tp.Stdout.String()
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("expected JSON in output, got: %s", out)
	}
	jsonStr := out[idx:]
	depth := 0
	end := -1
	for i, c := range jsonStr {
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		t.Fatalf("incomplete JSON in output: %s", jsonStr)
	}
	jsonStr = jsonStr[:end]

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		t.Fatalf("failed to parse report JSON: %v\nraw: %s", err, jsonStr)
	}

	// Version
	version, _ := report["version"].(string)
	if version == "" || version == "unknown" {
		t.Errorf("expected valid version, got %q", version)
	}

	// Analysis
	analysis, _ := report["analysis"].(map[string]interface{})
	if analysis == nil {
		t.Fatal("expected report.analysis to be populated after analyze")
	}
	fileCount, _ := analysis["fileCount"].(float64)
	if fileCount < 1 {
		t.Errorf("expected at least 1 file in analysis, got %v", fileCount)
	}

	// Groups
	groups, _ := report["groups"].([]interface{})
	if len(groups) == 0 {
		t.Error("expected report.groups to be populated after group")
	}

	// Plan
	plan, _ := report["plan"].(map[string]interface{})
	if plan == nil {
		t.Fatal("expected report.plan to be populated after plan")
	}
	splitCount, _ := plan["splitCount"].(float64)
	if splitCount < 1 {
		t.Errorf("expected at least 1 split in plan, got %v", splitCount)
	}
}
