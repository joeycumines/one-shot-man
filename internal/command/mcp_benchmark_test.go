package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// benchSink prevents compiler optimisation of benchmark results.
var benchSink any

// mcpBenchEnv wraps an MCP test environment for benchmarks.
type mcpBenchEnv struct {
	session *mcp.ClientSession
	dir     string
	cancel  context.CancelFunc
}

// newMCPBenchEnv creates an MCP server+client pair via InMemoryTransport for
// use in benchmarks. Mirrors newMCPTestEnv but accepts *testing.B.
func newMCPBenchEnv(b *testing.B, goals []Goal) *mcpBenchEnv {
	b.Helper()
	dir := b.TempDir()

	cm, err := scripting.NewContextManager(dir)
	if err != nil {
		b.Fatalf("NewContextManager: %v", err)
	}

	goalRegistry := &mcpTestGoalRegistry{goals: goals}
	server := newMCPServer(cm, goalRegistry, "bench")

	ctx, cancel := context.WithCancel(context.Background())

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "bench-client", Version: "bench"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		b.Fatalf("client.Connect: %v", err)
	}

	b.Cleanup(func() {
		_ = session.Close()
		cancel()
		select {
		case <-serverDone:
		case <-time.After(5 * time.Second):
			b.Error("server did not shut down within 5s")
		}
	})

	return &mcpBenchEnv{session: session, dir: dir, cancel: cancel}
}

func (e *mcpBenchEnv) callTool(b *testing.B, name string, args map[string]any) *mcp.CallToolResult {
	b.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := e.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		b.Fatalf("CallTool(%q): %v", name, err)
	}
	return result
}

// ---------------------------------------------------------------------------
// BenchmarkMCPBuildPrompt — measures the cost of assembling the final prompt
// at various context sizes.
// ---------------------------------------------------------------------------

func BenchmarkMCPBuildPrompt(b *testing.B) {
	b.Run("Empty", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "buildPrompt", nil)
		}
	})

	b.Run("5Files", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		for i := 0; i < 5; i++ {
			f := filepath.Join(env.dir, fmt.Sprintf("file%d.go", i))
			content := fmt.Sprintf("package f%d\n\nimport \"fmt\"\n\nfunc Hello%d() {\n\tfmt.Println(%q)\n}\n", i, i, "hello")
			if err := os.WriteFile(f, []byte(content), 0644); err != nil {
				b.Fatal(err)
			}
			r := env.callTool(b, "addFile", map[string]any{"path": f})
			if r.IsError {
				b.Fatal("addFile: error")
			}
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "buildPrompt", nil)
		}
	})

	b.Run("20FilesWithNotes", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		for i := 0; i < 20; i++ {
			f := filepath.Join(env.dir, fmt.Sprintf("src%d.go", i))
			content := fmt.Sprintf("package src%d\n\nimport \"fmt\"\n\nfunc Do%d() {\n\tfmt.Println(%d)\n}\n", i, i, i)
			if err := os.WriteFile(f, []byte(content), 0644); err != nil {
				b.Fatal(err)
			}
			r := env.callTool(b, "addFile", map[string]any{"path": f})
			if r.IsError {
				b.Fatal("addFile: error")
			}
		}
		for i := 0; i < 5; i++ {
			env.callTool(b, "addNote", map[string]any{
				"text":  fmt.Sprintf("Review note %d: check error handling in src%d.go", i, i),
				"label": fmt.Sprintf("note-%d", i),
			})
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "buildPrompt", nil)
		}
	})

	b.Run("WithDiffsAndNotes", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		// 5 files
		for i := 0; i < 5; i++ {
			f := filepath.Join(env.dir, fmt.Sprintf("mod%d.go", i))
			if err := os.WriteFile(f, []byte(fmt.Sprintf("package mod%d\n", i)), 0644); err != nil {
				b.Fatal(err)
			}
			env.callTool(b, "addFile", map[string]any{"path": f})
		}
		// 5 diffs
		for i := 0; i < 5; i++ {
			env.callTool(b, "addDiff", map[string]any{
				"diff":  fmt.Sprintf("--- a/mod%d.go\n+++ b/mod%d.go\n@@ -1 +1,3 @@\n-package mod%d\n+package mod%d\n+\n+func New%d() {}\n", i, i, i, i, i),
				"label": fmt.Sprintf("diff-%d", i),
			})
		}
		// 5 notes
		for i := 0; i < 5; i++ {
			env.callTool(b, "addNote", map[string]any{
				"text":  fmt.Sprintf("Added New%d constructor to mod%d — verify initialisation logic", i, i),
				"label": fmt.Sprintf("note-%d", i),
			})
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "buildPrompt", nil)
		}
	})
}

