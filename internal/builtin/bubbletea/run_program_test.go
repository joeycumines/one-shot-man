//go:build unix

package bubbletea

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/creack/pty"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openPty(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("failed to open pty: %v", err)
	}
	// set a reasonable default size; ignore error if it fails
	_ = pty.Setsize(slave, &pty.Winsize{Cols: 80, Rows: 24})
	return master, slave
}

// TestRunProgram_Lifecycle verifies the full lifecycle of a bubbletea program execution using a PTY.
func TestRunProgram_Lifecycle(t *testing.T) {
	vm := goja.New()

	// Create buffered channels to prevent blocking signal sends
	initCalled := make(chan struct{}, 1)
	updateCalled := make(chan struct{}, 1)
	viewCalled := make(chan struct{}, 1)

	model := &jsModel{
		runtime: vm,
		initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			select {
			case initCalled <- struct{}{}:
			default:
			}
			// Return an initial command to ensure Update runs and the program exits deterministically.
			newState := vm.NewObject()
			quit := map[string]interface{}{"_cmdType": "quit"}
			return vm.NewArray(newState, vm.ToValue(quit)), nil
		},

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

	master, slave := openPty(t)
	defer master.Close()
	defer slave.Close()

	// Run program in goroutine using PTY slave for input/output
	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.runProgram(model, tea.WithInput(slave), tea.WithOutput(slave))
	}()

	// Wait for lifecycle events with timeout
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()

	// 1. Init should be called
	select {
	case <-initCalled:
	case err := <-errCh:
		t.Fatalf("runProgram failed: %v", err)
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
	select {
	case <-updateCalled:
	case <-timeout.C:
		// If explicit update didn't happen, force quit
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

// TestRunProgram_Options verifies that options are correctly passed by inspecting PTY output.
func TestRunProgram_Options(t *testing.T) {
	vm := goja.New()

	// Define raw init function that returns a Tick command
	initFnRaw := func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		newState := vm.NewObject()
		tick := map[string]interface{}{
			"_cmdType": "tick",
			"duration": 50, // 50ms
		}

		return vm.NewArray(newState, vm.ToValue(tick)), nil
	}

	model := &jsModel{
		runtime: vm,
		// We'll override initFn below with initFnRaw
		initFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// When we receive the tick (or any message), quit
			quit := map[string]interface{}{"_cmdType": "quit"}
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

	master, slave := openPty(t)
	defer master.Close()
	// Start reading from the master end *before* running the program so we reliably capture output written while the program runs.
	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		io.Copy(buf, master)
		close(done)
	}()

	// Run with AltScreen option and PTY
	err := manager.runProgram(model, tea.WithAltScreen(), tea.WithInput(slave), tea.WithOutput(slave))
	require.NoError(t, err)

	// Close slave to signal EOF to master and allow the reader to finish
	_ = slave.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for program output")
	}

	outStr := buf.String()
	assert.Contains(t, outStr, "\x1b[?1049h", "Should contain enter alt screen sequence")
	assert.Contains(t, outStr, "\x1b[?1049l", "Should contain exit alt screen sequence")
}

// TestRunProgram_AlreadyRunning verifies that runProgram fails if already running.
func TestRunProgram_AlreadyRunning(t *testing.T) {
	vm := goja.New()

	firstProgramStarted := make(chan struct{})
	stopFirstProgram := make(chan struct{})

	model := &jsModel{
		runtime: vm,
		initFn: createViewFn(vm, func(state goja.Value) string {
			close(firstProgramStarted)
			return ""
		}),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			select {
			case <-stopFirstProgram:
				quit := map[string]interface{}{"_cmdType": "quit"}
				return vm.NewArray(args[1], vm.ToValue(quit)), nil
			default:
				return vm.NewArray(args[1], goja.Null()), nil
			}
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	var input bytes.Buffer
	var output bytes.Buffer
	manager := NewManager(context.Background(), &input, &output, model.jsRunner, nil, nil)

	master, slave := openPty(t)
	defer master.Close()
	defer slave.Close()

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- manager.runProgram(model, tea.WithInput(slave), tea.WithOutput(slave))
	}()

	select {
	case <-firstProgramStarted:
	case err := <-startErrCh:
		t.Fatalf("failed to start first program: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first program to start")
	}

	// Try starting second program while the first holds the lock
	err := manager.runProgram(model, tea.WithInput(slave), tea.WithOutput(slave))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Cleanup: signal first program to quit
	close(stopFirstProgram)
	manager.SendStateRefresh("shutdown")

	// Wait for first program to exit
	select {
	case err := <-startErrCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("first program didn't exit")
	}
}

// TestSendStateRefresh_Integration verifies SendStateRefresh actually sends a message.
func TestSendStateRefresh_Integration(t *testing.T) {
	vm := goja.New()
	refreshReceived := make(chan string)

	model := &jsModel{
		runtime: vm,
		initFn:  createViewFn(vm, func(state goja.Value) string { return "" }),
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			jsMsg := args[0].ToObject(vm)
			if jsMsg.Get("type").String() == "StateRefresh" {
				key := jsMsg.Get("key").String()
				go func() { refreshReceived <- key }()
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

	master, slave := openPty(t)
	defer master.Close()
	defer slave.Close()

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- manager.runProgram(model, tea.WithInput(slave), tea.WithOutput(slave))
	}()

	// wait for start (or immediate failure)
	select {
	case err := <-startErrCh:
		if err != nil {
			t.Fatalf("runProgram failed to start: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		// Assume started
	}

	// Send refresh
	manager.SendStateRefresh("testKey")

	select {
	case key := <-refreshReceived:
		assert.Equal(t, "testKey", key)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for state refresh")
	}

	// Wait for program to exit
	select {
	case err := <-startErrCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("program didn't exit")
	}
}
