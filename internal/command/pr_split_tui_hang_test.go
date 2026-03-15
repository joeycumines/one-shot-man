package command

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// loadTUIEngineRaw creates a fully loaded TUI engine and returns both the
// engine (for direct RunOnLoopSync access) and an evalJS function.
func loadTUIEngineRaw(t testing.TB) *scripting.Engine {
	t.Helper()

	var stdout safeBuffer
	var stderr bytes.Buffer

	b := scriptCommandBase{
		config:   config.NewConfig(),
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	jsConfig := map[string]any{
		"baseBranch":    "main",
		"strategy":      "directory",
		"maxFiles":      10,
		"branchPrefix":  "split/",
		"verifyCommand": "true",
		"dryRun":        false,
		"jsonOutput":    false,
	}

	engine.SetGlobal("config", map[string]any{"name": "pr-split"})
	engine.SetGlobal("prSplitConfig", jsConfig)

	chunkMap := map[string]*string{}
	for _, c := range prSplitChunks {
		chunkMap[c.name] = c.source
	}

	for _, name := range allChunksThrough12 {
		src, ok := chunkMap[name]
		if !ok {
			t.Fatalf("unknown chunk %q", name)
		}
		script := engine.LoadScriptFromString("pr-split/"+name, *src)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("chunk %s: %v", name, err)
		}
	}

	// Inject TUI mocks.
	evalJS := makeEvalJS(t, engine, 30*time.Second)
	if _, err := evalJS(setupTUIMocks); err != nil {
		t.Fatalf("TUI mocks: %v", err)
	}

	// Load TUI chunks.
	tuiChunks := []struct {
		name   string
		source string
	}{
		{"13_tui", prSplitChunk13TUI},
		{"14_tui_commands", prSplitChunk14TUICommands},
		{"15_tui_views", prSplitChunk15TUIViews},
		{"16_tui_core", prSplitChunk16TUICore},
	}
	for _, chunk := range tuiChunks {
		if _, err := evalJS(chunk.source); err != nil {
			t.Fatalf("chunk %s: %v", chunk.name, err)
		}
	}

	return engine
}

// ---------------------------------------------------------------------------
// TUI Hang Reproducer Tests
//
// These tests exercise the REAL async analysis pipeline (with exec.spawn
// and actual git operations) through the TUI update function. The goal is
// to reproduce and verify the fix for the "Processing..." hang bug where
// the interactive TUI would get stuck on the CONFIG screen forever after
// pressing "Start Analysis".
//
// Unlike the mocked tests in pr_split_16_async_pipeline_test.go, these
// tests use REAL analyzeDiffAsync calls with exec.spawn, verifying that
// the Promise chain resolves correctly through the event loop.
// ---------------------------------------------------------------------------

