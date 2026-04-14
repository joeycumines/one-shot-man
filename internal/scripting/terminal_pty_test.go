//go:build unix

package scripting

import (
	"bytes"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminalRun_TerminalStateSaveRestore swaps os.Stdin to a real PTY so
// that term.IsTerminal(os.Stdin.Fd()) returns true, exercising the terminal
// state save (GetState) and restore (Restore) branches in Terminal.Run().
//
// DO NOT call t.Parallel() — this test mutates the global os.Stdin.
func TestTerminalRun_TerminalStateSaveRestore(t *testing.T) {
	// Open a PTY pair. The slave end becomes the fake os.Stdin.
	master, slave, err := pty.Open()
	require.NoError(t, err)

	origStdin := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() {
		os.Stdin = origStdin
		slave.Close()
		master.Close()
	})

	var engineOut bytes.Buffer
	ctx := t.Context()
	sessionID := testutil.NewTestSessionID("test", t.Name())

	engine, err := NewEngine(ctx, &engineOut, &engineOut, sessionID, "memory", nil, 0, slog.LevelInfo)
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })

	// Replace TUI I/O so go-prompt exits immediately via EOF → Ctrl-D.
	engine.tuiManager.reader = NewTUIReaderFromIO(bytes.NewReader(nil))
	var tuiOut bytes.Buffer
	engine.tuiManager.writer = NewTUIWriterFromIO(&tuiOut)

	terminal := NewTerminal(ctx, engine)

	done := make(chan struct{})
	go func() {
		defer close(done)
		terminal.Run()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Terminal.Run() did not exit within timeout")
	}

	// Session persistence should still succeed.
	output := tuiOut.String()
	assert.Contains(t, output, "Saving session...")
	assert.Contains(t, output, "Session saved successfully.")
}
