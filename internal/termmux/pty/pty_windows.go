//go:build windows

package pty

import (
	"context"
)

// Spawn returns ErrNotSupported on Windows.
// ConPTY integration is planned for a follow-up task.
func Spawn(_ context.Context, cfg SpawnConfig) (*Process, error) {
	cfg.applyDefaults()
	return nil, ErrNotSupported
}

// platformResize returns ErrNotSupported on Windows.
func (p *Process) platformResize(_, _ uint16) error {
	return ErrNotSupported
}
