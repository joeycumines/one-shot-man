// Package internal_test contains performance benchmarks and regression tests for one-shot-man.
package internal_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/command"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/session"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// Performance thresholds (in microseconds) for regression detection.
// Thresholds set generously to avoid intermittent failures due to system load variations
// in CI/CD environments across different platforms.
//
// Cross-platform verification (T061, Feb 2026):
//
//	macOS (Apple M-series, 3 runs):
//	  SessionIDGeneration:    0-1 μs   (threshold: 10,000 μs, headroom: >10,000x)
//	  SessionPersistenceWrite: 6-7 μs  (threshold: 10,000 μs, headroom: ~1,400x)
//	  SessionPersistenceRead:  2 μs    (threshold: 5,000 μs,  headroom: ~2,500x)
//	  RuntimeCreation:        81-97 μs (threshold: 100,000 μs, headroom: ~1,000x)
//	  ScriptExecution:        13-16 μs (threshold: 10,000 μs, headroom: ~625x)
//	  ConcurrentAccess:       1 μs     (threshold: 20,000 μs, headroom: ~20,000x)
//
//	Linux (Docker golang:1.25.7, 1 run):
//	  SessionIDGeneration:    25 μs    (threshold: 10,000 μs, headroom: ~400x)
//	  SessionPersistenceWrite: 3 μs    (threshold: 10,000 μs, headroom: ~3,333x)
//	  SessionPersistenceRead:  2 μs    (threshold: 5,000 μs,  headroom: ~2,500x)
//	  RuntimeCreation:        144 μs   (threshold: 100,000 μs, headroom: ~694x)
//	  ScriptExecution:        10 μs    (threshold: 10,000 μs, headroom: ~1,000x)
//	  ConcurrentAccess:       2 μs     (threshold: 20,000 μs, headroom: ~10,000x)
//
//	Benchmark variance (macOS, count=5): <10% for most operations,
//	highest observed ~30% (VMCreation single outlier, sub-μs operation).
//	No platform-specific multipliers needed — minimum headroom is 400x.
const (
	// Session operation thresholds
	thresholdSessionIDGenerationUnix    = 10000
	thresholdSessionIDGenerationWindows = 50000
	thresholdSessionPersistenceWrite    = 10000
	thresholdSessionPersistenceRead     = 5000
	thresholdConcurrentSessionAccess    = 20000

	// Scripting engine thresholds
	thresholdRuntimeCreation  = 100000
	thresholdSimpleScriptExec = 10000
)

