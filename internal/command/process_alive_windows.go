//go:build windows

package command

// processAlive reports whether a process with the given PID is running.
// On Windows, there is no reliable way to check PID liveness without
// additional system calls. This implementation conservatively returns
// true, relying on the lock age timeout (syncConfigLockMaxAge) to
// detect stale locks instead.
func processAlive(pid int) bool {
	_ = pid
	return true
}
