package builtin

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestRegister_BubbleteaWaitForProgramUsesRegisteredAdapter(t *testing.T) {
	t.Parallel()

	eventLoopProvider := testutil.NewTestEventLoopProvider()
	t.Cleanup(eventLoopProvider.Stop)

	registry := eventLoopProvider.Registry()
	runtime := eventLoopProvider.Runtime()

	settled := make(chan string, 1)
	notify := func(state string) {
		select {
		case settled <- state:
		default:
		}
	}
	if err := eventLoopProvider.Loop().Submit(func() {
		Register(context.Background(), nil, registry, &mockTerminalProvider{
			reader: strings.NewReader(""),
			writer: io.Discard,
		}, eventLoopProvider)
		exports, err := registry.Enable(runtime).Require("osm:bubbletea")
		if err != nil {
			notify("require: " + err.Error())
			return
		}
		if err := runtime.Set("__registeredWaitNotify", notify); err != nil {
			notify("setup: " + err.Error())
			return
		}
		waitForProgram, ok := goja.AssertFunction(exports.ToObject(runtime).Get("waitForProgram"))
		if !ok {
			notify("setup: waitForProgram is not a function")
			return
		}
		promise, err := waitForProgram(goja.Undefined())
		if err != nil {
			notify("call: " + err.Error())
			return
		}
		if err := runtime.Set("__registeredWaitPromise", promise); err != nil {
			notify("setup: " + err.Error())
			return
		}
		if _, err := runtime.RunString(`__registeredWaitPromise.then(
			function () { __registeredWaitNotify("resolved"); },
			function (e) { __registeredWaitNotify("rejected: " + e); }
		);`); err != nil {
			notify("setup: " + err.Error())
		}
	}); err != nil {
		t.Fatalf("submit waitForProgram call: %v", err)
	}

	select {
	case state := <-settled:
		if state != "resolved" {
			t.Fatalf("waitForProgram settlement = %q, want resolved", state)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("registered waitForProgram did not settle within 5s")
	}
}