// BenchmarkSessionOperations benchmarks session operations.
func BenchmarkSessionOperations(b *testing.B) {
	b.Run("SessionIDGeneration", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := session.GetSessionID("")
			if err != nil {
				b.Fatalf("failed to generate session ID: %v", err)
			}
		}
	})

	b.Run("SessionCreation", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = &storage.Session{
				ID:          "benchmark-test-session",
				Version:     storage.CurrentSchemaVersion,
				History:     make([]storage.HistoryEntry, 0),
				ScriptState: make(map[string]map[string]any),
				SharedState: make(map[string]any),
			}
		}
	})

	b.Run("SessionPersistenceWrite", func(b *testing.B) {
		storage.ClearAllInMemorySessions()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sess := &storage.Session{
				ID:          "benchmark-test-session",
				Version:     storage.CurrentSchemaVersion,
				History:     make([]storage.HistoryEntry, 0),
				ScriptState: make(map[string]map[string]any),
				SharedState: make(map[string]any),
			}
			backend, err := storage.NewInMemoryBackend(sess.ID)
			if err != nil {
				b.Fatalf("failed to create backend: %v", err)
			}
			if err := backend.SaveSession(sess); err != nil {
				b.Fatalf("failed to save session: %v", err)
			}
			backend.Close()
		}
	})

	b.Run("SessionPersistenceRead", func(b *testing.B) {
		storage.ClearAllInMemorySessions()
		sess := &storage.Session{
			ID:          "benchmark-test-session",
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, 0),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		backend, _ := storage.NewInMemoryBackend(sess.ID)
		backend.SaveSession(sess)
		backend.Close()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			backend, err := storage.NewInMemoryBackend(sess.ID)
			if err != nil {
				b.Fatalf("failed to create backend: %v", err)
			}
			loaded, err := backend.LoadSession(sess.ID)
			if err != nil {
				b.Fatalf("failed to load session: %v", err)
			}
			if loaded == nil {
				b.Fatal("session not found")
			}
			backend.Close()
		}
	})

	b.Run("ConcurrentSessionAccess", func(b *testing.B) {
		storage.ClearAllInMemorySessions()
		sess := &storage.Session{
			ID:          "benchmark-test-session",
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, 0),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		backend, _ := storage.NewInMemoryBackend(sess.ID)
		backend.SaveSession(sess)
		backend.Close()

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				backend, err := storage.NewInMemoryBackend(sess.ID)
				if err != nil {
					b.Fatalf("failed to create backend: %v", err)
				}
				loaded, err := backend.LoadSession(sess.ID)
				if err != nil {
					b.Fatalf("failed to load session: %v", err)
				}
				if loaded == nil {
					b.Fatal("session not found")
				}
				backend.Close()
			}
		})
	})

	b.Run("SessionJSONMarshaling", func(b *testing.B) {
		sess := &storage.Session{
			ID:          "benchmark-test-session",
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, 100),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		for i := 0; i < 100; i++ {
			sess.History[i] = storage.HistoryEntry{
				EntryID:    "entry",
				ModeID:     "test-mode",
				Command:    "echo 'test'",
				ReadTime:   time.Now(),
				FinalState: json.RawMessage(`{"state":"test"}`),
			}
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data, err := json.Marshal(sess)
			if err != nil {
				b.Fatalf("failed to marshal session: %v", err)
			}
			_ = data
		}
	})

	b.Run("SessionJSONUnmarshaling", func(b *testing.B) {
		sess := &storage.Session{
			ID:          "benchmark-test-session",
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, 100),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		for i := 0; i < 100; i++ {
			sess.History[i] = storage.HistoryEntry{
				EntryID:    "entry",
				ModeID:     "test-mode",
				Command:    "echo 'test'",
				ReadTime:   time.Now(),
				FinalState: json.RawMessage(`{"state":"test"}`),
			}
		}
		data, _ := json.Marshal(sess)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var loaded storage.Session
			if err := json.Unmarshal(data, &loaded); err != nil {
				b.Fatalf("failed to unmarshal session: %v", err)
			}
			_ = loaded
		}
	})
}

