package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type writerFunc func([]byte) (int, error)

func (w writerFunc) Write(p []byte) (int, error) { return w(p) }

// closeSchedulingSlack is how far past a configured Close bound a test tolerates
// the shutdown finishing. It is scheduling allowance, not part of the Close
// contract: CI runners oversubscribe their CPUs, so a correct Close can be
// descheduled well past the configured grace and force-wait totals. 30s
// rather than 5s: a loaded windows-latest runner once descheduled the Close
// goroutine for 163ms past grace+force (run 43785914039 measured 12.163s
// against a 12s bound with the old 5s slack already in place), and the tests
// assert bounded shutdown, not wall-clock precision — the bound being
// asserted is the configured grace+force total, not the slack.
const closeSchedulingSlack = 30 * time.Second

// conPTYOutputDeadline bounds how long a ConPTY test waits for the child to
// produce recognisable output on a loaded runner. PowerShell CLR startup on
// an oversubscribed windows-latest host has exceeded 10s without any defect
// in Spawn/Read (run 43785914039 emitted only the ConPTY enable sequences
// within 10s), so the deadline is a generous ceiling, not a performance
// assertion.
const conPTYOutputDeadline = 60 * time.Second

func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix-specific commands (sh, cat, sleep, pwd, echo)")
	}
}

// conPTYProbeResult caches the ready/unavailable outcome of probing the host
// for a working ConPTY. Ten tests call requireConPTYRuntime and several of
// them t.Parallel with each other, so probing per call meant up to thirty
// spawns per package run, each up to 10s, competing for the same loaded runner.
// A FAILURE is deliberately NOT cached: one unlucky three-attempt window would
// otherwise fail every ConPTY test for the rest of the package run, hiding a
// transient runner problem as a permanent contract failure.
var conPTYProbeResult struct {
	mu     sync.Mutex
	probed bool // true once a ready/unavailable verdict has been cached
	// unavailable is a human-readable reason, or empty when ConPTY works.
	unavailable string
	// probeMu serializes probeConPTY calls: the check-then-probe sequence must
	// be single-flight, or parallel t.Parallel callers all probe at once — the
	// spawn storm the cache exists to prevent. Held across the probe so later
	// callers reuse the cached verdict; a failure is still not cached, so the
	// next caller re-probes sequentially instead of inheriting a bad verdict.
	probeMu sync.Mutex
}

// conPTYStatusDLLInitFailed is STATUS_DLL_INIT_FAILED (0xC0000142), the
// documented headless-runner limitation where the child's console cannot
// initialise. Together with the API-absence errnos below it is the only probe
// outcome that legitimately means "this host cannot run these tests".
const conPTYStatusDLLInitFailed = 3221225794

// probeExitCode converts an unsigned Windows status code to the int field the
// probe records. The conversion must happen at runtime: writing
// `conPTYProbeAttempt{code: conPTYStatusDLLInitFailed}` is a compile-time
// constant conversion that overflows on windows/386, where release binaries are
// built. On 32-bit the wrapped value is negative; classifyConPTYAttempt reads
// it back through uint32, so the verdict is identical.
func probeExitCode(status uint32) int {
	return int(status)
}

// Windows errnos meaning the ConPTY API itself is not present on this host.
// They are declared as plain syscall.Errno values because this file builds on
// every platform, while golang.org/x/sys/windows does not. Pre-1809 Windows
// Server has no pseudoconsole at all, which is host capability, not a
// regression in this package.
const (
	conPTYErrCallNotImplemented = syscall.Errno(120) // ERROR_CALL_NOT_IMPLEMENTED
	conPTYErrProcNotFound       = syscall.Errno(127) // ERROR_PROC_NOT_FOUND
)

// conPTYProbeVerdict is the outcome of one probe attempt.
type conPTYProbeVerdict int

const (
	// conPTYProbeReady: the host spawned, waited on and closed a console.
	conPTYProbeReady conPTYProbeVerdict = iota
	// conPTYProbeUnavailable: the host cannot initialise a console.
	conPTYProbeUnavailable
	// conPTYProbeFailure: the pty contract itself is broken.
	conPTYProbeFailure
)

// conPTYProbeAttempt is one attempt's raw observations.
type conPTYProbeAttempt struct {
	spawnErr   error
	waitErr    error
	closeErr   error
	code       int
	outputTail string
}

