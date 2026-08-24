package aimux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojanodejsconsole "github.com/joeycumines/goja_nodejs/console"
	gojarequire "github.com/joeycumines/goja_nodejs/require"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	osmod "github.com/joeycumines/one-shot-man/internal/builtin/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prSplitTestEnv sets up a JS environment with osm:bt and osm:exec for PR
// split tests. Returns the bridge and a JS runner function.
func prSplitTestEnv(t *testing.T) (*btmod.Bridge, func(string) goja.Value) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping PR-split test in -short mode")
	}

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
	bridge := btmod.NewBridge(ctx, loop, vm, reg, nil)
	t.Cleanup(func() { bridge.Stop() })

	// Register exec module (bt is auto-registered by bridge).
	reg.RegisterNativeModule("osm:exec", execmod.Require(ctx, adapter))

	// Register os module so JS code can use os.tmpdir() for worktrees
	// instead of falling back to /tmp (which may contain a stale .git).
	reg.RegisterNativeModule("osm:os", osmod.Require(ctx, adapter, nil))

	// Register aimux module for strategy selection.
	reg.RegisterNativeModule("osm:aimux", Require(ctx, adapter))

	runJS := func(script string) goja.Value {
		t.Helper()
		var res goja.Value
		err := bridge.RunSync(func(vm *goja.Runtime) error {
			var e error
			res, e = vm.RunString(script)
			return e
		})
		require.NoError(t, err, "JS execution failed")
		return res
	}

	return bridge, runJS
}

// runAsyncJS executes a JavaScript script that uses async/await on the event
// loop. The script is wrapped in an async IIFE and must call __signalDone()
// when complete. This is necessary because vm.RunString cannot await Promises
// directly — the event loop must process microtasks after RunString returns.
//
// Results that need to be read by subsequent runJS calls must be stored on
// globalThis (e.g. globalThis.__result = await prSplit.executeSplit(...);).
func runAsyncJS(t *testing.T, bridge *btmod.Bridge, script string) {
	t.Helper()
	done := make(chan error, 1)

	ok := bridge.Run(func(vm *goja.Runtime) {
		_ = vm.Set("__signalDone", func(call goja.FunctionCall) goja.Value {
			if arg := call.Argument(0); !goja.IsUndefined(arg) && !goja.IsNull(arg) {
				done <- fmt.Errorf("%s", arg.String())
			} else {
				done <- nil
			}
			return goja.Undefined()
		})
		wrapped := "(async () => {\n" + script + "\n})().catch(function(e) { __signalDone(String(e)); });"
		_, err := vm.RunString(wrapped)
		if err != nil {
			done <- err
		}
	})
	require.True(t, ok, "failed to submit async script to event loop")

	select {
	case err := <-done:
		require.NoError(t, err, "async script error")
	case <-time.After(120 * time.Second):
		t.Fatalf("timeout waiting for async script")
	}
}

// prSplitScriptPath returns the path to a temporary JS file that
// concatenates all pr-split chunk files with a module.exports tail, allowing
// require(path) to work identically to the former monolith.
func prSplitScriptPath(t *testing.T) string {
	t.Helper()
	return combinedChunkScript(t)
}

// prSplitChunkFilesFromManifest reads the manifest and returns chunk file
// names up to and including "13_tui", matching the previous hardcoded list.
func prSplitChunkFilesFromManifest(t *testing.T) []string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(wd, "..", "..", "command")
	absDir, err := filepath.Abs(dir)
	require.NoError(t, err)

	manifestPath := filepath.Join(absDir, "pr_split_manifest.json")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var m struct {
		Chunks []struct {
			ID   string `json:"id"`
			File string `json:"file"`
		} `json:"chunks"`
	}
	require.NoError(t, json.Unmarshal(data, &m))

	var files []string
	for _, chunk := range m.Chunks {
		files = append(files, chunk.File)
		if chunk.ID == "13_tui" {
			break
		}
	}
	return files
}

