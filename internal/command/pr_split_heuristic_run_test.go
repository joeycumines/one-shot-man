package command

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestPrSplitCommand_RunHeuristicEndToEnd(t *testing.T) {
	skipSlow(t)
	// NOT parallel — we chdir.
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

	// Dispatch the "run" command (the full workflow).
	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run command returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run output:\n%s", output)

	// Step 1: Analysis completed.
	if !contains(output, "Analysis:") {
		t.Error("expected analysis output")
	}
	if !contains(output, "changed files") {
		t.Error("expected 'changed files' message")
	}

	// Step 2: Grouping completed.
	if !contains(output, "Grouped into") {
		t.Error("expected grouping output")
	}

	// Step 3: Plan created.
	if !contains(output, "Plan created:") {
		t.Error("expected plan output")
	}

	// Step 4: Execution completed.
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}

	// Step 5: Equivalence verified.
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification")
	}

	// Verify that split branches were actually created in the repo.
	branches := runGitCmd(t, dir, "branch")
	if !strings.Contains(branches, "split/") {
		t.Errorf("expected split branches, got:\n%s", branches)
	}
	// Should have at least cmd, docs, pkg groups.
	expectedGroups := []string{"cmd", "docs", "pkg"}
	for _, g := range expectedGroups {
		if !strings.Contains(branches, g) {
			t.Errorf("expected branch containing %q, got:\n%s", g, branches)
		}
	}

	// Verify we're back on the feature branch.
	current := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if current != "feature" {
		t.Errorf("expected to be on 'feature' branch, got %q", current)
	}
}

// TestPrSplitCommand_RunZeroChanges verifies that the run handler exits
// gracefully when there are no changes between the current and base branch.
func TestPrSplitCommand_RunZeroChanges(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	// Create a repo with NO feature branch changes (stay on main).
	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial commit")
	// Create a feature branch but don't add any changes.
	runGitCmd(t, dir, "checkout", "-b", "feature")

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
		t.Fatalf("run command returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (zero changes) output:\n%s", output)

	if !contains(output, "No changes found") {
		t.Errorf("expected 'No changes found' message, got:\n%s", output)
	}

	// No split branches should be created.
	branches := runGitCmd(t, dir, "branch")
	if strings.Contains(branches, "split/") {
		t.Errorf("expected no split branches, got:\n%s", branches)
	}
}

// TestPrSplitCommand_RunDryRun verifies that dry-run mode shows the plan
// without creating any branches.
func TestPrSplitCommand_RunDryRun(t *testing.T) {
	skipSlow(t)
	// NOT parallel — we chdir.
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"dryRun": true,
	})

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run command returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (dry-run) output:\n%s", output)

	// Should show analysis and plan but NOT execution.
	if !contains(output, "Analysis:") {
		t.Error("expected analysis output in dry-run")
	}
	if !contains(output, "Plan created:") {
		t.Error("expected plan output in dry-run")
	}
	if !contains(output, "DRY RUN") {
		t.Error("expected DRY RUN indicator")
	}

	// Should NOT contain execution messages.
	if contains(output, "Split executed:") {
		t.Error("dry-run should NOT execute splits")
	}

	// No branches should be created.
	branches := runGitCmd(t, dir, "branch")
	if strings.Contains(branches, "split/") {
		t.Errorf("dry-run should not create branches, got:\n%s", branches)
	}
}