// probeTail is a bounded rolling buffer keeping the MOST RECENT max bytes:
// for startup diagnosis the tail of the output is the evidence, and a buffer
// that stops accepting after max bytes would keep the head and drop exactly
// the diagnostic tail. Writes crossing the cap keep the end of the new data.
type probeTail struct {
	buf []byte
	max int
}

func (t *probeTail) write(p []byte) {
	if len(p) >= t.max {
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		return
	}
	if len(t.buf)+len(p) > t.max {
		drop := len(t.buf) + len(p) - t.max
		t.buf = t.buf[drop:]
	}
	t.buf = append(t.buf, p...)
}

func (t *probeTail) String() string { return string(t.buf) }

// truncateForProbeReason keeps a diagnostic tail bounded so a pathological
// child cannot turn the probe reason into an unbounded log line. The cut is
// rune-aware so a multi-byte UTF-8 sequence is never split mid-rune.
func truncateForProbeReason(s string) string {
	const max = 512
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[len(runes)-max:])
}

// classifyConPTYAttempt decides what one probe attempt means.
//
// The probe runs the code under test, so a failure can be either "this host
// cannot run a console" or "Spawn/Wait/Close is broken". Only two things are the
// former: STATUS_DLL_INIT_FAILED, the documented headless-runner console-init
// limitation, and a spawn that reports the ConPTY API itself is absent. Everything
// else — any other Spawn error, a Wait error (including a deadlock that hit the
// attempt bound), an unexpected exit code, or a Close that could not release the
// pseudoconsole — is a broken contract and must fail the suite loudly rather
// than skip it green.
func classifyConPTYAttempt(attempt conPTYProbeAttempt) (conPTYProbeVerdict, string) {
	if attempt.spawnErr != nil {
		if errors.Is(attempt.spawnErr, ErrNotSupported) ||
			errors.Is(attempt.spawnErr, conPTYErrCallNotImplemented) ||
			errors.Is(attempt.spawnErr, conPTYErrProcNotFound) {
			return conPTYProbeUnavailable, fmt.Sprintf("ConPTY is not available on this host: %v", attempt.spawnErr)
		}
		return conPTYProbeFailure, fmt.Sprintf("pty.Spawn failed: %v", attempt.spawnErr)
	}
	if attempt.waitErr != nil {
		return conPTYProbeFailure, fmt.Sprintf("pty.Wait failed: %v", attempt.waitErr)
	}
	if uint32(attempt.code) == conPTYStatusDLLInitFailed {
		return conPTYProbeUnavailable, fmt.Sprintf("child console init failed with %#x (STATUS_DLL_INIT_FAILED): this host cannot run ConPTY", uint32(attempt.code))
	}
	if attempt.closeErr != nil {
		return conPTYProbeFailure, fmt.Sprintf("pty.Close failed: %v", attempt.closeErr)
	}
	if attempt.code != 0 {
		reason := fmt.Sprintf("probe command exited with code %d", attempt.code)
		if tail := strings.TrimSpace(attempt.outputTail); tail != "" {
			reason += "; output tail: " + truncateForProbeReason(tail)
		}
		return conPTYProbeFailure, reason
	}
	return conPTYProbeReady, ""
}

// requireConPTYRuntime skips the calling test unless the host can run ConPTY,
// and fails it when the probe proves the pty contract is broken.
//
// The distinction matters: a host that cannot spawn a working console is a
// reason to skip, because no ConPTY test in this package can then say anything
// about the code (run 129562188488 reported `ConPTY runtime probe exited with
// code 1 after 3 attempts`; that exit 1 came from the old PowerShell CLR
// startup timing out, not from STATUS_DLL_INIT_FAILED 0xC0000142, and is now a
// hard failure). But a Spawn or Wait regression is exactly what these tests
// exist to catch, so it must fail the package instead of silently
// skipping all ten callers.
func requireConPTYRuntime(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY runtime probe requires Windows")
	}

	conPTYProbeResult.mu.Lock()
	probed := conPTYProbeResult.probed
	cachedUnavailable := conPTYProbeResult.unavailable
	conPTYProbeResult.mu.Unlock()
	if probed {
		if cachedUnavailable != "" {
			t.Skipf("skipping ConPTY tests: %s", cachedUnavailable)
		}
		return
	}
	// Single-flight: hold probeMu across the probe and the cache write so
	// parallel callers wait for the one active probe and reuse its verdict
	// instead of stampeding the host with concurrent spawns.
	conPTYProbeResult.probeMu.Lock()
	defer conPTYProbeResult.probeMu.Unlock()
	// Re-check under the probe lock: another caller may have completed the
	// probe while this one waited.
	conPTYProbeResult.mu.Lock()
	probed = conPTYProbeResult.probed
	cachedUnavailable = conPTYProbeResult.unavailable
	conPTYProbeResult.mu.Unlock()
	if probed {
		if cachedUnavailable != "" {
			t.Skipf("skipping ConPTY tests: %s", cachedUnavailable)
		}
		return
	}
	unavailable, failure := probeConPTY()
	conPTYProbeResult.mu.Lock()
	// Only a definitive readiness or host-capability verdict is cached. A
	// failure may be transient (handle exhaustion, runner load); the next
	// caller re-probes instead of inheriting a permanent t.Fatalf.
	if failure == "" {
		conPTYProbeResult.probed = true
		conPTYProbeResult.unavailable = unavailable
	}
	conPTYProbeResult.mu.Unlock()
	if failure != "" {
		t.Fatalf("ConPTY runtime probe failed: %s", failure)
	}
	if unavailable != "" {
		t.Skipf("skipping ConPTY tests: %s", unavailable)
	}
}

