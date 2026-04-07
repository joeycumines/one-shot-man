package termmux

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// failWriter is an io.Writer that always returns an error.
type failWriter struct{ err error }

func (fw failWriter) Write([]byte) (int, error) { return 0, fw.err }

func TestWriteOrLog_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	writeOrLog(&buf, []byte("hello"), "test-ctx")
	if buf.String() != "hello" {
		t.Fatalf("buf = %q; want %q", buf.String(), "hello")
	}
}

func TestWriteOrLog_Error_Logged(t *testing.T) {
	// NOT parallel — mutates global slog default.
	var logBuf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	fw := failWriter{err: bytes.ErrTooLarge}
	writeOrLog(fw, []byte("data"), "bell")

	got := logBuf.String()
	for _, want := range []string{"terminal write failed", "too large", "bell"} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q; got:\n%s", want, got)
		}
	}
}
