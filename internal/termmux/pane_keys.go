package termmux

// PaneAction represents an action triggered by a pane navigation key binding.
type PaneAction int

const (
	PaneActionNone PaneAction = iota
	PaneFocusLeft
	PaneFocusRight
	PaneFocusUp
	PaneFocusDown
	PaneSplitH
	PaneSplitV
	PaneClose
	PaneCloseConfirm
)

// PaneKeyHandler detects pane navigation key sequences in stdin data and
// returns the corresponding action. When panes exist, matched keys are
// consumed (not forwarded to the child). When no panes exist, all keys
// pass through.
type PaneKeyHandler struct {
	// HasPanes reports whether any panes are currently active.
	// When false, all key bindings pass through to the child process.
	HasPanes func() bool
}

// HandleKey scans data for pane navigation key sequences. If a pane key is
// found and panes exist, it returns consumed=true with the matching action.
// The caller should not forward the consumed bytes to the child process.
//
// Key bindings:
//
//	Ctrl+H / Ctrl+B: Focus pane left
//	Ctrl+L / Ctrl+F: Focus pane right
//	Ctrl+J / Ctrl+N: Focus pane below
//	Ctrl+K / Ctrl+P: Focus pane above
//	Alt+H  (0x1B 'h'): Split horizontal
//	Alt+V  (0x1B 'v'): Split vertical
//	Ctrl+X: Close current pane
//	Ctrl+D: Close current pane (with confirmation)
//
// When HasPanes returns false, all keys pass through (consumed=false).
func (h *PaneKeyHandler) HandleKey(data []byte) (consumed bool, action PaneAction) {
	if h.HasPanes == nil || !h.HasPanes() {
		return false, PaneActionNone
	}

	if len(data) == 0 {
		return false, PaneActionNone
	}

	// Check for Alt+H (0x1B followed by 'h') or Alt+V (0x1B followed by 'v').
	// These are two-byte sequences, so we need at least 2 bytes.
	if len(data) >= 2 && data[0] == 0x1B {
		switch data[1] {
		case 'h':
			return true, PaneSplitH
		case 'v':
			return true, PaneSplitV
		}
	}

	// Single-byte control characters.
	switch data[0] {
	case 0x08, 0x02: // Ctrl+H (BS), Ctrl+B (STX)
		return true, PaneFocusLeft
	case 0x0C, 0x06: // Ctrl+L (FF), Ctrl+F (ACK)
		return true, PaneFocusRight
	case 0x0A, 0x0E: // Ctrl+J (LF), Ctrl+N (SO)
		return true, PaneFocusDown
	case 0x0B, 0x10: // Ctrl+K (VT), Ctrl+P (DLE)
		return true, PaneFocusUp
	case 0x18: // Ctrl+X (CAN)
		return true, PaneClose
	case 0x04: // Ctrl+D (EOT)
		return true, PaneCloseConfirm
	}

	return false, PaneActionNone
}

// HandleKeyInBuffer scans a full input buffer for pane key sequences at the
// start of the buffer. It returns:
//   - prefixLen: bytes consumed from the front of data (0 if no pane key matched)
//   - action: the detected PaneAction (PaneActionNone if no match)
//   - remaining: the unconsumed tail of data (data[prefixLen:])
//
// This is designed for integration with the PreProcess callback in
// forward_stdin, where the first bytes of a read chunk determine whether
// a pane key was pressed.
func (h *PaneKeyHandler) HandleKeyInBuffer(data []byte) (prefixLen int, action PaneAction, remaining []byte) {
	consumed, action := h.HandleKey(data)
	if !consumed {
		return 0, PaneActionNone, data
	}

	// Determine how many bytes the matched sequence occupies.
	n := 1
	if len(data) >= 2 && data[0] == 0x1B && (data[1] == 'h' || data[1] == 'v') {
		n = 2
	}

	return n, action, data[n:]
}
