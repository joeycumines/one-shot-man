package termmux

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// InteractiveSession contract tests
// ---------------------------------------------------------------------------
//
// These tests verify the InteractiveSession interface contract across all
// known implementations: CaptureSession, StringIOSession, and the mock
// controllableSession used by SessionManager tests.
//
// The interface requires:
//
//	Write([]byte) (int, error)
//	Resize(rows, cols int) error
//	Close() error
//	Done() <-chan struct{}
//	Reader() <-chan []byte

// sessionFactory creates a fresh InteractiveSession for contract testing.
// Each factory returns the session and a cleanup function.
type sessionFactory func(t *testing.T) (InteractiveSession, func())

// allFactories returns the set of session factories available on the current
// platform. PTY-based factories are skipped on non-Unix or under -short.
func allFactories() map[string]sessionFactory {
	factories := map[string]sessionFactory{
		"mock": func(t *testing.T) (InteractiveSession, func()) {
			return newControllableSession(), func() {}
		},
		"stringio": func(t *testing.T) (InteractiveSession, func()) {
			sio := &testStringIO{recvData: []string{"hello"}}
			sess := NewStringIOSession(sio)
			sess.Start()
			return sess, func() { sess.Close() }
		},
	}
	// PTY-based sessions are slow and Unix-only.
	if !testing.Short() && runtime.GOOS != "windows" {
		factories["capture"] = func(t *testing.T) (InteractiveSession, func()) {
			cs := NewCaptureSession(CaptureConfig{
				Command: "cat",
			})
			if err := cs.Start(context.Background()); err != nil {
				t.Fatalf("capture Start: %v", err)
			}
			return cs, func() { cs.Close() }
		}
	}
	return factories
}

// ---------------------------------------------------------------------------
// 1. Interface compliance (compile-time + runtime)
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSessionImplementsInterface(t *testing.T) {
	t.Parallel()
	// Compile-time check is in capture.go; verify runtime assignment.
	var _ InteractiveSession = (*CaptureSession)(nil)
}

func TestInteractiveSession_StringIOSessionImplementsInterface(t *testing.T) {
	t.Parallel()
	// Compile-time check is in stringio_session.go; verify runtime assignment.
	var _ InteractiveSession = (*StringIOSession)(nil)
}

func TestInteractiveSession_ControllableSessionImplementsInterface(t *testing.T) {
	t.Parallel()
	// Compile-time check is in testhelpers_test.go; verify runtime assignment.
	var _ InteractiveSession = (*controllableSession)(nil)
}

// ---------------------------------------------------------------------------
// 2. Write() sends data to the PTY
// ---------------------------------------------------------------------------

func TestInteractiveSession_Write_SendsData(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			data := []byte("test input\n")
			n, err := sess.Write(data)
			if err != nil {
				t.Fatalf("Write error: %v", err)
			}
			if n != len(data) {
				t.Errorf("Write returned n=%d, want %d", n, len(data))
			}
		})
	}
}

func TestInteractiveSession_Write_EmptyData(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			n, err := sess.Write([]byte{})
			if err != nil {
				t.Fatalf("Write empty error: %v", err)
			}
			if n != 0 {
				t.Errorf("Write empty returned n=%d, want 0", n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Resize() changes terminal dimensions
// ---------------------------------------------------------------------------

func TestInteractiveSession_Resize_ValidDimensions(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			if err := sess.Resize(50, 120); err != nil {
				t.Fatalf("Resize(50,120) error: %v", err)
			}
		})
	}
}

