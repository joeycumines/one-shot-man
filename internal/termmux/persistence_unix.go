//go:build !windows

package termmux

import (
	"errors"
	"syscall"
)

// processAlive checks for process existence on Unix using signal 0.
// Signal 0 does not deliver a real signal but verifies the process
// exists and the caller has permission to signal it. EPERM indicates
// the process exists but belongs to another user.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
