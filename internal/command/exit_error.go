package command

import (
	"errors"
	"fmt"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// ExitError carries the process status a command wants to return to its caller.
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

func scriptExitError(engine *scripting.Engine) error {
	if code, ok := engine.ExitCode(); ok && code != 0 {
		return &SilentError{Err: &ExitError{Code: code}}
	}
	return nil
}
