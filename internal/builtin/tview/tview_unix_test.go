//go:build unix

package tview

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// pipeTerm is a test helper that wraps an os.File to implement TerminalOps.
// It's used for testing the TcellAdapter with pipe file descriptors.
type pipeTerm struct{ f *os.File }

func (p *pipeTerm) Read(b []byte) (int, error)      { return p.f.Read(b) }
func (p *pipeTerm) Write(b []byte) (int, error)     { return 0, nil }
func (p *pipeTerm) Close() error                    { return p.f.Close() }
func (p *pipeTerm) Fd() uintptr                     { return p.f.Fd() }
func (p *pipeTerm) MakeRaw() (*term.State, error)   { return &term.State{}, nil }
func (p *pipeTerm) Restore(state *term.State) error { return nil }
func (p *pipeTerm) GetSize() (int, int, error)      { return 80, 24, nil }
func (p *pipeTerm) IsTerminal() bool                { return false }

// TestTcellAdapter_DrainPreservesFlags verifies that the Drain() method
// does not alter the file descriptor flags. This test uses Unix-specific
// system calls and is therefore only run on Unix platforms.
func TestTcellAdapter_DrainPreservesFlags(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	// Write some data so there's something to drain
	_, err = w.WriteString("hello")
	require.NoError(t, err)
	// Close the writer so reads will hit EOF once drained
	require.NoError(t, w.Close())

	origFlags, err := unix.FcntlInt(uintptr(r.Fd()), unix.F_GETFL, 0)
	require.NoError(t, err)

	pt := &pipeTerm{f: r}
	adapter := NewTcellAdapter(pt)
	require.NoError(t, adapter.Drain())

	gotFlags, err := unix.FcntlInt(uintptr(r.Fd()), unix.F_GETFL, 0)
	require.NoError(t, err)
	assert.Equal(t, origFlags, gotFlags)
}
