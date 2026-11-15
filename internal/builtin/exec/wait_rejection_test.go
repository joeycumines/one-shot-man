package exec

import (
	"errors"
	"fmt"
	osexec "os/exec"
	"syscall"
	"testing"
)

// TestWaitRejection guards the decision that a real exit status is delivered
// even when cleaning up the process tree also failed.
//
// wait() used to reject whenever its error chain carried a
// processTreeCloseError, even alongside a valid exit status. A timeout-killed
// child whose process group had already gone away hit exactly that: wait()
// rejected with "signal: killed" although the child demonstrably exited, JS
// saw an unhandled rejection, and the adapter's fatal-exit path shut the event
// loop down, stranding every other pending promise in the process.
func TestWaitRejection(t *testing.T) {
	exitStatus := &osexec.ExitError{ProcessState: nil}

	for _, tc := range []struct {
		name string
		err  error
		want bool // true = must reject
	}{
		{name: "no error", err: nil, want: false},
		{name: "exit status", err: exitStatus, want: false},
		{
			name: "exit status with tree close failure",
			err:  errors.Join(exitStatus, &processTreeCloseError{err: syscall.EPERM}),
			want: false,
		},
		{
			name: "tree close failure only",
			err:  &processTreeCloseError{err: syscall.EPERM},
			want: true,
		},
		{name: "opaque failure", err: fmt.Errorf("wait failed"), want: true},
		{
			name: "context cancellation",
			err:  fmt.Errorf("wait: %w", errors.New("context canceled")),
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := waitRejection(tc.err) != nil
			if got != tc.want {
				t.Fatalf("waitRejection(%v) rejected = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
