//go:build windows

package ptyio

// WindowsBlockingGuard is a no-op. Windows does not use O_NONBLOCK.
type WindowsBlockingGuard struct{}

var _ BlockingGuard = WindowsBlockingGuard{}

func (WindowsBlockingGuard) EnsureBlocking(fd int) (origFlags int, err error) { return 0, nil }
func (WindowsBlockingGuard) Restore(fd int, origFlags int)                    {}
