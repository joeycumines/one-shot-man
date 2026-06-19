package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// newTestRuntime creates a goja runtime with the osm:gitops module registered.
// Uses a nil adapter — sync bindings work; async bindings will panic if called.
func newTestRuntime(t *testing.T) *goja.Runtime {
	t.Helper()
	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	Require(context.Background(), nil)(runtime, module)
	_ = runtime.Set("gitops", module.Get("exports"))
	return runtime
}

// asyncTestEnv creates a goja runtime with a running event loop and adapter.
// Returns the runtime and a runJS helper that executes a script on the loop
// and waits for JS to call __collect(value) or __collectErr(msg), returning
// the value or error. This is thread-safe — the callbacks fire on the event
// loop goroutine; the test goroutine blocks on a channel.
func asyncTestEnv(t *testing.T) (*goja.Runtime, func(string) (goja.Value, error)) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping async gitops test in -short mode")
	}
	runtime := goja.New()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	Require(context.Background(), adapter)(runtime, module)
	_ = runtime.Set("gitops", module.Get("exports"))

	resultCh := make(chan goja.Value, 1)
	errCh := make(chan error, 1)
	_ = runtime.Set("__collect", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	})
	_ = runtime.Set("__collectErr", func(call goja.FunctionCall) goja.Value {
		errCh <- fmt.Errorf("%s", call.Argument(0).String())
		return goja.Undefined()
	})

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	runJS := func(script string) (goja.Value, error) {
		t.Helper()
		submitErr := loop.Submit(func() {
			_, runErr := runtime.RunString(script)
			if runErr != nil {
				errCh <- runErr
			}
		})
		if submitErr != nil {
			return goja.Undefined(), submitErr
		}
		select {
		case val := <-resultCh:
			return val, nil
		case err := <-errCh:
			return goja.Undefined(), err
		case <-time.After(5 * time.Second):
			return goja.Undefined(), fmt.Errorf("timeout waiting for async result")
		}
	}
	return runtime, runJS
}

// setupTestRepo creates a non-bare git repo with one commit on the "main"
// branch and returns its path. The repo is created in a temp directory.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	// Set HEAD to refs/heads/main.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName("main"),
	)); err != nil {
		t.Fatalf("SetReference HEAD: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sig := &object.Signature{
		Name:  "test",
		Email: "test@test.com",
		When:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return dir
}

// setupTestRepoWithBranch creates a non-bare git repo with one commit on the
// specified branch and returns its path.
func setupTestRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName(branchName),
	)); err != nil {
		t.Fatalf("SetReference HEAD: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sig := &object.Signature{
		Name:  "test",
		Email: "test@test.com",
		When:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return dir
}

func TestModule_IsRepo(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("isRepo"))
	if !ok {
		t.Fatal("isRepo is not a function")
	}

	// True for a real repo.
	val, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("isRepo(repo dir): %v", err)
	}
	if !val.ToBoolean() {
		t.Error("expected isRepo=true for git repo")
	}

	// False for an empty dir.
	val, err = fn(goja.Undefined(), runtime.ToValue(t.TempDir()))
	if err != nil {
		t.Fatalf("isRepo(empty dir): %v", err)
	}
	if val.ToBoolean() {
		t.Error("expected isRepo=false for empty dir")
	}
}

func TestModule_ErrorConstants(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	g := runtime.Get("gitops").ToObject(runtime)

	cases := map[string]string{
		"ERR_NOT_REPO":          "gitops: not a git repository",
		"ERR_NOTHING_TO_COMMIT": "gitops: nothing to commit",
		"ERR_CONFLICT":          "gitops: merge conflict",
		"ERR_DETACHED_HEAD":     "gitops: detached HEAD",
	}

	for name, want := range cases {
		got := g.Get(name).String()
		if got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestModule_Open_DefaultBranch(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("open"))
	if !ok {
		t.Fatal("open is not a function")
	}

	repoObj, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("open(%q): %v", dir, err)
	}

	// Call defaultBranch() on the repo wrapper.
	dbFn, ok := goja.AssertFunction(repoObj.ToObject(runtime).Get("defaultBranch"))
	if !ok {
		t.Fatal("defaultBranch is not a function")
	}

	val, err := dbFn(goja.Undefined())
	if err != nil {
		t.Fatalf("defaultBranch(): %v", err)
	}
	if val.String() != "main" {
		t.Errorf("expected 'main', got %q", val.String())
	}
}

