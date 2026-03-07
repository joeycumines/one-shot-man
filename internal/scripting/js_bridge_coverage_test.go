package scripting

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// js_context_api.go — direct Go-level unit tests for JS bridge wrappers
// ============================================================================

func TestJsContextAddPath(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "sample.txt")
	if err := os.WriteFile(f, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	err := eng.jsContextAddPath(f)
	if err != nil {
		t.Fatalf("jsContextAddPath(%q): %v", f, err)
	}

	paths := eng.jsContextListPaths()
	found := false
	for _, p := range paths {
		if p == f {
			found = true
		}
	}
	if !found {
		t.Errorf("expected %q in ListPaths, got %v", f, paths)
	}
}

func TestJsContextRemovePath(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "remove-me.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := eng.jsContextAddPath(f); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := eng.jsContextRemovePath(f); err != nil {
		t.Fatalf("remove: %v", err)
	}

	paths := eng.jsContextListPaths()
	for _, p := range paths {
		if p == f {
			t.Errorf("expected %q to be removed, but still in ListPaths", f)
		}
	}
}

func TestJsContextRefreshPath(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "refresh.txt")
	if err := os.WriteFile(f, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := eng.jsContextAddPath(f); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Overwrite with new content
	if err := os.WriteFile(f, []byte("v2-updated"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := eng.jsContextRefreshPath(f); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	// Verify the refreshed content is reflected in txtar output
	txtar := eng.jsContextToTxtar()
	if !bytes.Contains([]byte(txtar), []byte("v2-updated")) {
		t.Errorf("expected txtar to contain 'v2-updated' after refresh, got: %s", txtar)
	}
}

func TestJsContextToTxtar(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "txtar-src.txt")
	if err := os.WriteFile(f, []byte("content for txtar"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := eng.jsContextAddPath(f); err != nil {
		t.Fatalf("add: %v", err)
	}

	result := eng.jsContextToTxtar()
	if result == "" {
		t.Error("expected non-empty txtar output")
	}
	if !bytes.Contains([]byte(result), []byte("content for txtar")) {
		t.Errorf("expected txtar to contain file content, got: %s", result)
	}
}

func TestJsContextGetStats(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "stats.txt")
	if err := os.WriteFile(f, []byte("123456789"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := eng.jsContextAddPath(f); err != nil {
		t.Fatalf("add: %v", err)
	}

	stats := eng.jsContextGetStats()
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	// Verify expected keys exist
	for _, key := range []string{"files", "totalSize", "totalPaths"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats missing expected key %q, got keys: %v", key, stats)
		}
	}
}

func TestJsContextFilterPaths(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	// Use txtar to add paths with short names (no directory separators)
	// since filepath.Match("*.go", "/full/path/main.go") won't match.
	txtarData := "-- main.go --\npackage main\n-- notes.txt --\nnotes\n"
	if err := eng.jsContextFromTxtar(txtarData); err != nil {
		t.Fatalf("fromTxtar: %v", err)
	}

	result, err := eng.jsContextFilterPaths("*.go")
	if err != nil {
		t.Fatalf("filter: %v", err)
	}

	foundGo := false
	for _, p := range result {
		if p == "main.go" {
			foundGo = true
		}
		if p == "notes.txt" {
			t.Error("*.go filter should not include .txt files")
		}
	}
	if !foundGo {
		t.Errorf("expected *.go filter to include main.go, got: %v", result)
	}
}

func TestJsContextGetFilesByExtension(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "app.go")
	mdFile := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(goFile, []byte("package app"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mdFile, []byte("# Readme"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := eng.jsContextAddPath(goFile); err != nil {
		t.Fatalf("add go: %v", err)
	}
	if err := eng.jsContextAddPath(mdFile); err != nil {
		t.Fatalf("add md: %v", err)
	}

	goFiles := eng.jsContextGetFilesByExtension(".go")
	foundGo := false
	for _, p := range goFiles {
		if filepath.Base(p) == "app.go" {
			foundGo = true
		}
		if filepath.Base(p) == "README.md" {
			t.Error(".go extension filter should not include .md files")
		}
	}
	if !foundGo {
		t.Errorf("expected .go extension to include app.go, got: %v", goFiles)
	}
}

// ============================================================================
// js_output_api.go — direct Go-level unit tests
// ============================================================================

func TestJsOutputPrint(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	eng := mustNewEngine(t, ctx, &stdout, &stderr)

	eng.jsOutputPrint("hello from Go test")

	// PrintToTUI writes to tuiWriter (stdout) when no TUI sink is set.
	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("hello from Go test")) {
		t.Errorf("expected stdout to contain 'hello from Go test', got %q", out)
	}
}

func TestJsOutputPrintf(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	eng := mustNewEngine(t, ctx, &stdout, &stderr)

	eng.jsOutputPrintf("value=%d, name=%s", 42, "test")

	// PrintfToTUI writes to tuiWriter (stdout) when no TUI sink is set.
	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("value=42, name=test")) {
		t.Errorf("expected stdout to contain 'value=42, name=test', got %q", out)
	}
}

// ============================================================================
// js_logging_api.go — direct Go-level unit tests
// ============================================================================

func TestJsLogWarn(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	eng.jsLogWarn("warning message", map[string]any{
		"severity": "high",
	})

	// Verify the log was recorded
	logs := eng.jsGetLogs()
	if logs == nil {
		t.Fatal("expected logs after jsLogWarn")
	}
}

func TestJsLogError(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	eng.jsLogError("error message", map[string]any{
		"code": "E001",
	})

	logs := eng.jsGetLogs()
	if logs == nil {
		t.Fatal("expected logs after jsLogError")
	}
}

func TestJsLogPrintf(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	eng.jsLogPrintf("formatted: %d + %d = %d", 1, 2, 3)

	logs := eng.jsGetLogs()
	if logs == nil {
		t.Fatal("expected logs after jsLogPrintf")
	}
}

func TestJsLogSearch(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	eng.jsLogInfo("alpha message")
	eng.jsLogWarn("beta warning")
	eng.jsLogError("gamma error")

	result := eng.jsLogSearch("beta")
	if result == nil {
		t.Fatal("expected non-nil search result for 'beta'")
	}
}

func TestJsGetLogs_WithCount(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	// Log 5 messages
	for i := 0; i < 5; i++ {
		eng.jsLogInfo("msg")
	}

	// Request only 2
	result := eng.jsGetLogs(2)
	if result == nil {
		t.Fatal("expected non-nil result for jsGetLogs(2)")
	}

	switch v := result.(type) {
	case []logEntry:
		if len(v) > 2 {
			t.Errorf("requested 2 logs but got %d", len(v))
		}
	default:
		t.Errorf("expected []logEntry, got %T", result)
	}
}

func TestJsGetLogs_ZeroCount(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)

	eng.jsLogInfo("test")

	// Zero count should fall through to GetLogs (all logs)
	result := eng.jsGetLogs(0)
	if result == nil {
		t.Fatal("expected non-nil result for jsGetLogs(0)")
	}
}
