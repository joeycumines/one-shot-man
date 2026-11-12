package termmux

import (
	"fmt"
	"io"
	"log/slog"
)

// ErrShortWrite is returned when a Write reports success but wrote fewer
// bytes than requested, which would silently truncate data delivered to a
// child PTY (possible when the kernel buffer is full). Every input path that
// writes to a child PTY goes through WriteFull so a short write is an error,
// never a silent truncation.
var ErrShortWrite = io.ErrShortWrite

// WriteFull writes data to w and reports an error on a short write (n < len
// with nil error), which PTY writes can produce when the kernel buffer is
// full. A silent short write would truncate input delivered to the child.
func WriteFull(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("%w: wrote %d of %d bytes", ErrShortWrite, n, len(data))
	}
	return nil
}

// writeOrLog writes data to w, logging at Warn level on failure and
// returning the error.
// Terminal output failures typically mean the controlling terminal has
// been closed or disconnected — these are visible in normal logging
// since they indicate a real problem (e.g., terminal disconnect during
// passthrough). Callers decide whether to treat the error as fatal.
func writeOrLog(w io.Writer, data []byte, context string) error {
	if _, err := w.Write(data); err != nil {
		slog.Warn("terminal write failed", "error", err, "context", context)
		return err
	}
	return nil
}
