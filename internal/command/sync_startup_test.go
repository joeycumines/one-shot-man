package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestSyncAutoPull_Disabled(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	// auto-pull not set — should be a no-op.
	var stderr bytes.Buffer
	SyncAutoPull(cfg, &stderr)
	if stderr.Len() > 0 {
		t.Fatalf("expected no output, got %q", stderr.String())
	}
}

func TestSyncAutoPull_NilConfig(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	SyncAutoPull(nil, &stderr)
	if stderr.Len() > 0 {
		t.Fatalf("expected no output, got %q", stderr.String())
	}
}

func TestSyncAutoPull_EnabledButNotRepo(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.auto-pull", "true")
	cfg.SetGlobalOption("sync.local-path", filepath.Join(t.TempDir(), "not-a-repo"))
	var stderr bytes.Buffer
	SyncAutoPull(cfg, &stderr)
	// No git repo → should silently skip.
	if stderr.Len() > 0 {
		t.Fatalf("expected no output for non-repo, got %q", stderr.String())
	}
}

func TestApplySyncDiscoveryPaths_NilConfig(t *testing.T) {
	t.Parallel()
	// Should not panic.
	ApplySyncDiscoveryPaths(nil)
}

func TestApplySyncDiscoveryPaths_NoSyncDir(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", filepath.Join(t.TempDir(), "nonexistent"))
	ApplySyncDiscoveryPaths(cfg)
	// No paths should be set.
	if val, exists := cfg.GetGlobalOption("goal.paths"); exists && val != "" {
		t.Fatalf("expected no goal.paths, got %q", val)
	}
}

func TestApplySyncDiscoveryPaths_InjectsGoals(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	goalsDir := filepath.Join(root, "goals")
	if err := os.MkdirAll(goalsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", root)
	ApplySyncDiscoveryPaths(cfg)

	val, exists := cfg.GetGlobalOption("goal.paths")
	if !exists || val == "" {
		t.Fatal("expected goal.paths to be set")
	}
	if !strings.Contains(val, goalsDir) {
		t.Fatalf("expected goal.paths to contain %q, got %q", goalsDir, val)
	}
}

func TestApplySyncDiscoveryPaths_InjectsScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", root)
	ApplySyncDiscoveryPaths(cfg)

	// script.paths should contain sync scripts dir.
	val, _ := cfg.GetGlobalOption("script.paths")
	if !strings.Contains(val, scriptsDir) {
		t.Fatalf("expected script.paths to contain %q, got %q", scriptsDir, val)
	}

	// script.module-paths should also contain sync scripts dir.
	val2, _ := cfg.GetGlobalOption("script.module-paths")
	if !strings.Contains(val2, scriptsDir) {
		t.Fatalf("expected script.module-paths to contain %q, got %q", scriptsDir, val2)
	}
}

func TestApplySyncDiscoveryPaths_InjectsBothGoalsAndScripts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "goals"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", root)
	ApplySyncDiscoveryPaths(cfg)

	goalPaths, _ := cfg.GetGlobalOption("goal.paths")
	scriptPaths, _ := cfg.GetGlobalOption("script.paths")
	if goalPaths == "" {
		t.Fatal("expected goal.paths to be set")
	}
	if scriptPaths == "" {
		t.Fatal("expected script.paths to be set")
	}
}

func TestApplySyncDiscoveryPaths_DoesNotDuplicate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	goalsDir := filepath.Join(root, "goals")
	if err := os.MkdirAll(goalsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", root)

	// Apply twice.
	ApplySyncDiscoveryPaths(cfg)
	ApplySyncDiscoveryPaths(cfg)

	val, _ := cfg.GetGlobalOption("goal.paths")
	// Should only appear once.
	count := strings.Count(val, goalsDir)
	if count != 1 {
		t.Fatalf("expected goal.paths to contain %q exactly once, got %d times in %q", goalsDir, count, val)
	}
}

func TestApplySyncDiscoveryPaths_PreservesExisting(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	goalsDir := filepath.Join(root, "goals")
	if err := os.MkdirAll(goalsDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("goal.paths", "/existing/path")

	ApplySyncDiscoveryPaths(cfg)

	val, _ := cfg.GetGlobalOption("goal.paths")
	if !strings.Contains(val, "/existing/path") {
		t.Fatalf("expected existing path to be preserved, got %q", val)
	}
	if !strings.Contains(val, goalsDir) {
		t.Fatalf("expected sync goals dir to be added, got %q", val)
	}
}

func TestSyncRootFromConfig_WithPath(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", "/custom/path")
	got := syncRootFromConfig(cfg)
	if got != "/custom/path" {
		t.Fatalf("expected /custom/path, got %q", got)
	}
}

func TestSyncRootFromConfig_Default(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	got := syncRootFromConfig(cfg)
	if !strings.HasSuffix(got, filepath.Join(".osm", "sync")) {
		t.Fatalf("expected default path, got %q", got)
	}
}

func TestAppendConfigPath_NewKey(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	appendConfigPath(cfg, "test.key", "/new/path")
	val, _ := cfg.GetGlobalOption("test.key")
	if val != "/new/path" {
		t.Fatalf("expected /new/path, got %q", val)
	}
}

func TestAppendConfigPath_AppendToExisting(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("test.key", "/first")
	appendConfigPath(cfg, "test.key", "/second")
	val, _ := cfg.GetGlobalOption("test.key")
	if !strings.Contains(val, "/first") || !strings.Contains(val, "/second") {
		t.Fatalf("expected both paths, got %q", val)
	}
}

func TestAppendConfigPath_NoDuplicate(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	appendConfigPath(cfg, "test.key", "/path")
	appendConfigPath(cfg, "test.key", "/path")
	val, _ := cfg.GetGlobalOption("test.key")
	if val != "/path" {
		t.Fatalf("expected single path, got %q", val)
	}
}

func TestApplySyncDiscoveryPaths_SkipsNonDirGoals(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Create a file called "goals" (not a directory).
	if err := os.WriteFile(filepath.Join(root, "goals"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", root)
	ApplySyncDiscoveryPaths(cfg)

	val, _ := cfg.GetGlobalOption("goal.paths")
	if val != "" {
		t.Fatalf("expected goal.paths to be empty for non-dir goals, got %q", val)
	}
}
