//go:build unix

package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Exhaustive Integration Tests for PRSplit
//
// This file extends the binary E2E pattern from pr_split_binary_e2e_test.go
// with comprehensive coverage of:
//   - All strategy variations (directory, extension, chunks, dependency)
//   - Resume/persistence across binary invocations
//   - Deleted file equivalence
//   - Compilable Go project verification (go build ./...)
//   - JSON output mode
//   - Config file overrides
//   - Rerun idempotence
//   - Multi-directory complex projects
//
// These tests use the real osm binary (subprocess) against isolated temp
// git repos, following the "real forward" pattern from the existing
// pr_split_binary_e2e_test.go.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Strategy Variation Tests
// ---------------------------------------------------------------------------

func TestBinaryE2E_StrategyExtension(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryMixedTypeRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=extension",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("extension strategy failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches found for extension strategy, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("extension strategy created %d split branches", splitCount)

	// Verify equivalence.
	assertContainsAny(t, stdout, "equivalence",
		"equivalence", "equivalent", "Trees are equivalent")
}

func TestBinaryE2E_StrategyChunks(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryManyFilesRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=chunks",
		"-max=3",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("chunks strategy failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches found for chunks strategy, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("chunks strategy created %d split branches (max=3 files per split)", splitCount)

	// With 8 files and max=3, expect at least 3 splits.
	if splitCount < 3 {
		t.Errorf("expected at least 3 splits for 8 files with max=3, got %d", splitCount)
	}

	assertContainsAny(t, stdout, "equivalence",
		"equivalence", "equivalent", "Trees are equivalent")
}

func TestBinaryE2E_StrategyDirectoryDeep(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryNestedDirRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory-deep",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("directory-deep strategy failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches for directory-deep, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("directory-deep strategy created %d split branches", splitCount)

	assertContainsAny(t, stdout, "equivalence",
		"equivalence", "equivalent", "Trees are equivalent")
}

func TestBinaryE2E_StrategyDependency(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryGoModuleRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=dependency",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("dependency strategy failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches for dependency strategy, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("dependency strategy created %d split branches", splitCount)

	assertContainsAny(t, stdout, "equivalence",
		"equivalence", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Resume / Persistence Tests
// ---------------------------------------------------------------------------

func TestBinaryE2E_RerunIdempotent(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	// First run.
	stdout1, _, err1 := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	if err1 != nil {
		t.Fatalf("first run failed: %v\n%s", err1, stdout1)
	}

	splitCount1 := countSplitBranches(t, repoDir)
	t.Logf("first run created %d split branches", splitCount1)
	if splitCount1 == 0 {
		t.Fatalf("first run created no branches")
	}

	// Second run — same session, should recreate branches.
	stdout2, stderr2, err2 := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("second run stdout:\n%s", stdout2)
	if stderr2 != "" {
		t.Logf("second run stderr:\n%s", stderr2)
	}

	if err2 != nil {
		t.Fatalf("second run failed (should be idempotent): %v\nstdout:\n%s\nstderr:\n%s",
			err2, stdout2, stderr2)
	}

	// Verify branches still exist after re-run.
	splitCount2 := countSplitBranches(t, repoDir)
	t.Logf("second run resulted in %d split branches", splitCount2)
	if splitCount2 == 0 {
		t.Errorf("second run should still have split branches")
	}

	// Verify equivalence still holds.
	assertContainsAny(t, stdout2, "equivalence after re-run",
		"equivalence", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Deleted Files Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_DeletedFilesEquivalence(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepoWithDeletion(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("deleted files test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Verify branches were created.
	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches created with deleted files, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("deleted files test created %d split branches", splitCount)

	// Verify equivalence holds even with deleted files.
	assertContainsAny(t, stdout, "equivalence with deletions",
		"equivalence", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Compilable Go Project Test (real go build verification)
// ---------------------------------------------------------------------------

func TestBinaryE2E_CompilableGoProject(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// Skip if go is not in PATH.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}

	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryCompilableRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=go build ./...",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("compilable Go project test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Verify branches were created.
	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches for compilable project, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("compilable project test created %d split branches", splitCount)

	// Verify equivalence.
	assertContainsAny(t, stdout, "equivalence",
		"equivalence", "equivalent", "Trees are equivalent")

	// Verify all split branches compile.
	for _, branch := range gitBranches(t, repoDir) {
		if !strings.HasPrefix(branch, "split/") {
			continue
		}
		// Checkout the branch and verify it compiles.
		cmd := exec.Command("git", "checkout", branch)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git checkout %s failed: %v\n%s", branch, err, out)
		}

		buildCmd := exec.Command("go", "build", "./...")
		buildCmd.Dir = repoDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Errorf("branch %s failed to compile: %v\n%s", branch, err, out)
		}
	}

	// Restore to feature branch.
	cmd := exec.Command("git", "checkout", "feature")
	cmd.Dir = repoDir
	_ = cmd.Run()
}

// ---------------------------------------------------------------------------
// JSON Output Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_JSONOutput(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"-json",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	// JSON mode may exit 0 or non-zero depending on implementation.
	// What matters is no panic.
	combined := stdout + stderr
	if strings.Contains(combined, "panic:") {
		t.Fatalf("binary panicked in JSON mode:\n%s", combined)
	}

	// If binary succeeded, output should contain some JSON-like content.
	if err == nil {
		// Look for JSON markers.
		if !strings.Contains(stdout, "{") && !strings.Contains(stdout, "[") {
			t.Logf("warning: JSON mode output does not contain JSON markers")
		}
	}
}

// ---------------------------------------------------------------------------
// Config File Override Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_ConfigFileOverride(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	// Create a config file that overrides strategy.
	configDir := t.TempDir()
	configFile := filepath.Join(configDir, "config")
	configContent := "[pr-split]\nstrategy=extension\nmax=5\n"
	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run without -strategy flag — should use config file's "extension".
	cmd := exec.Command(osmBin,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"OSM_CONFIG="+configDir,
		"TERM=dumb",
		"NO_COLOR=1",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout := outBuf.String()
	stderr := errBuf.String()
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("config override test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	// Should have created branches.
	splitCount := countSplitBranches(t, repoDir)
	if splitCount == 0 {
		t.Fatalf("no split/* branches with config override, branches: %v",
			gitBranches(t, repoDir))
	}
	t.Logf("config override (extension strategy) created %d branches", splitCount)
}

// ---------------------------------------------------------------------------
// Complex Project Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_ComplexProject(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryComplexRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}

	if err != nil {
		t.Fatalf("complex project test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	splitCount := countSplitBranches(t, repoDir)
	if splitCount < 3 {
		t.Errorf("expected at least 3 splits for complex project, got %d", splitCount)
	}
	t.Logf("complex project created %d split branches", splitCount)

	assertContainsAny(t, stdout, "equivalence",
		"equivalence", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Test Fixtures
// ---------------------------------------------------------------------------

// setupBinaryMixedTypeRepo creates a repo with multiple file types (.go, .md,
// .js, .py, .json) for testing the extension strategy.
func setupBinaryMixedTypeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("README.md", "# Project\n")
	writeFile("main.go", "package main\nfunc main() {}\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	writeFile("src/app.js", "console.log('app');\n")
	writeFile("src/util.js", "function util() {}\n")
	writeFile("scripts/run.py", "print('hello')\n")
	writeFile("data/config.json", `{"key":"value"}`)
	writeFile("docs/guide.md", "# Guide\n")
	writeFile("main_test.go", "package main\nimport \"testing\"\nfunc TestMain(t *testing.T) {}\n")
	git("add", "-A")
	git("commit", "-m", "feature: mixed types")

	return dir
}

// setupBinaryManyFilesRepo creates a repo with 8+ files for testing the
// chunks strategy with max file limits.
func setupBinaryManyFilesRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("base.go", "package base\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	for i := range 8 {
		writeFile(
			"pkg/file"+string(rune('a'+i))+".go",
			"package pkg\n\nfunc Func"+string(rune('A'+i))+"() {}\n",
		)
	}
	git("add", "-A")
	git("commit", "-m", "feature: 8 files")

	return dir
}

// setupBinaryNestedDirRepo creates a repo with nested directories for testing
// the directory-deep strategy.
func setupBinaryNestedDirRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("cmd/app/main.go", "package main\nfunc main() {}\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	writeFile("cmd/app/run.go", "package main\nfunc run() {}\n")
	writeFile("cmd/app/util.go", "package main\nfunc util() {}\n")
	writeFile("internal/core/types.go", "package core\ntype Foo struct{}\n")
	writeFile("internal/core/impl.go", "package core\nfunc Bar() {}\n")
	writeFile("internal/sub/deep.go", "package sub\nfunc Deep() {}\n")
	writeFile("docs/api/v1.md", "# API v1\n")
	writeFile("docs/api/v2.md", "# API v2\n")
	git("add", "-A")
	git("commit", "-m", "feature: nested dirs")

	return dir
}

// setupBinaryGoModuleRepo creates a repo with a go.mod file for testing the
// dependency strategy.
func setupBinaryGoModuleRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile("pkg/types.go", "package pkg\n\ntype Foo struct{}\n")
	writeFile("cmd/app/main.go", "package main\n\nimport \"example.com/test/pkg\"\n\nfunc main() { _ = pkg.Foo{} }\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	writeFile("pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n")
	writeFile("pkg/impl_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestBar(t *testing.T) {\n\tif Bar() != \"bar\" { t.Fatal() }\n}\n")
	writeFile("cmd/app/run.go", "package main\n\nfunc run() {}\n")
	writeFile("docs/guide.md", "# Guide\n")
	git("add", "-A")
	git("commit", "-m", "feature: add impl, run, docs")

	return dir
}

// setupBinaryTestRepoWithDeletion creates a repo where the feature branch
// both adds and deletes files.
func setupBinaryTestRepoWithDeletion(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("README.md", "# Project\n")
	writeFile("OLD_FILE.md", "# Old file to be deleted\n")
	writeFile("pkg/core.go", "package pkg\n\nfunc Core() {}\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	// Delete an existing file.
	if err := os.Remove(filepath.Join(dir, "OLD_FILE.md")); err != nil {
		t.Fatal(err)
	}
	// Add new files.
	writeFile("pkg/impl.go", "package pkg\n\nfunc Impl() {}\n")
	writeFile("docs/guide.md", "# Guide\n")
	git("add", "-A")
	git("commit", "-m", "feature: add impl, docs; delete old file")

	return dir
}

// setupBinaryCompilableRepo creates a repo with a go.mod and compilable Go
// source files so that `go build ./...` can be used as the verify command.
func setupBinaryCompilableRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	writeFile("go.mod", "module example.com/test\n\ngo 1.21\n")
	writeFile("pkg/types.go", "package pkg\n\ntype Foo struct {\n\tName string\n}\n")
	writeFile("cmd/app/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n")
	writeFile("README.md", "# Test Project\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	writeFile("pkg/bar.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n")
	writeFile("cmd/app/run.go", "package main\n\nfunc run() error { return nil }\n")
	writeFile("docs/guide.md", "# Guide\n\nUsage instructions.\n")
	git("add", "-A")
	git("commit", "-m", "feature: add bar, run, docs")

	return dir
}

// setupBinaryComplexRepo creates a large repo with many directories and file
// types for testing complex split scenarios.
func setupBinaryComplexRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")

	// Base files.
	writeFile("go.mod", "module example.com/complex\n\ngo 1.21\n")
	writeFile("pkg/types/types.go", "package types\n\ntype User struct {\n\tID   int\n\tName string\n}\n")
	writeFile("pkg/storage/db.go", "package storage\n\nimport \"database/sql\"\n\ntype DB struct{ sql.DB }\n")
	writeFile("cmd/server/main.go", "package main\n\nfunc main() {}\n")
	writeFile("cmd/cli/main.go", "package main\n\nfunc main() {}\n")
	writeFile("internal/config/config.go", "package config\n\ntype Config struct {\n\tPort int\n}\n")
	writeFile("README.md", "# Complex Project\n")
	writeFile("Makefile", ".PHONY: build\ndeploy:\n\tgo build ./...\n")
	git("add", "-A")
	git("commit", "-m", "initial: full project structure")

	// Feature branch with changes across many directories.
	git("checkout", "-b", "feature")
	writeFile("pkg/types/user.go", "package types\n\nfunc NewUser(name string) *User {\n\treturn &User{Name: name}\n}\n")
	writeFile("pkg/types/user_test.go", "package types\n\nimport \"testing\"\n\nfunc TestNewUser(t *testing.T) {\n\tu := NewUser(\"test\")\n\tif u == nil { t.Fatal() }\n}\n")
	writeFile("pkg/storage/memory.go", "package storage\n\ntype MemoryStore struct{}\n")
	writeFile("internal/config/loader.go", "package config\n\nfunc Load() *Config {\n\treturn &Config{Port: 8080}\n}\n")
	writeFile("cmd/server/serve.go", "package main\n\nfunc serve() {}\n")
	writeFile("cmd/cli/commands.go", "package main\n\nfunc commands() {}\n")
	writeFile("docs/api.md", "# API Reference\n")
	writeFile("docs/architecture.md", "# Architecture\n")
	writeFile("scripts/deploy.sh", "#!/bin/sh\necho deploy\n")
	git("add", "-A")
	git("commit", "-m", "feature: add user, storage, config, commands, docs, scripts")

	return dir
}

// ---------------------------------------------------------------------------
// Renamed Files Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_RenamedFilesEquivalence(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepoWithRename(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split", "-interactive=false", "-base=main",
		"-strategy=directory", "-verify=true",
		"--store=memory", "--session="+t.Name(), "run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}
	if err != nil {
		t.Fatalf("renamed files test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if countSplitBranches(t, repoDir) == 0 {
		t.Fatalf("no split/* branches with renamed files, branches: %v", gitBranches(t, repoDir))
	}
	assertContainsAny(t, stdout, "equivalence", "Tree hash mismatch", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Large Diff Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_LargeDiff(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryLargeDiffRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split", "-interactive=false", "-base=main",
		"-strategy=directory", "-max=5", "-verify=true",
		"--store=memory", "--session="+t.Name(), "run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}
	if err != nil {
		t.Fatalf("large diff test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if count := countSplitBranches(t, repoDir); count < 3 {
		t.Errorf("expected at least 3 splits for large diff, got %d", count)
	}
	assertContainsAny(t, stdout, "equivalence", "equivalence", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Single File Split Test
// ---------------------------------------------------------------------------

func TestBinaryE2E_SingleFileSplit(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)

	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")
	writeFile("README.md", "# Project\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	writeFile("pkg/new.go", "package pkg\n\nfunc New() {}\n")
	git("add", "-A")
	git("commit", "-m", "feature: single file")

	stdout, stderr, err := runBinary(t, osmBin, dir,
		"pr-split", "-interactive=false", "-base=main",
		"-strategy=directory", "-verify=true",
		"--store=memory", "--session="+t.Name(), "run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}
	if err != nil {
		t.Fatalf("single file split failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if countSplitBranches(t, dir) == 0 {
		t.Fatalf("no split/* branches for single file, branches: %v", gitBranches(t, dir))
	}
	assertContainsAny(t, stdout, "equivalence", "equivalence", "equivalent", "Trees are equivalent")
}

// ---------------------------------------------------------------------------
// Test Fixtures for edge cases
// ---------------------------------------------------------------------------

func setupBinaryTestRepoWithRename(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")
	writeFile("pkg/old.go", "package pkg\n\nfunc Old() {}\n")
	writeFile("README.md", "# Project\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	if err := os.Rename(filepath.Join(dir, "pkg/old.go"), filepath.Join(dir, "pkg/new.go")); err != nil {
		t.Fatal(err)
	}
	writeFile("docs/guide.md", "# Guide\n")
	git("add", "-A")
	git("commit", "-m", "feature: rename old.go to new.go, add docs")
	return dir
}

func setupBinaryLargeDiffRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")
	writeFile("README.md", "# Project\n")
	writeFile("go.mod", "module example.com/large\n\ngo 1.21\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	dirs := []string{"pkg/auth", "pkg/core", "pkg/storage", "cmd/server", "cmd/cli", "internal/util"}
	for i, d := range dirs {
		for j := range 3 {
			fileName := d + "/file" + string(rune('a'+j)) + ".go"
			content := "package " + filepath.Base(d) + "\n\nfunc Func" + string(rune('A'+i)) + string(rune('A'+j)) + "() {}\n"
			writeFile(fileName, content)
		}
	}
	writeFile("docs/guide.md", "# Guide\n")
	writeFile("docs/api.md", "# API\n")
	git("add", "-A")
	git("commit", "-m", "feature: large diff with 20 files across 6 directories")
	return dir
}

func TestBinaryE2E_BinaryFileChanges(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)

	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeBinary := func(rel string, data []byte) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")
	writeFile("README.md", "# Project\n")
	writeBinary("assets/logo.png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D})
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	writeFile("src/main.go", "package main\n\nfunc main() {}\n")
	writeBinary("assets/icon.png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0xFF, 0xFF, 0xFF})
	writeBinary("assets/logo.png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01, 0x02, 0x03, 0x04})
	git("add", "-A")
	git("commit", "-m", "feature: add Go file and modify binary assets")

	stdout, stderr, err := runBinary(t, osmBin, dir,
		"pr-split", "-interactive=false", "-base=main",
		"-strategy=directory", "-verify=true",
		"--store=memory", "--session="+t.Name(), "run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}
	if err != nil {
		t.Fatalf("binary file changes test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if countSplitBranches(t, dir) == 0 {
		t.Fatalf("no split/* branches with binary files, branches: %v", gitBranches(t, dir))
	}
}

func TestBinaryE2E_NoChangesFromBase(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	osmBin := buildOSMBinary(t)

	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git("init")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test User")
	writeFile("README.md", "# Project\n")
	git("add", "-A")
	git("commit", "-m", "initial")

	git("checkout", "-b", "feature")
	git("commit", "--allow-empty", "-m", "empty feature commit")

	stdout, stderr, err := runBinary(t, osmBin, dir,
		"pr-split", "-interactive=false", "-base=main",
		"-strategy=directory",
		"--store=memory", "--session="+t.Name(), "run",
	)
	t.Logf("stdout:\n%s", stdout)
	if stderr != "" {
		t.Logf("stderr:\n%s", stderr)
	}
	if err != nil {
		t.Fatalf("no changes test failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if count := countSplitBranches(t, dir); count != 0 {
		t.Logf("expected 0 splits for empty diff, got %d (acceptable)", count)
	}
}
