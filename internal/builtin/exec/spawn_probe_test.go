package exec

// A failed pre-reap probe is a process-lifecycle failure: the owner must
// attempt termination while the PID is still pinned, then report the failure.

import (
	"errors"
	"os"
	osexec "os/exec"
	"runtime"
	"syscall"
	"testing"
)

// recordingTree records the close ordering without touching real processes.
type recordingTree struct {
	closed int
}

func (*recordingTree) beginReap(bool)                      {}
func (*recordingTree) signal(*osexec.Cmd, os.Signal) error { return nil }
func (r *recordingTree) attach(*osexec.Cmd) error          { return nil }
func (r *recordingTree) kill(*osexec.Cmd) error            { return nil }
func (r *recordingTree) close(*osexec.Cmd) error {
	r.closed++
	return nil
}

type reapRetryTree struct {
	closed     int
	persistent bool
}

func (*reapRetryTree) beginReap(bool)                      {}
func (*reapRetryTree) signal(*osexec.Cmd, os.Signal) error { return nil }
func (*reapRetryTree) attach(*osexec.Cmd) error            { return nil }
func (*reapRetryTree) kill(*osexec.Cmd) error              { return nil }
func (r *reapRetryTree) close(cmd *osexec.Cmd) error {
	r.closed++
	if r.persistent || cmd.ProcessState == nil {
		return syscall.EPERM
	}
	return nil
}

// TestWaitAndCloseProcessTree_ProbeFailureIsReported retains the exact cause
// even if the child happened to exit successfully before fallback teardown.
func TestWaitAndCloseProcessTree_ProbeFailureIsReported(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}

	for _, tc := range []struct {
		name  string
		probe error
	}{
		// Any observation failure requires conservative teardown and a
		// diagnostic, independently of the recorded direct-child status.
		{"unsupported platform", errors.New("pre-reap process observation is unavailable on this platform")},
		{"genuine probe failure", errors.New("ECHILD")},
		{"kqueue failure", errors.New("EVFILT_PROC lookup failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := func(*osexec.Cmd) (bool, error) { return false, tc.probe }

			// `true` exits 0 immediately.
			cmd := osexec.Command("sh", "-c", "exit 0")
			if err := cmd.Start(); err != nil {
				t.Skipf("cannot start helper process: %v", err)
			}
			tree := &recordingTree{}

			waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, probe)
			if waitErr != nil {
				if _, ok := errors.AsType[*osexec.ExitError](waitErr); !ok {
					t.Errorf("unexpected wait failure: %v", waitErr)
				}
			}
			if !errors.Is(closeErr, tc.probe) {
				t.Errorf("closeErr = %v, want original observation failure %v", closeErr, tc.probe)
			}
			if tree.closed != 1 {
				t.Errorf("tree closed %d times, want 1", tree.closed)
			}
		})
	}
}

func TestWaitAndCloseProcessTree_ProbeFailureAbortsBeforeWaiting(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}
	probeErr := errors.New("pre-reap process observation is unavailable")
	cmd := osexec.Command("sh", "-c", "read line")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	tree := &recordingTree{}
	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, func(*osexec.Cmd) (bool, error) { return false, probeErr })
	var exitErr *osexec.ExitError
	if !errors.As(waitErr, &exitErr) || processExitSignal(waitErr) != "SIGKILL" {
		t.Fatalf("blocked command was not terminated before waiting: %v", waitErr)
	}
	if !errors.Is(closeErr, probeErr) || tree.closed != 1 {
		t.Fatalf("close=%v calls=%d, want original probe failure and one close", closeErr, tree.closed)
	}
}

// TestWaitAndCloseProcessTree_ProbeSuccessClosesBeforeWait pins the ordering
// the probe exists for.
func TestWaitAndCloseProcessTree_ProbeSuccessClosesBeforeWait(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}
	probe := func(*osexec.Cmd) (bool, error) { return true, nil }

	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	tree := &recordingTree{}
	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, probe)
	if waitErr != nil || closeErr != nil {
		t.Fatalf("waitErr=%v closeErr=%v, want nil nil", waitErr, closeErr)
	}
	if tree.closed != 1 {
		t.Errorf("tree closed %d times, want 1", tree.closed)
	}
}

func TestWaitAndCloseProcessTree_RetriesPermissionFailureAfterReap(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group behavior")
	}
	probe := func(*osexec.Cmd) (bool, error) { return true, nil }

	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	tree := &reapRetryTree{}

	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, probe)
	if waitErr != nil || closeErr != nil {
		t.Fatalf("waitErr=%v closeErr=%v, want nil nil", waitErr, closeErr)
	}
	if tree.closed != 2 {
		t.Errorf("tree closed %d times, want 2", tree.closed)
	}
}

func TestWaitAndCloseProcessTree_PropagatesPersistentPermissionFailure(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group behavior")
	}
	probe := func(*osexec.Cmd) (bool, error) { return true, nil }

	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	tree := &reapRetryTree{persistent: true}

	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, probe)
	if waitErr != nil {
		t.Errorf("waitErr = %v, want nil for an exit-0 command", waitErr)
	}
	if !errors.Is(closeErr, syscall.EPERM) {
		t.Errorf("closeErr = %v, want EPERM", closeErr)
	}
	if tree.closed != 2 {
		t.Errorf("tree closed %d times, want 2", tree.closed)
	}
}
