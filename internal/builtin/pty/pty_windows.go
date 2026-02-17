//go:build windows

package pty

import (
	"context"
	"os"
)

// windowsProcessHandle is a stub for Windows.
type windowsProcessHandle struct{}

func (h *windowsProcessHandle) Wait() error              { return ErrNotSupported }
func (h *windowsProcessHandle) Signal(_ os.Signal) error { return ErrNotSupported }
func (h *windowsProcessHandle) Pid() int                 { return 0 }

// Spawn returns ErrNotSupported on Windows.
// ConPTY integration is planned for a follow-up task.
func Spawn(_ context.Context, _ SpawnConfig) (*Process, error) {
	return nil, ErrNotSupported
}

// platformResize returns ErrNotSupported on Windows.
func (p *Process) platformResize(_, _ uint16) error {
	return ErrNotSupported
}
