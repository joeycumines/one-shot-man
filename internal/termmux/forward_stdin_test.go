package termmux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestForwardStdin_ToggleKey(t *testing.T) {
	var written bytes.Buffer
	stdin := strings.NewReader("hello\x1dworld") // 0x1d = Ctrl+]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan forwardResult, 1)
	go forwardStdin(ctx, resultCh, forwardConfig{
		Stdin:     stdin,
		Writer:    &written,
		ToggleKey: 0x1d,
	})

	select {
	case r := <-resultCh:
		if r.reason != ExitToggle {
			t.Errorf("reason: got %v, want ExitToggle", r.reason)
		}
		if r.err != nil {
			t.Errorf("err: got %v, want nil", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for toggle key")
	}

	if got := written.String(); got != "hello" {
		t.Errorf("written: got %q, want %q", got, "hello")
	}
}

func TestForwardStdin_WriteError(t *testing.T) {
	errWriter := &errorWriter{err: errors.New("write failed")}
	stdin := strings.NewReader("data")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan forwardResult, 1)
	go forwardStdin(ctx, resultCh, forwardConfig{
		Stdin:     stdin,
		Writer:    errWriter,
		ToggleKey: 0x1d,
	})

	select {
	case r := <-resultCh:
		if r.reason != ExitError {
			t.Errorf("reason: got %v, want ExitError", r.reason)
		}
		if r.err == nil {
			t.Error("expected non-nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for write error")
	}
}

func TestForwardStdin_ContextCancel(t *testing.T) {
	// stdin that blocks until done is closed, and signals when the read is
	// actually in flight so the test never depends on a sleep.
	stdin := &neverReader{done: make(chan struct{}), reading: make(chan struct{})}
	var written bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan forwardResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		forwardStdin(ctx, resultCh, forwardConfig{
			Stdin:     stdin,
			Writer:    &written,
			ToggleKey: 0x1d,
		})
	}()

	// Cancel only once the forwarder is provably blocked in Read, so the
	// assertion below is about behavior rather than about how fast this
	// machine schedules goroutines.
	select {
	case <-stdin.reading:
	case <-time.After(2 * time.Second):
		t.Fatal("forwardStdin never entered the blocking read")
	}
	cancel()

	// A cancelled context must not release a reader that ignores it: the
	// forwarder stays blocked until the reader is released below.
	select {
	case <-done:
		t.Fatal("forwardStdin returned before the blocking reader was released")
	default:
	}

	// Unblock the reader so the forwardStdin goroutine can exit.
	close(stdin.done)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("forwardStdin did not exit after its reader was released")
	}

	select {
	case r := <-resultCh:
		t.Errorf("unexpected result: %v", r)
	default:
	}
}

func TestForwardStdin_ContextCancelClosesFallbackReader(t *testing.T) {
	stdin := newBlockingReadCloser()
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan forwardResult, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		forwardStdin(ctx, resultCh, forwardConfig{
			Stdin:     stdin,
			Writer:    io.Discard,
			ToggleKey: 0x1d,
		})
	}()

	select {
	case <-stdin.started:
	case <-time.After(time.Second):
		t.Fatal("forwardStdin did not enter the blocking read")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("forwardStdin did not join after cancellation")
	}
	if !stdin.wasClosed() {
		t.Fatal("fallback reader was not closed on cancellation")
	}
	select {
	case r := <-resultCh:
		t.Errorf("unexpected result: %v", r)
	default:
	}
}