// probeConPTY reports why the host cannot run ConPTY tests (unavailable) or why
// the pty contract is broken (failure), or two empty strings when ConPTY works.
//
// The command is cmd.exe rather than powershell.exe: PowerShell loads the CLR,
// which made the probe slow enough to need a wide bound and flaky enough on a
// loaded runner to need retries. Output is drained concurrently so a child that
// writes before exiting cannot fill the ConPTY pipe and wedge Wait. Each attempt
// releases its own process and context (probeConPTYAttempt), so a failed attempt
// leaves no handle behind for the retry.
func probeConPTY() (unavailable, failure string) {
	const attempts = 3

	lastFailure := "probe produced no verdict"
	for attempt := 1; attempt <= attempts; attempt++ {
		verdict, reason := probeConPTYAttempt()
		switch verdict {
		case conPTYProbeReady:
			return "", ""
		case conPTYProbeUnavailable:
			// A console that cannot initialise will not start working on a
			// retry: it is a property of this host, so report it immediately.
			return reason, ""
		case conPTYProbeFailure:
			// A failure may be a loaded-runner artefact (handle exhaustion,
			// a timeout), so retry before calling the contract broken.
			lastFailure = reason
		}
		if attempt < attempts {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return "", lastFailure + fmt.Sprintf(" (after %d attempts)", attempts)
}

// probeConPTYAttempt runs one spawn/wait/close cycle and classifies it.
func probeConPTYAttempt() (conPTYProbeVerdict, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var observed conPTYProbeAttempt

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "cmd.exe",
		Args:    []string{"/c", "exit 0"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		observed.spawnErr = err
		return classifyConPTYAttempt(observed)
	}

	// Drain concurrently into a bounded ROLLING tail: an undrained ConPTY
	// output pipe can block the child before it exits, which would surface as
	// a bogus Wait timeout, and discarding the output made a repeat exit-1
	// undiagnosable. The tail keeps the most recent 4096 bytes — the end of
	// the output is what distinguishes a console-init failure from a broken
	// Spawn/Wait contract.
	var tailMu sync.Mutex
	tail := &probeTail{max: 4096}
	proc.DrainOutput(writerFunc(func(p []byte) (int, error) {
		n := len(p)
		tailMu.Lock()
		tail.write(p)
		tailMu.Unlock()
		return n, nil
	}))

	observed.code, observed.waitErr = proc.Wait()
	observed.closeErr = proc.Close()
	tailMu.Lock()
	observed.outputTail = tail.String()
	tailMu.Unlock()

	return classifyConPTYAttempt(observed)
}

func TestSpawn_EchoHello(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "echo",
		Args:    []string{"hello"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "hello") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello") {
		t.Fatalf("expected output to contain %q, got %q", "hello", output.String())
	}

	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestSpawn_EmptyCommand(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	_, err := Spawn(context.Background(), SpawnConfig{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSpawn_InvalidCommand(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	_, err := Spawn(context.Background(), SpawnConfig{
		Command: "/nonexistent/command/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error for invalid command")
	}
}

func TestSpawn_EnvVars(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildEnvEchoProgram(t, "MY_TEST_VAR"),
		Env: map[string]string{
			"MY_TEST_VAR": "test_value_42",
		},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "test_value_42") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "test_value_42") {
		t.Fatalf("expected output to contain %q, got %q", "test_value_42", output.String())
	}
}

func TestSpawn_EnvReplace(t *testing.T) {
	// No t.Parallel: the parent marker mutates this test's environment.
	skipIfWindows(t)

	if err := os.Setenv("OSM_ENVREPLACE_PARENT_MARKER", "parent_value_must_not_leak"); err != nil {
		t.Fatalf("set parent marker: %v", err)
	}
	defer os.Unsetenv("OSM_ENVREPLACE_PARENT_MARKER")
	prog := buildProgram(t, `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("parent=" + os.Getenv("OSM_ENVREPLACE_PARENT_MARKER"))
	fmt.Println("child=" + os.Getenv("OSM_ENVREPLACE_CHILD_MARKER"))
}
`)
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: prog,
		Env: map[string]string{
			"OSM_ENVREPLACE_CHILD_MARKER": "child_value_kept",
		},
		EnvReplace: true,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "child=child_value_kept") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "child=child_value_kept") {
		t.Fatalf("expected configured var in child env, got %q", output.String())
	}
	if strings.Contains(output.String(), "parent_value_must_not_leak") {
		t.Fatalf("parent env leaked into EnvReplace child, got %q", output.String())
	}
}

func TestSpawn_WorkingDirectory(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	dir := t.TempDir()
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "pwd",
		Dir:     dir,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if output.Len() > 0 && readErr != nil {
			break
		}
		if readErr != nil {
			break
		}
	}

	outStr := strings.TrimSpace(output.String())
	outStr = strings.ReplaceAll(outStr, "\r", "")

	// macOS may resolve /var -> /private/var via symlinks.
	// filepath.EvalSymlinks resolves the full chain, unlike os.Readlink
	// which only reads a single symlink target.
	resolvedDir, resolveErr := filepath.EvalSymlinks(dir)
	if resolveErr != nil {
		resolvedDir = dir
	}
	if !strings.Contains(outStr, dir) && !strings.Contains(outStr, resolvedDir) {
		t.Fatalf("expected output to contain %q or %q, got %q", dir, resolvedDir, outStr)
	}
}

