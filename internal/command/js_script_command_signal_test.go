package command

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests deliver real POSIX signals to the process running the JS
// engine. Signal delivery and Node-style fallback statuses are unavailable
// on Windows, so those tests are skipped there at runtime.
func requireSignalTestPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("delivers POSIX signals to the current process")
	}
}

func sendSignalToSelf(t *testing.T, sig os.Signal) {
	t.Helper()
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Errorf("find current process: %v", err)
		return
	}
	if err := process.Signal(sig); err != nil {
		t.Errorf("send %s to current process: %v", sig, err)
	}
}

type signalTestCommand interface {
	Execute([]string, io.Writer, io.Writer) error
}

type signalTestBuffer struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	changed chan struct{}
}

func newSignalTestBuffer() *signalTestBuffer {
	return &signalTestBuffer{changed: make(chan struct{}, 1)}
}

func (b *signalTestBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buffer.Write(p)
	b.mu.Unlock()
	select {
	case b.changed <- struct{}{}:
	default:
	}
	return n, err
}

func (b *signalTestBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *signalTestBuffer) waitFor(t *testing.T, done <-chan error, text string) {
	t.Helper()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for {
		if strings.Contains(b.String(), text) {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("command exited before writing %q: %v", text, err)
		case <-b.changed:
		case <-timeout.C:
			t.Fatalf("timed out waiting for %q; output=%q", text, b.String())
		}
	}
}

func startSignalCommand(t *testing.T, command signalTestCommand, args []string) (*signalTestBuffer, *signalTestBuffer, <-chan error) {
	t.Helper()
	stdout := newSignalTestBuffer()
	stderr := newSignalTestBuffer()
	done := make(chan error, 1)
	go func() {
		done <- command.Execute(args, stdout, stderr)
	}()
	stdout.waitFor(t, done, "signal-ready")
	return stdout, stderr, done
}

func waitSignalCommand(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(12 * time.Second):
		t.Fatal("command did not exit after signal delivery")
		return nil
	}
}

func TestJSScriptCommand_Execute_SigintWithListenerScriptDecides(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}
	requireSignalTestPlatform(t)

	cmd := newExitChannelScriptCommand(t, `
		var handle = setInterval(function () {}, 100);
		process.on("SIGINT", function () {
			process.exitCode = 42;
			clearInterval(handle);
		});
		output.print("signal-ready");
	`)

	// The listener ran (it decided the exit), proving first-signal delivery to
	// a script listener — Node's semantic is that the script's exitCode wins.
	stdout, stderr, done := startSignalCommand(t, cmd, nil)
	sendSignalToSelf(t, syscall.SIGINT)
	err := waitSignalCommand(t, done)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 42 {
		t.Fatalf("Execute error = %v, want exit status 42 (the script's listener decided)\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
}

func TestJSScriptCommand_Execute_SigintWithoutListenerExits130(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}
	requireSignalTestPlatform(t)

	cmd := newExitChannelScriptCommand(t, `
		// No SIGINT listener. Keep the loop alive until the signal arrives.
		var deadline = Date.now() + 10000;
		setInterval(function () {
			if (Date.now() > deadline) { process.exit(0); }
		}, 100);
		output.print("signal-ready");
	`)

	stdout, stderr, done := startSignalCommand(t, cmd, nil)
	sendSignalToSelf(t, syscall.SIGINT)
	err := waitSignalCommand(t, done)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 130 {
		t.Fatalf("Execute error = %v, want exit status 130 for unlistened SIGINT\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
}

// TestJSScriptCommand_Execute_SigintListenerExitWins covers R5-5.1: a SIGINT
// listener that calls process.exit(5) must make the run exit 5 — the exit
// signal counts as a delivered signal, and the 128+N fallback (130) must not
// override the script's own exit code.
func TestJSScriptCommand_Execute_SigintListenerExitWins(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}
	requireSignalTestPlatform(t)

	cmd := newExitChannelScriptCommand(t, `
		var handle = setInterval(function () {}, 100);
		process.on("SIGINT", function () {
			process.exit(5);
		});
		output.print("signal-ready");
	`)

	stdout, stderr, done := startSignalCommand(t, cmd, nil)
	sendSignalToSelf(t, syscall.SIGINT)
	err := waitSignalCommand(t, done)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 5 {
		t.Fatalf("Execute error = %v, want exit status 5 (listener's process.exit wins over the 130 fallback)\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
}

// TestJSScriptCommand_Execute_SigtermWithoutListenerRunningProgram covers a
// signal that arrives while a bubbletea program is live (tea.run blocks in
// WaitForProgram). With no listener the engine force-cancels, the program is
// quit through the binding's ctx.Done arm, and the run must still report
// Node's default status (143) promptly rather than a generic failure.
//
// The readiness print comes from the model's Init, not the script body:
// bubbletea initializes its input reader (an epoll interest-list add on
// Linux) BEFORE calling Init, so a signal sent after this print cannot
// interrupt program startup. Printing from the body raced that window — a
// SIGTERM landing inside EpollCtl fails the reader with EINTR and the run
// died with a generic error before the 143 attribution could be ordered.
func TestJSScriptCommand_Execute_SigtermWithoutListenerRunningProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}
	requireSignalTestPlatform(t)

	cmd := newExitChannelScriptCommand(t, `
		var tea = require("osm:bubbletea");
		function init() {
			output.print("signal-ready");
			return [{ n: 0 }, tea.tick(60000, "tick")];
		}
		function update(msg, m) { return [m, tea.tick(60000, "tick")]; }
		function view(m) { return { content: "probe " + m.n }; }
		setTimeout(function () { process.exit(0); }, 8000);
		tea.run(tea.newModel({ init: init, update: update, view: view }));
	`)

	stdout, stderr, done := startSignalCommand(t, cmd, nil)
	start := time.Now()
	sendSignalToSelf(t, syscall.SIGTERM)
	err := waitSignalCommand(t, done)
	elapsed := time.Since(start)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 143 {
		t.Fatalf("Execute error = %v, want exit status 143 for unlistened SIGTERM with a running program\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	if elapsed > 5*time.Second {
		t.Fatalf("SIGTERM with a running program took %v; the program was not quit promptly", elapsed)
	}
}

func TestJSScriptCommand_Execute_DoubleSigintForcesTermination(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}
	requireSignalTestPlatform(t)

	cmd := newExitChannelScriptCommand(t, `
		// A listener that ignores the first signal and keeps running.
		process.on("SIGINT", function () {
			globalThis.interrupts = (globalThis.interrupts || 0) + 1;
			if (globalThis.interrupts === 1) { output.print("first-signal"); }
		});
		var deadline = Date.now() + 10000;
		setInterval(function () {
			if (Date.now() > deadline) { process.exit(0); }
		}, 100);
		output.print("signal-ready");
	`)

	stdout, stderr, done := startSignalCommand(t, cmd, nil)
	start := time.Now()
	sendSignalToSelf(t, syscall.SIGINT)
	stdout.waitFor(t, done, "first-signal")
	sendSignalToSelf(t, syscall.SIGINT)
	err := waitSignalCommand(t, done)
	elapsed := time.Since(start)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 130 {
		t.Fatalf("Execute error = %v, want exit status 130 for forced double SIGINT\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	// The second signal must force termination well before the script's own
	// 10-second deadline.
	if elapsed > 5*time.Second {
		t.Fatalf("double SIGINT took %v to terminate; force path did not fire", elapsed)
	}
}
