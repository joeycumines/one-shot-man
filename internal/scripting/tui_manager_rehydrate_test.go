package scripting

import (
	"bytes"
	"context"

	// storage package intentionally not used to avoid clearing global store
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestRehydrateNormalizesForwardSlashes ensures rehydrateContextManager will
// accept stored labels using '/' separators and correctly resolve them on the
// host OS (critical on Windows where separators differ).
func TestRehydrateNormalizesForwardSlashes(t *testing.T) {
	// Use a unique session ID and in-memory backend instance; avoid
	// modifying global test store to prevent races with parallel tests.

	tmpDir := t.TempDir()

	// create nested file: <tmpDir>/dir/file.txt
	nested := filepath.Join(tmpDir, "dir", "file.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(nested, []byte("hello"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Create engine but replace its ContextManager base path so AddPath resolves
	// relative labels against our tmpDir.
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("rehyd", t.Name())
	engine, err := NewEngineWithConfig(ctx, &stdout, &stderr, sessionID, "memory")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to new context manager: %v", err)
	}
	engine.contextManager = cm

	// Create a TUI manager using the same engine and a memory backend state manager.
	tm := NewTUIManagerWithConfig(ctx, engine, nil, &stderr, sessionID, "memory")

	// Simulate stored state containing a file label using forward slashes
	// (e.g., from txtar or older snapshots).
	item := map[string]interface{}{"type": "file", "label": filepath.ToSlash(filepath.Join("dir", "file.txt"))}
	tm.stateManager.SetState("contextItems", []interface{}{item})

	total, restored := tm.rehydrateContextManager()
	if restored != 1 {
		t.Fatalf("expected restored == 1; got total=%d restored=%d", total, restored)
	}

	// Confirm ContextManager has the canonical owner path key (relative to base).
	paths := engine.contextManager.ListPaths()
	if len(paths) == 0 {
		t.Fatalf("expected manager to contain paths after rehydration")
	}

	// The canonical owner key used by ContextManager is the OS-native
	// filepath.Join form. Regardless of the stored label format, after
	// rehydration we expect the persisted label to be normalized to this
	// native form.
	expected := filepath.Join("dir", "file.txt")
	found := false
	for _, p := range paths {
		if p == expected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected path %q in manager paths: %+v", expected, paths)
	}

	// Verify the TUI state was updated to match the normalized backend label
	finalStateRaw, ok := tm.stateManager.GetState("contextItems")
	if !ok {
		t.Fatalf("expected contextItems to be present in state")
	}

	finalState, ok := finalStateRaw.([]interface{})
	if !ok || len(finalState) != 1 {
		t.Fatalf("expected 1 item in contextItems state, got: %#v", finalStateRaw)
	}

	itemMap, ok := finalState[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected context item to be a map, got: %#v", finalState[0])
	}

	if lbl, _ := itemMap["label"].(string); lbl != expected {
		t.Fatalf("TUI state mismatch: expected label %q, got %q", expected, lbl)
	}
}

// TestRehydrateNormalizesBackslashes verifies that rehydration handles
// stored labels that contain Windows-style backslashes when run on other
// platforms. This guards against sessions created on Windows becoming
// unusable when rehydrated on POSIX systems.
func TestRehydrateNormalizesBackslashes(t *testing.T) {
	// Use a unique session ID and in-memory backend instance; avoid
	// modifying global test store to prevent races with parallel tests.

	tmpDir := t.TempDir()

	// create nested file: <tmpDir>/dir/file.txt
	nested := filepath.Join(tmpDir, "dir", "file.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(nested, []byte("hello"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("rehyd-bslash", t.Name())
	engine, err := NewEngineWithConfig(ctx, &stdout, &stderr, sessionID, "memory")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to new context manager: %v", err)
	}
	engine.contextManager = cm

	tm := NewTUIManagerWithConfig(ctx, engine, nil, &stderr, sessionID, "memory")

	// Simulate stored state containing a file label using backslashes
	// (e.g., a Windows snapshot).
	item := map[string]interface{}{"type": "file", "label": "dir\\file.txt"}
	tm.stateManager.SetState("contextItems", []interface{}{item})

	total, restored := tm.rehydrateContextManager()
	if restored != 1 {
		t.Fatalf("expected restored == 1; got total=%d restored=%d", total, restored)
	}

	paths := engine.contextManager.ListPaths()
	if len(paths) == 0 {
		t.Fatalf("expected manager to contain paths after rehydration")
	}

	expected := filepath.Join("dir", "file.txt")
	found := false
	for _, p := range paths {
		if p == expected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected path %q in manager paths: %+v", expected, paths)
	}

	// Verify the TUI state was updated to match the normalized backend label
	finalStateRaw, ok := tm.stateManager.GetState("contextItems")
	if !ok {
		t.Fatalf("expected contextItems to be present in state")
	}

	finalState, ok := finalStateRaw.([]interface{})
	if !ok || len(finalState) != 1 {
		t.Fatalf("expected 1 item in contextItems state, got: %#v", finalStateRaw)
	}

	itemMap, ok := finalState[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected context item to be a map, got: %#v", finalState[0])
	}

	// On Windows the initial backslash label may already match the OS-native
	// expected value. In all cases we expect the persisted label to equal the
	// canonical owner key reported by the ContextManager (filepath.Join form).
	if lbl, _ := itemMap["label"].(string); lbl != expected {
		t.Fatalf("TUI state mismatch: expected label %q, got %q", expected, lbl)
	}
}

// TestRehydrateNormalizesDotPrefix verifies that labels like "./file.txt"
// are normalized to the canonical owner key (relative path) and persisted
// back into the TUI state so the label matches the backend key.
func TestRehydrateNormalizesDotPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	nested := filepath.Join(tmpDir, "config.txt")
	if err := os.WriteFile(nested, []byte("hello"), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("rehyd-dot", t.Name())
	engine, err := NewEngineWithConfig(ctx, &stdout, &stderr, sessionID, "memory")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	cm, err := NewContextManager(tmpDir)
	if err != nil {
		t.Fatalf("failed to new context manager: %v", err)
	}
	engine.contextManager = cm

	tm := NewTUIManagerWithConfig(ctx, engine, nil, &stderr, sessionID, "memory")

	// Simulate stored state containing a dot-prefixed label.
	item := map[string]interface{}{"type": "file", "label": "./config.txt"}
	tm.stateManager.SetState("contextItems", []interface{}{item})

	total, restored := tm.rehydrateContextManager()
	if restored != 1 {
		t.Fatalf("expected restored == 1; got total=%d restored=%d", total, restored)
	}

	expected := "config.txt"

	// Confirm ContextManager has the canonical owner path key
	paths := engine.contextManager.ListPaths()
	found := false
	for _, p := range paths {
		if p == expected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected path %q in manager paths: %+v", expected, paths)
	}

	// And confirm TUI persisted the normalized label
	finalStateRaw, ok := tm.stateManager.GetState("contextItems")
	if !ok {
		t.Fatalf("expected contextItems to be present in state")
	}
	finalState, ok := finalStateRaw.([]interface{})
	if !ok || len(finalState) != 1 {
		t.Fatalf("expected 1 item in contextItems state, got: %#v", finalStateRaw)
	}
	itemMap, ok := finalState[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected context item to be a map, got: %#v", finalState[0])
	}
	if lbl, _ := itemMap["label"].(string); lbl != expected {
		t.Fatalf("TUI state mismatch: expected label %q, got %q", expected, lbl)
	}
}