func TestProcess_Resize(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if err := proc.Resize(48, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	if err := proc.Resize(100, 200); err != nil {
		t.Fatalf("second Resize failed: %v", err)
	}
}

func TestProcess_Signal(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if !proc.IsAlive() {
		t.Fatal("expected process to be alive immediately after spawn")
	}

	if err := proc.Signal("SIGINT"); err != nil {
		t.Fatalf("Signal(SIGINT) failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process to exit after SIGINT")
	}

	if proc.IsAlive() {
		t.Fatal("expected process to be dead after SIGINT")
	}
}

func TestProcess_Signal_Unsupported(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	err = proc.Signal("NOSUCHSIG")
	if err == nil {
		t.Fatal("expected error for unsupported signal")
	}
	if !strings.Contains(err.Error(), "unsupported signal") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcess_Close_Idempotent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	if err := proc.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := proc.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestProcess_Write_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	_, err = proc.Write([]byte("hello"))
	if err == nil {
		t.Fatal("expected error when writing to closed process")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got: %v", err)
	}
}

func TestProcess_Read_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	_, readErr := proc.Read()
	if readErr != nil && !errors.Is(readErr, ErrClosed) {
		// ErrClosed is expected; other errors are also acceptable post-close.
		_ = readErr
	}
}

func TestProcess_Pid(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	pid := proc.Pid()
	if pid == 0 {
		t.Fatal("expected non-zero PID")
	}
}

func TestSpawn_DefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := SpawnConfig{Command: "echo", Args: []string{"test"}}
	cfg.applyDefaults()

	if cfg.Rows != DefaultRows {
		t.Fatalf("expected default rows %d, got %d", DefaultRows, cfg.Rows)
	}
	if cfg.Cols != DefaultCols {
		t.Fatalf("expected default cols %d, got %d", DefaultCols, cfg.Cols)
	}
	if cfg.Term != DefaultTerm {
		t.Fatalf("expected default term %q, got %q", DefaultTerm, cfg.Term)
	}
	if cfg.WriteTimeout != DefaultWriteTimeout {
		t.Fatalf("expected default write timeout %v, got %v", DefaultWriteTimeout, cfg.WriteTimeout)
	}
	if cfg.CloseGracePeriod != DefaultCloseGracePeriod {
		t.Fatalf("expected default close grace period %v, got %v", DefaultCloseGracePeriod, cfg.CloseGracePeriod)
	}
	if cfg.CloseForceWait != DefaultCloseForceWait {
		t.Fatalf("expected default close force wait %v, got %v", DefaultCloseForceWait, cfg.CloseForceWait)
	}
}

