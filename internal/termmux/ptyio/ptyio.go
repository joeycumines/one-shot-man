package ptyio

import (
	"io"

	"golang.org/x/term"
)

// PTY abstracts a pseudo-terminal file descriptor.
type PTY interface {
	io.ReadWriteCloser
	Fd() uintptr
	Resize(rows, cols uint16) error
}

// BlockingGuard ensures a file descriptor is in blocking mode.
// Go's os.File.Read does NOT handle EAGAIN for TTY fds.
type BlockingGuard interface {
	EnsureBlocking(fd int) (origFlags int, err error)
	Restore(fd int, origFlags int)
}

// TermState abstracts terminal state operations for testability.
type TermState interface {
	MakeRaw(fd int) (*term.State, error)
	Restore(fd int, state *term.State) error
	GetSize(fd int) (width, height int, err error)
}

// RealTermState delegates to golang.org/x/term.
type RealTermState struct{}

var _ TermState = RealTermState{}

func (RealTermState) MakeRaw(fd int) (*term.State, error)           { return term.MakeRaw(fd) }
func (RealTermState) Restore(fd int, state *term.State) error       { return term.Restore(fd, state) }
func (RealTermState) GetSize(fd int) (width, height int, err error) { return term.GetSize(fd) }