func TestModule_OpenDetect_DefaultBranch(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	// Open from a subdirectory.
	subDir := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("openDetect"))
	if !ok {
		t.Fatal("openDetect is not a function")
	}

	repoObj, err := fn(goja.Undefined(), runtime.ToValue(subDir))
	if err != nil {
		t.Fatalf("openDetect(%q): %v", subDir, err)
	}

	// Call defaultBranch() on the repo wrapper.
	dbFn, ok := goja.AssertFunction(repoObj.ToObject(runtime).Get("defaultBranch"))
	if !ok {
		t.Fatal("defaultBranch is not a function")
	}

	val, err := dbFn(goja.Undefined())
	if err != nil {
		t.Fatalf("defaultBranch(): %v", err)
	}
	if val.String() != "main" {
		t.Errorf("expected 'main', got %q", val.String())
	}
}

func TestModule_Open_BranchExists(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("open"))
	if !ok {
		t.Fatal("open is not a function")
	}

	repoObj, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("open(%q): %v", dir, err)
	}

	beFn, ok := goja.AssertFunction(repoObj.ToObject(runtime).Get("branchExists"))
	if !ok {
		t.Fatal("branchExists is not a function")
	}

	// "main" exists.
	val, err := beFn(goja.Undefined(), runtime.ToValue("main"))
	if err != nil {
		t.Fatalf("branchExists('main'): %v", err)
	}
	if !val.ToBoolean() {
		t.Error("expected branchExists('main')=true")
	}

	// "nonexistent" doesn't exist.
	val, err = beFn(goja.Undefined(), runtime.ToValue("nonexistent-xyz"))
	if err != nil {
		t.Fatalf("branchExists('nonexistent-xyz'): %v", err)
	}
	if val.ToBoolean() {
		t.Error("expected branchExists('nonexistent-xyz')=false")
	}
}

func TestModule_Open_IsWorkTree(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("open"))
	if !ok {
		t.Fatal("open is not a function")
	}

	repoObj, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("open(%q): %v", dir, err)
	}

	iwtFn, ok := goja.AssertFunction(repoObj.ToObject(runtime).Get("isWorkTree"))
	if !ok {
		t.Fatal("isWorkTree is not a function")
	}

	val, err := iwtFn(goja.Undefined())
	if err != nil {
		t.Fatalf("isWorkTree(): %v", err)
	}
	if !val.ToBoolean() {
		t.Error("expected isWorkTree()=true for normal repo")
	}
}

func TestModule_Open_HeadBranchName(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("open"))
	if !ok {
		t.Fatal("open is not a function")
	}

	repoObj, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("open(%q): %v", dir, err)
	}

	hbnFn, ok := goja.AssertFunction(repoObj.ToObject(runtime).Get("headBranchName"))
	if !ok {
		t.Fatal("headBranchName is not a function")
	}

	val, err := hbnFn(goja.Undefined())
	if err != nil {
		t.Fatalf("headBranchName(): %v", err)
	}
	if val.String() != "main" {
		t.Errorf("expected 'main', got %q", val.String())
	}
}

func TestModule_Open_NotARepo(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("open"))
	if !ok {
		t.Fatal("open is not a function")
	}

	_, err := fn(goja.Undefined(), runtime.ToValue(t.TempDir()))
	if err == nil {
		t.Fatal("expected TypeError for non-repo dir")
	}

	// Verify the error message mentions the gitops error.
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' in error, got: %v", err)
	}
}

func TestModule_Convenience_DefaultBranch(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("defaultBranch"))
	if !ok {
		t.Fatal("defaultBranch is not a function")
	}

	val, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("defaultBranch(%q): %v", dir, err)
	}
	if val.String() != "main" {
		t.Errorf("expected 'main', got %q", val.String())
	}
}

func TestModule_Convenience_BranchExists(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("branchExists"))
	if !ok {
		t.Fatal("branchExists is not a function")
	}

	val, err := fn(goja.Undefined(), runtime.ToValue(dir), runtime.ToValue("main"))
	if err != nil {
		t.Fatalf("branchExists(%q, 'main'): %v", dir, err)
	}
	if !val.ToBoolean() {
		t.Error("expected branchExists=true for 'main'")
	}

	val, err = fn(goja.Undefined(), runtime.ToValue(dir), runtime.ToValue("nonexistent"))
	if err != nil {
		t.Fatalf("branchExists(%q, 'nonexistent'): %v", dir, err)
	}
	if val.ToBoolean() {
		t.Error("expected branchExists=false for 'nonexistent'")
	}
}

func TestModule_Convenience_IsWorkTree(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("isWorkTree"))
	if !ok {
		t.Fatal("isWorkTree is not a function")
	}

	val, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("isWorkTree(%q): %v", dir, err)
	}
	if !val.ToBoolean() {
		t.Error("expected isWorkTree=true for normal repo")
	}
}

