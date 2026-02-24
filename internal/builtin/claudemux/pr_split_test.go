package claudemux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
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
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	reg.Enable(vm)
	gojanodejsconsole.Enable(vm)
	adapter, err := gojaeventloop.New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	ctx := context.Background()
	bridge := btmod.NewBridgeWithEventLoop(ctx, loop, vm, reg)
	t.Cleanup(func() { bridge.Stop() })

	// Register exec module (bt is auto-registered by bridge).
	reg.RegisterNativeModule("osm:exec", execmod.Require(ctx))

	// Register claudemux module for strategy selection.
	reg.RegisterNativeModule("osm:claudemux", Require(ctx))

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
// internal/command/pr_split_script.js relative to this package.
func prSplitScriptPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	p := filepath.Join(wd, "..", "..", "command", "pr_split_script.js")
	absP, err := filepath.Abs(p)
	require.NoError(t, err)
	_, err = os.Stat(absP)
	require.NoError(t, err, "pr_split_script.js not found at %s", absP)
	return absP
}

// initTestGitRepo creates a temporary git repo with an initial commit
// containing a few files. Returns the path to the repo directory.
func initTestGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
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
	assert.Equal(t, "5.0.0", val.String())
}

func TestPRSplit_ExportedFunctions(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	fns := []string{
		"analyzeDiff", "analyzeDiffStats",
		"groupByDirectory", "groupByExtension", "groupByPattern", "groupByChunks",
		"selectStrategy",
		"classifyChangesWithClaudeMux", "suggestSplitPlanWithClaudeMux",
		"createSplitPlan", "validatePlan",
		"executeSplit",
		"verifySplit", "verifySplits", "verifyEquivalence", "verifyEquivalenceDetailed",
		"cleanupBranches",
		"createAnalyzeNode", "createGroupNode", "createPlanNode",
		"createSplitNode", "createVerifyNode", "createEquivalenceNode",
		"createSelectStrategyNode",
		"createClaudeMuxClassifyNode", "createClaudeMuxPlanNode",
		"createClaudeMuxWorkflowTree",
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
		verifyCommand: 'true',
		fileStatuses: analysis.fileStatuses
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
		branchPrefix: 'split/',
		fileStatuses: analysis.fileStatuses
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
		verifyCommand: 'true',
		fileStatuses: analysis.fileStatuses
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
		branchPrefix: 'split/',
		fileStatuses: analysis.fileStatuses
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
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	// E2E test runs many git commands; increase timeout to avoid flakes under load.
	bridge.SetTimeout(30 * time.Second)

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

	errVal := runJS(`analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "error should be null for no-changes case")

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

// ---------------------------------------------------------------------------
//  Strategy selection tests (claudemux integration)
// ---------------------------------------------------------------------------

func TestPRSplit_SelectStrategy(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.selectStrategy(
		['pkg/a.go', 'pkg/b.go', 'cmd/main.go', 'docs/readme.md', 'Makefile']
	))`)
	s := val.String()
	assert.Contains(t, s, `"strategy"`)
	assert.Contains(t, s, `"reason"`)
	assert.Contains(t, s, `"groups"`)

	// Strategy should be one of the known values.
	stratVal := runJS(`prSplit.selectStrategy(
		['pkg/a.go', 'pkg/b.go', 'cmd/main.go', 'docs/readme.md', 'Makefile']
	).strategy`)
	known := []string{"directory", "directory-deep", "extension", "chunks", "dependency"}
	assert.Contains(t, known, stratVal.String())
}

func TestPRSplit_SelectStrategy_Scored(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	// With many files across multiple directories, scored should have entries.
	runJS(`var result = prSplit.selectStrategy([
		'pkg/a.go', 'pkg/b.go', 'pkg/c.go',
		'cmd/main.go', 'cmd/run.go',
		'docs/readme.md', 'docs/guide.md',
		'internal/foo.go', 'internal/bar.go',
		'tests/test_a.go'
	]);`)

	scoredLen := runJS(`result.scored.length`)
	assert.Equal(t, int64(5), scoredLen.ToInteger(), "should score 5 strategies")
}

// ---------------------------------------------------------------------------
//  Enhanced equivalence verification tests
// ---------------------------------------------------------------------------

func TestPRSplit_VerifyEquivalenceDetailed_Equivalent(t *testing.T) {
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
		fileStatuses: analysis.fileStatuses
	});`)
	runJS(`prSplit.executeSplit(plan);`)

	runJS(`var equiv = prSplit.verifyEquivalenceDetailed(plan);`)

	equivVal := runJS(`equiv.equivalent`)
	assert.Equal(t, true, equivVal.ToBoolean())

	// When equivalent, diffFiles should be empty.
	diffLen := runJS(`equiv.diffFiles.length`)
	assert.Equal(t, int64(0), diffLen.ToInteger())

	diffSummary := runJS(`equiv.diffSummary`)
	assert.Equal(t, "", diffSummary.String())
}

// ---------------------------------------------------------------------------
//  Conflict handling tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
//  T209: End-to-end PR split with real compilation verification
// ---------------------------------------------------------------------------

// initCompilableGitRepo creates a temporary git repo with go.mod and
// compilable Go source files. Each split branch from this repo can be
// verified with "go build ./..." independently.
func initCompilableGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")

	for _, f := range []struct{ path, content string }{
		{"go.mod", "module example.com/test-project\n\ngo 1.21\n"},
		{"pkg/types.go", "package pkg\n\n// Foo is a basic type.\ntype Foo struct {\n\tName string\n}\n"},
		{"cmd/app/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n"},
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

// addCompilableFeatureFiles creates a "feature" branch with new Go source
// files across several directories. Each file uses only stdlib imports so
// that "go build ./..." succeeds without network access.
func addCompilableFeatureFiles(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	for _, f := range []struct{ path, content string }{
		{"pkg/bar.go", "package pkg\n\n// Bar returns a greeting string.\nfunc Bar() string { return \"bar\" }\n"},
		{"cmd/app/run.go", "package main\n\n// run executes the application logic.\nfunc run() error { return nil }\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
	} {
		fullPath := filepath.Join(dir, f.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(f.content), 0o644))
	}

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "add feature implementation and docs")
}

// TestPRSplit_EndToEnd_WithCompilation verifies the FULL PR split workflow
// end-to-end with real compilation verification on each stacked branch.
//
// This is T209: the most critical integration test proving that the PR
// split script is a useful command line application that actually works.
//
// Workflow: analyze → group → plan → execute → verify (go build) → equivalence
func TestPRSplit_EndToEnd_WithCompilation(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)

	// E2E test with real compilation; increase timeout to avoid flakes.
	bridge.SetTimeout(60 * time.Second)

	sp := prSplitScriptPath(t)

	dir := initCompilableGitRepo(t)
	addCompilableFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// 1. Analyze what changed.
	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)

	errVal := runJS(`analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "analysis error: %v", errVal)
	assert.Equal(t, "feature", runJS(`analysis.currentBranch`).String())
	assert.Equal(t, int64(3), runJS(`analysis.files.length`).ToInteger(),
		"expected 3 changed files: pkg/bar.go, cmd/app/run.go, docs/guide.md")

	// 2. Group by directory (depth=1).
	runJS(`var groups = prSplit.groupByDirectory(analysis.files, 1);`)
	groupKeys := runJS(`Object.keys(groups).sort().join(',')`)
	assert.Equal(t, "cmd,docs,pkg", groupKeys.String())

	// 3. Create split plan with "go build ./..." as verification command.
	runJS(`var plan = prSplit.createSplitPlan(groups, {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		branchPrefix: 'split/',
		verifyCommand: 'go build ./...',
		fileStatuses: analysis.fileStatuses
	});`)

	// Validate plan.
	valResult := runJS(`JSON.stringify(prSplit.validatePlan(plan))`)
	assert.Contains(t, valResult.String(), `"valid":true`)

	assert.Equal(t, int64(3), runJS(`plan.splits.length`).ToInteger())
	t.Logf("Split plan: %d splits", 3)
	for i := 0; i < 3; i++ {
		name := runJS(fmt.Sprintf(`plan.splits[%d].name`, i)).String()
		filesLen := runJS(fmt.Sprintf(`plan.splits[%d].files.length`, i)).ToInteger()
		t.Logf("  %s (%d files)", name, filesLen)
	}

	// 4. Execute the split — creates stacked branches.
	runJS(`var execResult = prSplit.executeSplit(plan);`)
	execErr := runJS(`execResult.error`)
	assert.True(t, goja.IsNull(execErr) || goja.IsUndefined(execErr),
		"execute error: %v", execErr)

	splitCount := runJS(`execResult.results.length`).ToInteger()
	assert.Equal(t, int64(3), splitCount)
	for i := int64(0); i < splitCount; i++ {
		sha := runJS(fmt.Sprintf(`execResult.results[%d].sha`, i)).String()
		name := runJS(fmt.Sprintf(`execResult.results[%d].name`, i)).String()
		assert.NotEmpty(t, sha, "split %s should have a SHA", name)
		t.Logf("  Created: %s (sha=%s)", name, sha[:8])
	}

	// Verify branches were created in git.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/01-cmd")
	assert.Contains(t, branches, "split/02-docs")
	assert.Contains(t, branches, "split/03-pkg")

	// 5. Verify each split compiles with "go build ./..."
	runJS(`var verify = prSplit.verifySplits(plan);`)
	allPassed := runJS(`verify.allPassed`).ToBoolean()

	verifyLen := runJS(`verify.results.length`).ToInteger()
	for i := int64(0); i < verifyLen; i++ {
		name := runJS(fmt.Sprintf(`verify.results[%d].name`, i)).String()
		passed := runJS(fmt.Sprintf(`verify.results[%d].passed`, i)).ToBoolean()
		t.Logf("  Verify: %s compiled=%v", name, passed)
		if !passed {
			errStr := runJS(fmt.Sprintf(`verify.results[%d].error`, i)).String()
			t.Logf("    Error: %s", errStr)
		}
		assert.True(t, passed, "split %s should compile with 'go build ./...'", name)
	}
	assert.True(t, allPassed, "all split branches must compile independently")

	// 6. Verify tree equivalence — final split tree must match source.
	runJS(`var equiv = prSplit.verifyEquivalence(plan);`)
	equivalent := runJS(`equiv.equivalent`).ToBoolean()
	assert.True(t, equivalent, "final split tree hash must equal source branch tree hash")
	if !equivalent {
		splitTree := runJS(`equiv.splitTree`).String()
		sourceTree := runJS(`equiv.sourceTree`).String()
		t.Fatalf("Tree mismatch: split=%s source=%s", splitTree, sourceTree)
	}

	// 7. Verify current branch was restored.
	currentBranch := strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, "feature", currentBranch)

	t.Log("T209 PASS: Full PR split workflow with real compilation verification")
}

