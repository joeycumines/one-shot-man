package command

import (
	"encoding/json"
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
		if defaults.ClassifyTimeoutMs != 1200000 {
			t.Errorf("classifyTimeoutMs: got %d, want 1200000", defaults.ClassifyTimeoutMs)
		}
		if defaults.PlanTimeoutMs != 1200000 {
			t.Errorf("planTimeoutMs: got %d, want 1200000", defaults.PlanTimeoutMs)
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
		if timeouts.Plan != 1200000 {
			t.Errorf("plan: got %d, want 1200000 (default)", timeouts.Plan)
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
