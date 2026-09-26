//go:build unix && !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package exec

import (
	"errors"
	osexec "os/exec"
)

// errProcessObservationUnsupported marks platforms that cannot observe a
// child's exit state before reaping it. It lives here, beside its only user,
// so the platforms that do support observation do not carry a dead symbol.
var errProcessObservationUnsupported = errors.New("pre-reap process observation is unavailable on this platform")

func waitProcessBeforeReap(*osexec.Cmd) (bool, error) {
	return false, errProcessObservationUnsupported
}