// ---------------------------------------------------------------------------
// BenchmarkMCPToolLatency — individual MCP tool round-trip cost.
// ---------------------------------------------------------------------------

func BenchmarkMCPToolLatency(b *testing.B) {
	b.Run("AddFile", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		f := filepath.Join(env.dir, "bench.go")
		if err := os.WriteFile(f, []byte("package bench\n\nfunc Noop() {}\n"), 0644); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		// Re-adding the same file is consistent: handler stat+read+replace.
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "addFile", map[string]any{"path": f})
		}
	})

	b.Run("AddNote", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "addNote", map[string]any{
				"text":  "benchmark note content — review this carefully",
				"label": "bench-note",
			})
		}
	})

	b.Run("AddDiff", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "addDiff", map[string]any{
				"diff":  "--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n",
				"label": "bench-diff",
			})
		}
	})

	b.Run("ListContext", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		// Populate context so listContext has real work.
		for i := 0; i < 10; i++ {
			env.callTool(b, "addNote", map[string]any{
				"text":  fmt.Sprintf("note %d with some content", i),
				"label": fmt.Sprintf("n%d", i),
			})
		}
		for i := 0; i < 3; i++ {
			f := filepath.Join(env.dir, fmt.Sprintf("ctx%d.txt", i))
			os.WriteFile(f, []byte("content"), 0644)
			env.callTool(b, "addFile", map[string]any{"path": f})
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "listContext", nil)
		}
	})

	b.Run("ClearContext", func(b *testing.B) {
		b.ReportAllocs()
		env := newMCPBenchEnv(b, nil)
		// Seed some context; first clear drains it, subsequent clears are no-ops
		// but still exercise the full MCP round-trip + mutex.
		for i := 0; i < 5; i++ {
			env.callTool(b, "addNote", map[string]any{
				"text":  fmt.Sprintf("temp note %d", i),
				"label": fmt.Sprintf("tmp-%d", i),
			})
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			benchSink = env.callTool(b, "clearContext", nil)
		}
	})
}

// ---------------------------------------------------------------------------
// BenchmarkMCPBacktickFence — pure-function benchmark of mcpBacktickFence.
// Safe for b.RunParallel (no shared mutable state).
// ---------------------------------------------------------------------------

func BenchmarkMCPBacktickFence(b *testing.B) {
	b.Run("NoBackticks", func(b *testing.B) {
		b.ReportAllocs()
		content := "hello world, this is some content without any backticks at all. " +
			"It contains regular prose and nothing special."
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = mcpBacktickFence(content)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("TripleBackticks", func(b *testing.B) {
		b.ReportAllocs()
		content := "```go\nfmt.Println(\"hello\")\n```\nSome surrounding text."
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = mcpBacktickFence(content)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("DeepNesting", func(b *testing.B) {
		b.ReportAllocs()
		// 100 consecutive backticks — fence must be 101 backticks.
		content := strings.Repeat("`", 100) + "\nsome code\n" + strings.Repeat("`", 100)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = mcpBacktickFence(content)
			}
			runtime.KeepAlive(r)
		})
	})

	b.Run("LargeContent", func(b *testing.B) {
		b.ReportAllocs()
		// ~100KB content with periodic backtick runs.
		var sb strings.Builder
		for sb.Len() < 100*1024 {
			sb.WriteString("Some regular content without special characters. ")
			sb.WriteString("Here is an inline ```go\nfmt.Println()\n``` block. ")
			sb.WriteString("And more prose to fill the buffer. ")
		}
		content := sb.String()
		b.SetBytes(int64(len(content)))
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var r string
			for pb.Next() {
				r = mcpBacktickFence(content)
			}
			runtime.KeepAlive(r)
		})
	})
}
