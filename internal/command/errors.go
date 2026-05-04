package command

import "errors"

// ErrUnexpectedArguments is returned when a command receives arguments
// it does not accept. It is typically wrapped in a SilentError because
// the human-readable message (including the offending arguments) has
// already been written to stderr.
var ErrUnexpectedArguments = errors.New("unexpected arguments")
