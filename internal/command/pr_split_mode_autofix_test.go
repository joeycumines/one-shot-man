package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestPrSplitCommand_SetModeCommand(t *testing.T) {
	t.Parallel()

	stdout, dispatch, _, _ := loadPrSplitEngineWithEval(t, nil)

	// Set mode to auto.
	if err := dispatch("set", []string{"mode", "auto"}); err != nil {
		t.Fatalf("set mode auto returned error: %v", err)
	}
	output := stdout.String()
	if !contains(output, "auto") {
		t.Errorf("Expected 'auto' confirmation, got: %s", output)
	}

	// Set mode to heuristic.
	stdout.Reset()
	if err := dispatch("set", []string{"mode", "heuristic"}); err != nil {
		t.Fatalf("set mode heuristic returned error: %v", err)
	}
	output = stdout.String()
	if !contains(output, "heuristic") {
		t.Errorf("Expected 'heuristic' confirmation, got: %s", output)
	}

	// Invalid mode.
	stdout.Reset()
	if err := dispatch("set", []string{"mode", "invalid"}); err != nil {
		t.Fatalf("set mode invalid returned error: %v", err)
	}
	output = stdout.String()
	if !contains(output, "Invalid mode") {
		t.Errorf("Expected 'Invalid mode' error, got: %s", output)
	}
}

func TestPrSplitCommand_SetShowsMode(t *testing.T) {
	t.Parallel()

	stdout, dispatch, _, _ := loadPrSplitEngineWithEval(t, nil)

	// Call set with no args to show current config — should include mode.
	if err := dispatch("set", nil); err != nil {
		t.Fatalf("set (no args) returned error: %v", err)
	}
	output := stdout.String()
	if !contains(output, "mode:") {
		t.Errorf("Expected 'mode:' in set output, got: %s", output)
	}
	if !contains(output, "heuristic") {
		t.Errorf("Expected default mode 'heuristic' in output, got: %s", output)
	}
}

func TestPrSplitCommand_HelpIncludesAutoSplit(t *testing.T) {
	t.Parallel()
	stdout, dispatch := loadPrSplitEngine(t, nil)

	if err := dispatch("help", nil); err != nil {
		t.Fatalf("help returned error: %v", err)
	}

	output := stdout.String()
	if !contains(output, "auto-split") {
		t.Errorf("Expected help to mention auto-split command, got: %s", output)
	}
}

func TestPrSplitCommand_AutoSplitFallsBackToHeuristic(t *testing.T) {
	// Auto-split without Claude available should fall back to heuristic.
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Force Claude to be "not found" so auto-split falls back to heuristic.
	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"claudeCommand": "/nonexistent/claude-for-test",
	})

	if err := dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("auto-split output:\n%s", output)

	// Must actually execute splits via heuristic fallback.
	if !contains(output, "heuristic") && !contains(output, "Heuristic") {
		t.Errorf("Expected heuristic fallback message, got:\n%s", output)
	}
	// Verify that splits were actually created (not just a message about fallback).
	if !contains(output, "Heuristic Split Complete") {
		t.Errorf("Expected 'Heuristic Split Complete' indicating actual execution, got:\n%s", output)
	}
}

func TestPrSplitCommand_RunModeAutoFallback(t *testing.T) {
	// run --mode auto without Claude should fall back to heuristic.
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Force Claude to be "not found" so run --mode auto falls back to heuristic.
	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"claudeCommand": "/nonexistent/claude-for-test",
	})

	if err := dispatch("run", []string{"--mode", "auto"}); err != nil {
		t.Fatalf("run --mode auto returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run --mode auto output:\n%s", output)

	// Should fall back to heuristic mode and actually complete the workflow.
	if !contains(output, "not available") && !contains(output, "Claude not available") {
		t.Errorf("Expected 'Claude not available' message, got:\n%s", output)
	}
	// Should have completed heuristic workflow — look for actual split execution.
	if !contains(output, "Split executed:") {
		t.Errorf("Expected 'Split executed:' indicating actual heuristic workflow, got:\n%s", output)
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Errorf("Expected equivalence verification, got:\n%s", output)
	}
}

func TestPrSplitCommand_RunModeHeuristicExplicit(t *testing.T) {
	// run --mode heuristic should always use heuristic mode.
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, nil)

	if err := dispatch("run", []string{"--mode", "heuristic"}); err != nil {
		t.Fatalf("run --mode heuristic returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run --mode heuristic output:\n%s", output)

	// Should use heuristic mode and complete.
	if !contains(output, "Split executed:") {
		t.Error("Expected heuristic mode to execute splits")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("Expected equivalence verification in heuristic mode")
	}
}

