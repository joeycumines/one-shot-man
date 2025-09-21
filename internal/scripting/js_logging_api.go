package scripting

// JavaScript API functions for logging

// jsLogDebug logs a debug message.
func (e *Engine) jsLogDebug(msg string, attrs ...map[string]interface{}) {
	e.logger.Debug(msg)
}

// jsLogInfo logs an info message.
func (e *Engine) jsLogInfo(msg string, attrs ...map[string]interface{}) {
	e.logger.Info(msg)
}

// jsLogWarn logs a warning message.
func (e *Engine) jsLogWarn(msg string, attrs ...map[string]interface{}) {
	e.logger.Warn(msg)
}

// jsLogError logs an error message.
func (e *Engine) jsLogError(msg string, attrs ...map[string]interface{}) {
	e.logger.Error(msg)
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
