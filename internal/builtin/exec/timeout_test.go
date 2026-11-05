package exec

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func hangingCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", "ping -n 6 127.0.0.1 > nul"}
	}
	return "sh", []string{"-c", "sleep 5"}
}

func TestRunExec_ContextDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process timeout test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	command, args := hangingCommand()
	start := time.Now()
	result := runExec(ctx, command, args...)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("runExec exceeded deadline: %s", elapsed)
	}
	if code, ok := result["code"].(int); !ok || code == 0 {
		t.Fatalf("timed command returned successful result: %#v", result)
	}
}

func TestSpawnChild_ContextDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process timeout test in short mode")
	}

	// The deadline must be generous enough that process startup itself
	// (process creation, job assignment, pipe setup) is not racing the clock:
	// on a loaded CI runner, CreateProcess can take longer than 75ms, and
	// os/exec's Start then returns the raw ctx error, which used to fail this
	// test with "SpawnChild: context deadline exceeded". The contract under
	// test is that the deadline terminates the process TREE promptly once the
	// child is running, so the budget covers startup and the Wait bound below
	// covers the kill.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command, args := hangingCommand()
	child, err := SpawnChild(ctx, SpawnConfig{Command: command, Args: args})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	// Cut the deadline well below the process's own runtime (5s) so the
	// tree kill — not the command finishing on its own — is what ends Wait.
	deadline := time.AfterFunc(250*time.Millisecond, cancel)
	defer deadline.Stop()

	done := make(chan struct{})
	go func() {
		_, _ = child.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SpawnChild did not terminate its process tree at the deadline")
	}
}

// TestSpawnChild_AlreadyExpiredContext pins the explicit error contract:
// SpawnChild with an already-cancelled context reports a *SpawnContextError
// wrapping the context error (no process is started), so callers can tell a
// pre-expired deadline apart from a genuine start failure. This is the exact
// signature the windows CI runner produced when a 75ms deadline expired
// during CreateProcess on a loaded machine — the raw "context deadline
// exceeded" from os/exec.Start read like a SpawnChild bug.
func TestSpawnChild_AlreadyExpiredContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command, args := hangingCommand()
	child, err := SpawnChild(ctx, SpawnConfig{Command: command, Args: args})
	if err == nil {
		_ = child.Kill()
		_, _ = child.Wait()
		t.Fatal("SpawnChild with an expired context succeeded, want an error")
	}
	var spawnCtxErr *SpawnContextError
	if !errors.As(err, &spawnCtxErr) {
		t.Fatalf("SpawnChild error = %v (%T), want a *SpawnContextError", err, err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SpawnChild error = %v, want it to wrap context.Canceled", err)
	}
}

func TestRunExec_DoesNotWaitForDescendantPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX shell background semantics")
	}
	if testing.Short() {
		t.Skip("skipping process-tree pipe test in short mode")
	}

	start := time.Now()
	result := runExec(context.Background(), "sh", "-c", "(sleep 2) & echo parent-done")
	elapsed := time.Since(start)
	if elapsed > time.Second {
		t.Fatalf("runExec waited for descendant-held pipes: %s", elapsed)
	}
	if stdout, _ := result["stdout"].(string); !strings.Contains(stdout, "parent-done") {
		t.Fatalf("stdout = %q, want parent-done (result: %#v)", stdout, result)
	}
	if result["error"] == true {
		t.Fatalf("runExec reported an error after tree cleanup: %#v", result)
	}
}

func TestTimeoutFromMilliseconds(t *testing.T) {
	t.Parallel()

	if got := timeoutFromMilliseconds(0); got != 0 {
		t.Fatalf("zero timeout = %s, want zero", got)
	}
	if got := timeoutFromMilliseconds(-1); got != 0 {
		t.Fatalf("negative timeout = %s, want zero", got)
	}
	if got := timeoutFromMilliseconds(125); got != 125*time.Millisecond {
		t.Fatalf("timeout = %s, want 125ms", got)
	}
}