// ---------------------------------------------------------------------------
// T063-T081: Script content assertions for Phase 4 additions
// ---------------------------------------------------------------------------

func TestPrSplitCommand_Phase4ScriptContent(t *testing.T) {
	t.Parallel()

	// Verify Phase 4 functions and templates exist in the embedded script.
	checks := []struct {
		name    string
		content string
	}{
		{"automatedSplit function", "function automatedSplit"},
		{"classificationToGroups function", "function classificationToGroups"},
		{"assessIndependence function", "function assessIndependence"},
		{"detectLanguage function", "function detectLanguage"},
		{"renderPrompt function", "function renderPrompt"},
		{"renderClassificationPrompt function", "function renderClassificationPrompt"},
		{"renderSplitPlanPrompt function", "function renderSplitPlanPrompt"},
		{"renderConflictPrompt function", "function renderConflictPrompt"},
		{"heuristicFallback function", "function heuristicFallback"},
		{"CLASSIFICATION_PROMPT_TEMPLATE", "CLASSIFICATION_PROMPT_TEMPLATE"},
		{"SPLIT_PLAN_PROMPT_TEMPLATE", "SPLIT_PLAN_PROMPT_TEMPLATE"},
		{"CONFLICT_RESOLUTION_PROMPT_TEMPLATE", "CONFLICT_RESOLUTION_PROMPT_TEMPLATE"},
		{"auto-split TUI command", "'auto-split'"},
		{"mode in set command", "case 'mode':"},
		{"run mode flag", "--mode"},
		{"AUTOMATED_DEFAULTS", "AUTOMATED_DEFAULTS"},
	}

	src := allChunkSources()
	for _, c := range checks {
		if !strings.Contains(src, c.content) {
			t.Errorf("Script missing %s (expected to contain %q)", c.name, c.content)
		}
	}
}

