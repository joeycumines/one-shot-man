package command

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  Chunk 03: Planning — createSplitPlan, savePlan, loadPlan
// ---------------------------------------------------------------------------

// TestChunk03_CreateSplitPlan_BasicGroups verifies createSplitPlan produces
// correct splits from group objects, using sanitized branch names and padded
// indices.
func TestChunk03_CreateSplitPlan_BasicGroups(t *testing.T) {
	dir := initGitRepo(t)
	evalJS := loadChunkEngine(t, map[string]interface{}{
		"baseBranch":   "main",
		"branchPrefix": "split/",
	}, "00_core", "01_analysis", "02_grouping", "03_planning")

	// Create some files and commit so we have a HEAD.
	writeFile(t, filepath.Join(dir, "a.go"), "package a")
	writeFile(t, filepath.Join(dir, "b.go"), "package b")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "init")

	result, err := evalJS(`
		(function() {
			var groups = {
				'backend': { files: ['a.go', 'b.go'], description: 'Backend changes' },
				'frontend': ['c.js', 'd.js']
			};
			return JSON.stringify(globalThis.prSplit.createSplitPlan(groups, {
				dir: '` + escapeJSPath(dir) + `',
				sourceBranch: 'main'
			}));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var plan struct {
		BaseBranch   string `json:"baseBranch"`
		SourceBranch string `json:"sourceBranch"`
		Splits       []struct {
			Name         string   `json:"name"`
			Files        []string `json:"files"`
			Message      string   `json:"message"`
			Order        int      `json:"order"`
			Dependencies []string `json:"dependencies"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &plan); err != nil {
		t.Fatal(err)
	}

	if plan.BaseBranch != "main" {
		t.Errorf("baseBranch = %q, want 'main'", plan.BaseBranch)
	}
	if len(plan.Splits) != 2 {
		t.Fatalf("got %d splits, want 2", len(plan.Splits))
	}

	// Groups are sorted alphabetically → "backend" first, "frontend" second.
	if !strings.Contains(plan.Splits[0].Name, "backend") {
		t.Errorf("split 0 name = %q, want to contain 'backend'", plan.Splits[0].Name)
	}
	if !strings.Contains(plan.Splits[1].Name, "frontend") {
		t.Errorf("split 1 name = %q, want to contain 'frontend'", plan.Splits[1].Name)
	}

	// Files are sorted.
	if len(plan.Splits[0].Files) != 2 || plan.Splits[0].Files[0] != "a.go" {
		t.Errorf("split 0 files = %v, want [a.go, b.go]", plan.Splits[0].Files)
	}

	// Description propagates as message for new format.
	if plan.Splits[0].Message != "Backend changes" {
		t.Errorf("split 0 message = %q, want 'Backend changes'", plan.Splits[0].Message)
	}

	// Legacy format (array) → message falls back to commitPrefix+name.
	if plan.Splits[1].Message != "frontend" {
		t.Errorf("split 1 message = %q, want 'frontend'", plan.Splits[1].Message)
	}

	// Dependencies: first split has none, second depends on first.
	if len(plan.Splits[0].Dependencies) != 0 {
		t.Errorf("split 0 deps = %v, want empty", plan.Splits[0].Dependencies)
	}
	if len(plan.Splits[1].Dependencies) != 1 {
		t.Errorf("split 1 deps = %v, want 1 dependency", plan.Splits[1].Dependencies)
	}
}

// TestChunk03_CreateSplitPlan_EmptyGroups verifies that an empty groups
// object produces zero splits (no panic).
func TestChunk03_CreateSplitPlan_EmptyGroups(t *testing.T) {
	_ = initGitRepo(t)
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var plan = globalThis.prSplit.createSplitPlan({}, { sourceBranch: 'main' });
			return plan.splits.length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if toInt(result) != 0 {
		t.Errorf("expected 0 splits for empty groups, got %v", result)
	}
}

// TestChunk03_CreateSplitPlan_NilGroups verifies graceful handling of nil input.
func TestChunk03_CreateSplitPlan_NilGroups(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var plan = globalThis.prSplit.createSplitPlan(null, { sourceBranch: 'main' });
			return plan.splits.length;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if toInt(result) != 0 {
		t.Errorf("expected 0 splits for null groups, got %v", result)
	}
}

