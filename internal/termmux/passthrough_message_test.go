package termmux

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

func TestPassthrough_MessageBarRendered(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, id := passthroughTestManager(t)

	session.readerCh <- []byte("hello")
	waitForSnapshotContains(t, m, id, "hello", 2*time.Second)

	if err := m.DisplayMessage(id, "warn: low fuel", 10*time.Minute); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}
	sb := statusbar.New(stdout)

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: 3,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
			TermState: ts,
		},
		ResizeConfig: ResizeConfig{
			RestoreScreen: false,
		},
		UIConfig: UIConfig{
			StatusBar: sb,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	got := stdout.String()
	if !strings.Contains(got, "warn: low fuel") {
		t.Errorf("stdout missing message bar text; got %q", got)
	}
}

func TestPassthrough_MessageBarAbsentWhenEmpty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, id := passthroughTestManager(t)

	session.readerCh <- []byte("hello")
	waitForSnapshotContains(t, m, id, "hello", 2*time.Second)

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}
	sb := statusbar.New(stdout)

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: 3,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
			TermState: ts,
		},
		UIConfig: UIConfig{
			StatusBar: sb,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	got := stdout.String()
	if strings.Contains(got, "warn: not shown") {
		t.Errorf("stdout unexpectedly contained message bar text; got %q", got)
	}
}

func TestPassthrough_MessageBarResizeAccounting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, id := passthroughTestManager(t)

	session.readerCh <- []byte("hello")
	waitForSnapshotContains(t, m, id, "hello", 2*time.Second)

	if err := m.DisplayMessage(id, "warn: low fuel", 10*time.Minute); err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}
	sb := statusbar.New(stdout)

	var resizeRows, resizeCols int
	resizeCalled := false

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: 3,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
			TermState: ts,
		},
		ResizeConfig: ResizeConfig{
			ResizeFn: func(rows, cols uint16) error {
				resizeRows, resizeCols = int(rows), int(cols)
				resizeCalled = true
				return nil
			},
		},
		UIConfig: UIConfig{
			StatusBar: sb,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	if !resizeCalled {
		t.Fatal("ResizeFn was not called")
	}
	if resizeRows != 22 {
		t.Errorf("ResizeFn rows = %d, want 22 (24 - status bar - message bar)", resizeRows)
	}
	if resizeCols != 80 {
		t.Errorf("ResizeFn cols = %d, want 80", resizeCols)
	}
}
