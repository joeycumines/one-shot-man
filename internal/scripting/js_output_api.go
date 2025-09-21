package scripting

// JavaScript API functions for terminal output

// jsOutputPrint prints to terminal output.
func (e *Engine) jsOutputPrint(msg string) {
	e.logger.PrintToTUI(msg)
}

// jsOutputPrintf prints formatted text to terminal output.
func (e *Engine) jsOutputPrintf(format string, args ...interface{}) {
	e.logger.PrintfToTUI(format, args...)
}