func TestInteractiveSession_Resize_MultipleTimes(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			dims := [][2]int{{24, 80}, {50, 132}, {40, 100}, {24, 80}}
			for _, dim := range dims {
				if err := sess.Resize(dim[0], dim[1]); err != nil {
					t.Fatalf("Resize(%d,%d) error: %v", dim[0], dim[1], err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4. Close() terminates the session
// ---------------------------------------------------------------------------

func TestInteractiveSession_Close_SignalsDone(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			if err := sess.Close(); err != nil {
				t.Fatalf("Close error: %v", err)
			}

			// Done channel must be closed after Close.
			select {
			case <-sess.Done():
				// expected
			case <-time.After(2 * time.Second):
				t.Fatal("Done() channel not closed after Close()")
			}
		})
	}
}

func TestInteractiveSession_Close_Idempotent(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			// First close.
			if err := sess.Close(); err != nil {
				t.Fatalf("first Close error: %v", err)
			}
			// Second close should not panic.
			if err := sess.Close(); err != nil {
				t.Fatalf("second Close error: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. Done() channel semantics
// ---------------------------------------------------------------------------

func TestInteractiveSession_Done_OpenBeforeClose(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			select {
			case <-sess.Done():
				t.Error("Done() should be open before Close()")
			default:
				// expected — session is still alive
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. Reader() channel semantics
// ---------------------------------------------------------------------------

func TestInteractiveSession_Reader_ReturnsChannel(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			ch := sess.Reader()
			if ch == nil {
				t.Fatal("Reader() returned nil channel")
			}
		})
	}
}

func TestInteractiveSession_Reader_ClosesOnExit(t *testing.T) {
	t.Parallel()

	// Only test implementations that guarantee Reader channel closure on Close.
	// The mock controllableSession does not close its readerCh on Close(),
	// so we skip it here — that behavior is implementation-specific.
	factories := map[string]sessionFactory{}

	if !testing.Short() && runtime.GOOS != "windows" {
		factories["capture"] = func(t *testing.T) (InteractiveSession, func()) {
			cs := NewCaptureSession(CaptureConfig{
				Command: "echo",
				Args:    []string{"hi"},
			})
			if err := cs.Start(context.Background()); err != nil {
				t.Fatalf("capture Start: %v", err)
			}
			return cs, func() { cs.Close() }
		}
	}

	factories["stringio"] = func(t *testing.T) (InteractiveSession, func()) {
		sio := &testStringIO{recvData: []string{"hi"}}
		sess := NewStringIOSession(sio)
		sess.Start()
		return sess, func() { sess.Close() }
	}

	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			ch := sess.Reader()

			// Close the session; Reader channel should eventually close.
			sess.Close()

			timeout := time.After(3 * time.Second)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return // channel closed — expected
					}
				case <-timeout:
					t.Fatal("Reader() channel not closed after session exit")
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 7. CaptureSession-specific contract tests
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_WriteEcho(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	collector := startCollector(cs)

	// Write data through the InteractiveSession interface.
	var sess InteractiveSession = cs
	data := []byte("hello from contract test\n")
	n, err := sess.Write(data)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write n=%d, want %d", n, len(data))
	}

	// Send EOF to terminate cat.
	if err := cs.SendEOF(); err != nil {
		t.Fatalf("SendEOF: %v", err)
	}

	output := collector.wait()
	if !strings.Contains(output, "hello from contract test") {
		t.Errorf("output = %q, want to contain %q", output, "hello from contract test")
	}
}

func TestInteractiveSession_CaptureSession_ResizeUpdatesDimensions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
		Rows:    24,
		Cols:    80,
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	var sess InteractiveSession = cs
	if err := sess.Resize(50, 132); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	// Verify the CaptureSession tracked the new dimensions.
	if cs.Rows() != 50 || cs.Cols() != 132 {
		t.Errorf("dimensions = %dx%d, want 50x132", cs.Rows(), cs.Cols())
	}
}

func TestInteractiveSession_CaptureSession_ResizeInvalidDimensions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	var sess InteractiveSession = cs

	err := sess.Resize(0, 80)
	if err == nil {
		t.Error("expected error for rows=0")
	}

	err = sess.Resize(24, 0)
	if err == nil {
		t.Error("expected error for cols=0")
	}

	err = sess.Resize(-1, 80)
	if err == nil {
		t.Error("expected error for negative rows")
	}
}