// BenchmarkFileSystemIO benchmarks filesystem-based session I/O operations.
// This covers the full FileSystemBackend path: flock acquisition, JSON
// serialization (MarshalIndent), AtomicWriteFile (CreateTemp → Write →
// Sync → Close → Chmod → Rename), file read, JSON deserialization, and
// lock release + cleanup.
func BenchmarkFileSystemIO(b *testing.B) {
	// Helper to create a session with n history entries for benchmarking.
	makeSession := func(id string, nEntries int) *storage.Session {
		sess := &storage.Session{
			ID:          id,
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, nEntries),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		for i := range sess.History {
			sess.History[i] = storage.HistoryEntry{
				EntryID:    "entry",
				ModeID:     "test-mode",
				Command:    "echo 'test'",
				ReadTime:   time.Now(),
				FinalState: json.RawMessage(`{"state":"test"}`),
			}
		}
		return sess
	}

	b.Run("FullCycle", func(b *testing.B) {
		// Full FileSystem backend cycle: create → save → load → close.
		// Measures combined cost of flock + JSON serialize + AtomicWriteFile
		// + file read + JSON deserialize + lock release.
		//
		// Profiling notes (pprof CPU profile, 2s benchtime):
		//   FullCycle: ~5.4ms/op, ~16KB, 104 allocs (10 history entries).
		//   WriteOnly: ~5.4ms/op — write path dominates the full cycle.
		//   ReadOnly: ~115μs/op — reads are ~47x faster than writes.
		//   AtomicWriteFile: ~5.7ms/op — >99% of write cost.
		//     Dominated by fsync (tempFile.Sync), required for crash safety.
		//   MarshalIndent vs Marshal: 2.6x overhead (116μs vs 45μs, 100 entries)
		//     but only ~2% of total write cost — not worth switching.
		//   CPU profile: app code = 1.54% (10ms json.Unmarshal). Remaining
		//     ~98.5% is OS syscalls (fsync, flock, file I/O).
		//   Conclusion: no application-level optimization targets. The
		//   dominant cost (fsync ~5ms) is required for crash-safe atomic
		//   writes. MarshalIndent→Marshal saves ~1.3% of write time at
		//   the cost of human-readable session files. Read path is already
		//   fast (115μs = flock + ReadFile + Unmarshal).
		dir := b.TempDir()
		storage.SetTestPaths(dir)
		b.Cleanup(storage.ResetPaths)

		sess := makeSession("bench-fs-full", 10)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			backend, err := storage.NewFileSystemBackend(sess.ID)
			if err != nil {
				b.Fatalf("failed to create backend: %v", err)
			}
			if err := backend.SaveSession(sess); err != nil {
				b.Fatalf("failed to save: %v", err)
			}
			loaded, err := backend.LoadSession(sess.ID)
			if err != nil {
				b.Fatalf("failed to load: %v", err)
			}
			if loaded == nil {
				b.Fatal("session not found")
			}
			backend.Close()
		}
	})

	b.Run("WriteOnly", func(b *testing.B) {
		// Isolates write path: flock + MarshalIndent + AtomicWriteFile + unlock.
		dir := b.TempDir()
		storage.SetTestPaths(dir)
		b.Cleanup(storage.ResetPaths)

		sess := makeSession("bench-fs-write", 10)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			backend, err := storage.NewFileSystemBackend(sess.ID)
			if err != nil {
				b.Fatalf("failed to create backend: %v", err)
			}
			if err := backend.SaveSession(sess); err != nil {
				b.Fatalf("failed to save: %v", err)
			}
			backend.Close()
		}
	})

	b.Run("ReadOnly", func(b *testing.B) {
		// Isolates read path: flock + ReadFile + Unmarshal + unlock.
		dir := b.TempDir()
		storage.SetTestPaths(dir)
		b.Cleanup(storage.ResetPaths)

		sess := makeSession("bench-fs-read", 10)
		// Pre-write the session so reads find it.
		backend, err := storage.NewFileSystemBackend(sess.ID)
		if err != nil {
			b.Fatalf("setup: %v", err)
		}
		if err := backend.SaveSession(sess); err != nil {
			b.Fatalf("setup: %v", err)
		}
		backend.Close()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			backend, err := storage.NewFileSystemBackend(sess.ID)
			if err != nil {
				b.Fatalf("failed to create backend: %v", err)
			}
			loaded, err := backend.LoadSession(sess.ID)
			if err != nil {
				b.Fatalf("failed to load: %v", err)
			}
			if loaded == nil {
				b.Fatal("session not found")
			}
			backend.Close()
		}
	})

	b.Run("AtomicWriteFile", func(b *testing.B) {
		// Isolates AtomicWriteFile: CreateTemp → Write → Sync → Close → Chmod → Rename.
		// Measures raw filesystem write overhead without JSON or locking.
		dir := b.TempDir()
		data := []byte(`{"test":"data","values":[1,2,3,4,5]}`)
		target := filepath.Join(dir, "atomic-bench.json")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := storage.AtomicWriteFile(target, data, 0644); err != nil {
				b.Fatalf("failed: %v", err)
			}
		}
	})

	b.Run("MarshalIndentVsMarshal", func(b *testing.B) {
		// Compares json.MarshalIndent (used by SaveSession) with json.Marshal.
		// SaveSession uses MarshalIndent for human-readable session files.
		sess := makeSession("marshal-cmp", 100)

		b.Run("MarshalIndent", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := json.MarshalIndent(sess, "", "  "); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("Marshal", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := json.Marshal(sess); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

// BenchmarkConfigLoading benchmarks configuration loading operations.
func BenchmarkConfigLoading(b *testing.B) {
	b.Run("NewConfig", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = config.NewConfig()
		}
	})

	b.Run("ConfigOptionGet", func(b *testing.B) {
		cfg := config.NewConfig()
		cfg.SetGlobalOption("test-key", "test-value")
		cfg.SetCommandOption("test-command", "flag", "value")

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = cfg.GetGlobalOption("test-key")
			_, _ = cfg.GetCommandOption("test-command", "flag")
		}
	})

	b.Run("ConfigOptionSet", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cfg := config.NewConfig()
			cfg.SetGlobalOption("key", "value")
		}
	})

	b.Run("ConfigLoadFromReader", func(b *testing.B) {
		configContent := `# Test configuration
global-option global-value
[command1]
option1 value1
`
		reader := strings.NewReader(configContent)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := config.LoadFromReader(reader)
			if err != nil {
				b.Fatalf("failed to load config: %v", err)
			}
			reader.Seek(0, 0)
		}
	})

	b.Run("ConfigPathResolution", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := config.GetConfigPath()
			if err != nil {
				b.Fatalf("failed to get config path: %v", err)
			}
		}
	})

	b.Run("ConfigValidation", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cfg := config.NewConfig()
			cfg.SetGlobalOption("max-age-days", "90")
			cfg.SetGlobalOption("max-count", "100")
			cfg.SetGlobalOption("auto-cleanup-enabled", "true")
			_ = cfg
		}
	})
}

