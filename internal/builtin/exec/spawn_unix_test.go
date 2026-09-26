package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const spawnSignalHelperMarker = "osm-spawn-signal-helper"

func skipSlow(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow subprocess test in short mode")
	}
}

func hasSpawnSignalHelperMarker(args []string) bool {
	return slices.Contains(args, spawnSignalHelperMarker)
}

func readSignalHelperLine(ctx context.Context, child *ChildProcess) (string, error) {
	type result struct {
		value string
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		var line strings.Builder
		for {
			chunk, done, err := child.ReadStdout()
			if err != nil {
				resultCh <- result{err: err}
				return
			}
			line.WriteString(chunk)
			value := line.String()
			if before, _, ok := strings.Cut(value, "\n"); ok {
				resultCh <- result{value: strings.TrimSpace(before)}
				return
			}
			if done {
				resultCh <- result{err: fmt.Errorf("signal helper closed before acknowledgement")}
				return
			}
		}
	}()

	select {
	case result := <-resultCh:
		return result.value, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestSpawnChild_RepeatedSignalsAreDelivered(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signal delivery test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	child, err := SpawnChild(ctx, SpawnConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestSpawnChild_SignalHelper$", spawnSignalHelperMarker},
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	defer func() {
		_ = child.Kill()
		_, _ = child.Wait()
	}()

	ready, err := readSignalHelperLine(ctx, child)
	if err != nil {
		t.Fatalf("read readiness: %v", err)
	}
	if ready != "ready" {
		t.Fatalf("helper readiness = %q, want %q", ready, "ready")
	}
	if err := child.Signal("SIGTERM"); err != nil {
		t.Fatalf("first SIGTERM: %v", err)
	}
	ack, err := readSignalHelperLine(ctx, child)
	if err != nil {
		t.Fatalf("read acknowledgement: %v", err)
	}
	if ack != "ack-1" {
		t.Fatalf("helper acknowledgement = %q, want %q", ack, "ack-1")
	}
	if err := child.Signal("SIGTERM"); err != nil {
		t.Fatalf("second SIGTERM: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, waitErr := child.Wait()
		done <- waitErr
	}()
	select {
	case waitErr := <-done:
		if waitErr != nil {
			t.Fatalf("Wait after repeated signals: %v", waitErr)
		}
	case <-ctx.Done():
		t.Fatalf("child did not exit after receiving repeated SIGTERM: %v", ctx.Err())
	}
}

func TestSpawnChild_SignalHelperIgnoresAmbientEnvironment(t *testing.T) {
	skipSlow(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := osexec.CommandContext(ctx, os.Args[0], "-test.run=^TestSpawnChild_SignalHelper$")
	command.Env = append(os.Environ(), "OSM_SPAWN_SIGNAL_HELPER=1")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("ambient helper subprocess did not finish: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("ambient helper subprocess: %v; output: %s", err, output)
	}
	if strings.Contains(string(output), "ready") {
		t.Fatalf("ambient environment activated signal helper: %s", output)
	}
}

func TestSpawnChild_SignalHelper(t *testing.T) {
	if !hasSpawnSignalHelperMarker(os.Args) {
		return
	}
	if testing.Short() {
		t.Skip("skipping slow signal helper in short mode")
	}
	term := make(chan os.Signal, 2)
	signal.Notify(term, syscall.SIGTERM)
	defer signal.Stop(term)
	fmt.Fprintln(os.Stdout, "ready")
	<-term
	fmt.Fprintln(os.Stdout, "ack-1")
	<-term
}

func TestSpawnChild_ConcurrentSignalsAfterExitAreNoOps(t *testing.T) {
	command := "sh"
	args := []string{"-c", "exit 0"}
	if runtime.GOOS == "windows" {
		command = "cmd"
		args = []string{"/C", "exit", "0"}
	}
	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command: command,
		Args:    args,
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	if _, err := child.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	const signalCount = 200
	errorsCh := make(chan error, signalCount)
	var wg sync.WaitGroup
	wg.Add(signalCount)
	for range signalCount {
		go func() {
			defer wg.Done()
			errorsCh <- child.Signal("SIGTERM")
		}()
	}
	wg.Wait()
	close(errorsCh)
	for signalErr := range errorsCh {
		if signalErr != nil {
			t.Fatalf("Signal after exit = %v, want nil", signalErr)
		}
	}
}

func TestSpawnChild_CloseKillsDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group descendant probe")
	}
	if testing.Short() {
		t.Skip("skipping process-tree cleanup test in short mode")
	}

	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", "sleep 2 & echo $!"},
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	var output strings.Builder
	for {
		chunk, done, readErr := child.ReadStdout()
		if readErr != nil {
			t.Fatalf("ReadStdout: %v", readErr)
		}
		output.WriteString(chunk)
		if done {
			break
		}
	}
	pidText := strings.TrimSpace(output.String())
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatalf("descendant pid = %q: %v", pidText, err)
	}

	waitDone := make(chan error, 1)
	go func() {
		_, waitErr := child.Wait()
		waitDone <- waitErr
	}()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("Wait: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("child did not finish after direct process exit")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		proc, findErr := os.FindProcess(pid)
		if findErr != nil {
			return
		}
		err := proc.Signal(syscall.Signal(0))
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatalf("checking descendant pid: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant pid %d remained alive after Wait", pid)
}
