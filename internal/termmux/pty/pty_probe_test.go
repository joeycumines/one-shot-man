package pty

import (
	"errors"
	"fmt"
	"testing"
)

// TestClassifyConPTYAttempt pins the decision that separates "this host cannot
// run a console" from "the pty contract is broken".
//
// The probe executes the code under test, so routing every failure through
// t.Skipf made a regression indistinguishable from an unavailable runner: a
// broken Spawn or Wait would skip all ten ConPTY tests and report a green
// package. Only STATUS_DLL_INIT_FAILED (0xC0000142), the documented
// headless-runner console-init limitation, is a reason to skip.
func TestClassifyConPTYAttempt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		attempt     conPTYProbeAttempt
		wantVerdict conPTYProbeVerdict
		wantReason  bool
	}{
		{
			name:        "clean exit",
			attempt:     conPTYProbeAttempt{},
			wantVerdict: conPTYProbeReady,
		},
		{
			// The runner limitation: the child cannot initialise its console.
			name:        "dll init failed",
			attempt:     conPTYProbeAttempt{code: conPTYStatusDLLInitFailed},
			wantVerdict: conPTYProbeUnavailable,
			wantReason:  true,
		},
		{
			name:        "dll init failed with a close failure",
			attempt:     conPTYProbeAttempt{code: conPTYStatusDLLInitFailed, closeErr: errors.New("close conpty")},
			wantVerdict: conPTYProbeUnavailable,
			wantReason:  true,
		},
		{
			// A spawn error means Spawn itself is broken, which no amount of
			// host limitation explains.
			name:        "spawn failed",
			attempt:     conPTYProbeAttempt{spawnErr: errors.New("CreatePseudoConsole: access denied")},
			wantVerdict: conPTYProbeFailure,
			wantReason:  true,
		},
		{
			// Pre-1809 Windows Server has no pseudoconsole API at all. That is
			// host capability, not a regression in this package.
			name:        "conpty api absent",
			attempt:     conPTYProbeAttempt{spawnErr: conPTYErrCallNotImplemented},
			wantVerdict: conPTYProbeUnavailable,
			wantReason:  true,
		},
		{
			name:        "conpty proc not found",
			attempt:     conPTYProbeAttempt{spawnErr: fmt.Errorf("resolve kernel32: %w", conPTYErrProcNotFound)},
			wantVerdict: conPTYProbeUnavailable,
			wantReason:  true,
		},
		{
			name:        "platform reports no pty support",
			attempt:     conPTYProbeAttempt{spawnErr: ErrNotSupported},
			wantVerdict: conPTYProbeUnavailable,
			wantReason:  true,
		},
		{
			// Same for Wait: a deadlock or a handle error is a regression.
			name:        "wait failed",
			attempt:     conPTYProbeAttempt{waitErr: errors.New("WaitForSingleObject: unexpected event")},
			wantVerdict: conPTYProbeFailure,
			wantReason:  true,
		},
		{
			name:        "unexpected exit code",
			attempt:     conPTYProbeAttempt{code: 1},
			wantVerdict: conPTYProbeFailure,
			wantReason:  true,
		},
		{
			// A Close that cannot release the pseudoconsole would leak across
			// every caller in the package, so it is a failure, not a skip.
			name:        "clean exit with a close failure",
			attempt:     conPTYProbeAttempt{closeErr: errors.New("close conpty")},
			wantVerdict: conPTYProbeFailure,
			wantReason:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			verdict, reason := classifyConPTYAttempt(tc.attempt)
			if verdict != tc.wantVerdict {
				t.Fatalf("verdict = %v, want %v (reason %q)", verdict, tc.wantVerdict, reason)
			}
			if tc.wantReason && reason == "" {
				t.Fatal("expected a human-readable reason for a non-ready verdict")
			}
			if !tc.wantReason && reason != "" {
				t.Fatalf("ready verdict carries a reason: %q", reason)
			}
		})
	}
}

// TestConPTYStatusDLLInitFailed pins the constant against the value Windows
// reports so a typo cannot silently turn the one legitimate skip into a hard
// failure on every headless runner.
func TestConPTYStatusDLLInitFailed(t *testing.T) {
	t.Parallel()
	if conPTYStatusDLLInitFailed != 3221225794 {
		t.Fatalf("conPTYStatusDLLInitFailed = %d, want 3221225794 (0xC0000142)", conPTYStatusDLLInitFailed)
	}
}
