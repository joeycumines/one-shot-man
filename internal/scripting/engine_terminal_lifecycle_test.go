package scripting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func newTerminalLifecycleEngine(t *testing.T, parent context.Context) (*Engine, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(parent)
	engine, err := NewEngine(ctx, io.Discard, io.Discard, testutil.NewTestSessionID("test", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		cancel()
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		done := make(chan error, 1)
		go func() { done <- engine.Close() }()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("cleanup Engine.Close: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("cleanup Engine.Close did not finish")
		}
	})
	return engine, cancel
}

// startTerminalLifecycleTea reaches the real model Init before returning. The
// supplied action runs asynchronously, outside ExecuteScript's initial callback.
func startTerminalLifecycleTea(t *testing.T, engine *Engine, setup, action string) <-chan error {
	t.Helper()
	ready := make(chan struct{})
	var readyOnce sync.Once
	engine.SetGlobal("__terminalLifecycleReady", func() { readyOnce.Do(func() { close(ready) }) })
	script := engine.LoadScriptString("terminal-lifecycle.js", setup+`
		var tea = require("osm:bubbletea");
		function init() {
			__terminalLifecycleReady();
			`+action+`
			return [{}, null];
		}
		function update(msg, model) { return [model, null]; }
		function view(model) { return {content: "idle"}; }
		tea.run(tea.newModel({init: init, update: update, view: view}));
	`)
	done := make(chan error, 1)
	go func() { done <- engine.ExecuteScript(script) }()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("idle BubbleTea program did not reach Init")
	}
	return done
}

func terminalLifecycleResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not finish before the lifecycle deadline")
		return nil
	}
}

func waitTerminalLifecycle(t *testing.T, engine *Engine) {
	t.Helper()
	done := make(chan error, 1)
	go func() { engine.Wait(); done <- nil }()
	terminalLifecycleResult(t, done)
	select {
	case <-engine.Adapter().Done():
	default:
		t.Fatal("Engine.Wait returned before adapter terminal completion")
	}
}

func TestEngine_TerminalExitReleasesIdleTea(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live event loops and BubbleTea programs")
	}
	for _, tc := range []struct {
		name, setup, action string
		code                int
		codeSet             bool
	}{
		{name: "zero", action: `process.exit(0);`, codeSet: true},
		{name: "nonzero", action: `process.exit(7);`, code: 7, codeSet: true},
		{name: "undefined", action: `process.exit(undefined);`},
		{name: "replaced_exit_emitter", setup: `process.emit = function() { return false; };`, action: `process.exit(7);`, code: 7, codeSet: true},
		{name: "exit_listener_final_code", setup: `process.on("exit", function() { process.exitCode = 9; });`, action: `process.exit(7);`, code: 9, codeSet: true},
		{name: "unhandled_rejection", action: `Promise.reject(new Error("terminal rejection"));`, code: 1, codeSet: true},
		{name: "fatal_exception_handler", setup: `process.on("uncaughtException", function() { throw new Error("handler failed"); });`, action: `throw new Error("original failure");`, code: 7, codeSet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := newTerminalLifecycleEngine(t, context.Background())
			done := startTerminalLifecycleTea(t, engine, tc.setup, `setTimeout(function() { `+tc.action+` }, 0);`)
			if err := terminalLifecycleResult(t, done); err != nil {
				t.Fatalf("ExecuteScript reported a shutdown artifact: %v", err)
			}
			// Neither Engine.Close nor cancellation is used to release the
			// program or bootstrap worker before this terminal join.
			waitTerminalLifecycle(t, engine)
			if code, set := engine.ExitCode(); code != tc.code || set != tc.codeSet {
				t.Fatalf("ExitCode = (%d, %v), want (%d, %v)", code, set, tc.code, tc.codeSet)
			}
			if cause := context.Cause(engine.Ctx()); cause != errRuntimeExit {
				t.Fatalf("engine cancellation cause = %v, want terminal script status", cause)
			}
			if err := engine.ExecuteScript(engine.LoadScriptString("after-exit.js", `throw new Error("must not run");`)); !errors.Is(err, context.Canceled) {
				t.Fatalf("fresh ExecuteScript after terminal exit = %v, want cancellation", err)
			}
		})
	}
}

