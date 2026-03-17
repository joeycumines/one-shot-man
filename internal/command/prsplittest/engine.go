package prsplittest

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// Engine wraps a [scripting.Engine] with chunk-loading and JS evaluation
// helpers. Created via [NewEngine] or [NewChunkEngine].
type Engine struct {
	engine *scripting.Engine
	stdout *SafeBuffer
	cancel context.CancelFunc
}

// NewEngine creates a new scripting engine configured for PR Split testing.
// It sets the standard globals (config, prSplitConfig) but does NOT load
// any chunks — call [Engine.LoadChunks] to load specific chunks.
//
// overrides are merged into the default prSplitConfig. The engine and its
// context are cleaned up automatically via t.Cleanup.
func NewEngine(t testing.TB, overrides map[string]any) *Engine {
	t.Helper()

	var stdout SafeBuffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	engine, err := scripting.NewEngineDetailed(
		ctx, &stdout, &stderr,
		t.Name(), // sessionID
		"memory", // store
		nil,      // logFile
		0,        // logBufferSize (defaults to 1000)
		slog.LevelInfo,
	)
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()
		engine.Close()
	})

	// Set up globals matching PrSplitCommand.Execute / loadChunkEngine.
	jsConfig := map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"dryRun":        false,
		"jsonOutput":    false,
	}
	for k, v := range overrides {
		jsConfig[k] = v
	}
	// B00-safety: ensure a dir is always set so resolveDir never falls back
	// to process CWD which would target the real repository.
	if _, ok := jsConfig["dir"]; !ok {
		jsConfig["dir"] = t.TempDir()
	}

	engine.SetGlobal("config", map[string]any{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)
	engine.SetGlobal("args", []string{})

	return &Engine{
		engine: engine,
		stdout: &stdout,
		cancel: cancel,
	}
}

// LoadChunks loads the specified JS chunks into the engine by name.
// Chunk names should be like: "00_core", "01_analysis", etc.
// Sources are read from disk via [discoverChunks].
func (e *Engine) LoadChunks(t testing.TB, names ...string) {
	t.Helper()
	sources, _, err := discoverChunks()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		src, ok := sources[name]
		if !ok {
			t.Fatalf("prsplittest: unknown chunk name %q", name)
		}
		script := e.engine.LoadScriptFromString("pr-split/"+name, src)
		if err := e.engine.ExecuteScript(script); err != nil {
			t.Fatalf("prsplittest: failed to load chunk %s: %v", name, err)
		}
	}
}

// EvalJS returns an evaluation function that executes JS code on the
// engine's event loop. It handles both synchronous and asynchronous (await)
// expressions, properly resolving Promises.
//
// The returned function is equivalent to the evalJS helpers in the existing
// test infrastructure.
func (e *Engine) EvalJS(t testing.TB) func(string) (any, error) {
	t.Helper()
	return makeEvalJS(t, e.engine, 30*time.Second)
}

// EvalJSTimeout is like [Engine.EvalJS] but with a custom timeout.
func (e *Engine) EvalJSTimeout(t testing.TB, timeout time.Duration) func(string) (any, error) {
	t.Helper()
	return makeEvalJS(t, e.engine, timeout)
}

// ScriptingEngine returns the underlying [scripting.Engine] for direct access
// when the higher-level helpers are insufficient.
func (e *Engine) ScriptingEngine() *scripting.Engine {
	return e.engine
}

// SetGlobal sets a global variable in the JS runtime. Must be called during
// engine setup (before async operations).
func (e *Engine) SetGlobal(name string, value any) {
	e.engine.SetGlobal(name, value)
}

// Stdout returns the captured stdout buffer.
func (e *Engine) Stdout() *SafeBuffer {
	return e.stdout
}

// NewChunkEngine is a convenience function that creates an engine and loads
// the specified chunks in one call. It is a drop-in replacement for the
// internal loadChunkEngine function.
//
// Returns an evalJS function identical to what loadChunkEngine returns.
func NewChunkEngine(t testing.TB, overrides map[string]any, chunkNames ...string) func(string) (any, error) {
	t.Helper()
	eng := NewEngine(t, overrides)
	eng.LoadChunks(t, chunkNames...)
	return eng.EvalJS(t)
}

// NewFullEngine creates an engine loaded with ALL discovered chunks plus the
// [ChunkCompatShim]. This is a drop-in replacement for the internal
// loadPrSplitEngineWithEval function when only the evalJS function is needed.
//
// The compat shim re-exposes monolith-era globals (e.g., renderPrompt,
// analyzeChanges) at the top level for backward-compatible test code.
func NewFullEngine(t testing.TB, overrides map[string]any) func(string) (any, error) {
	t.Helper()
	evalJS := NewChunkEngine(t, overrides, AllChunkNames()...)
	if _, err := evalJS(ChunkCompatShim); err != nil {
		t.Fatalf("prsplittest: compat shim failed: %v", err)
	}
	return evalJS
}