// BenchmarkScriptingEngine benchmarks scripting engine operations.
func BenchmarkScriptingEngine(b *testing.B) {
	b.Run("VMCreation", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = goja.New()
		}
	})

	b.Run("RuntimeCreation", func(b *testing.B) {
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rt, err := scripting.NewRuntime(ctx)
			if err != nil {
				b.Fatalf("failed to create runtime: %v", err)
			}
			rt.Close()
		}
	})

	b.Run("GlobalRegistration", func(b *testing.B) {
		ctx := context.Background()
		rt, err := scripting.NewRuntime(ctx)
		if err != nil {
			b.Fatalf("failed to create runtime: %v", err)
		}
		defer rt.Close()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := rt.SetGlobal("testVar", "testValue"); err != nil {
				b.Fatalf("failed to set global: %v", err)
			}
		}
	})

	b.Run("SimpleScriptExecution", func(b *testing.B) {
		ctx := context.Background()
		rt, err := scripting.NewRuntime(ctx)
		if err != nil {
			b.Fatalf("failed to create runtime: %v", err)
		}
		defer rt.Close()

		script := `var x = 42; var y = 58; var z = x + y;`

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := rt.LoadScript("test.js", script); err != nil {
				b.Fatalf("failed to load script: %v", err)
			}
		}
	})

	b.Run("ScriptCompilation", func(b *testing.B) {
		script := `function add(a, b) { return a + b; }`

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := goja.Compile("test.js", script, true)
			if err != nil {
				b.Fatalf("failed to compile script: %v", err)
			}
		}
	})

	b.Run("ScriptExecutionWithVM", func(b *testing.B) {
		script := `var x = 42; var y = 58; var z = x + y;`
		prg, _ := goja.Compile("test.js", script, true)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			vm := goja.New()
			if _, err := vm.RunProgram(prg); err != nil {
				b.Fatalf("failed to run script: %v", err)
			}
		}
	})

	b.Run("FullEngineCreation", func(b *testing.B) {
		// Profiling notes (pprof CPU profile, 2s benchtime):
		//   ~134μs/op, ~161KB/op, ~862 allocs/op.
		//   Application code accounts for <0.3% of CPU time. The dominant costs
		//   are runtime scheduling (goroutine start/stop for eventloop) and GC
		//   (161KB allocated per iteration). No single function is a hotspot.
		//   Breakdown: VMCreation ~480ns, RuntimeCreation ~18μs, remaining
		//   ~116μs spread across ContextManager, TerminalIO, TUIManager,
		//   builtin.Register (20 modules), require.Enable, setupGlobals,
		//   and Engine.Close. All are negligible individually.
		//   Conclusion: no optimization targets exist — startup is already
		//   sub-millisecond with no lazy init or caching opportunities that
		//   would produce measurable improvement.
		ctx := context.Background()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			engine, err := scripting.NewEngineWithConfig(ctx, io.Discard, io.Discard, "", "memory")
			if err != nil {
				b.Fatalf("failed to create engine: %v", err)
			}
			engine.Close()
		}
	})
}

