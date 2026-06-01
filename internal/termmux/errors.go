package termmux

import "errors"

var (
	// ErrNoChild is returned when an operation requires an attached session
	// but none is present.
	ErrNoChild = errors.New("termmux: no child process attached")

	// ErrPassthroughActive is returned when an operation conflicts with
	// an active passthrough session (e.g. Detach during passthrough).
	ErrPassthroughActive = errors.New("termmux: passthrough is active")

	// ErrPauseNotSupported is returned on Windows where ConPTY does not
	// support process suspension (SIGSTOP). Use IsPaused() to check state.
	ErrPauseNotSupported = errors.New("termmux: pause not supported on Windows: ConPTY does not support process suspension (SIGTSTP)")

	// ErrResumeNotSupported is returned on Windows where ConPTY does not
	// support process resumption (SIGCONT).
	ErrResumeNotSupported = errors.New("termmux: resume not supported on Windows: ConPTY does not support process resumption (SIGCONT)")
)
