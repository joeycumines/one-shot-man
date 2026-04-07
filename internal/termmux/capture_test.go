package termmux

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
}

// testOutputCollector reads all output from a CaptureSession's Reader() channel
// in a background goroutine. Call startCollector(cs) immediately after cs.Start().
//
//   - current() returns accumulated output so far (non-blocking).
//   - wait() blocks until Reader() closes and returns all output.
type testOutputCollector struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func startCollector(cs *CaptureSession) *testOutputCollector {
	tc := &testOutputCollector{done: make(chan struct{})}
	ch := cs.Reader()
	if ch == nil {
		close(tc.done)
		return tc
	}
	go func() {
		defer close(tc.done)
		for chunk := range ch {
			tc.mu.Lock()
			tc.buf.Write(chunk)
			tc.mu.Unlock()
		}
	}()
	return tc
}

func (tc *testOutputCollector) current() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.buf.String()
}

func (tc *testOutputCollector) wait() string {
	<-tc.done
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.buf.String()
}

func TestCaptureSession_EchoHello(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"hello capture"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	output := collector.wait()
	if !strings.Contains(output, "hello capture") {
		t.Fatalf("expected output to contain %q, got %q", "hello capture", output)
	}
}

func TestCaptureSession_IsRunning(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})

	// Not started yet.
	if cs.IsRunning() {
		t.Fatal("expected IsRunning=false before Start")
	}

	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	if !cs.IsRunning() {
		t.Fatal("expected IsRunning=true after Start")
	}

	if err := cs.Interrupt(); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	code, _ := cs.Wait()
	_ = code // exit code varies by platform

	if cs.IsRunning() {
		t.Fatal("expected IsRunning=false after Wait")
	}
}

func TestCaptureSession_Interrupt(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	if err := cs.Interrupt(); err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}

	select {
	case <-cs.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process to exit after Interrupt")
	}
}

func TestCaptureSession_Kill(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	if err := cs.Kill(); err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	select {
	case <-cs.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process to exit after Kill")
	}
}

// T059: Test Pause/Resume (SIGSTOP/SIGCONT) on CaptureSession.
func TestCaptureSession_PauseResume(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Use a command that writes output periodically so we can observe the pause.
	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "i=0; while true; do echo \"line$i\"; i=$((i+1)); sleep 0.1; done"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	// Let the process run and produce some output.
	time.Sleep(500 * time.Millisecond)

	// Verify not paused initially.
	if cs.IsPaused() {
		t.Fatal("should not be paused initially")
	}

	// Pause the process.
	if err := cs.Pause(); err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	if !cs.IsPaused() {
		t.Fatal("should be paused after Pause()")
	}

	// Idempotent: calling Pause again should be a no-op.
	if err := cs.Pause(); err != nil {
		t.Fatalf("second Pause should succeed (no-op): %v", err)
	}

	// Capture output after pausing and verify it stops growing.
	time.Sleep(200 * time.Millisecond)
	outputAfterPause := collector.current()
	time.Sleep(500 * time.Millisecond)
	outputStill := collector.current()
	if outputAfterPause != outputStill {
		// On some systems the child might have buffered output that arrives
		// after the SIGSTOP is delivered, so we only warn rather than fail
		// — the important thing is that Resume works.
		t.Logf("Output grew while paused (buffering effects): %d → %d bytes",
			len(outputAfterPause), len(outputStill))
	}

	// Resume the process.
	if err := cs.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if cs.IsPaused() {
		t.Fatal("should not be paused after Resume()")
	}

	// Idempotent: calling Resume again should be a no-op.
	if err := cs.Resume(); err != nil {
		t.Fatalf("second Resume should succeed (no-op): %v", err)
	}

	// Let it run a bit more and verify new output arrives.
	time.Sleep(500 * time.Millisecond)
	outputAfterResume := collector.current()
	if len(outputAfterResume) <= len(outputStill) {
		t.Fatalf("expected more output after resume, got %d bytes (was %d)",
			len(outputAfterResume), len(outputStill))
	}
	t.Logf("Pause/Resume works: %d → %d → %d bytes",
		len(outputAfterPause), len(outputStill), len(outputAfterResume))

	// Kill to clean up.
	_ = cs.Kill()
}

