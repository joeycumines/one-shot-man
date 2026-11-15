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
		code int
		err  error
		want bool // true = must reject
	}{
		{name: "no error", code: 0, err: nil, want: false},
		{name: "exit status", code: 7, err: exitStatus, want: false},
		{
			name: "exit status with tree close failure",
			code: 0,
			err:  errors.Join(exitStatus, &processTreeCloseError{err: syscall.EPERM}),
			want: false,
		},
		{
			name: "tree close failure only",
			code: 0,
			err:  &processTreeCloseError{err: syscall.EPERM},
			want: true,
		},
		{name: "opaque failure", code: 0, err: fmt.Errorf("wait failed"), want: true},
		{
			name: "context cancellation",
			code: -1,
			err:  fmt.Errorf("wait: %w", errors.New("context canceled")),
			want: true,
		},
		{
			// A negative code with no signal is unclassified: the reaper sets
			// -1 whenever it cannot extract an exit status, so it must not be
			// presented as one.
			name: "unclassified negative code with exit status in chain",
			code: -1,
			err:  exitStatus,
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := waitRejection(tc.code, tc.err) != nil
			if got != tc.want {
				t.Fatalf("waitRejection(%d, %v) rejected = %v, want %v", tc.code, tc.err, got, tc.want)
			}
		})
	}
}
