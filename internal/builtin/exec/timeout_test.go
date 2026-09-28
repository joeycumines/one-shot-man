package exec

import (
	"context"
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

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	command, args := hangingCommand()
	child, err := SpawnChild(ctx, SpawnConfig{Command: command, Args: args})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_, _ = child.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SpawnChild did not terminate its process tree at the deadline")
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
