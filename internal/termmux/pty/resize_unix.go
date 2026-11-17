//go:build !windows

package pty

// conPTYHandleOpen reports whether resize is backed by a live terminal. On
// Unix the PTY master fd always backs Resize (TIOCSWINSZ), so it is true
// whenever the process is not closed; the closed check itself lives in
// Process.Resize, so this reports true unconditionally here.
func (p *Process) conPTYHandleOpen() bool {
	return true
}