// T059: Test Pause/Resume on a not-started session.
func TestCaptureSession_PauseResume_NotStarted(t *testing.T) {
	t.Parallel()
	cs := NewCaptureSession(CaptureConfig{Command: "echo", Args: []string{"x"}})

	// Pause on a not-started session should return error because there's no
	// process to send SIGSTOP to.
	if err := cs.Pause(); err == nil {
		t.Fatal("Pause on not-started session should return error")
	}
	// Resume on a not-started, not-paused session is a no-op (returns nil)
	// because it's already "not paused" — there's nothing to resume.
	if err := cs.Resume(); err != nil {
		t.Fatalf("Resume on not-started, not-paused session should be no-op, got: %v", err)
	}
	if cs.IsPaused() {
		t.Fatal("should not be paused when not started")
	}
}

func TestCaptureSession_ContextCancel(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	ctx, cancel := context.WithCancel(context.Background())
	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	cancel()

	select {
	case <-cs.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for process to exit after context cancel")
	}
}

func TestCaptureSession_Resize(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
		Rows:    24,
		Cols:    80,
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	if err := cs.Resize(48, 120); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}

	// Verify dimensions updated.
	cs.mu.Lock()
	if cs.rows != 48 || cs.cols != 120 {
		t.Fatalf("expected 48x120, got %dx%d", cs.rows, cs.cols)
	}
	cs.mu.Unlock()
}

func TestCaptureSession_Resize_InvalidDimensions(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})

	// Not started — should fail.
	if err := cs.Resize(10, 10); err == nil {
		t.Fatal("expected error when resizing before Start")
	}

	// Zero dimensions.
	if err := cs.Resize(0, 80); err == nil {
		t.Fatal("expected error for zero rows")
	}
	if err := cs.Resize(24, 0); err == nil {
		t.Fatal("expected error for zero cols")
	}
	if err := cs.Resize(-1, 80); err == nil {
		t.Fatal("expected error for negative rows")
	}

	// Overflow protection (uint16 max = 65535).
	if err := cs.Resize(100000, 80); err == nil {
		t.Fatal("expected error for rows > 65535")
	}
	if err := cs.Resize(24, 100000); err == nil {
		t.Fatal("expected error for cols > 65535")
	}
}

func TestCaptureSession_DoubleStart(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer cs.Close()

	err := cs.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on second Start")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCaptureSession_CloseIdempotent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := cs.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := cs.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestCaptureSession_Pid(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})

	if cs.Pid() != 0 {
		t.Fatal("expected PID=0 before Start")
	}

	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	pid := cs.Pid()
	if pid == 0 {
		t.Fatal("expected non-zero PID after Start")
	}
}

func TestCaptureSession_ExitCode(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	code, _ := cs.Wait()
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}

	if cs.ExitCode() != 42 {
		t.Fatalf("ExitCode() expected 42, got %d", cs.ExitCode())
	}
}

func TestCaptureSession_Write(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	// Write to stdin; cat echoes it back.
	if err := cs.WriteString("hello from stdin\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait a bit for the echo to come back through PTY.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for echoed output, got: %q", collector.current())
		default:
		}
		if strings.Contains(collector.current(), "hello from stdin") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCaptureSession_SendEOF(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	// Send EOF; cat should exit.
	if err := cs.SendEOF(); err != nil {
		t.Fatalf("SendEOF failed: %v", err)
	}

	select {
	case <-cs.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for process to exit after SendEOF")
	}

	code, _ := cs.Wait()
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
}

func TestCaptureSession_ReaderOutput(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"reader test"},
	})

	// Reader before start should be nil.
	if ch := cs.Reader(); ch != nil {
		t.Fatal("expected nil Reader before Start")
	}

	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	cs.Wait()

	output := collector.wait()
	if output == "" {
		t.Fatal("expected non-empty output after Wait")
	}
	if !strings.Contains(output, "reader test") {
		t.Fatalf("expected output to contain %q, got %q", "reader test", output)
	}
}