func TestEngine_ExitMetadataDoesNotTerminate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live JavaScript engines")
	}
	for _, tc := range []struct {
		name, script string
		code         int
		codeSet      bool
	}{
		{name: "synthetic_exit_event", script: `process.emit("exit", 7);`},
		{name: "exit_code_assignment", script: `process.exitCode = 7;`, code: 7, codeSet: true},
		{name: "invalid_string", script: `var caught = false; try { process.exit("bad"); } catch (e) { caught = true; } if (!caught) throw new Error("invalid exit accepted");`},
		{name: "invalid_boolean", script: `var caught = false; try { process.exit(true); } catch (e) { caught = true; } if (!caught) throw new Error("invalid exit accepted");`},
		{name: "invalid_fraction", script: `var caught = false; try { process.exit(1.5); } catch (e) { caught = true; } if (!caught) throw new Error("invalid exit accepted");`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := newTerminalLifecycleEngine(t, context.Background())
			if err := engine.ExecuteScript(engine.LoadScriptString("metadata.js", tc.script)); err != nil {
				t.Fatalf("ExecuteScript: %v", err)
			}
			if err := engine.Ctx().Err(); err != nil {
				t.Fatalf("live engine was cancelled by metadata or rejected exit: %v", err)
			}
			if engine.runtime.processExitRequested.Load() {
				t.Fatal("metadata or an invalid exit left processExitRequested set")
			}
			if err := engine.ExecuteScript(engine.LoadScriptString("continues.js", `var stillRunning = true;`)); err != nil {
				t.Fatalf("subsequent script did not run: %v", err)
			}
			if value := engine.GetGlobal("stillRunning"); value != true {
				t.Fatalf("subsequent script result = %v, want true", value)
			}
			waitTerminalLifecycle(t, engine)
			if code, set := engine.ExitCode(); code != tc.code || set != tc.codeSet {
				t.Fatalf("ExitCode = (%d, %v), want (%d, %v)", code, set, tc.code, tc.codeSet)
			}
			if engine.Ctx().Err() == nil {
				t.Fatal("natural auto-exit did not cancel dependent lifecycle")
			}
		})
	}
}

func TestEngine_HostCancellationReleasesIdleTea(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live event loops and BubbleTea programs")
	}
	for _, tc := range []struct {
		name, mode string
		codeSet    bool
		scriptExit bool
	}{
		{name: "parent", mode: "parent", codeSet: true},
		{name: "engine_close", mode: "engine_close", codeSet: true},
		{name: "runtime_close", mode: "runtime_close", codeSet: true},
		{name: "raw_loop_close", mode: "loop_close"},
		{name: "raw_loop_shutdown", mode: "loop_shutdown"},
		// Raw loop termination publishes no host cancellation cause. A clean
		// return therefore completes with a recorded script status; callers
		// needing cancellation must use Engine.Close or Runtime.Close.
		{name: "raw_loop_close_assigned_status", mode: "loop_close", codeSet: true, scriptExit: true},
		{name: "raw_loop_shutdown_assigned_status", mode: "loop_shutdown", codeSet: true, scriptExit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, cancel := newTerminalLifecycleEngine(t, context.Background())
			setup := ""
			if tc.codeSet {
				setup = `process.exitCode = 9;`
			}
			execDone := startTerminalLifecycleTea(t, engine, setup, "")
			closeDone := make(chan error, 1)
			go func() {
				switch tc.mode {
				case "parent":
					cancel()
					closeDone <- nil
				case "engine_close":
					closeDone <- engine.Close()
				case "runtime_close":
					closeDone <- engine.runtime.Close()
				case "loop_close":
					closeDone <- engine.Loop().Close()
				case "loop_shutdown":
					ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
					defer stop()
					closeDone <- engine.Loop().Shutdown(ctx)
				}
			}()
			err := terminalLifecycleResult(t, execDone)
			wantCause := error(context.Canceled)
			if tc.scriptExit {
				wantCause = errRuntimeExit
				if err != nil {
					t.Fatalf("raw loop completion with script status = %v, want nil", err)
				}
			} else if !errors.Is(err, context.Canceled) {
				t.Fatalf("ExecuteScript = %v, want cancellation", err)
			}
			if err := terminalLifecycleResult(t, closeDone); err != nil {
				t.Fatalf("host termination: %v", err)
			}
			waitTerminalLifecycle(t, engine)
			if cause := context.Cause(engine.Ctx()); cause != wantCause {
				t.Fatalf("engine cancellation cause = %v, want %v", cause, wantCause)
			}
			if code, set := engine.ExitCode(); set != tc.codeSet || (set && code != 9) {
				t.Fatalf("ExitCode = (%d, %v), want assigned status preserved (set=%v)", code, set, tc.codeSet)
			}
		})
	}
}

