package termmux

import (
	"fmt"
	"io"
	"maps"
	"sort"
	"strconv"
	"sync"
)

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

// PrefixResult is the outcome of dispatching a prefix command to a
// SessionManager. It is suitable for returning to JavaScript callers.
type PrefixResult struct {
	Action      PrefixAction
	Description string
	Consumed    bool
	ListKeys    string
	Result      any
}

// PrefixDispatcher maps prefix-key actions to real SessionManager operations.
type PrefixDispatcher struct {
	mgr     *SessionManager
	handler *PrefixKeyHandler
}

// NewPrefixDispatcher creates a dispatcher that executes prefix actions on mgr.
func NewPrefixDispatcher(mgr *SessionManager, handler *PrefixKeyHandler) *PrefixDispatcher {
	return &PrefixDispatcher{mgr: mgr, handler: handler}
}

// Handle looks up key in the command table and executes the corresponding
// SessionManager operation. Unbound keys return Consumed=false with no state
// change; bound keys return Consumed=true even if the operation errors.
func (d *PrefixDispatcher) Handle(key string) (*PrefixResult, error) {
	kind, ok := d.handler.Commands()[key]
	if !ok {
		return &PrefixResult{
			Action:      PrefixAction{Kind: PrefixActionNone},
			Description: "unbound key",
			Consumed:    false,
		}, nil
	}

	action := PrefixAction{Kind: kind}
	switch kind {
	case PrefixActionNewWindow:
		return d.handleNewWindow(action)
	case PrefixActionNextWindow:
		return d.handleNextWindow(action)
	case PrefixActionPrevWindow:
		return d.handlePrevWindow(action)
	case PrefixActionRenameWindow:
		return d.handleRenameWindow(action)
	case PrefixActionClosePane:
		return d.handleClosePane(action)
	case PrefixActionSplitHorizontal:
		return d.handleSplitHorizontal(action)
	case PrefixActionSplitVertical:
		return d.handleSplitVertical(action)
	case PrefixActionCopyMode:
		return d.handleCopyMode(action)
	case PrefixActionZoomPane:
		return d.handleZoomPane(action)
	case PrefixActionListKeys:
		return d.handleListKeys(action)
	case PrefixActionDetach:
		return &PrefixResult{Action: action, Description: "detach", Consumed: true}, nil
	case PrefixActionCancel:
		return &PrefixResult{Action: action, Description: "cancel", Consumed: true}, nil
	default:
		return &PrefixResult{Action: action, Description: "unknown action", Consumed: false}, nil
	}
}

func (d *PrefixDispatcher) handleNewWindow(action PrefixAction) (*PrefixResult, error) {
	wid, err := d.mgr.NewWindow("")
	if err != nil {
		return nil, err
	}

	sess := newDefaultInteractiveSession()
	pid, err := d.mgr.AddPaneToWindow(sess, SessionTarget{Name: "default", Kind: SessionKindPTY}, wid, SplitRight)
	if err != nil {
		return nil, err
	}
	d.activateWindowPane(wid, pid)

	return &PrefixResult{
		Action:      action,
		Description: "new window",
		Consumed:    true,
		Result:      uint64(wid),
	}, nil
}

func defaultSessionName(name string) string {
	if name == "" {
		return "default"
	}
	return name
}

func (d *PrefixDispatcher) activateWindowPane(wid WindowID, pid PaneID) {
	for _, w := range d.mgr.windowMgr.Windows() {
		if w.ID != wid {
			continue
		}
		for _, p := range w.paneMgr.Panes() {
			if p.ID == pid && p.SessionID != 0 {
				_ = d.mgr.Activate(p.SessionID)
				return
			}
		}
	}
}

func (d *PrefixDispatcher) handleNextWindow(action PrefixAction) (*PrefixResult, error) {
	id := d.mgr.NextWindow()
	return &PrefixResult{
		Action:      action,
		Description: "next window",
		Consumed:    true,
		Result:      uint64(id),
	}, nil
}

func (d *PrefixDispatcher) handlePrevWindow(action PrefixAction) (*PrefixResult, error) {
	id := d.mgr.PrevWindow()
	return &PrefixResult{
		Action:      action,
		Description: "previous window",
		Consumed:    true,
		Result:      uint64(id),
	}, nil
}