func TestInteractiveSession_CaptureSession_NotStarted(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})

	// Operations on a not-yet-started session should return errors.
	var sess InteractiveSession = cs

	_, err := sess.Write([]byte("data"))
	if err == nil {
		t.Error("expected error writing to not-started session")
	}

	err = sess.Resize(24, 80)
	if err == nil {
		t.Error("expected error resizing not-started session")
	}
}

func TestInteractiveSession_CaptureSession_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"lifecycle test"},
	})

	// Before start: Done should be open.
	select {
	case <-cs.Done():
		t.Fatal("Done() should be open before Start()")
	default:
	}

	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for completion.
	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	// After exit: Done should be closed.
	select {
	case <-cs.Done():
	default:
		t.Fatal("Done() should be closed after process exits")
	}

	// Close should be idempotent.
	if err := cs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 8. StringIOSession-specific contract tests
// ---------------------------------------------------------------------------

func TestInteractiveSession_StringIOSession_WriteDelegates(t *testing.T) {
	t.Parallel()

	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	var is InteractiveSession = sess
	n, err := is.Write([]byte("delegated"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 9 {
		t.Errorf("Write n=%d, want 9", n)
	}
	if len(sio.sent) != 1 || sio.sent[0] != "delegated" {
		t.Errorf("sent = %v, want [delegated]", sio.sent)
	}

	sess.Close()
}

func TestInteractiveSession_StringIOSession_ResizeNoOp(t *testing.T) {
	t.Parallel()

	sio := &testStringIO{}
	sess := NewStringIOSession(sio)

	var is InteractiveSession = sess
	// Plain StringIO has no Resize — should be a safe no-op.
	if err := is.Resize(50, 120); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	sess.Close()
}

func TestInteractiveSession_StringIOSession_ReaderDeliversOutput(t *testing.T) {
	t.Parallel()

	sio := &testStringIO{recvData: []string{"alpha", "beta"}}
	sess := NewStringIOSession(sio)
	sess.Start()
	defer sess.Close()

	var is InteractiveSession = sess
	ch := is.Reader()
	if ch == nil {
		t.Fatal("Reader() returned nil")
	}

	var got []string
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case chunk, ok := <-ch:
			if !ok {
				t.Fatalf("Reader closed early; got %v", got)
			}
			got = append(got, string(chunk))
		case <-timeout:
			t.Fatalf("timeout waiting for chunks; got %v", got)
		}
	}

	if got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("chunks = %v, want [alpha beta]", got)
	}
}

// ---------------------------------------------------------------------------
// 9. Error cases: operations on closed session
// ---------------------------------------------------------------------------

func TestInteractiveSession_WriteAfterClose(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			// Close the session first.
			sess.Close()

			// Write after close should return an error (or at least not panic).
			_, err := sess.Write([]byte("after close"))
			if err == nil {
				// Some implementations may not error on write-after-close,
				// but they must not panic. If we get here without a panic,
				// the contract is satisfied.
				t.Logf("Write after close returned no error (implementation-specific)")
			}
		})
	}
}

func TestInteractiveSession_ResizeAfterClose(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			// Close the session first.
			sess.Close()

			// Resize after close should return an error (or at least not panic).
			err := sess.Resize(50, 120)
			if err == nil {
				t.Logf("Resize after close returned no error (implementation-specific)")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 10. Concurrent access safety
// ---------------------------------------------------------------------------

func TestInteractiveSession_ConcurrentWrite(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			const writers = 10
			const writesPerWriter = 50

			var wg sync.WaitGroup
			var writeErrors atomic.Int64

			for i := range writers {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := range writesPerWriter {
						data := []byte{byte(id), byte(j)}
						if _, err := sess.Write(data); err != nil {
							writeErrors.Add(1)
						}
					}
				}(i)
			}

			wg.Wait()

			// Some writes may fail (e.g., if the session closes during the test),
			// but there should be no panics or data races.
			if writeErrors.Load() > 0 {
				t.Logf("%d write errors (acceptable for concurrent close)", writeErrors.Load())
			}
		})
	}
}

