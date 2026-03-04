package command

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// session.go — delete (54.2% → target ~85%)
// ==========================================================================

// TestSessionDelete_DryRun covers the dry-run early return.
func TestSessionDelete_DryRun(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)
	cmd.dry = true

	var buf bytes.Buffer
	err := cmd.delete(&buf, "test-session")
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Dry-run: would delete session test-session")
}

// TestSessionDelete_HappyPath creates a real session file, acquires the lock,
// and deletes it. Uses storage.SetTestPaths to isolate to a temp dir.
func TestSessionDelete_HappyPath(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	t.Cleanup(storage.ResetPaths)

	// cmd.delete acquires a lock file — ensure leftover lock artifacts are
	// removed before t.TempDir cleanup runs RemoveAll.
	t.Cleanup(func() {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	})

	id := "happy-delete"
	// Create a fake session file.
	sessionFile := filepath.Join(dir, id+".session.json")
	require.NoError(t, os.WriteFile(sessionFile, []byte(`{}`), 0644))

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)
	cmd.yes = true

	var buf bytes.Buffer
	err := cmd.delete(&buf, id)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "deleted "+id)

	// Verify session file was removed.
	_, statErr := os.Stat(sessionFile)
	assert.True(t, os.IsNotExist(statErr), "session file should be removed")
}

// TestSessionDelete_NonexistentSession tries to delete a session whose file
// doesn't exist. os.Remove should fail.
func TestSessionDelete_NonexistentSession(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	t.Cleanup(storage.ResetPaths)

	// cmd.delete acquires a lock file before attempting os.Remove. If the
	// session file doesn't exist the lock file is left behind (by design:
	// "close descriptor to avoid fd leaks but keep lockfile artifact").
	// t.TempDir cleanup can race with the OS releasing the file descriptor,
	// so explicitly remove any leftover lock artifacts.
	t.Cleanup(func() {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	})

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)
	cmd.yes = true

	var buf bytes.Buffer
	err := cmd.delete(&buf, "nonexistent")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err), "expected file-not-found error, got: %v", err)
}

// TestSessionDelete_LockedSession creates a session, acquires its lock
// externally, and then verifies delete refuses.
func TestSessionDelete_LockedSession(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	t.Cleanup(storage.ResetPaths)

	id := "locked-session"
	sessionFile := filepath.Join(dir, id+".session.json")
	require.NoError(t, os.WriteFile(sessionFile, []byte(`{}`), 0644))

	// Acquire the lock externally.
	lockPath := filepath.Join(dir, id+".session.lock")
	lockF, ok, err := storage.AcquireLockHandle(lockPath)
	require.NoError(t, err)
	require.True(t, ok, "must acquire lock for test setup")
	t.Cleanup(func() { _ = lockF.Close() })

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)
	cmd.yes = true

	var buf bytes.Buffer
	err = cmd.delete(&buf, id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "appears active or locked")
}

// ==========================================================================
// registry.go — isExecutable (42.9%)
// ==========================================================================

// TestIsExecutable_UnixPaths covers the Unix branch with and without exec bits.
func TestIsExecutable_UnixPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific execute bit test")
	}
	t.Parallel()

	dir := t.TempDir()

	// File with execute bit.
	execFile := filepath.Join(dir, "exec.sh")
	require.NoError(t, os.WriteFile(execFile, []byte("#!/bin/sh\n"), 0755))
	fi, err := os.Stat(execFile)
	require.NoError(t, err)
	assert.True(t, isExecutable(fi), "file with 0755 should be executable")

	// File without execute bit.
	noExecFile := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(noExecFile, []byte("data"), 0644))
	fi2, err := os.Stat(noExecFile)
	require.NoError(t, err)
	assert.False(t, isExecutable(fi2), "file with 0644 should not be executable")

	// File with only group execute bit.
	groupExec := filepath.Join(dir, "group.sh")
	require.NoError(t, os.WriteFile(groupExec, []byte("#!/bin/sh\n"), 0010))
	fi3, err := os.Stat(groupExec)
	require.NoError(t, err)
	assert.True(t, isExecutable(fi3), "file with group execute should be executable")
}

// TestIsExecutable_WindowsPaths covers the Windows branch by extension check.
// Only runs on Windows.
func TestIsExecutable_WindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific extension test")
	}
	t.Parallel()

	dir := t.TempDir()

	for _, ext := range []string{".exe", ".com", ".bat", ".cmd"} {
		name := "test" + ext
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("content"), 0644))
		fi, err := os.Stat(p)
		require.NoError(t, err)
		assert.True(t, isExecutable(fi), "Windows: %s should be executable", name)
	}

	nonExec := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(nonExec, []byte("content"), 0644))
	fi, err := os.Stat(nonExec)
	require.NoError(t, err)
	assert.False(t, isExecutable(fi), "Windows: .txt should not be executable")
}

