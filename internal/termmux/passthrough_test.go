package termmux

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
)

func TestSessionManager_Passthrough_ToggleKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	// Create stdin that sends some bytes then the toggle key.
	toggleKey := byte(0x1D) // Ctrl+]
	stdinData := append([]byte("hello"), toggleKey)
	stdin := bytes.NewReader(stdinData)
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: -1,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}
}

func TestSessionManager_Passthrough_ChildExit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, _ := passthroughTestManager(t)

	// Use a blocking stdin that never sends data.
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	stdout := &bytes.Buffer{}

	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)
	go func() {
		reason, err := m.Passthrough(context.Background(), PassthroughConfig{
			TerminalIO: TerminalIO{
				Stdin:  stdinR,
				Stdout: stdout,
				TermFd: -1,
			},
			PassthroughOptions: PassthroughOptions{
				ToggleKey: 0x1D,
			},
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	// Wait briefly to ensure passthrough is running, then simulate session exit.
	time.Sleep(100 * time.Millisecond)
	close(session.readerCh) // EOF on reader → session exits
	close(session.doneCh)   // Signal session Done channel

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("Passthrough error: %v", r.err)
		}
		if r.reason != ExitChildExit {
			t.Errorf("reason = %v, want ExitChildExit", r.reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Passthrough to return")
	}
}

func TestSessionManager_Passthrough_Context(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	stdout := &bytes.Buffer{}

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)
	go func() {
		reason, err := m.Passthrough(ctx, PassthroughConfig{
			TerminalIO: TerminalIO{
				Stdin:  stdinR,
				Stdout: stdout,
				TermFd: -1,
			},
			PassthroughOptions: PassthroughOptions{
				ToggleKey: 0x1D,
			},
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	// Wait briefly then cancel the context.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case r := <-resultCh:
		if r.reason != ExitContext {
			t.Errorf("reason = %v, want ExitContext", r.reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Passthrough to return")
	}
}

func TestSessionManager_Passthrough_InputForwarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, _ := passthroughTestManager(t)

	// Send "hello" followed by toggle key. Bytes before toggle should
	// be forwarded to the session.
	toggleKey := byte(0x1D)
	stdinData := append([]byte("hello"), toggleKey)
	stdin := bytes.NewReader(stdinData)
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: -1,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Verify the session received the bytes before the toggle key.
	got := string(session.Written())
	if got != "hello" {
		t.Errorf("session received %q, want %q", got, "hello")
	}
}

func TestSessionManager_Passthrough_OutputForwarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, _ := passthroughTestManager(t)

	// Use a blocking stdin.
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	stdout := &syncBuffer{}

	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		reason, err := m.Passthrough(ctx, PassthroughConfig{
			TerminalIO: TerminalIO{
				Stdin:  stdinR,
				Stdout: stdout,
				TermFd: -1,
			},
			PassthroughOptions: PassthroughOptions{
				ToggleKey: 0x1D,
			},
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	// Wait for passthrough to start (tee to be enabled).
	time.Sleep(200 * time.Millisecond)

	// Send output through the session's Reader channel.
	// This should be teed to stdout by the passthroughWriter.
	session.readerCh <- []byte("output-data")

	// Wait for the output to appear in stdout.
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(stdout.String(), "output-data") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for output; stdout = %q", stdout.String())
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Clean exit via context cancel.
	cancel()
	<-resultCh
}

func TestSessionManager_Passthrough_TerminalRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	ts := &ptTestTermState{width: 80, height: 24}
	bg := &ptTestBlockingGuard{}

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &bytes.Buffer{}

	// TermFd=999 is a fake fd; MakeRaw/Restore just record calls.
	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:         stdin,
			Stdout:        stdout,
			TermFd:        999,
			BlockingGuard: bg,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
			TermState: ts,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Verify terminal state was saved and restored.
	if !ts.isRawCalled() {
		t.Error("MakeRaw was not called")
	}
	if ts.getRawFd() != 999 {
		t.Errorf("MakeRaw fd = %d, want 999", ts.getRawFd())
	}
	if !ts.isRestoreCalled() {
		t.Error("Restore was not called")
	}
	if ts.getRestoreFd() != 999 {
		t.Errorf("Restore fd = %d, want 999", ts.getRestoreFd())
	}
	if !bg.isEnsureCalled() {
		t.Error("EnsureBlocking was not called")
	}
	if bg.getEnsureFd() != 999 {
		t.Errorf("EnsureBlocking fd = %d, want 999", bg.getEnsureFd())
	}
	if !bg.isRestoreCalled() {
		t.Error("BlockingGuard.Restore was not called")
	}
	if bg.getRestoreFd() != 999 {
		t.Errorf("BlockingGuard.Restore fd = %d, want 999", bg.getRestoreFd())
	}
}

func TestSessionManager_Passthrough_BeforeRun(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	// Do NOT call Run.

	stdin := bytes.NewReader([]byte("hello"))
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: -1,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: 0x1D,
		},
	})
	if reason != ExitError {
		t.Errorf("reason = %v, want ExitError", reason)
	}
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestSessionManager_Passthrough_NoActiveSession(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t)
	t.Cleanup(cleanup)

	// Manager is running but has no registered sessions.
	stdin := bytes.NewReader([]byte("hello"))
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: -1,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: 0x1D,
		},
	})
	if reason != ExitError {
		t.Errorf("reason = %v, want ExitError", reason)
	}
	if err == nil {
		t.Error("expected error when no active session exists, got nil")
	}
}

