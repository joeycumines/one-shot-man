package scripting

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestContextManagerRehydration verifies that the ContextManager is correctly
// re-hydrated from persisted state after a session restart.
func TestContextManagerRehydration(t *testing.T) {
	t.Setenv("OSM_SESSION", testutil.NewTestSessionID("ctxrehyd", t.Name()))
	t.Setenv("OSM_STORE", "memory")

	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	// Create test files
	testFile1 := filepath.Join(tmpDir, "test1.txt")
	testFile2 := filepath.Join(tmpDir, "test2.txt")
	if err := os.WriteFile(testFile1, []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile2, []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create engine and TUI manager
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := NewEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Create StateManager
	backend := &mockBackend{session: nil}
	stateManager, err := NewStateManager(backend, "test-session")
	if err != nil {
		t.Fatalf("Failed to create StateManager: %v", err)
	}
	defer stateManager.Close()

	output := &testOutput{}
	tm := &TUIManager{
		engine:       engine,
		output:       output,
		stateManager: stateManager,
	}

	// Build the items array in the format expected by ctxutil
	items := []map[string]interface{}{
		{
			"id":      float64(1),
			"type":    "file",
			"label":   testFile1,
			"payload": "",
		},
		{
			"id":      float64(2),
			"type":    "file",
			"label":   testFile2,
			"payload": "",
		},
		{
			"id":      float64(3),
			"type":    "note",
			"label":   "test note",
			"payload": "note content",
		},
	}

	// Set contextItems in the shared state (using the string key "contextItems")
	stateManager.SetState("contextItems", items)

	// Initially, the ContextManager should be empty
	initialPaths := engine.contextManager.ListPaths()
	if len(initialPaths) != 0 {
		t.Errorf("Expected empty ContextManager initially, got %d paths", len(initialPaths))
	}

	// Call rehydrateContextManager (no args in new architecture)
	tm.rehydrateContextManager()

	// Verify that the ContextManager now contains the file paths
	paths := engine.contextManager.ListPaths()

	// Should have 2 paths (the 2 file items, note is excluded)
	if len(paths) != 2 {
		t.Errorf("Expected 2 paths after rehydration, got %d", len(paths))
	}

	// Verify specific files are present
	pathMap := make(map[string]bool)
	for _, p := range paths {
		pathMap[p] = true
	}

	if !pathMap[testFile1] {
		t.Errorf("Expected path %s in ContextManager", testFile1)
	}

	if !pathMap[testFile2] {
		t.Errorf("Expected path %s in ContextManager", testFile2)
	}

	// Verify that remove works (critical test)
	if err := engine.contextManager.RemovePath(testFile1); err != nil {
		t.Errorf("RemovePath failed after rehydration: %v", err)
	}

	// Verify removal
	pathsAfterRemove := engine.contextManager.ListPaths()
	if len(pathsAfterRemove) != 1 {
		t.Errorf("Expected 1 path after removal, got %d", len(pathsAfterRemove))
	}

	if pathsAfterRemove[0] != testFile2 {
		t.Errorf("Expected remaining path to be %s, got %s", testFile2, pathsAfterRemove[0])
	}

	// Verify that toTxtar works
	archive := engine.contextManager.ToTxtar()
	if len(archive.Files) != 1 {
		t.Errorf("Expected 1 file in txtar archive, got %d", len(archive.Files))
	}

	if archive.Files[0].Name != "test2.txt" {
		t.Errorf("Expected file name 'test2.txt', got '%s'", archive.Files[0].Name)
	}
}

// TestContextManagerRehydrationWithMissingFiles verifies graceful handling of missing files
func TestContextManagerRehydrationWithMissingFiles(t *testing.T) {
	t.Setenv("OSM_SESSION", testutil.NewTestSessionID("ctxrehydmissfile", t.Name()))
	t.Setenv("OSM_STORE", "memory")

	tmpDir := t.TempDir()

	// Create only one test file, reference a non-existent one
	testFile1 := filepath.Join(tmpDir, "exists.txt")
	testFile2 := filepath.Join(tmpDir, "missing.txt")
	if err := os.WriteFile(testFile1, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := NewEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Create StateManager
	backend := &mockBackend{session: nil}
	stateManager, err := NewStateManager(backend, "test-session-missing")
	if err != nil {
		t.Fatalf("Failed to create StateManager: %v", err)
	}
	defer stateManager.Close()

	output := &testOutput{}
	tm := &TUIManager{
		engine:       engine,
		output:       output,
		stateManager: stateManager,
	}

	items := []map[string]interface{}{
		{
			"id":      float64(1),
			"type":    "file",
			"label":   testFile1,
			"payload": "",
		},
		{
			"id":      float64(2),
			"type":    "file",
			"label":   testFile2, // This file doesn't exist
			"payload": "",
		},
	}

	// Set contextItems in shared state
	stateManager.SetState("contextItems", items)

	// Call rehydrateContextManager (no args in new architecture)
	tm.rehydrateContextManager()

	// Should only have 1 path (the missing file should be skipped)
	paths := engine.contextManager.ListPaths()
	if len(paths) != 1 {
		t.Errorf("Expected 1 path after rehydration (missing file excluded), got %d", len(paths))
	}

	if paths[0] != testFile1 {
		t.Errorf("Expected path %s, got %s", testFile1, paths[0])
	}

	// Check that an informational message was logged
	outputStr := output.String()
	if outputStr != "" && len(outputStr) > 0 {
		// Message should mention the missing file
		// (exact format checked via integration test, here we just verify no crash)
		t.Logf("Output (expected info about missing file): %s", outputStr)
	}
}

// TestContextManagerRehydrationNoItemsKey verifies graceful handling when no items key exists
func TestContextManagerRehydrationNoItemsKey(t *testing.T) {
	t.Setenv("OSM_SESSION", testutil.NewTestSessionID("noitem", t.Name()))
	t.Setenv("OSM_STORE", "memory")

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := NewEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()

	// Create StateManager
	backend := &mockBackend{session: nil}
	stateManager, err := NewStateManager(backend, "test-session-no-items")
	if err != nil {
		t.Fatalf("Failed to create StateManager: %v", err)
	}
	defer stateManager.Close()

	output := &testOutput{}
	tm := &TUIManager{
		engine:       engine,
		output:       output,
		stateManager: stateManager,
	}

	// Don't set contextItems - leave state empty

	// Call rehydrateContextManager - should be a no-op (no args in new architecture)
	tm.rehydrateContextManager()

	// ContextManager should remain empty
	paths := engine.contextManager.ListPaths()
	if len(paths) != 0 {
		t.Errorf("Expected empty ContextManager (no items key), got %d paths", len(paths))
	}

	// Should produce no error output
	if output.String() != "" {
		t.Errorf("Expected no output, got: %s", output.String())
	}
}

// testOutput is a simple io.Writer for capturing test output
type testOutput struct {
	data []byte
}

func (o *testOutput) Write(p []byte) (n int, err error) {
	o.data = append(o.data, p...)
	return len(p), nil
}

func (o *testOutput) String() string {
	return string(o.data)
}

func (o *testOutput) Printf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	o.Write([]byte(msg))
}
