package termtest

import (
	"context"
	"testing"
	"time"
)

func TestPTYBasicFunctionality(t *testing.T) {
	ctx := context.Background()

	pty, err := NewForProgram(ctx)
	if err != nil {
		t.Fatalf("failed to create PTY: %v", err)
	}
	defer pty.Close()

	// Test writing to the PTY and reading back
	testMessage := "Hello PTY Test\n"

	// Send input
	if err := pty.SendInput(testMessage); err != nil {
		t.Fatalf("failed to send input: %v", err)
	}

	// Give time for data to flow through
	time.Sleep(100 * time.Millisecond)

	// Check output
	output := pty.GetOutput()
	if output == "" {
		t.Error("no output captured from PTY")
	}

	t.Logf("PTY output: %q", output)
}

func TestTestSession(t *testing.T) {
	ctx := context.Background()

	session, err := NewTestSession(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create test session: %v", err)
	}
	defer session.Close()

	// Test the session API
	session.SendInput("test_input", "hello").
		WaitForOutput("wait_echo", "hello", 1*time.Second).
		SendLine("test_line", "world")

	// Write directly to PTY to simulate a program
	pty := session.GetPTY()
	_, err = pty.GetPTS().WriteString("hello world\n")
	if err != nil {
		t.Fatalf("failed to write to PTS: %v", err)
	}

	// Execute the session
	if err := session.Execute(); err != nil {
		t.Fatalf("session execution failed: %v", err)
	}

	// Check output
	output := session.GetOutput()
	if output == "" {
		t.Error("no output captured from session")
	}

	t.Logf("Session output: %q", output)
}
