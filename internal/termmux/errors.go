package termmux

import "errors"

var (
	// ErrNoChild is returned when an operation requires an attached session
	// but none is present.
	ErrNoChild = errors.New("termmux: no child process attached")

	// ErrPassthroughActive is returned when an operation conflicts with
	// an active passthrough session (e.g. Detach during passthrough).
	ErrPassthroughActive = errors.New("termmux: passthrough is active")
)
