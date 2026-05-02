package termmux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
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
	// stdin that blocks forever
	stdin := &neverReader{}
	var written bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan forwardResult, 1)
	go forwardStdin(ctx, resultCh, forwardConfig{
		Stdin:     stdin,
		Writer:    &written,
		ToggleKey: 0x1d,
	})

	// Cancel context after a short delay.
	time.AfterFunc(100*time.Millisecond, cancel)

	// forwardStdin should exit silently (no result sent).
	select {
	case r := <-resultCh:
		t.Errorf("unexpected result: %v", r)
	case <-time.After(500 * time.Millisecond):
		// Expected: no result sent.
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

// neverReader is an io.Reader that blocks forever.
type neverReader struct{}

func (r *neverReader) Read(p []byte) (int, error) {
	select {} // block forever
}
