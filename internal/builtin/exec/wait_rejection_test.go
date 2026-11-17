package exec

import (
	"context"
	"errors"
	"fmt"
	osexec "os/exec"
	"runtime"
	"syscall"
	"testing"
)

// exitCodeCommand returns a shell command that exits with the given code.
func exitCodeCommand(code int) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", fmt.Sprintf("exit %d", code)}
	}
	return "sh", []string{"-c", fmt.Sprintf("exit %d", code)}
}

// exitErrorFor produces a real *osexec.ExitError with the given code by
// running a trivial child, so the decision table is fed the same value type
// the reaper records rather than a synthesized struct with no ProcessState.
func exitErrorFor(t *testing.T, code int) *osexec.ExitError {
	t.Helper()
	command, args := exitCodeCommand(code)
	cmd := osexec.Command(command, args...)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("helper command for exit %d unexpectedly succeeded", code)
	}
	var exitErr *osexec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("helper command for exit %d returned %T, want *osexec.ExitError", code, err)
	}
	return exitErr
}

// TestWaitRejection covers the decision table that turns a reaped child into a
// JS wait() settlement.
//
// wait() used to reject whenever its error chain carried a
// processTreeCloseError, even alongside a valid exit status. A timeout-killed
// child whose process group had already gone away hit exactly that: wait()
// rejected with "signal: killed" although the child demonstrably exited, JS
// saw an unhandled rejection, and the adapter's fatal-exit path shut the event
// loop down, stranding every other pending promise in the process.
//
// The asymmetry went further than the timeout case: a child that exited 0 had
// `cmd.Wait() == nil`, so its recorded error chain held *only* the tree-close
// failure and wait() rejected, while the identical cleanup failure next to
// `exit 1` resolved. A real exit status therefore has to be recorded as a fact
// about the child (exitStatus) rather than inferred from the error type, which
// is what these cases pin down.
func TestWaitRejection(t *testing.T) {
	exitSeven := exitErrorFor(t, 7)
	exitOne := exitErrorFor(t, 1)

	for _, tc := range []struct {
		name          string
		waitErr       error
		closeErr      error
		wantCode      int
		wantReject    bool
		skipOnWindows bool
	}{
		{
			name:     "exit zero",
			wantCode: 0,
		},
		{
			// The reported defect: a clean exit whose process-group close
			// failed. It resolves, because the exit status is a fact.
			name:     "exit zero with tree close failure",
			closeErr: syscall.EPERM,
			wantCode: 0,
		},
		{
			name:     "non-zero exit",
			waitErr:  exitSeven,
			wantCode: 7,
		},
		{
			name:     "non-zero exit with tree close failure",
			waitErr:  exitOne,
			closeErr: syscall.EPERM,
			wantCode: 1,
		},
		{
			name:       "opaque wait failure",
			waitErr:    fmt.Errorf("wait failed"),
			wantCode:   -1,
			wantReject: true,
		},
		{
			name:       "opaque wait failure with tree close failure",
			waitErr:    fmt.Errorf("wait failed"),
			closeErr:   syscall.EPERM,
			wantCode:   -1,
			wantReject: true,
		},
		{
			name:       "context cancellation",
			waitErr:    fmt.Errorf("wait: %w", context.Canceled),
			wantCode:   -1,
			wantReject: true,
		},
		{
			// A synthesized ExitError carries no ProcessState, so there is no
			// signal to read: a negative code with no signal is unclassified
			// and must reject rather than masquerade as a status.
			// processExitSignal tolerates the nil ProcessState.
			name:          "unclassified negative code with exit status in chain",
			waitErr:       &osexec.ExitError{},
			wantCode:      -1,
			wantReject:    true,
			skipOnWindows: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.skipOnWindows && runtime.GOOS == "windows" {
				t.Skip("Windows exit codes are unsigned, so a negative code is never unclassified there")
			}
			child := &ChildProcess{}
			child.recordExit(tc.waitErr, tc.closeErr)

			if child.exitCode != tc.wantCode {
				t.Fatalf("exitCode = %d, want %d", child.exitCode, tc.wantCode)
			}
			// Use the values the reaper actually recorded, not the table's
			// inputs: passing (tc.wantCode, tc.waitErr) let the headline
			// exit-zero-with-tree-close row call waitRejection(0, nil) and
			// assert nothing about exitStatus.
			if got := child.waitRejection(child.exitCode, child.exitErr) != nil; got != tc.wantReject {
				t.Fatalf("waitRejection(%v) rejected = %v, want %v (exitCode=%d exitStatus=%v exitErr=%v)",
					tc.waitErr, got, tc.wantReject, child.exitCode, child.exitStatus, child.exitErr)
			}
		})
	}
}