// ==========================================================================
// base.go — SetupFlags (0%)
// ==========================================================================

// TestBaseCommand_SetupFlags_NoOp_Batch10 verifies the no-op default
// implementation — duplicate coverage guard.
func TestBaseCommand_SetupFlags_NoOp_Batch10(t *testing.T) {
	t.Parallel()
	bc := NewBaseCommand("test-cmd", "testing help", "test-cmd [args]")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	// Should not panic and should not register any flags.
	bc.SetupFlags(fs)

	count := 0
	fs.VisitAll(func(*flag.Flag) { count++ })
	assert.Equal(t, 0, count, "BaseCommand.SetupFlags should not register any flags")
}

// ==========================================================================
// sync.go — SetupFlags (0%)
// ==========================================================================

// TestSyncCommand_SetupFlags_NoOp verifies the no-op sync SetupFlags.
func TestSyncCommand_SetupFlags_NoOp(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSyncCommand(cfg)
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)

	// Should not panic and should not register any flags.
	cmd.SetupFlags(fs)

	count := 0
	fs.VisitAll(func(*flag.Flag) { count++ })
	assert.Equal(t, 0, count, "SyncCommand.SetupFlags should not register any flags")
}

// ==========================================================================
// script_discovery.go — traverseForGitRepos (65%), autodiscoverPaths (70%)
// ==========================================================================

// TestTraverseForGitRepos_FindsRepos exercises the upward traversal finding git repos
// and returning script paths within them.
// traverseForGitRepos walks UPWARD from startDir, finds git repos via
// `git rev-parse --git-dir`, then returns script directories matching
// ScriptPathPatterns inside those repos.
func TestTraverseForGitRepos_FindsRepos(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Initialize a real git repo at root.
	gitInit := exec.Command("git", "init", root)
	gitInit.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	require.NoError(t, gitInit.Run(), "git init must succeed")

	// Create a "scripts" directory inside the repo (matches ScriptPathPatterns).
	scriptsDir := filepath.Join(root, "scripts")
	require.NoError(t, os.MkdirAll(scriptsDir, 0755))

	// Start traversal from a subdirectory — the function walks upward,
	// finds root as a git repo, then looks for "scripts" inside it.
	startDir := filepath.Join(root, "src", "pkg")
	require.NoError(t, os.MkdirAll(startDir, 0755))

	sd := &ScriptDiscovery{config: &ScriptDiscoveryConfig{
		MaxTraversalDepth:  5,
		ScriptPathPatterns: []string{"scripts"},
	}}
	paths := sd.traverseForGitRepos(startDir)

	assert.GreaterOrEqual(t, len(paths), 1, "should find at least 1 script path from git repo")

	// Resolve to real paths (TempDir may use symlinks on macOS).
	realScripts, err := filepath.EvalSymlinks(scriptsDir)
	require.NoError(t, err)

	found := false
	for _, p := range paths {
		realP, _ := filepath.EvalSymlinks(p)
		if realP == realScripts {
			found = true
		}
	}
	assert.True(t, found, "should find the scripts directory inside the git repo")
}

// TestTraverseForGitRepos_DepthLimit exercises the depth limiter.
// traverseForGitRepos walks UPWARD from startDir, so depth limits how
// many parent directories are checked.
func TestTraverseForGitRepos_DepthLimit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Initialize a real git repo at root.
	gitInit := exec.Command("git", "init", root)
	gitInit.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	require.NoError(t, gitInit.Run(), "git init must succeed")

	// Create a scripts dir so the repo has discoverable paths.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts"), 0755))

	// Start deep — root is 4 levels up from start.
	startDir := filepath.Join(root, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(startDir, 0755))

	// With depth 2, traversal visits startDir and its parent (2 levels up).
	// Root is 4 levels up — should NOT be found.
	sd := &ScriptDiscovery{config: &ScriptDiscoveryConfig{
		MaxTraversalDepth:  2,
		ScriptPathPatterns: []string{"scripts"},
	}}
	repos := sd.traverseForGitRepos(startDir)
	assert.Empty(t, repos, "with depth 2, root repo (4 levels up) should not be found")

	// With depth 5, traversal reaches root — should find it.
	sd2 := &ScriptDiscovery{config: &ScriptDiscoveryConfig{
		MaxTraversalDepth:  5,
		ScriptPathPatterns: []string{"scripts"},
	}}
	repos2 := sd2.traverseForGitRepos(startDir)
	assert.NotEmpty(t, repos2, "with depth 5, root repo should be found")
}

// TestTraverseForGitRepos_EmptyDir exercises traversal of an empty directory.
func TestTraverseForGitRepos_EmptyDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	sd := &ScriptDiscovery{config: &ScriptDiscoveryConfig{}}
	repos := sd.traverseForGitRepos(root)
	assert.Empty(t, repos, "empty directory should yield no repos")
}