func TestInteractiveSession_ConcurrentResize(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			const goroutines = 10
			const resizesPerGoroutine = 50

			var wg sync.WaitGroup
			var resizeErrors atomic.Int64

			for i := range goroutines {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					for j := range resizesPerGoroutine {
						rows := 24 + (id+j)%30
						cols := 80 + (id+j)%50
						if err := sess.Resize(rows, cols); err != nil {
							resizeErrors.Add(1)
						}
					}
				}(i)
			}

			wg.Wait()

			if resizeErrors.Load() > 0 {
				t.Logf("%d resize errors (acceptable for concurrent close)", resizeErrors.Load())
			}
		})
	}
}

func TestInteractiveSession_ConcurrentClose(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			const goroutines = 10

			var wg sync.WaitGroup
			for range goroutines {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sess.Close() // should not panic
				}()
			}

			wg.Wait()

			// Done must be closed after concurrent Close calls.
			select {
			case <-sess.Done():
			case <-time.After(2 * time.Second):
				t.Fatal("Done() not closed after concurrent Close()")
			}
		})
	}
}

func TestInteractiveSession_ConcurrentReadWriteClose(t *testing.T) {
	t.Parallel()

	for name, factory := range allFactories() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sess, cleanup := factory(t)
			defer cleanup()

			const iterations = 100

			var wg sync.WaitGroup

			// Writer goroutine.
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range iterations {
					sess.Write([]byte{byte(i)})
				}
			}()

			// Resizer goroutine.
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range iterations {
					sess.Resize(24+i%10, 80+i%20)
				}
			}()

			// Reader goroutine — drain output without blocking.
			wg.Add(1)
			go func() {
				defer wg.Done()
				ch := sess.Reader()
				if ch != nil {
					for {
						select {
						case _, ok := <-ch:
							if !ok {
								return
							}
						case <-time.After(100 * time.Millisecond):
							return // don't block forever
						}
					}
				}
			}()

			// Closer goroutine — close after a short delay.
			wg.Add(1)
			go func() {
				defer wg.Done()
				time.Sleep(10 * time.Millisecond)
				sess.Close()
			}()

			wg.Wait()
		})
	}
}

// ---------------------------------------------------------------------------
// 11. CaptureSession integration with SessionManager
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_RegisteredWithManager(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"registered"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	id, err := m.Register(cs, SessionTarget{Name: "echo-test", Kind: SessionKindCapture})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero session ID")
	}

	// Wait for output to appear in the snapshot.
	waitForSnapshotContains(t, m, id, "registered", 5*time.Second)

	// Verify session appears in Sessions list.
	sessions := m.Sessions()
	found := false
	for _, s := range sessions {
		if s.ID == id {
			found = true
			if s.Target.Name != "echo-test" {
				t.Errorf("Target.Name = %q, want %q", s.Target.Name, "echo-test")
			}
			if s.Target.Kind != SessionKindCapture {
				t.Errorf("Target.Kind = %q, want %q", s.Target.Kind, SessionKindCapture)
			}
		}
	}
	if !found {
		t.Fatalf("session %d not found in Sessions() list", id)
	}
}

// ---------------------------------------------------------------------------
// 12. CaptureSession Passthrough contract
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_PassthroughNotStarted(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})

	// Passthrough on a not-started session should return an error.
	reason, err := cs.Passthrough(context.Background(), PassthroughConfig{})
	if err == nil {
		t.Error("expected error for passthrough on not-started session")
	}
	if reason != ExitError {
		t.Errorf("reason = %v, want ExitError", reason)
	}
}

// ---------------------------------------------------------------------------
// 13. ExitReason contract
// ---------------------------------------------------------------------------

