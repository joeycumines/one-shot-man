package exec

import (
	"context"
	"fmt"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

func skipIfWindows(t *testing.T) {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("spawn tests use POSIX shell")
	}
}

// shSpawn is a helper that spawns "sh -c <script>" and returns the child.
func shSpawn(t *testing.T, ctx context.Context, script string) *ChildProcess {
	t.Helper()
	child, err := SpawnChild(ctx, SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", script},
	})
	if err != nil {
		t.Fatalf("SpawnChild failed: %v", err)
	}
	return child
}

func TestSpawnChild_BasicStdout(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `echo hello`)

	var all strings.Builder
	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout error: %v", err)
		}
		if done {
			break
		}
		all.WriteString(data)
	}
	got := strings.TrimSpace(all.String())
	if got != "hello" {
		t.Errorf("stdout = %q, want %q", got, "hello")
	}

	code, err := child.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if err != nil {
		t.Errorf("exitErr = %v, want nil", err)
	}
}

func TestSpawnChild_BasicStderr(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `echo error >&2`)

	var all strings.Builder
	for {
		data, done, err := child.ReadStderr()
		if err != nil {
			t.Fatalf("ReadStderr error: %v", err)
		}
		if done {
			break
		}
		all.WriteString(data)
	}
	got := strings.TrimSpace(all.String())
	if got != "error" {
		t.Errorf("stderr = %q, want %q", got, "error")
	}

	code, _ := child.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestSpawnChild_ExitCode(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `exit 42`)

	code, _ := child.Wait()
	if code != 42 {
		t.Errorf("exit code = %d, want 42", code)
	}
}

func TestSpawnChild_Stdin(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `cat`)

	if err := child.WriteStdin("ping\n"); err != nil {
		t.Fatalf("WriteStdin: %v", err)
	}
	if err := child.CloseStdin(); err != nil {
		t.Fatalf("CloseStdin: %v", err)
	}

	var all strings.Builder
	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout error: %v", err)
		}
		if done {
			break
		}
		all.WriteString(data)
	}
	got := strings.TrimSpace(all.String())
	if got != "ping" {
		t.Errorf("stdout = %q, want %q", got, "ping")
	}

	code, _ := child.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestSpawnChild_MixedStdoutStderr(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `echo out1; echo err1 >&2; echo out2; echo err2 >&2`)

	var stdout, stderr strings.Builder

	// Drain both in separate goroutines.
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		for {
			data, done, err := child.ReadStderr()
			if err != nil {
				return
			}
			if done {
				return
			}
			stderr.WriteString(data)
		}
	}()

	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout error: %v", err)
		}
		if done {
			break
		}
		stdout.WriteString(data)
	}

	<-doneCh

	outLines := strings.TrimSpace(stdout.String())
	errLines := strings.TrimSpace(stderr.String())

	if !strings.Contains(outLines, "out1") || !strings.Contains(outLines, "out2") {
		t.Errorf("stdout = %q, want to contain out1 and out2", outLines)
	}
	if !strings.Contains(errLines, "err1") || !strings.Contains(errLines, "err2") {
		t.Errorf("stderr = %q, want to contain err1 and err2", errLines)
	}
}

func TestSpawnChild_LargeOutput(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Generate 10,000 lines.
	child := shSpawn(t, context.Background(), `seq 1 10000`)

	var lineCount int
	var buf strings.Builder
	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout error: %v", err)
		}
		if done {
			break
		}
		buf.WriteString(data)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	lineCount = len(lines)
	if lineCount != 10000 {
		t.Errorf("got %d lines, want 10000", lineCount)
	}
	if lines[0] != "1" {
		t.Errorf("first line = %q, want %q", lines[0], "1")
	}
	if lines[lineCount-1] != "10000" {
		t.Errorf("last line = %q, want %q", lines[lineCount-1], "10000")
	}

	code, _ := child.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestSpawnChild_Kill(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `sleep 300`)

	if err := child.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	code, _ := child.Wait()
	if code == 0 {
		t.Errorf("expected non-zero exit code after kill, got 0")
	}
}

func TestSpawnChild_KillIdempotent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `sleep 300`)

	if err := child.Kill(); err != nil {
		t.Fatalf("Kill 1: %v", err)
	}
	// Second kill should be no-op.
	if err := child.Kill(); err != nil {
		t.Fatalf("Kill 2: %v", err)
	}

	child.Wait()
}

func TestSpawnChild_ContextCancel(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	ctx, cancel := context.WithCancel(context.Background())
	child := shSpawn(t, ctx, `sleep 300`)

	cancel()

	// Should exit due to context cancellation.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	done := make(chan struct{})
	go func() {
		child.Wait()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-timer.C:
		t.Fatal("timed out waiting for process to exit after context cancel")
	}
}

