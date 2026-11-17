//go:build windows

package pty

// conPTYHandleOpen reports whether the process still holds an open ConPTY
// pseudoconsole handle, i.e. whether platformResize can succeed. After
// ClosePseudoConsole (or a failed console init) the handle is zero and
// ResizePseudoConsole would not be called with a live console.
func (p *Process) conPTYHandleOpen() bool {
	h, ok := p.cmd.(*windowsProcessHandle)
	if !ok {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conPTY != 0
}
