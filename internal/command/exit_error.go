package command

import (
	"errors"
	"fmt"
)

// ExitError carries the status a command wants the process to exit with. It
// exists for tools whose own status is meaningful to the caller and must be
// preserved rather than flattened to a generic failure: a supervised tool that
// exits 7 makes the command exit 7.
type ExitError struct {
	Code int
	Err  error
}

// Error describes the propagated failure.
func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// Unwrap returns the underlying error for errors.Is/errors.As chains.
func (e *ExitError) Unwrap() error {
	return e.Err
}

// ExitCode reports the status an error asks the process to exit with, if any.
func ExitCode(err error) (int, bool) {
	if target, ok := errors.AsType[*ExitError](err); ok {
		return target.Code, true
	}
	return 0, false
}
