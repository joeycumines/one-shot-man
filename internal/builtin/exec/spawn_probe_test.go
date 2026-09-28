package exec

// spawn_probe_test.go — the pre-reap probe must never decide a command's
// success.
//
// The probe exists to observe a child's exit state before reaping it so the
// process tree can be closed in the right order. It is an ordering
// optimization: when it fails, cmd.Wait still decides the outcome. Folding
// the probe error into the returned error reported a command that exited 0 as
// failed, which failed verify and test steps that had actually passed.

import (
	"errors"
	"fmt"
	osexec "os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// recordingTree records the close ordering without touching real processes.
type recordingTree struct {
	closed       int
	closedSignal chan struct{}
}

func (r *recordingTree) attach(*osexec.Cmd) error { return nil }
func (r *recordingTree) kill(*osexec.Cmd) error   { return nil }
func (r *recordingTree) close(*osexec.Cmd) error {
	r.closed++
	if r.closedSignal != nil {
		close(r.closedSignal)
	}
	return nil
}

type reapRetryTree struct {
	closed     int
	persistent bool
}

func (*reapRetryTree) attach(*osexec.Cmd) error { return nil }
func (*reapRetryTree) kill(*osexec.Cmd) error   { return nil }
func (r *reapRetryTree) close(cmd *osexec.Cmd) error {
	r.closed++
	if r.persistent || cmd.ProcessState == nil {
		return syscall.EPERM
	}
	return nil
}

// TestWaitAndCloseProcessTree_ProbeFailureDoesNotFailCommand is the regression
// guard: a genuine probe failure (not just "unsupported") must not turn a
// successful command into an error.
func TestWaitAndCloseProcessTree_ProbeFailureDoesNotFailCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}
	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })

	for _, tc := range []struct {
		name  string
		probe error
	}{
		// The sentinel only exists on the fallback platform, so mirror its
		// text here: the contract under test is that ANY probe error leaves
		// cmd.Wait as the sole decider.
		{"unsupported platform", errors.New("pre-reap process observation is unavailable on this platform")},
		{"genuine probe failure", errors.New("ECHILD")},
		{"kqueue failure", errors.New("EVFILT_PROC lookup failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) { return false, tc.probe }

			// `true` exits 0 immediately.
			cmd := osexec.Command("sh", "-c", "exit 0")
			if err := cmd.Start(); err != nil {
				t.Skipf("cannot start helper process: %v", err)
			}
			tree := &recordingTree{}

			waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
			if waitErr != nil {
				t.Errorf("waitErr = %v, want nil for an exit-0 command", waitErr)
			}
			if closeErr != nil {
				t.Errorf("closeErr = %v, want nil", closeErr)
			}
			if tree.closed != 1 {
				t.Errorf("tree closed %d times, want 1", tree.closed)
			}
		})
	}
}

func TestWaitAndCloseProcessTree_ProbeFailureWaitsBeforeClosing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}
	if testing.Short() {
		t.Skip("skipping subprocess lifecycle test in short mode")
	}

	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })
	waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) {
		return false, errors.New("pre-reap process observation is unavailable")
	}

	cmd := osexec.Command("sh", "-c", "read line")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}

	tree := &recordingTree{closedSignal: make(chan struct{})}
	type result struct {
		waitErr  error
		closeErr error
	}
	resultCh := make(chan result, 1)
	go func() {
		waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
		resultCh <- result{waitErr: waitErr, closeErr: closeErr}
	}()

	closedEarly := false
	select {
	case <-tree.closedSignal:
		closedEarly = true
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := fmt.Fprintln(stdin, "release"); err != nil {
		t.Fatalf("release child: %v", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatalf("close child stdin: %v", err)
	}
	got := <-resultCh
	if closedEarly {
		t.Fatal("process tree closed before the running child exited")
	}
	if got.waitErr != nil || got.closeErr != nil {
		t.Fatalf("waitErr=%v closeErr=%v, want nil nil", got.waitErr, got.closeErr)
	}
	if tree.closed != 1 {
		t.Errorf("tree closed %d times, want 1", tree.closed)
	}
}

// TestWaitAndCloseProcessTree_ProbeSuccessClosesBeforeWait pins the ordering
// the probe exists for.
func TestWaitAndCloseProcessTree_ProbeSuccessClosesBeforeWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell helper")
	}
	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })
	waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) { return true, nil }

	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	tree := &recordingTree{}
	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
	if waitErr != nil || closeErr != nil {
		t.Fatalf("waitErr=%v closeErr=%v, want nil nil", waitErr, closeErr)
	}
	if tree.closed != 1 {
		t.Errorf("tree closed %d times, want 1", tree.closed)
	}
}

func TestWaitAndCloseProcessTree_RetriesPermissionFailureAfterReap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group behavior")
	}
	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })
	waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) { return true, nil }

	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	tree := &reapRetryTree{}

	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
	if waitErr != nil || closeErr != nil {
		t.Fatalf("waitErr=%v closeErr=%v, want nil nil", waitErr, closeErr)
	}
	if tree.closed != 2 {
		t.Errorf("tree closed %d times, want 2", tree.closed)
	}
}

func TestWaitAndCloseProcessTree_PropagatesPersistentPermissionFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group behavior")
	}
	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })
	waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) { return true, nil }

	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	tree := &reapRetryTree{persistent: true}

	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
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
