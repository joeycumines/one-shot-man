package exec

import (
	"errors"
	osexec "os/exec"
	"runtime"
	"syscall"
	"testing"
)

// transientEPERMTree fails only its first close, modelling the Darwin case
// where the group is not yet addressable at the moment of the post-wait close.
type transientEPERMTree struct {
	closed int
}

func (*transientEPERMTree) attach(*osexec.Cmd) error { return nil }
func (*transientEPERMTree) kill(*osexec.Cmd) error   { return nil }
func (t *transientEPERMTree) close(*osexec.Cmd) error {
	t.closed++
	if t.closed == 1 {
		return syscall.EPERM
	}
	return nil
}

// TestWaitAndCloseProcessTree_ProbeErrorRetriesEPERM pins the contract that a
// transient EPERM from a post-wait process-group close is retried on EVERY
// branch, not only the pre-reap-close branch. The Darwin failure named in the
// changelog takes the probe-error branch: the pre-reap observation fails, the
// child is reaped first, and the group close can still return EPERM while the
// only group member is the exited leader.
func TestWaitAndCloseProcessTree_ProbeErrorRetriesEPERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group EPERM semantics are Unix-specific")
	}
	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })
	waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) {
		return false, errors.New("probe unavailable")
	}

	command, args := immediateExitCommand()
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	// The first post-wait close returns EPERM; the retry succeeds.
	tree := &transientEPERMTree{}
	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
	if waitErr != nil {
		t.Fatalf("waitErr = %v, want nil for an exit-0 command", waitErr)
	}
	if closeErr != nil {
		t.Fatalf("closeErr = %v, want nil after a successful EPERM retry, got %v", closeErr, closeErr)
	}
	if tree.closed != 2 {
		t.Fatalf("tree closed %d times, want 2 (EPERM then retry)", tree.closed)
	}
}

// TestWaitAndCloseProcessTree_NoSafeCloseRetriesEPERM covers the third branch:
// the platform reports no safe pre-reap close point, so the close happens
// after Wait and must still get the same transient-EPERM retry.
func TestWaitAndCloseProcessTree_NoSafeCloseRetriesEPERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group EPERM semantics are Unix-specific")
	}
	orig := waitProcessBeforeReapFn
	t.Cleanup(func() { waitProcessBeforeReapFn = orig })
	waitProcessBeforeReapFn = func(*osexec.Cmd) (bool, error) { return false, nil }

	command, args := immediateExitCommand()
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	tree := &transientEPERMTree{}
	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
	if waitErr != nil || closeErr != nil {
		t.Fatalf("(wait, close) = (%v, %v), want both nil after EPERM retry", waitErr, closeErr)
	}
	if tree.closed != 2 {
		t.Fatalf("tree closed %d times, want 2 (EPERM then retry)", tree.closed)
	}
}
