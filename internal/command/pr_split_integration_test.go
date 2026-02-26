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
// Integration Test: Heuristic Split (no AI required, real git)
// ---------------------------------------------------------------------------

// TestIntegration_HeuristicSplitEndToEnd creates a realistic git repository,
// runs the full heuristic split pipeline (analyze → group → plan → execute →
// verify equivalence), and validates that:
//   - Split branches are created with the correct files.
//   - The combined tree hash of all splits is equivalent to the original.
//   - No content is lost or duplicated.
//
// This test does NOT require AI infrastructure; it runs in every CI build.
func TestIntegration_HeuristicSplitEndToEnd(t *testing.T) {
	t.Parallel()

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Verify we're on the feature branch.
	branch := runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	branch = strings.TrimSpace(branch)
	if branch != "feature" {
		t.Fatalf("expected to be on 'feature' branch, got %q", branch)
	}

	// Set up the pr-split JS engine pointing at our temp repo.
	_, _, evalJS := loadPrSplitEngineWithEval(t, map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true", // always passes
	})

	// Configure git mocks to point at real git repo instead.
	// We override the exec.execv to NOT mock — use real git commands.
	// The JS script's gitExec uses `git -C <dir>` when dir != '.'.
	// We'll use evalJS to call functions with the dir parameter.

	// Step 1: Analyze diff (using real git).
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var analysis struct {
		Files         []string          `json:"files"`
		FileStatuses  map[string]string `json:"fileStatuses"`
		CurrentBranch string            `json:"currentBranch"`
		BaseBranch    string            `json:"baseBranch"`
		Error         *string           `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &analysis); err != nil {
		t.Fatalf("failed to parse analysis: %v", err)
	}
	if analysis.Error != nil {
		t.Fatalf("analyzeDiff returned error: %s", *analysis.Error)
	}
	if len(analysis.Files) == 0 {
		t.Fatal("analyzeDiff returned no files")
	}
	t.Logf("Analyzed %d files: %v", len(analysis.Files), analysis.Files)
	t.Logf("File statuses: %v", analysis.FileStatuses)

	// Step 2: Apply strategy (directory grouping).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.applyStrategy(
		` + mustJSON(t, analysis.Files) + `,
		'directory',
		{
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
			maxFiles: 10,
			baseBranch: 'main'
		}
	))`)
	if err != nil {
		t.Fatalf("applyStrategy failed: %v", err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatalf("failed to parse groups: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("applyStrategy returned no groups")
	}
	t.Logf("Groups: %v", groups)

	// Step 3: Create split plan (with real git to detect current branch).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.createSplitPlan(
		` + mustJSON(t, groups) + `,
		{
			baseBranch: 'main',
			sourceBranch: 'feature',
			branchPrefix: 'split/',
			maxFiles: 10,
			dir: ` + jsString(repoDir) + `,
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `
		}
	))`)
	if err != nil {
		t.Fatalf("createSplitPlan failed: %v", err)
	}

	var plan struct {
		BaseBranch   string `json:"baseBranch"`
		SourceBranch string `json:"sourceBranch"`
		Dir          string `json:"dir"`
		Splits       []struct {
			Name    string   `json:"name"`
			Files   []string `json:"files"`
			Message string   `json:"message"`
			Order   int      `json:"order"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &plan); err != nil {
		t.Fatalf("failed to parse plan: %v", err)
	}
	if len(plan.Splits) == 0 {
		t.Fatal("createSplitPlan produced no splits")
	}
	t.Logf("Plan: %d splits", len(plan.Splits))
	for i, s := range plan.Splits {
		t.Logf("  Split %d: %s (%d files: %v)", i+1, s.Name, len(s.Files), s.Files)
	}

	// Verify all files are accounted for.
	allPlanFiles := make(map[string]bool)
	for _, s := range plan.Splits {
		for _, f := range s.Files {
			if allPlanFiles[f] {
				t.Errorf("duplicate file in plan: %s", f)
			}
			allPlanFiles[f] = true
		}
	}
	for _, f := range analysis.Files {
		if !allPlanFiles[f] {
			t.Errorf("file %s in analysis but missing from plan", f)
		}
	}

	// Step 4: Execute the split (creates real branches).
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: ` + jsString(repoDir) + `,
		verifyCommand: 'true',
		fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
		splits: ` + mustJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("executeSplit failed: %v", err)
	}

	var execResult struct {
		Error   *string `json:"error"`
		Results []struct {
			Branch string  `json:"branch"`
			Error  *string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &execResult); err != nil {
		t.Fatalf("failed to parse exec result: %v", err)
	}
	if execResult.Error != nil {
		t.Fatalf("executeSplit returned top-level error: %s", *execResult.Error)
	}
	for _, r := range execResult.Results {
		if r.Error != nil {
			t.Errorf("split branch %s failed: %s", r.Branch, *r.Error)
		}
	}

	// Verify branches exist in the repo.
	branchOutput := runGit(t, repoDir, "branch", "--list", "split/*")
	for _, s := range plan.Splits {
		if !strings.Contains(branchOutput, s.Name) {
			t.Errorf("expected branch %s to exist, not found in:\n%s", s.Name, branchOutput)
		}
	}
	t.Logf("Created branches:\n%s", branchOutput)

	// Step 5: Verify tree hash equivalence.
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.verifyEquivalenceDetailed({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: ` + jsString(repoDir) + `,
		splits: ` + mustJSON(t, plan.Splits) + `
	}))`)
	if err != nil {
		t.Fatalf("verifyEquivalenceDetailed failed: %v", err)
	}

	var equiv struct {
		Equivalent  bool     `json:"equivalent"`
		SplitTree   string   `json:"splitTree"`
		SourceTree  string   `json:"sourceTree"`
		Error       *string  `json:"error"`
		DiffFiles   []string `json:"diffFiles"`
		DiffSummary string   `json:"diffSummary"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &equiv); err != nil {
		t.Fatalf("failed to parse equivalence result: %v", err)
	}
	if equiv.Error != nil {
		t.Errorf("verifyEquivalence error: %s", *equiv.Error)
	}
	if !equiv.Equivalent {
		t.Errorf("tree hash mismatch! splitTree=%s sourceTree=%s diffFiles=%v diffSummary=%s",
			equiv.SplitTree, equiv.SourceTree, equiv.DiffFiles, equiv.DiffSummary)
	} else {
		t.Logf("✅ Tree hash equivalence verified: %s", equiv.SplitTree)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Cancellation Flow
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitCancel verifies that the auto-split pipeline
// responds to cooperative cancellation within a reasonable time. It mocks
// the autoSplitTUI.cancelled() function to return true during the pipeline
// and verifies the pipeline exits with a cancellation error.
func TestIntegration_AutoSplitCancel(t *testing.T) {
	t.Parallel()

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	_, _, evalJS := loadPrSplitEngineWithEval(t, map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"branchPrefix":  "split/",
		"verifyCommand": "true",
	})

	// Inject a mock autoSplitTUI that returns cancelled immediately.
	// This simulates the user pressing q before the pipeline starts any
	// blocking operation.
	_, err := evalJS(`
		globalThis.autoSplitTUI = {
			runAsync: function() {},
			wait: function() { return null; },
			stepStart: function() {},
			stepDone: function() {},
			appendOutput: function() {},
			appendError: function() {},
			done: function() {},
			stepDetail: function() {},
			cancelled: function() { return true; },
			forceCancelled: function() { return false; },
			quit: function() {}
		};
	`)
	if err != nil {
		t.Fatalf("failed to inject mock autoSplitTUI: %v", err)
	}

	// Run auto-split — it should detect cancellation at the first step
	// boundary and return immediately.
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.automatedSplit({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `,
		strategy: 'directory'
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit failed: %v", err)
	}

	var result struct {
		Error  *string `json:"error"`
		Report struct {
			Error *string `json:"error"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// The pipeline should have returned a cancellation error.
	if result.Error == nil || !strings.Contains(*result.Error, "cancel") {
		t.Errorf("expected cancellation error, got: %v", result.Error)
	} else {
		t.Logf("✅ Cancellation detected: %s", *result.Error)
	}

	// The original branch should still be intact.
	branch := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "feature" {
		t.Errorf("expected to be on 'feature' branch after cancel, got %q", branch)
	}
}

// ---------------------------------------------------------------------------
// Integration Helpers
// ---------------------------------------------------------------------------

// initIntegrationRepo creates a temporary git repository mimicking a real
// Go project with multiple packages. The initial commit contains baseline
// files that the feature branch will build upon.
func initIntegrationRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Verify git is available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available — skipping integration test")
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "integration-test@osm.dev")
	runGit(t, dir, "config", "user.name", "OSM Integration Test")

	// Create a realistic Go project structure.
	initialFiles := []struct{ path, content string }{
		{"go.mod", "module example.com/test-project\n\ngo 1.21\n"},
		{"README.md", "# Test Project\n\nA sample project for integration testing.\n"},
		{"cmd/app/main.go", `package main

import (
	"fmt"
	"example.com/test-project/pkg/core"
)

func main() {
	fmt.Println(core.Version())
}
`},
		{"pkg/core/core.go", `package core

// Version returns the project version.
func Version() string {
	return "1.0.0"
}
`},
		{"pkg/core/core_test.go", `package core

import "testing"

func TestVersion(t *testing.T) {
	if v := Version(); v == "" {
		t.Fatal("version should not be empty")
	}
}
`},
		{"internal/util/strings.go", `package util

// TrimAll trims whitespace from all strings in a slice.
func TrimAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s // placeholder
		_ = s
	}
	return out
}
`},
		{"docs/getting-started.md", "# Getting Started\n\nFollow these steps.\n"},
		{".gitignore", "*.exe\n*.test\n/bin/\n"},
	}

	for _, f := range initialFiles {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial project structure")

	return dir
}

// addIntegrationFeatureFiles creates a "feature" branch with changes across
// multiple concerns: new package, modified existing code, added tests, docs
// updates, and config changes. This diversity ensures the split algorithm
// must make non-trivial grouping decisions.
func addIntegrationFeatureFiles(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	featureFiles := []struct{ path, content string }{
		// New package: authentication
		{"pkg/auth/auth.go", `package auth

import "errors"

// ErrUnauthorized is returned when authentication fails.
var ErrUnauthorized = errors.New("unauthorized")

// Token represents an authentication token.
type Token struct {
	Value  string
	Expiry int64
}

// Validate checks if a token is valid.
func (t Token) Validate() error {
	if t.Value == "" {
		return ErrUnauthorized
	}
	return nil
}
`},
		{"pkg/auth/auth_test.go", `package auth

import "testing"

func TestToken_Validate(t *testing.T) {
	tests := []struct {
		name    string
		token   Token
		wantErr bool
	}{
		{"valid", Token{Value: "abc", Expiry: 9999}, false},
		{"empty", Token{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.token.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
`},
		// Modified: core package gets a new function
		{"pkg/core/config.go", `package core

// Config holds application configuration.
type Config struct {
	Debug   bool
	Verbose bool
	Port    int
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Debug:   false,
		Verbose: false,
		Port:    8080,
	}
}
`},
		{"pkg/core/config_test.go", `package core

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.Debug {
		t.Error("debug should be false by default")
	}
}
`},
		// Modified: util package gets new function
		{"internal/util/numbers.go", `package util

// Max returns the larger of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
`},
		{"internal/util/numbers_test.go", `package util

import "testing"

func TestMax(t *testing.T) {
	if Max(1, 2) != 2 {
		t.Error("Max(1,2) should be 2")
	}
	if Max(5, 3) != 5 {
		t.Error("Max(5,3) should be 5")
	}
}
`},
		// New: middleware package
		{"internal/middleware/logging.go", `package middleware

import "fmt"

// Logger provides request logging.
type Logger struct {
	Prefix string
}

// Log writes a log entry.
func (l Logger) Log(msg string) {
	fmt.Printf("[%s] %s\n", l.Prefix, msg)
}
`},
		// Documentation updates
		{"docs/api-reference.md", `# API Reference

## Authentication

Use the auth package for token-based authentication.

## Configuration

Use core.DefaultConfig() to get default settings.
`},
		{"docs/changelog.md", `# Changelog

## Unreleased

- Added authentication package
- Added configuration support
- Added middleware logging
- Updated documentation
`},
		// Modified: main.go to use new packages
		{"cmd/app/main.go", `package main

import (
	"fmt"
	"example.com/test-project/pkg/auth"
	"example.com/test-project/pkg/core"
)

func main() {
	fmt.Println(core.Version())
	cfg := core.DefaultConfig()
	fmt.Printf("Port: %d\n", cfg.Port)

	token := auth.Token{Value: "test-token", Expiry: 9999}
	if err := token.Validate(); err != nil {
		fmt.Printf("Auth error: %v\n", err)
	}
}
`},
	}

	for _, f := range featureFiles {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "feature: auth, config, middleware, docs")
}

// jsString returns a JavaScript string literal (single-quoted, with escaping)
// for embedding a Go string into a JS expression.
func jsString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return `'` + escaped + `'`
}

// mustJSON marshals v to a JSON string, failing the test on error.
func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// runGit runs a git command and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\noutput: %s", args, err, out)
	}
	return string(out)
}