// combinedChunkScript concatenates all pr-split chunk files from
// internal/command/ into a temporary JS file with module.exports =
// globalThis.prSplit appended. The returned path is suitable for require().
func combinedChunkScript(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(wd, "..", "..", "command")
	absDir, err := filepath.Abs(dir)
	require.NoError(t, err)

	chunkFiles := prSplitChunkFilesFromManifest(t)

	var buf strings.Builder
	for _, name := range chunkFiles {
		content, err := os.ReadFile(filepath.Join(absDir, name))
		require.NoError(t, err, "failed to read chunk %s at %s", name, absDir)
		buf.Write(content)
		buf.WriteByte('\n')
	}
	buf.WriteString("module.exports = globalThis.prSplit;\n")

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "pr_split_combined.js")
	err = os.WriteFile(tmpFile, []byte(buf.String()), 0644)
	require.NoError(t, err)
	return tmpFile
}

// initTestGitRepo creates a temporary git repo with an initial commit
// containing a few files. Returns the path to the repo directory.
func initTestGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
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

func TestPRSplit_ModuleLoads(t *testing.T) {
	t.Parallel()
	_, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	runJS(`var prSplit = require('` + sp + `');`)
	val := runJS(`prSplit.VERSION`)
	assert.Equal(t, "6.0.0", val.String())
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
		"createSplitPlan", "validatePlan",
		"executeSplit",
		"verifySplit", "verifySplits", "verifyEquivalence", "verifyEquivalenceDetailed",
		"cleanupBranches",
	}
	for _, fn := range fns {
		val := runJS(`typeof prSplit.` + fn)
		assert.Equal(t, "function", val.String(), "%s should be a function", fn)
	}
}

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
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		globalThis.__planResult = await prSplit.createSplitPlan(
			{ pkg: ['pkg/a.go', 'pkg/b.go'], docs: ['docs/readme.md'] },
			{ baseBranch: 'main', sourceBranch: 'feat', branchPrefix: 'pr/' }
		);
		__signalDone();
	`)
	val := runJS(`JSON.stringify(__planResult)`)
	s := val.String()
	// Should have two splits, sorted by group name: docs first, then pkg
	assert.Contains(t, s, `"pr/01-docs"`)
	assert.Contains(t, s, `"pr/02-pkg"`)
	assert.Contains(t, s, `"baseBranch":"main"`)
	assert.Contains(t, s, `"sourceBranch":"feat"`)
}

func TestPRSplit_AnalyzeDiff(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	runJS(`var prSplit = require('` + sp + `');`)

	// Escape backslashes for Windows paths (though test is skipped on Windows).
	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)

	runAsyncJS(t, bridge, `
		globalThis.__analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		__signalDone();
	`)

	// Error should be null.
	errVal := runJS(`__analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "error should be null, got: %v", errVal)

	// Current branch should be feature.
	branchVal := runJS(`__analysis.currentBranch`)
	assert.Equal(t, "feature", branchVal.String())

	// Should find 5 changed files.
	lenVal := runJS(`__analysis.files.length`)
	assert.Equal(t, int64(5), lenVal.ToInteger())

	// Spot-check specific files.
	filesVal := runJS(`JSON.stringify(__analysis.files.sort())`)
	assert.Contains(t, filesVal.String(), "pkg/impl.go")
	assert.Contains(t, filesVal.String(), "docs/guide.md")
	assert.Contains(t, filesVal.String(), "cmd/run.go")
}

