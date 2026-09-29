package userk8s

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

func TestExecRunnerRedactsStderrAndHonorsTimeout(t *testing.T) {
	ctx := context.Background()

	t.Run("stderr is counted, never quoted", func(t *testing.T) {
		const secret = "leaked-secret-value"
		t.Setenv(execRunnerProcessModeEnv, "stderr")
		t.Setenv(execRunnerProcessSecretEnv, secret)
		_, err := (ExecRunner{}).Run(ctx, []string{os.Args[0], "-test.run=^TestExecRunnerProcessHelper$"}, time.Second)
		if err == nil {
			t.Fatal("Run: want an error for a failing command")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked stderr content: %v", err)
		}
	})

	t.Run("timeout is enforced and reported", func(t *testing.T) {
		t.Setenv(execRunnerProcessModeEnv, "sleep")
		start := time.Now()
		_, err := (ExecRunner{}).Run(ctx, []string{os.Args[0], "-test.run=^TestExecRunnerProcessHelper$"}, 50*time.Millisecond)
		if err == nil {
			t.Fatal("Run: want a timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("error: got %v, want a timeout message", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error: got %v, want context.DeadlineExceeded", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("elapsed %s: the timeout was not enforced", elapsed)
		}
	})
}

func TestCanceledContextIsReportedAsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	objects := documents(t,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`,
		`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-direct
spec:
  provider: alpha
  auth:
    scheme: bearer
    requiredEnv:
      - ALPHA_API_KEY
`)
	if _, err := mustBackend(t, objects, &fakeRunner{}).ResolveCredential(ctx, "alpha-direct"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveCredential: got %v, want context.Canceled", err)
	}

	t.Setenv(execRunnerProcessModeEnv, "sleep")
	_, err := (ExecRunner{}).Run(ctx, []string{os.Args[0], "-test.run=^TestExecRunnerProcessHelper$"}, time.Minute)
	if err == nil {
		t.Fatal("ExecRunner.Run: want an error for a canceled context")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("ExecRunner.Run: got %v, want a cancellation message rather than a timeout", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecRunner.Run: got %v, want context.Canceled", err)
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("ExecRunner.Run: got %v, a canceled command must not be reported as a timeout", err)
	}
}

func TestExecRunnerCleansInheritedOutputPipesAfterParentExit(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess pipe-lifecycle test")
	}

	pidPath := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv(execRunnerProcessModeEnv, "parent")
	t.Setenv(execRunnerProcessPIDEnv, pidPath)
	t.Cleanup(func() {
		pid, err := resolverPipeHolderPID(pidPath)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			t.Errorf("read child PID: %v", err)
			return
		}
		alive, err := resolverProcessAlive(pid)
		if err != nil {
			t.Errorf("check child process %d: %v", pid, err)
			return
		}
		if !alive {
			return
		}
		child, err := os.FindProcess(pid)
		if err != nil {
			t.Errorf("find child process %d: %v", pid, err)
			return
		}
		if err := child.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("kill child process %d: %v", pid, err)
		}
	})

	start := time.Now()
	_, err := (ExecRunner{}).Run(context.Background(), []string{os.Args[0], "-test.run=^TestExecRunnerProcessHelper$"}, 10*time.Second)
	if err != nil {
		t.Fatalf("ExecRunner.Run: got %v, want successful parent completion", err)
	}
	if elapsed := time.Since(start); elapsed > 12*time.Second {
		t.Fatalf("elapsed %s: inherited output pipe was not bounded", elapsed)
	}

	pid, err := resolverPipeHolderPID(pidPath)
	if err != nil {
		t.Fatalf("read child PID: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		alive, err := resolverProcessAlive(pid)
		if err != nil {
			t.Fatalf("check child process %d: %v", pid, err)
		}
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived resolver completion", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestResolveBindingClassifiesRunnerDeadline(t *testing.T) {
	binding := v1alpha1.LocalSecretBinding{
		Spec: v1alpha1.LocalSecretBindingSpec{
			EnvVar: "API_KEY",
			Resolvers: []v1alpha1.LocalSecretResolver{{
				Command: &v1alpha1.CommandResolver{Argv: []string{"resolver"}},
			}},
		},
	}
	runner := &fakeRunner{steps: []fakeStep{{
		argv: []string{"resolver"},
		err:  context.DeadlineExceeded,
	}}}
	_, ok, failures := resolveBinding(context.Background(), binding, runner)
	if ok {
		t.Fatal("resolveBinding: deadline failure unexpectedly returned a credential")
	}
	if len(failures) != 1 || failures[0] != "command resolver: timed out" {
		t.Fatalf("failures = %v, want [command resolver: timed out]", failures)
	}
}

func resolverPipeHolderPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

const (
	execRunnerProcessModeEnv   = "OSM_TEST_EXEC_RUNNER_PROCESS_MODE"
	execRunnerProcessPIDEnv    = "OSM_TEST_EXEC_RUNNER_PROCESS_PID"
	execRunnerProcessSecretEnv = "OSM_TEST_EXEC_RUNNER_PROCESS_SECRET"
)

func TestExecRunnerProcessHelper(t *testing.T) {
	switch os.Getenv(execRunnerProcessModeEnv) {
	case "stderr":
		if _, err := os.Stderr.Write([]byte(os.Getenv(execRunnerProcessSecretEnv) + "\n")); err != nil {
			os.Exit(2)
		}
		os.Exit(3)
	case "sleep":
		time.Sleep(30 * time.Second)
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestExecRunnerProcessHelper$")
		child.Env = []string{execRunnerProcessModeEnv + "=sleep"}
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			t.Fatalf("start pipe-holding child: %v", err)
		}
		if err := os.WriteFile(os.Getenv(execRunnerProcessPIDEnv), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			_ = child.Process.Kill()
			t.Fatalf("write pipe-holding child PID: %v", err)
		}
		if _, err := os.Stdout.Write([]byte("resolver output\n")); err != nil {
			t.Fatalf("write resolver output: %v", err)
		}
	}
}

func TestResolverOutputIsBounded(t *testing.T) {
	buffer := &limitedBuffer{limit: 16}
	written, err := buffer.Write([]byte(strings.Repeat("x", 1024)))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written != 1024 {
		t.Fatalf("Write: got %d, want the full length so the child never sees a short write", written)
	}
	if got := len(buffer.String()); got != 16 {
		t.Fatalf("buffer length: got %d, want the 16-byte limit", got)
	}
	if !buffer.truncated {
		t.Fatal("buffer: want truncation recorded")
	}

	counter := &countingWriter{limit: 8}
	if _, err := counter.Write([]byte(strings.Repeat("y", 4096))); err != nil {
		t.Fatalf("countingWriter.Write: %v", err)
	}
	if counter.Count() != 8 {
		t.Fatalf("countingWriter.Count: got %d, want the 8-byte limit", counter.Count())
	}
}

func TestReadCredentialFileUsesTheFirstLineOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("first\nsecond\nthird\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	value, err := readCredentialFile(path)
	if err != nil {
		t.Fatalf("readCredentialFile: %v", err)
	}
	if value != "first" {
		t.Fatalf("value: got %q, want the first line", value)
	}
}
