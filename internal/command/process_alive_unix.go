//go:build !windows

package command

import (
	"errors"
	"syscall"
)

// processAlive reports whether a process with the given PID is running.
// On Unix, this sends signal 0 which checks existence without affecting
// the process.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