func TestPRSplit_AnalyzeDiffStats(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)
	runAsyncJS(t, bridge, `
		globalThis.__stats = await prSplit.analyzeDiffStats({baseBranch: 'main', dir: '`+escapedDir+`'});
		__signalDone();
	`)

	errVal := runJS(`__stats.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal))

	lenVal := runJS(`__stats.files.length`)
	assert.Equal(t, int64(5), lenVal.ToInteger())

	// Each file should have additions > 0.
	addVal := runJS(`__stats.files[0].additions`)
	assert.Greater(t, addVal.ToInteger(), int64(0))
}

func TestPRSplit_ExecuteSplit(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Create plan from analysis and execute.
	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			verifyCommand: 'true',
			fileStatuses: analysis.fileStatuses
		});
		globalThis.__plan = plan;
		globalThis.__result = await prSplit.executeSplit(plan);
		__signalDone();
	`)

	// Validate plan.
	valResult := runJS(`JSON.stringify(prSplit.validatePlan(__plan))`)
	assert.Contains(t, valResult.String(), `"valid":true`)

	// No error.
	errVal := runJS(`__result.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "execute error: %v", errVal)

	// All splits should have SHAs.
	splitCount := runJS(`__result.results.length`)
	assert.Equal(t, int64(3), splitCount.ToInteger()) // cmd, docs, pkg

	// Verify each result has a non-empty SHA.
	for i := range 3 {
		shaVal := runJS(fmt.Sprintf(`__result.results[%d].sha`, i))
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
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Analyze, group, plan, execute, verify equivalence.
	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			fileStatuses: analysis.fileStatuses
		});
		await prSplit.executeSplit(plan);
		globalThis.__equiv = await prSplit.verifyEquivalence(plan);
		__signalDone();
	`)

	equivVal := runJS(`__equiv.equivalent`)
	assert.Equal(t, true, equivVal.ToBoolean(), "tree hashes should match")

	errVal := runJS(`__equiv.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal))
}

func TestPRSplit_VerifySplits(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			verifyCommand: 'true',
			fileStatuses: analysis.fileStatuses
		});
		await prSplit.executeSplit(plan);
		globalThis.__verify = await prSplit.verifySplits(plan);
		__signalDone();
	`)

	// Verify all splits (with 'true' command, should all pass).
	allPassed := runJS(`__verify.allPassed`)
	assert.Equal(t, true, allPassed.ToBoolean())

	verifyLen := runJS(`__verify.results.length`)
	assert.Equal(t, int64(3), verifyLen.ToInteger())

	// Restore to feature after verifySplits.
	currentBranch := strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, "feature", currentBranch)
}

func TestPRSplit_CleanupBranches(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			fileStatuses: analysis.fileStatuses
		});
		await prSplit.executeSplit(plan);
		globalThis.__plan = plan;
		__signalDone();
	`)

	// Verify branches exist before cleanup.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/01-cmd")

	// Cleanup.
	runAsyncJS(t, bridge, `
		globalThis.__cleanup = await prSplit.cleanupBranches(__plan);
		__signalDone();
	`)
	deletedLen := runJS(`__cleanup.deleted.length`)
	assert.Equal(t, int64(3), deletedLen.ToInteger())

	errLen := runJS(`__cleanup.errors.length`)
	assert.Equal(t, int64(0), errLen.ToInteger())

	// Verify branches are gone.
	branchesAfter := runGit(t, dir, "branch")
	assert.NotContains(t, branchesAfter, "split/01-cmd")
	assert.NotContains(t, branchesAfter, "split/02-docs")
	assert.NotContains(t, branchesAfter, "split/03-pkg")
}

func TestPRSplit_AnalyzeDiff_NoChanges(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	// Don't add feature files — no changes from main.

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		globalThis.__analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		__signalDone();
	`)

	errVal := runJS(`__analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "error should be null for no-changes case")

	filesLen := runJS(`__analysis.files.length`)
	assert.Equal(t, int64(0), filesLen.ToInteger())
}

func TestPRSplit_ExecuteSplit_InvalidPlan(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		globalThis.__result = await prSplit.executeSplit({ splits: [] });
		__signalDone();
	`)
	errVal := runJS(`__result.error`)
	assert.Contains(t, errVal.String(), "invalid plan")
}