// TestChunk03_CreateSplitPlan_BranchNames verifies sanitized branch names
// with padded indices.
func TestChunk03_CreateSplitPlan_BranchNames(t *testing.T) {
	evalJS := loadChunkEngine(t, map[string]interface{}{
		"branchPrefix": "pr/",
	}, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var groups = {
				'feat/login page': ['login.go'],
				'fix/bug #42': ['bug.go']
			};
			var plan = globalThis.prSplit.createSplitPlan(groups, { sourceBranch: 'main' });
			return JSON.stringify(plan.splits.map(function(s) { return s.name; }));
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	if err := json.Unmarshal([]byte(result.(string)), &names); err != nil {
		t.Fatal(err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}

	// Branch names should not contain spaces or other problematic chars.
	for _, name := range names {
		if strings.Contains(name, " ") {
			t.Errorf("branch name %q contains space", name)
		}
		// Should contain the prefix and index.
		if !strings.HasPrefix(name, "pr/") {
			t.Errorf("branch name %q doesn't start with 'pr/'", name)
		}
	}
}

// TestChunk03_SavePlan_NoPlan verifies savePlan returns error when no
// plan is cached.
func TestChunk03_SavePlan_NoPlan(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.savePlan();
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	errStr, ok := result.(string)
	if !ok || errStr == "" {
		t.Errorf("expected error string, got %v", result)
	}
	if !strings.Contains(errStr, "no plan") {
		t.Errorf("error = %q, want to mention 'no plan'", errStr)
	}
}

// TestChunk03_SaveLoadPlan_RoundTrip verifies that saving a plan and
// loading it back produces the same data.
func TestChunk03_SaveLoadPlan_RoundTrip(t *testing.T) {
	dir := initGitRepo(t)

	// Write something so the git repo has a HEAD.
	writeFile(t, filepath.Join(dir, "x.go"), "package x")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "init")

	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	planPath := filepath.Join(dir, "test-plan.json")

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;

			// Create a plan and cache it.
			var groups = { 'grp1': ['a.go'], 'grp2': ['b.go', 'c.go'] };
			var plan = prSplit.createSplitPlan(groups, {
				dir: '` + escapeJSPath(dir) + `',
				sourceBranch: 'main'
			});

			// Cache the plan (savePlan reads from prSplit._state.planCache).
			prSplit._state.planCache = plan;

			// Also cache some analysis data.
			prSplit._state.analysisCache = {
				files: ['a.go', 'b.go', 'c.go'],
				fileStatuses: { 'a.go': 'M', 'b.go': 'A', 'c.go': 'M' },
				baseBranch: 'main',
				currentBranch: 'feature'
			};

			// Save.
			var saveResult = prSplit.savePlan('` + escapeJSPath(planPath) + `');
			if (saveResult.error) return 'save error: ' + saveResult.error;

			// Clear caches.
			prSplit._state.planCache = null;
			prSplit._state.analysisCache = null;
			prSplit._state.groupsCache = null;
			prSplit._state.executionResultCache = null;

			// Load.
			var loadResult = prSplit.loadPlan('` + escapeJSPath(planPath) + `');
			if (loadResult.error) return 'load error: ' + loadResult.error;

			return JSON.stringify({
				totalSplits: loadResult.totalSplits,
				executedSplits: loadResult.executedSplits,
				pendingSplits: loadResult.pendingSplits,
				planSplitCount: prSplit._state.planCache.splits.length,
				restoredBaseBranch: prSplit._state.planCache.baseBranch,
				analysisFiles: prSplit._state.analysisCache ? prSplit._state.analysisCache.files.length : 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	str, ok := result.(string)
	if !ok {
		t.Fatalf("unexpected result type %T: %v", result, result)
	}
	if strings.HasPrefix(str, "save error:") || strings.HasPrefix(str, "load error:") {
		t.Fatal(str)
	}

	var data struct {
		TotalSplits        int    `json:"totalSplits"`
		ExecutedSplits     int    `json:"executedSplits"`
		PendingSplits      int    `json:"pendingSplits"`
		PlanSplitCount     int    `json:"planSplitCount"`
		RestoredBaseBranch string `json:"restoredBaseBranch"`
		AnalysisFiles      int    `json:"analysisFiles"`
	}
	if err := json.Unmarshal([]byte(str), &data); err != nil {
		t.Fatal(err)
	}

	if data.TotalSplits != 2 {
		t.Errorf("totalSplits = %d, want 2", data.TotalSplits)
	}
	if data.ExecutedSplits != 0 {
		t.Errorf("executedSplits = %d, want 0", data.ExecutedSplits)
	}
	if data.PendingSplits != 2 {
		t.Errorf("pendingSplits = %d, want 2", data.PendingSplits)
	}
	if data.PlanSplitCount != 2 {
		t.Errorf("planSplitCount = %d, want 2", data.PlanSplitCount)
	}
	if data.RestoredBaseBranch != "main" {
		t.Errorf("restoredBaseBranch = %q, want 'main'", data.RestoredBaseBranch)
	}
	if data.AnalysisFiles != 3 {
		t.Errorf("analysisFiles = %d, want 3", data.AnalysisFiles)
	}
}

// TestChunk03_LoadPlan_CorruptJSON verifies loadPlan returns error for
// invalid JSON.
func TestChunk03_LoadPlan_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(planPath, []byte("not json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.loadPlan('` + escapeJSPath(planPath) + `');
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	errStr, ok := result.(string)
	if !ok || errStr == "" {
		t.Fatalf("expected error string, got %v", result)
	}
	if !strings.Contains(errStr, "invalid JSON") {
		t.Errorf("error = %q, want 'invalid JSON'", errStr)
	}
}

// TestChunk03_LoadPlan_MissingFile verifies loadPlan returns error for
// non-existent file.
func TestChunk03_LoadPlan_MissingFile(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.loadPlan('/nonexistent/path/plan.json');
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	errStr, ok := result.(string)
	if !ok || errStr == "" {
		t.Fatalf("expected error string, got %v", result)
	}
	if !strings.Contains(errStr, "failed to read") {
		t.Errorf("error = %q, want 'failed to read'", errStr)
	}
}

// TestChunk03_LoadPlan_MissingSplits verifies loadPlan rejects a file with
// no splits field.
func TestChunk03_LoadPlan_MissingSplits(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "nosplits.json")
	data := `{"version":1,"plan":{}}`
	if err := os.WriteFile(planPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.loadPlan('` + escapeJSPath(planPath) + `');
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	errStr, ok := result.(string)
	if !ok || errStr == "" {
		t.Fatalf("expected error string, got %v", result)
	}
	if !strings.Contains(errStr, "missing splits") {
		t.Errorf("error = %q, want 'missing splits'", errStr)
	}
}

// TestChunk03_LoadPlan_UnsupportedVersion verifies loadPlan rejects version 0.
func TestChunk03_LoadPlan_UnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "badver.json")
	data := `{"version":0,"plan":{"splits":[]}}`
	if err := os.WriteFile(planPath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.loadPlan('` + escapeJSPath(planPath) + `');
			return r.error;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	errStr, ok := result.(string)
	if !ok || errStr == "" {
		t.Fatalf("expected error string, got %v", result)
	}
	if !strings.Contains(errStr, "unsupported plan version") {
		t.Errorf("error = %q, want 'unsupported plan version'", errStr)
	}
}

// TestChunk03_SavePlan_WithLastCompletedStep verifies snapshot version
// is bumped to 2 when lastCompletedStep is provided.
func TestChunk03_SavePlan_WithLastCompletedStep(t *testing.T) {
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "x.go"), "package x")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "init")

	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	planPath := filepath.Join(dir, "step-plan.json")

	result, err := evalJS(`
		(function() {
			var prSplit = globalThis.prSplit;
			prSplit._state.planCache = {
				baseBranch: 'main',
				splits: [{ name: 'split/01-test', files: ['a.go'] }]
			};
			var r = prSplit.savePlan('` + escapeJSPath(planPath) + `', 'classify');
			if (r.error) return 'error: ' + r.error;
			return 'ok';
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	if result != "ok" {
		t.Fatal(result)
	}

	// Read the saved file and check version.
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Version           int    `json:"version"`
		LastCompletedStep string `json:"lastCompletedStep"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 2 {
		t.Errorf("version = %d, want 2", snapshot.Version)
	}
	if snapshot.LastCompletedStep != "classify" {
		t.Errorf("lastCompletedStep = %q, want 'classify'", snapshot.LastCompletedStep)
	}
}

// TestChunk03_DefaultPlanPath verifies the exported constant.
func TestChunk03_DefaultPlanPath(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, "00_core", "01_analysis", "02_grouping", "03_planning")

	result, err := evalJS(`globalThis.prSplit.DEFAULT_PLAN_PATH`)
	if err != nil {
		t.Fatal(err)
	}
	if result != ".pr-split-plan.json" {
		t.Errorf("DEFAULT_PLAN_PATH = %q, want '.pr-split-plan.json'", result)
	}
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

// initGitRepo creates a temporary git repo and returns its path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Use init + symbolic-ref instead of init -b for compatibility
	// with git versions older than 2.28 (e.g. Windows CI).
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test")
	return dir
}

// writeFile creates a file with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// gitCmd runs a git command in a directory.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// escapeJSPath escapes a file path for embedding in a JS string literal.
func escapeJSPath(p string) string {
	return strings.ReplaceAll(p, `\`, `\\`)
}

// toInt converts a goja result to int for comparison.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return -1
	}
}
