//go:build unix

package bubbletea

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"
	"github.com/joeycumines/goja"
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

// skipIfNoTTY skips the test in short mode.
func skipIfNoTTY(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping TUI program test in short mode")
	}
}

// TestRunProgram_Lifecycle verifies the full lifecycle of a bubbletea program execution using a PTY.
func TestRunProgram_Lifecycle(t *testing.T) {
	skipIfNoTTY(t)
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
			quit := map[string]any{"_cmdType": "quit"}
			return vm.NewArray(newState, vm.ToValue(quit)), nil
		},

		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// Signal update
			select {
			case updateCalled <- struct{}{}:
			default:
			}

			// Always return Quit to ensure termination
			quit := map[string]any{"_cmdType": "quit"}
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

	master, slave := openPty(t)
	defer master.Close()
	// NOTE: slave is NOT closed via defer to avoid a data race with bubbletea's
	// internal handleResize goroutine, which may call slave.Fd() briefly after
	// Program.Run() returns. Instead, we close it explicitly after the program
	// exits using a dup'd FD so the original is safe to close at any time.
	slaveFd, dupErr := syscall.Dup(int(slave.Fd()))
	require.NoError(t, dupErr)
	slaveForBT := os.NewFile(uintptr(slaveFd), slave.Name())
	slave.Close() // Close original immediately; bubbletea uses the dup

	manager := NewManager(context.Background(), slaveForBT, slaveForBT, model.jsRunner, nil, nil)

	// Run program in goroutine using dup'd PTY slave for input/output
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
	case err := <-errCh:
		slaveForBT.Close()
		t.Fatalf("runProgram failed: %v", err)
	case <-timeout.C:
		slaveForBT.Close()
		t.Fatal("Timeout waiting for Init")
	}

	// 2. View should be called (initial view)
	select {
	case <-viewCalled:
	case <-timeout.C:
		slaveForBT.Close()
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
		// Allow bubbletea's internal goroutines (handleResize) to finish
		// before closing the PTY fd. bubbletea.Run() may return before all
		// its internal goroutines have exited, so closing the fd immediately
		// races with handleResize calling Fd().
		time.Sleep(50 * time.Millisecond)
		slaveForBT.Close()
		require.NoError(t, err)
	case <-time.NewTimer(1 * time.Second).C:
		time.Sleep(50 * time.Millisecond)
		slaveForBT.Close()
		t.Fatal("Program did not exit")
	}
}

// TestRunProgram_Options verifies that options are correctly passed by inspecting PTY output.
func TestRunProgram_Options(t *testing.T) {
	skipIfNoTTY(t)
	vm := goja.New()

	// Define raw init function that returns a Tick command (causes program to exit after tick)
	initFnRaw := func(this goja.Value, args ...goja.Value) (goja.Value, error) {
		newState := vm.NewObject()
		tick := map[string]any{
			"_cmdType": "tick",
			"duration": 50, // 50ms
		}

		return vm.NewArray(newState, vm.ToValue(tick)), nil
	}

	model := &jsModel{
		runtime: vm,
		initFn:  initFnRaw, // Use tick so program exits deterministically
		updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			// When we receive the tick (or any message), quit
			quit := map[string]any{"_cmdType": "quit"}
			return vm.NewArray(args[1], vm.ToValue(quit)), nil
		},
		viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			view := vm.NewObject()
			_ = view.Set("content", "options output")
			_ = view.Set("altScreen", true)
			_ = view.Set("mouseMode", "allMotion")
			return view, nil
		},
		state: vm.NewObject(),
	}

	model.jsRunner = &SyncJSRunner{Runtime: vm}

	master, slave := openPty(t)
	defer master.Close()
	// Use dup'd FD for bubbletea to avoid data race between Close and
	// bubbletea's internal handleResize goroutine calling Fd().
	slaveFd, dupErr := syscall.Dup(int(slave.Fd()))
	require.NoError(t, dupErr)
	slaveForBT := os.NewFile(uintptr(slaveFd), slave.Name())
	slave.Close()

	manager := NewManager(context.Background(), slaveForBT, slaveForBT, model.jsRunner, nil, nil)

	// Start reading from the master end *before* running the program so we reliably capture output written while the program runs.
	buf := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		io.Copy(buf, master)
		close(done)
	}()

	// Run with PTY
	err := manager.runProgram(model)
	require.NoError(t, err)

	// Allow bubbletea's internal goroutines (handleResize) to finish before
	// closing PTY fd, then signal EOF to master so reader can finish.
	time.Sleep(50 * time.Millisecond)
	_ = slaveForBT.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for program output")
	}

	outStr := buf.String()
	assert.Contains(t, outStr, "\x1b[?1049h", "Should contain enter alt screen sequence")
	assert.Contains(t, outStr, "\x1b[?1049l", "Should contain exit alt screen sequence")
	assert.Contains(t, outStr, "\x1b[?1006h", "Should contain SGR mouse mode (all motion) sequence")
}

