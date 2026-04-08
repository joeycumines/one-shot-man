//go:build !windows

package termmux

import (
	"os"
	"syscall"
)

// processAlive checks for process existence on Unix using signal 0.
// Signal 0 does not deliver a real signal but verifies the process
// exists and the caller has permission to signal it.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