// TestPrSplitCommand_HelpCommand verifies that the "help" TUI command
// dispatches correctly and outputs documentation.
func TestPrSplitCommand_HelpCommand(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	stdout, dispatch := loadPrSplitEngine(t, nil)

	if err := dispatch("help", nil); err != nil {
		t.Fatalf("help command returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("help output:\n%s", output)

	expectedKeywords := []string{
		"analyze", "stats", "group", "plan", "execute",
		"verify", "equivalence", "run", "help",
	}
	for _, kw := range expectedKeywords {
		if !contains(output, kw) {
			t.Errorf("expected help output to contain %q", kw)
		}
	}
}

func TestPrSplitCommand_TemplateContent(t *testing.T) {
	skipSlow(t)
	// Verify the template has expected sections
	if !contains(prSplitTemplate, "baseBranch") {
		t.Error("Expected template to contain baseBranch variable")
	}
	if !contains(prSplitTemplate, "Split Strategy") {
		t.Error("Expected template to contain Split Strategy section")
	}
	if !contains(prSplitTemplate, "Execution Plan") {
		t.Error("Expected template to contain Execution Plan section")
	}
	if !contains(prSplitTemplate, "Verification") {
		t.Error("Expected template to contain Verification section")
	}
}

func TestPrSplitCommand_ScriptContent(t *testing.T) {
	skipSlow(t)
	// Verify the chunked script source has expected functions
	src := allChunkSources()
	if !contains(src, "function analyzeDiff") {
		t.Error("Expected script to contain analyzeDiff function")
	}
	if !contains(src, "function groupByDirectory") {
		t.Error("Expected script to contain groupByDirectory function")
	}
	if !contains(src, "function createSplitPlan") {
		t.Error("Expected script to contain createSplitPlan function")
	}
	if !contains(src, "function executeSplit") {
		t.Error("Expected script to contain executeSplit function")
	}
	if !contains(src, "function verifyEquivalence") {
		t.Error("Expected script to contain verifyEquivalence function")
	}
	if !contains(src, "tui.registerMode") {
		t.Error("Expected script to register TUI mode")
	}
	if !contains(src, "VERSION") {
		t.Error("Expected script to contain VERSION constant")
	}
}

func TestPrSplitCommand_ConfigInjection(t *testing.T) {
	skipSlow(t)
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	var stdout, stderr bytes.Buffer

	// Set non-default flag values to verify they're injected into JS
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()
	cmd.baseBranch = "develop"
	cmd.strategy = "extension"
	cmd.maxFiles = 3
	cmd.dryRun = true

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with custom config, got: %v", err)
	}

	output := stdout.String()
	// The TUI onEnter message should show the injected config
	if !contains(output, "base=develop") {
		t.Errorf("Expected output to contain injected baseBranch, got: %s", output)
	}
	if !contains(output, "strategy=extension") {
		t.Errorf("Expected output to contain injected strategy, got: %s", output)
	}
	if !contains(output, "max=3") {
		t.Errorf("Expected output to contain injected maxFiles, got: %s", output)
	}
	if !contains(output, "DRY RUN") {
		t.Errorf("Expected output to contain DRY RUN indicator, got: %s", output)
	}
}

// setupTestGitRepoWithDeletions creates a repo where the feature branch
// adds new files AND deletes an existing file (README.md).
func setupTestGitRepoWithDeletions(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	for _, f := range []struct{ path, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"README.md", "# Test Project\n"},
		{"docs/old-guide.md", "# Old Guide\n\nDeprecated.\n"},
	} {
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

	runGitCmd(t, dir, "checkout", "-b", "feature")

	// Add new files.
	for _, f := range []struct{ path, content string }{
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Delete existing files.
	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "docs", "old-guide.md")); err != nil {
		t.Fatal(err)
	}

	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature: add impl, docs; delete README and old-guide")

	return dir
}

// TestPrSplitCommand_RunWithDeletedFiles verifies that the run handler
// correctly handles deleted files (uses git rm instead of checkout).
func TestPrSplitCommand_RunWithDeletedFiles(t *testing.T) {
	skipSlow(t)
	dir := setupTestGitRepoWithDeletions(t)

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
		t.Fatalf("run command returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (with deletions) output:\n%s", output)

	// Should complete full workflow.
	if !contains(output, "Analysis:") {
		t.Error("expected analysis output")
	}
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification — deletions must be handled correctly")
	}

	// Verify we're back on feature.
	current := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if current != "feature" {
		t.Errorf("expected to be on 'feature' branch, got %q", current)
	}
}