func TestProcess_ContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	skipIfWindows(t)

	ctx, cancel := context.WithCancel(context.Background())

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	cancel()

	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for process to exit after context cancel")
	}
}

func TestProcess_WriteAndReadCat(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if _, err := proc.Write([]byte("hello from pty\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "hello from pty") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello from pty") {
		t.Fatalf("expected output to contain %q, got %q", "hello from pty", output.String())
	}
}

func TestParseSignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{"SIGINT", false},
		{"SIGTERM", false},
		{"SIGKILL", false},
		{"SIGHUP", false},
		{"SIGQUIT", false},
		{"NOSUCHSIG", true},
		{"", true},
	}

	for _, tt := range tests {
		sig, err := parseSignal(tt.name)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseSignal(%q): expected error", tt.name)
			}
		} else {
			if err != nil {
				t.Errorf("parseSignal(%q): unexpected error: %v", tt.name, err)
			}
			if sig == nil {
				t.Errorf("parseSignal(%q): expected signal, got nil", tt.name)
			}
		}
	}
}

func TestProcess_Resize_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	err = proc.Resize(48, 120)
	if err == nil {
		t.Fatal("expected error when resizing closed process")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got: %v", err)
	}
}

func TestProcess_Signal_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	proc.Close()

	err = proc.Signal("SIGINT")
	if err == nil {
		t.Fatal("expected error when signaling closed process")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got: %v", err)
	}
}

func TestSpawn_CommandWithSpaces(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// When Command contains spaces and Args is empty, Spawn should
	// split the command string automatically.
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "echo hello_from_split",
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "hello_from_split") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello_from_split") {
		t.Fatalf("expected output to contain %q, got %q", "hello_from_split", output.String())
	}
}

func TestSpawn_CommandWithSpacesAndExplicitArgs(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// When Args is provided, Command should NOT be split — even if it
	// contains spaces. This preserves backward compatibility.
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildEchoProgram(t, "explicit_args"),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "explicit_args") {
			break
		}
		if readErr != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "explicit_args") {
		t.Fatalf("expected output to contain %q, got %q", "explicit_args", output.String())
	}
}

// TestProcess_Pid_NilCmd exercises the nil-cmd guard in Pid().
func TestProcess_Pid_NilCmd(t *testing.T) {
	t.Parallel()
	p := &Process{done: make(chan struct{})}
	// cmd is nil → Pid() should return 0.
	pid := p.Pid()
	if pid != 0 {
		t.Fatalf("expected 0 for nil cmd, got %d", pid)
	}
}

// TestProcess_WriteSignalDeadlock verifies that Signal can be called while
// Write is blocked on a full PTY buffer. Before the fix, Write held p.mu
// for the entire duration of the blocking kernel write, causing Signal
// (which also needs p.mu) to deadlock.
//
// Regression test for: auto-split hang when the consumer doesn't read stdin
// fast enough — cancel (SIGKILL) could never be delivered.
func TestProcess_WriteSignalDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	skipIfWindows(t)

	// Spawn `sleep 3600` — it never reads stdin, so a large write will
	// eventually block when the kernel PTY buffer fills.
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"3600"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	// Start a large write in a goroutine. On most systems the PTY
	// buffer is 4–64 KiB; 1 MiB should reliably fill it.
	writeDone := make(chan error, 1)
	go func() {
		_, err := proc.Write([]byte(strings.Repeat("x", 1<<20)))
		writeDone <- err
	}()

	// Give the write time to start blocking.
	time.Sleep(200 * time.Millisecond)

	// Try to send SIGKILL from another goroutine. If there's a mutex
	// deadlock, this will block forever.
	sigDone := make(chan error, 1)
	go func() {
		sigDone <- proc.Signal("SIGKILL")
	}()

	select {
	case err := <-sigDone:
		if err != nil {
			t.Logf("Signal returned error (acceptable): %v", err)
		}
		t.Log("Signal completed without deadlock")
	case <-time.After(5 * time.Second):
		t.Fatal("DEADLOCK: Signal blocked while Write is in progress — " +
			"Write must release the mutex before blocking I/O")
	}

	// The write goroutine should also complete (with error) now that
	// the child is dead.
	select {
	case err := <-writeDone:
		t.Logf("Write completed after SIGKILL, err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Write goroutine did not unblock after SIGKILL")
	}
}

