package command

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// T018: Complex Go Project Integration Tests
// ---------------------------------------------------------------------------
//
// These tests create a realistic Go project with multiple packages, inter-
// package imports, and diverse feature branch changes. They verify that the
// split pipeline (heuristic or AI-driven) produces branches where:
//
//   - Each branch compiles: `go build ./...`
//   - Each branch's tests pass: `go test ./...`
//   - Tree-hash equivalence is preserved across all splits
//   - The split grouping is logged for inspection
//
// The heuristic test runs in every CI build. The AI test requires:
//
//   go test -race -v -count=1 -timeout=15m -integration \
//     -claude-command=claude ./internal/command/... \
//     -run TestIntegration_AutoSplitComplexGoProject

// ---------------------------------------------------------------------------
// Heuristic Split (no AI, runs in CI)
// ---------------------------------------------------------------------------

// TestIntegration_ComplexGoProject_HeuristicSplit runs the full heuristic
// split pipeline against a complex multi-package Go project and verifies
// each split branch compiles and passes tests independently.
//
// The project structure has inter-package import chains:
//
//	cmd/server → internal/api → internal/db → internal/models
//
// The feature branch changes are designed so that each directory's diff is
// independently compilable against the base branch (no cross-directory
// import of NEW functions/types). This allows the directory strategy to
// produce branches that build.
func TestIntegration_ComplexGoProject_HeuristicSplit(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	repoDir := initComplexGoRepo(t)
	addComplexGoFeatureChanges(t, repoDir)

	branch := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch != "feature" {
		t.Fatalf("expected feature branch, got %q", branch)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory-deep",
		"maxFiles":      20,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
	})

	// --- Step 1: Analyze diff. ---
	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.analyzeDiff({
		baseBranch: 'main',
		dir: ` + jsString(repoDir) + `
	}))`)
	if err != nil {
		t.Fatalf("analyzeDiff failed: %v", err)
	}

	var analysis struct {
		Files        []string          `json:"files"`
		FileStatuses map[string]string `json:"fileStatuses"`
		Error        *string           `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &analysis); err != nil {
		t.Fatalf("parse analysis: %v", err)
	}
	if analysis.Error != nil {
		t.Fatalf("analyzeDiff error: %s", *analysis.Error)
	}
	if len(analysis.Files) < 10 {
		t.Fatalf("expected ≥10 changed files, got %d: %v", len(analysis.Files), analysis.Files)
	}
	t.Logf("Analyzed %d files: %v", len(analysis.Files), analysis.Files)

	// --- Step 2: Apply directory strategy. ---
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.applyStrategy(
		` + mustJSON(t, analysis.Files) + `,
		'directory-deep',
		{
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `,
			maxFiles: 20,
			baseBranch: 'main'
		}
	))`)
	if err != nil {
		t.Fatalf("applyStrategy failed: %v", err)
	}
	var groups map[string][]string
	if err := json.Unmarshal([]byte(raw.(string)), &groups); err != nil {
		t.Fatalf("parse groups: %v", err)
	}
	if len(groups) < 3 {
		t.Fatalf("expected ≥3 groups from complex project, got %d: %v", len(groups), groups)
	}
	t.Logf("Groups (%d):", len(groups))
	for name, files := range groups {
		t.Logf("  %s: %v", name, files)
	}

	// --- Step 3: Create split plan. ---
	raw, err = evalJS(`JSON.stringify(globalThis.prSplit.createSplitPlan(
		` + mustJSON(t, groups) + `,
		{
			baseBranch: 'main',
			sourceBranch: 'feature',
			branchPrefix: 'split/',
			maxFiles: 20,
			dir: ` + jsString(repoDir) + `,
			fileStatuses: ` + mustJSON(t, analysis.FileStatuses) + `
		}
	))`)
	if err != nil {
		t.Fatalf("createSplitPlan failed: %v", err)
	}

	var plan struct {
		Splits []struct {
			Name    string   `json:"name"`
			Files   []string `json:"files"`
			Message string   `json:"message"`
			Order   int      `json:"order"`
		} `json:"splits"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &plan); err != nil {
		t.Fatalf("parse plan: %v", err)
	}
	if len(plan.Splits) < 3 {
		t.Fatalf("expected ≥3 splits, got %d", len(plan.Splits))
	}
	t.Logf("Plan (%d splits):", len(plan.Splits))
	for i, s := range plan.Splits {
		t.Logf("  Split %d: %s (%d files: %v)", i+1, s.Name, len(s.Files), s.Files)
	}

	// Verify all files accounted for (no loss, no duplication).
	allFiles := make(map[string]bool)
	for _, s := range plan.Splits {
		for _, f := range s.Files {
			if allFiles[f] {
				t.Errorf("duplicate file in plan: %s", f)
			}
			allFiles[f] = true
		}
	}
	for _, f := range analysis.Files {
		if !allFiles[f] {
			t.Errorf("file %s in diff but missing from plan", f)
		}
	}

	// --- Step 4: Execute split. ---
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
		t.Fatalf("parse exec result: %v", err)
	}
	if execResult.Error != nil {
		t.Fatalf("executeSplit error: %s", *execResult.Error)
	}
	for _, r := range execResult.Results {
		if r.Error != nil {
			t.Errorf("split %s error: %s", r.Branch, *r.Error)
		}
	}

	// --- Step 5: Verify each split branch builds and tests pass. ---
	t.Log("Verifying each split branch builds and tests pass...")
	for _, s := range plan.Splits {
		branchName := s.Name // Name already includes branchPrefix (e.g. "split/01-docs")
		t.Logf("  Checking out %s...", branchName)
		runGit(t, repoDir, "checkout", branchName)

		// Only run build/test if this branch modifies Go files.
		hasGoFiles := false
		for _, f := range s.Files {
			if strings.HasSuffix(f, ".go") {
				hasGoFiles = true
				break
			}
		}
		if hasGoFiles {
			verifyGoBuild(t, repoDir)
			verifyGoTest(t, repoDir)
			t.Logf("  ✅ %s: build + test pass", branchName)
		} else {
			t.Logf("  ⏭  %s: no Go files, skipping build/test", branchName)
		}
	}

	// Restore feature branch for equivalence check.
	runGit(t, repoDir, "checkout", "feature")

	// --- Step 6: Verify tree-hash equivalence. ---
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
		t.Fatalf("parse equivalence: %v", err)
	}
	if equiv.Error != nil {
		t.Errorf("equivalence error: %s", *equiv.Error)
	}
	if !equiv.Equivalent {
		t.Errorf("tree hash mismatch: split=%s source=%s diff=%v summary=%s",
			equiv.SplitTree, equiv.SourceTree, equiv.DiffFiles, equiv.DiffSummary)
	} else {
		t.Logf("✅ Tree hash equivalence verified: %s", equiv.SplitTree)
	}
}

// ---------------------------------------------------------------------------
// AI-Driven Split (requires -integration -claude-command=... flags)
// ---------------------------------------------------------------------------

// TestIntegration_AutoSplitComplexGoProject runs the full AI-driven auto-split
// pipeline against a complex Go project with inter-package imports.
//
// It verifies that the AI classification produces coherent groups where each
// split branch:
//
//   - Compiles independently: `go build ./...`
//   - Passes all tests: `go test ./...`
//   - Preserves tree-hash equivalence
//
// Run with:
//
//	go test -race -v -count=1 -timeout=15m -integration \
//	  -claude-command=claude ./internal/command/... \
//	  -run TestIntegration_AutoSplitComplexGoProject
//
// Or via make:
//
//	make integration-test-prsplit
func TestIntegration_AutoSplitComplexGoProject(t *testing.T) {
	skipIfNoClaude(t)

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}

	repoDir := initComplexGoRepo(t)
	addComplexGoFeatureChanges(t, repoDir)

	claudeArgsList := make([]string, len(claudeTestArgs))
	copy(claudeArgsList, claudeTestArgs)

	configOverrides := map[string]interface{}{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      20,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"claudeCommand": claudeTestCommand,
		"claudeArgs":    claudeArgsList,
		"timeoutMs":     int64(5 * 60 * 1000), // 5 minutes per step (JS layer)
		"_evalTimeout":  25 * time.Minute,     // T32: Go-layer evalJS timeout (must exceed JS classifyTimeoutMs=20min)
	}
	if integrationModel != "" {
		configOverrides["claudeModel"] = integrationModel
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, configOverrides)

	// Inject autoSplitTUI mock for headless CI/terminal execution.
	_, err := evalJS(`
		globalThis.autoSplitTUI = {
			runAsync: function() {},
			wait: function() { return null; },
			stepStart: function(name) { log.printf('STEP START: %s', name); },
			stepDone: function(name, err, elapsed) {
				log.printf('STEP DONE: %s err=%s elapsed=%dms', name, err || 'ok', elapsed);
			},
			appendOutput: function(text) { log.printf('OUTPUT: %s', text); },
			appendError: function(text) { log.printf('ERROR: %s', text); },
			done: function(summary) { log.printf('DONE: %s', summary); },
			stepDetail: function(name, detail) { log.printf('DETAIL: %s — %s', name, detail); },
			cancelled: function() { return false; },
			forceCancelled: function() { return false; },
			quit: function() {}
		};
	`)
	if err != nil {
		t.Fatalf("inject autoSplitTUI mock: %v", err)
	}

	// Run the full auto-split pipeline with AI classification.
	t.Log("Starting auto-split pipeline with real AI agent...")
	t.Logf("Claude command: %s %v", claudeTestCommand, claudeArgsList)
	raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({
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
			Error              *string  `json:"error"`
			ClaudeInteractions int      `json:"claudeInteractions"`
			FallbackUsed       bool     `json:"fallbackUsed"`
			SplitsCreated      int      `json:"splitsCreated"`
			Classification     *string  `json:"classification"`
			Plan               *string  `json:"plan"`
			Groups             []string `json:"groups"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	// Log the full pipeline result for diagnostic purposes.
	t.Logf("Pipeline result: %s", raw)

	if result.Report.FallbackUsed {
		t.Log("⚠️  Pipeline fell back to heuristic mode — AI may not be responding")
	}
	if result.Error != nil {
		t.Fatalf("pipeline error: %s", *result.Error)
	}
	if result.Report.ClaudeInteractions == 0 && !result.Report.FallbackUsed {
		t.Error("expected ≥1 Claude interaction in AI mode")
	}
	if result.Report.SplitsCreated == 0 {
		t.Fatal("expected splits to be created")
	}

	t.Logf("AI created %d splits (interactions: %d, fallback: %v)",
		result.Report.SplitsCreated, result.Report.ClaudeInteractions, result.Report.FallbackUsed)

	// Log classification details if available.
	if result.Report.Classification != nil {
		t.Logf("Classification:\n%s", *result.Report.Classification)
	}
	if result.Report.Plan != nil {
		t.Logf("Plan:\n%s", *result.Report.Plan)
	}
	if len(result.Report.Groups) > 0 {
		t.Logf("Groups: %v", result.Report.Groups)
	}

	// Enumerate split branches.
	branchOutput := strings.TrimSpace(runGit(t, repoDir, "branch", "--list", "split/*"))
	if branchOutput == "" {
		t.Fatal("no split/* branches found after pipeline completion")
	}
	branches := splitBranchNames(branchOutput)
	t.Logf("Split branches (%d): %v", len(branches), branches)

	// Verify each split branch builds and tests pass.
	t.Log("Verifying each split branch builds and tests pass...")
	for _, b := range branches {
		t.Logf("  Checking out %s...", b)
		runGit(t, repoDir, "checkout", b)
		verifyGoBuild(t, repoDir)
		verifyGoTest(t, repoDir)
		t.Logf("  ✅ %s: build + test pass", b)
	}

	// Restore feature branch.
	runGit(t, repoDir, "checkout", "feature")

	t.Logf("✅ All %d split branches verified: build + test pass", len(branches))
}

// ---------------------------------------------------------------------------
// Complex Go Project Helpers
// ---------------------------------------------------------------------------

// initComplexGoRepo creates a temporary git repository with a multi-package
// Go project. Packages have inter-package imports forming a dependency chain:
//
//	cmd/server → internal/api → internal/db → internal/models
//
// All packages compile and all tests pass from the initial commit.
func initComplexGoRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available — skipping integration test")
	}

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@osm.dev")
	runGit(t, dir, "config", "user.name", "OSM Test")

	files := []struct{ path, content string }{
		{"go.mod", "module example.com/complex\n\ngo 1.21\n"},
		{"Makefile", "build:\n\tgo build ./...\ntest:\n\tgo test ./...\n\n.PHONY: build test\n"},
		{"docs/README.md", "# Complex Test Project\n\nMulti-package Go project for integration testing.\n"},

		// --- internal/models: domain types (dependency root) ---
		{"internal/models/user.go", `package models

// User represents a user in the system.
type User struct {
	ID   int
	Name string
}

// NewUser creates a new User with the given ID and name.
func NewUser(id int, name string) User {
	return User{ID: id, Name: name}
}
`},
		{"internal/models/user_test.go", `package models

import "testing"

func TestNewUser(t *testing.T) {
	u := NewUser(1, "alice")
	if u.ID != 1 {
		t.Errorf("ID = %d, want 1", u.ID)
	}
	if u.Name != "alice" {
		t.Errorf("Name = %q, want alice", u.Name)
	}
}
`},

		// --- internal/db: data access (imports models) ---
		{"internal/db/store.go", `package db

import "example.com/complex/internal/models"

// Store provides in-memory data access.
type Store struct {
	users map[int]models.User
}

// NewStore creates an in-memory store with seed data.
func NewStore() *Store {
	return &Store{users: map[int]models.User{
		1: models.NewUser(1, "admin"),
		2: models.NewUser(2, "viewer"),
	}}
}

// GetUser retrieves a user by ID.
func (s *Store) GetUser(id int) (models.User, bool) {
	u, ok := s.users[id]
	return u, ok
}
`},
		{"internal/db/store_test.go", `package db

import "testing"

func TestStore_GetUser(t *testing.T) {
	s := NewStore()
	u, ok := s.GetUser(1)
	if !ok {
		t.Fatal("expected user 1 to exist")
	}
	if u.Name != "admin" {
		t.Errorf("Name = %q, want admin", u.Name)
	}
	_, ok = s.GetUser(999)
	if ok {
		t.Error("expected user 999 to not exist")
	}
}
`},

		// --- internal/api: handlers (imports db + models) ---
		{"internal/api/handler.go", `package api

import (
	"example.com/complex/internal/db"
	"example.com/complex/internal/models"
)

// Handler provides request handling.
type Handler struct {
	store *db.Store
}

// NewHandler creates a handler backed by the given store.
func NewHandler(store *db.Store) *Handler {
	return &Handler{store: store}
}

// GetUser retrieves a user by ID.
func (h *Handler) GetUser(id int) (models.User, error) {
	u, ok := h.store.GetUser(id)
	if !ok {
		return models.User{}, ErrNotFound
	}
	return u, nil
}

// ErrNotFound indicates a resource was not found.
var ErrNotFound = &NotFoundError{}

// NotFoundError is a not-found sentinel error.
type NotFoundError struct{}

func (e *NotFoundError) Error() string { return "not found" }
`},
		{"internal/api/handler_test.go", `package api

import (
	"testing"
	"example.com/complex/internal/db"
)

func TestHandler_GetUser(t *testing.T) {
	h := NewHandler(db.NewStore())
	u, err := h.GetUser(1)
	if err != nil {
		t.Fatalf("GetUser(1): %v", err)
	}
	if u.Name != "admin" {
		t.Errorf("Name = %q, want admin", u.Name)
	}
	_, err = h.GetUser(999)
	if err == nil {
		t.Error("expected error for user 999")
	}
}
`},

		// --- cmd/server: main entrypoint (imports api + db) ---
		{"cmd/server/main.go", `package main

import (
	"fmt"
	"example.com/complex/internal/api"
	"example.com/complex/internal/db"
)

func main() {
	store := db.NewStore()
	h := api.NewHandler(store)
	u, err := h.GetUser(1)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("User: %s (ID: %d)\n", u.Name, u.ID)
}
`},
	}

	for _, f := range files {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	// Verify the base project compiles and tests pass before committing.
	verifyGoBuild(t, dir)
	verifyGoTest(t, dir)

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial: multi-package Go project")

	return dir
}