func TestPRSplit_SelectStrategy(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		globalThis.__stratResult = await prSplit.selectStrategy(
			['pkg/a.go', 'pkg/b.go', 'cmd/main.go', 'docs/readme.md', 'Makefile']
		);
		__signalDone();
	`)

	val := runJS(`JSON.stringify(__stratResult)`)
	s := val.String()
	assert.Contains(t, s, `"strategy"`)
	assert.Contains(t, s, `"reason"`)
	assert.Contains(t, s, `"groups"`)

	// Strategy should be one of the known values.
	stratVal := runJS(`__stratResult.strategy`)
	known := []string{"directory", "directory-deep", "extension", "chunks", "dependency"}
	assert.Contains(t, known, stratVal.String())
}

func TestPRSplit_SelectStrategy_Scored(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	// With many files across multiple directories, scored should have entries.
	runAsyncJS(t, bridge, `
		globalThis.__stratResult = await prSplit.selectStrategy([
			'pkg/a.go', 'pkg/b.go', 'pkg/c.go',
			'cmd/main.go', 'cmd/run.go',
			'docs/readme.md', 'docs/guide.md',
			'internal/foo.go', 'internal/bar.go',
			'tests/test_a.go'
		]);
		__signalDone();
	`)

	scoredLen := runJS(`__stratResult.scored.length`)
	assert.Equal(t, int64(5), scoredLen.ToInteger(), "should score 5 strategies")
}

func TestPRSplit_VerifyEquivalenceDetailed_Equivalent(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			fileStatuses: analysis.fileStatuses
		});
		await prSplit.executeSplit(plan);
		globalThis.__equiv = await prSplit.verifyEquivalenceDetailed(plan);
		__signalDone();
	`)

	equivVal := runJS(`__equiv.equivalent`)
	assert.Equal(t, true, equivVal.ToBoolean())

	// When equivalent, diffFiles should be empty.
	diffLen := runJS(`__equiv.diffFiles.length`)
	assert.Equal(t, int64(0), diffLen.ToInteger())

	diffSummary := runJS(`__equiv.diffSummary`)
	assert.Equal(t, "", diffSummary.String())
}

// ---------------------------------------------------------------------------
//  T209: End-to-end PR split with real compilation verification
// ---------------------------------------------------------------------------

// initCompilableGitRepo creates a temporary git repo with go.mod and
// compilable Go source files. Each split branch from this repo can be
// verified with "go build ./..." independently.
func initCompilableGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
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
	// Not parallel: t.Setenv isolates TMPDIR to avoid VCS stamping
	// issues from stale /tmp/.git directories left by other processes.
	t.Setenv("TMPDIR", t.TempDir())
	bridge, runJS := prSplitTestEnv(t)

	// E2E test with real compilation; increase timeout to avoid flakes.
	bridge.SetTimeout(60 * time.Second)

	sp := prSplitScriptPath(t)

	dir := initCompilableGitRepo(t)
	addCompilableFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// 1. Analyze what changed.
	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			verifyCommand: 'go build ./...',
			fileStatuses: analysis.fileStatuses
		});
		globalThis.__analysis = analysis;
		globalThis.__groups = groups;
		globalThis.__plan = plan;
		__signalDone();
	`)

	errVal := runJS(`__analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal), "analysis error: %v", errVal)
	assert.Equal(t, "feature", runJS(`__analysis.currentBranch`).String())
	assert.Equal(t, int64(3), runJS(`__analysis.files.length`).ToInteger(),
		"expected 3 changed files: pkg/bar.go, cmd/app/run.go, docs/guide.md")

	// 2. Group by directory (depth=1).
	groupKeys := runJS(`Object.keys(__groups).sort().join(',')`)
	assert.Equal(t, "cmd,docs,pkg", groupKeys.String())

	// 3. Validate plan.
	valResult := runJS(`JSON.stringify(prSplit.validatePlan(__plan))`)
	assert.Contains(t, valResult.String(), `"valid":true`)

	assert.Equal(t, int64(3), runJS(`__plan.splits.length`).ToInteger())
	t.Logf("Split plan: %d splits", 3)
	for i := range 3 {
		name := runJS(fmt.Sprintf(`__plan.splits[%d].name`, i)).String()
		filesLen := runJS(fmt.Sprintf(`__plan.splits[%d].files.length`, i)).ToInteger()
		t.Logf("  %s (%d files)", name, filesLen)
	}

	// 4. Execute the split — creates stacked branches.
	runAsyncJS(t, bridge, `
		globalThis.__execResult = await prSplit.executeSplit(__plan);
		__signalDone();
	`)
	execErr := runJS(`__execResult.error`)
	assert.True(t, goja.IsNull(execErr) || goja.IsUndefined(execErr),
		"execute error: %v", execErr)

	splitCount := runJS(`__execResult.results.length`).ToInteger()
	assert.Equal(t, int64(3), splitCount)
	for i := range splitCount {
		sha := runJS(fmt.Sprintf(`__execResult.results[%d].sha`, i)).String()
		name := runJS(fmt.Sprintf(`__execResult.results[%d].name`, i)).String()
		assert.NotEmpty(t, sha, "split %s should have a SHA", name)
		t.Logf("  Created: %s (sha=%s)", name, sha[:8])
	}

	// Verify branches were created in git.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/01-cmd")
	assert.Contains(t, branches, "split/02-docs")
	assert.Contains(t, branches, "split/03-pkg")

	// 5. Verify each split compiles with "go build ./..."
	runAsyncJS(t, bridge, `
		globalThis.__verify = await prSplit.verifySplits(__plan);
		__signalDone();
	`)
	allPassed := runJS(`__verify.allPassed`).ToBoolean()

	verifyLen := runJS(`__verify.results.length`).ToInteger()
	for i := range verifyLen {
		name := runJS(fmt.Sprintf(`__verify.results[%d].name`, i)).String()
		passed := runJS(fmt.Sprintf(`__verify.results[%d].passed`, i)).ToBoolean()
		t.Logf("  Verify: %s compiled=%v", name, passed)
		if !passed {
			errStr := runJS(fmt.Sprintf(`__verify.results[%d].error`, i)).String()
			t.Logf("    Error: %s", errStr)
		}
		assert.True(t, passed, "split %s should compile with 'go build ./...'", name)
	}
	assert.True(t, allPassed, "all split branches must compile independently")

	// 6. Verify tree equivalence — final split tree must match source.
	runAsyncJS(t, bridge, `
		globalThis.__equiv = await prSplit.verifyEquivalence(__plan);
		__signalDone();
	`)
	equivalent := runJS(`__equiv.equivalent`).ToBoolean()
	assert.True(t, equivalent, "final split tree hash must equal source branch tree hash")
	if !equivalent {
		splitTree := runJS(`__equiv.splitTree`).String()
		sourceTree := runJS(`__equiv.sourceTree`).String()
		t.Fatalf("Tree mismatch: split=%s source=%s", splitTree, sourceTree)
	}

	// 7. Verify current branch was restored.
	currentBranch := strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
	assert.Equal(t, "feature", currentBranch)

	t.Log("T209 PASS: Full PR split workflow with real compilation verification")
}

