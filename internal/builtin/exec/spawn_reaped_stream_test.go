package exec

import (
	"context"
	"errors"
	goruntime "runtime"
	"testing"
)

// TestSpawnChild_ReapedChildStreamIsNotTruncated pins the Windows failure from
// CI run 44067871364 (TestChunk05_ExecuteSplit_ResumesCompletedPrefix on
// windows-latest: `checkout files failed: command output capture was cut
// short before EOF`). On Windows the child runs in a Job Object with
// KILL_ON_JOB_CLOSE; waitAndCloseProcessTree closes that job before cmd.Wait
// drains the pipes. Closing the job can surface as a non-EOF read error on the
// pipe, and pumpPipe's error branch calls closeTruncated, so a child that
// exited cleanly was reported as a truncated capture even though its exit
// status was observed. The contract: once the reaper has recorded an exit,
// the streams must terminate cleanly (done without an error), never truncated.
//
// SCOPE NOTE: this invariant covers the child ITSELF exiting and its pipes
// draining. It does NOT cover an escaped descendant that keeps the pipe open
// past the recorded exit — in that case the capture genuinely can be cut
// after the reaper ran (see TestSettlePumpsAbortStillCutsEscapedWriter and
// TestPumpPipe_ReadFailureAfterRecordedExitStaysTruncated), and the stream is
// correctly reported truncated.
//
// The helper is cross-platform: POSIX uses sh, Windows uses cmd.exe. Each
// iteration spawns a child that writes a line and exits 0, waits for the
// recorded exit, then reads stdout to its terminal chunk and asserts
// err == nil && done && !cutShort, plus wait code 0.
func TestSpawnChild_ReapedChildStreamIsNotTruncated(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	command, args := "sh", []string{"-c", "printf 'ok\\n'; exit 0"}
	if goruntime.GOOS == "windows" {
		command, args = "cmd.exe", []string{"/C", "echo ok & exit /b 0"}
	}

	for i := 0; i < 25; i++ {
		child, err := SpawnChild(context.Background(), SpawnConfig{Command: command, Args: args})
		if err != nil {
			t.Fatalf("iter %d: SpawnChild: %v", i, err)
		}

		code, waitErr := child.WaitContext(context.Background())
		if waitErr != nil || code != 0 {
			t.Fatalf("iter %d: Wait = (%d, %v), want (0, nil)", i, code, waitErr)
		}

		// Read stdout to the terminal chunk after the recorded exit.
		var out string
		for {
			data, done, readErr := child.ReadStdoutContext(context.Background())
			out += data
			if readErr != nil {
				if errors.Is(readErr, errOutputStreamCutShort) {
					t.Fatalf("iter %d: reaped exit-0 child stream reported cut short (out=%q) — the recorded exit must win over a pipe read error", i, out)
				}
				t.Fatalf("iter %d: terminal read error = %v", i, readErr)
			}
			if done {
				break
			}
		}
		if child.stdout.cutShort() || child.stderr.cutShort() {
			t.Fatalf("iter %d: reaped exit-0 child queue marked truncated (out=%q)", i, out)
		}
		if out == "" {
			t.Fatalf("iter %d: expected the child's output, got empty stdout", i)
		}
	}
}

// TestPumpPipe_NonEOFErrorAfterCleanReapIsEOF pins the Windows job-close
// discriminator directly: once recordExit(nil, nil) has cleanly recorded an
// exit status and neither abort nor settlePumps forced a drain cut, a non-EOF
// pipe read error is treated as clean EOF rather than truncation.
func TestPumpPipe_NonEOFErrorAfterCleanReapIsEOF(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{
		stdout: newPipeQueue(),
		stderr: newPipeQueue(),
		done:   make(chan struct{}),
		abort:  make(chan struct{}),
	}
	child.recordExit(nil, nil)
	close(child.done)

	child.pumpPipe(&prefixThenEIOReader{prefix: []byte("ok\n")}, &child.stdout)

	if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "ok\n" {
		t.Fatalf("first stdout read = (%q, %v, %v), want (%q, false, nil)", data, done, err, "ok\n")
	}
	if _, done, err := child.ReadStdoutContext(context.Background()); err != nil || !done {
		t.Fatalf("terminal stdout read = (%v, %v), want (true, nil) after clean reap without abort", done, err)
	}
	if child.stdout.cutShort() {
		t.Fatal("stdout queue unexpectedly marked cut short after clean reap without abort")
	}
}
