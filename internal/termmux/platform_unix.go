//go:build !windows

package termmux

import "github.com/joeycumines/one-shot-man/internal/termmux/ptyio"

func defaultBlockingGuard() ptyio.BlockingGuard {
	return ptyio.UnixBlockingGuard{}
}