// TestPrSplitCommand_RunRerun verifies that running pr-split twice on the
// same branch works (pre-existing split branches are cleaned up).
func TestPrSplitCommand_RunRerun(t *testing.T) {
	skipSlow(t)
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout1, dispatch1 := loadPrSplitEngine(t, nil)

	// First run.
	if err := dispatch1("run", nil); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	out1 := stdout1.String()
	if !contains(out1, "Split executed:") {
		t.Fatalf("first run did not execute splits:\n%s", out1)
	}

	// Verify branches exist after first run.
	branches := runGitCmd(t, dir, "branch")
	if !strings.Contains(branches, "split/") {
		t.Fatalf("expected split branches after first run, got:\n%s", branches)
	}

	// Second run — branches already exist. Must not crash.
	stdout2, dispatch2 := loadPrSplitEngine(t, nil)
	if err := dispatch2("run", nil); err != nil {
		t.Fatalf("second run failed: %v", err)
	}

	out2 := stdout2.String()
	t.Logf("second run output:\n%s", out2)

	if !contains(out2, "Split executed:") {
		t.Error("second run should still execute splits")
	}
	if !contains(out2, "Tree hash equivalence verified") {
		t.Error("second run should verify equivalence")
	}
}

// ---------------------------------------------------------------------------
// T014: Integration tests — heuristic pr-split end-to-end
// ---------------------------------------------------------------------------

// setupTestGitRepoExtension creates a temp git repo where the feature
// branch has files with diverse extensions to exercise the extension
// grouping strategy.
func setupTestGitRepoExtension(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial commit")

	runGitCmd(t, dir, "checkout", "-b", "feature")
	for _, f := range []struct{ path, content string }{
		{"main.go", "package main\n\nfunc main() {}\n"},
		{"util.go", "package main\n\nfunc util() string { return \"ok\" }\n"},
		{"docs/guide.md", "# Guide\n"},
		{"docs/api.md", "# API\n"},
		{"config.yaml", "key: value\n"},
		{"config.json", "{\"a\":1}\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature: mixed extensions")

	return dir
}

// TestPrSplitCommand_RunExtensionStrategy tests the extension grouping
// strategy end-to-end: analyze → group by extension → plan → execute → verify.
func TestPrSplitCommand_RunExtensionStrategy(t *testing.T) {
	skipSlow(t)
	dir := setupTestGitRepoExtension(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"strategy": "extension",
	})

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run (extension) returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (extension) output:\n%s", output)

	// Should use extension strategy.
	if !contains(output, "extension") {
		t.Error("expected extension strategy name in output")
	}

	// Should produce groups based on extensions (.go, .md, .yaml, .json).
	if !contains(output, "Grouped into") {
		t.Error("expected grouping output")
	}

	// Should complete full workflow.
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification")
	}

	// Verify that split branches exist.
	branches := runGitCmd(t, dir, "branch")
	if !strings.Contains(branches, "split/") {
		t.Errorf("expected split branches, got:\n%s", branches)
	}

	// We're back on feature.
	current := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	if current != "feature" {
		t.Errorf("expected to be on 'feature' branch, got %q", current)
	}
}

// setupTestGitRepoWithModifications creates a repo where the feature branch
// modifies existing files AND adds new files.
func setupTestGitRepoWithModifications(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Seed initial files.
	for _, f := range []struct{ path, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"README.md", "# Test Project\n"},
		{"Makefile", "build:\n\techo ok\n"},
	} {
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

	// Feature branch: modify existing files + add new files.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	for _, f := range []struct{ path, content string }{
		// Modifications to existing files.
		{"pkg/types.go", "package pkg\n\ntype Foo struct{ Name string }\n\ntype Bar struct{}\n"},
		{"README.md", "# Test Project\n\nThis is updated.\n"},
		// New files.
		{"pkg/impl.go", "package pkg\n\nfunc NewFoo() Foo { return Foo{Name: \"default\"} }\n"},
		{"docs/spec.md", "# Spec\n\nDesign document.\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature: modify types + add impl + update readme")

	return dir
}

