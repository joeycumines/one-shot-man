//go:build unix && !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package exec

import (
	osexec "os/exec"
)

func waitProcessBeforeReap(*osexec.Cmd) (bool, error) {
	return false, errProcessObservationUnsupported
}