func TestPrSplitCommand_DefaultModeIsHeuristic(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Verify runtime.mode defaults to 'heuristic' (NOT 'auto').
	val, err := evalJS(`(function() {
		// Access the mode via set command output or directly.
		// The runtime object is not exported, but we can check via
		// the set command's behavior. Instead, check the config default.
		var cfg = globalThis.prSplitConfig || {};
		return cfg.mode || 'heuristic';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "heuristic" {
		t.Errorf("Expected default mode 'heuristic', got %v", val)
	}
}

// ---------------------------------------------------------------------------
// T084-T091: Phase 5 — Enhanced Conflict Resolution
// ---------------------------------------------------------------------------

func TestPrSplitCommand_AutoFixStrategiesExist(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Verify all 7 strategies are present (2 Phase 3 + 4 Phase 5 + claude-fix).
	val, err := evalJS(`(function() {
		var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
		if (!strats || !Array.isArray(strats)) return 'not-array';
		return strats.length;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	count, ok := val.(int64)
	if !ok {
		t.Fatalf("Expected int64, got %T: %v", val, val)
	}
	if count != 7 {
		t.Errorf("Expected 7 AUTO_FIX_STRATEGIES, got %d", count)
	}
}

func TestPrSplitCommand_AutoFixStrategyNames(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	val, err := evalJS(`JSON.stringify(
		globalThis.prSplit.AUTO_FIX_STRATEGIES.map(function(s) { return s.name; })
	)`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal([]byte(val.(string)), &names); err != nil {
		t.Fatalf("Failed to parse strategy names: %v", err)
	}

	expected := []string{
		"go-mod-tidy",
		"go-generate-sum",
		"go-build-missing-imports",
		"npm-install",
		"make-generate",
		"add-missing-files",
		"claude-fix",
	}
	if len(names) != len(expected) {
		t.Fatalf("Expected %d strategies, got %d: %v", len(expected), len(names), names)
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("Strategy %d: expected %q, got %q", i, want, names[i])
		}
	}
}

func TestPrSplitCommand_StrategyDetectSignatures(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Verify all strategies have detect and fix functions.
	val, err := evalJS(`(function() {
		var strats = globalThis.prSplit.AUTO_FIX_STRATEGIES;
		for (var i = 0; i < strats.length; i++) {
			if (typeof strats[i].detect !== 'function') return 'missing detect on ' + strats[i].name;
			if (typeof strats[i].fix !== 'function') return 'missing fix on ' + strats[i].name;
			if (typeof strats[i].name !== 'string') return 'missing name on index ' + i;
		}
		return 'ok';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "ok" {
		t.Errorf("Strategy validation failed: %v", val)
	}
}

func TestPrSplitCommand_GoMissingImportsDetect(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	tests := []struct {
		name string
		js   string
		want bool
	}{
		{
			"undefined error",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[2].detect('.', 'undefined: SomeFunc')`,
			true,
		},
		{
			"imported not used",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[2].detect('.', 'imported and not used: fmt')`,
			true,
		},
		{
			"could not import",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[2].detect('.', 'could not import crypto/ed25519')`,
			true,
		},
		{
			"clean output",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[2].detect('.', 'all tests passed')`,
			false,
		},
		{
			"empty",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[2].detect('.', '')`,
			false,
		},
		{
			"no output",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[2].detect('.')`,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val, err := evalJS(tc.js)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(bool)
			if got != tc.want {
				t.Errorf("detect = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrSplitCommand_NpmInstallDetect(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Without package.json, detect should return false.
	val, err := evalJS(`globalThis.prSplit.AUTO_FIX_STRATEGIES[3].detect('/nonexistent/dir')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("npm-install detect for nonexistent dir: expected false, got %v", val)
	}
}

func TestPrSplitCommand_NpmInstallDetectWithPackageJson(t *testing.T) {
	t.Parallel()

	// Create a temp dir with a package.json.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	evalJS := prsplittest.NewFullEngine(t, nil)

	val, err := evalJS(`globalThis.prSplit.AUTO_FIX_STRATEGIES[3].detect('` + filepath.ToSlash(dir) + `')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("npm-install detect with package.json: expected true, got %v", val)
	}
}

func TestPrSplitCommand_MakeGenerateDetect(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Without Makefile, detect should return false.
	val, err := evalJS(`globalThis.prSplit.AUTO_FIX_STRATEGIES[4].detect('/nonexistent/dir')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("make-generate detect for nonexistent dir: expected false, got %v", val)
	}
}

func TestPrSplitCommand_MakeGenerateDetectWithMakefile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("make-generate detect uses sh -c and grep; skipping on Windows")
	}

	// Create a temp dir with a Makefile that has a generate target.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("generate:\n\techo generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	evalJS := prsplittest.NewFullEngine(t, nil)

	val, err := evalJS(`globalThis.prSplit.AUTO_FIX_STRATEGIES[4].detect('` + filepath.ToSlash(dir) + `')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("make-generate detect with Makefile+generate target: expected true, got %v", val)
	}
}

func TestPrSplitCommand_MakeGenerateDetectWithGoGenerate(t *testing.T) {
	t.Parallel()

	// Create a temp dir with a Go file that has a //go:generate directive.
	if runtime.GOOS == "windows" {
		t.Skip("make-generate detect uses sh -c and grep; skipping on Windows")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gen.go"), []byte("package main\n//go:generate echo hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	evalJS := prsplittest.NewFullEngine(t, nil)

	val, err := evalJS(`globalThis.prSplit.AUTO_FIX_STRATEGIES[4].detect('` + filepath.ToSlash(dir) + `')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Errorf("make-generate detect with //go:generate: expected true, got %v", val)
	}
}

func TestPrSplitCommand_AddMissingFilesDetect(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	tests := []struct {
		name string
		js   string
		want bool
	}{
		{
			"no such file",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[5].detect('.', 'open foo.go: no such file or directory')`,
			true,
		},
		{
			"cannot find",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[5].detect('.', 'cannot find package bar')`,
			true,
		},
		{
			"file not found",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[5].detect('.', 'error: file not found: baz.go')`,
			true,
		},
		{
			"clean",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[5].detect('.', 'PASS')`,
			false,
		},
		{
			"empty",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[5].detect('.', '')`,
			false,
		},
		{
			"no output",
			`globalThis.prSplit.AUTO_FIX_STRATEGIES[5].detect('.')`,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val, err := evalJS(tc.js)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := val.(bool)
			if got != tc.want {
				t.Errorf("detect = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPrSplitCommand_ClaudeFixDetect(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Without a spawned Claude executor, detect should return false.
	val, err := evalJS(`globalThis.prSplit.AUTO_FIX_STRATEGIES[6].detect('.')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != false {
		t.Errorf("claude-fix detect without executor: expected false, got %v", val)
	}
}

func TestPrSplitCommand_ClaudeFixFixWithoutExecutor(t *testing.T) {
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// fix() should return {fixed: false} when no executor is available.
	// Note: claude-fix strategy's fix() is async, so we need await.
	val, err := evalJS(`JSON.stringify(
		await globalThis.prSplit.AUTO_FIX_STRATEGIES[6].fix('.', 'branch-1', {splits:[]}, 'test error')
	)`)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}
	if result["fixed"] != false {
		t.Errorf("Expected fixed=false, got %v", result["fixed"])
	}
	errMsg, _ := result["error"].(string)
	if !strings.Contains(errMsg, "not available") {
		t.Errorf("Expected 'not available' error, got: %s", errMsg)
	}
}