// TestRunExec_TimeoutKeepsInGroupWritersOutput pins the abort grace on the
// execv timeout path: when ctx kills the command, the process-group close has
// already reaped every in-group writer, so the readers' natural EOF — and the
// output the command had already written — must be collected within the
// abort grace instead of the capture being force-closed at 0ms and reported
// as a cut stream. The command itself was killed, so the result is still an
// error (non-zero status), but stdout/stderr must be COMPLETE, not a prefix
// with the capture-aborted diagnostic.
func TestRunExec_TimeoutKeepsInGroupWritersOutput(t *testing.T) {
	skipSlow(t)

	const payload = "timeout-path-complete-output\n"
	// The reader reaches the pipe only after the ctx deadline (120ms > 100ms
	// grace): with the group kill done, this is pure scheduling delay past the
	// abort, and only a second grace AFTER force-closing can still deliver it.
	// A no-grace abort closes the reader at the deadline and loses the payload.
	stdoutReader := newDelayedCommandPipe(t, payload, 120*time.Millisecond)
	pipeNo := 0
	newPipe := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		pipeNo++
		if pipeNo == 1 {
			return stdoutReader, &failingCommandPipe{}, nil
		}
		return &failingCommandPipe{}, &failingCommandPipe{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	command, args := hangingCommand()
	start := time.Now()
	result := runExecCommand(ctx, command, args, newPipe, outputEOFCeiling)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("runExecCommand took %s after the deadline — the abort grace did not bound the wait", elapsed)
	}
	if code, ok := result["code"].(int); !ok || code == 0 {
		t.Fatalf("timed-out command result code = %v, want non-zero (result: %#v)", result["code"], result)
	}
	if stdout, _ := result["stdout"].(string); stdout != payload {
		t.Fatalf("runExec stdout = %q, want the complete %q — the timeout must not cut in-group output (result: %#v)", stdout, payload, result)
	}
	message, _ := result["message"].(string)
	// The command was killed, so the message carries the kill; the capture
	// itself must NOT add the aborted diagnostic — the payload was delivered
	// within the post-close grace even though the reader arrived after the
	// deadline.
	if strings.Contains(message, "output capture aborted") {
		t.Fatalf("runExec message = %q — the capture was aborted although the only writer died with the group and its output arrived within the grace", message)
	}
	if !strings.Contains(message, "signal: killed") && !strings.Contains(message, "deadline exceeded") && !strings.Contains(message, "TerminateJobObject") && !strings.Contains(message, "exit status") {
		t.Fatalf("runExec message = %q, want the kill/deadline cause for the timed-out command", message)
	}
}

// TestSpawn_TimeoutDeliversBufferedOutputClean pins the same grace on the
// spawn path: a child that writes output and then hangs is killed by timeoutMs;
// because the group kill reaps the only writer, the pumps reach natural EOF
// within the abort grace, so the stream ends CLEAN (done, not truncated) with
// the full output — the timeout is reported by elapsedMs/wait, not by a cut
// stream.
func TestSpawn_TimeoutDeliversBufferedOutputClean(t *testing.T) {
	skipIfWindows(t)
	skipSlow(t)
	_, runJS := asyncTestEnv(t)

	value, err := runJS(`
		const child = await exec.spawn("sh", ["-c", "printf 'all-written\\n'; sleep 30"], {timeoutMs: 400});
		let out = "";
		let truncated = false;
		while (true) {
			const chunk = await child.stdout.read();
			if (chunk.done) { truncated = !!chunk.truncated; break; }
			out += chunk.value;
		}
		const waitResult = await child.wait();
		__collect(JSON.stringify({out, truncated, code: waitResult.code}));
	`)
	if err != nil {
		t.Fatalf("runJS: %v", err)
	}
	got := value.String()
	if !strings.Contains(got, `"out":"all-written\n"`) {
		t.Fatalf("stdout = %s, want the complete pre-timeout output", got)
	}
	if strings.Contains(got, `"truncated":true`) {
		t.Fatalf("stream was cut by the timeout kill although the only writer died with the group: %s", got)
	}
}
