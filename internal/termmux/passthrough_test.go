package termmux

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/statusbar"
	"golang.org/x/term"
)

// syncBuffer is a goroutine-safe bytes.Buffer for concurrent test writes
// and reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// ptTestTermState implements ptyio.TermState for passthrough testing.
type ptTestTermState struct {
	rawCalled     bool
	restoreCalled bool
	width, height int
}

func (m *ptTestTermState) MakeRaw(fd int) (*term.State, error) {
	m.rawCalled = true
	return nil, nil // nil state is fine for tests
}

func (m *ptTestTermState) Restore(fd int, state *term.State) error {
	m.restoreCalled = true
	return nil
}

func (m *ptTestTermState) GetSize(fd int) (width, height int, err error) {
	w, h := m.width, m.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	return w, h, nil
}

// ptTestBlockingGuard implements ptyio.BlockingGuard for passthrough testing.
type ptTestBlockingGuard struct {
	ensureCalled  bool
	restoreCalled bool
}

func (m *ptTestBlockingGuard) EnsureBlocking(fd int) (origFlags int, err error) {
	m.ensureCalled = true
	return 0, nil
}

func (m *ptTestBlockingGuard) Restore(fd int, origFlags int) {
	m.restoreCalled = true
}

// passthroughTestManager creates a SessionManager with a registered
// controllable session and returns everything needed for passthrough testing.
func passthroughTestManager(t *testing.T) (*SessionManager, *controllableSession, SessionID) {
	t.Helper()
	m := NewSessionManager(WithTermSize(24, 80))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test-pt", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Pump some output to transition to Running state.
	session.readerCh <- []byte("ready")
	// Wait for the output to be processed.
	deadline := time.After(2 * time.Second)
	for {
		snap := m.Snapshot(id)
		if snap != nil && strings.Contains(snap.PlainText, "ready") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for session to reach Running state")
		case <-time.After(10 * time.Millisecond):
		}
	}

	t.Cleanup(func() {
		cancel()
		<-errCh
	})

	return m, session, id
}

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
		Stdin:     stdin,
		Stdout:    stdout,
		TermFd:    -1,
		ToggleKey: toggleKey,
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
			Stdin:     stdinR,
			Stdout:    stdout,
			TermFd:    -1,
			ToggleKey: 0x1D,
		})
		resultCh <- struct {
			reason ExitReason
			err    error
		}{reason, err}
	}()

	// Wait briefly to ensure passthrough is running, then close session.
	time.Sleep(100 * time.Millisecond)
	close(session.readerCh) // EOF on reader → session exits

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
			Stdin:     stdinR,
			Stdout:    stdout,
			TermFd:    -1,
			ToggleKey: 0x1D,
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
		Stdin:     stdin,
		Stdout:    stdout,
		TermFd:    -1,
		ToggleKey: toggleKey,
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
			Stdin:     stdinR,
			Stdout:    stdout,
			TermFd:    -1,
			ToggleKey: 0x1D,
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
		Stdin:         stdin,
		Stdout:        stdout,
		TermFd:        999,
		ToggleKey:     toggleKey,
		TermState:     ts,
		BlockingGuard: bg,
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Verify terminal state was saved and restored.
	if !ts.rawCalled {
		t.Error("MakeRaw was not called")
	}
	if !ts.restoreCalled {
		t.Error("Restore was not called")
	}
	if !bg.ensureCalled {
		t.Error("EnsureBlocking was not called")
	}
	if !bg.restoreCalled {
		t.Error("BlockingGuard.Restore was not called")
	}
}

func TestSessionManager_Passthrough_BeforeRun(t *testing.T) {
	t.Parallel()

	m := NewSessionManager()
	// Do NOT call Run.

	stdin := bytes.NewReader([]byte("hello"))
	stdout := &bytes.Buffer{}

	reason, err := m.Passthrough(context.Background(), PassthroughConfig{
		Stdin:     stdin,
		Stdout:    stdout,
		TermFd:    -1,
		ToggleKey: 0x1D,
	})
	if reason != ExitError {
		t.Errorf("reason = %v, want ExitError", reason)
	}
	if err == nil {
		t.Error("expected error, got nil")
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
		Stdin:         stdin,
		Stdout:        stdout,
		TermFd:        -1,
		ToggleKey:     toggleKey,
		RestoreScreen: true,
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
		Stdin:     stdin,
		Stdout:    stdout,
		TermFd:    3, // non-negative enables terminal state
		TermState: ts,
		ToggleKey: toggleKey,
		StatusBar: sb,
	})
	if err != nil {
		t.Fatalf("Passthrough error: %v", err)
	}
	if reason != ExitToggle {
		t.Errorf("reason = %v, want ExitToggle", reason)
	}

	// Verify TermState.MakeRaw was called (proves TermFd was used).
	if !ts.rawCalled {
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

	// Verify that status bar content was rendered.
	if !strings.Contains(got, "[Claude]") {
		t.Errorf("stdout missing status bar render; got %q", got)
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
		Stdin:     stdin,
		Stdout:    stdout,
		TermFd:    3,
		TermState: ts,
		ToggleKey: 0x1D,
		StatusBar: sb,
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
		Stdin:         stdin,
		Stdout:        stdout,
		TermFd:        3,
		TermState:     ts,
		ToggleKey:     toggleKey,
		StatusBar:     sb,
		RestoreScreen: true,
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

	// After RestoreScreen, the status bar should be re-rendered
	// (passthrough.go line 85-86: if cfg.StatusBar != nil && statusBarLines > 0).
	// Count occurrences of "[Claude]" — should appear more than once
	// (initial render + post-restore render).
	renderCount := strings.Count(got, "[Claude]")
	if renderCount < 2 {
		t.Errorf("status bar render count = %d, want >= 2 (initial + post-restore); got %q", renderCount, got)
	}
}
