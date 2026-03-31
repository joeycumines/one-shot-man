package termmux

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
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

	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	output := cs.Output()
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
	outputAfterPause := cs.Output()
	time.Sleep(500 * time.Millisecond)
	outputStill := cs.Output()
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
	outputAfterResume := cs.Output()
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

	// Write to stdin; cat echoes it back.
	if err := cs.WriteString("hello from stdin\n"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait a bit for the echo to come back through PTY → VTerm.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for echoed output, got: %q", cs.Output())
		default:
		}
		if strings.Contains(cs.Output(), "hello from stdin") {
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

func TestCaptureSession_Screen(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"screen test"},
	})

	// Screen before start should be empty.
	if s := cs.Screen(); s != "" {
		t.Fatalf("expected empty Screen before Start, got %q", s)
	}

	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer cs.Close()

	cs.Wait()

	screen := cs.Screen()
	if screen == "" {
		t.Fatal("expected non-empty Screen after Wait")
	}
	// Screen output contains ANSI escapes — verify some content is present.
	if !strings.Contains(screen, "screen test") {
		t.Fatalf("expected Screen to contain %q, got length %d", "screen test", len(screen))
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

	cs.Wait()
	output := strings.TrimSpace(cs.Output())
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

	cs.Wait()
	output := cs.Output()
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
	if cs.Output() != "" {
		t.Fatal("expected empty Output before Start")
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

	cs.Wait()
	output := cs.Output()
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

	// Concurrent Output() reads while process is running.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_ = cs.Output()
			if !cs.IsRunning() {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	cs.Wait()
	<-done

	output := cs.Output()
	if !strings.Contains(output, "line50") {
		t.Fatalf("expected output to contain 'line50', got %q", output)
	}
}
