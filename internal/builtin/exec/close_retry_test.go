package exec

import (
	"errors"
	"os"
	osexec "os/exec"
	"runtime"
	"syscall"
	"testing"
)

func TestCleanupFailedRunExec_RetriesEPERMAfterReap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		retryErr error
	}{
		{name: "recovered"},
		{name: "permission_failure", retryErr: syscall.EPERM},
		{name: "observation_failure", retryErr: syscall.EIO},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			primary := errors.New("attach failed")
			var reaped bool
			closes := 0
			var events []string
			err := cleanupFailedRunExec(primary,
				func() error { events = append(events, "kill"); return nil },
				func() error {
					events = append(events, "close-tree")
					closes++
					if closes == 1 {
						return syscall.EPERM
					}
					if !reaped {
						t.Fatal("retry ran before the child was reaped")
					}
					return tc.retryErr
				},
				func() { events = append(events, "begin-reap") },
				func() error { events = append(events, "wait"); reaped = true; return nil },
				func() error { events = append(events, "close-pipes"); return nil },
			)
			wantEvents := []string{"kill", "close-tree", "begin-reap", "wait", "close-tree", "close-pipes"}
			if len(events) != len(wantEvents) {
				t.Fatalf("cleanup events = %v, want %v", events, wantEvents)
			}
			for i := range wantEvents {
				if events[i] != wantEvents[i] {
					t.Fatalf("cleanup events = %v, want %v", events, wantEvents)
				}
			}
			if !errors.Is(err, primary) {
				t.Fatalf("primary failure was lost: %v", err)
			}
			if tc.retryErr == nil {
				if errors.Is(err, syscall.EPERM) || treeCleanupErrorMessage(err) != "" {
					t.Fatalf("successful retry kept a stale tree failure: %v", err)
				}
			} else if !errors.Is(err, syscall.EPERM) || !errors.Is(err, tc.retryErr) || treeCleanupErrorMessage(err) == "" {
				t.Fatalf("failed retry lost original or final tree failure: %v", err)
			}
		})
	}
}

// transientEPERMTree fails only its first close, modelling the Darwin case
// where the group is not yet addressable at the moment of the post-wait close.
type transientEPERMTree struct {
	closed int
}

func (*transientEPERMTree) beginReap(bool)                      {}
func (*transientEPERMTree) signal(*osexec.Cmd, os.Signal) error { return nil }
func (*transientEPERMTree) attach(*osexec.Cmd) error            { return nil }
func (*transientEPERMTree) kill(*osexec.Cmd) error              { return nil }
func (t *transientEPERMTree) close(*osexec.Cmd) error {
	t.closed++
	if t.closed == 1 {
		return syscall.EPERM
	}
	return nil
}

// Failed observation triggers owned teardown before reap. A recovered EPERM
// must disappear, while the independent observation failure remains visible.
func TestWaitAndCloseProcessTree_ProbeErrorRetriesEPERM(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("process-group EPERM semantics are Unix-specific")
	}
	probeErr := errors.New("probe unavailable")
	probe := func(*osexec.Cmd) (bool, error) { return false, probeErr }

	command, args := immediateExitCommand()
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	// The first pre-reap close returns EPERM; the post-reap retry succeeds.
	tree := &transientEPERMTree{}
	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, probe)
	var exitErr *osexec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		t.Fatal(waitErr)
	}
	if !errors.Is(closeErr, probeErr) || errors.Is(closeErr, syscall.EPERM) {
		t.Fatalf("closeErr = %v, want probe failure without recovered EPERM", closeErr)
	}
	if tree.closed != 2 {
		t.Fatalf("tree closed %d times, want 2 (EPERM then retry)", tree.closed)
	}
}

// An unavailable observation uses the same pre-reap teardown and recovery,
// preserving the lifecycle failure independently of a recovered close error.
func TestWaitAndCloseProcessTree_NoSafeCloseRetriesEPERM(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("process-group EPERM semantics are Unix-specific")
	}
	probe := func(*osexec.Cmd) (bool, error) { return false, nil }

	command, args := immediateExitCommand()
	cmd := osexec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	tree := &transientEPERMTree{}
	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, probe)
	var exitErr *osexec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		t.Fatal(waitErr)
	}
	if !errors.Is(closeErr, errProcessObservationUnavailable) || errors.Is(closeErr, syscall.EPERM) {
		t.Fatalf("closeErr = %v, want unavailable observation without recovered EPERM", closeErr)
	}
	if tree.closed != 2 {
		t.Fatalf("tree closed %d times, want 2 (EPERM then retry)", tree.closed)
	}
}
