package exec

import (
	"bytes"
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type runCommandReadyWriter struct {
	buffer bytes.Buffer
	ready  chan struct{}
	once   sync.Once
}

func (w *runCommandReadyWriter) Write(p []byte) (int, error) {
	n, err := w.buffer.Write(p)
	if strings.Contains(w.buffer.String(), "ready") {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

// The safety requirement is that Start never admitted os/exec.watchCtx; merely
// inspecting Cancel after completion cannot prove that. Go is pinned in go.mod.
// Read the pinned implementation's retained watcher channel only after Wait,
// without unsafe access or concurrent runtime state inspection. A toolchain
// change to this field requires re-auditing the watchdog admission contract.
func assertRunCommandNoWatchdog(t *testing.T, cmd *osexec.Cmd) {
	t.Helper()
	field := reflect.ValueOf(cmd).Elem().FieldByName("ctxResult")
	if !field.IsValid() || field.Kind() != reflect.Chan {
		t.Fatal("pinned os/exec watcher state changed; re-audit watchdog admission")
	}
	if !field.IsNil() {
		t.Fatal("RunCommand admitted the independent os/exec cancellation watchdog")
	}
}

func TestRunCommandPreservesOutputAndWaitDelay(t *testing.T) {
	skipSlow(t)
	t.Setenv(outputLifecycleHelperEnv, "ascii")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := osexec.CommandContext(ctx, os.Args[0])
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	const waitDelay = 20 * time.Millisecond
	cmd.WaitDelay = waitDelay
	if err := RunCommand(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "0123456789abcdef" || stderr.String() != "ready" {
		t.Fatalf("output=(%q,%q)", stdout.String(), stderr.String())
	}
	if cmd.WaitDelay != waitDelay {
		t.Fatalf("WaitDelay=%v, want %v", cmd.WaitDelay, waitDelay)
	}
	assertRunCommandNoWatchdog(t, cmd)
}

func TestRunCommandOwnsCancellationWithWaitDelay(t *testing.T) {
	skipSlow(t)
	t.Setenv(outputLifecycleHelperEnv, "wait")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := osexec.CommandContext(ctx, os.Args[0])
	ready := &runCommandReadyWriter{ready: make(chan struct{})}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = ready
	cmd.WaitDelay = time.Millisecond
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	result := make(chan error, 1)
	defer func() {
		cancel()
		_ = stdin.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("owned command did not join during cleanup")
		}
	}()
	go func() { defer close(done); result <- RunCommand(ctx, cmd) }()
	select {
	case <-ready.ready:
	case err := <-result:
		t.Fatalf("command exited before readiness: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("command did not become ready")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunCommand=%v, want caller cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("owned cancellation did not finish")
	}
	if cmd.ProcessState == nil {
		t.Fatal("cancellation returned without reaping")
	}
	if cmd.WaitDelay != time.Millisecond {
		t.Fatal("WaitDelay was not restored")
	}
	assertRunCommandNoWatchdog(t, cmd)
}

func TestRunCommandRestoresWaitDelayOnStartFailure(t *testing.T) {
	skipSlow(t)
	ctx := context.Background()
	cmd := osexec.CommandContext(ctx, filepath.Join(t.TempDir(), "missing-command"))
	cmd.WaitDelay = time.Second
	if err := RunCommand(ctx, cmd); err == nil {
		t.Fatal("missing command unexpectedly started")
	}
	if cmd.WaitDelay != time.Second || cmd.Process != nil {
		t.Fatalf("failed start changed caller state: WaitDelay=%v Process=%v", cmd.WaitDelay, cmd.Process)
	}
	assertRunCommandNoWatchdog(t, cmd)
}

func TestRunCommandRejectsCancelledCallerBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command, args := immediateExitCommand()
	cmd := osexec.Command(command, args...)
	cmd.WaitDelay = time.Second
	if err := RunCommand(ctx, cmd); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunCommand=%v", err)
	}
	if cmd.Process != nil || cmd.WaitDelay != time.Second {
		t.Fatal("cancelled caller mutated or started its command")
	}
}
