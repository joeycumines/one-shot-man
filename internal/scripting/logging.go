package scripting

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// TUILogger provides structured logging integrated with the TUI system.
// It differentiates between application logs and terminal output.
type TUILogger struct {
	logger    *slog.Logger
	handler   *TUILogHandler
	tuiWriter io.Writer
	// Optional sink used by the interactive TUI to enqueue output and flush
	// it at safe points in the render lifecycle. When set, PrintToTUI will
	// call this instead of writing directly to tuiWriter.
	sinkMu  sync.RWMutex
	tuiSink func(string)
}

// LogEntry represents a single log entry with metadata.
type LogEntry struct {
	Time    time.Time         `json:"time"`
	Level   slog.Level        `json:"level"`
	Message string            `json:"message"`
	Attrs   map[string]string `json:"attrs"`
	Source  string            `json:"source,omitempty"`
}

// NewTUILogger creates a new TUI-integrated logger with a specified log level.
// If logFile is not nil, logs will also be written to it in JSON format.
// The level parameter controls the minimum level of logs that will be processed.
func NewTUILogger(tuiWriter io.Writer, logFile io.Writer, maxEntries int, level slog.Level) *TUILogger {
	if maxEntries <= 0 {
		maxEntries = 1000
	}

	var jsonHandler slog.Handler
	if logFile != nil {
		jsonHandler = slog.NewJSONHandler(logFile, &slog.HandlerOptions{
			Level: level,
		})
	}

	handler := &TUILogHandler{
		entries:     make([]LogEntry, 0, maxEntries),
		maxSize:     maxEntries,
		mutex:       sync.RWMutex{},
		fileHandler: jsonHandler,
		level:       level,
	}

	logger := slog.New(handler)

	return &TUILogger{
		logger:    logger,
		handler:   handler,
		tuiWriter: tuiWriter,
	}
}

// TUILogHandler implements slog.Handler for TUI-integrated logging.
type TUILogHandler struct {
	entries     []LogEntry
	maxSize     int
	mutex       sync.RWMutex
	fileHandler slog.Handler // Optional handler for file logging (JSON)
	level       slog.Level   // Minimum level to log
}

// Enabled implements slog.Handler.
// It returns true if the given level is at or above the logger's minimum level.
// This method is thread-safe and does not modify state.
//
// Thread Safety: This method only reads the immutable level field, so it requires
// no locking. The level is set once at construction and never changed.
func (h *TUILogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle implements slog.Handler.
func (h *TUILogHandler) Handle(ctx context.Context, record slog.Record) error {
	h.mutex.Lock()

	attrs := make(map[string]string)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.String()
		return true
	})

	entry := LogEntry{
		Time:    record.Time,
		Level:   record.Level,
		Message: record.Message,
		Attrs:   attrs,
	}

	// Add source information if available
	if record.PC != 0 {
		// Extract source info from PC
		entry.Source = "scripting" // simplified for now
	}

	h.entries = append(h.entries, entry)

	// Maintain max size by removing oldest entries
	if len(h.entries) > h.maxSize {
		h.entries[0] = LogEntry{} // Release strings/maps for GC
		h.entries = h.entries[1:]
	}

	// Forward to file handler if configured
	if h.fileHandler != nil {
		// We can't hold the lock while calling the file handler as it might be slow
		// OR we can hold it if we're sure it's fast enough.
		// Detailed logging to file is IO-bound.
		// Standard slog handlers are thread-safe.
		// So we can unlock before calling file handler.
		h.mutex.Unlock()
		return h.fileHandler.Handle(ctx, record)
	}

	h.mutex.Unlock()
	return nil
}

// WithAttrs implements slog.Handler.
func (h *TUILogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// For simplicity, return the same handler
	// In a full implementation, this would create a new handler with the attributes
	return h
}

// WithGroup implements slog.Handler.
func (h *TUILogHandler) WithGroup(name string) slog.Handler {
	// For simplicity, return the same handler
	// In a full implementation, this would create a new handler with the group
	return h
}

// Debug logs a debug message.
func (l *TUILogger) Debug(msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(context.Background(), slog.LevelDebug, msg, attrs...)
}