// ==========================================================================
// goal_discovery.go — traverseForGoalDirs (74.4%)
// ==========================================================================

// TestTraverseForGoalDirs_FindsGoalDirs exercises finding goal directories.
// traverseForGoalDirs walks UPWARD from startDir, checking for subdirs
// matching GoalPathPatterns at each level.
func TestTraverseForGoalDirs_FindsGoalDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create a "goals" subdirectory (matches the pattern) with a JSON file.
	goalDir := filepath.Join(root, "goals")
	require.NoError(t, os.MkdirAll(goalDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(goalDir, "review.json"), []byte(`{"name":"review"}`), 0644))

	// Create a directory without goal files — should not appear.
	emptyDir := filepath.Join(root, "empty-dir")
	require.NoError(t, os.MkdirAll(emptyDir, 0755))

	gd := &GoalDiscovery{config: &GoalDiscoveryConfig{
		GoalPathPatterns:  []string{"goals"},
		MaxTraversalDepth: 3,
	}}
	// Start from root; first iteration checks root and finds root/goals.
	dirs := gd.traverseForGoalDirs(root)

	found := false
	for _, d := range dirs {
		if d == goalDir {
			found = true
		}
	}
	assert.True(t, found, "should find the goals directory matching GoalPathPatterns")
}

// TestTraverseForGoalDirs_DepthLimit exercises depth limiting.
// traverseForGoalDirs walks UPWARD from startDir, so depth limits how
// many parent directories are checked for goal subdirectories.
func TestTraverseForGoalDirs_DepthLimit(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create a "goals" dir at root with a JSON file.
	goalDir := filepath.Join(root, "goals")
	require.NoError(t, os.MkdirAll(goalDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(goalDir, "review.json"), []byte(`{"name":"review"}`), 0644))

	// Start deep — root is 4 levels up from start.
	startDir := filepath.Join(root, "a", "b", "c", "d")
	require.NoError(t, os.MkdirAll(startDir, 0755))

	// With depth 2, traversal visits startDir and one parent (2 levels).
	// Root's goals dir (4 levels up) should NOT be found.
	gd := &GoalDiscovery{config: &GoalDiscoveryConfig{
		GoalPathPatterns:  []string{"goals"},
		MaxTraversalDepth: 2,
	}}
	dirs := gd.traverseForGoalDirs(startDir)
	for _, d := range dirs {
		assert.NotEqual(t, goalDir, d, "with depth 2, root goals dir should not be found")
	}

	// With depth 5, traversal reaches root — should find it.
	gd2 := &GoalDiscovery{config: &GoalDiscoveryConfig{
		GoalPathPatterns:  []string{"goals"},
		MaxTraversalDepth: 5,
	}}
	dirs2 := gd2.traverseForGoalDirs(startDir)
	found := false
	for _, d := range dirs2 {
		if d == goalDir {
			found = true
		}
	}
	assert.True(t, found, "with depth 5, root goals dir should be found")
}

// ==========================================================================
// goal_discovery.go — DiscoverPromptFilePaths (74.4%)
// ==========================================================================

// TestDiscoverPromptFilePaths exercises the prompt file path discovery.
// DiscoverPromptFilePaths returns directory paths (not individual files).
func TestDiscoverPromptFilePaths(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create some files so the directory is non-empty.
	require.NoError(t, os.WriteFile(filepath.Join(root, "test.prompt"), []byte("prompt content"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "readme.md"), []byte("# README"), 0644))

	gd := &GoalDiscovery{
		config: &GoalDiscoveryConfig{
			PromptFilePaths:      []string{root},
			DisableStandardPaths: true, // Avoid CWD .github/prompts lookup.
		},
	}

	paths := gd.DiscoverPromptFilePaths()

	// DiscoverPromptFilePaths returns directories, not individual files.
	realRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)

	found := false
	for _, p := range paths {
		realP, _ := filepath.EvalSymlinks(p)
		if realP == realRoot {
			found = true
		}
	}
	assert.True(t, found, "should include the configured prompt directory")
}

// ==========================================================================
// goal_registry.go — NewDynamicGoalRegistry (75%)
// ==========================================================================

// TestNewDynamicGoalRegistry_Basic exercises the constructor.
func TestNewDynamicGoalRegistry_Basic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	goalDir := filepath.Join(root, "goals")
	require.NoError(t, os.MkdirAll(goalDir, 0755))

	gd := &GoalDiscovery{
		config: &GoalDiscoveryConfig{
			CustomPaths: []string{goalDir},
		},
	}

	registry := NewDynamicGoalRegistry(nil, gd)
	assert.NotNil(t, registry)
}
