package exec

import (
	"context"
	"errors"
	"io"
	osexec "os/exec"
	"runtime"
	"testing"
)

// TestRecordExit_ContextErrorWithProcessState pins the defect where recordExit
// inferred the exit status from the error's type instead of reading the status
// cmd.Wait() always stores in cmd.ProcessState.
//
// os/exec's documented Cmd.Cancel semantics make this reachable without a race:
// when Cancel returns nil (as unixProcessTree.kill does for an already-dead
// leader) and the child had already exited 0, Wait returns the context error
// AND ProcessState reports ExitCode 0. The old inference recorded
// exitStatus=false, code=-1 and wait() rejected, tearing down the event loop
// for a command whose status was fully known.
func TestRecordExit_ContextErrorWithProcessState(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a short-lived helper process")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, cancelCalled := contextErrorAfterCleanExit(t, ctx)
	cancel()
	// Handshake, not a sleep: watchCtx is committed to the ctx.Done() branch
	// once Cancel has run, so Wait is guaranteed to observe the context error
	// alongside the real ProcessState no matter how fast the child exited.
	<-cancelCalled

	waitErr := cmd.Wait()
	if waitErr == nil || (!errors.Is(waitErr, context.Canceled) && !errors.Is(waitErr, context.DeadlineExceeded)) {
		t.Fatalf("Wait error = %v, want a context error for the reproduced case", waitErr)
	}
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("ProcessState = %v, want an observed exit code 0", cmd.ProcessState)
	}

	child := &ChildProcess{cmd: cmd}
	child.recordExit(waitErr, nil)

	if !child.exitStatus {
		t.Fatal("exitStatus = false, want true: ProcessState holds a real exit status")
	}
	if child.exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", child.exitCode)
	}
	// The wait error must survive recordExit for Go callers of WaitContext:
	// dropping it here is what made waitRejection(child.exitCode, child.exitErr)
	// below a vacuous waitRejection(0, nil).
	if child.exitErr == nil || !errors.Is(child.exitErr, context.Canceled) {
		t.Fatalf("exitErr = %v, want the context error preserved for Go callers", child.exitErr)
	}
	if rejectErr := child.waitRejection(child.exitCode, child.exitErr); rejectErr != nil {
		t.Fatalf("a known exit 0 must resolve, rejected with: %v", rejectErr)
	}
}

// TestRecordExit_ReapedNonZeroExitPreservesWaitErr pins the Go-facing contract
// for the ordinary case: a reaped child that exited non-zero must keep its
// *osexec.ExitError in exitErr, because WaitContext returns (code, exitErr) to
// Go callers who use err != nil to detect failure.
func TestRecordExit_ReapedNonZeroExitPreservesWaitErr(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a short-lived helper process")
	}
	command, args := exitCodeCommand(7)
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitErr := cmd.Wait()
	if waitErr == nil {
		t.Fatal("Wait error = nil, want a non-zero ExitError")
	}
	var exitErr *osexec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("Wait error = %T, want *osexec.ExitError", waitErr)
	}
	if cmd.ProcessState == nil {
		t.Fatal("ProcessState = nil, want the reaped state")
	}

	child := &ChildProcess{cmd: cmd}
	child.recordExit(waitErr, nil)

	if child.exitCode != 7 || !child.exitStatus {
		t.Fatalf("(exitCode, exitStatus) = (%d, %v), want (7, true)", child.exitCode, child.exitStatus)
	}
	if child.exitErr == nil || !errors.As(child.exitErr, &exitErr) {
		t.Fatalf("exitErr = %v, want the *osexec.ExitError preserved for Go callers", child.exitErr)
	}
	// JS settlement still resolves with the recorded code, not a rejection.
	if rejectErr := child.waitRejection(child.exitCode, child.exitErr); rejectErr != nil {
		t.Fatalf("a known exit 7 must resolve with its code, rejected with: %v", rejectErr)
	}
}

// TestWaitContext_CallerAbortBeforeReapRejects pins the other side of the
// completion boundary: when the caller's context cancels BEFORE the reaper has
// recorded anything, WaitContext must report the abort as a waitAbortedError
// (never resolve {code: -1} from a status this wait never observed), and
// waitRejection must reject it.
func TestWaitContext_CallerAbortBeforeReapRejects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal semantics")
	}
	if testing.Short() {
		t.Skip("spawns a short-lived helper process")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := osexec.CommandContext(ctx, "sh", "-c", "kill -TERM $$; sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	_ = cmd.Wait()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != -1 {
		t.Fatalf("ProcessState = %v, want a signal exit (-1)", cmd.ProcessState)
	}

	// done is never closed and recordExit never ran: the reaper has not
	// recorded, so a caller-aborted wait must report the abort.
	child := &ChildProcess{cmd: cmd, done: make(chan struct{})}

	abortCtx, abort := context.WithCancel(context.Background())
	abort()
	code, err := child.WaitContext(abortCtx)
	if code != -1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitContext = (%d, %v), want (-1, context.Canceled) for a caller-aborted wait", code, err)
	}
	var aborted *waitAbortedError
	if !errors.As(err, &aborted) {
		t.Fatalf("WaitContext error = %T, want *waitAbortedError so waitRejection rejects unconditionally", err)
	}
	if rejectErr := child.waitRejection(code, err); rejectErr == nil || !errors.Is(rejectErr, context.Canceled) {
		t.Fatalf("a caller-aborted wait must reject with the context error, got %v", rejectErr)
	}
}

