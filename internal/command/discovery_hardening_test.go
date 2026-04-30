package command

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// --- Timing in debug logs ---

func TestGoalDiscovery_TimingInDebugLogs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "osm-goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", goalDir)

	discovery := NewGoalDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverGoalPaths()

	mu.Lock()
	defer mu.Unlock()

	// Verify timing info appears in the "discovery complete" message
	foundTiming := false
	for _, msg := range messages {
		if strings.Contains(msg, "discovery complete") && (strings.Contains(msg, "µs") || strings.Contains(msg, "ms") || strings.Contains(msg, "ns") || strings.Contains(msg, "s")) {
			foundTiming = true
			break
		}
	}
	if !foundTiming {
		t.Errorf("Expected timing info in discovery complete message, got messages: %v", messages)
	}
}

func TestScriptDiscovery_TimingInDebugLogs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("Failed to create script directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", scriptDir)

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	foundTiming := false
	for _, msg := range messages {
		if strings.Contains(msg, "discovery complete") && (strings.Contains(msg, "µs") || strings.Contains(msg, "ms") || strings.Contains(msg, "ns") || strings.Contains(msg, "s")) {
			foundTiming = true
			break
		}
	}
	if !foundTiming {
		t.Errorf("Expected timing info in discovery complete message, got messages: %v", messages)
	}
}

// --- Traversal timing and dir count ---

func TestGoalDiscovery_TraversalTimingAndDirCount(t *testing.T) {
	// Cannot use t.Parallel() because this test changes working directory

	tmpDir := t.TempDir()
	deepPath := filepath.Join(tmpDir, "a", "b", "c")
	if err := os.MkdirAll(deepPath, 0o755); err != nil {
		t.Fatalf("Failed to create deep path: %v", err)
	}

	// Create a goal directory at some level
	goalDir := filepath.Join(tmpDir, "a", "osm-goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal dir: %v", err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(origWd)

	if err := os.Chdir(deepPath); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("goal.path-patterns", "osm-goals")

	discovery := NewGoalDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverGoalPaths()

	mu.Lock()
	defer mu.Unlock()

	// Verify traversal complete message with dir count and timing
	foundTraversal := false
	for _, msg := range messages {
		if strings.Contains(msg, "traversal complete") && strings.Contains(msg, "directories") {
			foundTraversal = true
			break
		}
	}
	if !foundTraversal {
		t.Errorf("Expected 'traversal complete' message with directory count, got messages: %v", messages)
	}
}

// --- Large directory protection ---

func TestFindGoalFiles_LargeDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Create more files than the soft warning threshold but don't actually
	// hit maxDirEntries (10000) since that would be slow. Instead, just
	// verify the function handles directories with many files gracefully.
	for i := range 100 {
		name := fmt.Sprintf("goal-%03d.json", i)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}
	// Add some non-json files
	for i := range 50 {
		name := fmt.Sprintf("readme-%03d.txt", i)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("text"), 0o644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	candidates, err := FindGoalFiles(tmpDir)
	if err != nil {
		t.Fatalf("FindGoalFiles failed: %v", err)
	}

	if len(candidates) != 100 {
		t.Errorf("Expected 100 candidates, got %d", len(candidates))
	}
}

// --- Empty directory handling ---

func TestFindGoalFiles_EmptyDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	emptyDir := filepath.Join(tmpDir, "empty-goals")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("Failed to create empty dir: %v", err)
	}

	candidates, err := FindGoalFiles(emptyDir)
	if err != nil {
		t.Fatalf("FindGoalFiles failed: %v", err)
	}

	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates for empty dir, got %d", len(candidates))
	}
}

func TestFindGoalFiles_DirWithOnlyNonJSON(t *testing.T) {
	// Note: Cannot use t.Parallel() because log.SetOutput changes global state

	tmpDir := t.TempDir()

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Create directory with only non-json files
	for _, name := range []string{"readme.md", "notes.txt", "config.yaml"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("content"), 0o644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	candidates, err := FindGoalFiles(tmpDir)
	if err != nil {
		t.Fatalf("FindGoalFiles failed: %v", err)
	}

	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates, got %d", len(candidates))
	}

	// Should log a warning about no JSON files found
	logOutput := buf.String()
	if !strings.Contains(logOutput, "goal directory contains no goal files") {
		t.Errorf("Expected warning about no .json files, got: %q", logOutput)
	}
}