// TestWaitRejection_TreeCloseFailureAfterCleanExit is the end-to-end form of
// the decision above: a real child exits 0, the process-group close then
// fails, and the wait must still deliver {code: 0, signal: null}.
//
// This is the shape a Darwin timeout produced in CI, where the only group
// member is the exited-but-unreaped leader and the pre-reap signal returns
// EPERM.
func TestWaitRejection_TreeCloseFailureAfterCleanExit(t *testing.T) {
	command, args := immediateExitCommand()
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}

	waitErr, closeErr := waitAndCloseProcessTree(cmd, &failingCloseTree{err: syscall.EPERM})
	if waitErr != nil {
		t.Fatalf("waitErr = %v, want nil for an exit-0 command", waitErr)
	}
	if !errors.Is(closeErr, syscall.EPERM) {
		t.Fatalf("closeErr = %v, want EPERM", closeErr)
	}

	// cmd is attached so recordExit takes the real ProcessState branch (the
	// branch every production child takes), not the nil-cmd fallback.
	child := &ChildProcess{cmd: cmd}
	child.recordExit(waitErr, closeErr)

	if child.exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", child.exitCode)
	}
	if !child.exitStatus {
		t.Fatal("exitStatus = false, want true for a child whose exit 0 was observed")
	}
	if rejectErr := child.waitRejection(child.exitCode, child.exitErr); rejectErr != nil {
		t.Fatalf("a clean exit must outrank the tree-cleanup failure, rejected with: %v", rejectErr)
	}
	if child.exitErr == nil || !errors.Is(child.exitErr, syscall.EPERM) {
		t.Fatalf("the cleanup failure must still be carried for Go callers, got %v", child.exitErr)
	}
}

// TestWaitRejection_TreeCloseFailureAfterNonZeroExit keeps the case that
// already worked from regressing: a non-zero exit alongside a cleanup failure
// resolves with its code.
func TestWaitRejection_TreeCloseFailureAfterNonZeroExit(t *testing.T) {
	command, args := exitCodeCommand(1)
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}

	waitErr, closeErr := waitAndCloseProcessTree(cmd, &failingCloseTree{err: syscall.EPERM})
	// cmd is attached so recordExit takes the real ProcessState branch (the
	// branch every production child takes), not the nil-cmd fallback.
	child := &ChildProcess{cmd: cmd}
	child.recordExit(waitErr, closeErr)

	if child.exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", child.exitCode)
	}
	if !child.exitStatus {
		t.Fatal("exitStatus = false, want true for a child whose exit 1 was observed")
	}
	if child.exitErr == nil || !errors.As(child.exitErr, new(*osexec.ExitError)) {
		t.Fatalf("exitErr = %v, want the wait error preserved for Go callers", child.exitErr)
	}
	if rejectErr := child.waitRejection(child.exitCode, child.exitErr); rejectErr != nil {
		t.Fatalf("non-zero exit must resolve, rejected with: %v", rejectErr)
	}
}

// failingCloseTree fails every close, standing in for a process group that has
// already gone away.
type failingCloseTree struct {
	err error
}

func (*failingCloseTree) attach(*osexec.Cmd) error { return nil }
func (*failingCloseTree) kill(*osexec.Cmd) error   { return nil }
func (f *failingCloseTree) close(*osexec.Cmd) error {
	return f.err
}