// Info logs an info message.
func (l *TUILogger) Info(msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, msg, attrs...)
}

// Warn logs a warning message.
func (l *TUILogger) Warn(msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(context.Background(), slog.LevelWarn, msg, attrs...)
}

// Error logs an error message.
func (l *TUILogger) Error(msg string, attrs ...slog.Attr) {
	l.logger.LogAttrs(context.Background(), slog.LevelError, msg, attrs...)
}

// Printf logs a formatted message at info level.
func (l *TUILogger) Printf(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

// PrintToTUI prints a message directly to the terminal interface.
//
// Semantics:
//   - Always ensures the message ends with a trailing newline ("\n"), regardless of
//     whether the interactive TUI sink is set or not. This guarantees consistent
//     line-oriented output across both interactive and non-interactive modes.
//   - The decision of whether to route to the interactive sink or the fallback
//     writer is performed atomically with respect to SetTUISink by holding a
//     read-lock for the duration of the operation. This prevents a race where a
//     print that observed a nil sink writes directly to the writer after the TUI
//     has taken control of the terminal.
func (l *TUILogger) PrintToTUI(msg string) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}

	// Hold the read lock across sink selection AND the subsequent action so that
	// SetTUISink (which takes the write lock) cannot interleave between the
	// check and the write/call. This avoids corrupting the TUI output by writing
	// directly to the terminal after the TUI has started.
	l.sinkMu.RLock()
	defer l.sinkMu.RUnlock()

	if l.tuiSink != nil {
		l.tuiSink(msg)
		return
	}

	if l.tuiWriter != nil {
		_, _ = l.tuiWriter.Write([]byte(msg))
	}
}

// PrintfToTUI prints a formatted message directly to the terminal interface.
func (l *TUILogger) PrintfToTUI(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.PrintToTUI(msg)
}

// SetTUISink configures an optional sink used by the interactive TUI to
// integrate script output with the prompt render cycle. Pass nil to disable.
func (l *TUILogger) SetTUISink(sink func(string)) {
	l.sinkMu.Lock()
	defer l.sinkMu.Unlock()
	l.tuiSink = sink
}

// GetLogs returns all log entries.
func (l *TUILogger) GetLogs() []LogEntry {
	l.handler.mutex.RLock()
	defer l.handler.mutex.RUnlock()

	// Return a copy to prevent race conditions
	logs := make([]LogEntry, len(l.handler.entries))
	copy(logs, l.handler.entries)
	return logs
}

// GetRecentLogs returns the most recent N log entries.
func (l *TUILogger) GetRecentLogs(count int) []LogEntry {
	l.handler.mutex.RLock()
	defer l.handler.mutex.RUnlock()

	if count <= 0 || count > len(l.handler.entries) {
		count = len(l.handler.entries)
	}

	start := len(l.handler.entries) - count
	logs := make([]LogEntry, count)
	copy(logs, l.handler.entries[start:])
	return logs
}

// SearchLogs searches for log entries containing the given text.
func (l *TUILogger) SearchLogs(query string) []LogEntry {
	l.handler.mutex.RLock()
	defer l.handler.mutex.RUnlock()

	query = strings.ToLower(query)
	var matches []LogEntry

	for _, entry := range l.handler.entries {
		if strings.Contains(strings.ToLower(entry.Message), query) {
			matches = append(matches, entry)
			continue
		}

		// Also search in attributes
		for key, value := range entry.Attrs {
			if strings.Contains(strings.ToLower(key), query) ||
				strings.Contains(strings.ToLower(value), query) {
				matches = append(matches, entry)
				break
			}
		}
	}

	return matches
}

// ClearLogs removes all log entries.
func (l *TUILogger) ClearLogs() {
	l.handler.mutex.Lock()
	defer l.handler.mutex.Unlock()

	l.handler.entries = l.handler.entries[:0] // Clear slice while keeping capacity
}

// Logger returns the underlying slog.Logger.
// This can be used to set the global slog default via slog.SetDefault().
func (l *TUILogger) Logger() *slog.Logger {
	return l.logger
}