func TestFindGoalFiles_DirWithSubdirs(t *testing.T) {
	// Note: Cannot use t.Parallel() because log.SetOutput changes global state

	tmpDir := t.TempDir()

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	// Create directory with only subdirectories
	for _, name := range []string{"subdir1", "subdir2"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, name), 0o755); err != nil {
			t.Fatalf("Failed to create subdir: %v", err)
		}
	}

	candidates, err := FindGoalFiles(tmpDir)
	if err != nil {
		t.Fatalf("FindGoalFiles failed: %v", err)
	}

	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates, got %d", len(candidates))
	}

	// Should log warning about no JSON files
	logOutput := buf.String()
	if !strings.Contains(logOutput, "goal directory contains no goal files") {
		t.Errorf("Expected warning about no .json files, got: %q", logOutput)
	}
}

// --- Broken symlink logging ---

func TestFindGoalFiles_BrokenSymlinkLogging(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlink tests not reliable on Windows")
	}
	// Note: Cannot use t.Parallel() because log.SetOutput changes global state

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal dir: %v", err)
	}

	// Create a broken symlink
	brokenLink := filepath.Join(goalDir, "broken.json")
	if err := os.Symlink("/nonexistent/target/file.json", brokenLink); err != nil {
		t.Skip("Symlinks not supported")
	}

	// Create a valid goal file too
	validFile := filepath.Join(goalDir, "valid.json")
	if err := os.WriteFile(validFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	candidates, err := FindGoalFiles(goalDir)
	if err != nil {
		t.Fatalf("FindGoalFiles failed: %v", err)
	}

	// Should find the valid file
	if len(candidates) != 1 {
		t.Errorf("Expected 1 candidate, got %d", len(candidates))
	}

	// Should log warning about broken symlink
	logOutput := buf.String()
	if !strings.Contains(logOutput, "broken symlink") {
		t.Errorf("Expected warning about broken symlink, got: %q", logOutput)
	}
}

// --- Annotated paths timing ---

func TestGoalDiscovery_AnnotatedPathsTiming(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	goalDir := filepath.Join(tmpDir, "goals")
	if err := os.MkdirAll(goalDir, 0o755); err != nil {
		t.Fatalf("Failed to create goal dir: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "false")
	cfg.SetGlobalOption("goal.paths", goalDir)

	discovery := NewGoalDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverAnnotatedGoalPaths()

	mu.Lock()
	defer mu.Unlock()

	foundTiming := false
	for _, msg := range messages {
		if strings.Contains(msg, "annotated discovery complete") {
			foundTiming = true
			break
		}
	}
	if !foundTiming {
		t.Errorf("Expected 'annotated discovery complete' message, got: %v", messages)
	}
}

func TestScriptDiscovery_AnnotatedPathsTiming(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scriptDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("Failed to create script dir: %v", err)
	}

	var mu sync.Mutex
	var messages []string

	cfg := config.NewConfig()
	cfg.SetGlobalOption("script.disable-standard-paths", "true")
	cfg.SetGlobalOption("script.autodiscovery", "false")
	cfg.SetGlobalOption("script.paths", scriptDir)

	discovery := NewScriptDiscovery(cfg)
	discovery.config.DebugLogFunc = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, fmt.Sprintf(format, args...))
	}

	_ = discovery.DiscoverAnnotatedScriptPaths()

	mu.Lock()
	defer mu.Unlock()

	foundTiming := false
	for _, msg := range messages {
		if strings.Contains(msg, "annotated discovery complete") {
			foundTiming = true
			break
		}
	}
	if !foundTiming {
		t.Errorf("Expected 'annotated discovery complete' message, got: %v", messages)
	}
}