func TestPRSplit_ExecuteSplit_MissingFile(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
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
	runAsyncJS(t, bridge, `
		globalThis.__result = await prSplit.executeSplit(plan);
		__signalDone();
	`)

	errVal := runJS(`__result.error`)
	assert.Contains(t, errVal.String(), "checkout file")
	assert.Contains(t, errVal.String(), "does-not-exist.go")
}

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
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFilesWithDeletions(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)
	runAsyncJS(t, bridge, `
		globalThis.__analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		__signalDone();
	`)

	// Error should be null.
	errVal := runJS(`__analysis.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal))

	// Should find 3 files: 2 added + 1 deleted.
	lenVal := runJS(`__analysis.files.length`)
	assert.Equal(t, int64(3), lenVal.ToInteger())

	// Verify fileStatuses is populated correctly.
	implStatus := runJS(`__analysis.fileStatuses['pkg/impl.go']`)
	assert.Equal(t, "A", implStatus.String())

	docsStatus := runJS(`__analysis.fileStatuses['docs/guide.md']`)
	assert.Equal(t, "A", docsStatus.String())

	readmeStatus := runJS(`__analysis.fileStatuses['README.md']`)
	assert.Equal(t, "D", readmeStatus.String())
}

func TestPRSplit_ExecuteSplit_WithDeletedFiles(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFilesWithDeletions(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Full pipeline: analyze → group → plan → execute → verify equivalence.
	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			verifyCommand: 'true',
			fileStatuses: analysis.fileStatuses
		});
		var result = await prSplit.executeSplit(plan);
		globalThis.__plan = plan;
		globalThis.__result = result;
		globalThis.__equiv = await prSplit.verifyEquivalence(plan);
		__signalDone();
	`)

	errVal := runJS(`__result.error`)
	assert.True(t, goja.IsNull(errVal) || goja.IsUndefined(errVal),
		"executeSplit should succeed with deleted files, got: %v", errVal)

	// Verify equivalence — tree hashes must match.
	equivVal := runJS(`__equiv.equivalent`)
	assert.True(t, equivVal.ToBoolean(), "tree hashes should match when deletions are handled correctly")

	// The branch containing the deletion (README.md is in '.') should exist.
	branches := runGit(t, dir, "branch")
	assert.Contains(t, branches, "split/")

	// Verify README.md is actually gone on the last split branch.
	lastSplit := runJS(`__plan.splits[__plan.splits.length-1].name`).String()
	runGit(t, dir, "checkout", lastSplit)
	_, err := os.Stat(filepath.Join(dir, "README.md"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "README.md should not exist on the last split branch")

	// Restore to feature.
	runGit(t, dir, "checkout", "feature")
}

