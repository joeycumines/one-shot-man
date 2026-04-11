//go:build !windows

package termmux

import "github.com/joeycumines/one-shot-man/internal/termmux/ptyio"

// DefaultBlockingGuard returns the platform-appropriate BlockingGuard.
func DefaultBlockingGuard() ptyio.BlockingGuard {
	return ptyio.UnixBlockingGuard{}
}
