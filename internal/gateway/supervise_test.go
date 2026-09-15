package gateway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stubScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("writing the stub: %v", err)
	}
	return path
}

func TestSupervisePropagatesTheExitCode(t *testing.T) {
	dir := t.TempDir()
	stub := stubScript(t, dir, "exit-seven", "exit 7\n")

	var stdout strings.Builder
	code, err := Supervise(context.Background(), SuperviseOptions{
		Argv:   []string{stub},
		Stdout: &stdout,
	})
	if err != nil {
		t.Fatalf("Supervise: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code: got %d, want the child's 7", code)
	}

	success := stubScript(t, dir, "exit-zero", "exit 0\n")
	code, err = Supervise(context.Background(), SuperviseOptions{Argv: []string{success}})
	if err != nil {
		t.Fatalf("Supervise: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}
}

func TestSuperviseTerminatesAndReapsOnCancellation(t *testing.T) {
	dir := t.TempDir()
	stub := stubScript(t, dir, "linger", "trap 'exit 0' INT; sleep 30 &\nwait\n")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var code int
	var err error
	start := time.Now()
	go func() {
		code, err = Supervise(ctx, SuperviseOptions{Argv: []string{stub}, GracePeriod: 2 * time.Second})
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Supervise did not return after cancellation, so the child was not reaped")
	}
	if err != nil {
		t.Fatalf("Supervise: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 12*time.Second {
		t.Fatalf("cancellation took %s, want it honoured within the grace period", elapsed)
	}
	if code != 0 && code < 128 {
		t.Fatalf("exit code after cancellation: got %d, want the child's status or a signal convention", code)
	}
}

func TestSuperviseCarriesTheComposedEnvironmentOnly(t *testing.T) {
	dir := t.TempDir()
	environmentPath := filepath.Join(dir, "env.txt")
	stub := stubScript(t, dir, "echo-env", "env > "+environmentPath+"\nexit 0\n")
	t.Setenv("SHOULD_NOT_APPEAR", "outer-value")

	code, err := Supervise(context.Background(), SuperviseOptions{
		Argv:        []string{stub},
		Environment: []string{"COMPOSED_VARIABLE=composed-value"},
	})
	if err != nil {
		t.Fatalf("Supervise: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}

	environment, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatalf("reading the child environment: %v", err)
	}
	if !strings.Contains(string(environment), "COMPOSED_VARIABLE=composed-value") {
		t.Fatalf("the composed variable is missing: %s", environment)
	}
	if strings.Contains(string(environment), "SHOULD_NOT_APPEAR") {
		t.Fatalf("the child inherited a variable the launcher did not compose: %s", environment)
	}
}

func TestSuperviseRejectsAnEmptyCommand(t *testing.T) {
	if _, err := Supervise(context.Background(), SuperviseOptions{}); err == nil {
		t.Fatal("Supervise: want an error for an empty command")
	}
}