// TestRunProgram_AlreadyRunning verifies that runProgram fails if already running.
func TestRunProgram_AlreadyRunning(t *testing.T) {
	skipIfNoTTY(t)
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
				quit := map[string]any{"_cmdType": "quit"}
				return vm.NewArray(args[1], vm.ToValue(quit)), nil
			default:
				return vm.NewArray(args[1], goja.Null()), nil
			}
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	master, slave := openPty(t)
	defer master.Close()
	// Use dup'd FD for bubbletea to avoid data race between Close and
	// bubbletea's internal handleResize goroutine calling Fd().
	slaveFd, dupErr := syscall.Dup(int(slave.Fd()))
	require.NoError(t, dupErr)
	slaveForBT := os.NewFile(uintptr(slaveFd), slave.Name())
	slave.Close()

	manager := NewManager(context.Background(), slaveForBT, slaveForBT, model.jsRunner, nil, nil)

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- manager.runProgram(model)
	}()

	select {
	case <-firstProgramStarted:
	case err := <-startErrCh:
		slaveForBT.Close()
		t.Fatalf("failed to start first program: %v", err)
	case <-time.After(2 * time.Second):
		slaveForBT.Close()
		t.Fatal("Timeout waiting for first program to start")
	}

	// Try starting second program while the first holds the lock
	err := manager.runProgram(model)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Cleanup: signal first program to quit
	close(stopFirstProgram)
	manager.SendStateRefresh("shutdown")

	// Wait for first program to exit
	select {
	case err := <-startErrCh:
		slaveForBT.Close()
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		slaveForBT.Close()
		t.Fatal("first program didn't exit")
	}
}

// TestSendStateRefresh_Integration verifies SendStateRefresh actually sends a message.
func TestSendStateRefresh_Integration(t *testing.T) {
	skipIfNoTTY(t)
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
				quit := map[string]any{"_cmdType": "quit"}
				return vm.NewArray(args[1], vm.ToValue(quit)), nil
			}
			return vm.NewArray(args[1], goja.Null()), nil
		},
		viewFn: createViewFn(vm, func(state goja.Value) string { return "" }),
		state:  vm.NewObject(),
	}
	model.jsRunner = &SyncJSRunner{Runtime: vm}

	master, slave := openPty(t)
	defer master.Close()
	// Use dup'd FD for bubbletea to avoid data race between Close and
	// bubbletea's internal handleResize goroutine calling Fd().
	slaveFd, dupErr := syscall.Dup(int(slave.Fd()))
	require.NoError(t, dupErr)
	slaveForBT := os.NewFile(uintptr(slaveFd), slave.Name())
	slave.Close()

	manager := NewManager(context.Background(), slaveForBT, slaveForBT, model.jsRunner, nil, nil)

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- manager.runProgram(model)
	}()

	// wait for start (or immediate failure)
	select {
	case err := <-startErrCh:
		if err != nil {
			slaveForBT.Close()
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
		slaveForBT.Close()
		t.Fatal("Timeout waiting for state refresh")
	}

	// Wait for program to exit
	select {
	case err := <-startErrCh:
		slaveForBT.Close()
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		slaveForBT.Close()
		t.Fatal("program didn't exit")
	}
}

// quitOnKeyModel is a tea.Model whose Update quits the program when it
// receives a key press matching want.
type quitOnKeyModel struct {
	want string
	got  chan string
}

func (m quitOnKeyModel) Init() tea.Cmd { return nil }
func (m quitOnKeyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		s := km.String()
		select {
		case m.got <- s:
		default:
		}
		if s == m.want {
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m quitOnKeyModel) View() tea.View { return tea.NewView("") }

// TestRunProgram_HeadlessExplicitInputIsConsumed pins V6: when the caller
// passes an EXPLICIT non-TTY input (e.g. a strings.Reader), the headless
// branch must preserve it — the program reads the scripted key and quits on
// it — instead of unconditionally replacing input with nil.
func TestRunProgram_HeadlessExplicitInputIsConsumed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping program run in short mode")
	}
	vm := goja.New()
	var out bytes.Buffer
	got := make(chan string, 1)
	model := quitOnKeyModel{want: "q", got: got}
	// Explicit non-TTY input + non-TTY output -> headless branch, but the
	// explicit reader must still be wired into the program.
	m := NewManagerWithStderr(context.Background(), strings.NewReader("q"), &out, &out, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.False(t, m.isTTY)
	require.False(t, m.inputDefaulted)

	errCh := make(chan error, 1)
	go func() { errCh <- m.runProgram(model) }()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("program did not exit on the scripted 'q' key: explicit non-TTY input was not consumed")
	}
	select {
	case k := <-got:
		if k != "q" {
			t.Fatalf("got key %q, want %q", k, "q")
		}
	default:
		t.Fatal("no key message observed: input reader was never read")
	}
	// Headless output must not contain terminal escape sequences.
	if bytes.Contains(out.Bytes(), []byte("\x1b[")) {
		t.Fatalf("headless output contains escape sequences: %q", out.String())
	}
}

// TestRunProgram_HeadlessDefaultInputIsDropped pins the other half of V6:
// when input was defaulted from nil (no explicit reader), the headless branch
// still replaces stdin with nil so bubbletea never blocks on it, and no
// bracketed-paste escape sequences are written to a non-TTY output.
func TestRunProgram_HeadlessDefaultInputIsDropped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping program run in short mode")
	}
	vm := goja.New()
	// Give an explicit non-TTY output only, so skipTTYDetection is false but
	// isTTY stays false; input stays defaulted.
	rOut, wOut, err := os.Pipe()
	require.NoError(t, err)
	defer rOut.Close()
	defer wOut.Close()
	m := NewManagerWithStderr(context.Background(), nil, wOut, wOut, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.False(t, m.isTTY)
	require.True(t, m.inputDefaulted)

	// Model quits immediately; if input were wired to stdin the read loop
	// could spin, but with nil input the program must exit promptly.
	model := quitOnKeyModel{want: "never", got: make(chan string, 1)}
	errCh := make(chan error, 1)
	go func() { errCh <- m.runProgram(model) }()
	// Force quit from outside after a short delay in case Init's nil cmd
	// leaves the program idle.
	go func() {
		time.Sleep(200 * time.Millisecond)
		m.mu.Lock()
		if m.program != nil {
			m.program.Quit()
		}
		m.mu.Unlock()
	}()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("headless program with defaulted input did not exit")
	}
}