// BenchmarkCommandExecution benchmarks command execution operations.
func BenchmarkCommandExecution(b *testing.B) {
	b.Run("CommandRegistration", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			registry := command.NewRegistryWithConfig(config.NewConfig())
			registry.Register(&testCommand{name: "test"})
			_ = registry
		}
	})
}

// TestPerformanceRegression tests that critical operations complete within acceptable thresholds.
func TestPerformanceRegression(t *testing.T) {
	t.Run("SessionIDGeneration", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		// Use platform-specific threshold
		threshold := thresholdSessionIDGenerationUnix
		if runtime.GOOS == "windows" {
			threshold = thresholdSessionIDGenerationWindows
		}

		start := time.Now()
		const iterations = 100
		for i := 0; i < iterations; i++ {
			_, _, err := session.GetSessionID("")
			if err != nil {
				t.Fatalf("failed to generate session ID: %v", err)
			}
		}

		elapsed := time.Since(start)
		avgUs := elapsed.Microseconds() / iterations

		if avgUs > int64(threshold) {
			t.Errorf("Session ID generation too slow: avg %d μs (threshold: %d μs)", avgUs, threshold)
		}
		t.Logf("Session ID generation: avg %d μs (threshold: %d μs)", avgUs, threshold)
	})

	t.Run("SessionPersistence", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		storage.ClearAllInMemorySessions()

		// Write benchmark
		writeStart := time.Now()
		const writeIter = 50
		sess := &storage.Session{
			ID:          "test-session",
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, 0),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		for i := 0; i < writeIter; i++ {
			backend, _ := storage.NewInMemoryBackend(sess.ID)
			backend.SaveSession(sess)
			backend.Close()
		}
		writeElapsed := time.Since(writeStart)
		avgWriteUs := writeElapsed.Microseconds() / writeIter

		if avgWriteUs > thresholdSessionPersistenceWrite {
			t.Errorf("Session write too slow: avg %d μs (threshold: %d μs)", avgWriteUs, thresholdSessionPersistenceWrite)
		}

		// Read benchmark
		readStart := time.Now()
		const readIter = 50
		for i := 0; i < readIter; i++ {
			backend, _ := storage.NewInMemoryBackend(sess.ID)
			loaded, _ := backend.LoadSession(sess.ID)
			if loaded == nil {
				t.Fatal("session not found")
			}
			backend.Close()
		}
		readElapsed := time.Since(readStart)
		avgReadUs := readElapsed.Microseconds() / readIter

		if avgReadUs > thresholdSessionPersistenceRead {
			t.Errorf("Session read too slow: avg %d μs (threshold: %d μs)", avgReadUs, thresholdSessionPersistenceRead)
		}

		t.Logf("Session write: avg %d μs, read: avg %d μs", avgWriteUs, avgReadUs)
	})

	t.Run("ScriptingEngineStartup", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		ctx := context.Background()
		start := time.Now()
		const iterations = 10
		for i := 0; i < iterations; i++ {
			rt, err := scripting.NewRuntime(ctx)
			if err != nil {
				t.Fatalf("failed to create runtime: %v", err)
			}
			rt.Close()
		}

		elapsed := time.Since(start)
		avgUs := elapsed.Microseconds() / iterations

		if avgUs > thresholdRuntimeCreation {
			t.Errorf("Runtime creation too slow: avg %d μs (threshold: %d μs)", avgUs, thresholdRuntimeCreation)
		}
		t.Logf("Runtime creation: avg %d μs (threshold: %d μs)", avgUs, thresholdRuntimeCreation)
	})

	t.Run("ScriptExecution", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		ctx := context.Background()
		rt, err := scripting.NewRuntime(ctx)
		if err != nil {
			t.Fatalf("failed to create runtime: %v", err)
		}
		defer rt.Close()

		script := `var x = 42; var y = 58; var z = x + y;`
		start := time.Now()
		const iterations = 50
		for i := 0; i < iterations; i++ {
			if err := rt.LoadScript("test.js", script); err != nil {
				t.Fatalf("failed to load script: %v", err)
			}
		}

		elapsed := time.Since(start)
		avgUs := elapsed.Microseconds() / iterations

		if avgUs > thresholdSimpleScriptExec {
			t.Errorf("Script execution too slow: avg %d μs (threshold: %d μs)", avgUs, thresholdSimpleScriptExec)
		}
		t.Logf("Script execution: avg %d μs (threshold: %d μs)", avgUs, thresholdSimpleScriptExec)
	})

	t.Run("ConcurrentSessionAccess", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		storage.ClearAllInMemorySessions()
		sess := &storage.Session{
			ID:          "test-session",
			Version:     storage.CurrentSchemaVersion,
			History:     make([]storage.HistoryEntry, 0),
			ScriptState: make(map[string]map[string]any),
			SharedState: make(map[string]any),
		}
		backend, _ := storage.NewInMemoryBackend(sess.ID)
		backend.SaveSession(sess)
		backend.Close()

		const numGoroutines = 10
		const iterPerGoroutine = 20

		var wg sync.WaitGroup
		start := time.Now()

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < iterPerGoroutine; i++ {
					backend, err := storage.NewInMemoryBackend(sess.ID)
					if err != nil {
						t.Errorf("failed to create backend: %v", err)
						return
					}
					loaded, err := backend.LoadSession(sess.ID)
					if err != nil {
						t.Errorf("failed to load session: %v", err)
						backend.Close()
						return
					}
					if loaded == nil {
						t.Error("session not found")
						backend.Close()
						return
					}
					backend.Close()
				}
			}()
		}
		wg.Wait()
		elapsed := time.Since(start)

		totalOps := numGoroutines * iterPerGoroutine
		avgUs := elapsed.Microseconds() / int64(totalOps)

		if avgUs > thresholdConcurrentSessionAccess {
			t.Errorf("Concurrent access too slow: avg %d μs (threshold: %d μs)", avgUs, thresholdConcurrentSessionAccess)
		}
		t.Logf("Concurrent access: avg %d μs for %d ops", avgUs, totalOps)
	})
}

