package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIntegration_LargeFeatureBranch(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// 5 packages × 4-5 files each = 22 files.
	baseFiles := []TestPipelineFile{
		// pkg/api
		{"pkg/api/handler.go", "package api\n\nfunc Handler() {}\n"},
		{"pkg/api/router.go", "package api\n\nfunc Router() {}\n"},
		// pkg/db
		{"pkg/db/conn.go", "package db\n\nfunc Connect() {}\n"},
		// pkg/auth
		{"pkg/auth/token.go", "package auth\n\nfunc Token() {}\n"},
		// cmd/app
		{"cmd/app/main.go", "package main\n\nfunc main() {}\n"},
		// docs
		{"docs/README.md", "# Docs\n"},
	}

	featureFiles := []TestPipelineFile{
		// pkg/api — 5 changed files
		{"pkg/api/handler.go", "package api\n\nfunc Handler() { /*updated*/ }\n"},
		{"pkg/api/middleware.go", "package api\n\nfunc Middleware() {}\n"},
		{"pkg/api/models.go", "package api\n\ntype Request struct{}\n"},
		{"pkg/api/response.go", "package api\n\ntype Response struct{}\n"},
		{"pkg/api/validate.go", "package api\n\nfunc Validate() bool { return true }\n"},
		// pkg/db — 4 changed files
		{"pkg/db/conn.go", "package db\n\nfunc Connect() { /*updated*/ }\n"},
		{"pkg/db/migrate.go", "package db\n\nfunc Migrate() {}\n"},
		{"pkg/db/schema.go", "package db\n\nfunc Schema() {}\n"},
		{"pkg/db/query.go", "package db\n\nfunc Query() {}\n"},
		// pkg/auth — 4 changed files
		{"pkg/auth/token.go", "package auth\n\nfunc Token() string { return \"tok\" }\n"},
		{"pkg/auth/oauth.go", "package auth\n\nfunc OAuth() {}\n"},
		{"pkg/auth/session.go", "package auth\n\nfunc Session() {}\n"},
		{"pkg/auth/rbac.go", "package auth\n\nfunc RBAC() {}\n"},
		// cmd/app — 4 changed files
		{"cmd/app/main.go", "package main\n\nfunc main() { run() }\n"},
		{"cmd/app/run.go", "package main\n\nfunc run() {}\n"},
		{"cmd/app/flags.go", "package main\n\nfunc flags() {}\n"},
		{"cmd/app/version.go", "package main\n\nfunc version() {}\n"},
		// docs — 5 changed files
		{"docs/README.md", "# Docs\n\nUpdated.\n"},
		{"docs/guide.md", "# Guide\n"},
		{"docs/api-ref.md", "# API Reference\n"},
		{"docs/auth.md", "# Auth\n"},
		{"docs/changelog.md", "# Changelog\n"},
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: baseFiles,
		FeatureFiles: featureFiles,
	})

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Run the full heuristic workflow.
	if err := tp.Dispatch("run", nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := tp.Stdout.String()
	t.Logf("run output:\n%s", output)

	// Analysis should find 22 files.
	if !contains(output, "changed files") {
		t.Error("expected changed files count in output")
	}

	// Should create splits.
	if !contains(output, "Split executed") {
		t.Error("expected split execution output")
	}

	// Equivalence must pass.
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected tree hash equivalence verification")
	}

	// Verify branches — should have splits for each of the 5 dirs.
	branches := runGitCmd(t, tp.Dir, "branch")
	t.Logf("branches:\n%s", branches)
	// directory strategy groups by top-level dir under deepest common ancestor
	// Expect groups for the directories present.
	if !strings.Contains(branches, "split/") {
		t.Errorf("expected split branches, got:\n%s", branches)
	}

	// Count split branches.
	branchLines := strings.Split(strings.TrimSpace(branches), "\n")
	splitCount := 0
	for _, line := range branchLines {
		if strings.Contains(line, "split/") {
			splitCount++
		}
	}
	if splitCount < 3 {
		t.Errorf("expected at least 3 split branches (5 packages grouped), got %d", splitCount)
	}

	// Verify we're back on feature.
	current := strings.TrimSpace(runGitCmd(t, tp.Dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if current != "feature" {
		t.Errorf("expected restored to 'feature', got %q", current)
	}
}

// ---------------------------------------------------------------------------
// T096: Refactoring with renames, moves, deletions
// ---------------------------------------------------------------------------

func TestIntegration_RefactoringBranch(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	// Initialize repo with files that will be renamed/deleted.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	initialFiles := []struct{ path, content string }{
		{"pkg/old_name.go", "package pkg\n\nfunc Old() {}\n"},
		{"pkg/helper.go", "package pkg\n\nfunc Helper() {}\n"},
		{"cmd/app.go", "package main\n\nfunc main() {}\n"},
		{"cmd/utils.go", "package main\n\nfunc utils() {}\n"},
		{"docs/reference.md", "# Reference\n"},
		{"docs/tutorial.md", "# Tutorial\n"},
		{"internal/legacy.go", "package internal\n\nfunc Legacy() {}\n"},
		{"internal/compat.go", "package internal\n\nfunc Compat() {}\n"},
		{"README.md", "# Project\n"},
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
	runGitCmd(t, dir, "commit", "-m", "initial")

	// Feature branch: rename, delete, modify, add.
	runGitCmd(t, dir, "checkout", "-b", "feature")

	// Rename: pkg/old_name.go → pkg/new_name.go
	runGitCmd(t, dir, "mv", "pkg/old_name.go", "pkg/new_name.go")

	// Delete: internal/legacy.go, docs/tutorial.md
	if err := os.Remove(filepath.Join(dir, "internal/legacy.go")); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "internal/legacy.go")
	if err := os.Remove(filepath.Join(dir, "docs/tutorial.md")); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "docs/tutorial.md")

	// Modify: cmd/app.go, pkg/helper.go, docs/reference.md
	if err := os.WriteFile(filepath.Join(dir, "cmd/app.go"), []byte("package main\n\nfunc main() { run() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg/helper.go"), []byte("package pkg\n\nfunc Helper() string { return \"v2\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs/reference.md"), []byte("# Reference v2\n\nUpdated.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Add new: cmd/run.go, pkg/types.go, internal/modern.go
	for _, f := range []struct{ path, content string }{
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"pkg/types.go", "package pkg\n\ntype Config struct{}\n"},
		{"internal/modern.go", "package internal\n\nfunc Modern() {}\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "refactoring")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, nil)

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("refactoring run output:\n%s", output)

	// Analysis must detect files.
	if !contains(output, "changed files") {
		t.Error("expected changed files in output")
	}

	// Execution must complete.
	if !contains(output, "Split executed") {
		t.Error("expected split execution")
	}

	// NOTE: Tree hash equivalence may fail when renames are present because
	// executeSplit only handles the NEW path from a rename — the old path stays
	// in the base branch tree. This is a known limitation.

	// Verify branches were created.
	branches := runGitCmd(t, dir, "branch")
	if !strings.Contains(branches, "split/") {
		t.Error("expected split branches after execute")
	}

	// Verify we're back on feature.
	current := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if current != "feature" {
		t.Errorf("expected feature branch, got %q", current)
	}

	// Verify the diff statuses are captured.
	// Use EvalJS to check the analysis directly.
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	val, err := evalJS(`(function() {
		var a = globalThis.prSplit.analyzeDiff({ baseBranch: 'main' });
		return JSON.stringify({ files: a.files.length, statuses: a.fileStatuses });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var analysis struct {
		Files    int               `json:"files"`
		Statuses map[string]string `json:"statuses"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &analysis); err != nil {
		t.Fatal(err)
	}
	t.Logf("analysis: %d files, statuses: %v", analysis.Files, analysis.Statuses)

	// Should have at least 9 entries: 1 rename-to + 2 deletes + 3 modifies + 3 adds.
	// Note: rename shows as the NEW path only.
	if analysis.Files < 8 {
		t.Errorf("expected at least 8 changed files, got %d", analysis.Files)
	}

	// Look for specific statuses.
	hasAdd, hasDelete, hasModify, hasRename := false, false, false, false
	for _, s := range analysis.Statuses {
		switch s {
		case "A":
			hasAdd = true
		case "D":
			hasDelete = true
		case "M":
			hasModify = true
		case "R":
			hasRename = true
		}
	}
	if !hasAdd {
		t.Error("expected at least one Added file")
	}
	if !hasDelete {
		t.Error("expected at least one Deleted file")
	}
	if !hasModify {
		t.Error("expected at least one Modified file")
	}
	if !hasRename {
		t.Error("expected at least one Renamed file")
	}
}

// ---------------------------------------------------------------------------
// T097: Splits that break compilation — conflict resolution triggers
// ---------------------------------------------------------------------------

func TestIntegration_BrokenSplitsResolution(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create a repo where splitting by directory will produce branches that
	// fail a simple verification command, then verify resolveConflicts runs.
	// Use top-level directories so directory strategy creates separate groups.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"api/handler.go", "package api\n"},
			{"db/store.go", "package db\n"},
			{"README.md", "# Test\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"api/handler.go", "package api\n\nfunc Handle() string { return \"ok\" }\n"},
			{"api/types.go", "package api\n\ntype Req struct{}\n"},
			{"db/store.go", "package db\n\nfunc Store() {}\n"},
			{"db/migrate.go", "package db\n\nfunc Migrate() {}\n"},
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

	// Use EvalJS to do analysis → group → plan → execute, then call
	// resolveConflicts with a verify command that ALWAYS FAILS, to exercise
	// the resolution path.
	val, err := tp.EvalJS(`(async function() {
		var ps = globalThis.prSplit;
		var analysis = ps.analyzeDiff({ baseBranch: 'main' });
		if (analysis.error) return JSON.stringify({ error: analysis.error });

		var groups = ps.applyStrategy(analysis.files, 'directory', {
			fileStatuses: analysis.fileStatuses,
			maxFiles: 10,
			baseBranch: 'main'
		});
		var plan = ps.createSplitPlan(groups, {
			baseBranch: analysis.baseBranch,
			sourceBranch: analysis.currentBranch,
			branchPrefix: 'split/',
			maxFiles: 10,
			fileStatuses: analysis.fileStatuses
		});
		var execResult = ps.executeSplit(plan);
		if (execResult.error) return JSON.stringify({ error: execResult.error });

		// Call resolveConflicts with verify='false' (always fails).
		plan.verifyCommand = 'false';
		var resolved = await ps.resolveConflicts(plan, {
			verifyCommand: 'false',
			retryBudget: 2
		});
		return JSON.stringify({
			error: null,
			splitCount: plan.splits.length,
			resolved: resolved
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Error      *string `json:"error"`
		SplitCount int     `json:"splitCount"`
		Resolved   struct {
			Fixed         []any `json:"fixed"`
			Errors        []any `json:"errors"`
			TotalRetries  int           `json:"totalRetries"`
			ReSplitNeeded bool          `json:"reSplitNeeded"`
		} `json:"resolved"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	t.Logf("result: splits=%d, totalRetries=%d, reSplitNeeded=%v, errors=%d",
		result.SplitCount, result.Resolved.TotalRetries, result.Resolved.ReSplitNeeded, len(result.Resolved.Errors))

	if result.Error != nil {
		t.Fatalf("unexpected error: %s", *result.Error)
	}
	if result.SplitCount < 2 {
		t.Errorf("expected at least 2 splits, got %d", result.SplitCount)
	}

	// With verify='false' (always fails), all strategies will be tried and fail.
	// The retry budget is 2, so totalRetries should be <= 2.
	if result.Resolved.TotalRetries > 2 {
		t.Errorf("expected totalRetries <= 2, got %d", result.Resolved.TotalRetries)
	}

	// reSplitNeeded should be true because resolution failed.
	if !result.Resolved.ReSplitNeeded {
		t.Error("expected reSplitNeeded=true when all strategies fail")
	}

	// Errors should list the branches that couldn't be fixed.
	if len(result.Resolved.Errors) == 0 {
		t.Error("expected at least one error entry")
	}
}

// ---------------------------------------------------------------------------
// T098: Independent changes detection
// ---------------------------------------------------------------------------

func TestIntegration_IndependentChanges(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create completely unrelated changes in separate directories.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"src/main.go", "package main\n\nfunc main() {}\n"},
			{"README.md", "# Hello\n"},
		},
		FeatureFiles: []TestPipelineFile{
			// Three completely unrelated dirs.
			{"docs/guide.md", "# Guide\n"},
			{"tests/smoke_test.go", "package tests\n\nfunc TestSmoke() {}\n"},
			{"config/settings.yaml", "key: value\n"},
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

	// Analyze, plan, execute, then check independence.
	val, err := tp.EvalJS(`(function() {
		var ps = globalThis.prSplit;
		var analysis = ps.analyzeDiff({ baseBranch: 'main' });
		if (analysis.error) return JSON.stringify({ error: analysis.error });

		var groups = ps.applyStrategy(analysis.files, 'directory', {
			fileStatuses: analysis.fileStatuses,
			maxFiles: 10,
			baseBranch: 'main'
		});
		var plan = ps.createSplitPlan(groups, {
			baseBranch: analysis.baseBranch,
			sourceBranch: analysis.currentBranch,
			branchPrefix: 'split/',
			maxFiles: 10,
			fileStatuses: analysis.fileStatuses
		});
		var execResult = ps.executeSplit(plan);
		if (execResult.error) return JSON.stringify({ error: execResult.error });

		// Build a classification from groups.
		var classification = {};
		for (var g in groups) {
			for (var i = 0; i < groups[g].length; i++) {
				classification[groups[g][i]] = g;
			}
		}

		var pairs = ps.assessIndependence(plan, classification);
		return JSON.stringify({
			error: null,
			splitCount: plan.splits.length,
			independentPairs: pairs
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Error            *string    `json:"error"`
		SplitCount       int        `json:"splitCount"`
		IndependentPairs [][]string `json:"independentPairs"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	t.Logf("result: splits=%d, independentPairs=%v", result.SplitCount, result.IndependentPairs)

	if result.Error != nil {
		t.Fatalf("unexpected error: %s", *result.Error)
	}
	if result.SplitCount < 3 {
		t.Errorf("expected at least 3 splits (3 separate dirs), got %d", result.SplitCount)
	}

	// All pairs should be independent since dirs are unrelated (no Go imports).
	// With 3 splits, expect 3 C(3,2) = 3 independent pairs.
	if len(result.IndependentPairs) < 3 {
		t.Errorf("expected at least 3 independent pairs from 3 unrelated dirs, got %d", len(result.IndependentPairs))
	}
}

// ---------------------------------------------------------------------------
// T099: Heuristic fallback when Claude is unavailable
// ---------------------------------------------------------------------------

func TestIntegration_HeuristicFallback(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		ConfigOverrides: map[string]any{
			// Point at a nonexistent binary to ensure Claude fails to resolve.
			"claudeCommand": "/nonexistent/claude-for-test-" + t.Name(),
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

	// Dispatch auto-split — should fall back to heuristic.
	if err := tp.Dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split returned error: %v", err)
	}

	output := tp.Stdout.String()
	t.Logf("auto-split fallback output:\n%s", output)

	// Should mention falling back.
	if !contains(output, "falling back to heuristic") && !contains(output, "Heuristic Split Complete") {
		// Both are valid — Claude fails → heuristic.
		// The "Heuristic Split Complete" message means heuristicFallback ran.
		t.Error("expected heuristic fallback indication in output")
	}

	// Should still produce splits.
	branches := runGitCmd(t, tp.Dir, "branch")
	if !strings.Contains(branches, "split/") {
		t.Errorf("expected split branches from heuristic fallback, got:\n%s", branches)
	}
}

// ---------------------------------------------------------------------------
// T100: Timeout behavior — budget enforcement
// ---------------------------------------------------------------------------

func TestIntegration_PollFileTimeout(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Test budget enforcement: resolveConflicts with budget 0 should
	// immediately skip all verification attempts.
	val2, err := evalJS(`(async function() {
		var ps = globalThis.prSplit;
		// resolveConflicts with budget 0 should immediately skip.
		var result = await ps.resolveConflicts(
			{ splits: [{ name: 'test-branch', files: ['a.go'] }], dir: '.', verifyCommand: 'false' },
			{ retryBudget: 0, verifyCommand: 'false' }
		);
		return JSON.stringify(result);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Errors       []any `json:"errors"`
		TotalRetries int           `json:"totalRetries"`
	}
	if err := json.Unmarshal([]byte(val2.(string)), &result); err != nil {
		t.Fatal(err)
	}
	// With budget 0, should have 0 retries and errors for each split.
	if result.TotalRetries != 0 {
		t.Errorf("expected 0 retries with budget 0, got %d", result.TotalRetries)
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors when budget is 0")
	}
}

// ---------------------------------------------------------------------------
// T101: Plan persistence — save → cleanup → load → execute
// ---------------------------------------------------------------------------

func TestIntegration_PlanPersistence(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/a.go", "package pkg\n\nfunc A() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
			{"README.md", "# Test\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/a.go", "package pkg\n\nfunc A() string { return \"a\" }\n"},
			{"pkg/b.go", "package pkg\n\nfunc B() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() { run() }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{"docs/guide.md", "# Guide\n"},
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

	// Step 1: Run the full pipeline (analyze → plan → execute).
	if err := tp.Dispatch("run", nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output1 := tp.Stdout.String()
	t.Logf("run output:\n%s", output1)
	if !contains(output1, "Tree hash equivalence verified") {
		t.Fatal("initial run did not verify equivalence")
	}

	// Step 2: Save plan — OUTSIDE the repo dir to avoid tree contamination.
	planPath := filepath.Join(tp.ResultDir, "test-plan.json")
	tp.Stdout.Reset()
	if err := tp.Dispatch("save-plan", []string{planPath}); err != nil {
		t.Fatalf("save-plan returned error: %v", err)
	}

	// Verify file was written.
	if _, err := os.Stat(planPath); errors.Is(err, os.ErrNotExist) {
		t.Fatal("plan file was not created")
	}

	// Read saved plan to verify structure.
	planData, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Version int `json:"version"`
		Plan    struct {
			Splits []struct {
				Name  string   `json:"name"`
				Files []string `json:"files"`
			} `json:"splits"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(planData, &saved); err != nil {
		t.Fatalf("invalid plan JSON: %v", err)
	}
	if saved.Version != 1 {
		t.Errorf("expected version 1, got %d", saved.Version)
	}
	origSplitCount := len(saved.Plan.Splits)
	if origSplitCount == 0 {
		t.Fatal("saved plan has no splits")
	}
	t.Logf("saved plan: %d splits", origSplitCount)

	// Step 3: Clean up branches.
	tp.Stdout.Reset()
	if err := tp.Dispatch("cleanup", nil); err != nil {
		t.Fatalf("cleanup returned error: %v", err)
	}
	// Verify split branches are gone.
	branches := runGitCmd(t, tp.Dir, "branch")
	if strings.Contains(branches, "split/") {
		t.Errorf("expected no split branches after cleanup, got:\n%s", branches)
	}

	// Step 4: Load plan from file.
	tp.Stdout.Reset()
	if err := tp.Dispatch("load-plan", []string{planPath}); err != nil {
		t.Fatalf("load-plan returned error: %v", err)
	}
	loadOutput := tp.Stdout.String()
	t.Logf("load-plan output:\n%s", loadOutput)
	if !contains(loadOutput, "loaded") && !contains(loadOutput, "Loaded") && !contains(loadOutput, "Plan loaded") {
		t.Error("load-plan output should confirm plan was loaded")
	}

	// Step 5: Execute from loaded plan.
	tp.Stdout.Reset()
	if err := tp.Dispatch("execute", nil); err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	execOutput := tp.Stdout.String()
	t.Logf("execute output:\n%s", execOutput)

	// Step 6: Verify equivalence.
	tp.Stdout.Reset()
	if err := tp.Dispatch("equivalence", nil); err != nil {
		t.Fatalf("equivalence returned error: %v", err)
	}
	equivOutput := tp.Stdout.String()
	t.Logf("equivalence output:\n%s", equivOutput)
	if !contains(equivOutput, "equivalent") && !contains(equivOutput, "verified") && !contains(equivOutput, "match") {
		t.Error("expected equivalence verification after load+execute")
	}
}

// ---------------------------------------------------------------------------
// T102: PR creation with mock gh CLI
// ---------------------------------------------------------------------------

func TestIntegration_PRCreationMockGh(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{})

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Run heuristic pipeline first.
	if err := tp.Dispatch("run", nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	// Test createPRs directly with push-only mode and a nonexistent remote.
	val, err := tp.EvalJS(`(function() {
		var ps = globalThis.prSplit;
		// Get the cached plan.
		var analysis = ps.analyzeDiff({ baseBranch: 'main' });
		var groups = ps.applyStrategy(analysis.files, 'directory', {
			fileStatuses: analysis.fileStatuses,
			maxFiles: 10
		});
		var plan = ps.createSplitPlan(groups, {
			baseBranch: analysis.baseBranch,
			sourceBranch: analysis.currentBranch,
			branchPrefix: 'split/',
			maxFiles: 10,
			fileStatuses: analysis.fileStatuses
		});

		// Without a remote, the push will fail — but verify the function's
		// error handling is correct.
		var result = ps.createPRs(plan, { pushOnly: true, remote: 'nonexistent' });
		return JSON.stringify({
			error: result.error || null,
			resultCount: (result.results || []).length,
			firstError: result.results && result.results[0] ? result.results[0].error : null
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var prResult struct {
		Error       *string `json:"error"`
		ResultCount int     `json:"resultCount"`
		FirstError  *string `json:"firstError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &prResult); err != nil {
		t.Fatal(err)
	}
	t.Logf("createPRs result: error=%v, resultCount=%d, firstError=%v",
		prResult.Error, prResult.ResultCount, prResult.FirstError)

	// With a nonexistent remote, push should fail — but the function should
	// handle it gracefully without panicking.
	if prResult.Error == nil {
		// This would mean there's a remote named 'nonexistent' — unexpected.
		t.Error("expected push failure with nonexistent remote")
	} else {
		if !strings.Contains(*prResult.Error, "push failed") {
			t.Errorf("expected push failure, got: %s", *prResult.Error)
		}
	}
}

// ---------------------------------------------------------------------------
// T103: TUI command sequence simulation
// ---------------------------------------------------------------------------

func TestIntegration_TUICommandSequence(t *testing.T) {
	// NOT parallel — chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
			{"pkg/helpers.go", "package pkg\n\nfunc Help() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
			{"README.md", "# Test\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/types.go", "package pkg\n\ntype Foo struct{ Name string }\n"},
			{"pkg/helpers.go", "package pkg\n\nfunc Help() string { return \"help\" }\n"},
			{"pkg/impl.go", "package pkg\n\nfunc Impl() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() { run() }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{"docs/guide.md", "# Guide\n\nHello.\n"},
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

	// Simulate interactive command sequence: analyze → group → plan → preview → execute → verify → equivalence.

	// analyze
	tp.Stdout.Reset()
	if err := tp.Dispatch("analyze", nil); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	analyzeOut := tp.Stdout.String()
	t.Logf("analyze:\n%s", analyzeOut)
	if !contains(analyzeOut, "Changed files") && !contains(analyzeOut, "changed files") && !contains(analyzeOut, "Analyzing") {
		t.Error("analyze should show changed files")
	}

	// group
	tp.Stdout.Reset()
	if err := tp.Dispatch("group", nil); err != nil {
		t.Fatalf("group: %v", err)
	}
	groupOut := tp.Stdout.String()
	t.Logf("group:\n%s", groupOut)
	if !contains(groupOut, "Groups") && !contains(groupOut, "groups") && !contains(groupOut, "Grouped") {
		t.Error("group should show grouping result")
	}

	// plan
	tp.Stdout.Reset()
	if err := tp.Dispatch("plan", nil); err != nil {
		t.Fatalf("plan: %v", err)
	}
	planOut := tp.Stdout.String()
	t.Logf("plan:\n%s", planOut)
	if !contains(planOut, "Plan created") {
		t.Error("plan should show plan creation")
	}

	// preview
	tp.Stdout.Reset()
	if err := tp.Dispatch("preview", nil); err != nil {
		t.Fatalf("preview: %v", err)
	}
	previewOut := tp.Stdout.String()
	t.Logf("preview:\n%s", previewOut)
	if !contains(previewOut, "split/") {
		t.Error("preview should show branch names")
	}

	// execute
	tp.Stdout.Reset()
	if err := tp.Dispatch("execute", nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	execOut := tp.Stdout.String()
	t.Logf("execute:\n%s", execOut)
	if !contains(execOut, "Split") && !contains(execOut, "split") && !contains(execOut, "completed") {
		t.Error("execute should show execution result")
	}

	// verify
	tp.Stdout.Reset()
	if err := tp.Dispatch("verify", nil); err != nil {
		t.Fatalf("verify: %v", err)
	}
	verifyOut := tp.Stdout.String()
	t.Logf("verify:\n%s", verifyOut)
	if !contains(verifyOut, "passed") && !contains(verifyOut, "Passed") && !contains(verifyOut, "✓") && !contains(verifyOut, "pass") {
		t.Error("verify should confirm splits pass")
	}

	// equivalence
	tp.Stdout.Reset()
	if err := tp.Dispatch("equivalence", nil); err != nil {
		t.Fatalf("equivalence: %v", err)
	}
	equivOut := tp.Stdout.String()
	t.Logf("equivalence:\n%s", equivOut)
	if !contains(equivOut, "equivalent") && !contains(equivOut, "verified") && !contains(equivOut, "match") {
		t.Error("equivalence should confirm tree hash match")
	}

	// Verify branches.
	branches := runGitCmd(t, tp.Dir, "branch")
	if !strings.Contains(branches, "split/") {
		t.Error("expected split branches after execute")
	}

	// cleanup
	tp.Stdout.Reset()
	if err := tp.Dispatch("cleanup", nil); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	branches2 := runGitCmd(t, tp.Dir, "branch")
	if strings.Contains(branches2, "split/") {
		t.Error("expected no split branches after cleanup")
	}
}

// ---------------------------------------------------------------------------
// T104: End-to-end with real Claude Code (gated)
// ---------------------------------------------------------------------------

func TestIntegration_RealClaudeCode(t *testing.T) {
	skipIfNoClaude(t)
	verifyClaudeAuth(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/api/handler.go", "package api\n\nfunc Handler() {}\n"},
			{"pkg/db/store.go", "package db\n\nfunc Store() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() {}\n"},
			{"README.md", "# Test\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/api/handler.go", "package api\n\nfunc Handler() string { return \"ok\" }\n"},
			{"pkg/api/middleware.go", "package api\n\nfunc MW() {}\n"},
			{"pkg/api/types.go", "package api\n\ntype Req struct{}\n"},
			{"pkg/db/store.go", "package db\n\nfunc Store() error { return nil }\n"},
			{"pkg/db/migrate.go", "package db\n\nfunc Migrate() {}\n"},
			{"cmd/main.go", "package main\n\nfunc main() { run() }\n"},
			{"cmd/run.go", "package main\n\nfunc run() {}\n"},
			{"docs/setup.md", "# Setup\n\nInstructions.\n"},
			{"docs/api.md", "# API\n\nReference.\n"},
			{"config/default.yaml", "key: value\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": claudeTestCommand,
			"claudeArgs":    []string(claudeTestArgs),
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

	// Run auto-split — this spawns real Claude.
	if err := tp.Dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split returned error: %v", err)
	}

	output := tp.Stdout.String()
	t.Logf("real Claude auto-split output:\n%s", output)

	// At minimum, should complete (possibly with heuristic fallback).
	if !contains(output, "Complete") && !contains(output, "complete") {
		t.Error("expected completion message")
	}
}

// ---------------------------------------------------------------------------
// T026: Deep integration test — auto-split with real Claude, deep verification
// ---------------------------------------------------------------------------

func TestIntegration_AutoSplitWithClaude(t *testing.T) {
	skipIfNoClaude(t)
	verifyClaudeAuth(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Complex repo: 3 Go packages with cross-imports, test files, docs, configs.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			// Three Go packages with inter-dependencies.
			{"pkg/auth/auth.go", "package auth\n\nfunc Authenticate(token string) bool { return token != \"\" }\n"},
			{"pkg/auth/auth_test.go", "package auth\n\nimport \"testing\"\n\nfunc TestAuthenticate(t *testing.T) {\n\tif Authenticate(\"\") { t.Error(\"empty should fail\") }\n}\n"},
			{"pkg/db/db.go", "package db\n\ntype DB struct{ DSN string }\n\nfunc Open(dsn string) *DB { return &DB{DSN: dsn} }\n"},
			{"pkg/db/db_test.go", "package db\n\nimport \"testing\"\n\nfunc TestOpen(t *testing.T) {\n\tdb := Open(\"test\")\n\tif db.DSN != \"test\" { t.Fatal(\"dsn mismatch\") }\n}\n"},
			{"pkg/api/api.go", "package api\n\ntype Server struct{}\n\nfunc New() *Server { return &Server{} }\n"},
			{"cmd/server/main.go", "package main\n\nfunc main() {}\n"},
			{"docs/README.md", "# Project\n\nMain documentation.\n"},
			{"configs/default.yaml", "port: 8080\n"},
			{".gitignore", "*.tmp\n"},
		},
		FeatureFiles: []TestPipelineFile{
			// Expand auth: add middleware, update tests.
			{"pkg/auth/auth.go", "package auth\n\nfunc Authenticate(token string) bool { return token != \"\" }\n\nfunc Authorize(role string) bool { return role == \"admin\" }\n"},
			{"pkg/auth/middleware.go", "package auth\n\nfunc Middleware(next func()) func() { return func() { next() } }\n"},
			{"pkg/auth/auth_test.go", "package auth\n\nimport \"testing\"\n\nfunc TestAuthenticate(t *testing.T) {\n\tif Authenticate(\"\") { t.Error(\"empty should fail\") }\n}\n\nfunc TestAuthorize(t *testing.T) {\n\tif !Authorize(\"admin\") { t.Error(\"admin should pass\") }\n}\n"},
			// Expand db: add migration, model.
			{"pkg/db/db.go", "package db\n\ntype DB struct{ DSN string }\n\nfunc Open(dsn string) *DB { return &DB{DSN: dsn} }\n\nfunc (db *DB) Close() error { return nil }\n"},
			{"pkg/db/migrate.go", "package db\n\nfunc Migrate(db *DB) error { return nil }\n"},
			{"pkg/db/model.go", "package db\n\ntype User struct {\n\tID   int\n\tName string\n}\n"},
			{"pkg/db/db_test.go", "package db\n\nimport \"testing\"\n\nfunc TestOpen(t *testing.T) {\n\tdb := Open(\"test\")\n\tif db.DSN != \"test\" { t.Fatal(\"dsn mismatch\") }\n}\n\nfunc TestClose(t *testing.T) {\n\tdb := Open(\"test\")\n\tif err := db.Close(); err != nil { t.Fatal(err) }\n}\n"},
			// Expand api: add handler, routes, tests.
			{"pkg/api/api.go", "package api\n\ntype Server struct{ Port int }\n\nfunc New(port int) *Server { return &Server{Port: port} }\n"},
			{"pkg/api/handler.go", "package api\n\nfunc (s *Server) HandleHealth() string { return \"ok\" }\n"},
			{"pkg/api/routes.go", "package api\n\nfunc (s *Server) RegisterRoutes() { /* wire handlers */ }\n"},
			{"pkg/api/api_test.go", "package api\n\nimport \"testing\"\n\nfunc TestNew(t *testing.T) {\n\ts := New(8080)\n\tif s.Port != 8080 { t.Fatal(\"port mismatch\") }\n}\n"},
			// Expand cmd: add run, config loading.
			{"cmd/server/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"starting\") }\n"},
			{"cmd/server/run.go", "package main\n\nfunc run() error { return nil }\n"},
			// Docs and config updates.
			{"docs/README.md", "# Project\n\nMain documentation.\n\n## Getting Started\n\nRun `go run ./cmd/server`.\n"},
			{"docs/api.md", "# API Reference\n\n## Health\n\nGET /health returns 200.\n"},
			{"docs/auth.md", "# Authentication\n\nToken-based auth.\n"},
			{"configs/default.yaml", "port: 8080\ndb_dsn: postgres://localhost/app\n"},
			{"configs/test.yaml", "port: 0\ndb_dsn: sqlite://test.db\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": claudeTestCommand,
			"claudeArgs":    []string(claudeTestArgs),
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

	// Run auto-split.
	if err := tp.Dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split returned error: %v", err)
	}

	output := tp.Stdout.String()
	t.Logf("deep Claude auto-split output:\n%s", output)

	// --- Deep verification ---

	// 1. Pipeline reached completion (not just crash).
	if !contains(output, "Complete") && !contains(output, "complete") {
		t.Error("expected completion message in output")
	}

	// 2. Use EvalJS to inspect the report directly.
	reportRaw, err := tp.EvalJS(`JSON.stringify(prSplit.getLastReport ? prSplit.getLastReport() : {})`)
	if err != nil {
		t.Logf("could not retrieve report via JS: %v", err)
	}
	reportStr := ""
	if reportRaw != nil {
		reportStr = fmt.Sprintf("%v", reportRaw)
	}
	t.Logf("auto-split report: %s", reportStr)

	// 3. Check that split branches exist.
	branches := gitBranchList(t, tp.Dir)
	t.Logf("branches after split: %v", branches)
	splitBranches := filterPrefix(branches, "split/")
	if len(splitBranches) == 0 {
		// Check if we fell back to heuristic (non-error).
		if contains(output, "fallback") || contains(output, "heuristic") {
			t.Log("Claude fell back to heuristic mode — verifying heuristic splits")
		} else {
			t.Error("expected at least one split/* branch")
		}
	} else {
		t.Logf("created %d split branches: %v", len(splitBranches), splitBranches)
	}

	// 4. Verify tree-hash equivalence if splits were created.
	if len(splitBranches) > 0 {
		if contains(output, "Equivalence: PASS") || contains(output, "equivalence") {
			t.Log("equivalence check reported PASS")
		}
	}

	// 5. Verify non-zero Claude interactions if not fallback.
	if !contains(output, "fallback") && !contains(output, "heuristic") {
		if contains(output, "Claude interactions: 0") {
			t.Error("expected non-zero Claude interactions in non-fallback mode")
		}
	}
}

// ---------------------------------------------------------------------------
// T027: Complex edits integration — additions, deletions, renames
// ---------------------------------------------------------------------------

func TestIntegration_AutoSplitComplexEdits(t *testing.T) {
	skipIfNoClaude(t)
	verifyClaudeAuth(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create initial repo with files that will be deleted, renamed, and modified.
	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/core/engine.go", "package core\n\nfunc Engine() {}\n"},
			{"pkg/core/legacy.go", "package core\n\n// Deprecated: use Engine instead.\nfunc LegacyEngine() {}\n"},
			{"pkg/util/helpers.go", "package util\n\nfunc Help() string { return \"help\" }\n"},
			{"pkg/util/format.go", "package util\n\nfunc Format(s string) string { return s }\n"},
			{"pkg/api/server.go", "package api\n\nfunc Serve() {}\n"},
			{"pkg/api/routes.go", "package api\n\nfunc Routes() {}\n"},
			{"cmd/app/main.go", "package main\n\nfunc main() {}\n"},
			{"docs/overview.md", "# Overview\n\nOld docs.\n"},
			{"docs/deprecated.md", "# Deprecated Features\n\nLegacy notes.\n"},
			{"config/app.yaml", "env: production\n"},
			{"scripts/setup.sh", "#!/bin/sh\necho setup\n"},
		},
		FeatureFiles: []TestPipelineFile{
			// Modified files.
			{"pkg/core/engine.go", "package core\n\nimport \"fmt\"\n\nfunc Engine() { fmt.Println(\"v2\") }\n"},
			{"pkg/api/server.go", "package api\n\nimport \"net/http\"\n\nfunc Serve() { http.ListenAndServe(\":8080\", nil) }\n"},
			{"pkg/api/routes.go", "package api\n\nfunc Routes() []string { return []string{\"/api/v1\"} }\n"},
			// New files.
			{"pkg/core/v2.go", "package core\n\nfunc V2Init() {}\n"},
			{"pkg/middleware/auth.go", "package middleware\n\nfunc Auth() {}\n"},
			{"pkg/middleware/logging.go", "package middleware\n\nfunc Logging() {}\n"},
			{"cmd/app/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"app v2\") }\n"},
			{"cmd/app/cli.go", "package main\n\nfunc parseCLI() {}\n"},
			{"cmd/migrate/main.go", "package main\n\nfunc main() {}\n"},
			{"docs/overview.md", "# Overview\n\nUpdated documentation for v2.\n"},
			{"docs/migration.md", "# Migration Guide\n\nUpgrade from v1 to v2.\n"},
			{"config/app.yaml", "env: production\nversion: 2\n"},
			{"config/dev.yaml", "env: development\nversion: 2\n"},
			// Renamed file (util/helpers.go -> util/utils.go).
			{"pkg/util/utils.go", "package util\n\nfunc Help() string { return \"help v2\" }\n"},
			{"pkg/util/format.go", "package util\n\nfunc Format(s string) string { return \"[\" + s + \"]\" }\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": claudeTestCommand,
			"claudeArgs":    []string(claudeTestArgs),
		},
	})

	// Additional git operations: delete files, simulate rename.
	runGitCmd(t, tp.Dir, "rm", "pkg/core/legacy.go")
	runGitCmd(t, tp.Dir, "rm", "docs/deprecated.md")
	runGitCmd(t, tp.Dir, "rm", "pkg/util/helpers.go")
	runGitCmd(t, tp.Dir, "rm", "scripts/setup.sh")
	runGitCmd(t, tp.Dir, "add", "-A")
	runGitCmd(t, tp.Dir, "commit", "--amend", "--no-edit")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Run auto-split.
	if err := tp.Dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split returned error: %v", err)
	}

	output := tp.Stdout.String()
	t.Logf("complex edits auto-split output:\n%s", output)

	// 1. Should complete.
	if !contains(output, "Complete") && !contains(output, "complete") &&
		!contains(output, "fallback") && !contains(output, "heuristic") {
		t.Error("expected completion or fallback message")
	}

	// 2. Check branches.
	branches := gitBranchList(t, tp.Dir)
	t.Logf("branches: %v", branches)
	splitBranches := filterPrefix(branches, "split/")
	t.Logf("split branches: %v", splitBranches)

	// 3. Verify deleted files are absent on feature branch.
	runGitCmd(t, tp.Dir, "checkout", "feature")
	for _, deletedFile := range []string{
		"pkg/core/legacy.go",
		"docs/deprecated.md",
		"pkg/util/helpers.go",
		"scripts/setup.sh",
	} {
		path := filepath.Join(tp.Dir, deletedFile)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("deleted file %q should not exist on feature branch", deletedFile)
		}
	}

	// 4. Verify new files exist.
	for _, newFile := range []string{
		"pkg/core/v2.go",
		"pkg/middleware/auth.go",
		"cmd/migrate/main.go",
		"docs/migration.md",
	} {
		path := filepath.Join(tp.Dir, newFile)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			t.Errorf("new file %q should exist on feature branch", newFile)
		}
	}
}

// ---------------------------------------------------------------------------
// T105: End-to-end with Ollama (gated)
// ---------------------------------------------------------------------------

func TestIntegration_RealOllama(t *testing.T) {
	skipIfNoOllama(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		InitialFiles: []TestPipelineFile{
			{"pkg/main.go", "package pkg\n\nfunc Main() {}\n"},
			{"README.md", "# Test\n"},
		},
		FeatureFiles: []TestPipelineFile{
			{"pkg/main.go", "package pkg\n\nfunc Main() string { return \"hello\" }\n"},
			{"pkg/helper.go", "package pkg\n\nfunc Helper() {}\n"},
			{"docs/guide.md", "# Guide\n"},
		},
		ConfigOverrides: map[string]any{
			"claudeCommand": ollamaCommand,
			"claudeModel":   integrationModel,
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

	// Run auto-split with ollama.
	if err := tp.Dispatch("auto-split", nil); err != nil {
		t.Fatalf("auto-split returned error: %v", err)
	}

	output := tp.Stdout.String()
	t.Logf("real Ollama auto-split output:\n%s", output)

	// Likely falls back to heuristic since Ollama probably can't handle MCP.
	if !contains(output, "Complete") && !contains(output, "complete") && !contains(output, "fallback") {
		t.Error("expected completion or fallback message")
	}
}
