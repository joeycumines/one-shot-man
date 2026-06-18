package aimux

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/one-shot-man/internal/aimuxcore"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// eventStreamTestEnv sets up a JS environment with osm:aimux for testing
// the EventStream and HealthMonitor bindings directly.
func eventStreamTestEnv(t *testing.T) (*goja.Runtime, func(string) goja.Value) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping event stream binding test in -short mode")
	}

	reg := gojarequire.NewRegistry()
	loop, err := goeventloop.New()
	require.NoError(t, err)
	vm := goja.New()
	reg.Enable(vm)
	gojanodejsconsole.Enable(vm)
	adapter, err := gojaeventloop.New(loop, vm)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	ctx := context.Background()
	bridge := btmod.NewBridgeWithEventLoop(ctx, loop, vm, reg)
	t.Cleanup(func() { bridge.Stop() })

	reg.RegisterNativeModule("osm:aimux", Require(ctx, adapter))

	runJS := func(script string) goja.Value {
		t.Helper()
		var res goja.Value
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			var e error
			res, e = vm.RunString(script)
			return e
		})
		require.NoError(t, err, "JS execution failed: %s", script)
		return res
	}

	return vm, runJS
}

func TestBinding_EventStream_NewEventStream_NilHandle(t *testing.T) {
	t.Parallel()
	_, runJS := eventStreamTestEnv(t)

	// Calling newEventStream with null handle should throw.
	result := runJS(`
		try {
			require('osm:aimux').newEventStream(null, null);
			false;
		} catch (e) {
			e instanceof TypeError;
		}
	`)
	assert.True(t, result.ToBoolean(), "newEventStream(null, null) should throw TypeError")
}

func TestBinding_EventStream_NewEventStream_WithFakeHandle(t *testing.T) {
	t.Parallel()
	vm, runJS := eventStreamTestEnv(t)

	// Create a fake handle object with _handle pointing to a real aimuxcore handle.
	// We use the testutil fake handle via Go directly.
	handle := aimuxcore.NewProcessProvider("test", "echo", nil, aimuxcore.ProviderCapabilities{})

	// We can't easily create a real handle without spawning, so let's test
	// that newEventStream accepts a handle object and returns an object with
	// events() and close() methods.
	_ = handle

	// Instead, test via JS that the function exists and returns an object.
	result := runJS(`typeof require('osm:aimux').newEventStream`)
	assert.Equal(t, "function", result.String())

	// Test newHealthMonitor exists.
	result = runJS(`typeof require('osm:aimux').newHealthMonitor`)
	assert.Equal(t, "function", result.String())

	// Test that calling with a valid handle object works.
	// We need a handle with _handle property pointing to an AgentHandle.
	// Since we can't easily create one in JS, let's use the vm directly.
	handleObj := vm.NewObject()
	_ = handleObj.Set("_handle", &fakeAgentHandleForBinding{})

	runJS(`var aimux = require('osm:aimux');`)

	// Store the handle in globalThis for JS access.
	_ = vm.Set("testHandle", handleObj)

	result = runJS(`
		var es = aimux.newEventStream(testHandle, null);
		typeof es.events === 'function' && typeof es.close === 'function';
	`)
	assert.True(t, result.ToBoolean(), "newEventStream should return object with events() and close()")
}

func TestBinding_HealthMonitor_WithFakeHandle(t *testing.T) {
	t.Parallel()
	vm, runJS := eventStreamTestEnv(t)

	handleObj := vm.NewObject()
	fake := &fakeAgentHandleForBinding{}
	_ = handleObj.Set("_handle", fake)

	_ = vm.Set("testHandle", handleObj)

	runJS(`var aimux = require('osm:aimux');`)

	result := runJS(`
		var hm = aimux.newHealthMonitor(testHandle, 100);
		var snap = hm.snapshot();
		typeof snap.alive === 'boolean' &&
		typeof snap.lastEventMs === 'number' &&
		typeof snap.lastSendMs === 'number' &&
		typeof hm.close === 'function';
	`)
	assert.True(t, result.ToBoolean(), "newHealthMonitor should return object with snapshot() and close()")

	// Verify the snapshot reflects the fake handle's state.
	result = runJS(`
		var hm2 = aimux.newHealthMonitor(testHandle, 100);
		var snap = hm2.snapshot();
		snap.alive;
	`)
	assert.True(t, result.ToBoolean(), "snapshot.alive should be true for fake handle")

	// Close the health monitor.
	runJS(`hm.close(); hm2.close();`)
}

