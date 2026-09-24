//go:build unix

package command

import (
	"bytes"
	"errors"
	"syscall"
	"testing"
	"time"
)

// These tests deliver real POSIX signals to the process running the JS
// engine. The signal delivery itself (syscall.Kill) and the Node-style
// fallback exit statuses (128+N) are Unix semantics, so the tests are
// unix-gated: the Windows syscall package has neither Kill nor the signal
// constants used here.

func TestJSScriptCommand_Execute_SigintWithListenerScriptDecides(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}

	cmd := newExitChannelScriptCommand(t, `
		var handle = setInterval(function () {}, 100);
		process.on("SIGINT", function () {
			process.exitCode = 42;
			clearInterval(handle);
		});
	`)

	// Deliver a real SIGINT while Execute is blocked in Wait.
	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
			t.Logf("Kill: %v", err)
		}
	}()

	// The listener ran (it decided the exit), proving first-signal delivery to
	// a script listener — Node's semantic is that the script's exitCode wins.
	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 42 {
		t.Fatalf("Execute error = %v, want exit status 42 (the script's listener decided)\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
}

func TestJSScriptCommand_Execute_SigintWithoutListenerExits130(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}

	cmd := newExitChannelScriptCommand(t, `
		// No SIGINT listener. Keep the loop alive until the signal arrives.
		var deadline = Date.now() + 10000;
		setInterval(function () {
			if (Date.now() > deadline) { process.exit(0); }
		}, 100);
	`)

	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
			t.Logf("Kill: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
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

	cmd := newExitChannelScriptCommand(t, `
		var handle = setInterval(function () {}, 100);
		process.on("SIGINT", function () {
			process.exit(5);
		});
	`)

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
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
func TestJSScriptCommand_Execute_SigtermWithoutListenerRunningProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}

	cmd := newExitChannelScriptCommand(t, `
		var tea = require("osm:bubbletea");
		function init() { return [{ n: 0 }, tea.tick(60000, "tick")]; }
		function update(msg, m) { return [m, tea.tick(60000, "tick")]; }
		function view(m) { return { content: "probe " + m.n }; }
		tea.run(tea.newModel({ init: init, update: update, view: view }));
	`)

	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
			t.Logf("Kill: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	start := time.Now()
	err := cmd.Execute(nil, &stdout, &stderr)
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

	cmd := newExitChannelScriptCommand(t, `
		// A listener that ignores the first signal and keeps running.
		process.on("SIGINT", function () { globalThis.interrupts = (globalThis.interrupts || 0) + 1; });
		var deadline = Date.now() + 10000;
		setInterval(function () {
			if (Date.now() > deadline) { process.exit(0); }
		}, 100);
	`)

	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
		time.Sleep(200 * time.Millisecond)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
	}()

	var stdout, stderr bytes.Buffer
	start := time.Now()
	err := cmd.Execute(nil, &stdout, &stderr)
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