func TestModule_Convenience_HeadBranchName(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepo(t)

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("headBranchName"))
	if !ok {
		t.Fatal("headBranchName is not a function")
	}

	val, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("headBranchName(%q): %v", dir, err)
	}
	if val.String() != "main" {
		t.Errorf("expected 'main', got %q", val.String())
	}
}

func TestModule_Open_MasterBranch(t *testing.T) {
	t.Parallel()
	runtime := newTestRuntime(t)
	dir := setupTestRepoWithBranch(t, "master")

	fn, ok := goja.AssertFunction(runtime.Get("gitops").ToObject(runtime).Get("open"))
	if !ok {
		t.Fatal("open is not a function")
	}

	repoObj, err := fn(goja.Undefined(), runtime.ToValue(dir))
	if err != nil {
		t.Fatalf("open(%q): %v", dir, err)
	}

	dbFn, ok := goja.AssertFunction(repoObj.ToObject(runtime).Get("defaultBranch"))
	if !ok {
		t.Fatal("defaultBranch is not a function")
	}

	val, err := dbFn(goja.Undefined())
	if err != nil {
		t.Fatalf("defaultBranch(): %v", err)
	}
	if val.String() != "master" {
		t.Errorf("expected 'master', got %q", val.String())
	}
}

func TestModule_Async_HasStagedChanges(t *testing.T) {
	_, runJS := asyncTestEnv(t)
	dir := setupTestRepo(t)

	val, err := runJS(fmt.Sprintf(`
		var repo = gitops.open(%q);
		repo.hasStagedChanges().then(
			function(has) { __collect(has); },
			function(err) { __collectErr(err.toString()); }
		);
	`, dir))
	if err != nil {
		t.Fatalf("hasStagedChanges: %v", err)
	}
	if val.ToBoolean() {
		t.Error("expected hasStagedChanges=false for clean repo")
	}
}

func TestModule_Async_AddAll_Commit(t *testing.T) {
	_, runJS := asyncTestEnv(t)
	dir := setupTestRepo(t)

	// Create a new file, addAll, then commit — all via async bindings.
	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("new content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := runJS(fmt.Sprintf(`
		var repo = gitops.open(%q);
		repo.addAll().then(function() {
			return repo.commit("test commit");
		}).then(
			function(hash) { __collect(hash); },
			function(err) { __collectErr(err.toString()); }
		);
	`, dir))
	if err != nil {
		t.Fatalf("addAll+commit: %v", err)
	}

	// Verify the commit was created by checking hasStagedChanges is false.
	val, err := runJS(fmt.Sprintf(`
		var repo = gitops.open(%q);
		repo.hasStagedChanges().then(
			function(has) { __collect(has); },
			function(err) { __collectErr(err.toString()); }
		);
	`, dir))
	if err != nil {
		t.Fatalf("hasStagedChanges after commit: %v", err)
	}
	if val.ToBoolean() {
		t.Error("expected hasStagedChanges=false after commit")
	}
}

func TestModule_Async_AddAll_WithNewFile(t *testing.T) {
	_, runJS := asyncTestEnv(t)
	dir := setupTestRepo(t)

	// Create a new untracked file in the repo dir.
	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("new content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// addAll should stage the new file, making hasStagedChanges true.
	_, err := runJS(fmt.Sprintf(`
		var repo = gitops.open(%q);
		repo.addAll().then(function() {
			return repo.hasStagedChanges();
		}).then(
			function(has) { __collect(has); },
			function(err) { __collectErr(err.toString()); }
		);
	`, dir))
	if err != nil {
		t.Fatalf("addAll+hasStagedChanges: %v", err)
	}

	// Now commit to clean the index.
	_, err = runJS(fmt.Sprintf(`
		var repo = gitops.open(%q);
		repo.commit("add newfile").then(
			function(hash) { __collect(hash); },
			function(err) { __collectErr(err.toString()); }
		);
	`, dir))
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestModule_Async_Commit_NoStagedChanges_Rejects(t *testing.T) {
	_, runJS := asyncTestEnv(t)
	dir := setupTestRepo(t)

	// Repo already has one commit and no staged changes.
	// commit() should reject with "nothing to commit".
	_, err := runJS(fmt.Sprintf(`
		var repo = gitops.open(%q);
		repo.commit("nothing to commit").then(
			function(hash) { __collect("unexpected success: " + hash); },
			function(err) { __collectErr(err.toString()); }
		);
	`, dir))
	if err == nil {
		t.Fatal("expected commit to reject with nothing-to-commit error, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to commit") {
		t.Errorf("expected 'nothing to commit' in error, got: %v", err)
	}
}
