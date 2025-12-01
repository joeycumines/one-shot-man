//go:build unix

package termtest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPTY creates a PTYTest instance for the helper process.
func newTestPTY(t *testing.T, command string, args ...string) *PTYTest {
	t.Helper()

	ctx := context.Background()
	// Re-exec the current test binary to run the helper process
	cmdPath := os.Args[0]
	cmdArgs := append([]string{"-test.run=^TestMain$", "--", command}, args...)

	p, err := New(ctx, cmdPath, cmdArgs...)
	require.NoError(t, err)

	// The helper process is triggered by this environment variable
	p.SetEnv([]string{"GO_TEST_MODE=helper"})

	return p
}

func TestPTY_New_Start_WithOptions(t *testing.T) {
	t.Run("should start process successfully", func(t *testing.T) {
		p := newTestPTY(t, "echo", "hello")
		defer p.Close()

		startLen := p.OutputLen()
		err := p.Start()
		require.NoError(t, err)
		err = p.WaitForOutputSince("hello", startLen, 2*time.Second)
		assert.NoError(t, err)
	})

	t.Run("should fail to start non-existent command", func(t *testing.T) {
		ctx := context.Background()
		p, err := New(ctx, "/non/existent/command")
		require.NoError(t, err)
		defer p.Close()
		err = p.Start()
		assert.Error(t, err)
	})

	t.Run("should apply custom working directory", func(t *testing.T) {
		tempDir := t.TempDir()
		p := newTestPTY(t, "pwd")
		defer p.Close()

		p.SetDir(tempDir)
		startLen := p.OutputLen()
		err := p.Start()
		require.NoError(t, err)

		err = p.WaitForOutputSince(tempDir, startLen, 2*time.Second)
		assert.NoError(t, err, "Expected output to contain the custom directory")
	})

	t.Run("should apply custom environment variables", func(t *testing.T) {
		p := newTestPTY(t, "env", "MY_VAR")
		defer p.Close()

		p.SetEnv([]string{"MY_VAR=my_value"})
		startLen := p.OutputLen()
		err := p.Start()
		require.NoError(t, err)

		err = p.WaitForOutputSince("my_value", startLen, 2*time.Second)
		assert.NoError(t, err, "Expected output to contain the custom env var value")
	})
}

func TestPTY_NewForProgram(t *testing.T) {
	p, err := NewForProgram(context.Background())
	require.NoError(t, err)
	defer p.Close()

	require.NotNil(t, p.GetPTM())
	require.NotNil(t, p.GetPTS())

	// Write to master, which should be captured in the output buffer.
	startLen := p.OutputLen()
	_, err = p.GetPTM().WriteString("hello program")
	require.NoError(t, err)

	err = p.WaitForOutputSince("hello program", startLen, 1*time.Second)
	assert.NoError(t, err)
}

func TestPTY_InputMethods(t *testing.T) {
	p := newTestPTY(t, "interactive")
	defer p.Close()
	startLen := p.OutputLen()
	err := p.Start()
	require.NoError(t, err)
	err = p.WaitForOutputSince("Interactive mode ready", startLen, 2*time.Second)
	require.NoError(t, err)

	t.Run("SendLine", func(t *testing.T) {
		p.ClearOutput()
		startLen := p.OutputLen()
		err := p.SendLine("hello world")
		require.NoError(t, err)
		err = p.WaitForOutputSince("ECHO: hello world", startLen, 2*time.Second)
		assert.NoError(t, err)
	})

	t.Run("SendKeys", func(t *testing.T) {
		p.ClearOutput()
		startLen := p.OutputLen()
		err := p.SendInput("test-keys")
		require.NoError(t, err)
		// Send "enter" key to submit
		err = p.SendKeys("enter")
		require.NoError(t, err)
		err = p.WaitForOutputSince("ECHO: test-keys", startLen, 2*time.Second)
		assert.NoError(t, err)
	})

	t.Run("SendKeys unknown key", func(t *testing.T) {
		err := p.SendKeys("unknown-key")
		assert.Error(t, err)
	})

	t.Run("Send to closed PTY", func(t *testing.T) {
		p := newTestPTY(t, "echo")
		p.Close() // Close immediately
		err := p.SendInput("should fail")
		assert.Error(t, err)
	})
}

