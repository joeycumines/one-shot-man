package termmux

import "errors"

var (
	// ErrNoChild is returned when an operation requires an attached child
	// process but none is present.
	ErrNoChild = errors.New("termmux: no child process attached")

	// ErrAlreadyAttached is returned when Attach is called but a child
	// process is already attached.
	ErrAlreadyAttached = errors.New("termmux: child already attached")

	// ErrPassthroughActive is returned when an operation conflicts with
	// an active passthrough session (e.g. Detach during passthrough).
	ErrPassthroughActive = errors.New("termmux: passthrough is active")

	// ErrDetached is returned when the mux has been detached and cannot
	// perform the requested operation.
	ErrDetached = errors.New("termmux: mux is detached")
)