func TestSessionManager_Passthrough_UnregisteredDuringPassthrough(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, id := passthroughTestManager(t)

	// Unregister the session so ActiveID() returns 0.
	m.Unregister(id)

	// Verify the session is gone.
	session.writeMu.Lock()
	closed := session.closeCalled.Load()
	session.writeMu.Unlock()
	if !closed {
		t.Fatal("session should have been closed by Unregister")
	}

	stdin := bytes.NewReader([]byte("hello"))
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: -1,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: 0x1D,
		},
	})
	if reason != ExitError {
		t.Errorf("reason = %v, want ExitError", reason)
	}
	if err == nil {
		t.Error("expected error when active session was unregistered, got nil")
	}
}

func TestSessionManager_Passthrough_RestoreScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, id := passthroughTestManager(t)

	// Send output so the VTerm has content.
	session.readerCh <- []byte("screen-content")
	deadline := time.After(2 * time.Second)
	for {
		snap := m.Snapshot(id)
		if snap != nil && strings.Contains(snap.PlainText, "screen-content") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for snapshot")
		case <-time.After(10 * time.Millisecond):
		}
	}

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: -1,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
		},
		ResizeConfig: ResizeConfig{
			RestoreScreen: true,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Verify stdout received the VTerm restore content.
	// FullScreen output should contain the screen-content with CUP sequences.
	got := stdout.String()
	if !strings.Contains(got, "screen-content") {
		t.Errorf("stdout did not contain restored screen content; got %q", got)
	}

	// Verify that the erase-below sequence is emitted after FullScreen.
	// This clears rows beyond the VTerm's height to prevent ghost content.
	// The manager was created with WithTermSize(24, 80), so snap.Rows=24.
	// The erase sequence is: CUP(row 25, col 1) + ED(0).
	if !strings.Contains(got, "\x1b[25;1H\x1b[0J") {
		t.Errorf("stdout missing erase-below sequence (ghost row clear); got %q", got)
	}
}

func TestPassthroughStatusBar_ScrollRegionSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}
	sb := statusbar.New(stdout) // writes to stdout so we can inspect

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:  stdin,
			Stdout: stdout,
			TermFd: 3, // non-negative enables terminal state
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

	// Verify TermState.MakeRaw was called (proves TermFd was used).
	if !ts.isRawCalled() {
		t.Error("MakeRaw not called")
	}

	// Verify stdout contains a DECSTBM scroll region escape sequence.
	// Format: ESC [ 1 ; <height-1> r
	// For 24-row terminal: ESC [1;23r
	got := stdout.String()
	if !strings.Contains(got, "\x1b[1;23r") {
		t.Errorf("stdout missing scroll region setup (DECSTBM); got %q", got)
	}

	// Verify that the reset scroll region is emitted (from deferred ResetScrollRegion).
	if !strings.Contains(got, "\x1b[r") {
		t.Errorf("stdout missing scroll region reset; got %q", got)
	}

	// Verify that status bar content was rendered (generic title, no product branding).
	if strings.Contains(got, "[Claude]") {
		t.Errorf("stdout should not contain product-specific branding; got %q", got)
	}
	// The status bar should contain the toggle key hint.
	if !strings.Contains(got, "switch") {
		t.Errorf("stdout missing status bar render (toggle hint); got %q", got)
	}
}

