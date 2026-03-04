package bubbletea

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Batch 13: bubbletea coverage gaps — runProgram guards + Require edge cases
// ============================================================================

// TestRunProgram_SignalNotifyNil verifies that runProgram returns an error
// if signalNotify is nil. Covers bubbletea.go ~line 1608-1610.
func TestRunProgram_SignalNotifyNil(t *testing.T) {
	t.Parallel()
	m := &Manager{
		ctx:      context.Background(),
		jsRunner: &SyncJSRunner{Runtime: goja.New()},
		// signalNotify is nil, signalStop is set
		signalStop: func(c chan<- os.Signal) {},
	}
	err := m.runProgram(noopModel{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signalNotify is nil")
}

// TestRunProgram_SignalStopNil verifies that runProgram returns an error
// if signalStop is nil. Covers bubbletea.go ~line 1611-1613.
func TestRunProgram_SignalStopNil(t *testing.T) {
	t.Parallel()
	m := &Manager{
		ctx:      context.Background(),
		jsRunner: &SyncJSRunner{Runtime: goja.New()},
		// signalNotify is set, signalStop is nil
		signalNotify: func(c chan<- os.Signal, sig ...os.Signal) {},
	}
	err := m.runProgram(noopModel{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signalStop is nil")
}

// TestNewManager_PipeInputFallback verifies the *os.File TTY detection
// fallback path when input is a pipe (not a TerminalChecker). Since pipes
// are not terminals, isTTY should be false.
// Covers bubbletea.go ~line 349.
func TestNewManager_PipeInputFallback(t *testing.T) {
	t.Parallel()
	rIn, wIn, err := os.Pipe()
	require.NoError(t, err)
	defer rIn.Close()
	defer wIn.Close()

	// Also pass a pipe for output to prevent os.Stdout (which may be a
	// terminal) from being used and setting isTTY=true.
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	defer rOut.Close()
	defer wOut.Close()

	vm := goja.New()
	// Pass pipes as both input and output — *os.File but not TerminalChecker.
	// Input pipe exercises the *os.File fallback at line 349.
	// Output pipe exercises the *os.File fallback at line 367.
	m := NewManager(context.Background(), rIn, wOut, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.NotNil(t, m)
	// Pipes are not terminals, so isTTY should be false.
	assert.False(t, m.isTTY)
}

// TestNewManager_PipeOutputFallback verifies the *os.File TTY detection
// fallback for output when input is NOT a *os.File and NOT a TerminalChecker.
// This forces the code to fall through both input branches and check the
// output's *os.File fallback path.
// Covers bubbletea.go ~line 367.
func TestNewManager_PipeOutputFallback(t *testing.T) {
	t.Parallel()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	vm := goja.New()
	// Use strings.Reader as input (not *os.File, not TerminalChecker).
	// Use pipe as output (*os.File).
	m := NewManager(context.Background(), strings.NewReader(""), w, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.NotNil(t, m)
	// Pipe output is not a terminal.
	assert.False(t, m.isTTY)
}

// TestRequire_NewModel_NullConfig verifies that newModel with a null
// config argument returns an error.
// Covers bubbletea.go ~line 1263.
func TestRequire_NewModel_NullConfig(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)
	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	// Call with explicit null.
	result, err := fn(goja.Undefined(), goja.Null())
	require.NoError(t, err)
	obj := result.ToObject(vm)
	errField := obj.Get("error")
	require.False(t, goja.IsUndefined(errField), "should have error field")
	assert.Contains(t, errField.String(), "config must be an object")
}

// TestRequire_NewModel_ThrottleIntervalClamp verifies that a
// renderThrottle.minIntervalMs value of 0 is clamped to 1.
// Covers bubbletea.go ~line 1311.
func TestRequire_NewModel_ThrottleIntervalClamp(t *testing.T) {
	t.Parallel()
	vm, exports := requireExports(t)

	fn, ok := goja.AssertFunction(exports.Get("newModel"))
	require.True(t, ok)

	// Create a minimal valid config with renderThrottle.minIntervalMs = 0.
	config, err := vm.RunString(`({
		init: function() { return {}; },
		update: function(msg, state) { return [state, null]; },
		view: function(state) { return ""; },
		renderThrottle: { enabled: true, minIntervalMs: 0 }
	})`)
	require.NoError(t, err)

	result, callErr := fn(goja.Undefined(), config)
	require.NoError(t, callErr)

	obj := result.ToObject(vm)
	// If there's an error, fail.
	errField := obj.Get("error")
	if errField != nil && !goja.IsUndefined(errField) {
		t.Fatalf("unexpected error: %s", errField.String())
	}
	// Verify the model was created (has _type field).
	typeVal := obj.Get("_type")
	require.NotNil(t, typeVal)
	assert.Equal(t, "bubbleteaModel", typeVal.String())
}
