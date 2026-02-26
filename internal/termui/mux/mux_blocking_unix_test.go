//go:build !windows

package mux

import (
	"syscall"
	"testing"
)

func TestEnsureBlockingFd(t *testing.T) {
	t.Parallel()

	// Create a pipe — we can control fd flags on pipes.
	r, w, err := pipeFds()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = syscall.Close(r); _ = syscall.Close(w) }()

	// Set the read end to non-blocking.
	if err := syscall.SetNonblock(r, true); err != nil {
		t.Fatalf("SetNonblock: %v", err)
	}

	// Verify it IS non-blocking (read returns EAGAIN).
	var buf [1]byte
	_, err = syscall.Read(r, buf[:])
	if err != syscall.EAGAIN {
		t.Fatalf("expected EAGAIN before fix, got: %v", err)
	}

	// Apply ensureBlockingFd.
	origFlags, err := ensureBlockingFd(r)
	if err != nil {
		t.Fatalf("ensureBlockingFd: %v", err)
	}
	if origFlags&syscall.O_NONBLOCK == 0 {
		t.Error("origFlags should have O_NONBLOCK set")
	}

	// Now verify the fd is blocking by checking flags.
	flags, err := fcntlGetFlags(r)
	if err != nil {
		t.Fatalf("fcntlGetFlags: %v", err)
	}
	if flags&syscall.O_NONBLOCK != 0 {
		t.Error("O_NONBLOCK should be cleared after ensureBlockingFd")
	}

	// Restore and verify original state.
	restoreBlockingFd(r, origFlags)
	flags, err = fcntlGetFlags(r)
	if err != nil {
		t.Fatalf("fcntlGetFlags after restore: %v", err)
	}
	if flags&syscall.O_NONBLOCK == 0 {
		t.Error("O_NONBLOCK should be restored after restoreBlockingFd")
	}
}

func TestEnsureBlockingFd_AlreadyBlocking(t *testing.T) {
	t.Parallel()

	r, w, err := pipeFds()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = syscall.Close(r); _ = syscall.Close(w) }()

	// fd starts in blocking mode — ensureBlockingFd should be a no-op.
	origFlags, err := ensureBlockingFd(r)
	if err != nil {
		t.Fatalf("ensureBlockingFd: %v", err)
	}
	if origFlags&syscall.O_NONBLOCK != 0 {
		t.Error("origFlags should NOT have O_NONBLOCK for a blocking fd")
	}

	// Verify still blocking.
	flags, err := fcntlGetFlags(r)
	if err != nil {
		t.Fatalf("fcntlGetFlags: %v", err)
	}
	if flags&syscall.O_NONBLOCK != 0 {
		t.Error("fd should still be blocking")
	}
}

// pipeFds creates a pipe and returns the raw file descriptors.
func pipeFds() (r, w int, err error) {
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return 0, 0, err
	}
	return fds[0], fds[1], nil
}
