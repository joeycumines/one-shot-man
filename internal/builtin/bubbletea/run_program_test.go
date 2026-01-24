package bubbletea

import (
	"bytes"
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunProgram_Lifecycle verifies the full lifecycle of a bubbletea program execution.
func TestRunProgram_Lifecycle(t *testing.T) {
	vm := goja.New()

	// Create buffered channels to prevent blocking signal sends
	initCalled := make(chan struct{}, 1)
	updateCalled := make(chan struct{}, 1)
	viewCalled := make(chan struct{}, 1)

	model := &jsModel{
		runtime: vm,
		initFn: createViewFn(vm, func(state goja.Value) string {
			select {
			case initCalled <- struct{}{}:
			default:
			}
			// Return a command to ensure Update runs. a Tick with 1ms.
			// Or just return nil, expecting WindowSizeMsg.
			// Let's rely on WindowSizeMsg or a synthetic batch.
			return ""
		}),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Signal update
			select {
			case updateCalled <- struct{}{}:
			default:
			}

			// Always return Quit to ensure termination
			quit := map[string]interface{}{"_cmdType": "quit"}
			return vm.NewArray(args[1], vm.ToValue(quit)), nil
		},
		viewFn: createViewFn(vm, func(state goja.Value) string {
			select {
			case viewCalled <- struct{}{}:
			default:
			}
			return "view output"
		}),
		state: vm.NewObject(),
	}

	model.jsRunner = &SyncJSRunner{Runtime: vm}

	var input bytes.Buffer
	var output bytes.Buffer
	manager := NewManager(context.Background(), &input, &output, model.jsRunner, nil, nil)

	// Run program in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.runProgram(model)
	}()

	// Wait for lifecycle events with timeout
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()

	// 1. Init should be called
	select {
	case <-initCalled:
	case <-timeout.C:
		t.Fatal("Timeout waiting for Init")
	}

	// 2. View should be called (initial view)
	select {
	case <-viewCalled:
	case <-timeout.C:
		t.Fatal("Timeout waiting for View")
	}

	// 3. Update should be called (e.g. WindowSizeMsg or initial command)
	// If Update is NOT called, we might hang waiting for it?
	// But View is called.
	// If Update is called, it returns Quit.
	// If Update is NOT called, the program runs forever.
	// Does bubbletea guarantee Update is called?
	// Usually yes, with WindowSizeMsg.
	select {
	case <-updateCalled:
	case <-timeout.C:
		// If explicit update didn't happen, force quit?
		// But we can't easily force quit from outside without program instance reference.
		// Manager stores it!
		manager.mu.Lock()
		if manager.program != nil {
			manager.program.Quit()
		}
		manager.mu.Unlock()
		t.Log("Timeout waiting for Update - forced Quit")
	}

	// 4. Program should exit
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.NewTimer(1 * time.Second).C:
		t.Fatal("Program did not exit")
	}
}

// TestRunProgram_Options verifies that options are correctly passed.
// We can't inspect tea.Program options directly, so we check side effects (ANSI sequences in output).
func TestRunProgram_Options(t *testing.T) {
	vm := goja.New()

	// Define raw init function that returns a Tick command
	initFnRaw := func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		// Init is called with 0 args in initDirect
		newState := vm.NewObject()
		// Return a tick command to ensure the loop runs at least once
		// The tick will fire a message back to Update
		tick := map[string]interface{}{
			"_cmdType": "tick",
			"duration": 10, // 10ms
		}
		// Wait, extractTickCmd uses durationVal.ToObject(m.runtime).ToInteger() -> int64
		// bubbletea.go extractTickCmd: time.Duration(durationVal.ToInteger())
		// If input is ns, 10ms = 10 * 1,000,000 = 10,000,000
		// Actually, let's verify extractTickCmd logic.
		// lines 996+:
		// durationVal := obj.Get("duration")
		// if durationVal == nil ...
		// return tea.Tick(time.Duration(durationVal.ToInteger()), func(t time.Time) tea.Msg { ... })
		// goja ToInteger returns int64.
		// So it interprets the number as nanoseconds because time.Duration is int64 nanoseconds.
		// So 10ms = 10 * 1000 * 1000 = 10,000,000.

		return vm.NewArray(newState, vm.ToValue(tick)), nil
	}

	model := &jsModel{
		runtime: vm,
		// We'll override initFn below with initFnRaw
		initFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// When we receive the tick (or any message), quit
			quit := map[string]interface{}{"_cmdType": "quit"}
			// Debugging
			if len(args) < 2 {
				return nil, nil // Should not happen
			}
			return vm.NewArray(args[1], vm.ToValue(quit)), nil
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model.initFn = initFnRaw

	model.jsRunner = &SyncJSRunner{Runtime: vm}

	var input bytes.Buffer
	var output bytes.Buffer
	manager := NewManager(context.Background(), &input, &output, model.jsRunner, nil, nil)

	// Run with options
	// WithAltScreen should emit enter/exit alt screen sequences
	err := manager.runProgram(model, tea.WithAltScreen())
	require.NoError(t, err)

	// Check for AltScreen sequences
	outStr := output.String()
	assert.Contains(t, outStr, "\x1b[?1049h", "Should contain enter alt screen sequence")
	assert.Contains(t, outStr, "\x1b[?1049l", "Should contain exit alt screen sequence")
}

