//go:build windows

package mux

// ensureBlockingFd is a no-op on Windows. Windows does not use O_NONBLOCK
// on file descriptors; non-blocking I/O uses a different mechanism (overlapped I/O).
func ensureBlockingFd(fd int) (origFlags int, err error) {
	return 0, nil
}

// restoreBlockingFd is a no-op on Windows.
func restoreBlockingFd(fd int, origFlags int) {}
