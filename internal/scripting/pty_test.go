package scripting_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
)

// TestPTYBasic is a minimal test to verify PTY functionality
func TestPTYBasic(t *testing.T) {
	t.Log("Starting basic PTY test...")

	// Use a helper test process (the same test binary) to avoid relying on
	// external tools being available in PATH.
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperEchoProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
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

// TestHelperEchoProcess is used as a helper subprocess for PTY tests. It
// prints a single line to stdout and exits immediately. It only runs when
// the GO_WANT_HELPER_PROCESS environment variable is set to avoid executing
// during normal test runs.
func TestHelperEchoProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	println("hello")
	os.Exit(0)
}