// addComplexGoFeatureChanges creates a "feature" branch with diverse
// changes across the complex Go project:
//
//   - New package: internal/auth (imports internal/models — base-branch API only)
//   - New package: internal/config (standalone, no inter-package imports)
//   - Modified: internal/api (adds HealthCheck — no new cross-package deps)
//   - Modified: internal/db (adds ListUsers — uses base-branch models API)
//   - Modified: internal/models (adds Email field + DisplayName method)
//   - New + modified: docs/ (new architecture doc, updated README)
//   - Modified: Makefile (adds lint target)
//
// INVARIANT: each directory's changes are independently compilable against
// the base (main) branch. New code never references NEW functions, fields,
// or types from other directories' feature changes. This ensures that
// directory-based splits produce branches that build and pass tests.
func addComplexGoFeatureChanges(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	files := []struct{ path, content string }{
		// --- NEW: internal/auth (imports internal/models, base API only) ---
		{"internal/auth/auth.go", `package auth

import "example.com/complex/internal/models"

// Authenticate checks that the user has the minimum required fields.
// It uses only the base-branch models.User API (ID, Name) so this
// package compiles independently of other feature changes.
func Authenticate(u models.User) bool {
	return u.ID > 0 && u.Name != ""
}

// Role represents a user role.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)
`},
		{"internal/auth/auth_test.go", `package auth

import (
	"testing"
	"example.com/complex/internal/models"
)

func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name string
		user models.User
		want bool
	}{
		{"valid", models.NewUser(1, "alice"), true},
		{"zero id", models.NewUser(0, "alice"), false},
		{"empty name", models.NewUser(1, ""), false},
		{"zero value", models.User{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Authenticate(tt.user); got != tt.want {
				t.Errorf("Authenticate(%+v) = %v, want %v", tt.user, got, tt.want)
			}
		})
	}
}
`},

		// --- NEW: internal/config (standalone, no inter-package imports) ---
		{"internal/config/config.go", `package config

// Config holds application configuration.
type Config struct {
	Port     int
	LogLevel string
	Debug    bool
}

// Default returns a sensible default configuration.
func Default() Config {
	return Config{
		Port:     8080,
		LogLevel: "info",
		Debug:    false,
	}
}
`},
		{"internal/config/config_test.go", `package config

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.Debug {
		t.Error("Debug should be false by default")
	}
}
`},

		// --- MODIFIED: internal/api (adds HealthCheck — no new deps) ---
		{"internal/api/handler.go", `package api

import (
	"example.com/complex/internal/db"
	"example.com/complex/internal/models"
)

// Handler provides request handling.
type Handler struct {
	store *db.Store
}

// NewHandler creates a handler backed by the given store.
func NewHandler(store *db.Store) *Handler {
	return &Handler{store: store}
}

// GetUser retrieves a user by ID.
func (h *Handler) GetUser(id int) (models.User, error) {
	u, ok := h.store.GetUser(id)
	if !ok {
		return models.User{}, ErrNotFound
	}
	return u, nil
}

// HealthCheck returns the service health status. This function has no
// new cross-package dependencies, so the api split branch compiles
// independently of other feature changes.
func (h *Handler) HealthCheck() string {
	return "ok"
}

// ErrNotFound indicates a resource was not found.
var ErrNotFound = &NotFoundError{}

// NotFoundError is a not-found sentinel error.
type NotFoundError struct{}

func (e *NotFoundError) Error() string { return "not found" }
`},
		{"internal/api/handler_test.go", `package api

import (
	"testing"
	"example.com/complex/internal/db"
)

func TestHandler_GetUser(t *testing.T) {
	h := NewHandler(db.NewStore())
	u, err := h.GetUser(1)
	if err != nil {
		t.Fatalf("GetUser(1): %v", err)
	}
	if u.Name != "admin" {
		t.Errorf("Name = %q, want admin", u.Name)
	}
	_, err = h.GetUser(999)
	if err == nil {
		t.Error("expected error for user 999")
	}
}

func TestHandler_HealthCheck(t *testing.T) {
	h := NewHandler(db.NewStore())
	if got := h.HealthCheck(); got != "ok" {
		t.Errorf("HealthCheck() = %q, want ok", got)
	}
}
`},

		// --- MODIFIED: internal/db (adds ListUsers — base models API) ---
		{"internal/db/store.go", `package db

import "example.com/complex/internal/models"

// Store provides in-memory data access.
type Store struct {
	users map[int]models.User
}

// NewStore creates an in-memory store with seed data.
func NewStore() *Store {
	return &Store{users: map[int]models.User{
		1: models.NewUser(1, "admin"),
		2: models.NewUser(2, "viewer"),
	}}
}

// GetUser retrieves a user by ID.
func (s *Store) GetUser(id int) (models.User, bool) {
	u, ok := s.users[id]
	return u, ok
}

// ListUsers returns all users. Uses only the base-branch models API
// so this compiles independently of other feature changes.
func (s *Store) ListUsers() []models.User {
	out := make([]models.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out
}
`},
		{"internal/db/store_test.go", `package db

import "testing"

func TestStore_GetUser(t *testing.T) {
	s := NewStore()
	u, ok := s.GetUser(1)
	if !ok {
		t.Fatal("expected user 1 to exist")
	}
	if u.Name != "admin" {
		t.Errorf("Name = %q, want admin", u.Name)
	}
	_, ok = s.GetUser(999)
	if ok {
		t.Error("expected user 999 to not exist")
	}
}

func TestStore_ListUsers(t *testing.T) {
	s := NewStore()
	users := s.ListUsers()
	if len(users) != 2 {
		t.Errorf("len(ListUsers) = %d, want 2", len(users))
	}
}
`},

		// --- MODIFIED: internal/models (adds Email + DisplayName) ---
		{"internal/models/user.go", `package models

import "fmt"

// User represents a user in the system.
type User struct {
	ID    int
	Name  string
	Email string
}

// NewUser creates a new User with the given ID and name.
func NewUser(id int, name string) User {
	return User{ID: id, Name: name}
}

// DisplayName returns a formatted display name. If the user has an
// email, it is included in angle brackets.
func (u User) DisplayName() string {
	if u.Email != "" {
		return fmt.Sprintf("%s <%s>", u.Name, u.Email)
	}
	return u.Name
}
`},
		{"internal/models/user_test.go", `package models

import "testing"

func TestNewUser(t *testing.T) {
	u := NewUser(1, "alice")
	if u.ID != 1 {
		t.Errorf("ID = %d, want 1", u.ID)
	}
	if u.Name != "alice" {
		t.Errorf("Name = %q, want alice", u.Name)
	}
}

func TestUser_DisplayName(t *testing.T) {
	tests := []struct {
		name string
		user User
		want string
	}{
		{"name only", User{Name: "alice"}, "alice"},
		{"with email", User{Name: "alice", Email: "alice@example.com"}, "alice <alice@example.com>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
`},

		// --- MODIFIED + NEW: docs ---
		{"docs/README.md", "# Complex Test Project\n\nMulti-package Go project with auth, config, and enhanced features.\n"},
		{"docs/architecture.md", `# Architecture

## Package Dependency Graph

` + "```" + `
cmd/server → internal/api → internal/db → internal/models
                                         ↑
             internal/auth ──────────────┘
             internal/config (standalone)
` + "```" + `

## Packages

- **cmd/server**: Application entry point
- **internal/api**: HTTP-like request handlers
- **internal/auth**: Authentication (token validation, roles)
- **internal/config**: Application configuration
- **internal/db**: In-memory data store
- **internal/models**: Domain types (User)
`},

		// --- MODIFIED: Makefile (adds lint target) ---
		{"Makefile", "build:\n\tgo build ./...\ntest:\n\tgo test ./...\nlint:\n\tgo vet ./...\n\n.PHONY: build test lint\n"},
	}

	for _, f := range files {
		fullPath := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(f.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
	}

	// Verify the feature branch compiles and tests pass before committing.
	verifyGoBuild(t, dir)
	verifyGoTest(t, dir)

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "feature: auth, config, list users, display name, docs")
}

