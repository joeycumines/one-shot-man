package termmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
	"golang.org/x/term"
)

// ---------------------------------------------------------------------------
// SessionManager helpers
// ---------------------------------------------------------------------------

// startManager creates a SessionManager, starts the worker, and returns
// the manager and a cleanup function. Cleanup cancels the context and
// waits for the worker to stop.
func startManager(t *testing.T, opts ...ManagerOption) (*SessionManager, func()) {
	t.Helper()
	m := NewSessionManager(opts...)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()
	// Wait for the worker goroutine to start processing before
	// returning — prevents races where API calls arrive before
	// the worker is ready.
	<-m.Started()
	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, cleanup
}

// waitForSnapshotContains polls the SessionManager for a snapshot that
// contains the given substring. Fatals on timeout.
func waitForSnapshotContains(t *testing.T, m *SessionManager, id SessionID, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		snap := m.Snapshot(id)
		if snap != nil && strings.Contains(snap.GetPlainText(), substr) {
			return
		}
		select {
		case <-deadline:
			snap := m.Snapshot(id)
			t.Fatalf("timed out waiting for snapshot containing %q; snap=%v", substr, snap)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// Mock InteractiveSession implementations
// ---------------------------------------------------------------------------

// mockSession is a minimal InteractiveSession for type-level tests.
type mockSession struct{}

func (mockSession) Resize(int, int) error     { return nil }
func (mockSession) Write([]byte) (int, error) { return 0, nil }
func (mockSession) Close() error              { return nil }
func (mockSession) Done() <-chan struct{}     { ch := make(chan struct{}); close(ch); return ch }
func (mockSession) Reader() <-chan []byte     { ch := make(chan []byte); close(ch); return ch }

// controllableSession is a richer mock that records calls and allows
// controlling behavior from tests.
type controllableSession struct {
	writtenData []byte
	writeMu     sync.Mutex
	writeErr    error
	resizeCalls []resizePayload
	closeCalled atomic.Bool
	closeOnce   sync.Once
	doneCh      chan struct{}
	readerCh    chan []byte
}

func newControllableSession() *controllableSession {
	return &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
}

func (s *controllableSession) Done() <-chan struct{} { return s.doneCh }
func (s *controllableSession) Reader() <-chan []byte { return s.readerCh }

func (s *controllableSession) Write(data []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	s.writtenData = append(s.writtenData, data...)
	return len(data), nil
}

func (s *controllableSession) Resize(rows, cols int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.resizeCalls = append(s.resizeCalls, resizePayload{rows: rows, cols: cols})
	return nil
}

func (s *controllableSession) Close() error {
	s.closeOnce.Do(func() {
		s.closeCalled.Store(true)
		select {
		case <-s.doneCh:
		default:
			close(s.doneCh)
		}
	})
	return nil
}

func (s *controllableSession) Written() []byte {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]byte, len(s.writtenData))
	copy(cp, s.writtenData)
	return cp
}

func (s *controllableSession) Resizes() []resizePayload {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]resizePayload, len(s.resizeCalls))
	copy(cp, s.resizeCalls)
	return cp
}

// ---------------------------------------------------------------------------
// Passthrough mock types
// ---------------------------------------------------------------------------

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
// All fields are accessed atomically or under a mutex so tests can read
// them concurrently from a different goroutine.
type ptTestTermState struct {
	mu            sync.Mutex
	rawCalled     bool
	restoreCalled bool
	rawFd         int
	restoreFd     int
	width, height int
}

func (m *ptTestTermState) MakeRaw(fd int) (*term.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rawCalled = true
	m.rawFd = fd
	return nil, nil
}

func (m *ptTestTermState) Restore(fd int, state *term.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreCalled = true
	m.restoreFd = fd
	return nil
}

func (m *ptTestTermState) isRawCalled() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.rawCalled }
func (m *ptTestTermState) isRestoreCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restoreCalled
}
func (m *ptTestTermState) getRawFd() int     { m.mu.Lock(); defer m.mu.Unlock(); return m.rawFd }
func (m *ptTestTermState) getRestoreFd() int { m.mu.Lock(); defer m.mu.Unlock(); return m.restoreFd }

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
// All fields are accessed under a mutex so tests can read them concurrently.
type ptTestBlockingGuard struct {
	mu            sync.Mutex
	ensureCalled  bool
	restoreCalled bool
	ensureFd      int
	restoreFd     int
}

func (m *ptTestBlockingGuard) EnsureBlocking(fd int) (origFlags int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCalled = true
	m.ensureFd = fd
	return 0, nil
}

func (m *ptTestBlockingGuard) Restore(fd int, origFlags int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restoreCalled = true
	m.restoreFd = fd
}

func (m *ptTestBlockingGuard) isEnsureCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureCalled
}
func (m *ptTestBlockingGuard) isRestoreCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restoreCalled
}
func (m *ptTestBlockingGuard) getEnsureFd() int { m.mu.Lock(); defer m.mu.Unlock(); return m.ensureFd }
func (m *ptTestBlockingGuard) getRestoreFd() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restoreFd
}

