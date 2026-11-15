package exec

import (
	"context"
	"testing"
)

// TestWaitContext_ReapedChildPreemptsCancelledContext guards the select tie in
// WaitContext: when the child has already been reaped AND the caller's context
// is already done, a bare two-case select picks a ready case at random and
// reports a successfully exited child as a cancelled wait. Measured at roughly
// half the iterations before the pre-check was added.
//
// This is not cosmetic. The JS wait() binding routes a cancelled wait through
// waitRejection, which rejects, and an unhandled rejection is fatal to the event
// loop (goja-eventloop requestFatalExit), tearing down every other pending
// promise in the process.
//
// The tie-breaking code is platform independent, so the test spawns a
// cross-platform command and runs on every platform.
func TestWaitContext_ReapedChildPreemptsCancelledContext(t *testing.T) {
	skipSlow(t)

	const iters = 50
	for i := 0; i < iters; i++ {
		command, args := immediateExitCommand()
		child, err := SpawnChild(context.Background(), SpawnConfig{
			Command: command,
			Args:    args,
		})
		if err != nil {
			t.Fatalf("iter %d: spawn: %v", i, err)
		}

		// Guarantee the child is reaped and its exit recorded.
		code, waitErr := child.WaitContext(context.Background())
		if code != 0 || waitErr != nil {
			t.Fatalf("iter %d: warm wait = (%d, %v), want (0, nil)", i, code, waitErr)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		code, waitErr = child.WaitContext(ctx)
		if waitErr != nil {
			t.Fatalf("iter %d: a completed exit lost the tie to a done context: (%d, %v)", i, code, waitErr)
		}
		if code != 0 {
			t.Fatalf("iter %d: code = %d, want 0", i, code)
		}
	}
}