func (d *PrefixDispatcher) handleRenameWindow(action PrefixAction) (*PrefixResult, error) {
	wid := d.mgr.ActiveWindowID()
	if wid == 0 {
		return &PrefixResult{
			Action:      action,
			Description: "rename window (no active window)",
			Consumed:    true,
		}, nil
	}
	if err := d.mgr.RenameWindow(wid, "renamed"); err != nil {
		return nil, err
	}
	return &PrefixResult{
		Action:      action,
		Description: "rename window",
		Consumed:    true,
		Result:      uint64(wid),
	}, nil
}

func (d *PrefixDispatcher) handleClosePane(action PrefixAction) (*PrefixResult, error) {
	pid := d.mgr.ActivePaneID()
	if pid != 0 {
		if err := d.mgr.ClosePane(pid); err != nil {
			return nil, err
		}
	}
	return &PrefixResult{
		Action:      action,
		Description: "close pane",
		Consumed:    true,
	}, nil
}

func (d *PrefixDispatcher) handleSplitHorizontal(action PrefixAction) (*PrefixResult, error) {
	pid := d.mgr.ActivePaneID()
	if pid == 0 {
		return &PrefixResult{
			Action:      action,
			Description: "split horizontal (no active pane)",
			Consumed:    true,
		}, nil
	}
	newPane, err := d.mgr.SplitPaneHorizontal(pid, CaptureConfig{})
	if err != nil {
		return nil, err
	}
	return &PrefixResult{
		Action:      action,
		Description: "split horizontal",
		Consumed:    true,
		Result:      uint64(newPane),
	}, nil
}

func (d *PrefixDispatcher) handleSplitVertical(action PrefixAction) (*PrefixResult, error) {
	pid := d.mgr.ActivePaneID()
	if pid == 0 {
		return &PrefixResult{
			Action:      action,
			Description: "split vertical (no active pane)",
			Consumed:    true,
		}, nil
	}
	newPane, err := d.mgr.SplitPaneVertical(pid, CaptureConfig{})
	if err != nil {
		return nil, err
	}
	return &PrefixResult{
		Action:      action,
		Description: "split vertical",
		Consumed:    true,
		Result:      uint64(newPane),
	}, nil
}

func (d *PrefixDispatcher) handleCopyMode(action PrefixAction) (*PrefixResult, error) {
	sid := d.mgr.ActiveID()
	if sid != 0 {
		if err := d.mgr.EnterCopyMode(sid); err != nil {
			return nil, err
		}
	}
	return &PrefixResult{
		Action:      action,
		Description: "enter copy mode",
		Consumed:    true,
	}, nil
}

func (d *PrefixDispatcher) handleZoomPane(action PrefixAction) (*PrefixResult, error) {
	pid := d.mgr.ActivePaneID()
	if pid != 0 {
		d.mgr.ToggleZoom(pid)
	}
	return &PrefixResult{
		Action:      action,
		Description: "zoom pane",
		Consumed:    true,
	}, nil
}

func (d *PrefixDispatcher) handleListKeys(action PrefixAction) (*PrefixResult, error) {
	return &PrefixResult{
		Action:      action,
		Description: "list keys",
		Consumed:    true,
		ListKeys:    d.listKeys(),
	}, nil
}

func (d *PrefixDispatcher) listKeys() string {
	cmds := d.handler.Commands()
	keys := make([]string, 0, len(cmds))
	for k := range cmds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b []byte
	for i, k := range keys {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, formatKey(k)...)
		b = append(b, "  "...)
		b = append(b, PrefixAction{Kind: cmds[k]}.String()...)
	}
	return string(b)
}

func formatKey(key string) string {
	switch key {
	case "ctrl+b":
		return "C-b"
	case "esc":
		return "ESC"
	default:
		if len(key) == 1 {
			return key
		}
		return strconv.Quote(key)
	}
}

// newDefaultInteractiveSession returns a started, inert interactive session
// suitable for placeholder panes. It never produces output and closes cleanly.
func newDefaultInteractiveSession() InteractiveSession {
	sio := &nopStringIO{done: make(chan struct{})}
	sess := NewStringIOSession(sio)
	sess.Start()
	return sess
}

// nopStringIO is a StringIO implementation that blocks on Receive until
// Close is called, then returns io.EOF. It is used for placeholder panes
// that do not need real PTY output.
type nopStringIO struct {
	done chan struct{}
	once sync.Once
}

func (n *nopStringIO) Send(string) error { return nil }

func (n *nopStringIO) Receive() (string, error) {
	<-n.done
	return "", io.EOF
}

func (n *nopStringIO) Close() error {
	n.once.Do(func() { close(n.done) })
	return nil
}
