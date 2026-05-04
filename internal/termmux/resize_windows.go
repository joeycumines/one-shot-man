//go:build windows

package termmux

import "context"

// watchResize is a no-op on Windows. Windows does not use SIGWINCH.
func watchResize(_ context.Context, _ int, _ interface {
	GetSize(fd int) (width, height int, err error)
}, _ func(rows, cols int)) {
}
