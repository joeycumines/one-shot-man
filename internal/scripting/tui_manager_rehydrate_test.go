package scripting

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "testing"
    "github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

// TestRehydrateNormalizesForwardSlashes ensures rehydrateContextManager will
// accept stored labels using '/' separators and correctly resolve them on the
// host OS (critical on Windows where separators differ).
func TestRehydrateNormalizesForwardSlashes(t *testing.T) {
    storage.ClearAllInMemorySessions()
    defer storage.ClearAllInMemorySessions()

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
    engine, err := NewEngineWithConfig(ctx, &stdout, &stderr, "rehyd-test", "memory")
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
    tm := NewTUIManagerWithConfig(ctx, engine, nil, &stderr, "rehyd-test", "memory")

    // Simulate stored state containing a file label using forward slashes
    // (e.g., from txtar or older snapshots).
    item := map[string]interface{}{"type": "file", "label": filepath.ToSlash(filepath.Join("dir", "file.txt"))}
    tm.stateManager.SetState("contextItems", []interface{}{item})

    total, restored := tm.rehydrateContextManager()
    if restored != 1 {
        t.Fatalf("expected restored == 1; got total=%d restored=%d", total, restored)
    }

    // Confirm ContextManager has the normalized path key (should be relative owner "dir/file.txt" or OS style)
    paths := engine.contextManager.ListPaths()
    if len(paths) == 0 {
        t.Fatalf("expected manager to contain paths after rehydration")
    }
}