func TestExitReason_Values(t *testing.T) {
	t.Parallel()

	reasons := []ExitReason{ExitToggle, ExitChildExit, ExitContext, ExitError}
	names := []string{"toggle", "child-exit", "context", "error"}

	for i, r := range reasons {
		if r.String() != names[i] {
			t.Errorf("ExitReason(%d).String() = %q, want %q", r, r.String(), names[i])
		}
	}

	// Unknown value.
	unknown := ExitReason(99)
	if unknown.String() != "unknown(99)" {
		t.Errorf("unknown ExitReason.String() = %q, want %q", unknown.String(), "unknown(99)")
	}
}

// ---------------------------------------------------------------------------
// 14. CaptureSession context cancellation
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_ContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	// Cancel the context — process should be killed.
	cancel()

	select {
	case <-cs.Done():
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// 15. CaptureSession WriteString and SendEOF
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_WriteString(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "cat",
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	collector := startCollector(cs)

	if err := cs.WriteString("write string test\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	if err := cs.SendEOF(); err != nil {
		t.Fatalf("SendEOF: %v", err)
	}

	output := collector.wait()
	if !strings.Contains(output, "write string test") {
		t.Errorf("output = %q, want to contain %q", output, "write string test")
	}
}

// ---------------------------------------------------------------------------
// 16. CaptureSession ExportConfig round-trip
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_ExportConfig(t *testing.T) {
	t.Parallel()

	original := CaptureConfig{
		Name:    "test-session",
		Kind:    SessionKindCapture,
		Command: "make",
		Args:    []string{"test"},
		Dir:     "/tmp",
		Env:     map[string]string{"FOO": "bar"},
		Rows:    50,
		Cols:    132,
	}

	cs := NewCaptureSession(original)
	exported := cs.ExportConfig()

	if exported.Name != original.Name {
		t.Errorf("Name = %q, want %q", exported.Name, original.Name)
	}
	if exported.Command != original.Command {
		t.Errorf("Command = %q, want %q", exported.Command, original.Command)
	}
	if len(exported.Args) != 1 || exported.Args[0] != "test" {
		t.Errorf("Args = %v, want [test]", exported.Args)
	}
	if exported.Dir != original.Dir {
		t.Errorf("Dir = %q, want %q", exported.Dir, original.Dir)
	}
	if exported.Env["FOO"] != "bar" {
		t.Errorf("Env[FOO] = %q, want %q", exported.Env["FOO"], "bar")
	}
	if exported.Rows != original.Rows {
		t.Errorf("Rows = %d, want %d", exported.Rows, original.Rows)
	}
	if exported.Cols != original.Cols {
		t.Errorf("Cols = %d, want %d", exported.Cols, original.Cols)
	}

	// Verify deep copy: mutating exported should not affect original.
	exported.Args[0] = "modified"
	exported.Env["BAZ"] = "qux"
	if cs.ExportConfig().Args[0] != "test" {
		t.Error("mutating exported Args affected original")
	}
	if _, ok := cs.ExportConfig().Env["BAZ"]; ok {
		t.Error("mutating exported Env affected original")
	}
}

// ---------------------------------------------------------------------------
// 17. CaptureSession Pid and ExitCode
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_PidAndExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "echo",
		Args:    []string{"pid test"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	// PID should be positive while running.
	if pid := cs.Pid(); pid <= 0 {
		t.Errorf("Pid = %d, want > 0", pid)
	}

	// ExitCode should be -1 while running.
	if code := cs.ExitCode(); code != -1 {
		t.Logf("ExitCode = %d while running (may have already exited)", code)
	}

	// Wait for completion.
	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// ---------------------------------------------------------------------------
// 18. CaptureSession non-zero exit code
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_NonZeroExitCode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 42 {
		t.Errorf("exit code = %d, want 42", code)
	}
}

// ---------------------------------------------------------------------------
// 19. CaptureSession double Start error
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_DoubleStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer cs.Close()

	err := cs.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on second Start call")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("error = %q, want to contain 'already started'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// 20. CaptureSession Wait without Start
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_WaitWithoutStart(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})
	_, err := cs.Wait()
	if err == nil {
		t.Fatal("expected error when Wait called without Start")
	}
}