func TestEngine_TerminalExitDoesNotEraseScriptErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a live JavaScript engine")
	}
	engine, _ := newTerminalLifecycleEngine(t, context.Background())
	err := engine.ExecuteScript(engine.LoadScriptString("failure.js", `throw new Error("real script failure");`))
	if err == nil || !strings.Contains(err.Error(), "real script failure") {
		t.Fatalf("ExecuteScript = %v, want actual script failure", err)
	}
	// Publish a real terminal exit, then verify that normalization recognizes
	// only host cancellation, not arbitrary callback failures wrapping it.
	if err := engine.runtime.RunSync(context.Background(), func(vm *goja.Runtime) error {
		_, err := vm.RunString(`setTimeout(function() { process.exit(0); }, 0);`)
		return err
	}); err != nil {
		t.Fatalf("schedule process.exit: %v", err)
	}
	waitTerminalLifecycle(t, engine)
	for _, err := range []error{
		errors.New("real callback failure"),
		fmt.Errorf("callback failed: %w", context.Canceled),
		fmt.Errorf("callback failed: %w", goeventloop.ErrLoopTerminated),
	} {
		if got := engine.scriptExecutionError(err); got != err {
			t.Fatalf("scriptExecutionError(%v) = %v, want unchanged callback failure", err, got)
		}
	}
	if got := engine.scriptExecutionError(context.Canceled); got != nil {
		t.Fatalf("committed exit cancellation = %v, want nil", got)
	}
}

func TestEngine_TerminalCancellationPreservesFirstCause(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a live JavaScript engine")
	}
	for _, mode := range []string{"synchronous_exit", "idle_tea_exit"} {
		t.Run(mode, func(t *testing.T) {
			parent, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			engine, _ := newTerminalLifecycleEngine(t, parent)
			want := errors.New("caller cancellation before terminal owner return")
			engine.SetGlobal("__terminalLifecycleCancel", func() { cancel(want) })
			setup := `process.on("exit", function() { __terminalLifecycleCancel(); });`
			var err error
			if mode == "synchronous_exit" {
				err = engine.ExecuteScript(engine.LoadScriptString("cancel-during-exit.js", setup+`process.exit(7);`))
			} else {
				done := startTerminalLifecycleTea(t, engine, setup, `setTimeout(function() { process.exit(7); }, 0);`)
				err = terminalLifecycleResult(t, done)
			}
			if err == nil || engine.IsProcessExitSignal(err) {
				t.Fatalf("caller cancellation was mistaken for successful script exit: %v", err)
			}
			waitTerminalLifecycle(t, engine)
			if cause := context.Cause(engine.Ctx()); cause != want {
				t.Fatalf("engine cause = %v, want original caller cause %v", cause, want)
			}
		})
	}
}