func TestBinding_EventStream_ConstantsExported(t *testing.T) {
	t.Parallel()
	_, runJS := eventStreamTestEnv(t)

	// Verify that all event type constants are exported.
	result := runJS(`
		var aimux = require('osm:aimux');
		typeof aimux.EVENT_TEXT === 'number' &&
		typeof aimux.EVENT_RATE_LIMIT === 'number' &&
		typeof aimux.EVENT_PERMISSION === 'number' &&
		typeof aimux.EVENT_MODEL_SELECT === 'number' &&
		typeof aimux.EVENT_SSO_LOGIN === 'number' &&
		typeof aimux.EVENT_COMPLETION === 'number' &&
		typeof aimux.EVENT_TOOL_USE === 'number' &&
		typeof aimux.EVENT_ERROR === 'number' &&
		typeof aimux.EVENT_THINKING === 'number';
	`)
	assert.True(t, result.ToBoolean(), "all EVENT_* constants should be exported as numbers")

	// Verify state constants.
	result = runJS(`
		var aimux = require('osm:aimux');
		typeof aimux.STATE_INITIALIZING === 'number' &&
		typeof aimux.STATE_READY === 'number' &&
		typeof aimux.STATE_PROCESSING === 'number' &&
		typeof aimux.STATE_RESPONDING === 'number' &&
		typeof aimux.STATE_ERROR === 'number' &&
		typeof aimux.STATE_RATE_LIMITED === 'number' &&
		typeof aimux.STATE_PERMISSION_PROMPT === 'number';
	`)
	assert.True(t, result.ToBoolean(), "all STATE_* constants should be exported as numbers")
}

func TestBinding_EventStream_NewParser(t *testing.T) {
	t.Parallel()
	_, runJS := eventStreamTestEnv(t)

	result := runJS(`
		var aimux = require('osm:aimux');
		var parser = aimux.newParser();
		typeof parser.parse === 'function' &&
		typeof parser.patterns === 'function' &&
		typeof parser.addPattern === 'function';
	`)
	assert.True(t, result.ToBoolean(), "newParser should return object with parse, patterns, addPattern")

	// Test that parse returns a proper event object.
	result = runJS(`
		var aimux = require('osm:aimux');
		var parser = aimux.newParser();
		var ev = parser.parse('Error: something went wrong');
		ev.type === aimux.EVENT_ERROR && ev.line === 'Error: something went wrong';
	`)
	assert.True(t, result.ToBoolean(), "parser.parse should detect error events")
}

func TestBinding_EventStream_NewTUIStateMachine(t *testing.T) {
	t.Parallel()
	_, runJS := eventStreamTestEnv(t)

	result := runJS(`
		var aimux = require('osm:aimux');
		var sm = aimux.newTUIStateMachine();
		typeof sm.processOutput === 'function' &&
		typeof sm.checkTimeout === 'function' &&
		typeof sm.state === 'function' &&
		typeof sm.stateName === 'function' &&
		typeof sm.reset === 'function';
	`)
	assert.True(t, result.ToBoolean(), "newTUIStateMachine should return object with all methods")

	// Initial state should be Initializing.
	result = runJS(`
		var aimux = require('osm:aimux');
		var sm = aimux.newTUIStateMachine();
		sm.stateName();
	`)
	assert.Equal(t, "Initializing", result.String(), "initial state should be Initializing")
}

func TestBinding_EventStream_handleFromJS_NilHandle(t *testing.T) {
	t.Parallel()
	vm, _ := eventStreamTestEnv(t)

	// Test that handleFromJS returns nil for various invalid inputs.
	assert.Nil(t, handleFromJS(vm, goja.Null()))
	assert.Nil(t, handleFromJS(vm, goja.Undefined()))
	assert.Nil(t, handleFromJS(vm, vm.NewObject())) // Object without _handle
}

// fakeAgentHandleForBinding is a minimal AgentHandle for testing JS bindings.
type fakeAgentHandleForBinding struct{}

func (h *fakeAgentHandleForBinding) Send(input string) error             { return nil }
func (h *fakeAgentHandleForBinding) Receive() (string, error)            { return "", nil }
func (h *fakeAgentHandleForBinding) Close() error                        { return nil }
func (h *fakeAgentHandleForBinding) IsAlive() bool                       { return true }
func (h *fakeAgentHandleForBinding) Wait() (int, error)                  { return 0, nil }
func (h *fakeAgentHandleForBinding) Resize(rows, cols int) error         { return nil }
func (h *fakeAgentHandleForBinding) WaitReady(ctx context.Context) error { return nil }
func (h *fakeAgentHandleForBinding) Events() <-chan aimuxcore.LineEvent {
	return make(chan aimuxcore.LineEvent)
}
func (h *fakeAgentHandleForBinding) Health() aimuxcore.HealthSnapshot {
	return aimuxcore.HealthSnapshot{
		Alive:     true,
		LastEvent: time.Now(),
		LastSend:  time.Now(),
	}
}
