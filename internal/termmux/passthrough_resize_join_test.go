//go:build !windows

package termmux

import (
	"context"
	"io"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

// scrollRegionEscape matches any DECSTBM sequence: a scroll-region SET
// (`\x1b[<top>;<bot>r`) or the full-screen RESET (`\x1b[r`).
var scrollRegionEscape = regexp.MustCompile(`\x1b\[[0-9;]*r`)

// finalScrollRegionEscape returns the last DECSTBM sequence written to s, or
// "" when the output contains none.
func finalScrollRegionEscape(s string) string {
	all := scrollRegionEscape.FindAllString(s, -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

// TestPassthrough_ResizeWatcherJoinedBeforeScrollRegionReset is the regression
// test for the terminal-corruption defect.
//
// Passthrough's SIGWINCH watcher writes to the real terminal: its callback
// re-applies the chrome scroll region (`\x1b[1;Nr`) on every resize. Teardown
// must therefore join that watcher BEFORE the deferred ResetScrollRegion
// writes `\x1b[r`. If it does not, a resize arriving during shutdown can
// re-apply a restricted scroll region after the reset, leaving the terminal
// scrolling inside a band — chrome and borders drawn in the wrong rows — and
// that state outlives the process.
//
// The invariant asserted here is that the reset is the FINAL scroll-region
// operation on the terminal. A SIGWINCH flood runs for the whole passthrough
// and briefly past it, so a callback is very likely to be in flight at
// teardown; without the join this fails, and with it the reset always wins.
func TestPassthrough_ResizeWatcherJoinedBeforeScrollRegionReset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	// Deliberately not t.Parallel: this drives real SIGWINCH delivery to the
	// process, and a parallel sibling would observe these signals too.

	const attempts = 20
	for attempt := range attempts {
		m, _, _ := passthroughTestManager(t)

		toggleKey := byte(0x1D)
		stdinR, stdinW := io.Pipe()
		stdout := &syncBuffer{}
		ts := &ptTestTermState{width: 80, height: 24}
		sb := statusbar.New(stdout) // writes to stdout so the escapes are inspectable

		// Flood SIGWINCH for as long as the passthrough runs.
		floodStop := make(chan struct{})
		floodDone := make(chan struct{})
		go func() {
			defer close(floodDone)
			for {
				select {
				case <-floodStop:
					return
				default:
				}
				_ = syscall.Kill(syscall.Getpid(), syscall.SIGWINCH)
				time.Sleep(time.Millisecond)
			}
		}()

		// Exit via the toggle key, after the flood has begun, so teardown
		// races the watcher.
		go func() {
			time.Sleep(20 * time.Millisecond)
			_, _ = stdinW.Write([]byte{toggleKey})
			_ = stdinW.Close()
		}()

		reason, err := m.Passthrough(context.Background(), PassthroughConfig{
			TerminalIO: TerminalIO{
				Stdin:  stdinR,
				Stdout: stdout,
				TermFd: 3, // non-negative enables terminal state and the watcher
			},
			ToggleKey: toggleKey,
			TermState: ts,
			StatusBar: sb,
		})
		if err != nil {
			t.Fatalf("attempt %d: Passthrough error: %v", attempt, err)
		}
		if reason != ExitToggle {
			t.Fatalf("attempt %d: reason = %v, want ExitToggle", attempt, reason)
		}

		// Keep signalling briefly past teardown, so a watcher that was NOT
		// joined still gets a chance to write a late scroll region.
		time.Sleep(5 * time.Millisecond)
		close(floodStop)
		<-floodDone
		_ = stdinR.Close()

		got := finalScrollRegionEscape(stdout.String())
		if got != "\x1b[r" {
			t.Fatalf("attempt %d: the final scroll-region escape is %q, want the reset \"\\x1b[r\"; a resize callback ran after teardown's reset and re-applied a restricted scroll region", attempt, got)
		}
	}
}