// TestPRSplit_EndToEnd_BTWorkflow_WithCompilation runs the BT-based workflow
// with real go build verification, proving the behavior tree integration
// works end-to-end with compilable projects.
func TestPRSplit_EndToEnd_BTWorkflow_WithCompilation(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)

	bridge.SetTimeout(60 * time.Second)

	sp := prSplitScriptPath(t)

	dir := initCompilableGitRepo(t)
	addCompilableFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Build BT workflow tree with real compilation verification.
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var tree = prSplit.createWorkflowTree(bb, {
		baseBranch: 'main',
		dir: '` + escapedDir + `',
		groupStrategy: 'directory',
		branchPrefix: 'bt-compile/',
		verifyCommand: 'go build ./...'
	});`)

	// Tick the tree — should succeed (all steps complete).
	statusVal := runJS(`bt.tick(tree)`)
	assert.Equal(t, "success", statusVal.String(), "BT workflow should succeed")

	// Verify equivalence was stored on blackboard.
	equivVal := runJS(`bb.get('equivalence').equivalent`)
	assert.True(t, equivVal.ToBoolean(), "BT workflow tree equivalence should hold")

	// Verify branches were created.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "bt-compile/01-cmd")
	assert.Contains(t, branches, "bt-compile/02-docs")
	assert.Contains(t, branches, "bt-compile/03-pkg")

	t.Log("T209 BT PASS: BT workflow with real compilation verification")
}

func TestPRSplit_ExecuteSplit_MissingFile(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Create a plan with a non-existent file but valid fileStatuses entry.
	runJS(`var plan = {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		verifyCommand: 'true',
		fileStatuses: { 'does-not-exist.go': 'A' },
		splits: [{
			name: 'split/01-bad',
			files: ['does-not-exist.go'],
			message: 'missing file'
		}]
	};`)
	runJS(`var result = prSplit.executeSplit(plan);`)

	errVal := runJS(`result.error`)
	assert.Contains(t, errVal.String(), "checkout file")
	assert.Contains(t, errVal.String(), "does-not-exist.go")
}

// ---------------------------------------------------------------------------
//  Deleted files + re-run tests
// ---------------------------------------------------------------------------

// addFeatureFilesWithDeletions creates a feature branch that adds new files
// AND deletes an existing file from the initial commit.
func addFeatureFilesWithDeletions(t *testing.T, dir string) {
	t.Helper()

	runGit(t, dir, "checkout", "-b", "feature")

	// Add new files.
	for _, f := range []struct{ path, content string }{
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
	} {
		fullPath := filepath.Join(dir, f.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(f.content), 0o644))
	}

	// Delete an existing file (README.md was in initial commit).
	require.NoError(t, os.Remove(filepath.Join(dir, "README.md")))

	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "feature: add impl, docs; delete README")
}

func TestPRSplit_AnalyzeDiff_FileStatuses(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFilesWithDeletions(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)

	// Error should be null.
	errVal := runJS(`analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal))

	// Should find 3 files: 2 added + 1 deleted.
	lenVal := runJS(`analysis.files.length`)
	assert.Equal(t, int64(3), lenVal.ToInteger())

	// Verify fileStatuses is populated correctly.
	implStatus := runJS(`analysis.fileStatuses['pkg/impl.go']`)
	assert.Equal(t, "A", implStatus.String())

	docsStatus := runJS(`analysis.fileStatuses['docs/guide.md']`)
	assert.Equal(t, "A", docsStatus.String())

	readmeStatus := runJS(`analysis.fileStatuses['README.md']`)
	assert.Equal(t, "D", readmeStatus.String())
}

