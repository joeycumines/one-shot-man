package command

import "errors"

// SilentError wraps an error that has already been reported to the user
// (e.g., printed to stderr). When main() encounters a SilentError, it
// should exit with a non-zero code without printing the error again.
//
// This prevents duplicate error output when commands print meaningful
// context to stderr and also return an error for exit-code propagation.
type SilentError struct {
	Err error
}

// Error returns the underlying error message.
func (e *SilentError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "silent error"
}

// Unwrap returns the underlying error for errors.Is/errors.As chains.
func (e *SilentError) Unwrap() error {
	return e.Err
}

// IsSilent reports whether err (or any error in its chain) is a SilentError.
func IsSilent(err error) bool {
	var silent *SilentError
	return errors.As(err, &silent)
}