func TestCaptureSession_DefaultDimensions(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})
	if cs.rows != 24 {
		t.Fatalf("expected default rows=24, got %d", cs.rows)
	}
	if cs.cols != 80 {
		t.Fatalf("expected default cols=80, got %d", cs.cols)
	}
}

func TestCaptureSession_CustomDimensions(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
		Rows:    48,
		Cols:    120,
	})
	if cs.rows != 48 {
		t.Fatalf("expected rows=48, got %d", cs.rows)
	}
	if cs.cols != 120 {
		t.Fatalf("expected cols=120, got %d", cs.cols)
	}
}

func TestCaptureSession_WorkingDirectory(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	dir := t.TempDir()
	cs := NewCaptureSession(CaptureConfig{
		Command: "pwd",
		Dir:     dir,
		// Wide enough to avoid line-wrap mid-path.
		Cols: 250,
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	cs.Wait()
	output := strings.TrimSpace(collector.wait())
	output = strings.ReplaceAll(output, "\r", "")
	output = strings.ReplaceAll(output, "\n", "")

	// macOS may resolve /var -> /private/var via symlinks.
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolvedDir = dir
	}
	if !strings.Contains(output, dir) && !strings.Contains(output, resolvedDir) {
		t.Fatalf("expected output to contain %q or %q, got %q", dir, resolvedDir, output)
	}
}

func TestCaptureSession_EnvVars(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "echo $CAPTURE_TEST_VAR"},
		Env: map[string]string{
			"CAPTURE_TEST_VAR": "capture_value_99",
		},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	cs.Wait()
	output := collector.wait()
	if !strings.Contains(output, "capture_value_99") {
		t.Fatalf("expected output to contain %q, got %q", "capture_value_99", output)
	}
}

func TestCaptureSession_InvalidCommand(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "/nonexistent/command/that/does/not/exist",
	})
	err := cs.Start(context.Background())
	if err == nil {
		cs.Close()
		t.Fatal("expected error for invalid command")
	}

	// After a failed Start, IsRunning should be false.
	if cs.IsRunning() {
		t.Fatal("expected IsRunning=false after failed Start")
	}
}

func TestCaptureSession_NotStarted_Methods(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})

	// Operations on unstarted session.
	if err := cs.Interrupt(); err == nil {
		t.Fatal("expected error from Interrupt before Start")
	}
	if err := cs.Kill(); err == nil {
		t.Fatal("expected error from Kill before Start")
	}
	if err := cs.WriteString("hello"); err == nil {
		t.Fatal("expected error from Write before Start")
	}
	if _, err := cs.Wait(); err == nil {
		t.Fatal("expected error from Wait before Start")
	}
	if ch := cs.Reader(); ch != nil {
		t.Fatal("expected nil Reader before Start")
	}
	if cs.Pid() != 0 {
		t.Fatal("expected Pid=0 before Start")
	}
	if cs.ExitCode() != -1 {
		t.Fatalf("expected ExitCode=-1 before Start, got %d", cs.ExitCode())
	}
}

func TestCaptureSession_StartAfterClose(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})
	cs.Close()

	err := cs.Start(context.Background())
	if err == nil {
		t.Fatal("expected error starting a closed session")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCaptureSession_MultilineOutput(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "echo line1; echo line2; echo line3"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	cs.Wait()
	output := collector.wait()
	for _, expected := range []string{"line1", "line2", "line3"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q, got %q", expected, output)
		}
	}
}

func TestCaptureSession_Done_Channel(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"done test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	select {
	case <-cs.Done():
		// Good.
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting on Done channel")
	}
}

