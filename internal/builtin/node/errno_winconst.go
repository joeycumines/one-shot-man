package node

import (
	"syscall"
)

// windowsErrnoCode maps Windows-native error numbers to Node's UV-style code
// strings. The Go syscall package on Windows reports raw Win32 (ERROR_*) and
// Winsock (WSA*) values instead of the POSIX errno constants, so a switch on
// syscall.ENOENT/EEXIST/etc. never matches. The numeric values are the stable
// Win32 API constants; errno_windows_test.go asserts them against
// golang.org/x/sys/windows so a drift is caught at compile-test time.
func windowsErrnoCode(errno syscall.Errno) (string, bool) {
	switch errno {
	case 2: // ERROR_FILE_NOT_FOUND
		return "ENOENT", true
	case 3: // ERROR_PATH_NOT_FOUND
		return "ENOENT", true
	case 80: // ERROR_FILE_EXISTS
		return "EEXIST", true
	case 183: // ERROR_ALREADY_EXISTS
		return "EEXIST", true
	case 5: // ERROR_ACCESS_DENIED
		return "EACCES", true
	case 267: // ERROR_DIRECTORY
		return "ENOTDIR", true
	case 145: // ERROR_DIR_NOT_EMPTY
		return "ENOTEMPTY", true
	case 87: // ERROR_INVALID_PARAMETER
		return "EINVAL", true
	case 10061: // WSAECONNREFUSED
		return "ECONNREFUSED", true
	case 10054: // WSAECONNRESET
		return "ECONNRESET", true
	case 10053: // WSAECONNABORTED
		return "ECONNABORTED", true
	case 10060: // WSAETIMEDOUT
		return "ETIMEDOUT", true
	case 10065: // WSAEHOSTUNREACH
		return "EHOSTUNREACH", true
	case 10051: // WSAENETUNREACH
		return "ENETUNREACH", true
	case 10058: // WSAESHUTDOWN (closest to EPIPE for a shut-down socket)
		return "EPIPE", true
	case 10013: // WSAEACCES
		return "EACCES", true
	}
	return "", false
}