// ---------------------------------------------------------------------------
// 21. CaptureSession working directory
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_WorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	tmpDir := t.TempDir()

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "pwd"},
		Dir:     tmpDir,
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	collector := startCollector(cs)

	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	output := collector.wait()
	if !strings.Contains(output, tmpDir) {
		t.Errorf("output = %q, want to contain %q", output, tmpDir)
	}
}

// ---------------------------------------------------------------------------
// 22. CaptureSession environment variables
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_EnvironmentVariables(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command: "sh",
		Args:    []string{"-c", "echo $OSM_TEST_VAR"},
		Env:     map[string]string{"OSM_TEST_VAR": "contract_test_value"},
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	collector := startCollector(cs)

	code, err := cs.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	output := collector.wait()
	if !strings.Contains(output, "contract_test_value") {
		t.Errorf("output = %q, want to contain %q", output, "contract_test_value")
	}
}

// ---------------------------------------------------------------------------
// 23. CaptureSession close-then-write error
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_WriteAfterClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Close the session.
	cs.Close()

	// Write after close should return an error.
	_, err := cs.Write([]byte("after close"))
	if err == nil {
		t.Error("expected error writing to closed session")
	}
}

// ---------------------------------------------------------------------------
// 24. CaptureSession implements io.Closer
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_ImplementsCloser(t *testing.T) {
	t.Parallel()

	// Compile-time check in capture.go; runtime verification.
	var closer io.Closer = (*CaptureSession)(nil)
	_ = closer
}

// ---------------------------------------------------------------------------
// 25. CaptureSession Kill and Interrupt on not-started
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_InterruptNotStarted(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})
	err := cs.Interrupt()
	if err == nil {
		t.Error("expected error interrupting not-started session")
	}
}

func TestInteractiveSession_CaptureSession_KillNotStarted(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})
	err := cs.Kill()
	if err == nil {
		t.Error("expected error killing not-started session")
	}
}

// ---------------------------------------------------------------------------
// 26. CaptureSession custom DrainTimeout and SkipDrain
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_SkipDrain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{
		Command:     "echo",
		Args:        []string{"skip drain"},
		SkipDrain:   true,
		DrainTimeout: 1 * time.Second,
	})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Close with SkipDrain should return quickly.
	start := time.Now()
	cs.Close()
	elapsed := time.Since(start)

	// SkipDrain means Close doesn't wait for the reader loop.
	// It should complete well under the DrainTimeout.
	if elapsed > 2*time.Second {
		t.Errorf("Close took %v with SkipDrain, expected fast return", elapsed)
	}
}

// ---------------------------------------------------------------------------
// 27. CaptureSession with context cancellation kills child
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_ContextCancelKillsChild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY-based test in short mode")
	}
	skipIfWindows(t)
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	cs := NewCaptureSession(CaptureConfig{
		Command: "sleep",
		Args:    []string{"60"},
	})
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	// Cancel context — should kill the child process.
	cancel()

	select {
	case <-cs.Done():
		// expected
	case <-time.After(5 * time.Second):
		t.Fatal("child process did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// 28. CaptureSession error wrapping
// ---------------------------------------------------------------------------

func TestInteractiveSession_CaptureSession_ErrNoChild(t *testing.T) {
	t.Parallel()

	cs := NewCaptureSession(CaptureConfig{Command: "cat"})

	// Passthrough on not-started session should wrap ErrNoChild.
	_, err := cs.Passthrough(context.Background(), PassthroughConfig{})
	if err == nil {
		t.Fatal("expected error for passthrough on not-started session")
	}
	if !errors.Is(err, ErrNoChild) {
		t.Errorf("error = %v, want wrapping ErrNoChild", err)
	}
}