// TestTUIHang_RealAsyncAnalysis exercises the full startAnalysis →
// runAnalysisAsync → handleAnalysisPoll flow using real exec.spawn and
// real git operations. This is the core reproducer for the "Processing..."
// hang bug.
func TestTUIHang_RealAsyncAnalysis(t *testing.T) {
	// Create a real git repo with a feature branch.
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "api/handler.go"), "package api\n\nfunc Handle() {}\n")
	writeFile(t, filepath.Join(dir, "pkg/util.go"), "package pkg\n\nfunc Util() {}\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"v2\") }\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature: add api and pkg")

	// Load the full TUI engine with helpers, pointing at our real git repo.
	evalJS := loadTUIEngineWithHelpers(t)

	// Configure the runtime to point at the real git repo.
	_, err := evalJS(`(function() {
		globalThis.prSplit.runtime.baseBranch = 'main';
		globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
		globalThis.prSplit.runtime.strategy = 'directory';
		globalThis.prSplit.runtime.mode = 'heuristic';
		globalThis.prSplit.runtime.verifyCommand = 'true';
		globalThis.prSplit.runtime.branchPrefix = 'split/';
		return 'ok';
	})()`)
	if err != nil {
		t.Fatalf("failed to configure runtime: %v", err)
	}

	// Step 1: Initialize state at CONFIG and trigger startAnalysis.
	// This calls the REAL analyzeDiffAsync with exec.spawn.
	result, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.focusIndex = 4; // nav-next element

		// Trigger startAnalysis via enter key.
		var r = sendKey(s, 'enter');
		s = r[0];

		// Save state globally so Step 2 can poll it.
		globalThis.__tuiHangState = s;

		if (!s.isProcessing) {
			return JSON.stringify({
				ok: false,
				error: 'isProcessing should be true after startAnalysis',
				state: s.wizardState,
				validationError: s.configValidationError || null,
				errorDetails: s.errorDetails || null
			});
		}
		if (!s.analysisRunning) {
			return JSON.stringify({
				ok: false,
				error: 'analysisRunning should be true after startAnalysis',
				state: s.wizardState
			});
		}

		return JSON.stringify({
			ok: true,
			state: s.wizardState,
			isProcessing: s.isProcessing,
			analysisRunning: s.analysisRunning
		});
	})()`)
	if err != nil {
		t.Fatalf("failed to trigger startAnalysis: %v", err)
	}
	t.Logf("Step 1 (startAnalysis): %v", result)

	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	var step1 map[string]any
	if err := unmarshalJSON(resultStr, &step1); err != nil {
		t.Fatalf("failed to parse step1 result: %v", err)
	}
	if step1["ok"] != true {
		t.Fatalf("startAnalysis failed: %v", step1["error"])
	}

	// Step 2: Wait for the async pipeline to complete and poll.
	// The real exec.spawn goroutines need time to finish.
	// We poll repeatedly with analysis-poll Ticks to check.
	deadline := time.Now().Add(30 * time.Second)
	attempts := 0
	for time.Now().Before(deadline) {
		attempts++
		time.Sleep(200 * time.Millisecond)

		result, err = evalJS(`(function() {
			// Get the state - it's been mutated directly by the async pipeline.
			// We need to call handleAnalysisPoll to check.
			var s = globalThis.__tuiHangState;
			if (!s) return JSON.stringify({ok: false, error: 'state lost'});

			var r = update({type: 'Tick', id: 'analysis-poll'}, s);
			s = r[0];
			globalThis.__tuiHangState = s;

			return JSON.stringify({
				isProcessing: s.isProcessing,
				analysisRunning: s.analysisRunning,
				wizardState: s.wizardState,
				analysisError: s.analysisError || null,
				errorDetails: s.errorDetails || null,
				configValidationError: s.configValidationError || null,
				progress: s.analysisProgress
			});
		})()`)
		if err != nil {
			t.Fatalf("poll attempt %d failed: %v", attempts, err)
		}

		resultStr, ok = result.(string)
		if !ok {
			t.Fatalf("unexpected result type on poll %d: %T", attempts, result)
		}
		var pollResult map[string]any
		if err := unmarshalJSON(resultStr, &pollResult); err != nil {
			t.Fatalf("failed to parse poll result: %v", err)
		}

		t.Logf("Poll %d: state=%v isProcessing=%v analysisRunning=%v progress=%v error=%v",
			attempts, pollResult["wizardState"], pollResult["isProcessing"],
			pollResult["analysisRunning"], pollResult["progress"], pollResult["analysisError"])

		// Check if done.
		isProcessing, _ := pollResult["isProcessing"].(bool)
		analysisRunning, _ := pollResult["analysisRunning"].(bool)
		if !isProcessing && !analysisRunning {
			state, _ := pollResult["wizardState"].(string)
			if state == "PLAN_REVIEW" {
				t.Logf("SUCCESS: Pipeline completed after %d polls, state=PLAN_REVIEW", attempts)
				return
			}
			if state == "ERROR" {
				t.Fatalf("Pipeline errored: %v", pollResult["errorDetails"])
			}
			if state == "CONFIG" {
				t.Fatalf("Pipeline stopped on CONFIG: configError=%v errorDetails=%v",
					pollResult["configValidationError"], pollResult["errorDetails"])
			}
			t.Fatalf("Pipeline stopped in unexpected state: %s, error=%v", state, pollResult["analysisError"])
		}
	}
	t.Fatalf("TIMEOUT: Pipeline did not complete in 30s after %d poll attempts — THIS IS THE HANG BUG", attempts)
}

func unmarshalJSON(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// TestTUIHang_ConcurrentPolling simulates BubbleTea's behavior: a separate
// goroutine rapidly polls via RunOnLoopSync (100ms intervals) while the
// async analysis pipeline runs on the event loop. This catches races where
// rapid external submissions can starve Promise/microtask resolution.
func TestTUIHang_ConcurrentPolling(t *testing.T) {
	dir := initGitRepo(t)
	writeFile(t, filepath.Join(dir, "README.md"), "# Test\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(dir, "api/handler.go"), "package api\n\nfunc Handle() {}\n")
	writeFile(t, filepath.Join(dir, "pkg/util.go"), "package pkg\n\nfunc Util() {}\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"v2\") }\n")
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-m", "feature: add api and pkg")

	// Get access to the underlying engine.
	engine := loadTUIEngineRaw(t)
	evalJS := makeEvalJS(t, engine, 30*time.Second)

	// Configure runtime.
	_, err := evalJS(`(function() {
		globalThis.prSplit.runtime.baseBranch = 'main';
		globalThis.prSplit.runtime.dir = '` + escapeJSPath(dir) + `';
		globalThis.prSplit.runtime.strategy = 'directory';
		globalThis.prSplit.runtime.mode = 'heuristic';
		globalThis.prSplit.runtime.verifyCommand = 'true';
		globalThis.prSplit.runtime.branchPrefix = 'split/';
		return 'ok';
	})()`)
	if err != nil {
		t.Fatalf("failed to configure runtime: %v", err)
	}

	// Load helpers.
	if _, err := evalJS(chunk16Helpers); err != nil {
		t.Fatalf("failed to load helpers: %v", err)
	}

	// Trigger startAnalysis (on event loop via evalJS/Submit).
	result, err := evalJS(`(function() {
		var s = initState('CONFIG');
		s.focusIndex = 4;
		var r = sendKey(s, 'enter');
		s = r[0];
		globalThis.__tuiHangState = s;
		return JSON.stringify({
			ok: s.isProcessing && s.analysisRunning,
			isProcessing: s.isProcessing,
			analysisRunning: s.analysisRunning,
			wizardState: s.wizardState
		});
	})()`)
	if err != nil {
		t.Fatalf("startAnalysis failed: %v", err)
	}
	t.Logf("startAnalysis result: %v", result)

	resultStr, _ := result.(string)
	var step1 map[string]any
	if err := unmarshalJSON(resultStr, &step1); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if step1["ok"] != true {
		t.Fatalf("startAnalysis didn't start: %+v", step1)
	}

	// NOW simulate BubbleTea's behavior: poll via RunOnLoopSync from a
	// separate goroutine, exactly like BubbleTea's Update handler.
	// This is the critical difference from TestTUIHang_RealAsyncAnalysis
	// which uses evalJS (loop.Submit) with 200ms sleeps between polls.
	type pollResult struct {
		IsProcessing    bool    `json:"isProcessing"`
		AnalysisRunning bool    `json:"analysisRunning"`
		WizardState     string  `json:"wizardState"`
		Error           string  `json:"analysisError"`
		Progress        float64 `json:"progress"`
	}

	loop := engine.Loop()
	vm := engine.Runtime()
	resultCh := make(chan pollResult, 1)
	errCh := make(chan error, 1)

	// runOnLoopSync is the critical function — it submits to the event loop
	// and blocks, exactly like BubbleTea's TryRunOnLoopSync does when called
	// from an external goroutine.
	runOnLoopSync := func(fn func(*goja.Runtime) error) error {
		done := make(chan error, 1)
		if submitErr := loop.Submit(func() {
			done <- fn(vm)
		}); submitErr != nil {
			return submitErr
		}
		return <-done
	}

	var pollWg sync.WaitGroup
	pollWg.Add(1)
	go func() {
		defer pollWg.Done()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.After(30 * time.Second)

		for attempts := 0; ; attempts++ {
			select {
			case <-deadline:
				errCh <- nil // signal timeout
				return
			case <-ticker.C:
			}

			// Poll: exactly what BubbleTea does via TryRunOnLoopSync.
			var pr pollResult
			if loopErr := runOnLoopSync(func(vm *goja.Runtime) error {
				val, runErr := vm.RunString(`(function() {
					var s = globalThis.__tuiHangState;
					if (!s) return JSON.stringify({error: 'state lost'});
					var r = update({type: 'Tick', id: 'analysis-poll'}, s);
					s = r[0];
					globalThis.__tuiHangState = s;
					return JSON.stringify({
						isProcessing: s.isProcessing,
						analysisRunning: s.analysisRunning,
						wizardState: s.wizardState,
						analysisError: s.analysisError || '',
						progress: s.analysisProgress
					});
				})()`)
				if runErr != nil {
					return runErr
				}
				return json.Unmarshal([]byte(val.String()), &pr)
			}); loopErr != nil {
				errCh <- loopErr
				return
			}

			// Also simulate View call (BubbleTea calls View after each Update).
			_ = runOnLoopSync(func(vm *goja.Runtime) error {
				_, _ = vm.RunString(`(function() {
					var s = globalThis.__tuiHangState;
					if (s && typeof view === 'function') { view(s); }
				})()`)
				return nil
			})

			if !pr.IsProcessing && !pr.AnalysisRunning {
				t.Logf("Poll %d: state=%s progress=%.2f (COMPLETE)", attempts, pr.WizardState, pr.Progress)
				resultCh <- pr
				return
			}
			if attempts%10 == 0 {
				t.Logf("Poll %d: state=%s running=%v progress=%.2f",
					attempts, pr.WizardState, pr.AnalysisRunning, pr.Progress)
			}
		}
	}()

	select {
	case pr := <-resultCh:
		pollWg.Wait()
		if pr.WizardState == "PLAN_REVIEW" {
			t.Logf("SUCCESS: Concurrent polling completed, state=PLAN_REVIEW")
		} else if pr.Error != "" {
			t.Fatalf("Pipeline errored: %s", pr.Error)
		} else {
			t.Fatalf("Pipeline stopped in unexpected state: %s", pr.WizardState)
		}
	case loopErr := <-errCh:
		pollWg.Wait()
		if loopErr != nil {
			t.Fatalf("RunOnLoopSync error: %v", loopErr)
		}
		t.Fatalf("TIMEOUT: Concurrent polling — pipeline did not complete in 30s. THIS REPRODUCES THE HANG BUG.")
	}
}