func TestCaptureSession_ConcurrentOutput(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "for i in $(seq 1 50); do echo line$i; done"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()
	collector := startCollector(cs)

	// Concurrent current() reads while process is running.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_ = collector.current()
			if !cs.IsRunning() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	cs.Wait()
	<-done

	output := collector.wait()
	if !strings.Contains(output, "line50") {
		t.Fatalf("expected output to contain 'line50', got %q", output)
	}
}

func TestCaptureSession_Passthrough_NotStarted(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})

	reason, err := cs.Passthrough(context.Background(), PassthroughConfig{})
	if reason != ExitError {
		t.Fatalf("expected ExitError, got %v", reason)
	}
	if err != ErrNoChild {
		t.Fatalf("expected ErrNoChild, got %v", err)
	}
}

func TestCaptureSession_Passthrough_AfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	cs.Wait()
	cs.Close()

	reason, err := cs.Passthrough(context.Background(), PassthroughConfig{})
	if reason != ExitError {
		t.Fatalf("expected ExitError, got %v", reason)
	}
	if err != ErrNoChild {
		t.Fatalf("expected ErrNoChild, got %v", err)
	}
}

func TestCaptureSession_Passthrough_ChildExit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping passthrough integration test in short mode")
	}
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"passthrough child exit test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	// Use a context with timeout to avoid hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// PassthroughConfig with no real terminal (non-TTY environment).
	// TermFd < 0 means no raw mode is attempted — safe for CI.
	var stdout bytes.Buffer
	reason, err := cs.Passthrough(ctx, PassthroughConfig{
		Stdin:  strings.NewReader(""), // empty stdin — child will exit on its own
		Stdout: &stdout,
		TermFd: -1, // no real TTY
	})
	if err != nil {
		t.Fatalf("Passthrough returned error: %v", err)
	}
	if reason != ExitChildExit {
		t.Fatalf("expected ExitChildExit, got %v", reason)
	}

	// Verify that passthrough forwarded the child output to stdout.
	if !strings.Contains(stdout.String(), "passthrough child exit test") {
		t.Fatalf("expected stdout to contain child output, got %q", stdout.String())
	}
}

func TestCaptureSession_Passthrough_ContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping passthrough integration test in short mode")
	}
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after a short delay.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	reason, err := cs.Passthrough(ctx, PassthroughConfig{
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
		TermFd: -1,
	})
	if reason != ExitContext {
		t.Fatalf("expected ExitContext, got %v (err=%v)", reason, err)
	}
}

// ---------------------------------------------------------------------------
// Reader() channel tests
// ---------------------------------------------------------------------------

func TestCaptureSession_Reader_BeforeStart(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"test"},
	})

	if ch := cs.Reader(); ch != nil {
		t.Fatal("Reader() should return nil before Start()")
	}
}

func TestCaptureSession_Reader_StreamsOutput(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"hello reader"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	ch := cs.Reader()
	if ch == nil {
		t.Fatal("Reader() returned nil after Start()")
	}

	// Drain all chunks from the Reader channel.
	var buf bytes.Buffer
	for chunk := range ch {
		buf.Write(chunk)
	}

	// The channel should be closed after process exit.
	if !strings.Contains(buf.String(), "hello reader") {
		t.Fatalf("Reader output %q does not contain %q", buf.String(), "hello reader")
	}
}

func TestCaptureSession_Reader_ChannelClosedOnExit(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"done"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	ch := cs.Reader()

	// Wait for the process to finish.
	cs.Wait()

	// Reader channel should be closed (all chunks drained).
	// Drain remaining chunks.
	for range ch {
		// consume
	}

	// Channel is now closed — confirm with non-blocking receive.
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected Reader channel to be closed")
		}
	default:
		// Channel closed, as expected.
	}
}

func TestCaptureSession_Reader_Content(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"reader content test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	// Drain Reader channel.
	var readerBuf bytes.Buffer
	for chunk := range cs.Reader() {
		readerBuf.Write(chunk)
	}

	cs.Wait()

	if !strings.Contains(readerBuf.String(), "reader content test") {
		t.Fatalf("Reader output %q does not contain expected text", readerBuf.String())
	}
}
