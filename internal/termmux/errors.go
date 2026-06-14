package termmux

import "errors"

var (
	ErrNoChild = errors.New("termmux: no child process attached")

	ErrPassthroughActive = errors.New("termmux: passthrough is active")

	ErrPauseNotSupported = errors.New("termmux: pause not supported on Windows: ConPTY does not support process suspension (SIGTSTP)")

	ErrResumeNotSupported = errors.New("termmux: resume not supported on Windows: ConPTY does not support process resumption (SIGCONT)")

	ErrPaneNotFound = errors.New("termmux: pane not found")
)
