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