func TestForwardStdin_PreProcess(t *testing.T) {
	var written bytes.Buffer
	stdin := strings.NewReader("abc\x1b[<0;10;24Mxyz") // SGR click on status bar row 24

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	preProcessCalls := 0
	resultCh := make(chan forwardResult, 1)
	go forwardStdin(ctx, resultCh, forwardConfig{
		Stdin:     stdin,
		Writer:    &written,
		ToggleKey: 0x1d,
		PreProcess: func(data []byte, carry []byte) ([]byte, []byte, bool) {
			preProcessCalls++
			filtered, partial, clicked := filterMouseForStatusBar(data, 24, 1)
			return filtered, partial, clicked
		},
	})

	select {
	case r := <-resultCh:
		if r.reason != ExitToggle {
			t.Errorf("reason: got %v, want ExitToggle (from status bar click)", r.reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	if preProcessCalls == 0 {
		t.Error("expected PreProcess to be called")
	}
}

func TestForwardStdin_EOF(t *testing.T) {
	var written bytes.Buffer
	stdin := strings.NewReader("hello") // will EOF after reading all data

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	resultCh := make(chan forwardResult, 1)
	go func() {
		defer close(done)
		forwardStdin(ctx, resultCh, forwardConfig{
			Stdin:     stdin,
			Writer:    &written,
			ToggleKey: 0x1d,
		})
	}()

	// EOF should cause forwardStdin to exit without sending a result.
	select {
	case r := <-resultCh:
		t.Errorf("unexpected result on EOF: %v", r)
	case <-done:
		// Expected: goroutine returns silently on EOF.
	}

	if got := written.String(); got != "hello" {
		t.Errorf("written: got %q, want %q", got, "hello")
	}
}

func TestForwardStdin_CarryOverNoAlias(t *testing.T) {
	// Regression test: carry-over bytes from PreProcess must not alias the
	// shared read buffer. forwardStdin deep-copies carry to prevent corruption.
	var written bytes.Buffer

	// Use a reader that provides two chunks: first has a partial prefix,
	// second has the completion that triggers a "click".
	chunk1 := []byte("data\x1b[<0;10;2") // incomplete SGR
	chunk2 := []byte("4Mmore")           // completes y=24 → status bar click
	reader := &chunkReader{chunks: [][]byte{chunk1, chunk2}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan forwardResult, 1)
	go forwardStdin(ctx, resultCh, forwardConfig{
		Stdin:     reader,
		Writer:    &written,
		ToggleKey: 0x1d,
		PreProcess: func(data []byte, carry []byte) ([]byte, []byte, bool) {
			// Prepend carry.
			if len(carry) > 0 {
				data = append(carry, data...)
			}
			// Use the real filterMouseForStatusBar to exercise the
			// subslice-aliasing behavior.
			filtered, partial, clicked := filterMouseForStatusBar(data, 24, 1)
			return filtered, partial, clicked
		},
	})

	// Expect the status bar click to trigger ExitToggle.
	select {
	case r := <-resultCh:
		if r.reason != ExitToggle {
			t.Errorf("reason: got %v, want ExitToggle (from status bar click)", r.reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for click")
	}

	// "data" was forwarded from first read; "more" (trailing data after the
	// intercepted click sequence) is forwarded before the toggle exit.
	if got := written.String(); got != "datamore" {
		t.Errorf("written: got %q, want %q", got, "datamore")
	}
}

// chunkReader is an io.Reader that returns each chunk in sequence, then EOF.
type chunkReader struct {
	chunks [][]byte
	idx    int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	chunk := r.chunks[r.idx]
	n := copy(p, chunk)
	if n < len(chunk) {
		r.chunks[r.idx] = chunk[n:]
	} else {
		r.idx++
	}
	return n, nil
}

// errorWriter is an io.Writer that always returns an error.
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

// neverReader is an io.Reader that blocks until its done channel is closed.
type neverReader struct {
	done    chan struct{}
	reading chan struct{}
	once    sync.Once
}

func (r *neverReader) Read(p []byte) (int, error) {
	if r.reading != nil {
		r.once.Do(func() { close(r.reading) })
	}
	<-r.done
	return 0, io.EOF
}

type blockingReadCloser struct {
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.done
	return 0, io.EOF
}

func (r *blockingReadCloser) Close() error {
	select {
	case <-r.done:
		return nil
	default:
		close(r.done)
		return nil
	}
}

func (r *blockingReadCloser) wasClosed() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}