func TestPRSplit_ExecuteSplit_RerunDeletesBranches(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// First run — analyze, plan, execute.
	runAsyncJS(t, bridge, `
		var analysis = await prSplit.analyzeDiff({baseBranch: 'main', dir: '`+escapedDir+`'});
		var groups = prSplit.groupByDirectory(analysis.files, 1);
		var plan = await prSplit.createSplitPlan(groups, {
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			branchPrefix: 'split/',
			fileStatuses: analysis.fileStatuses
		});
		globalThis.__plan = plan;
		globalThis.__result1 = await prSplit.executeSplit(plan);
		__signalDone();
	`)
	err1 := runJS(`__result1.error`)
	assert.True(t, goja.IsNull(err1) || goja.IsUndefined(err1), "first run should succeed")

	branches1 := runGit(t, dir, "branch")
	assert.Contains(t, branches1, "split/01-cmd")

	// Second run — same plan, branches already exist. Should NOT fail.
	runAsyncJS(t, bridge, `
		globalThis.__result2 = await prSplit.executeSplit(__plan);
		globalThis.__equiv = await prSplit.verifyEquivalence(__plan);
		__signalDone();
	`)
	err2 := runJS(`__result2.error`)
	assert.True(t, goja.IsNull(err2) || goja.IsUndefined(err2),
		"re-run should succeed (pre-existing branches deleted), got: %v", err2)

	// Verify equivalence still holds after re-run.
	equivVal := runJS(`__equiv.equivalent`)
	assert.True(t, equivVal.ToBoolean(), "tree hashes should match after re-run")
}

func TestPRSplit_ExecuteSplit_NoFileStatuses(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)
	runJS(`var prSplit = require('` + sp + `');`)

	// Plan with valid structure but missing fileStatuses.
	runAsyncJS(t, bridge, `
		globalThis.__result = await prSplit.executeSplit({
			baseBranch: 'main',
			sourceBranch: 'feature',
			splits: [{
				name: 'split/01-test',
				files: ['a.go'],
				message: 'test'
			}]
		});
		__signalDone();
	`)

	errVal := runJS(`__result.error`)
	assert.Contains(t, errVal.String(), "fileStatuses is required")
}

func TestPRSplit_ExecuteSplit_MissingFileStatus(t *testing.T) {
	t.Parallel()
	bridge, runJS := prSplitTestEnv(t)
	sp := prSplitScriptPath(t)

	dir := initTestGitRepo(t)
	addFeatureFiles(t, dir)

	escapedDir := strings.ReplaceAll(dir, `\`, `\\`)
	runJS(`var prSplit = require('` + sp + `');`)

	// Plan with fileStatuses that's missing an entry for one file.
	runAsyncJS(t, bridge, `
		globalThis.__result = await prSplit.executeSplit({
			baseBranch: 'main',
			sourceBranch: 'feature',
			dir: '`+escapedDir+`',
			fileStatuses: { 'pkg/impl.go': 'A' },
			splits: [{
				name: 'split/01-test',
				files: ['pkg/impl.go', 'cmd/run.go'],
				message: 'test'
			}]
		});
		__signalDone();
	`)

	errVal := runJS(`__result.error`)
	assert.Contains(t, errVal.String(), "cmd/run.go")
	assert.Contains(t, errVal.String(), "no entry in plan.fileStatuses")
}