func TestPTY_WaitForOutputSince_BasicUsage(t *testing.T) {
	t.Run("should find simple text", func(t *testing.T) {
		p := newTestPTY(t, "echo", "find me")
		defer p.Close()
		startLen := p.OutputLen()
		require.NoError(t, p.Start())
		err := p.WaitForOutputSince("find me", startLen, 2*time.Second)
		assert.NoError(t, err)
	})

	t.Run("should time out if text not found", func(t *testing.T) {
		p := newTestPTY(t, "echo", "something else")
		defer p.Close()
		startLen := p.OutputLen()
		require.NoError(t, p.Start())
		err := p.WaitForOutputSince("text that is not there", startLen, 50*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found in new output")
	})

	t.Run("should find text with ANSI codes", func(t *testing.T) {
		p := newTestPTY(t, "ansi")
		defer p.Close()
		startLen := p.OutputLen()
		require.NoError(t, p.Start())
		// We expect "Hello Red" even though the raw output has ANSI codes
		err := p.WaitForOutputSince("Hello Red", startLen, 2*time.Second)
		assert.NoError(t, err)
		// Verify raw output contains escape codes
		assert.Contains(t, p.GetOutput(), "\x1b[31m")
	})
}

func TestPTY_WaitForOutputSince(t *testing.T) {
	p := newTestPTY(t, "interactive")
	defer p.Close()
	startLen := p.OutputLen()
	require.NoError(t, p.Start())
	require.NoError(t, p.WaitForOutputSince("Interactive mode ready", startLen, 2*time.Second))

	// Send first line and wait for its echo
	lenBeforeFirst := p.OutputLen()
	require.NoError(t, p.SendLine("first line"))
	require.NoError(t, p.WaitForOutputSince("ECHO: first line", lenBeforeFirst, 2*time.Second))

	// Now, only look for output since the current position
	startOffset := p.OutputLen()
	require.NoError(t, p.SendLine("second line"))

	// This should find "second line" but ignore the "first line" that is already in the buffer
	err := p.WaitForOutputSince("ECHO: second line", startOffset, 2*time.Second)
	assert.NoError(t, err)

	// This should fail because it's looking for old text in the new output
	err = p.WaitForOutputSince("ECHO: first line", startOffset, 50*time.Millisecond)
	assert.Error(t, err)
}

func TestPTY_WaitForExit(t *testing.T) {
	t.Run("should get correct exit code 0", func(t *testing.T) {
		p := newTestPTY(t, "exit", "0")
		defer p.Close()
		require.NoError(t, p.Start())
		code, err := p.WaitForExit(2 * time.Second)
		require.NoError(t, err)
		assert.Equal(t, 0, code)
	})

	t.Run("should get correct non-zero exit code", func(t *testing.T) {
		p := newTestPTY(t, "exit", "42")
		defer p.Close()
		require.NoError(t, p.Start())
		code, err := p.WaitForExit(2 * time.Second)
		require.NoError(t, err)
		assert.Equal(t, 42, code)
	})

	t.Run("should time out if process does not exit", func(t *testing.T) {
		p := newTestPTY(t, "wait", "5s") // Process waits for 5 seconds
		defer p.Close()
		require.NoError(t, p.Start())
		_, err := p.WaitForExit(50 * time.Millisecond) // We only wait 50ms
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "command timeout")
	})
}

func TestPTY_OutputManagement(t *testing.T) {
	p, err := NewForProgram(context.Background())
	require.NoError(t, err)
	defer p.Close()

	startLen := p.OutputLen()
	_, err = p.GetPTM().WriteString("line 1\nline 2")
	require.NoError(t, err)
	require.NoError(t, p.WaitForOutputSince("line 2", startLen, 1*time.Second))

	// PTY converts LF to CRLF
	assert.Equal(t, "line 1\r\nline 2", p.GetOutput())
	assert.Equal(t, len("line 1\r\nline 2"), p.OutputLen())

	p.ClearOutput()
	assert.Equal(t, "", p.GetOutput())
	assert.Equal(t, 0, p.OutputLen())
}

func TestPTY_Assertions(t *testing.T) {
	p, err := NewForProgram(context.Background())
	require.NoError(t, err)
	defer p.Close()
	// Write to PTS (slave) so it appears at PTM (master) with proper terminal processing
	startLen := p.OutputLen()
	_, err = p.GetPTS().WriteString("Here is some text with \x1b[31mcolors\x1b[0m.")
	require.NoError(t, err)
	require.NoError(t, p.WaitForOutputSince("colors", startLen, 1*time.Second))

	t.Run("AssertOutput", func(t *testing.T) {
		assert.NoError(t, p.AssertOutput("some text"))   // Plain substring
		assert.NoError(t, p.AssertOutput("with colors")) // Works across ANSI codes
		assert.Error(t, p.AssertOutput("not present"))
	})

	t.Run("AssertNotOutput", func(t *testing.T) {
		assert.NoError(t, p.AssertNotOutput("not present"))
		assert.Error(t, p.AssertNotOutput("some text"))
		assert.Error(t, p.AssertNotOutput("with colors"))
	})
}

func Test_normalizeTTYOutput(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"Plain text", "hello world", "hello world"},
		{"Carriage return", "hello\rworld", "helloworld"},
		{"CRLF", "hello\r\nworld", "hello\nworld"},
		{"ANSI color codes", "hello \x1b[31mred\x1b[0m world", "hello red world"},
		{"ANSI cursor movement", "hello\x1b[2Aworld", "helloworld"},
		{"Mixed", "line 1\r\n\x1b[32mline 2\x1b[0m", "line 1\nline 2"},
		{"Empty", "", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizeTTYOutput(tc.input))
		})
	}
}

func Test_collapseWhitespace(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"Plain text", "hello world", "hello world"},
		{"Multiple spaces", "hello   world", "hello world"},
		{"Tabs", "hello\tworld", "hello world"},
		{"Newlines", "hello\nworld", "hello world"},
		{"Mixed whitespace", "  hello \n\t world  ", "hello world"},
		{"Leading/Trailing", "  hello world  ", "hello world"},
		{"Empty", "", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, collapseWhitespace(tc.input))
		})
	}
}

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

	// Wait for data to flow through
	if err := pty.WaitForOutputSince(testMessage, 0, 1*time.Second); err != nil {
		t.Fatalf("failed to wait for output: %v", err)
	}

	// Check output
	output := pty.GetOutput()
	if output == "" {
		t.Error("no output captured from PTY")
	}

	t.Logf("PTY output: %q", output)
}
