//go:build windows

package termmux

import "syscall"

// processAlive checks for process existence on Windows using OpenProcess.
// Signal 0 does not work on Windows; instead we attempt to open the
// process with PROCESS_QUERY_LIMITED_INFORMATION access rights.
func processAlive(pid int) bool {
	const processQueryLimitedInformation = 0x1000
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}