func TestPassthroughStatusBar_MouseRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	// Build an SGR mouse left-click on row 24 (the status bar row in a 24-row terminal).
	// Format: ESC [ < 0 ; 1 ; 24 M
	mouseClick := []byte("\x1b[<0;1;24M")

	stdin := bytes.NewReader(mouseClick)
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
			ToggleKey: 0x1D,
			TermState: ts,
		},
		UIConfig: UIConfig{
			StatusBar: sb,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}

	// The status bar click should trigger ExitToggle (same as toggle key).
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle (status bar click)", reason)
	}
}

func TestPassthroughStatusBar_RenderRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, id := passthroughTestManager(t)

	// Send output so VTerm has content for FullScreen restoration.
	session.readerCh <- []byte("restore-me")
	deadline := time.After(2 * time.Second)
	for {
		snap := m.Snapshot(id)
		if snap != nil && strings.Contains(snap.PlainText, "restore-me") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for snapshot")
		case <-time.After(10 * time.Millisecond):
		}
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
			RestoreScreen: true,
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

	// Verify VTerm screen was restored (FullScreen contains the content).
	if !strings.Contains(got, "restore-me") {
		t.Errorf("stdout missing restored screen content; got %q", got)
	}

	// Verify erase-below sequence is emitted. With status bar (1 line)
	// on a 24-row terminal, the VTerm is resized to 23 rows, so erase
	// starts at row 24 (the row after VTerm content). This clears any
	// ghost rows below the restored VTerm screen.
	if !strings.Contains(got, "\x1b[24;1H\x1b[0J") {
		t.Errorf("stdout missing erase-below sequence (ghost row clear with status bar); got %q", got)
	}

	// After RestoreScreen, the status bar should be re-rendered
	// (passthrough.go line 85-86: if cfg.StatusBar != nil && statusBarLines > 0).
	// The status bar toggle hint should appear more than once
	// (initial render + post-restore render).
	switchCount := strings.Count(got, "switch")
	if switchCount < 2 {
		t.Errorf("status bar toggle hint count = %d, want >= 2 (initial + post-restore); got %q", switchCount, got)
	}
	// Verify no product-specific branding leaked.
	if strings.Contains(got, "[Claude]") {
		t.Errorf("stdout should not contain product-specific branding; got %q", got)
	}
}

func TestPassthrough_InitialResizeOnSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, session, _ := passthroughTestManager(t)

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}

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
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Passthrough should have called mgr.Resize(24, 80) during setup
	// (no status bar, TermFd >= 0, TermState non-nil).
	// This propagates to session.Resize(24, 80).
	session.writeMu.Lock()
	resizes := append([]resizePayload(nil), session.resizeCalls...)
	session.writeMu.Unlock()

	found := false
	for _, r := range resizes {
		if r.rows == 24 && r.cols == 80 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session did not receive Resize(24, 80); got %v", resizes)
	}
}

func TestPassthrough_ResizeFnCallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}
	sb := statusbar.New(stdout)

	var resizeFnCalled bool
	var resizeFnRows, resizeFnCols uint16

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
				resizeFnCalled = true
				resizeFnRows = rows
				resizeFnCols = cols
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

	// ResizeFn should have been called during setup (passthrough.go ~line 95-98)
	// with dimensions accounting for the status bar: childH = 24 - 1 = 23, w = 80.
	if !resizeFnCalled {
		t.Error("ResizeFn was not called")
	}
	if resizeFnRows != 23 {
		t.Errorf("ResizeFn rows = %d, want 23 (24 terminal - 1 status bar)", resizeFnRows)
	}
	if resizeFnCols != 80 {
		t.Errorf("ResizeFn cols = %d, want 80", resizeFnCols)
	}
}