// TestWaitContext_RecordedExitWinsOverConcurrentCancel pins the completion
// boundary from the other side: recordExit has recorded a real status but
// done is not closed yet when the caller's context cancels. WaitContext must
// return the recorded exit — not (-1, context.Canceled) — and waitRejection
// must resolve it, because the status was observed before the abort.
func TestWaitContext_RecordedExitWinsOverConcurrentCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a short-lived helper process")
	}
	command, args := exitCodeCommand(0)
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		t.Fatalf("Wait error = %v, want nil for exit 0", waitErr)
	}
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("ProcessState = %v, want an observed exit code 0", cmd.ProcessState)
	}

	// Simulate the window between recordExit and close(done): the reaper has
	// recorded, but done is still open.
	child := &ChildProcess{cmd: cmd, done: make(chan struct{})}
	child.recordExit(waitErr, nil)
	if !child.exitStatus || child.exitCode != 0 {
		t.Fatalf("(exitStatus, exitCode) = (%v, %d), want (true, 0)", child.exitStatus, child.exitCode)
	}

	abortCtx, abort := context.WithCancel(context.Background())
	abort()
	code, err := child.WaitContext(abortCtx)
	if code != 0 || err != nil {
		t.Fatalf("WaitContext = (%d, %v), want (0, nil): an already-recorded exit wins over the concurrent cancel", code, err)
	}
	if rejectErr := child.waitRejection(code, err); rejectErr != nil {
		t.Fatalf("a recorded exit 0 must resolve, rejected with: %v", rejectErr)
	}
}

// TestRecordExit_SignalledChildRecordsSignal pins the other half of the status
// authority: a child killed by a signal has ExitCode -1, and the signal must be
// recorded from ProcessState so a context-error chain cannot hide it and turn a
// classified status into an unclassified rejection.
func TestRecordExit_SignalledChildRecordsSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal semantics")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := osexec.CommandContext(ctx, "sh", "-c", "kill -TERM $$; sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitErr := cmd.Wait()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != -1 {
		t.Fatalf("ProcessState = %v, want a signal exit (-1)", cmd.ProcessState)
	}

	child := &ChildProcess{cmd: cmd}
	child.recordExit(waitErr, nil)
	if child.recordedSignal == "" {
		t.Fatal("recordedSignal is empty, want the signal from ProcessState")
	}
	// A context-error chain (as os/exec can return) must not undo the recorded
	// signal: the status is known and wait() must resolve.
	if rejectErr := child.waitRejection(child.exitCode, context.Canceled); rejectErr != nil {
		t.Fatalf("a signalled child with a recorded signal must resolve, rejected with: %v", rejectErr)
	}
}

// contextErrorAfterCleanExit starts a child that exits 0, waits until it has
// exited, and installs a Cancel that returns nil — the shape os/exec uses to
// report the context error alongside a valid ProcessState. It returns the cmd
// and a channel that is closed once Cancel has actually run, so the test can
// synchronize on the watchCtx handshake instead of sleeping a fixed interval:
// os/exec's watcher selects between ctx.Done() and its result channel, and
// once Cancel ran the cancellation branch is committed, making Wait's context
// error deterministic regardless of how quickly the child exited.
func contextErrorAfterCleanExit(t *testing.T, ctx context.Context) (*osexec.Cmd, <-chan struct{}) {
	t.Helper()
	command, args := "sh", []string{"-c", "exit 0"}
	if runtime.GOOS == "windows" {
		command, args = "cmd.exe", []string{"/C", "exit 0"}
	}
	cmd := osexec.CommandContext(ctx, command, args...)
	cancelCalled := make(chan struct{})
	cmd.Cancel = func() error {
		close(cancelCalled)
		return nil
	}
	// Pipe stdout and read it to EOF in the background: EOF proves the child
	// has exited, so Cancel sees a zombie exactly as in the real path, with no
	// fixed sleep to flake under load.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	go func() {
		_, _ = io.Copy(io.Discard, stdout)
	}()
	return cmd, cancelCalled
}