// TestPrSplitCommand_RunWithModifications verifies that modified files
// (not just additions) are correctly handled through the split workflow.
func TestPrSplitCommand_RunWithModifications(t *testing.T) {
	skipSlow(t)
	dir := setupTestGitRepoWithModifications(t)

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
		t.Fatalf("run (modifications) returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (modifications) output:\n%s", output)

	// Should show analysis with correct count.
	if !contains(output, "4 changed files") {
		t.Error("expected 4 changed files (2 modified + 2 new)")
	}

	// Should complete full workflow.
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification — modifications must be handled correctly")
	}

	// Verify content on the last split branch matches the feature branch.
	featureTree := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "feature^{tree}"))
	// Find the last split branch.
	branchesRaw := runGitCmd(t, dir, "branch")
	var lastSplit string
	for line := range strings.SplitSeq(branchesRaw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		if strings.HasPrefix(line, "split/") {
			lastSplit = line
		}
	}
	if lastSplit == "" {
		t.Fatal("no split branches found")
	}
	splitTree := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", lastSplit+"^{tree}"))
	if featureTree != splitTree {
		t.Errorf("tree mismatch: feature=%s split=%s", featureTree, splitTree)
	}
}

// setupCompilableGoRepo creates a temp git repo with properly compilable Go
// code. Both main and feature branch have valid Go modules.
func setupCompilableGoRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Create a valid Go module.
	for _, f := range []struct{ path, content string }{
		{"go.mod", "module example.com/testrepo\n\ngo 1.21\n"},
		{"main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"},
		{"pkg/types.go", "package pkg\n\n// Config holds configuration.\ntype Config struct {\n\tName string\n}\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial: valid Go module")

	// Verify base compiles.
	goCmd := exec.Command("go", "build", "./...")
	goCmd.Dir = dir
	if out, err := goCmd.CombinedOutput(); err != nil {
		t.Fatalf("base does not compile: %s", string(out))
	}

	// Create feature branch with new packages and modifications.
	runGitCmd(t, dir, "checkout", "-b", "feature")
	for _, f := range []struct{ path, content string }{
		// New package.
		{"internal/helper/help.go", "package helper\n\n// Greet returns a greeting.\nfunc Greet(name string) string {\n\treturn \"Hello, \" + name\n}\n"},
		// Modify existing.
		{"main.go", "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/testrepo/internal/helper\"\n)\n\nfunc main() {\n\tfmt.Println(helper.Greet(\"world\"))\n}\n"},
		// New docs (non-Go).
		{"docs/README.md", "# Documentation\n\nUsage guide.\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature: add helper package + update main")

	// Verify feature compiles.
	goCmd = exec.Command("go", "build", "./...")
	goCmd.Dir = dir
	if out, err := goCmd.CombinedOutput(); err != nil {
		t.Fatalf("feature does not compile: %s", string(out))
	}

	return dir
}

// TestPrSplitCommand_RunCompilableGoRepo creates a real Go project with
// cross-package dependencies and verifies that:
// 1. The split completes successfully.
// 2. Tree hash equivalence holds.
// 3. The FINAL split branch actually compiles with `go build`.
func TestPrSplitCommand_RunCompilableGoRepo(t *testing.T) {
	skipSlow(t)
	dir := setupCompilableGoRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"verifyCommand": "go build ./...",
	})

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run (compilable) returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (compilable) output:\n%s", output)

	// Full workflow should complete.
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification")
	}

	// Verify that the LAST split branch compiles.
	branchesRaw := runGitCmd(t, dir, "branch")
	var lastSplit string
	for line := range strings.SplitSeq(branchesRaw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		if strings.HasPrefix(line, "split/") {
			lastSplit = line
		}
	}
	if lastSplit == "" {
		t.Fatal("no split branches found")
	}

	// Checkout the last split branch and run `go build`.
	runGitCmd(t, dir, "checkout", lastSplit)
	goCmd := exec.Command("go", "build", "./...")
	goCmd.Dir = dir
	if out, err := goCmd.CombinedOutput(); err != nil {
		t.Errorf("last split branch %q does not compile: %s", lastSplit, string(out))
	}

	// Return to feature branch.
	runGitCmd(t, dir, "checkout", "feature")
}

