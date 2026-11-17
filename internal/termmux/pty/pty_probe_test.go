package pty

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
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
			attempt:     conPTYProbeAttempt{code: probeExitCode(conPTYStatusDLLInitFailed)},
			wantVerdict: conPTYProbeUnavailable,
			wantReason:  true,
		},
		{
			name:        "dll init failed with a close failure",
			attempt:     conPTYProbeAttempt{code: probeExitCode(conPTYStatusDLLInitFailed), closeErr: errors.New("close conpty")},
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
			// The output tail is the evidence the next failure needs: an exit
			// 1 alone could not distinguish a console-init failure from a
			// broken Spawn/Wait contract.
			name:        "unexpected exit code with output tail",
			attempt:     conPTYProbeAttempt{code: 1, outputTail: "The system cannot find the file specified.\r\n"},
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
			if tc.attempt.outputTail != "" && !strings.Contains(reason, strings.TrimSpace(tc.attempt.outputTail)) {
				t.Fatalf("reason %q must carry the probe output tail, got tail %q", reason, tc.attempt.outputTail)
			}
		})
	}
}

// TestProbeTailRolling pins that probeTail keeps the MOST RECENT bytes, not the
// first ones: a probe child emitting more than the cap must surface the end of
// its output, because that tail is the diagnostic evidence a failure reason
// carries.
func TestProbeTailRolling(t *testing.T) {
	t.Parallel()

	tail := &probeTail{max: 16}
	tail.write([]byte("0123456789"))
	tail.write([]byte("ABCDEFGHIJKLMNOP")) // pushes the first write out entirely
	if got, want := tail.String(), "ABCDEFGHIJKLMNOP"; got != want {
		t.Fatalf("tail = %q, want %q (the most recent 16 bytes)", got, want)
	}
	// A write that only partially overflows keeps the end of the new data and
	// drops the oldest bytes.
	tail2 := &probeTail{max: 16}
	tail2.write([]byte("0123456789")) // 10 bytes
	tail2.write([]byte("ABCDEFGHIJ")) // 10 more, 4 over the cap
	if got, want := tail2.String(), "456789ABCDEFGHIJ"; got != want {
		t.Fatalf("tail = %q, want %q", got, want)
	}
	// A single write larger than the cap keeps its own end.
	tail.write([]byte("0123456789abcdefghij"))
	if got, want := tail.String(), "456789abcdefghij"; got != want {
		t.Fatalf("tail after oversize write = %q, want %q", got, want)
	}
}

// TestTruncateForProbeReason pins the rune-aware tail cut: slicing bytes can
// split a multi-byte UTF-8 sequence, so the truncated reason must stay valid
// UTF-8 and keep the last 512 runes.
func TestTruncateForProbeReason(t *testing.T) {
	t.Parallel()

	short := "short reason"
	if got := truncateForProbeReason(short); got != short {
		t.Fatalf("truncateForProbeReason(%q) = %q, want unchanged", short, got)
	}
	// 600 'a' runes plus a 4-byte rune at the front: byte slicing would cut
	// inside the rune; rune slicing keeps it whole.
	var sb strings.Builder
	sb.WriteString("😀")
	for i := 0; i < 600; i++ {
		sb.WriteByte('a')
	}
	long := sb.String()
	got := truncateForProbeReason(long)
	if len([]rune(got)) != 512 {
		t.Fatalf("truncated reason has %d runes, want 512", len([]rune(got)))
	}
	if !strings.HasSuffix(got, strings.Repeat("a", 512)) {
		t.Fatalf("truncated reason = %q, want it to end with the 512 trailing 'a' runes", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated reason is not valid UTF-8: %q", got)
	}
}

// TestConPTYStatusDLLInitFailed pins the constant against the value Windows
// reports so a typo cannot silently turn the one legitimate skip into a hard
// failure on every headless runner.
func TestConPTYStatusDLLInitFailed(t *testing.T) {
	t.Parallel()
	if uint64(conPTYStatusDLLInitFailed) != 3221225794 {
		t.Fatalf("conPTYStatusDLLInitFailed = %d, want 3221225794 (0xC0000142)", uint64(conPTYStatusDLLInitFailed))
	}
}
