package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	gojarequire "github.com/dop251/goja_nodejs/require"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prSplitTestEnv sets up a JS environment with osm:bt and osm:exec for PR
// split tests. Returns the bridge and a JS runner function.
func prSplitTestEnv(t *testing.T) (*btmod.Bridge, func(string) goja.Value) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("PR split uses sh -c; skipping on Windows")
	}

	reg := gojarequire.NewRegistry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(true),
	)
	loop.Start()
	t.Cleanup(func() { loop.Stop() })

	ctx := context.Background()
	bridge := btmod.NewBridgeWithEventLoop(ctx, loop, reg)
	t.Cleanup(func() { bridge.Stop() })

	// Register exec module (bt is auto-registered by bridge).
	reg.RegisterNativeModule("osm:exec", execmod.Require(ctx))

	runJS := func(script string) goja.Value {
		t.Helper()
		var res goja.Value
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			var e error
			res, e = vm.RunString(script)
			return e
		})
		require.NoError(t, err, "JS execution failed")
		return res
	}

	return bridge, runJS
}

// prSplitScriptPath returns the absolute path to
// scripts/orchestrate-pr-split.js relative to this package.
func prSplitScriptPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	p := filepath.Join(wd, "..", "..", "..", "scripts", "orchestrate-pr-split.js")
	absP, err := filepath.Abs(p)
	require.NoError(t, err)
	_, err = os.Stat(absP)
	require.NoError(t, err, "orchestrate-pr-split.js not found at %s", absP)
	return absP
}

// initTestGitRepo creates a temporary git repo with an initial commit
// containing a few files. Returns the path to the repo directory.
func initTestGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")

	// Create initial file structure.
	for _, f := range []struct{ path, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"README.md", "# Test Project\n"},
	} {
		fullPath := filepath.Join(dir, f.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(f.content), 0o644))
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial commit")

	return dir
}

// addFeatureFiles creates a "feature" branch with new/modified files across
// several directories.
func addFeatureFiles(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	for _, f := range []struct{ path, content string }{
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"pkg/impl_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestBar(t *testing.T) {\n\tif Bar() != \"bar\" {\n\t\tt.Fatal()\n\t}\n}\n"},
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
		{"docs/api.md", "# API\n\nAPI reference.\n"},
	} {
		fullPath := filepath.Join(dir, f.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(f.content), 0o644))
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "feature work")
}

// runGit executes a git command in the given directory, failing on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}

// ---------------------------------------------------------------------------
//  Module loading
// ---------------------------------------------------------------------------

func TestPRSplit_ModuleLoads(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	runJS(`var prSplit = require('` + sp + `');`)
	val := runJS(`prSplit.VERSION`)
	assert.Equal(t, "1.0.0", val.String())
}

func TestPRSplit_ExportedFunctions(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	fns := []string{
		"analyzeDiff", "analyzeDiffStats",
		"groupByDirectory", "groupByExtension", "groupByPattern", "groupByChunks",
		"createSplitPlan", "validatePlan",
		"executeSplit",
		"verifySplit", "verifySplits", "verifyEquivalence", "cleanupBranches",
		"createAnalyzeNode", "createGroupNode", "createPlanNode",
		"createSplitNode", "createVerifyNode", "createEquivalenceNode",
		"createWorkflowTree",
	}
	for _, fn := range fns {
		val := runJS(`typeof prSplit.` + fn)
		assert.Equal(t, "function", val.String(), "%s should be a function", fn)
	}
}

// ---------------------------------------------------------------------------
//  Pure function tests (no git repo needed)
// ---------------------------------------------------------------------------

func TestPRSplit_GroupByDirectory(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.groupByDirectory(
		['pkg/a.go', 'pkg/b.go', 'cmd/main.go', 'docs/readme.md', 'Makefile'], 1
	))`)
	s := val.String()
	assert.Contains(t, s, `"pkg"`)
	assert.Contains(t, s, `"cmd"`)
	assert.Contains(t, s, `"docs"`)
	assert.Contains(t, s, `"."`) // Makefile has no directory → '.'
}

func TestPRSplit_GroupByDirectory_Depth2(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.groupByDirectory(
		['pkg/sub/a.go', 'pkg/sub/b.go', 'pkg/other/c.go'], 2
	))`)
	s := val.String()
	assert.Contains(t, s, `"pkg/sub"`)
	assert.Contains(t, s, `"pkg/other"`)
}

func TestPRSplit_GroupByExtension(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.groupByExtension(
		['main.go', 'test.go', 'style.css', 'README.md', 'Makefile']
	))`)
	s := val.String()
	assert.Contains(t, s, `".go"`)
	assert.Contains(t, s, `".css"`)
	assert.Contains(t, s, `".md"`)
	assert.Contains(t, s, `"(none)"`) // Makefile has no extension
}

func TestPRSplit_GroupByPattern(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.groupByPattern(
		['pkg/types.go', 'pkg/types_test.go', 'cmd/main.go', 'docs/readme.md'],
		{ tests: /_test\.go$/, docs: /^docs\//, code: /\.go$/ }
	))`)
	s := val.String()
	assert.Contains(t, s, `"tests"`)
	assert.Contains(t, s, `"docs"`)
	assert.Contains(t, s, `"code"`)
}