func TestEngine_TerminalExitAfterTeaCompletion(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live event loops and BubbleTea programs")
	}
	for _, tc := range []struct {
		name, callback string
		code           int
		wantError      bool
	}{
		{name: "zero", callback: `process.exit(0);`},
		{name: "nonzero", callback: `process.exit(7);`, code: 7},
		{name: "callback_failure", callback: `setTimeout(function() { process.exit(7); }, 0); throw new Error("post-tea failure");`, code: 7, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := newTerminalLifecycleEngine(t, context.Background())
			script := engine.LoadScriptString("tea-completes-first.js", `
				var tea = require("osm:bubbletea");
				__postBubbleTeaExit = function() { `+tc.callback+` };
				tea.run(tea.newModel({
					init: function() { return [{}, tea.quit()]; },
					update: function(msg, model) { return [model, null]; },
					view: function() { return {content: ""}; }
				}));
			`)
			done := make(chan error, 1)
			go func() { done <- engine.ExecuteScript(script) }()
			err := terminalLifecycleResult(t, done)
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "post-tea failure") {
					t.Fatalf("ExecuteScript = %v, want actual post-tea error", err)
				}
			} else if err != nil {
				t.Fatalf("ExecuteScript = %v, want clean terminal status", err)
			}
			waitTerminalLifecycle(t, engine)
			if code, set := engine.ExitCode(); !set || code != tc.code {
				t.Fatalf("ExitCode = (%d, %v), want (%d, true)", code, set, tc.code)
			}
		})
	}
}

func TestEngine_HostCancellationInterruptsBusyJS(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live JavaScript engines")
	}
	for _, mode := range []string{"engine_close", "runtime_close", "parent"} {
		t.Run(mode, func(t *testing.T) {
			engine, cancel := newTerminalLifecycleEngine(t, context.Background())
			entered := make(chan struct{})
			engine.SetGlobal("__terminalLifecycleEntered", func() { close(entered) })
			done := make(chan error, 1)
			go func() {
				done <- engine.ExecuteScript(engine.LoadScriptString("busy.js", `__terminalLifecycleEntered(); while (true) {}`))
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("busy script did not enter its callback")
			}
			closeDone := make(chan error, 1)
			go func() {
				switch mode {
				case "engine_close":
					closeDone <- engine.Close()
				case "runtime_close":
					closeDone <- engine.runtime.Close()
				case "parent":
					cancel()
					closeDone <- nil
				}
			}()
			if err := terminalLifecycleResult(t, done); !errors.Is(err, context.Canceled) {
				t.Fatalf("busy ExecuteScript = %v, want cancellation", err)
			}
			if err := terminalLifecycleResult(t, closeDone); err != nil {
				t.Fatalf("host close: %v", err)
			}
			waitTerminalLifecycle(t, engine)
		})
	}
}

func TestRuntime_HostCancellationInterruptsBusyJS(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a live JavaScript runtime")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// An independent interrupt also releases a regressed implementation,
		// so a failing boundedness assertion cannot strand the test process.
		rt.vm.Interrupt(context.Canceled)
		cancel()
		select {
		case <-rt.done:
		case <-time.After(5 * time.Second):
			t.Error("runtime cleanup did not release the VM owner")
		}
	})
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- rt.RunSync(context.Background(), func(vm *goja.Runtime) error {
			if err := vm.Set("entered", func() { close(entered) }); err != nil {
				return err
			}
			_, err := vm.RunString(`entered(); while (true) {}`)
			return err
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("busy runtime did not enter its callback")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- rt.Close() }()
	if err := terminalLifecycleResult(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("busy RunSync = %v, want cancellation", err)
	}
	if err := terminalLifecycleResult(t, closeDone); err != nil {
		t.Fatalf("Runtime.Close: %v", err)
	}
}

