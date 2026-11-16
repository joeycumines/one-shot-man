package exec

import (
	"context"
	"errors"
	osexec "os/exec"
	"runtime"
	"testing"
	"time"
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
	cmd := contextErrorAfterCleanExit(t, ctx)
	cancel()

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
	if rejectErr := child.waitRejection(child.exitCode, child.exitErr); rejectErr != nil {
		t.Fatalf("a known exit 0 must resolve, rejected with: %v", rejectErr)
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
// report the context error alongside a valid ProcessState.
func contextErrorAfterCleanExit(t *testing.T, ctx context.Context) *osexec.Cmd {
	t.Helper()
	command, args := "sh", []string{"-c", "exit 0"}
	if runtime.GOOS == "windows" {
		command, args = "cmd.exe", []string{"/C", "exit 0"}
	}
	cmd := osexec.CommandContext(ctx, command, args...)
	cmd.Cancel = func() error { return nil }
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	// Give the child time to exit so Cancel sees a zombie, as in the real path.
	time.Sleep(200 * time.Millisecond)
	return cmd
}
