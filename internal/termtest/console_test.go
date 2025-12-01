//go:build unix

package termtest

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConsole creates a ConsoleProcess for the helper process.
func newTestConsole(t *testing.T, opts Options) (*ConsoleProcess, error) {
	t.Helper()

	// Point to the test binary itself
	opts.CmdName = os.Args[0]
	// Prepend arguments needed to re-exec the test binary as a helper
	args := append([]string{"-test.run=^TestMain$", "--"}, opts.Args...)
	opts.Args = args

	// The helper process is triggered by this environment variable
	opts.Env = append(opts.Env, "GO_TEST_MODE=helper")

	return NewTest(t, opts)
}

func TestConsole_NewTest(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		cp, err := newTestConsole(t, Options{
			Args: []string{"echo", "ready"},
		})
		require.NoError(t, err)
		defer cp.Close()
		startLen := cp.OutputLen()
		_, err = cp.ExpectSince("ready", startLen, 2*time.Second)
		assert.NoError(t, err)
	})

	t.Run("invalid command", func(t *testing.T) {
		_, err := NewTest(t, Options{
			CmdName: "/non/existent/command",
			Args:    []string{},
		})
		assert.Error(t, err)
	})

	t.Run("default timeout", func(t *testing.T) {
		cp, err := newTestConsole(t, Options{
			Args: []string{"echo", "ready"},
		})
		require.NoError(t, err)
		defer cp.Close()
		// Internal timeout should be the default 30s
		assert.Equal(t, 30*time.Second, cp.timeout)
	})

	t.Run("custom timeout", func(t *testing.T) {
		cp, err := newTestConsole(t, Options{
			Args:           []string{"echo", "ready"},
			DefaultTimeout: 5 * time.Second,
		})
		require.NoError(t, err)
		defer cp.Close()
		assert.Equal(t, 5*time.Second, cp.timeout)
	})

	t.Run("env and dir options", func(t *testing.T) {
		tempDir := t.TempDir()
		cp, err := newTestConsole(t, Options{
			Args: []string{"pwd"},
			Dir:  tempDir,
		})
		require.NoError(t, err)
		defer cp.Close()

		startLen := cp.OutputLen()
		cp.SendLine("pwd")

		_, err = cp.ExpectSince(tempDir, startLen, 2*time.Second)
		assert.NoError(t, err, "Should have used the specified directory")
	})
}

func TestConsole_Interaction(t *testing.T) {
	cp, err := newTestConsole(t, Options{
		Args: []string{"interactive"},
	})
	require.NoError(t, err)
	defer cp.Close()
	startLen := cp.OutputLen()
	_, err = cp.ExpectSince("Interactive mode ready", startLen, 3*time.Second)
	require.NoError(t, err)

	t.Run("SendLine and Expect", func(t *testing.T) {
		startLen := cp.OutputLen()
		err := cp.SendLine("hello console")
		require.NoError(t, err)
		output, err := cp.ExpectSince("ECHO: hello console", startLen, 2*time.Second)
		assert.NoError(t, err)
		assert.Contains(t, output, "ECHO: hello console")
	})

	t.Run("Expect timeout", func(t *testing.T) {
		startLen := cp.OutputLen()
		_, err := cp.ExpectSince("text that will not appear", startLen, 50*time.Millisecond)
		assert.Error(t, err)
	})
}

func TestConsole_ExpectSince_ExpectNew(t *testing.T) {
	cp, err := newTestConsole(t, Options{
		Args: []string{"interactive"},
	})
	require.NoError(t, err)
	defer cp.Close()
	startLen := cp.OutputLen()
	_, err = cp.ExpectSince("Interactive mode ready", startLen, 3*time.Second)
	require.NoError(t, err)

	startLen = cp.OutputLen()
	err = cp.SendLine("first")
	require.NoError(t, err)
	_, err = cp.ExpectSince("ECHO: first", startLen, 2*time.Second)
	require.NoError(t, err)

	t.Run("ExpectSince", func(t *testing.T) {
		start := cp.OutputLen()
		err = cp.SendLine("second")
		require.NoError(t, err)

		_, err = cp.ExpectSince("ECHO: second", start, 2*time.Second)
		assert.NoError(t, err)

		// Should not find old text
		_, err = cp.ExpectSince("ECHO: first", start, 50*time.Millisecond)
		assert.Error(t, err)
	})

	t.Run("ExpectNew", func(t *testing.T) {
		// ExpectNew captures OutputLen when called, then waits for new output after that.
		// To ensure we find "third", we need to call ExpectNew BEFORE the output arrives.

		// Start ExpectNew in a goroutine before sending the command
		resultCh := make(chan error, 1)
		go func() {
			_, err := cp.ExpectNew("ECHO: third", 2*time.Second)
			resultCh <- err
		}()

		// ExpectNew captures the position immediately when called in the goroutine.
		// No sleep needed - this was a race condition.

		// Now send the command
		err = cp.SendLine("third")
		require.NoError(t, err)

		// Wait for the goroutine to complete
		err = <-resultCh
		assert.NoError(t, err)

		// Similarly for "fourth"
		go func() {
			_, err := cp.ExpectNew("ECHO: fourth", 2*time.Second)
			resultCh <- err
		}()

		// No sleep needed here either

		err = cp.SendLine("fourth")
		require.NoError(t, err)

		err = <-resultCh
		require.NoError(t, err)

		// Now ExpectNew should not find "third" because it's before the current position
		_, err = cp.ExpectNew("ECHO: third", 50*time.Millisecond)
		assert.Error(t, err)
	})
}

func TestConsole_ExpectExitCode(t *testing.T) {
	t.Run("correct exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, Options{Args: []string{"exit", "17"}})
		require.NoError(t, err)
		// No need to defer close, ExpectExitCode waits for termination
		_, err = cp.ExpectExitCode(17, 2*time.Second)
		assert.NoError(t, err)
	})

	t.Run("incorrect exit code", func(t *testing.T) {
		cp, err := newTestConsole(t, Options{Args: []string{"exit", "17"}})
		require.NoError(t, err)
		_, err = cp.ExpectExitCode(18, 2*time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expected exit code 18, got 17")
	})

	t.Run("timeout waiting for exit", func(t *testing.T) {
		cp, err := newTestConsole(t, Options{Args: []string{"wait", "2s"}})
		require.NoError(t, err)
		defer cp.Close()
		_, err = cp.ExpectExitCode(0, 50*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to wait for exit")
	})
}

func TestConsole_OutputManagement(t *testing.T) {
	cp, err := newTestConsole(t, Options{
		Args: []string{"echo", "line 1", "line 2"},
	})
	require.NoError(t, err)
	defer cp.Close()
	startLen := cp.OutputLen()
	_, err = cp.ExpectSince("line 2", startLen, 2*time.Second)
	require.NoError(t, err)

	output := cp.GetOutput()
	// Output from helper process will have newlines
	assert.Contains(t, output, "line 1")
	assert.Contains(t, output, "line 2")

	assert.True(t, cp.OutputLen() > 0)

	cp.ClearOutput()
	assert.Equal(t, "", cp.GetOutput())
	assert.Equal(t, 0, cp.OutputLen())
}