func TestRuntime_ClosePublishesCancellationBeforeAutoExit(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a live JavaScript runtime")
	}
	rt, err := NewRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.LoadScript("assigned-status.js", `process.exitCode = 9;`); err != nil {
		t.Fatal(err)
	}
	// Delay the first host cancellation to expose the ordering boundary. If
	// Close has already released bootstrap, the owner can finish with status 9
	// before the delayed cancellation is published. If bootstrap is still held,
	// the cancellation retains precedence when the delay is released.
	entered, release := make(chan struct{}), make(chan struct{})
	var delayed atomic.Bool
	var releaseOnce sync.Once
	releaseCancel := func() { releaseOnce.Do(func() { close(release) }) }
	cancel := rt.cancel
	rt.cancel = func(cause error) {
		if cause == context.Canceled && delayed.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		cancel(cause)
	}
	t.Cleanup(func() {
		releaseCancel()
		cancel(context.Canceled)
		_ = rt.Close()
		select {
		case <-rt.done:
		case <-time.After(5 * time.Second):
			t.Error("runtime cleanup did not finish")
		}
	})
	done := make(chan error, 1)
	go func() { done <- rt.Close() }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not reach host cancellation")
	}
	select {
	case <-rt.bootstrapDone:
		// This is the regressed ordering. Let the already-unblocked owner
		// publish its outcome before allowing Close to cancel.
		select {
		case <-rt.done:
		case <-time.After(5 * time.Second):
			t.Error("owner did not finish after bootstrap release")
		}
	default:
	}
	releaseCancel()
	if err := terminalLifecycleResult(t, done); err != nil {
		t.Fatalf("Runtime.Close: %v", err)
	}
	if cause := context.Cause(rt.Ctx()); cause != context.Canceled {
		t.Fatalf("Runtime.Close lost host cancellation to idle auto-exit: %v", cause)
	}
}

func TestEngine_TerminalLifecycleAllowsBeforeExitRevival(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a live JavaScript engine")
	}
	engine, _ := newTerminalLifecycleEngine(t, context.Background())
	observed := make(chan error, 2)
	engine.SetGlobal("__terminalLifecycleAlive", func() { observed <- engine.Ctx().Err() })
	script := engine.LoadScriptString("before-exit.js", `
		process.once("beforeExit", function() {
			__terminalLifecycleAlive();
			setTimeout(function() { __terminalLifecycleAlive(); }, 0);
		});
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript: %v", err)
	}
	waitTerminalLifecycle(t, engine)
	for range 2 {
		select {
		case err := <-observed:
			if err != nil {
				t.Fatalf("beforeExit revival observed premature cancellation: %v", err)
			}
		default:
			t.Fatal("beforeExit or its scheduled timer was lost")
		}
	}
}

func TestEngine_TerminalLifecycleCancelsPendingFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("runs JavaScript engines and a loopback HTTP server")
	}
	for _, mode := range []string{"engine_close", "runtime_close", "async_process_exit"} {
		t.Run(mode, func(t *testing.T) {
			arrived, requestCancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(arrived)
				select {
				case <-r.Context().Done():
					close(requestCancelled)
				case <-release:
					_, _ = io.WriteString(w, "released by cleanup")
				}
			}))
			t.Cleanup(func() { releaseHandler(); server.Close() })
			parent := context.Background()
			engine, _ := newTerminalLifecycleEngine(t, parent)
			// Cleanup releases the server first, even if lifecycle cancellation
			// regresses. A passing test must never depend on this release.
			t.Cleanup(releaseHandler)
			script := engine.LoadScriptString("pending-fetch.js", fmt.Sprintf(`require("osm:fetch").fetch(%q, {timeout: 30}).catch(function() {});`, server.URL))
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("start fetch: %v", err)
			}
			select {
			case <-arrived:
			case <-time.After(5 * time.Second):
				t.Fatal("fetch did not reach the local server")
			}
			done := make(chan error, 1)
			if mode == "async_process_exit" {
				if err := engine.runtime.RunSync(context.Background(), func(vm *goja.Runtime) error {
					_, err := vm.RunString(`setTimeout(function() { process.exit(7); }, 0);`)
					return err
				}); err != nil {
					t.Fatalf("schedule exit: %v", err)
				}
				go func() { engine.Wait(); done <- nil }()
			} else {
				go func() {
					if mode == "engine_close" {
						done <- engine.Close()
					} else {
						done <- engine.runtime.Close()
					}
				}()
			}
			if err := terminalLifecycleResult(t, done); err != nil {
				t.Fatalf("terminal worker join: %v", err)
			}
			select {
			case <-requestCancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("terminal completion did not cancel the pending HTTP request")
			}
			if parent.Err() != nil {
				t.Fatal("engine shutdown cancelled its caller")
			}
			if mode == "async_process_exit" {
				if code, set := engine.ExitCode(); !set || code != 7 {
					t.Fatalf("ExitCode = (%d, %v), want (7, true)", code, set)
				}
			}
		})
	}
}
