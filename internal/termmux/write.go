package termmux

import (
	"io"
	"log/slog"
)

// writeOrLog writes data to w, logging at Warn level on failure.
// Terminal output failures typically mean the controlling terminal has
// been closed or disconnected — these are visible in normal logging
// since they indicate a real problem (e.g., terminal disconnect during
// passthrough).
func writeOrLog(w io.Writer, data []byte, context string) {
	if _, err := w.Write(data); err != nil {
		slog.Warn("terminal write failed", "error", err, "context", context)
	}
}