// TestProcess_CloseWhileWriteBlocked verifies that Close can proceed
// while Write is blocked on a full PTY buffer.
func TestProcess_CloseWhileWriteBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sleep",
		Args:    []string{"3600"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	// Start a large write that will block.
	writeDone := make(chan error, 1)
	go func() {
		_, err := proc.Write([]byte(strings.Repeat("x", 1<<20)))
		writeDone <- err
	}()

	time.Sleep(200 * time.Millisecond)

	// Close should not deadlock even if Write is blocking.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- proc.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Logf("Close returned error (acceptable): %v", err)
		}
		t.Log("Close completed without deadlock")
	case <-time.After(10 * time.Second):
		t.Fatal("DEADLOCK: Close blocked while Write is in progress")
	}

	// Write should also complete (with error).
	select {
	case err := <-writeDone:
		t.Logf("Write completed after Close, err=%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Write goroutine did not unblock after Close")
	}
}

type stuckHandle struct {
	mu      sync.Mutex
	signals []os.Signal
}

func (h *stuckHandle) Wait() error { select {} }

func (h *stuckHandle) Signal(sig os.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signals = append(h.signals, sig)
	return nil
}

func (h *stuckHandle) Pid() int { return 12345 }

func (h *stuckHandle) SignalCount(sig os.Signal) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	count := 0
	for _, s := range h.signals {
		if s == sig {
			count++
		}
	}
	return count
}

// TestProcess_Close_ForceKillWaitTimeout verifies Close does not block forever
// when the process never reports exit after SIGKILL.
func TestProcess_Close_ForceKillWaitTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	handle := &stuckHandle{}
	proc := &Process{
		ptyFile:           r,
		ttyFile:           w,
		done:              make(chan struct{}), // never closed => process appears alive forever
		exitCode:          -1,
		cmd:               handle,
		closeGracefulWait: DefaultCloseGracePeriod,
		closeForceWait:    DefaultCloseForceWait,
	}

	start := time.Now()
	closeErr := proc.Close()
	elapsed := time.Since(start)

	// Leave wider slack for busy CI runners; this test validates bounded
	// shutdown semantics, not tight wall-clock precision.
	maxExpected := DefaultCloseGracePeriod + DefaultCloseForceWait + closeSchedulingSlack
	if elapsed > maxExpected {
		t.Fatalf("Close took too long: %v (max expected ~%v)", elapsed, maxExpected)
	}
	if closeErr == nil {
		t.Fatal("expected timeout error when process never exits after SIGKILL")
	}
	var timeoutErr *ErrForceKillTimeout
	if !errors.As(closeErr, &timeoutErr) {
		t.Fatalf("expected ErrForceKillTimeout, got: %v", closeErr)
	}
	if timeoutErr.Wait != DefaultCloseForceWait {
		t.Fatalf("expected Wait=%v, got %v", DefaultCloseForceWait, timeoutErr.Wait)
	}
	if handle.SignalCount(syscall.SIGTERM) != 1 {
		t.Fatalf("expected exactly one SIGTERM, got %d", handle.SignalCount(syscall.SIGTERM))
	}
	if handle.SignalCount(syscall.SIGKILL) != 1 {
		t.Fatalf("expected exactly one SIGKILL, got %d", handle.SignalCount(syscall.SIGKILL))
	}
}

