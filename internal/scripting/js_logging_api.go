package scripting

import (
	"log/slog"
)

// JavaScript API functions for logging

func mapToAttrs(maps []map[string]interface{}) []slog.Attr {
	var attrs []slog.Attr
	for _, m := range maps {
		for k, v := range m {
			attrs = append(attrs, slog.Any(k, v))
		}
	}
	return attrs
}

// jsLogDebug logs a debug message.
func (e *Engine) jsLogDebug(msg string, attrs ...map[string]interface{}) {
	e.logger.Debug(msg, mapToAttrs(attrs)...)
}

// jsLogInfo logs an info message.
func (e *Engine) jsLogInfo(msg string, attrs ...map[string]interface{}) {
	e.logger.Info(msg, mapToAttrs(attrs)...)
}

// jsLogWarn logs a warning message.
func (e *Engine) jsLogWarn(msg string, attrs ...map[string]interface{}) {
	e.logger.Warn(msg, mapToAttrs(attrs)...)
}

// jsLogError logs an error message.
func (e *Engine) jsLogError(msg string, attrs ...map[string]interface{}) {
	e.logger.Error(msg, mapToAttrs(attrs)...)
}

// jsLogPrintf logs a formatted message.
func (e *Engine) jsLogPrintf(format string, args ...interface{}) {
	e.logger.Printf(format, args...)
}

// jsGetLogs returns log entries.
func (e *Engine) jsGetLogs(count ...int) interface{} {
	if len(count) > 0 && count[0] > 0 {
		return e.logger.GetRecentLogs(count[0])
	}
	return e.logger.GetLogs()
}

// jsLogClear clears all logs.
func (e *Engine) jsLogClear() {
	e.logger.ClearLogs()
}

// jsLogSearch searches logs for a query.
func (e *Engine) jsLogSearch(query string) interface{} {
	return e.logger.SearchLogs(query)
}
