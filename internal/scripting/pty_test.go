package scripting_test

import (
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestPTYBasic is a minimal test to verify PTY functionality
func TestPTYBasic(t *testing.T) {
	t.Log("Starting basic PTY test...")

	cmd := exec.Command("echo", "hello")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("Failed to start command with PTY: %v", err)
	}
	defer ptmx.Close()

	t.Log("PTY started, reading output...")

	buf := make([]byte, 100)
	done := make(chan bool)

	go func() {
		n, _ := ptmx.Read(buf)
		t.Logf("Read %d bytes: %s", n, string(buf[:n]))
		done <- true
	}()

	select {
	case <-done:
		t.Log("Test completed successfully")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout reading from PTY")
	}

	cmd.Wait()
}