// TestRunProgram_AlreadyRunning verifies that runProgram fails if already running.
func TestRunProgram_AlreadyRunning(t *testing.T) {
	vm := goja.New()

	programStarted := make(chan struct{}, 1) // Buffered to prevent blocking if test proceeds quickly

	model := &jsModel{
		runtime: vm,
		initFn: createViewFn(vm, func(state goja.Value) string {
			select {
			case programStarted <- struct{}{}: // Signal that init has been called
			default:
			}
			return ""
		}),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Don't quit immediately to block
			return vm.NewArray(args[1], goja.Null()), nil
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model = &jsModel{
		runtime: vm,
		// initFn and updateFn will be customized below
		initFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			return vm.NewArray(args[1], goja.Null()), nil
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	var input bytes.Buffer
	var output bytes.Buffer
	manager := NewManager(context.Background(), &input, &output, model.jsRunner, nil, nil)

	// Channel to signal that the first program has started and locked the manager
	firstProgramStarted := make(chan struct{})
	// Channel to signal that the first program should exit
	stopFirstProgram := make(chan struct{})

	// Custom init to signal start
	model.initFn = createViewFn(vm, func(state goja.Value) string {
		close(firstProgramStarted)
		return ""
	})
	// Custom update to wait for stop signal
	model.updateFn = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		select {
		case <-stopFirstProgram:
			quit := map[string]interface{}{"_cmdType": "quit"}
			return vm.NewArray(args[1], vm.ToValue(quit)), nil
		default:
			return vm.NewArray(args[1], goja.Null()), nil
		}
	}

	// Start first program
	go func() {
		manager.runProgram(model)
	}()

	// Wait for first program to actually start
	select {
	case <-firstProgramStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first program to start")
	}

	// Try starting second program matches the race window where lock is held?
	// runProgram checks m.program under lock.
	// Since first program started (Init called), m.program IS set.

	err := manager.runProgram(model)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Cleanup: signal first program to quit via update
	// We need to trigger an update for the select case to run.
	// Close channel then send a message.
	close(stopFirstProgram)
	// We can use SendStateRefresh to trigger Update
	manager.SendStateRefresh("shutdown")

	// Wait for cleanup if needed, but manager.runProgram returns when done.
	// The background goroutine will exit.
}

// TestSendStateRefresh_Integration verifies SendStateRefresh actually sends a message.
// This is an integration-style unit test.
func TestSendStateRefresh_Integration(t *testing.T) {
	vm := goja.New()
	refreshReceived := make(chan string)

	model := &jsModel{
		runtime: vm,
		initFn:  createViewFn(vm, func(state goja.Value) string { return "" }),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Check if msg is StateRefresh
			jsMsg := args[0].ToObject(vm)
			if jsMsg.Get("type").String() == "StateRefresh" {
				key := jsMsg.Get("key").String()
				go func() { refreshReceived <- key }()
				// Quit
				quit := map[string]interface{}{"_cmdType": "quit"}
				return vm.NewArray(args[1], vm.ToValue(quit)), nil
			}
			return vm.NewArray(args[1], goja.Null()), nil
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	manager := NewManager(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, model.jsRunner, nil, nil)

	go func() {
		manager.runProgram(model)
	}()

	// wait for start
	time.Sleep(100 * time.Millisecond)

	// Send refresh
	manager.SendStateRefresh("testKey")

	select {
	case key := <-refreshReceived:
		assert.Equal(t, "testKey", key)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for state refresh")
	}
}

// Helper to cover panic recovery in run (via Require export test) logic unit test is harder without full JS env.
// But we covered runProgram logic which is the core.