// TestPrSplitCommand_RunChainIntegrity verifies that split branches form
// a proper chain: each branch's parent commit is on the previous branch
// in the sequence (starting from the base branch).
func TestPrSplitCommand_RunChainIntegrity(t *testing.T) {
	skipSlow(t)
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

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	output := stdout.String()
	if !contains(output, "Tree hash equivalence verified") {
		t.Fatalf("run did not complete successfully:\n%s", output)
	}

	// Collect split branches in order.
	branchesRaw := runGitCmd(t, dir, "branch")
	var splitBranches []string
	for line := range strings.SplitSeq(branchesRaw, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		if strings.HasPrefix(line, "split/") {
			splitBranches = append(splitBranches, line)
		}
	}
	if len(splitBranches) < 2 {
		t.Skipf("only %d split branches — need ≥2 to verify chain", len(splitBranches))
	}

	// Verify chain: first branch's parent is on "main",
	// subsequent branches' parents are on the previous branch.
	expectedParent := "main"
	for _, branch := range splitBranches {
		// Get the parent commit of the branch tip.
		parentSHA := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", branch+"^"))
		// Get the tip of the expected parent branch.
		parentBranchSHA := strings.TrimSpace(runGitCmd(t, dir, "rev-parse", expectedParent))

		if parentSHA != parentBranchSHA {
			t.Errorf("chain broken: %s parent is %s, expected %s tip %s",
				branch, parentSHA[:8], expectedParent, parentBranchSHA[:8])
		}
		expectedParent = branch
	}
	t.Logf("✓ chain integrity verified: main → %s", strings.Join(splitBranches, " → "))
}

// TestPrSplitCommand_VerifyCommand tests the verify TUI command which runs
// the verify command on each split branch.
func TestPrSplitCommand_VerifyCommand(t *testing.T) {
	skipSlow(t)
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"verifyCommand": "true", // Always succeed.
	})

	// First run the full workflow to create branches.
	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	runOutput := stdout.String()
	if !contains(runOutput, "Split executed:") {
		t.Fatalf("run did not complete:\n%s", runOutput)
	}

	// Now run verify.
	stdout.Reset()
	if err := dispatch("verify", nil); err != nil {
		t.Fatalf("verify returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("verify output:\n%s", output)

	if !contains(output, "Verifying") {
		t.Error("expected 'Verifying' in output")
	}
	if !contains(output, "All splits pass verification") {
		t.Errorf("expected all splits to pass verification, got:\n%s", output)
	}
}

// TestPrSplitCommand_SetCommand tests the 'set' TUI command for changing
// runtime configuration.
func TestPrSplitCommand_SetCommand(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	stdout, dispatch := loadPrSplitEngine(t, nil)

	// Set base branch.
	if err := dispatch("set", []string{"base", "develop"}); err != nil {
		t.Fatalf("set base returned error: %v", err)
	}
	output := stdout.String()
	if !contains(output, "develop") {
		t.Errorf("expected 'develop' confirmation, got: %s", output)
	}

	// Set strategy.
	stdout.Reset()
	if err := dispatch("set", []string{"strategy", "extension"}); err != nil {
		t.Fatalf("set strategy returned error: %v", err)
	}
	output = stdout.String()
	if !contains(output, "extension") {
		t.Errorf("expected 'extension' confirmation, got: %s", output)
	}

	// Set dry-run.
	stdout.Reset()
	if err := dispatch("set", []string{"dry-run", "true"}); err != nil {
		t.Fatalf("set dry-run returned error: %v", err)
	}
	output = stdout.String()
	if !contains(output, "true") {
		t.Errorf("expected 'true' confirmation, got: %s", output)
	}
}

// TestPrSplitCommand_AnalyzeAndStatsCommands tests the analyze and stats
// TUI commands individually.
func TestPrSplitCommand_AnalyzeAndStatsCommands(t *testing.T) {
	skipSlow(t)
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

	// Test analyze.
	if err := dispatch("analyze", nil); err != nil {
		t.Fatalf("analyze returned error: %v", err)
	}
	output := stdout.String()
	t.Logf("analyze output:\n%s", output)
	if !contains(output, "Changed files: 4") {
		t.Errorf("expected 4 changed files, got:\n%s", output)
	}
	if !contains(output, "feature") && !contains(output, "Branch:") {
		t.Error("expected branch info in analyze output")
	}

	// Test stats.
	stdout.Reset()
	if err := dispatch("stats", nil); err != nil {
		t.Fatalf("stats returned error: %v", err)
	}
	output = stdout.String()
	t.Logf("stats output:\n%s", output)
	if !contains(output, "File stats") {
		t.Errorf("expected 'File stats' in output, got:\n%s", output)
	}
}

// TestPrSplitCommand_StepByStep exercises each step individually:
// analyze → group → plan → execute → verify → equivalence → cleanup.
func TestPrSplitCommand_StepByStep(t *testing.T) {
	skipSlow(t)
	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"verifyCommand": "true",
	})

	// Step 1: analyze
	if err := dispatch("analyze", nil); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if !contains(stdout.String(), "Changed files:") {
		t.Error("analyze: expected changed files")
	}

	// Step 2: group
	stdout.Reset()
	if err := dispatch("group", nil); err != nil {
		t.Fatalf("group: %v", err)
	}
	if !contains(stdout.String(), "Groups") {
		t.Error("group: expected Groups output")
	}

	// Step 3: plan
	stdout.Reset()
	if err := dispatch("plan", nil); err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !contains(stdout.String(), "Plan created:") {
		t.Error("plan: expected Plan created output")
	}

	// Step 4: preview
	stdout.Reset()
	if err := dispatch("preview", nil); err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !contains(stdout.String(), "Split Plan Preview") {
		t.Error("preview: expected preview output")
	}

	// Step 5: execute
	stdout.Reset()
	if err := dispatch("execute", nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !contains(stdout.String(), "Split completed successfully") {
		t.Errorf("execute: expected success, got:\n%s", stdout.String())
	}

	// Step 6: verify
	stdout.Reset()
	if err := dispatch("verify", nil); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !contains(stdout.String(), "All splits pass verification") {
		t.Errorf("verify: expected all pass, got:\n%s", stdout.String())
	}

	// Step 7: equivalence
	stdout.Reset()
	if err := dispatch("equivalence", nil); err != nil {
		t.Fatalf("equivalence: %v", err)
	}
	if !contains(stdout.String(), "Trees are equivalent") {
		t.Errorf("equivalence: expected equivalent, got:\n%s", stdout.String())
	}

	// Step 8: cleanup
	stdout.Reset()
	if err := dispatch("cleanup", nil); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if !contains(stdout.String(), "Deleted branches") {
		t.Errorf("cleanup: expected deletion, got:\n%s", stdout.String())
	}

	// After cleanup, no split branches should remain.
	branches := runGitCmd(t, dir, "branch")
	if strings.Contains(branches, "split/") {
		t.Errorf("expected no split branches after cleanup, got:\n%s", branches)
	}
}