// TestProcess_Close_CustomGracePeriod verifies that Close uses the configured
// grace period and force wait durations from SpawnConfig.
func TestProcess_Close_CustomGracePeriod(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	handle := &stuckHandle{}
	gracePeriod := 500 * time.Millisecond
	forceWait := 300 * time.Millisecond
	proc := &Process{
		ptyFile:           r,
		ttyFile:           w,
		done:              make(chan struct{}),
		exitCode:          -1,
		cmd:               handle,
		closeGracefulWait: gracePeriod,
		closeForceWait:    forceWait,
	}

	start := time.Now()
	closeErr := proc.Close()
	elapsed := time.Since(start)

	var timeoutErr *ErrForceKillTimeout
	if !errors.As(closeErr, &timeoutErr) {
		t.Fatalf("expected ErrForceKillTimeout, got: %v", closeErr)
	}
	if timeoutErr.Wait != forceWait {
		t.Fatalf("expected Wait=%v, got %v", forceWait, timeoutErr.Wait)
	}

	// Elapsed time should be approximately gracePeriod + forceWait: those are
	// the configured bounds Close is expected to honour. The upper allowance is
	// scheduling slack, not a second contract — a hard 2s ceiling failed a
	// loaded windows-latest runner at 3.4769077s (run 129562188488) while Close
	// was behaving correctly.
	minExpected := gracePeriod + forceWait
	maxExpected := minExpected + closeSchedulingSlack
	if elapsed < minExpected {
		t.Fatalf("Close returned too quickly: %v (min expected ~%v)", elapsed, minExpected)
	}
	if elapsed > maxExpected {
		t.Fatalf("Close took too long: %v (max expected ~%v)", elapsed, maxExpected)
	}
}

// TestProcess_Close_CustomGracePeriod_QuickExit verifies that Close returns
// promptly when the process exits before the grace period elapses.
func TestProcess_Close_CustomGracePeriod_QuickExit(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command:          "echo",
		Args:             []string{"fast-exit"},
		CloseGracePeriod: 10 * time.Second,
		CloseForceWait:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}

	start := time.Now()
	closeErr := proc.Close()
	elapsed := time.Since(start)

	if closeErr != nil {
		t.Fatalf("Close returned error: %v", closeErr)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Close took too long for fast-exiting process: %v", elapsed)
	}
}

// TestErrForceKillTimeout_Error verifies the error message and errors.As behavior.
func TestErrForceKillTimeout_Error(t *testing.T) {
	t.Parallel()

	err := &ErrForceKillTimeout{Wait: 2 * time.Second}
	if !strings.Contains(err.Error(), "2s") {
		t.Fatalf("expected error to contain duration, got: %v", err.Error())
	}

	var target *ErrForceKillTimeout
	if !errors.As(err, &target) {
		t.Fatal("errors.As should match ErrForceKillTimeout")
	}
	if target.Wait != 2*time.Second {
		t.Fatalf("expected Wait=2s, got %v", target.Wait)
	}
}

func TestProcess_Write_WithTimeout(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Normal write succeeds with timeout enabled (the default).
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t),
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if proc.writeTimeout != DefaultWriteTimeout {
		t.Fatalf("expected writeTimeout=%v, got %v", DefaultWriteTimeout, proc.writeTimeout)
	}

	// Write a small payload — should succeed instantly.
	if _, err := proc.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write with timeout failed: %v", err)
	}
}

func TestProcess_Write_TimeoutDisabled(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command:      buildIdleProgram(t),
		WriteTimeout: -1, // disable deadline
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if proc.writeTimeout != -1 {
		t.Fatalf("expected writeTimeout=-1 (disabled), got %v", proc.writeTimeout)
	}

	if _, err := proc.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write without timeout failed: %v", err)
	}
}

func TestProcess_Write_CustomTimeout(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command:      buildIdleProgram(t),
		WriteTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	if proc.writeTimeout != 5*time.Second {
		t.Fatalf("expected writeTimeout=5s, got %v", proc.writeTimeout)
	}

	if _, err := proc.Write([]byte("custom timeout\n")); err != nil {
		t.Fatalf("Write with custom timeout failed: %v", err)
	}
}