func TestPRSplit_GroupByChunks(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.groupByChunks(
		['a', 'b', 'c', 'd', 'e', 'f', 'g'], 3
	))`)
	s := val.String()
	assert.Contains(t, s, `"chunk-1"`)
	assert.Contains(t, s, `"chunk-2"`)
	assert.Contains(t, s, `"chunk-3"`)
}

func TestPRSplit_ValidatePlan_Valid(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.validatePlan({
		baseBranch: 'main',
		sourceBranch: 'feature',
		splits: [
			{ name: 'split-1', files: ['a.go', 'b.go'], message: 'first' },
			{ name: 'split-2', files: ['c.go'], message: 'second' }
		]
	}))`)
	assert.Contains(t, val.String(), `"valid":true`)
}

func TestPRSplit_ValidatePlan_NoSplits(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.validatePlan({ splits: [] }))`)
	assert.Contains(t, val.String(), `"valid":false`)
	assert.Contains(t, val.String(), `no splits`)
}

func TestPRSplit_ValidatePlan_DuplicateFiles(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.validatePlan({
		splits: [
			{ name: 's1', files: ['a.go'] },
			{ name: 's2', files: ['a.go'] }
		]
	}))`)
	assert.Contains(t, val.String(), `"valid":false`)
	assert.Contains(t, val.String(), `duplicate`)
}

func TestPRSplit_ValidatePlan_EmptySplit(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.validatePlan({
		splits: [
			{ name: 's1', files: [] }
		]
	}))`)
	assert.Contains(t, val.String(), `"valid":false`)
	assert.Contains(t, val.String(), `no files`)
}

func TestPRSplit_CreateSplitPlan(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.createSplitPlan(
		{ pkg: ['pkg/a.go', 'pkg/b.go'], docs: ['docs/readme.md'] },
		{ baseBranch: 'main', sourceBranch: 'feat', branchPrefix: 'pr/' }
	))`)
	s := val.String()
	// Should have two splits, sorted by group name: docs first, then pkg
	assert.Contains(t, s, `"pr/01-docs"`)
	assert.Contains(t, s, `"pr/02-pkg"`)
	assert.Contains(t, s, `"baseBranch":"main"`)
	assert.Contains(t, s, `"sourceBranch":"feat"`)
}

// ---------------------------------------------------------------------------
//  Git-dependent tests
// ---------------------------------------------------------------------------

func TestPRSplit_AnalyzeDiff(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	runJS(`var prSplit = require('` + sp + `');`)

	// Escape backslashes for Windows paths (though test is skipped on Windows).
	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)

	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)

	// Error should be null.
	errVal := runJS(`analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "error should be null, got: %v", errVal)

	// Current branch should be feature.
	branchVal := runJS(`analysis.currentBranch`)
	assert.Equal(t, "feature", branchVal.String())

	// Should find 5 changed files.
	lenVal := runJS(`analysis.files.length`)
	assert.Equal(t, int64(5), lenVal.ToInteger())

	// Spot-check specific files.
	filesVal := runJS(`JSON.stringify(analysis.files.sort())`)
	assert.Contains(t, filesVal.String(), "pkg/impl.go")
	assert.Contains(t, filesVal.String(), "docs/guide.md")
	assert.Contains(t, filesVal.String(), "cmd/run.go")
}

func TestPRSplit_AnalyzeDiffStats(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var stats = prSplit.analyzeDiffStats({baseBranch: 'main', dir: '` + escapedDir + `'});`)

	errVal := runJS(`stats.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal))

	lenVal := runJS(`stats.files.length`)
	assert.Equal(t, int64(5), lenVal.ToInteger())

	// Each file should have additions > 0.
	addVal := runJS(`stats.files[0].additions`)
	assert.Greater(t, addVal.ToInteger(), int64(0))
}

func TestPRSplit_ExecuteSplit(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Create plan from analysis.
	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)
	runJS(`var groups = prSplit.groupByDirectory(analysis.files, 1);`)
	runJS(`var plan = prSplit.createSplitPlan(groups, {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		branchPrefix: 'split/',
		verifyCommand: 'true'
	});`)

	// Validate plan.
	valResult := runJS(`JSON.stringify(prSplit.validatePlan(plan))`)
	assert.Contains(t, valResult.String(), `"valid":true`)

	// Execute split.
	runJS(`var result = prSplit.executeSplit(plan);`)

	// No error.
	errVal := runJS(`result.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "execute error: %v", errVal)

	// All splits should have SHAs.
	splitCount := runJS(`result.results.length`)
	assert.Equal(t, int64(3), splitCount.ToInteger()) // cmd, docs, pkg

	// Verify each result has a non-empty SHA.
	for i := 0; i < 3; i++ {
		shaVal := runJS(fmt.Sprintf(`result.results[%d].sha`, i))
		assert.NotEmpty(t, shaVal.String(), "split %d should have a SHA", i)
	}

	// Verify branches were created.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/01-cmd")
	assert.Contains(t, branches, "split/02-docs")
	assert.Contains(t, branches, "split/03-pkg")

	// Current branch should be restored to feature.
	currentBranch := strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, "feature", currentBranch)
}

