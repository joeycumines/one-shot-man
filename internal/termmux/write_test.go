package termmux

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// failWriter is an io.Writer that always returns an error.
type failWriter struct{ err error }

func (fw failWriter) Write([]byte) (int, error) { return 0, fw.err }

// shortWriteWriter reports a short write: n < len(p) with a nil error, the
// shape a PTY write can produce when the kernel buffer is full.
type shortWriteWriter struct{ n int }

func (sw *shortWriteWriter) Write(p []byte) (int, error) {
	if sw.n < len(p) {
		return sw.n, nil
	}
	return len(p), nil
}

func TestWriteFull_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteFull(&buf, []byte("hello")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "hello" {
		t.Fatalf("buf = %q; want %q", buf.String(), "hello")
	}
}

func TestWriteFull_Error(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("disk gone")
	if err := WriteFull(failWriter{err: wantErr}, []byte("data")); !errors.Is(err, wantErr) {
		t.Fatalf("WriteFull error = %v; want %v", err, wantErr)
	}
}

// TestWriteFull_ShortWrite verifies a short write (n < len with nil error) is
// an error wrapping ErrShortWrite, never a silent truncation.
func TestWriteFull_ShortWrite(t *testing.T) {
	t.Parallel()
	err := WriteFull(&shortWriteWriter{n: 3}, []byte("0123456789"))
	if err == nil {
		t.Fatal("expected error on short write, got nil")
	}
	if !errors.Is(err, ErrShortWrite) {
		t.Fatalf("WriteFull short-write error = %v; want errors.Is(ErrShortWrite)", err)
	}
	// ErrShortWrite aliases io.ErrShortWrite so all write paths share one
	// error identity.
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("ErrShortWrite must alias io.ErrShortWrite; got %v", err)
	}
}

func TestWriteOrLog_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := writeOrLog(&buf, []byte("hello"), "test-ctx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "hello" {
		t.Fatalf("buf = %q; want %q", buf.String(), "hello")
	}
}

func TestWriteOrLog_Error_Returned(t *testing.T) {
	t.Parallel()
	wantErr := bytes.ErrTooLarge
	fw := failWriter{err: wantErr}
	gotErr := writeOrLog(fw, []byte("data"), "bell")
	if gotErr != wantErr {
		t.Fatalf("writeOrLog error = %v; want %v", gotErr, wantErr)
	}
}

func TestWriteOrLog_Error_Logged(t *testing.T) {
	// NOT parallel — mutates global slog default.
	var logBuf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	fw := failWriter{err: bytes.ErrTooLarge}
	_ = writeOrLog(fw, []byte("data"), "bell")

	got := logBuf.String()
	for _, want := range []string{"terminal write failed", "too large", "bell"} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q; got:\n%s", want, got)
		}
	}
}
