//go:build windows

package command

import "syscall"

// processAlive reports whether a process with the given PID is running.
// On Windows, this uses OpenProcess with PROCESS_QUERY_LIMITED_INFORMATION
// to check process existence without needing elevated privileges.
func processAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}
