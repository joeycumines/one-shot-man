package btbridge

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

func TestBridge_NewAndStop(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	require.NotNil(t, bridge)
	require.True(t, bridge.IsRunning())

	bridge.Stop()
	require.False(t, bridge.IsRunning())

	// Stop should be idempotent
	bridge.Stop()
	require.False(t, bridge.IsRunning())
}

func TestBridge_WithContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	bridge, err := NewBridgeWithContext(ctx)
	require.NoError(t, err)
	require.True(t, bridge.IsRunning())

	// Cancel the context
	cancel()

	// Give it time to stop
	time.Sleep(10 * time.Millisecond)

	require.False(t, bridge.IsRunning())
}

func TestBridge_RunOnLoop(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Run code on the loop
	executed := make(chan bool, 1)
	ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
		executed <- true
	})
	require.True(t, ok)

	select {
	case <-executed:
		// Success
	case <-time.After(time.Second):
		t.Fatal("RunOnLoop callback not executed")
	}
}

func TestBridge_RunOnLoopAfterStop(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)

	bridge.Stop()

	// Should return false after stop
	ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
		t.Fatal("Should not execute after stop")
	})
	require.False(t, ok)
}

func TestBridge_LoadScript(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Load a simple script
	err = bridge.LoadScript("test.js", `
		function testFunc() {
			return 42;
		}
	`)
	require.NoError(t, err)

	// Verify the function exists
	val, err := bridge.GetGlobal("testFunc")
	require.NoError(t, err)
	require.NotNil(t, val)
}

func TestBridge_LoadScriptError(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Load invalid script
	err = bridge.LoadScript("bad.js", `this is not valid javascript {`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to compile")
}

func TestBridge_SetGetGlobal(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Set various types
	err = bridge.SetGlobal("intVal", 42)
	require.NoError(t, err)

	err = bridge.SetGlobal("strVal", "hello")
	require.NoError(t, err)

	err = bridge.SetGlobal("boolVal", true)
	require.NoError(t, err)

	// Get them back
	val, err := bridge.GetGlobal("intVal")
	require.NoError(t, err)
	require.Equal(t, int64(42), val)

	val, err = bridge.GetGlobal("strVal")
	require.NoError(t, err)
	require.Equal(t, "hello", val)

	val, err = bridge.GetGlobal("boolVal")
	require.NoError(t, err)
	require.Equal(t, true, val)

	// Non-existent global
	val, err = bridge.GetGlobal("nonexistent")
	require.NoError(t, err)
	require.Nil(t, val)
}

func TestBridge_ExposeBlackboard(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	bb := NewBlackboard()
	bb.Set("testKey", "testValue")

	err = bridge.ExposeBlackboard("myBlackboard", bb)
	require.NoError(t, err)

	// Access from JS
	err = bridge.LoadScript("test.js", `
		var val = myBlackboard.get("testKey");
		myBlackboard.set("fromJS", val + "_modified");
	`)
	require.NoError(t, err)

	// Verify the modification
	require.Equal(t, "testValue_modified", bb.Get("fromJS"))
}

func TestBridge_JSHelpers(t *testing.T) {
	t.Parallel()

	bridge, err := NewBridge()
	require.NoError(t, err)
	defer bridge.Stop()

	// Verify bt constants are available
	val, err := bridge.GetGlobal("bt")
	require.NoError(t, err)
	require.NotNil(t, val)

	btMap, ok := val.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "running", btMap["running"])
	require.Equal(t, "success", btMap["success"])
	require.Equal(t, "failure", btMap["failure"])

	// Verify runLeaf is available
	val, err = bridge.GetGlobal("runLeaf")
	require.NoError(t, err)
	require.NotNil(t, val)
}