func TestPRSplit_VerifyEquivalence(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Analyze, group, plan, execute.
	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)
	runJS(`var groups = prSplit.groupByDirectory(analysis.files, 1);`)
	runJS(`var plan = prSplit.createSplitPlan(groups, {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		branchPrefix: 'split/'
	});`)
	runJS(`prSplit.executeSplit(plan);`)

	// Verify equivalence.
	runJS(`var equiv = prSplit.verifyEquivalence(plan);`)

	equivVal := runJS(`equiv.equivalent`)
	assert.Equal(t, true, equivVal.ToBoolean(), "tree hashes should match")

	errVal := runJS(`equiv.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal))
}

func TestPRSplit_VerifySplits(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)
	runJS(`var groups = prSplit.groupByDirectory(analysis.files, 1);`)
	runJS(`var plan = prSplit.createSplitPlan(groups, {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		branchPrefix: 'split/',
		verifyCommand: 'true'
	});`)
	runJS(`prSplit.executeSplit(plan);`)

	// Verify all splits (with 'true' command, should all pass).
	runJS(`var verify = prSplit.verifySplits(plan);`)
	allPassed := runJS(`verify.allPassed`)
	assert.Equal(t, true, allPassed.ToBoolean())

	verifyLen := runJS(`verify.results.length`)
	assert.Equal(t, int64(3), verifyLen.ToInteger())

	// Restore to feature after verifySplits.
	currentBranch := strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, "feature", currentBranch)
}

func TestPRSplit_CleanupBranches(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)
	runJS(`var groups = prSplit.groupByDirectory(analysis.files, 1);`)
	runJS(`var plan = prSplit.createSplitPlan(groups, {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		branchPrefix: 'split/'
	});`)
	runJS(`prSplit.executeSplit(plan);`)

	// Verify branches exist before cleanup.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/01-cmd")

	// Cleanup.
	runJS(`var cleanup = prSplit.cleanupBranches(plan);`)
	deletedLen := runJS(`cleanup.deleted.length`)
	assert.Equal(t, int64(3), deletedLen.ToInteger())

	errLen := runJS(`cleanup.errors.length`)
	assert.Equal(t, int64(0), errLen.ToInteger())

	// Verify branches are gone.
	branchesAfter := runGit(t, dir, "branch")
	assert.NotContains(t, branchesAfter, "split/01-cmd")
	assert.NotContains(t, branchesAfter, "split/02-docs")
	assert.NotContains(t, branchesAfter, "split/03-pkg")
}

// ---------------------------------------------------------------------------
//  BT integration tests
// ---------------------------------------------------------------------------

func TestPRSplit_CreateWorkflowTree(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Create a workflow tree (doesn't execute, just builds the BT node).
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var tree = prSplit.createWorkflowTree(bb, {baseBranch: 'main'});`)

	// Verify tree is a valid BT node (returned as callable function).
	treeType := runJS(`typeof tree`)
	assert.Equal(t, "function", treeType.String())
}

func TestPRSplit_BTWorkflow_EndToEnd(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Build BT workflow tree.
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var tree = prSplit.createWorkflowTree(bb, {
		baseBranch: 'main',
		dir: '` + escapedDir + `',
		groupStrategy: 'directory',
		branchPrefix: 'bt-split/',
		verifyCommand: 'true'
	});`)

	// Tick the tree — should succeed (all steps complete).
	statusVal := runJS(`bt.tick(tree)`)
	assert.Equal(t, "success", statusVal.String())

	// Verify equivalence was stored on blackboard.
	equivVal := runJS(`bb.get('equivalence').equivalent`)
	assert.Equal(t, true, equivVal.ToBoolean())

	// Verify branches were created.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "bt-split/01-cmd")
	assert.Contains(t, branches, "bt-split/02-docs")
	assert.Contains(t, branches, "bt-split/03-pkg")
}

func TestPRSplit_AnalyzeDiff_NoChanges(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	// Don't add feature files — no changes from main.

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)
	filesLen := runJS(`analysis.files.length`)
	assert.Equal(t, int64(0), filesLen.ToInteger())
}

func TestPRSplit_ExecuteSplit_InvalidPlan(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	runJS(`var result = prSplit.executeSplit({ splits: [] });`)
	errVal := runJS(`result.error`)
	assert.Contains(t, errVal.String(), "invalid plan")
}
