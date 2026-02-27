//go:build !windows

package ptyio

import (
	"syscall"
	"testing"
)

func TestUnixBlockingGuard_EnsureBlocking(t *testing.T) {
	t.Parallel()
	r, w, err := pipeFds()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = syscall.Close(r); _ = syscall.Close(w) }()

	if err := syscall.SetNonblock(r, true); err != nil {
		t.Fatalf("SetNonblock: %v", err)
	}

	var g UnixBlockingGuard
	origFlags, err := g.EnsureBlocking(r)
	if err != nil {
		t.Fatalf("EnsureBlocking: %v", err)
	}
	if origFlags&syscall.O_NONBLOCK == 0 {
		t.Error("origFlags should have O_NONBLOCK")
	}

	flags, err := fcntlGetFlags(r)
	if err != nil {
		t.Fatalf("fcntlGetFlags: %v", err)
	}
	if flags&syscall.O_NONBLOCK != 0 {
		t.Error("O_NONBLOCK should be cleared")
	}

	g.Restore(r, origFlags)
	flags, err = fcntlGetFlags(r)
	if err != nil {
		t.Fatalf("fcntlGetFlags after restore: %v", err)
	}
	if flags&syscall.O_NONBLOCK == 0 {
		t.Error("O_NONBLOCK should be restored")
	}
}

func TestUnixBlockingGuard_AlreadyBlocking(t *testing.T) {
	t.Parallel()
	r, w, err := pipeFds()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = syscall.Close(r); _ = syscall.Close(w) }()

	var g UnixBlockingGuard
	origFlags, err := g.EnsureBlocking(r)
	if err != nil {
		t.Fatalf("EnsureBlocking: %v", err)
	}
	if origFlags&syscall.O_NONBLOCK != 0 {
		t.Error("origFlags should NOT have O_NONBLOCK")
	}
}

func pipeFds() (r, w int, err error) {
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return 0, 0, err
	}
	return fds[0], fds[1], nil
}