func TestSessionManager_Passthrough_SessionClosedEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, id := passthroughTestManager(t)

	// Use blocking stdin so passthrough stays running.
	stdinR, stdinW := io.Pipe()
	stdout := &syncBuffer{}

	// Subscribe to events so we can verify the EventSessionClosed event
	// was actually received.
	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)
	go func() {
		reason, err := m.Passthrough(context.Background(), PassthroughConfig{
			TerminalIO: TerminalIO{
				Stdin:  stdinR,
				Stdout: stdout,
				TermFd: -1,
			},
			PassthroughOptions: PassthroughOptions{
				ToggleKey: 0x1D,
			},
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	// Wait for passthrough to start.
	time.Sleep(200 * time.Millisecond)

	// Unregister the session — this should trigger EventSessionClosed
	// which passthrough should detect and exit with ExitChildExit.
	m.Unregister(id)

	select {
	case r := <-resultCh:
		if r.reason != ExitChildExit {
			t.Errorf("reason = %v, want ExitChildExit", r.reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for passthrough to return after session closed")
	}

	// Verify that the EventSessionClosed event was actually emitted
	// (not some other event that happened to unblock passthrough).
	// The event may not have been buffered yet, so poll with a timeout.
	var sawClosed bool
	drainDeadline := time.After(2 * time.Second)
	for !sawClosed {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionClosed && evt.SessionID == id {
				sawClosed = true
			}
		case <-drainDeadline:
			goto done
		}
	}
done:
	if !sawClosed {
		t.Error("EventSessionClosed event was not received for the unregistered session")
	}

	// Close stdinW to prevent goroutine leak (Pipe read-side in passthrough
	// is now done, but the write-side goroutine inside io.Pipe is still
	// waiting if we don't close it).
	stdinW.Close()
}

func TestSessionManager_Passthrough_ContextCancel_RestoresTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	t.Parallel()

	m, _, _ := passthroughTestManager(t)

	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	stdout := &syncBuffer{}

	ts := &ptTestTermState{width: 80, height: 24}
	bg := &ptTestBlockingGuard{}

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan struct {
		reason ExitReason
		err    error
	}, 1)
	go func() {
		reason, err := m.Passthrough(ctx, PassthroughConfig{
			TerminalIO: TerminalIO{
				Stdin:         stdinR,
				Stdout:        stdout,
				TermFd:        999,
				BlockingGuard: bg,
			},
			PassthroughOptions: PassthroughOptions{
				ToggleKey: 0x1D,
				TermState: ts,
			},
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	// Wait for passthrough to enter raw mode (poll instead of fixed sleep
	// to avoid flakiness under load).
	rawDeadline := time.After(5 * time.Second)
	for !ts.isRawCalled() {
		select {
		case <-rawDeadline:
			t.Fatal("timed out waiting for MakeRaw to be called")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Cancel the context — passthrough must restore terminal state even on
	// context cancellation (deferred Restore in passthrough.go).
	cancel()

	select {
	case r := <-resultCh:
		if r.reason != ExitContext {
			t.Errorf("reason = %v, want ExitContext", r.reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for passthrough to return after context cancel")
	}

	// Verify terminal state was restored even though exit was via context.
	if !ts.isRestoreCalled() {
		t.Error("Restore was not called after context cancellation")
	}
	if !bg.isRestoreCalled() {
		t.Error("BlockingGuard.Restore was not called after context cancellation")
	}
}

func TestCaptureSession_Passthrough_WithTerminalState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"capture-pt-term"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	ts := &ptTestTermState{width: 80, height: 24}
	bg := &ptTestBlockingGuard{}
	toggleKey := byte(0x1D)
	stdin := bytes.NewReader([]byte{toggleKey})
	stdout := &syncBuffer{}

	reason, err := cs.Passthrough(context.Background(), PassthroughConfig{
		TerminalIO: TerminalIO{
			Stdin:         stdin,
			Stdout:        stdout,
			TermFd:        3,
			BlockingGuard: bg,
		},
		PassthroughOptions: PassthroughOptions{
			ToggleKey: toggleKey,
			TermState: ts,
		},
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Verify terminal state was set and restored.
	if !ts.isRawCalled() {
		t.Error("MakeRaw was not called")
	}
	if !ts.isRestoreCalled() {
		t.Error("Restore was not called")
	}
	if !bg.isEnsureCalled() {
		t.Error("EnsureBlocking was not called")
	}
	if !bg.isRestoreCalled() {
		t.Error("BlockingGuard.Restore was not called")
	}
}