func TestPRSplit_ExecuteSplit_WithDeletedFiles(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFilesWithDeletions(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Full pipeline: analyze → group → plan → execute.
	runJS(`var analysis = prSplit.analyzeDiff({baseBranch: 'main', dir: '` + escapedDir + `'});`)
	runJS(`var groups = prSplit.groupByDirectory(analysis.files, 1);`)
	runJS(`var plan = prSplit.createSplitPlan(groups, {
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		branchPrefix: 'split/',
		verifyCommand: 'true',
		fileStatuses: analysis.fileStatuses
	});`)

	// Execute split — should handle deleted README.md correctly.
	runJS(`var result = prSplit.executeSplit(plan);`)

	errVal := runJS(`result.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal),
		"executeSplit should succeed with deleted files, got: %v", errVal)

	// Verify equivalence — tree hashes must match.
	runJS(`var equiv = prSplit.verifyEquivalence(plan);`)
	equivVal := runJS(`equiv.equivalent`)
	assert.True(t, equivVal.ToBoolean(), "tree hashes should match when deletions are handled correctly")

	// The branch containing the deletion (README.md is in '.') should exist.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/")

	// Verify README.md is actually gone on the last split branch.
	lastSplit := runJS(`plan.splits[plan.splits.length-1].name`).String()
	runGit(t, dir, "checkout", lastSplit)
	_, err := os.Stat(filepath.Join(dir, "README.md"))
	assert.True(t, os.IsNotExist(err), "README.md should not exist on the last split branch")

	// Restore to feature.
	runGit(t, dir, "checkout", "feature")
}

func TestPRSplit_ExecuteSplit_RerunDeletesBranches(t *testing.T) {
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
		fileStatuses: analysis.fileStatuses
	});`)

	// First run — creates branches.
	runJS(`var result1 = prSplit.executeSplit(plan);`)
	err1 := runJS(`result1.error`)
	assert.True(t, goja.IsNull(err1) || goja.IsUndefined(err1), "first run should succeed")

	branches1 := runGit(t, dir, "branch")
	assert.Contains(t, branches1, "split/01-cmd")

	// Second run — same plan, branches already exist. Should NOT fail.
	runJS(`var result2 = prSplit.executeSplit(plan);`)
	err2 := runJS(`result2.error`)
	assert.True(t, goja.IsNull(err2) || goja.IsUndefined(err2),
		"re-run should succeed (pre-existing branches deleted), got: %v", err2)

	// Verify equivalence still holds after re-run.
	runJS(`var equiv = prSplit.verifyEquivalence(plan);`)
	equivVal := runJS(`equiv.equivalent`)
	assert.True(t, equivVal.ToBoolean(), "tree hashes should match after re-run")
}

func TestPRSplit_ExecuteSplit_NoFileStatuses(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	// Plan with valid structure but missing fileStatuses.
	runJS(`var result = prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		splits: [{
			name: 'split/01-test',
			files: ['a.go'],
			message: 'test'
		}]
	});`)

	errVal := runJS(`result.error`)
	assert.Contains(t, errVal.String(), "fileStatuses is required")
}

func TestPRSplit_ExecuteSplit_MissingFileStatus(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Plan with fileStatuses that's missing an entry for one file.
	runJS(`var result = prSplit.executeSplit({
		baseBranch: 'main',
		sourceBranch: 'feature',
		dir: '` + escapedDir + `',
		fileStatuses: { 'pkg/impl.go': 'A' },
		splits: [{
			name: 'split/01-test',
			files: ['pkg/impl.go', 'cmd/run.go'],
			message: 'test'
		}]
	});`)

	errVal := runJS(`result.error`)
	assert.Contains(t, errVal.String(), "cmd/run.go")
	assert.Contains(t, errVal.String(), "no entry in plan.fileStatuses")
}

// ---------------------------------------------------------------------------
//  ClaudeMux AI function error-path tests (CI-safe, no real agent)
// ---------------------------------------------------------------------------

func TestPRSplit_ClassifyChangesWithClaudeMux_NoRegistry(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	// Call without registry — should return error, not throw.
	val := runJS(`JSON.stringify(prSplit.classifyChangesWithClaudeMux(
		['a.go', 'b.go'],
		{}
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `registry is required`)
}

func TestPRSplit_ClassifyChangesWithClaudeMux_EmptyFiles(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var cm = require('osm:claudemux');`)
	runJS(`var reg = cm.newRegistry();`)

	val := runJS(`JSON.stringify(prSplit.classifyChangesWithClaudeMux(
		[],
		{ registry: reg }
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `no files to classify`)
}

func TestPRSplit_ClassifyChangesWithClaudeMux_NullFiles(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var cm = require('osm:claudemux');`)
	runJS(`var reg = cm.newRegistry();`)

	val := runJS(`JSON.stringify(prSplit.classifyChangesWithClaudeMux(
		null,
		{ registry: reg }
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `no files`)
}

func TestPRSplit_ClassifyChangesWithClaudeMux_NoOptions(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.classifyChangesWithClaudeMux(
		['a.go'], null
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `registry is required`)
}

func TestPRSplit_ClassifyChangesWithClaudeMux_SpawnFailure(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var cm = require('osm:claudemux');`)

	// Registry with no providers registered — spawn should fail.
	runJS(`var reg = cm.newRegistry();`)

	val := runJS(`JSON.stringify(prSplit.classifyChangesWithClaudeMux(
		['a.go', 'b.go'],
		{ registry: reg, providerName: 'nonexistent-provider' }
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `spawn failed`)
}

func TestPRSplit_SuggestSplitPlanWithClaudeMux_NoRegistry(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	val := runJS(`JSON.stringify(prSplit.suggestSplitPlanWithClaudeMux(
		['a.go'], {}, {}
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `registry is required`)
}

func TestPRSplit_SuggestSplitPlanWithClaudeMux_EmptyFiles(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var cm = require('osm:claudemux');`)
	runJS(`var reg = cm.newRegistry();`)

	val := runJS(`JSON.stringify(prSplit.suggestSplitPlanWithClaudeMux(
		[], {}, { registry: reg }
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `no files to plan`)
}

func TestPRSplit_SuggestSplitPlanWithClaudeMux_SpawnFailure(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var cm = require('osm:claudemux');`)
	runJS(`var reg = cm.newRegistry();`)

	val := runJS(`JSON.stringify(prSplit.suggestSplitPlanWithClaudeMux(
		['a.go', 'b.go'], {'a.go': 'impl', 'b.go': 'test'},
		{ registry: reg, providerName: 'does-not-exist' }
	))`)
	s := val.String()
	assert.Contains(t, s, `"error"`)
	assert.Contains(t, s, `spawn failed`)
}

// ---------------------------------------------------------------------------
//  ClaudeMux BT node error-path tests (CI-safe, no real agent)
// ---------------------------------------------------------------------------

func TestPRSplit_ClaudeMuxClassifyNode_NoAnalysis(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Blackboard has no analysisResult — node should fail.
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var node = prSplit.createClaudeMuxClassifyNode(bb, {});`)

	statusVal := runJS(`bt.tick(node)`)
	assert.Equal(t, "failure", statusVal.String())

	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "no analysis result")
}

func TestPRSplit_ClaudeMuxClassifyNode_NoRegistry(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Blackboard has analysis but no registry.
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`bb.set('analysisResult', { files: ['a.go', 'b.go'], currentBranch: 'feature' });`)
	runJS(`var node = prSplit.createClaudeMuxClassifyNode(bb, {});`)

	statusVal := runJS(`bt.tick(node)`)
	assert.Equal(t, "failure", statusVal.String())

	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "no claudemux registry")
}

