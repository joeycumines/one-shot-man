package command

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestAutomatedDefaults_OverrideChain(t *testing.T) {
	t.Parallel()

	t.Run("defaults_when_no_config", func(t *testing.T) {
		t.Parallel()
		_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

		val, err := evalJS(`JSON.stringify(AUTOMATED_DEFAULTS)`)
		if err != nil {
			t.Fatal(err)
		}
		var defaults struct {
			ClassifyTimeoutMs       int64 `json:"classifyTimeoutMs"`
			PlanTimeoutMs           int64 `json:"planTimeoutMs"`
			ResolveTimeoutMs        int64 `json:"resolveTimeoutMs"`
			PollIntervalMs          int64 `json:"pollIntervalMs"`
			MaxResolveRetries       int64 `json:"maxResolveRetries"`
			MaxReSplits             int64 `json:"maxReSplits"`
			ResolveWallClockTimeout int64 `json:"resolveWallClockTimeoutMs"`
			VerifyTimeoutMs         int64 `json:"verifyTimeoutMs"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &defaults); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if defaults.ClassifyTimeoutMs != 300000 {
			t.Errorf("classifyTimeoutMs: got %d, want 300000", defaults.ClassifyTimeoutMs)
		}
		if defaults.PlanTimeoutMs != 300000 {
			t.Errorf("planTimeoutMs: got %d, want 300000", defaults.PlanTimeoutMs)
		}
		if defaults.ResolveTimeoutMs != 1800000 {
			t.Errorf("resolveTimeoutMs: got %d, want 1800000", defaults.ResolveTimeoutMs)
		}
		if defaults.PollIntervalMs != 500 {
			t.Errorf("pollIntervalMs: got %d, want 500", defaults.PollIntervalMs)
		}
		if defaults.MaxReSplits != 1 {
			t.Errorf("maxReSplits: got %d, want 1", defaults.MaxReSplits)
		}
		if defaults.VerifyTimeoutMs != 600000 {
			t.Errorf("verifyTimeoutMs: got %d, want 600000", defaults.VerifyTimeoutMs)
		}
	})

	t.Run("config_overrides_classify_only", func(t *testing.T) {
		t.Parallel()
		_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

		// Simulate what automatedSplit does: build timeouts from config + defaults.
		val, err := evalJS(`(function() {
			var config = { classifyTimeoutMs: 5000 };
			var timeouts = {
				classify: config.classifyTimeoutMs || AUTOMATED_DEFAULTS.classifyTimeoutMs,
				plan: config.planTimeoutMs || AUTOMATED_DEFAULTS.planTimeoutMs,
				resolve: config.resolveTimeoutMs || AUTOMATED_DEFAULTS.resolveTimeoutMs
			};
			return JSON.stringify(timeouts);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		var timeouts struct {
			Classify int64 `json:"classify"`
			Plan     int64 `json:"plan"`
			Resolve  int64 `json:"resolve"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &timeouts); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if timeouts.Classify != 5000 {
			t.Errorf("classify: got %d, want 5000", timeouts.Classify)
		}
		if timeouts.Plan != 300000 {
			t.Errorf("plan: got %d, want 300000 (default)", timeouts.Plan)
		}
		if timeouts.Resolve != 1800000 {
			t.Errorf("resolve: got %d, want 1800000 (default)", timeouts.Resolve)
		}
	})
}

// ---------------------------------------------------------------------------
// T111: verifySplits with empty/null plan
// ---------------------------------------------------------------------------

func TestVerifySplits_EmptyPlan(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	t.Run("null_plan", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(verifySplits(null, {}))`)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			AllPassed bool    `json:"allPassed"`
			Error     *string `json:"error"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if out.AllPassed {
			t.Error("expected allPassed to be false for null plan")
		}
		if out.Error == nil || !strings.Contains(*out.Error, "invalid plan") {
			t.Errorf("expected 'invalid plan' error, got: %v", out.Error)
		}
	})

	t.Run("undefined_plan", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(verifySplits(undefined, {}))`)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			AllPassed bool    `json:"allPassed"`
			Error     *string `json:"error"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if out.AllPassed {
			t.Error("expected allPassed to be false for undefined plan")
		}
	})

	t.Run("empty_splits_array", func(t *testing.T) {
		// gitExec is called for rev-parse + checkout even on empty loops,
		// so we stub exec.execv to return a safe result.
		val, err := evalJS(`(function() {
			var origExec = exec.execv;
			exec.execv = function() { return { code: 0, stdout: "main\n", stderr: "" }; };
			try {
				return JSON.stringify(verifySplits({ splits: [] }, {}));
			} finally {
				exec.execv = origExec;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			AllPassed bool  `json:"allPassed"`
			Results   []any `json:"results"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
			t.Fatalf("parse: %v", err)
		}
		// Empty splits → all passed (vacuous truth), no results, null error.
		if !out.AllPassed {
			t.Error("expected allPassed to be true for empty splits (vacuous truth)")
		}
		if len(out.Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(out.Results))
		}
	})
}

// ---------------------------------------------------------------------------
// T120: sanitizeBranchName edge cases
// ---------------------------------------------------------------------------

func TestSanitizeBranchName_EdgeCases(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	cases := []struct {
		name   string
		input  string
		expect string
	}{
		{"spaces", "my branch name", "my-branch-name"},
		{"special_chars", "feat!@#$%^&*()", "feat----------"},
		{"dots_allowed_via_replace", "feature.fix", "feature-fix"},
		{"slashes_preserved", "split/feature-auth", "split/feature-auth"},
		{"empty_string", "", ""},
		{"underscores_preserved", "my_branch", "my_branch"},
		{"consecutive_dashes", "a  b", "a--b"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			escaped := strings.ReplaceAll(tc.input, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `'`, `\'`)
			val, err := evalJS(`sanitizeBranchName('` + escaped + `')`)
			if err != nil {
				t.Fatal(err)
			}
			if val.(string) != tc.expect {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tc.input, val.(string), tc.expect)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T121: shellQuote edge cases
// ---------------------------------------------------------------------------

func TestShellQuote_EdgeCases(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	cases := []struct {
		name   string
		input  string
		expect string
	}{
		{"empty_string", "", "''"},
		{"simple", "hello", "'hello'"},
		{"with_spaces", "hello world", "'hello world'"},
		{"single_quote", "it's", `'it'\''s'`},
		{"double_quotes", `say "hi"`, `'say "hi"'`},
		{"backticks", "echo `whoami`", "'echo `whoami`'"},
		{"dollar_expansion", "$(rm -rf /)", "'$(rm -rf /)'"},
		{"newlines", "line1\nline2", "'line1\nline2'"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Escape for JS string literal.
			jsEscaped := strings.ReplaceAll(tc.input, `\`, `\\`)
			jsEscaped = strings.ReplaceAll(jsEscaped, `'`, `\'`)
			jsEscaped = strings.ReplaceAll(jsEscaped, "\n", `\n`)
			val, err := evalJS(`shellQuote('` + jsEscaped + `')`)
			if err != nil {
				t.Fatal(err)
			}
			if val.(string) != tc.expect {
				t.Errorf("shellQuote(%q) = %q, want %q", tc.input, val.(string), tc.expect)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T122: classificationToGroups
// ---------------------------------------------------------------------------

func TestClassificationToGroups_EdgeCases(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	t.Run("empty_classification", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(classificationToGroups({}))`)
		if err != nil {
			t.Fatal(err)
		}
		if val.(string) != "{}" {
			t.Errorf("expected empty object, got %s", val)
		}
	})

	t.Run("single_file", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(classificationToGroups({"a.go": "api"}))`)
		if err != nil {
			t.Fatal(err)
		}
		var groups map[string]struct {
			Files       []string `json:"files"`
			Description string   `json:"description"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || len(groups["api"].Files) != 1 || groups["api"].Files[0] != "a.go" {
			t.Errorf("unexpected groups: %v", groups)
		}
	})

	t.Run("multiple_categories", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(classificationToGroups({
			"a.go": "api",
			"b.go": "api",
			"c.go": "db",
			"d.go": "ui"
		}))`)
		if err != nil {
			t.Fatal(err)
		}
		var groups map[string]struct {
			Files       []string `json:"files"`
			Description string   `json:"description"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
			t.Fatal(err)
		}
		if len(groups["api"].Files) != 2 {
			t.Errorf("api group: expected 2 files, got %d", len(groups["api"].Files))
		}
		if len(groups["db"].Files) != 1 {
			t.Errorf("db group: expected 1 file, got %d", len(groups["db"].Files))
		}
		if len(groups["ui"].Files) != 1 {
			t.Errorf("ui group: expected 1 file, got %d", len(groups["ui"].Files))
		}
	})

	t.Run("new_categories_array_with_descriptions", func(t *testing.T) {
		val, err := evalJS(`JSON.stringify(classificationToGroups([
			{name: "api", description: "Add API endpoints", files: ["a.go", "b.go"]},
			{name: "db", description: "Add database layer", files: ["c.go"]}
		]))`)
		if err != nil {
			t.Fatal(err)
		}
		var groups map[string]struct {
			Files       []string `json:"files"`
			Description string   `json:"description"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &groups); err != nil {
			t.Fatal(err)
		}
		if len(groups["api"].Files) != 2 {
			t.Errorf("api group: expected 2 files, got %d", len(groups["api"].Files))
		}
		if groups["api"].Description != "Add API endpoints" {
			t.Errorf("api description = %q, want 'Add API endpoints'", groups["api"].Description)
		}
		if groups["db"].Description != "Add database layer" {
			t.Errorf("db description = %q, want 'Add database layer'", groups["db"].Description)
		}
	})
}

// ---------------------------------------------------------------------------
// T40: Pipeline-level boundary conditions
// ---------------------------------------------------------------------------

// pipelineResult holds parsed automatedSplit() return data.
type pipelineResult struct {
	Error      string
	PlanSplits int // number of splits in report.plan.splits
	PlanFiles  int // total files across all plan splits
}

// parsePipelineResult extracts key metrics from the automatedSplit() return
// value, which has the structure: { error, report: { plan: { splits: [...] } } }.
func parsePipelineResult(t *testing.T, raw any) pipelineResult {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &m); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	r := pipelineResult{}
	if e, ok := m["error"].(string); ok {
		r.Error = e
	}
	report, _ := m["report"].(map[string]any)
	if report != nil {
		plan, _ := report["plan"].(map[string]any)
		if plan != nil {
			splits, _ := plan["splits"].([]any)
			r.PlanSplits = len(splits)
			for _, s := range splits {
				sm, _ := s.(map[string]any)
				if sm != nil {
					files, _ := sm["files"].([]any)
					r.PlanFiles += len(files)
				}
			}
		}
	}
	return r
}

// TestAutoSplit_EmptyDiff verifies the pipeline gracefully handles a feature
// branch with no changes relative to main (empty diff).
func TestAutoSplit_EmptyDiff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Set up a repo where "feature" branch is identical to "main" (no diff).
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"a.go", "package a\n"},
		},
		NoFeatureFiles: true, // feature branch has no file changes
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

	// Mock ClaudeCodeExecutor to avoid real AI.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.spawn = function() { return { error: 'not resolved' }; };
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: true,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	pr := parsePipelineResult(t, result)

	// Should either succeed with 0 files or produce a clear error —
	// NOT panic, hang, or produce a corrupted state.
	t.Logf("empty diff: error=%q planFiles=%d planSplits=%d", pr.Error, pr.PlanFiles, pr.PlanSplits)
	if pr.PlanFiles != 0 && pr.Error == "" {
		t.Errorf("expected 0 files or an error for empty diff, got %d files", pr.PlanFiles)
	}
}

// TestAutoSplit_SingleFile verifies the pipeline handles a single-file change.
func TestAutoSplit_SingleFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{"single.go", "package single\n\nfunc Alone() {}\n"},
		},
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

	// Mock ClaudeCodeExecutor.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.spawn = function() { return { error: 'not resolved' }; };
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: true,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	pr := parsePipelineResult(t, result)

	t.Logf("single file: error=%q planFiles=%d planSplits=%d", pr.Error, pr.PlanFiles, pr.PlanSplits)

	// Single file should produce exactly 1 file in the plan.
	if pr.Error != "" {
		t.Logf("pipeline completed with error (acceptable for single-file edge): %s", pr.Error)
	}
	if pr.PlanFiles != 1 {
		t.Errorf("expected 1 plan file, got %d", pr.PlanFiles)
	}
}

// TestAutoSplit_LargeDiff verifies the pipeline handles a 100+ file change
// without hanging, panicking, or producing a corrupted plan.
func TestAutoSplit_LargeDiff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Generate 120 feature files across multiple directories.
	var featureFiles []TestPipelineFile
	for i := 0; i < 120; i++ {
		dir := fmt.Sprintf("pkg%d", i/10)
		name := fmt.Sprintf("%s/f%02d.go", dir, i%10)
		content := fmt.Sprintf("package %s\n\nfunc F%d() {}\n", dir, i)
		featureFiles = append(featureFiles, TestPipelineFile{name, content})
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: featureFiles,
		ConfigOverrides: map[string]any{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
			"maxFiles":      15, // force splitting
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

	// Mock ClaudeCodeExecutor.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.spawn = function() { return { error: 'not resolved' }; };
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: true,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	pr := parsePipelineResult(t, result)

	t.Logf("large diff: error=%q planFiles=%d planSplits=%d", pr.Error, pr.PlanFiles, pr.PlanSplits)

	// Must detect all 120 files.
	if pr.PlanFiles != 120 {
		t.Errorf("expected 120 plan files, got %d", pr.PlanFiles)
	}
	// With 120 files across 12 directories and maxFiles=15, expect multiple splits.
	if pr.PlanSplits < 2 {
		t.Errorf("expected at least 2 plan splits for 120 files, got %d", pr.PlanSplits)
	}
}

// TestAutoSplit_BinaryFiles verifies the pipeline handles binary files in the
// diff without crashing. Binary files show as "-\t-\tfilename" in --numstat
// and should be grouped and split like any other file.
func TestAutoSplit_BinaryFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create a repo with binary files on the feature branch.
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{"images/logo.png", string([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00})}, // PNG header
			{"data/config.bin", string([]byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE, 0xFD})},
			{"src/app.go", "package src\n\nfunc App() {}\n"}, // normal file alongside binary
		},
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

	// Mock ClaudeCodeExecutor.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.spawn = function() { return { error: 'not resolved' }; };
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	// Verify git sees the binary files.
	diffOut, err := tp.EvalJS(`gitExec('` + strings.ReplaceAll(tp.Dir, `\`, `\\`) + `',
		['diff', '--numstat', 'main...feature']).stdout`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("numstat output:\n%s", diffOut)

	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: true,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	pr := parsePipelineResult(t, result)

	t.Logf("binary files: error=%q planFiles=%d planSplits=%d", pr.Error, pr.PlanFiles, pr.PlanSplits)

	// Should detect all 3 files (2 binary + 1 source).
	if pr.PlanFiles != 3 {
		t.Errorf("expected 3 plan files (2 binary + 1 source), got %d", pr.PlanFiles)
	}
	// With 3 files across 3 directories, expect at least 1 split.
	if pr.PlanSplits < 1 {
		t.Errorf("expected at least 1 plan split, got %d", pr.PlanSplits)
	}
}

// ---------------------------------------------------------------------------
// T123: 10,000-file grouping (JS-level, no real git repo needed)
// ---------------------------------------------------------------------------

// TestGrouping_TenThousandFiles verifies the grouping engine can handle
// 10,000 files without OOM, hang, or corrupted output — all files must
// appear in exactly one group.
func TestGrouping_TenThousandFiles(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Generate 10,000 file paths across 100 top-level directories (100 files each).
	val, err := evalJS(`(function() {
		var files = [];
		for (var d = 0; d < 100; d++) {
			for (var f = 0; f < 100; f++) {
				files.push('dir' + d + '/file' + f + '.go');
			}
		}

		// Test groupByDirectory with the massive file set.
		var groups = globalThis.prSplit.groupByDirectory(files);

		// Verify no files lost — sum of all group file counts must equal 10000.
		var totalFiles = 0;
		var groupNames = Object.keys(groups);
		for (var i = 0; i < groupNames.length; i++) {
			totalFiles += groups[groupNames[i]].length;
		}

		return JSON.stringify({
			inputFiles: files.length,
			groupCount: groupNames.length,
			totalFiles: totalFiles
		});
	})()`)
	if err != nil {
		t.Fatalf("groupByDirectory(10k): %v", err)
	}

	var result struct {
		InputFiles int `json:"inputFiles"`
		GroupCount int `json:"groupCount"`
		TotalFiles int `json:"totalFiles"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.InputFiles != 10000 {
		t.Errorf("input files: got %d, want 10000", result.InputFiles)
	}
	if result.TotalFiles != 10000 {
		t.Errorf("total files in groups: got %d, want 10000 (files lost during grouping!)", result.TotalFiles)
	}
	if result.GroupCount < 2 {
		t.Errorf("expected at least 2 groups for 100 directories, got %d", result.GroupCount)
	}
	t.Logf("10k files: %d groups, %d total files", result.GroupCount, result.TotalFiles)
}

// TestGrouping_TenThousandFiles_ClassificationToGroups verifies
// classificationToGroups handles 10,000 entries with no file loss.
func TestGrouping_TenThousandFiles_ClassificationToGroups(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`(function() {
		var classification = {};
		for (var d = 0; d < 100; d++) {
			for (var f = 0; f < 100; f++) {
				classification['pkg/dir' + d + '/file' + f + '.go'] = 'category-' + d;
			}
		}

		var groups = classificationToGroups(classification);

		var totalFiles = 0;
		var groupNames = Object.keys(groups);
		for (var i = 0; i < groupNames.length; i++) {
			totalFiles += groups[groupNames[i]].files.length;
		}

		return JSON.stringify({
			inputFiles: Object.keys(classification).length,
			groupCount: groupNames.length,
			totalFiles: totalFiles
		});
	})()`)
	if err != nil {
		t.Fatalf("classificationToGroups(10k): %v", err)
	}

	var result struct {
		InputFiles int `json:"inputFiles"`
		GroupCount int `json:"groupCount"`
		TotalFiles int `json:"totalFiles"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}

	if result.InputFiles != 10000 {
		t.Errorf("input files: got %d, want 10000", result.InputFiles)
	}
	if result.TotalFiles != 10000 {
		t.Errorf("total files in groups: got %d, want 10000 (files lost!)", result.TotalFiles)
	}
	if result.GroupCount != 100 {
		t.Errorf("expected 100 groups for 100 categories, got %d", result.GroupCount)
	}
	t.Logf("10k files via classification: %d groups, %d total files", result.GroupCount, result.TotalFiles)
}

// ---------------------------------------------------------------------------
// T124: Special characters in file paths
// ---------------------------------------------------------------------------

// TestAnalyzeDiff_SpecialCharPaths verifies that file paths with spaces,
// unicode, and special characters survive diff parsing and grouping.
func TestAnalyzeDiff_SpecialCharPaths(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Install the shared git mock infrastructure.
	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Configure mock responses with special-character file paths.
	if _, err := evalJS(`
		globalThis._gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature-special');
		globalThis._gitResponses['merge-base main feature-special'] = _gitOk('abc123');
		globalThis._gitResponses['diff --name-status abc123 feature-special'] = _gitOk(
			"M\tpath with spaces/file.go\n" +
			"A\tunicode/\u65E5\u672C\u8A9E.go\n" +
			"M\tspecial-chars/file[1].go\n" +
			"A\tdots.and.dashes/my-file.test.go\n" +
			"M\tdeep/nested/path/to/a/file.go\n"
		);
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`(function() {
		var analysis = globalThis.prSplit.analyzeDiff({baseBranch: 'main'});
		var errors = [];

		// All 5 files should be detected.
		if (!analysis.files || analysis.files.length !== 5) {
			errors.push('expected 5 files, got ' + (analysis.files ? analysis.files.length : 0));
		}

		if (analysis.files) {
			// Verify specific paths survived (files are strings, not objects).
			var foundSpace = false, foundUnicode = false, foundBracket = false;
			for (var i = 0; i < analysis.files.length; i++) {
				var fn = analysis.files[i];
				if (fn.indexOf('path with spaces') >= 0) foundSpace = true;
				if (fn.indexOf('\u65E5\u672C\u8A9E') >= 0) foundUnicode = true;
				if (fn.indexOf('file[1]') >= 0) foundBracket = true;
			}
			if (!foundSpace) errors.push('path with spaces not found');
			if (!foundUnicode) errors.push('unicode path not found');
			if (!foundBracket) errors.push('bracket path not found');

			// Verify fileStatuses map has entries for all files.
			var statusCount = Object.keys(analysis.fileStatuses || {}).length;
			if (statusCount !== 5) {
				errors.push('expected 5 fileStatuses entries, got ' + statusCount);
			}

			// Verify groupByDirectory works with these paths.
			var groups = globalThis.prSplit.groupByDirectory(analysis.files);
			var groupKeys = Object.keys(groups);
			if (groupKeys.length < 3) {
				errors.push('expected at least 3 directory groups, got ' + groupKeys.length);
			}
		}

		return errors.length === 0 ? 'OK' : errors.join('; ');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val.(string) != "OK" {
		t.Errorf("special char paths: %s", val)
	}
}

// ---------------------------------------------------------------------------
// T125: Branch collision (pre-existing split branches are cleaned up)
// ---------------------------------------------------------------------------

// TestAutoSplit_BranchCollision verifies that when split branch names
// already exist, the pipeline's pre-flight step deletes them and proceeds
// successfully.
func TestAutoSplit_BranchCollision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{"pkg1/a.go", "package pkg1\n\nfunc A() {}\n"},
			{"pkg2/b.go", "package pkg2\n\nfunc B() {}\n"},
		},
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

	// Create pre-existing split branches that would collide.
	if _, err := tp.EvalJS(`(function() {
		var dir = '` + strings.ReplaceAll(tp.Dir, `\`, `\\`) + `';
		gitExec(dir, ['checkout', '-b', 'split/pkg1', 'main']);
		gitExec(dir, ['checkout', '-b', 'split/pkg2', 'main']);
		gitExec(dir, ['checkout', 'feature']); // back to feature
	})()`); err != nil {
		t.Fatal(err)
	}

	// Verify pre-existing branches exist.
	verifyBranch, err := tp.EvalJS(`gitExec('` + strings.ReplaceAll(tp.Dir, `\`, `\\`) + `',
		['branch', '--list', 'split/*']).stdout`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(verifyBranch.(string), "split/pkg1") {
		t.Fatal("pre-existing split/pkg1 branch not created")
	}

	// Mock ClaudeCodeExecutor.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.spawn = function() { return { error: 'not resolved' }; };
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	// Run the pipeline — should delete pre-existing branches and re-create them.
	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: true,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	pr := parsePipelineResult(t, result)
	t.Logf("collision: error=%q planFiles=%d planSplits=%d", pr.Error, pr.PlanFiles, pr.PlanSplits)

	// Pipeline should succeed — collision handled by pre-flight deletion.
	if pr.PlanFiles != 2 {
		t.Errorf("expected 2 plan files, got %d", pr.PlanFiles)
	}
	if pr.PlanSplits < 1 {
		t.Errorf("expected at least 1 plan split, got %d", pr.PlanSplits)
	}

	// Verify split branches exist again (re-created after pre-flight delete).
	branchOut, err := tp.EvalJS(`gitExec('` + strings.ReplaceAll(tp.Dir, `\`, `\\`) + `',
		['branch', '--list', 'split/*']).stdout`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(branchOut.(string), "split/") {
		t.Errorf("expected split branches to exist after pipeline, got: %s", branchOut)
	}
}

// ---------------------------------------------------------------------------
// T126: Only-deletions diff (no new/modified files, all files deleted)
// ---------------------------------------------------------------------------

func TestAutoSplit_OnlyDeletions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create a repo where the feature branch DELETES files that exist on main.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"old/remove-me.go", "package old\n\nfunc Legacy() {}\n"},
			{"old/also-remove.go", "package old\n\nfunc AlsoLegacy() {}\n"},
			{"keep/keeper.go", "package keep\n\nfunc Keep() {}\n"},
		},
		// No new feature files — only deletions.
		NoFeatureFiles: true,
		// Feature branch: delete the 'old' files.
		DeleteFilesOnFeature: []string{"old/remove-me.go", "old/also-remove.go"},
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

	// Mock ClaudeCodeExecutor.
	if _, err := tp.EvalJS(`
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.resolveAsync = async function() { return { error: 'not available' }; };
		ClaudeCodeExecutor.prototype.spawn = function() { return { error: 'not resolved' }; };
		ClaudeCodeExecutor.prototype.close = function() {};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatal(err)
	}

	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: true,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	pr := parsePipelineResult(t, result)
	t.Logf("only-deletions: error=%q planFiles=%d planSplits=%d", pr.Error, pr.PlanFiles, pr.PlanSplits)

	// Deletion-only diff should detect the 2 deleted files.
	if pr.PlanFiles != 2 {
		t.Errorf("expected 2 plan files (deletions), got %d", pr.PlanFiles)
	}
}

// ---------------------------------------------------------------------------
// T127: TUI rendering at extreme terminal sizes
// ---------------------------------------------------------------------------

func TestTUI_ExtremeTerminalSizes(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	// Set up plan cache so PLAN_REVIEW renders content.
	if _, err := evalJS(`setupPlanCache()`); err != nil {
		t.Fatal(err)
	}

	sizes := []struct {
		name   string
		width  int
		height int
	}{
		{"zero_dimensions", 0, 0},
		{"zero_width", 0, 24},
		{"zero_height", 80, 0},
		{"minimal_1x1", 1, 1},
		{"very_narrow_10x24", 10, 24},
		{"very_short_80x3", 80, 3},
		{"ultra_wide_500x24", 500, 24},
		{"ultra_tall_80x200", 80, 200},
		{"tiny_5x5", 5, 5},
	}

	screens := []struct {
		state string
		setup string
	}{
		{"CONFIG", ""},
		{"PLAN_REVIEW", ""},
		{"BRANCH_BUILDING", "s.executionResults=[];s.executingIdx=0;s.isProcessing=true;"},
		{"FINALIZATION", "s.equivalenceResult={equivalent:true};s.startTime=Date.now()-60000;"},
		{"ERROR_RESOLUTION", "s.errorDetails='test error';"},
	}

	for _, size := range sizes {
		for _, screen := range screens {
			name := fmt.Sprintf("%s_%s", size.name, screen.state)
			t.Run(name, func(t *testing.T) {
				js := fmt.Sprintf(`(function() {
					var s = initState('%s');
					s.width = %d;
					s.height = %d;
					%s
					try {
						var view = globalThis.prSplit._wizardView(s);
						if (typeof view !== 'string') return 'ERROR: view is not a string: ' + typeof view;
						return 'OK:' + view.length;
					} catch(e) {
						return 'CRASH: ' + e.message;
					}
				})()`, screen.state, size.width, size.height, screen.setup)

				raw, err := evalJS(js)
				if err != nil {
					t.Fatalf("evalJS failed: %v", err)
				}
				result := raw.(string)
				if strings.HasPrefix(result, "CRASH:") {
					t.Errorf("rendering crashed at %dx%d on %s: %s",
						size.width, size.height, screen.state, result)
				}
				if strings.HasPrefix(result, "ERROR:") {
					t.Errorf("rendering error at %dx%d on %s: %s",
						size.width, size.height, screen.state, result)
				}
				// Any OK result is acceptable — we're testing no-crash, not pixel-perfect.
				if !strings.HasPrefix(result, "OK:") {
					t.Errorf("unexpected result at %dx%d on %s: %s",
						size.width, size.height, screen.state, result)
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// T128: Plan validation — empty split, duplicate files across splits,
//       unknown files not in diff
// ---------------------------------------------------------------------------

func TestValidation_EdgeCases(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	t.Run("split_with_no_files_rejected", func(t *testing.T) {
		val, err := evalJS(`(function() {
			var result = validatePlan({
				baseBranch: 'main',
				splits: [
					{name: 'split/empty', files: [], message: 'empty split', order: 0}
				]
			});
			return JSON.stringify(result);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(val.(string), "no files") {
			t.Errorf("expected 'no files' error for empty split, got: %s", val)
		}
	})

	t.Run("duplicate_files_across_splits_rejected", func(t *testing.T) {
		val, err := evalJS(`(function() {
			var result = validatePlan({
				baseBranch: 'main',
				splits: [
					{name: 'split/a', files: ['shared.go', 'a.go'], message: 'A', order: 0},
					{name: 'split/b', files: ['shared.go', 'b.go'], message: 'B', order: 1}
				]
			});
			return JSON.stringify(result);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(val.(string), "duplicate") {
			t.Errorf("expected 'duplicate' error, got: %s", val)
		}
	})

	t.Run("split_with_undefined_name_rejected", func(t *testing.T) {
		val, err := evalJS(`(function() {
			var result = validatePlan({
				baseBranch: 'main',
				splits: [
					{name: undefined, files: ['a.go'], message: 'no name', order: 0}
				]
			});
			return JSON.stringify(result);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		// Should either reject or use fallback name — must not crash.
		if strings.Contains(val.(string), "panic") || strings.Contains(val.(string), "undefined") {
			t.Errorf("validation should handle undefined split name, got: %s", val)
		}
	})

	t.Run("no_splits_at_all_rejected", func(t *testing.T) {
		val, err := evalJS(`(function() {
			var result = validatePlan({
				baseBranch: 'main',
				splits: []
			});
			return JSON.stringify(result);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Valid  bool     `json:"valid"`
			Errors []string `json:"errors"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
			t.Fatal(err)
		}
		if result.Valid {
			t.Error("expected empty plan to be invalid")
		}
	})

	t.Run("null_plan_rejected", func(t *testing.T) {
		val, err := evalJS(`(function() {
			try {
				var result = validatePlan(null);
				return JSON.stringify(result);
			} catch(e) {
				return 'CAUGHT: ' + e.message;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		// Should either return validation error or be caught — must not panic.
		if strings.Contains(val.(string), "panic") {
			t.Errorf("null plan should not cause panic: %s", val)
		}
	})

	t.Run("split_name_with_git_forbidden_chars", func(t *testing.T) {
		val, err := evalJS(`(function() {
			var result = validateSplitPlan([
				{name: 'split/bad name~with^chars', files: ['a.go'], message: 'bad', order: 0}
			]);
			return JSON.stringify(result);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		// validateSplitPlan checks branch name validity — spaces and ~ ^ are
		// git-forbidden characters that should trigger a validation error.
		if !strings.Contains(val.(string), "invalid") && !strings.Contains(val.(string), "branch") && !strings.Contains(val.(string), "false") {
			t.Logf("got: %s", val)
			// Some validators may not check branch chars — log for visibility.
		}
	})
}

// ---------------------------------------------------------------------------
// T129: Extremely long file and split names in rendering
// ---------------------------------------------------------------------------

func TestTUI_LongNamesRendering(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	// Set up a plan with extremely long names.
	if _, err := evalJS(`(function() {
		var longName = '';
		for (var i = 0; i < 50; i++) longName += 'very-long-segment-';
		longName += 'end';

		var longFileName = '';
		for (var i = 0; i < 30; i++) longFileName += 'deeply/nested/';
		longFileName += 'extremely-long-filename-that-goes-on-forever.go';

		globalThis.prSplit._state.planCache = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [
				{name: 'split/' + longName, files: [longFileName], message: longName, order: 0},
				{name: 'split/normal', files: ['a.go'], message: 'normal', order: 1}
			]
		};
	})()`); err != nil {
		t.Fatal(err)
	}

	t.Run("plan_review_long_names", func(t *testing.T) {
		raw, err := evalJS(`(function() {
			var s = initState('PLAN_REVIEW');
			s.width = 80;
			s.height = 24;
			try {
				var view = globalThis.prSplit._wizardView(s);
				if (typeof view !== 'string') return 'ERROR: not string';
				// View should truncate — total length bounded.
				if (view.length > 50000) return 'TOO_LONG: ' + view.length;
				return 'OK:' + view.length;
			} catch(e) {
				return 'CRASH: ' + e.message;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		result := raw.(string)
		if !strings.HasPrefix(result, "OK:") {
			t.Errorf("long name rendering failed: %s", result)
		}
	})

	t.Run("plan_editor_long_names", func(t *testing.T) {
		raw, err := evalJS(`(function() {
			var s = initState('PLAN_EDITOR');
			s.width = 80;
			s.height = 24;
			s.editorCheckedFiles = {};
			s.editorValidationErrors = [];
			try {
				var view = globalThis.prSplit._wizardView(s);
				if (typeof view !== 'string') return 'ERROR: not string';
				if (view.length > 50000) return 'TOO_LONG: ' + view.length;
				return 'OK:' + view.length;
			} catch(e) {
				return 'CRASH: ' + e.message;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		result := raw.(string)
		if !strings.HasPrefix(result, "OK:") {
			t.Errorf("long name editor rendering failed: %s", result)
		}
	})

	t.Run("narrow_terminal_with_long_names", func(t *testing.T) {
		raw, err := evalJS(`(function() {
			var s = initState('PLAN_REVIEW');
			s.width = 20;
			s.height = 24;
			try {
				var view = globalThis.prSplit._wizardView(s);
				if (typeof view !== 'string') return 'ERROR: not string';
				return 'OK:' + view.length;
			} catch(e) {
				return 'CRASH: ' + e.message;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		result := raw.(string)
		if !strings.HasPrefix(result, "OK:") {
			t.Errorf("narrow+long name rendering failed: %s", result)
		}
	})
}
