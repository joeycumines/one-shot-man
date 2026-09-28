package node

import (
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

// The windowsErrnoCode table uses raw numbers so the mapping compiles on
// every platform; this test pins those numbers to the real Win32 constants.
func TestWindowsErrnoConstants(t *testing.T) {
	cases := map[syscall.Errno]string{
		syscall.Errno(windows.ERROR_FILE_NOT_FOUND):    "ENOENT",
		syscall.Errno(windows.ERROR_PATH_NOT_FOUND):    "ENOENT",
		syscall.Errno(windows.ERROR_FILE_EXISTS):       "EEXIST",
		syscall.Errno(windows.ERROR_ALREADY_EXISTS):    "EEXIST",
		syscall.Errno(windows.ERROR_ACCESS_DENIED):     "EACCES",
		syscall.Errno(windows.ERROR_DIRECTORY):         "ENOTDIR",
		syscall.Errno(windows.ERROR_DIR_NOT_EMPTY):     "ENOTEMPTY",
		syscall.Errno(windows.ERROR_INVALID_PARAMETER): "EINVAL",
		syscall.Errno(windows.WSAECONNREFUSED):         "ECONNREFUSED",
		syscall.Errno(windows.WSAECONNRESET):           "ECONNRESET",
		syscall.Errno(windows.WSAECONNABORTED):         "ECONNABORTED",
		syscall.Errno(windows.WSAETIMEDOUT):            "ETIMEDOUT",
		syscall.Errno(windows.WSAEHOSTUNREACH):         "EHOSTUNREACH",
		syscall.Errno(windows.WSAENETUNREACH):          "ENETUNREACH",
		syscall.Errno(windows.WSAESHUTDOWN):            "EPIPE",
		syscall.Errno(windows.WSAEACCES):               "EACCES",
	}
	for errno, want := range cases {
		if got, ok := windowsErrnoCode(errno); !ok || got != want {
			t.Errorf("windowsErrnoCode(%d) = %q, %v; want %q, true", uint32(errno), got, ok, want)
		}
	}
}
