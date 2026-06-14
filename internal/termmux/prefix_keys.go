package termmux

import "maps"

import "fmt"

// PrefixActionKind classifies the result of a prefix key dispatch.
type PrefixActionKind int

const (
	PrefixActionNone PrefixActionKind = iota
	PrefixActionNewWindow
	PrefixActionNextWindow
	PrefixActionPrevWindow
	PrefixActionDetach
	PrefixActionZoomPane
	PrefixActionClosePane
	PrefixActionSplitHorizontal
	PrefixActionSplitVertical
	PrefixActionCopyMode
	PrefixActionListKeys
	PrefixActionRenameWindow
	PrefixActionCancel
)

// PrefixAction is the result of dispatching a command after the prefix key.
type PrefixAction struct {
	Kind PrefixActionKind
}

func (a PrefixAction) String() string {
	switch a.Kind {
	case PrefixActionNewWindow:
		return "NewWindow"
	case PrefixActionNextWindow:
		return "NextWindow"
	case PrefixActionPrevWindow:
		return "PrevWindow"
	case PrefixActionDetach:
		return "Detach"
	case PrefixActionZoomPane:
		return "ZoomPane"
	case PrefixActionClosePane:
		return "ClosePane"
	case PrefixActionSplitHorizontal:
		return "SplitHorizontal"
	case PrefixActionSplitVertical:
		return "SplitVertical"
	case PrefixActionCopyMode:
		return "CopyMode"
	case PrefixActionListKeys:
		return "ListKeys"
	case PrefixActionRenameWindow:
		return "RenameWindow"
	case PrefixActionCancel:
		return "Cancel"
	default:
		return "None"
	}
}

// PrefixKeyHandler buffers keystrokes: when the prefix key is detected,
// the next key is intercepted and dispatched as a command (not forwarded
// to the child). Default prefix: Ctrl+B (configurable).
type PrefixKeyHandler struct {
	prefix   string
	awaiting bool
	commands map[string]PrefixActionKind
}

// NewPrefixKeyHandler creates a PrefixKeyHandler with the given prefix key
// and the default command table.
func NewPrefixKeyHandler(prefix string) *PrefixKeyHandler {
	if prefix == "" {
		prefix = "ctrl+b"
	}
	h := &PrefixKeyHandler{
		prefix:   prefix,
		commands: defaultPrefixCommands(),
	}
	return h
}

// HandleKey processes a key event. Returns (handled, action):
//   - handled=false: key should be forwarded to the child process
//   - handled=true, action.Kind=PrefixActionNone: prefix was detected, awaiting next key
//   - handled=true, action.Kind=other: command dispatched after prefix
func (h *PrefixKeyHandler) HandleKey(key string) (handled bool, action PrefixAction) {
	if h.awaiting {
		h.awaiting = false
		if kind, ok := h.commands[key]; ok {
			return true, PrefixAction{Kind: kind}
		}
		if key == "esc" || key == h.prefix {
			return true, PrefixAction{Kind: PrefixActionCancel}
		}
		return true, PrefixAction{Kind: PrefixActionNone}
	}

	if key == h.prefix {
		h.awaiting = true
		return true, PrefixAction{Kind: PrefixActionNone}
	}

	return false, PrefixAction{}
}

// SetCommand adds or overrides a command mapping.
func (h *PrefixKeyHandler) SetCommand(key string, kind PrefixActionKind) {
	h.commands[key] = kind
}

// RemoveCommand removes a command mapping.
func (h *PrefixKeyHandler) RemoveCommand(key string) {
	delete(h.commands, key)
}

// Prefix returns the configured prefix key.
func (h *PrefixKeyHandler) Prefix() string {
	return h.prefix
}

// SetPrefix changes the prefix key.
func (h *PrefixKeyHandler) SetPrefix(prefix string) {
	h.prefix = prefix
}

// Awaiting reports whether the handler is waiting for a command key
// after the prefix was pressed.
func (h *PrefixKeyHandler) Awaiting() bool {
	return h.awaiting
}

// Commands returns a copy of the current command table.
func (h *PrefixKeyHandler) Commands() map[string]PrefixActionKind {
	cp := make(map[string]PrefixActionKind, len(h.commands))
	maps.Copy(cp, h.commands)
	return cp
}

// Reset cancels any in-progress prefix sequence.
func (h *PrefixKeyHandler) Reset() {
	h.awaiting = false
}

func defaultPrefixCommands() map[string]PrefixActionKind {
	return map[string]PrefixActionKind{
		"c":   PrefixActionNewWindow,
		"n":   PrefixActionNextWindow,
		"p":   PrefixActionPrevWindow,
		"d":   PrefixActionDetach,
		"z":   PrefixActionZoomPane,
		"x":   PrefixActionClosePane,
		"%":   PrefixActionSplitHorizontal,
		"\"":  PrefixActionSplitVertical,
		"[":   PrefixActionCopyMode,
		"?":   PrefixActionListKeys,
		",":   PrefixActionRenameWindow,
		"esc": PrefixActionCancel,
	}
}

// Verify PrefixActionKind implements fmt.Stringer at compile time.
var _ fmt.Stringer = PrefixAction{}
