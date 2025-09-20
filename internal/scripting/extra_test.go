package scripting

import "log/slog"

// test-only extensions of types

// GetLogStats returns statistics about the log entries.
func (l *TUILogger) GetLogStats() map[string]int {
	l.handler.mutex.RLock()
	defer l.handler.mutex.RUnlock()

	stats := map[string]int{
		"total": len(l.handler.entries),
		"debug": 0,
		"info":  0,
		"warn":  0,
		"error": 0,
	}

	for _, entry := range l.handler.entries {
		switch entry.Level {
		case slog.LevelDebug:
			stats["debug"]++
		case slog.LevelInfo:
			stats["info"]++
		case slog.LevelWarn:
			stats["warn"]++
		case slog.LevelError:
			stats["error"]++
		}
	}

	return stats
}
