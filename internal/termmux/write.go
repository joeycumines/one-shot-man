package termmux

import (
	"io"
	"log/slog"
)

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