// ---------------------------------------------------------------------------
// T029: JSON reporting
// ---------------------------------------------------------------------------

// TestPrSplitCommand_ReportCommand tests the report TUI command that outputs
// current state as JSON.
func TestPrSplitCommand_ReportCommand(t *testing.T) {
	skipSlow(t)
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

	// First run the full workflow.
	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !contains(stdout.String(), "Tree hash equivalence verified") {
		t.Fatalf("run did not complete:\n%s", stdout.String())
	}

	// Now run report.
	stdout.Reset()
	if err := dispatch("report", nil); err != nil {
		t.Fatalf("report returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("report output:\n%s", output)

	// Should be valid JSON.
	if !strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Errorf("expected JSON output starting with '{', got:\n%s", output)
	}

	// Should contain key fields in JSON.
	if !contains(output, "\"baseBranch\"") {
		t.Error("expected baseBranch field in JSON report")
	}
	if !contains(output, "\"analysis\"") {
		t.Error("expected analysis field in JSON report")
	}
	if !contains(output, "\"plan\"") {
		t.Error("expected plan field in JSON report")
	}
	if !contains(output, "\"equivalence\"") {
		t.Error("expected equivalence field in JSON report")
	}
	if !contains(output, "\"verified\"") {
		t.Error("expected verified field in equivalence")
	}
}

// ---------------------------------------------------------------------------
// T027: Config section support
// ---------------------------------------------------------------------------

// TestPrSplitCommand_ConfigOverrides verifies that config file settings
// are applied as defaults and flags override them.
func TestPrSplitCommand_ConfigOverrides(t *testing.T) {
	skipSlow(t)
	cfg := config.NewConfig()
	cfg.Commands["pr-split"] = map[string]string{
		"base":     "develop",
		"strategy": "extension",
		"max":      "5",
		"prefix":   "pr/",
		"verify":   "go test ./...",
		"provider": "claude-code",
		"model":    "sonnet",
	}

	cmd := NewPrSplitCommand(cfg)

	var stdout, stderr bytes.Buffer

	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()
	// Config should be applied — check the onEnter output.
	if !contains(output, "base=develop") {
		t.Errorf("expected config base=develop in output, got: %s", output)
	}
	if !contains(output, "strategy=extension") {
		t.Errorf("expected config strategy=extension in output, got: %s", output)
	}
	if !contains(output, "max=5") {
		t.Errorf("expected config max=5 in output, got: %s", output)
	}
}

// ---------------------------------------------------------------------------
//  Plan Persistence (T038)
// ---------------------------------------------------------------------------

// TestPrSplitCommand_SaveLoadPlan exercises the save-plan and load-plan
// TUI commands. It creates a full plan (dry-run), saves it, then loads
// it into a fresh engine and verifies the plan state is restored.
func TestPrSplitCommand_SaveLoadPlan(t *testing.T) {
	skipSlow(t)
	// NOT parallel — we chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Phase 1: Create plan in dry-run mode and save it.
	stdout1, dispatch1 := loadPrSplitEngine(t, map[string]any{
		"dryRun": true,
	})

	if err := dispatch1("run", nil); err != nil {
		t.Fatalf("run (dry-run) returned error: %v", err)
	}
	output1 := stdout1.String()
	if !contains(output1, "DRY RUN") {
		t.Fatal("expected dry-run output")
	}

	// Save plan to a specific file.
	planFile := filepath.Join(dir, "test-plan.json")
	stdout1.Reset()
	if err := dispatch1("save-plan", []string{planFile}); err != nil {
		t.Fatalf("save-plan returned error: %v", err)
	}
	saveOutput := stdout1.String()
	t.Logf("save-plan output:\n%s", saveOutput)
	if !contains(saveOutput, "Plan saved to") {
		t.Error("expected 'Plan saved to' in output")
	}

	// Verify file exists.
	if _, err := os.Stat(planFile); errors.Is(err, os.ErrNotExist) {
		t.Fatal("plan file does not exist after save-plan")
	}

	// Read and verify it's valid JSON with expected structure.
	planData, err := os.ReadFile(planFile)
	if err != nil {
		t.Fatalf("failed to read plan file: %v", err)
	}
	if !strings.Contains(string(planData), `"version": 1`) {
		t.Error("plan file missing version field")
	}
	if !strings.Contains(string(planData), `"splits"`) {
		t.Error("plan file missing splits field")
	}

	// Phase 2: Load plan into a fresh engine.
	stdout2, dispatch2 := loadPrSplitEngine(t, nil)

	if err := dispatch2("load-plan", []string{planFile}); err != nil {
		t.Fatalf("load-plan returned error: %v", err)
	}
	loadOutput := stdout2.String()
	t.Logf("load-plan output:\n%s", loadOutput)
	if !contains(loadOutput, "Plan loaded from") {
		t.Error("expected 'Plan loaded from' in output")
	}
	if !contains(loadOutput, "Total splits:") {
		t.Error("expected 'Total splits:' in output")
	}
	if !contains(loadOutput, "Pending:") {
		t.Error("expected 'Pending:' in output")
	}

	// Phase 3: Verify we can preview the loaded plan.
	stdout2.Reset()
	if err := dispatch2("preview", nil); err != nil {
		t.Fatalf("preview returned error: %v", err)
	}
	previewOutput := stdout2.String()
	t.Logf("preview output after load:\n%s", previewOutput)
	if !contains(previewOutput, "Plan:") && !contains(previewOutput, "splits") && !contains(previewOutput, "split/") {
		t.Error("expected plan details in preview after loading")
	}
}

// TestPrSplitCommand_CreatePRsGuards verifies that the create-prs command
// requires a plan and executed splits before attempting PR creation.
func TestPrSplitCommand_CreatePRsGuards(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// create-prs without a plan should fail gracefully.
	if err := dispatch("create-prs", nil); err != nil {
		t.Fatalf("create-prs returned error: %v", err)
	}
	output := stdout.String()
	if !contains(output, "No plan") {
		t.Errorf("expected 'No plan' guard, got: %s", output)
	}
}

// TestPrSplitCommand_FixGuards verifies that the fix command requires a plan.
func TestPrSplitCommand_FixGuards(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	stdout, dispatch := loadPrSplitEngine(t, nil)

	// fix without a plan should fail gracefully.
	if err := dispatch("fix", nil); err != nil {
		t.Fatalf("fix returned error: %v", err)
	}
	output := stdout.String()
	if !contains(output, "No plan") {
		t.Errorf("expected 'No plan' guard, got: %s", output)
	}
}

// TestPrSplitCommand_PlanEditing exercises the interactive plan editing
// commands: move, rename, merge, reorder.
func TestPrSplitCommand_PlanEditing(t *testing.T) {
	skipSlow(t)
	// NOT parallel — we chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := setupTestGitRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"dryRun": true,
	})

	// Create a plan via dry-run.
	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run (dry-run) returned error: %v", err)
	}
	_ = stdout.String()

	// Test rename: rename split 1.
	stdout.Reset()
	if err := dispatch("rename", []string{"1", "infrastructure"}); err != nil {
		t.Fatalf("rename returned error: %v", err)
	}
	renameOutput := stdout.String()
	t.Logf("rename output: %s", renameOutput)
	if !contains(renameOutput, "Renamed split 1") {
		t.Error("expected rename confirmation")
	}
	if !contains(renameOutput, "infrastructure") {
		t.Error("expected new name in output")
	}

	// Test move: move a file between splits (only if there are 2+ splits).
	stdout.Reset()
	if err := dispatch("preview", nil); err != nil {
		t.Fatalf("preview returned error: %v", err)
	}
	previewOutput := stdout.String()
	t.Logf("preview after rename:\n%s", previewOutput)

	// Verify rename took effect.
	if !contains(previewOutput, "infrastructure") {
		t.Error("expected renamed split in preview")
	}

	// Test merge: merge last split into first (if 3+ splits).
	if strings.Count(previewOutput, "Split ") >= 3 {
		stdout.Reset()
		if err := dispatch("merge", []string{"1", "3"}); err != nil {
			t.Fatalf("merge returned error: %v", err)
		}
		mergeOutput := stdout.String()
		t.Logf("merge output: %s", mergeOutput)
		if !contains(mergeOutput, "Merged split") {
			t.Error("expected merge confirmation")
		}
	}

	// Test edge cases: move without plan.
	stdout.Reset()
	if err := dispatch("move", nil); err != nil {
		t.Fatalf("move returned error: %v", err)
	}
	if !contains(stdout.String(), "Usage:") {
		t.Error("expected usage hint for move without args")
	}

	// Test reorder: check that it updates plan position and renumber.
	stdout.Reset()
	if err := dispatch("preview", nil); err != nil {
		t.Fatalf("preview (pre-reorder) returned error: %v", err)
	}
	previewBeforeReorder := stdout.String()
	splitCountBefore := strings.Count(previewBeforeReorder, "Split ")
	t.Logf("preview before reorder (%d splits):\n%s", splitCountBefore, previewBeforeReorder)

	if splitCountBefore >= 2 {
		stdout.Reset()
		if err := dispatch("reorder", []string{"1", "2"}); err != nil {
			t.Fatalf("reorder returned error: %v", err)
		}
		reorderOutput := stdout.String()
		t.Logf("reorder output: %s", reorderOutput)
		if !contains(reorderOutput, "Moved split from position") {
			t.Error("expected reorder confirmation")
		}

		// Verify preview reflects the new order.
		stdout.Reset()
		if err := dispatch("preview", nil); err != nil {
			t.Fatalf("preview (post-reorder) returned error: %v", err)
		}
		previewAfterReorder := stdout.String()
		t.Logf("preview after reorder:\n%s", previewAfterReorder)
		// Split count should remain the same.
		splitCountAfter := strings.Count(previewAfterReorder, "Split ")
		if splitCountAfter != splitCountBefore {
			t.Errorf("reorder changed split count: %d → %d", splitCountBefore, splitCountAfter)
		}
	}
}
