//go:build !windows

package pty

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// T09: TestPTYSpawn_ForceKill_OrphanSurvival demonstrates that SIGKILL sent to a
// PTY process does NOT kill its children — they become orphans. This test proves
// the bug exists in the current implementation (before Setpgid fix).
//
// After the fix (T10), child processes should be killed along with the parent
// because they share a process group.
func TestPTYSpawn_ForceKill_OrphanSurvival(t *testing.T) {
	t.Parallel()
	skipIfWindows(t)

	// Spawn a shell that:
	// 1. Starts a background sleep process
	// 2. Prints the background process PID
	// 3. Waits a moment for output to flush
	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: "sh",
		Args:    []string{"-c", "sleep 3600 & echo CHILD_PID=$!; sleep 1"},
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	// Read until we capture the child PID
	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for CHILD_PID, got: %q", output.String())
		default:
		}
		data, readErr := proc.Read()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "CHILD_PID=") {
			break
		}
		if readErr != nil && data == "" {
			t.Fatalf("read error before CHILD_PID: %v, output so far: %q", readErr, output.String())
		}
	}

	// Parse child PID from output
	raw := output.String()
	idx := strings.Index(raw, "CHILD_PID=")
	if idx < 0 {
		t.Fatalf("CHILD_PID not found in output: %q", raw)
	}
	pidStr := strings.TrimSpace(strings.SplitN(raw[idx+len("CHILD_PID="):], "\n", 2)[0])
	// Strip ANSI/control sequences that PTY might include
	pidStr = strings.TrimFunc(pidStr, func(r rune) bool {
		return !('0' <= r && r <= '9')
	})
	childPID, err := strconv.Atoi(pidStr)
	if err != nil || childPID <= 0 {
		t.Fatalf("failed to parse child PID from %q: %v", pidStr, err)
	}
	t.Logf("Background child PID: %d", childPID)

	// Send SIGKILL to parent shell
	if err := proc.Signal("SIGKILL"); err != nil {
		t.Fatalf("Signal(SIGKILL) failed: %v", err)
	}

	// Wait for parent to die
	exitCh := make(chan struct{})
	go func() {
		proc.Wait()
		close(exitCh)
	}()
	select {
	case <-exitCh:
		t.Log("Parent shell exited after SIGKILL")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for parent to exit after SIGKILL")
	}

	// Give kernel a moment to clean up
	time.Sleep(100 * time.Millisecond)

	// Check if child is still alive using signal 0.
	// With Setpgid fix (T10), the child should be killed along with parent.
	err = syscall.Kill(childPID, 0)
	if err == nil {
		// Child is still alive — this is a bug! Clean up and fail.
		_ = syscall.Kill(childPID, syscall.SIGKILL)
		t.Fatalf("ORPHAN BUG: Child PID %d still exists after parent SIGKILL — Setpgid fix not working", childPID)
	}
	t.Logf("✓ Child PID %d was killed along with parent (Setpgid fix working): %v", childPID, err)
}

// T059: Test that SIGSTOP/SIGCONT are parsed correctly on Unix.
func TestParseSignal_UnixExtensions(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"SIGSTOP", "SIGCONT"} {
		sig, err := parseSignal(name)
		if err != nil {
			t.Errorf("parseSignal(%q): unexpected error: %v", name, err)
		}
		if sig == nil {
			t.Errorf("parseSignal(%q): expected signal, got nil", name)
		}
		t.Logf("parseSignal(%q) = %v", name, sig)
	}
}