// TestConPTY_Smoke verifies basic ConPTY functionality on Windows.
// This test only runs on Windows and is skipped on other platforms.
// It validates the core spawn→read→wait lifecycle that cross-compilation
// alone cannot verify.
func TestConPTY_Smoke(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY smoke test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY smoke test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), conPTYOutputDeadline)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-Command", "Write-Output 'conpty-smoke-test'; Start-Sleep -Milliseconds 250; exit 0"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	// Read output — should contain the echo string.
	var output strings.Builder
	deadline := time.After(conPTYOutputDeadline)
	done := false
	for !done {
		readCh := make(chan struct {
			data []byte
			err  error
		}, 1)
		go func() {
			data, readErr := proc.Read()
			readCh <- struct {
				data []byte
				err  error
			}{data: data, err: readErr}
		}()

		select {
		case <-deadline:
			_ = proc.Close()
			t.Fatalf("timed out waiting for conpty smoke output, got so far: %q", output.String())
		case readResult := <-readCh:
			if len(readResult.data) > 0 {
				output.Write(readResult.data)
			}
			if strings.Contains(output.String(), "conpty-smoke-test") {
				done = true
			}
			if readResult.err != nil {
				done = true
			}
		}
	}
	if !strings.Contains(output.String(), "conpty-smoke-test") {
		t.Fatalf("expected output to contain 'conpty-smoke-test', got %q", output.String())
	}

	code, waitErr := proc.Wait()
	if waitErr != nil {
		t.Fatalf("Wait failed: %v", waitErr)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

// TestConPTY_ContextCancel verifies that context cancellation kills the
// child process on Windows. This validates KILL-01 from the autopsy:
// the context watcher goroutine in spawnWithConPTY must actually work.
func TestConPTY_ContextCancel(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY context cancel test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY context cancel test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "cmd.exe",
		Args:    []string{"/c", "timeout", "/t", "60"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	// Cancel context - should kill the child via the context watcher goroutine.
	cancel()

	// Wait should complete promptly (child was killed).
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Child exited as expected.
	case <-time.After(10 * time.Second):
		t.Fatal("context cancellation did not kill the Windows child process within 10s")
	}
}

// TestConPTY_Resize verifies ResizePseudoConsole works on Windows.
func TestConPTY_Resize(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY resize test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY resize test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "cmd.exe",
		Args:    []string{"/c", "echo", "resize-test"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	// Resize should succeed without error.
	if err := proc.Resize(40, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
}

// TestConPTY_WriteRead exercises the ConPTY stdin→PTY→output path by
// spawning a cmd.exe echo loop, writing text, and verifying the echo
// arrives on stdout. This is the foundational flow that passthrough
// and capture session building on, but at the raw Process level.
func TestConPTY_WriteRead(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("ConPTY write/read test requires Windows")
	}
	if testing.Short() {
		t.Skip("skipping ConPTY write/read test in short mode")
	}
	requireConPTYRuntime(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "cmd.exe",
		Args:    []string{"/c", "echo", "write-read-conpty-test"},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Spawn via ConPTY failed: %v", err)
	}
	defer proc.Close()

	var output strings.Builder
	deadline := time.After(10 * time.Second)
	done := false
	for !done {
		select {
		case <-deadline:
			_ = proc.Close()
			t.Fatalf("timed out waiting for ConPTY echo output, got so far: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if len(data) > 0 {
			output.Write(data)
		}
		if strings.Contains(output.String(), "write-read-conpty-test") {
			done = true
		}
		if readErr != nil {
			done = true
		}
	}
	if !strings.Contains(output.String(), "write-read-conpty-test") {
		t.Fatalf("expected output to contain 'write-read-conpty-test', got %q", output.String())
	}
}

func TestProcess_File(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "echo",
		Args:    []string{"file-test"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()

	f := proc.File()
	if f == nil {
		t.Fatal("File() returned nil for running process")
	}

	// File should return the same fd on repeated calls.
	f2 := proc.File()
	if f2 != f {
		t.Error("File() returned different fd on second call")
	}
}

func TestProcess_File_AfterClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "echo",
		Args:    []string{"close-test"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	proc.Close()

	f := proc.File()
	if f != nil {
		t.Error("File() should return nil after Close")
	}
}

func TestProcess_DrainOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: buildEchoIdleProgram(t, "drain-test"),
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()

	// Start draining with a channel-based collector to avoid
	// data races with bytes.Buffer (DrainOutput writes from
	// a background goroutine).
	drainCh := make(chan string, 64)
	proc.DrainOutput(writerFunc(func(p []byte) (int, error) {
		select {
		case drainCh <- string(p):
		default:
		}
		return len(p), nil
	}))

	// Wait for the output to appear in the drain channel.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case chunk := <-drainCh:
			if strings.Contains(chunk, "drain-test") {
				goto found
			}
		case <-deadline:
			t.Fatal("timed out waiting for drain output")
		}
	}
found:
}

func TestProcess_DrainOutput_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proc, err := Spawn(ctx, SpawnConfig{
		Command: "echo",
		Args:    []string{"idempotent"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer proc.Close()

	// Calling DrainOutput twice should be a no-op on the second call.
	proc.DrainOutput(nil)
	proc.DrainOutput(nil) // no panic or error
}
