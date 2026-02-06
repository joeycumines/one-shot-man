// Package internal_test contains performance benchmarks and regression tests for one-shot-man.
package internal_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
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

// Performance thresholds (in microseconds) for regression detection
const (
	// Session operation thresholds
	thresholdSessionIDGeneration     = 100
	thresholdSessionCreation         = 500
	thresholdSessionPersistenceWrite = 1000
	thresholdSessionPersistenceRead  = 500
	thresholdConcurrentSessionAccess = 2000

	// Scripting engine thresholds
	thresholdRuntimeCreation    = 50000
	thresholdGlobalRegistration = 100
	thresholdSimpleScriptExec   = 1000
	thresholdVMCreation         = 5000
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

		if avgUs > thresholdSessionIDGeneration {
			t.Errorf("Session ID generation too slow: avg %d μs (threshold: %d μs)", avgUs, thresholdSessionIDGeneration)
		}
		t.Logf("Session ID generation: avg %d μs (threshold: %d μs)", avgUs, thresholdSessionIDGeneration)
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

		const maxMemoryIncrease = 100 * 1024 * 1024
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
