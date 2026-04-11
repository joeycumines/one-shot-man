package termmux

import (
	"io"
	"log/slog"
)

// writeOrLog writes data to w, logging at Debug level on failure.
// Terminal output failures typically mean the controlling terminal has
// been closed or disconnected — best-effort logging is appropriate.
func writeOrLog(w io.Writer, data []byte, context string) {
	if _, err := w.Write(data); err != nil {
		slog.Debug("terminal write failed", "error", err, "context", context)
	}
}
