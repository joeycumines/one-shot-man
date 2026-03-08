package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// allChunksThrough13 loads all 14 chunks for full pipeline smoke tests.
var allChunksThrough13 = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	"05_execution", "06_verification", "07_prcreation", "08_conflict",
	"09_claude", "10_pipeline", "11_utilities", "12_exports", "13_tui",
}

// setupSmokeTestRepo creates a git repo with 10+ files across 3 directories
// on a feature branch, suitable for end-to-end pipeline testing.
func setupSmokeTestRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Initial commit with files across 3 directories.
	initialFiles := []struct{ path, content string }{
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"cmd/version.go", "package main\n\nvar Version = \"1.0\"\n"},
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"pkg/util.go", "package pkg\n\nfunc Helper() string { return \"\" }\n"},
		{"docs/README.md", "# Project\n\nOverview.\n"},
	}
	for _, f := range initialFiles {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial commit")

	// Feature branch with changes across all 3 directories + new files.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	featureFiles := []struct{ path, content string }{
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"cmd/flags.go", "package main\n\nvar verbose bool\n"},
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"pkg/impl_test.go", "package pkg\n\nfunc TestBar() {}\n"},
		{"pkg/errors.go", "package pkg\n\nvar ErrNotFound = \"not found\"\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
		{"docs/api.md", "# API\n\nAPI reference.\n"},
		{"docs/changelog.md", "# Changelog\n\n## v1.1\n- Added features.\n"},
	}
	for _, f := range featureFiles {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Also modify an existing file.
	if err := os.WriteFile(filepath.Join(dir, "cmd/main.go"),
		[]byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature work")

	return dir
}

// Test_ChunkedPipeline_HeuristicRun exercises the complete pipeline end-to-end:
// heuristicFallback → createSplitPlan → executeSplit → verifySplits →
// verifyEquivalence → cleanupBranches.
func Test_ChunkedPipeline_HeuristicRun(t *testing.T) {
	t.Parallel()

	dir := setupSmokeTestRepo(t)
	escaped := escapeJSPath(dir)

	evalJS := loadChunkEngine(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"branchPrefix":  "smoke/",
		"verifyCommand": "true",
	}, allChunksThrough13...)

	// 1. Analyze diff
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: '` + escaped + `'
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff: %v", err)
	}
	var analysis struct {
		Files        []any             `json:"files"`
		FileStatuses map[string]string `json:"fileStatuses"`
		Error        *string           `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &analysis); err != nil {
		t.Fatalf("parse analyzeDiff: %v", err)
	}
	if analysis.Error != nil {
		t.Fatalf("analyzeDiff error: %s", *analysis.Error)
	}
	if len(analysis.Files) == 0 {
		t.Fatal("analyzeDiff returned zero files")
	}
	t.Logf("analyze: %d files changed", len(analysis.Files))

	// 2. Apply grouping strategy — returns {dirName: [files]} object.
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.applyStrategy(
		` + mustMarshalJSON(t, analysis.Files) + `,
		'directory',
		{ baseBranch: 'main', dir: '` + escaped + `' }
	))`)
	if err != nil {
		t.Fatalf("applyStrategy: %v", err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatalf("parse groups: %v", err)
	}
	if len(groups) < 2 {
		t.Fatalf("expected >= 2 groups, got %d", len(groups))
	}
	t.Logf("grouped into %d groups", len(groups))

	// 3. Create split plan
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.createSplitPlan(
		` + mustMarshalJSON(t, groups) + `,
		{
			sourceBranch: 'feature',
			baseBranch: 'main',
			branchPrefix: 'smoke/',
			dir: '` + escaped + `',
			fileStatuses: ` + mustMarshalJSON(t, analysis.FileStatuses) + `
		}
	))`)
	if err != nil {
		t.Fatalf("createSplitPlan: %v", err)
	}
	var plan struct {
		Splits []struct {
			Name  string   `json:"name"`
			Files []string `json:"files"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &plan); err != nil {
		t.Fatalf("parse plan: %v", err)
	}
	if len(plan.Splits) < 2 {
		t.Fatalf("expected >= 2 splits, got %d", len(plan.Splits))
	}
	t.Logf("plan: %d splits", len(plan.Splits))

	// 4. Execute split — creates branches
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escaped + `',
		verifyCommand: 'true',
		fileStatuses: ` + mustMarshalJSON(t, analysis.FileStatuses) + `,
		splits: ` + mustMarshalJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("executeSplit: %v", err)
	}
	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Name  string  `json:"name"`
			Error *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &execResult); err != nil {
		t.Fatalf("parse executeSplit: %v", err)
	}
	if execResult.Error != nil {
		t.Fatalf("executeSplit error: %s", *execResult.Error)
	}
	for _, r := range execResult.Results {
		if r.Error != nil {
			t.Errorf("split %s failed: %s", r.Name, *r.Error)
		}
	}

	// Verify branches exist.
	branches := runGitCmd(t, dir, "branch")
	for _, s := range plan.Splits {
		if !strings.Contains(branches, s.Name) {
			t.Errorf("expected branch %s, not found in: %s", s.Name, branches)
		}
	}
	t.Logf("execution: %d branches created", len(execResult.Results))

	// 5. Verify equivalence
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalence({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escaped + `',
		splits: ` + mustMarshalJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("verifyEquivalence: %v", err)
	}
	var equiv struct {
		Equivalent bool    `json:"equivalent"`
		Error      *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &equiv); err != nil {
		t.Fatalf("parse equivalence: %v", err)
	}
	if equiv.Error != nil {
		t.Fatalf("verifyEquivalence error: %s", *equiv.Error)
	}
	if !equiv.Equivalent {
		t.Error("verifyEquivalence: splits not equivalent to original diff")
	}
	t.Log("equivalence: PASS")

	// 6. Cleanup branches
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.cleanupBranches({
		baseBranch: 'main',
		dir: '` + escaped + `',
		splits: ` + mustMarshalJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("cleanupBranches: %v", err)
	}
	var cleanup struct {
		Deleted []string `json:"deleted"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &cleanup); err != nil {
		t.Fatalf("parse cleanup: %v", err)
	}
	if len(cleanup.Errors) > 0 {
		t.Errorf("cleanup errors: %v", cleanup.Errors)
	}
	if len(cleanup.Deleted) != len(plan.Splits) {
		t.Errorf("expected %d deleted branches, got %d", len(plan.Splits), len(cleanup.Deleted))
	}

	// Verify branches are gone.
	branchesAfter := runGitCmd(t, dir, "branch")
	for _, s := range plan.Splits {
		if strings.Contains(branchesAfter, s.Name) {
			t.Errorf("branch %s still exists after cleanup", s.Name)
		}
	}
	t.Log("cleanup: all branches removed")
}

// Test_ChunkedPipeline_CommandSequence exercises the interactive command
// sequence: analyze → group → plan → validate → execute → verify →
// equivalence → cleanup, each via direct globalThis.prSplit function calls.
func Test_ChunkedPipeline_CommandSequence(t *testing.T) {
	t.Parallel()

	dir := setupSmokeTestRepo(t)
	escaped := escapeJSPath(dir)

	evalJS := loadChunkEngine(t, map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"branchPrefix":  "seq/",
		"verifyCommand": "true",
	}, allChunksThrough13...)

	// cmd: analyze
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: '` + escaped + `'
	}))`)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	var analysis struct {
		Files        json.RawMessage `json:"files"`
		FileStatuses json.RawMessage `json:"fileStatuses"`
		Error        *string         `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &analysis); err != nil {
		t.Fatalf("parse analysis: %v", err)
	}
	if analysis.Error != nil {
		t.Fatalf("analyze error: %s", *analysis.Error)
	}

	// cmd: group (using selectStrategy for auto-detection)
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.selectStrategy(
		` + string(analysis.Files) + `,
		{ baseBranch: 'main', dir: '` + escaped + `' }
	))`)
	if err != nil {
		t.Fatalf("selectStrategy: %v", err)
	}
	var strategy struct {
		Strategy string `json:"strategy"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &strategy); err != nil {
		t.Fatalf("parse strategy: %v", err)
	}
	t.Logf("selected strategy: %s", strategy.Strategy)

	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.applyStrategy(
		` + string(analysis.Files) + `,
		'directory',
		{ baseBranch: 'main', dir: '` + escaped + `' }
	))`)
	if err != nil {
		t.Fatalf("applyStrategy: %v", err)
	}
	// applyStrategy returns {dirName: [files]} — pass directly to createSplitPlan.
	groupsJSON := raw.(string)

	// cmd: plan
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.createSplitPlan(
		` + groupsJSON + `,
		{
			sourceBranch: 'feature',
			baseBranch: 'main',
			branchPrefix: 'seq/',
			dir: '` + escaped + `',
			fileStatuses: ` + string(analysis.FileStatuses) + `
		}
	))`)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	planJSON := raw.(string)

	// cmd: validate — validateSplitPlan takes the splits array, not the full plan.
	var planForValidation struct {
		Splits json.RawMessage `json:"splits"`
	}
	if err := json.Unmarshal([]byte(planJSON), &planForValidation); err != nil {
		t.Fatalf("parse plan for validation: %v", err)
	}
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.validateSplitPlan(
		` + string(planForValidation.Splits) + `
	))`)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	var validation struct {
		Valid  bool     `json:"valid"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &validation); err != nil {
		t.Fatalf("parse validate: %v", err)
	}
	if !validation.Valid {
		t.Fatalf("plan validation failed: %v", validation.Errors)
	}
	t.Log("validate: PASS")

	// cmd: execute
	var plan struct {
		Splits json.RawMessage `json:"splits"`
	}
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		t.Fatalf("parse plan for execute: %v", err)
	}
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escaped + `',
		verifyCommand: 'true',
		fileStatuses: ` + string(analysis.FileStatuses) + `,
		splits: ` + string(plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var execResult struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &execResult); err != nil {
		t.Fatalf("parse execute: %v", err)
	}
	if execResult.Error != nil {
		t.Fatalf("execute error: %s", *execResult.Error)
	}
	t.Log("execute: PASS")

	// cmd: equivalence
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalence({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escaped + `',
		splits: ` + string(plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("equivalence: %v", err)
	}
	var equiv struct {
		Equivalent bool    `json:"equivalent"`
		Error      *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &equiv); err != nil {
		t.Fatalf("parse equivalence: %v", err)
	}
	if equiv.Error != nil {
		t.Fatalf("equivalence error: %s", *equiv.Error)
	}
	if !equiv.Equivalent {
		t.Error("equivalence: splits not equivalent to original diff")
	}
	t.Log("equivalence: PASS")

	// cmd: cleanup
	_, err = evalJS(`JSON.stringify(globalThis.prSplit.cleanupBranches({
		baseBranch: 'main',
		dir: '` + escaped + `',
		splits: ` + string(plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	t.Log("cleanup: PASS")
}

// mustMarshalJSON marshals v to JSON, failing the test on error.
func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}
