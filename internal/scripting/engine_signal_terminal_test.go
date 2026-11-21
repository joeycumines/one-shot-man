package scripting

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// signalWaitAfterOwnerContext models a descheduled DeliverSignal waiter. Its
// Done evaluation resumes only after the real event-loop owner has exited;
// all cancellation, callback, and exit-state behavior remains production code.
type signalWaitAfterOwnerContext struct {
	context.Context
	ownerDone <-chan struct{}
}

func (c signalWaitAfterOwnerContext) Done() <-chan struct{} {
	select {
	case <-c.ownerDone:
	case <-time.After(5 * time.Second):
		panic("signal callback did not finish the real event loop")
	}
	return c.Context.Done()
}

func TestEngine_DeliverSignalPreservesTerminalStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live event loops")
	}
	for _, action := range []struct{ name, script string }{
		{"explicit_exit", "process.exit(7);"},
		{"assigned_exit_code", "process.exitCode = 7; clearInterval(keepAlive);"},
	} {
		t.Run(action.name, func(t *testing.T) {
			// Completion and cancellation are both ready once the waiter resumes.
			// Every select outcome must preserve successful delivery; repetition
			// exercises the legal alternatives without retrying a failing result.
			for i := 0; i < 32; i++ {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					engine, _ := newTerminalLifecycleEngine(t, context.Background())
					err := engine.ExecuteScript(engine.LoadScriptString("signal-terminal-proof.js", `
						var keepAlive = setInterval(function() {}, 1000);
						process.on("SIGTERM", function() { `+action.script+` });
					`))
					if err != nil {
						t.Fatalf("register signal listener: %v", err)
					}
					engine.ctx = signalWaitAfterOwnerContext{Context: engine.ctx, ownerDone: engine.runtime.done}
					waited := make(chan error, 1)
					go func() { engine.Wait(); waited <- nil }()
					if !engine.DeliverSignal("SIGTERM") {
						t.Errorf("listener exited successfully with code 7 but DeliverSignal returned false; cause=%v", context.Cause(engine.ctx))
					}
					terminalLifecycleResult(t, waited)
					if code, ok := engine.ExitCode(); !ok || code != 7 {
						t.Errorf("final status = (%d, %v), want (7, true)", code, ok)
					}
				})
			}
		})
	}
}

func TestEngine_DeliverSignalPreservesFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live event loops")
	}
	for _, tc := range []struct{ name, setup string }{
		{"no_listener", ``},
		{"listener_failure", `process.on("SIGTERM", function() { process.exitCode = 7; throw new Error("listener failure"); });`},
		{"host_cancellation", `process.on("SIGTERM", function() { __cancelSignalHost(); process.exit(7); });`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, cancel := newTerminalLifecycleEngine(t, context.Background())
			engine.SetGlobal("__cancelSignalHost", cancel)
			if err := engine.ExecuteScript(engine.LoadScriptString("signal-failure.js", `
				var keepAlive = setInterval(function() {}, 1000);
			`+tc.setup)); err != nil {
				t.Fatalf("register signal listener: %v", err)
			}
			if engine.DeliverSignal("SIGTERM") {
				t.Fatal("missing/failed listener or host cancellation was reported as successful delivery")
			}
		})
	}
}