func TestSpawnChild_Cwd(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	dir := t.TempDir()
	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", "pwd"},
		Cwd:     dir,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	var buf strings.Builder
	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout: %v", err)
		}
		if done {
			break
		}
		buf.WriteString(data)
	}

	got := strings.TrimSpace(buf.String())
	// On macOS, /tmp may resolve to /private/tmp.
	if !strings.HasSuffix(got, strings.TrimPrefix(dir, "/private")) && !strings.HasSuffix(dir, strings.TrimPrefix(got, "/private")) && got != dir {
		t.Errorf("cwd = %q, want %q", got, dir)
	}
}

func TestSpawnChild_Env(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", `echo "$OSM_TEST_SPAWN_VAR"`},
		Env:     map[string]string{"OSM_TEST_SPAWN_VAR": "spawn_test_42"},
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	var buf strings.Builder
	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout: %v", err)
		}
		if done {
			break
		}
		buf.WriteString(data)
	}

	got := strings.TrimSpace(buf.String())
	if got != "spawn_test_42" {
		t.Errorf("env var = %q, want %q", got, "spawn_test_42")
	}
}

func TestSpawnChild_Pid(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `echo $$`)

	pid := child.Pid()
	if pid <= 0 {
		t.Errorf("pid = %d, want > 0", pid)
	}

	child.Wait()
}

func TestSpawnChild_ConcurrentSpawns(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	const n = 5
	type result struct {
		output string
		err    error
	}
	results := make(chan result, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			child, err := SpawnChild(context.Background(), SpawnConfig{
				Command: "sh",
				Args:    []string{"-c", fmt.Sprintf(`echo child-%d`, idx)},
			})
			if err != nil {
				results <- result{err: fmt.Errorf("spawn %d: %w", idx, err)}
				return
			}

			var buf strings.Builder
			for {
				data, done, readErr := child.ReadStdout()
				if readErr != nil {
					results <- result{err: fmt.Errorf("read %d: %w", idx, readErr)}
					return
				}
				if done {
					break
				}
				buf.WriteString(data)
			}

			child.Wait()
			results <- result{output: strings.TrimSpace(buf.String())}
		}(i)
	}

	seen := make(map[string]bool)
	for i := 0; i < n; i++ {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		seen[r.output] = true
	}

	for i := 0; i < n; i++ {
		expected := fmt.Sprintf("child-%d", i)
		if !seen[expected] {
			t.Errorf("missing output from %s", expected)
		}
	}
}

func TestSpawnChild_WaitIdempotent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `exit 7`)

	code1, _ := child.Wait()
	code2, _ := child.Wait() // should return immediately, same result

	if code1 != 7 || code2 != 7 {
		t.Errorf("Wait() codes = (%d, %d), want (7, 7)", code1, code2)
	}
}

func TestSpawnChild_CloseStdinIdempotent(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `cat`)

	if err := child.CloseStdin(); err != nil {
		t.Fatalf("CloseStdin 1: %v", err)
	}
	// Second close should be no-op (nil pipe).
	if err := child.CloseStdin(); err != nil {
		t.Errorf("CloseStdin 2: %v", err)
	}

	child.Wait()
}

func TestSpawnChild_WriteAfterClose(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	child := shSpawn(t, context.Background(), `cat`)

	if err := child.CloseStdin(); err != nil {
		t.Fatalf("CloseStdin: %v", err)
	}

	err := child.WriteStdin("should fail")
	if err == nil {
		t.Error("WriteStdin after close should return error")
	}

	child.Wait()
}

func TestSpawnChild_InvalidCommand(t *testing.T) {
	t.Parallel()

	_, err := SpawnChild(context.Background(), SpawnConfig{
		Command: "/nonexistent/binary/path/z3nrk82",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestSpawnChild_EmptyStdout(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Process that writes nothing to stdout.
	child := shSpawn(t, context.Background(), `true`)

	data, done, err := child.ReadStdout()
	if err != nil {
		t.Fatalf("ReadStdout error: %v", err)
	}
	if !done {
		t.Errorf("expected done=true for empty stdout, got data=%q", data)
	}

	code, _ := child.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestSpawnChild_BinaryData(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Send and receive binary data (bytes 0-255).
	child := shSpawn(t, context.Background(), `cat`)

	// Write all 256 byte values.
	var input [256]byte
	for i := range input {
		input[i] = byte(i)
	}
	if err := child.WriteStdin(string(input[:])); err != nil {
		t.Fatalf("WriteStdin: %v", err)
	}
	if err := child.CloseStdin(); err != nil {
		t.Fatalf("CloseStdin: %v", err)
	}

	var buf strings.Builder
	for {
		data, done, err := child.ReadStdout()
		if err != nil {
			t.Fatalf("ReadStdout: %v", err)
		}
		if done {
			break
		}
		buf.WriteString(data)
	}

	got := buf.String()
	if len(got) != 256 {
		t.Fatalf("output length = %d, want 256", len(got))
	}
	for i := 0; i < 256; i++ {
		if got[i] != byte(i) {
			t.Errorf("byte[%d] = %d, want %d", i, got[i], i)
			break
		}
	}
}