func TestPRSplit_ClaudeMuxClassifyNode_SpawnFailure(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)
	runJS(`var cm = require('osm:claudemux');`)

	// Registry with no providers — spawn will fail.
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`bb.set('analysisResult', { files: ['a.go'], currentBranch: 'feature' });`)
	runJS(`bb.set('claudemuxRegistry', cm.newRegistry());`)
	runJS(`var node = prSplit.createClaudeMuxClassifyNode(bb, { providerName: 'ghost' });`)

	statusVal := runJS(`bt.tick(node)`)
	assert.Equal(t, "failure", statusVal.String())

	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "classification")
}

func TestPRSplit_ClaudeMuxPlanNode_NoAnalysis(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var node = prSplit.createClaudeMuxPlanNode(bb, {});`)

	statusVal := runJS(`bt.tick(node)`)
	assert.Equal(t, "failure", statusVal.String())

	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "no analysis result")
}

func TestPRSplit_ClaudeMuxPlanNode_NoRegistry(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	runJS(`var bb = new bt.Blackboard();`)
	runJS(`bb.set('analysisResult', { files: ['a.go'], currentBranch: 'feature' });`)
	runJS(`var node = prSplit.createClaudeMuxPlanNode(bb, {});`)

	statusVal := runJS(`bt.tick(node)`)
	assert.Equal(t, "failure", statusVal.String())

	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "no claudemux registry")
}

func TestPRSplit_ClaudeMuxPlanNode_SpawnFailure(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)
	runJS(`var cm = require('osm:claudemux');`)

	runJS(`var bb = new bt.Blackboard();`)
	runJS(`bb.set('analysisResult', { files: ['a.go'], currentBranch: 'feature' });`)
	runJS(`bb.set('claudemuxRegistry', cm.newRegistry());`)
	runJS(`var node = prSplit.createClaudeMuxPlanNode(bb, { providerName: 'ghost' });`)

	statusVal := runJS(`bt.tick(node)`)
	assert.Equal(t, "failure", statusVal.String())

	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "planning")
}

func TestPRSplit_ClaudeMuxWorkflowTree_Builds(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Verify the workflow tree can be constructed (structural test).
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var tree = prSplit.createClaudeMuxWorkflowTree(bb, {
		baseBranch: 'main',
		dir: '/tmp/test',
		branchPrefix: 'ai-split/',
		verifyCommand: 'true'
	});`)

	treeType := runJS(`typeof tree`)
	assert.Equal(t, "function", treeType.String())
}

func TestPRSplit_ClaudeMuxWorkflowTree_FailsWithoutRegistry(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	bridge.SetTimeout(10 * time.Second)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)
	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)

	runJS(`var prSplit = require('` + sp + `');`)
	runJS(`var bt = require('osm:bt');`)

	// Build tree with real repo but no registry — should fail at classify step.
	runJS(`var bb = new bt.Blackboard();`)
	runJS(`var tree = prSplit.createClaudeMuxWorkflowTree(bb, {
		baseBranch: 'main',
		dir: '` + escapedDir + `'
	});`)

	statusVal := runJS(`bt.tick(tree)`)
	assert.Equal(t, "failure", statusVal.String())

	// lastError should mention missing registry.
	errVal := runJS(`bb.get('lastError')`)
	assert.Contains(t, errVal.String(), "registry")
}