// verifyGoBuild runs `go build ./...` in the given directory and fails the
// test if compilation fails. Uses a 2-minute timeout and suppresses race
// detector inheritance to avoid inflated compile times in synthetic projects.
func verifyGoBuild(t *testing.T, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = dir
	// Suppress race detector — this synthetic project does not need it,
	// and -race multiplies compile time significantly.
	cmd.Env = append(filterEnv(os.Environ(), "GOFLAGS"), "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed in %s:\n%v\n%s", dir, err, out)
	}
}

// verifyGoTest runs `go test ./...` in the given directory and fails the
// test if any test fails. Uses a 2-minute timeout and suppresses race
// detector inheritance.
func verifyGoTest(t *testing.T, dir string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-timeout=60s", "./...")
	cmd.Dir = dir
	// Suppress race detector — same rationale as verifyGoBuild.
	cmd.Env = append(filterEnv(os.Environ(), "GOFLAGS"), "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... failed in %s:\n%v\n%s", dir, err, out)
	}
}

// filterEnv returns a copy of environ with all entries matching key removed.
func filterEnv(environ []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// splitBranchNames parses `git branch --list` output into cleaned branch names.
func splitBranchNames(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		name := strings.TrimSpace(line)
		name = strings.TrimPrefix(name, "* ")
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}
