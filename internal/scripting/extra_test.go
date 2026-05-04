package scripting

import "log/slog"

// test-only extensions of types

// GetLogStats returns statistics about the log entries.
func (l *TUILogger) GetLogStats() map[string]int {
	l.handler.shared.mutex.RLock()
	defer l.handler.shared.mutex.RUnlock()

	stats := map[string]int{
		"total": len(l.handler.shared.entries),
		"debug": 0,
		"info":  0,
		"warn":  0,
		"error": 0,
	}

	for _, entry := range l.handler.shared.entries {
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
