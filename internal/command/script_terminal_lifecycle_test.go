package command

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestScriptCommands_TerminalExitWithIdleTea(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live JavaScript engines and BubbleTea programs")
	}
	logger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(logger) })
	for _, entrypoint := range []string{"js_file", "script_command"} {
		for _, tc := range []struct {
			name, setup, action string
			code                int
		}{
			{name: "zero", action: `process.exit(0);`},
			{name: "nonzero", action: `process.exit(7);`, code: 7},
			{name: "fatal_rejection", action: `Promise.reject(new Error("terminal rejection"));`, code: 1},
			{name: "fatal_exception_handler", setup: `process.on("uncaughtException", function() { throw new Error("handler failed"); });`, action: `throw new Error("original failure");`, code: 7},
		} {
			t.Run(entrypoint+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				content := tc.setup + `
					var tea = require("osm:bubbletea");
					tea.run(tea.newModel({
						init: function() {
							setTimeout(function() { ` + tc.action + ` }, 0);
							return [{}, null];
						},
						update: function(msg, model) { return [model, null]; },
						view: function() { return {content: "idle"}; }
					}));
				`
				var execute func([]string, io.Writer, io.Writer) error
				if entrypoint == "js_file" {
					cmd := newExitChannelScriptCommand(t, content)
					cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }
					execute = cmd.Execute
				} else {
					cmd := NewScriptingCommand(nil)
					cmd.script, cmd.store, cmd.session, cmd.testMode = content, "memory", t.Name(), true
					cmd.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }
					execute = cmd.Execute
				}
				done := make(chan error, 1)
				go func() { done <- execute(nil, io.Discard, io.Discard) }()
				var err error
				select {
				case err = <-done:
				case <-time.After(5 * time.Second):
					cancel()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("caller cancellation did not release the command during cleanup")
					}
					t.Fatal("command did not complete after asynchronous terminal exit")
				}
				if tc.code == 0 {
					if err != nil {
						t.Fatalf("Execute = %v, want success for exit 0", err)
					}
					return
				}
				if _, ok := errors.AsType[*SilentError](err); !ok {
					t.Fatalf("Execute = %v, want SilentError with the script status", err)
				}
				if code, set := ExitCode(err); !set || code != tc.code {
					t.Fatalf("ExitCode(%v) = (%d, %v), want (%d, true)", err, code, set, tc.code)
				}
			})
		}
	}
}

func TestScriptCommands_NestedRejectedExitPreservesOuterRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("runs live JavaScript engines")
	}
	logger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(logger) })
	for _, mode := range []string{"synchronous_zero", "idle_tea_undefined"} {
		t.Run(mode, func(t *testing.T) {
			content := `
                process.on("exit", function() {
                    var rejected = false;
                    try { process.exit("bad"); }
                    catch (e) {
                        if (e.code !== "ERR_INVALID_ARG_TYPE") throw e;
                        rejected = true;
                    }
                    if (!rejected) throw new Error("nested invalid exit was not rejected");
                });
            `
			if mode == "synchronous_zero" {
				content += `process.exit(0);`
			} else {
				content += `
                    var tea = require("osm:bubbletea");
                    tea.run(tea.newModel({
                        init: function() {
                            setTimeout(function() { process.exit(undefined); }, 0);
                            return [{}, null];
                        },
                        update: function(msg, model) { return [model, null]; },
                        view: function() { return {content: "idle"}; }
                    }));
                `
			}
			command := newExitChannelScriptCommand(t, content)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			command.ctxFactory = func() (context.Context, context.CancelFunc) { return ctx, cancel }
			done := make(chan error, 1)
			go func() { done <- command.Execute(nil, io.Discard, io.Discard) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("a valid outer exit 0 was lost after a caught nested invalid exit: %v", err)
				}
			case <-time.After(5 * time.Second):
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("command remained blocked during cleanup")
				}
				t.Fatal("outer process.exit did not finish")
			}
		})
	}
}