// TestMemoryUsageRegression tests for memory leaks.
func TestMemoryUsageRegression(t *testing.T) {
	t.Run("RuntimeCreationNoLeak", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		ctx := context.Background()
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		const iterations = 100
		for i := 0; i < iterations; i++ {
			rt, err := scripting.NewRuntime(ctx)
			if err != nil {
				t.Fatalf("failed to create runtime: %v", err)
			}
			rt.Close()
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)

		const maxMemoryIncrease = 200 * 1024 * 1024 // Increased for goja-eventloop adapter (binds Web Platform APIs)
		memoryIncrease := m2.TotalAlloc - m1.TotalAlloc

		if memoryIncrease > maxMemoryIncrease {
			t.Errorf("Memory leak detected: %d bytes (max: %d)", memoryIncrease, maxMemoryIncrease)
		}
		t.Logf("Memory usage: %d bytes allocated", memoryIncrease)
	})

	t.Run("SessionBackendNoLeak", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping in short mode")
		}

		storage.ClearAllInMemorySessions()
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		const iterations = 100
		for i := 0; i < iterations; i++ {
			sess := &storage.Session{
				ID:          "test-session",
				Version:     storage.CurrentSchemaVersion,
				History:     make([]storage.HistoryEntry, 0),
				ScriptState: make(map[string]map[string]any),
				SharedState: make(map[string]any),
			}
			backend, err := storage.NewInMemoryBackend(sess.ID)
			if err != nil {
				t.Fatalf("failed to create backend: %v", err)
			}
			backend.SaveSession(sess)
			backend.Close()
		}

		runtime.GC()
		runtime.ReadMemStats(&m2)

		const maxMemoryIncrease = 50 * 1024 * 1024
		memoryIncrease := m2.TotalAlloc - m1.TotalAlloc

		if memoryIncrease > maxMemoryIncrease {
			t.Errorf("Memory leak detected: %d bytes (max: %d)", memoryIncrease, maxMemoryIncrease)
		}
		t.Logf("Memory usage: %d bytes allocated", memoryIncrease)
	})
}

// Helper types for benchmarks

type testCommand struct {
	name string
}

func (c *testCommand) Name() string                { return c.name }
func (c *testCommand) Description() string         { return "Test command" }
func (c *testCommand) Usage() string               { return "test [options]" }
func (c *testCommand) SetupFlags(fs *flag.FlagSet) {}
func (c *testCommand) Execute(args []string, stdout, stderr io.Writer) error {
	return nil
}