// passthroughTestManager creates a SessionManager with a registered
// controllable session and returns everything needed for passthrough testing.
// The session starts in the Running state.
func passthroughTestManager(t *testing.T) (*SessionManager, *controllableSession, SessionID) {
	t.Helper()
	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test-pt", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Pump some output to transition to Running state.
	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	return m, session, id
}

// ---------------------------------------------------------------------------
// CaptureSession helpers
// ---------------------------------------------------------------------------

// testOutputCollector reads all output from a CaptureSession's Reader() channel
// in a background goroutine. Call startCollector(cs) immediately after cs.Start().
type testOutputCollector struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func startCollector(cs *CaptureSession) *testOutputCollector {
	tc := &testOutputCollector{done: make(chan struct{})}
	ch := cs.Reader()
	if ch == nil {
		close(tc.done)
		return tc
	}
	go func() {
		defer close(tc.done)
		for chunk := range ch {
			tc.mu.Lock()
			tc.buf.Write(chunk)
			tc.mu.Unlock()
		}
	}()
	return tc
}

func (tc *testOutputCollector) current() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.buf.String()
}

func (tc *testOutputCollector) wait() string {
	<-tc.done
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.buf.String()
}

// verify InteractiveSession contract at compile time.
var _ InteractiveSession = (*controllableSession)(nil)
var _ InteractiveSession = (mockSession{})
var _ ptyio.TermState = (*ptTestTermState)(nil)
var _ ptyio.BlockingGuard = (*ptTestBlockingGuard)(nil)

// ---------------------------------------------------------------------------
// Cross-platform test program builders
// ---------------------------------------------------------------------------

var ()

// buildProgram compiles a Go source string into a binary and returns its path.
func buildProgram(t *testing.T, src string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("spawns process to build test helper")
	}
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourceFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}
	binName := "testprog"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)
	cmd := exec.Command("go", "build", "-o", bin, sourceFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

// buildIdleProgram builds a binary that reads stdin until EOF then exits,
// replacing "cat" in tests that need a long-lived process attached to a PTY.
func buildIdleProgram(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return buildProgram(t, `package main

import (
	"io"
	"os"
)

func main() {
	io.Copy(os.Stdout, os.Stdin)
}
`)
}

// buildEchoIdleProgram builds a binary that prints the given text to stdout
// and then reads stdin until EOF, replacing "sh -c 'echo text; exec cat'"
// patterns in tests.
func buildEchoIdleProgram(t *testing.T, text string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	fmt.Println(%q)
	io.Copy(os.Stdout, os.Stdin)
}
`, text))
}

// buildEchoProgram builds a binary that prints the given text and exits,
// replacing "sh -c 'echo text'" patterns.
func buildEchoProgram(t *testing.T, text string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import "fmt"

func main() {
	fmt.Println(%q)
}
`, text))
}

// buildExitCodeProgram builds a binary that exits with the given code,
// replacing "sh -c 'exit N'" patterns.
func buildExitCodeProgram(t *testing.T, code int) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import "os"

func main() {
	os.Exit(%d)
}
`, code))
}

// buildEnvEchoProgram builds a binary that prints the value of the given
// environment variable and exits, replacing "sh -c 'echo $VAR'" patterns.
func buildEnvEchoProgram(t *testing.T, varName string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Getenv(%q))
}
`, varName))
}

// buildPwdProgram builds a binary that prints its working directory and exits,
// replacing "sh -c 'pwd'" patterns.
func buildPwdProgram(t *testing.T) string {
	t.Helper()
	return buildProgram(t, `package main

import (
	"fmt"
	"os"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(dir)
}
`)
}

// buildSeqProgram builds a binary that prints sequential lines (e.g. "line1"
// through "lineN") and exits, replacing "sh -c 'for i in $(seq 1 N); do
// echo prefix$i; done'" patterns. The format string uses %d for the index.
func buildSeqProgram(t *testing.T, count int, format string) string {
	t.Helper()
	var lines []string
	for i := 1; i <= count; i++ {
		lines = append(lines, fmt.Sprintf(format, i))
	}
	return buildEchoProgram(t, strings.Join(lines, "\n"))
}

// buildSeqIdleProgram builds a binary that prints sequential lines and then
// reads stdin until EOF, replacing "sh -c 'for i in ...; do echo ...; done;
// sleep 60'" patterns.
func buildSeqIdleProgram(t *testing.T, count int, format string) string {
	t.Helper()
	var lines []string
	for i := 1; i <= count; i++ {
		lines = append(lines, fmt.Sprintf(format, i))
	}
	return buildEchoIdleProgram(t, strings.Join(lines, "\n"))
}

// buildPeriodicProgram builds a binary that prints "line0", "line1", etc.
// every 10ms indefinitely, replacing "sh -c 'while true; do echo line$i;
// i=$((i+1)); sleep 0.1; done'" patterns. A brief startup delay lets the
// PTY reader pipeline initialize before the first write. The binary is
// cached at the package level to avoid concurrent go build contention.
func buildPeriodicProgram(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("spawns process to build test helper")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main

import (
	"fmt"
	"time"
)

func main() {
	time.Sleep(50 * time.Millisecond)
	i := 0
	for {
		fmt.Printf("line%d\n", i)
		i++
		time.Sleep(10 * time.Millisecond)
	}
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}
	binName := "periodicprogram"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)
	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}
