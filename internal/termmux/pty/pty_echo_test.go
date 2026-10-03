//go:build !windows

package pty

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The osm launcher answers the child's OSC 10/11 background/foreground
// queries by writing the reply sequences into the child PTY. Those writes
// can land before the child has switched its terminal into raw mode, so the
// kernel line discipline echoes them — and because ESC and BEL are control
// characters, ECHOCTL renders them as visible caret notation
// ("^[]10;rgb:...^G"), which is the mangled startup output the user sees
// before the TUI paints.
//
// A PTY spawned for a TUI must therefore start with echo disabled: the child
// enables echo itself if it wants canonical input echo. This test spawns a
// child that NEVER calls tcsetattr (so the inherited initial termios stays
// in force for the child's whole life), writes the exact reply bytes the
// launcher sends, and asserts none of it is echoed back.
func TestSpawn_NoCaretEchoOfControlSequences(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a process to build test helper")
	}
	t.Parallel()
	skipIfWindows(t)

	proc, err := Spawn(context.Background(), SpawnConfig{
		Command: buildIdleProgram(t), // reads stdin until EOF; never sets termios
	})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	defer proc.Close()

	// The exact reply bytes constructed by the launcher's terminal-theme
	// handling: OSC 10 (foreground) then OSC 11 (background), BEL-terminated.
	reply := "\x1b]10;rgb:0000/0000/0000\x07\x1b]11;rgb:ffff/ffff/ffff\x07"
	if _, err := proc.Write([]byte(reply)); err != nil {
		t.Fatalf("Write reply: %v", err)
	}

	// Process.Read blocks, so drain on a goroutine and consume the channel
	// with deadlines here. Echo is synchronous line-discipline work done
	// while the write is processed, so it would appear promptly; the idle
	// child produces no output of its own, so a silent window is decisive.
	type readResult struct {
		data []byte
		err  error
	}
	reads := make(chan readResult)
	go func() {
		defer close(reads)
		for {
			data, err := proc.Read()
			reads <- readResult{data: data, err: err}
			if err != nil {
				return
			}
		}
	}()

	var output strings.Builder
	quiet := 300 * time.Millisecond
	quietTimer := time.NewTimer(quiet)
	defer quietTimer.Stop()
	for {
		select {
		case <-quietTimer.C:
			got := output.String()
			for _, bad := range []string{"\x1b", "^[", "^G", "]10;rgb:", "]11;rgb:"} {
				if strings.Contains(got, bad) {
					t.Fatalf("PTY echoed control sequence (caret notation): %q", got)
				}
			}
			return
		case res, ok := <-reads:
			if !ok {
				t.Fatalf("reader exited unexpectedly; output: %q", output.String())
			}
			if len(res.data) > 0 {
				output.Write(res.data)
				if !quietTimer.Stop() {
					<-quietTimer.C
				}
				quietTimer.Reset(quiet)
			}
			if res.err != nil {
				// PTY closed (EOF): judge what was echoed before close.
				got := output.String()
				for _, bad := range []string{"\x1b", "^[", "^G", "]10;rgb:", "]11;rgb:"} {
					if strings.Contains(got, bad) {
						t.Fatalf("PTY echoed control sequence (caret notation): %q", got)
					}
				}
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out draining; output so far: %q", output.String())
		}
	}
}
