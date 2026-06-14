package termmux

import (
	"fmt"
	"sync"
	"time"
)

// WindowID uniquely identifies a window within the WindowManager.
// Zero is never assigned and serves as the sentinel "no window" value.
type WindowID uint64

// Window holds a collection of panes managed by a single paneManager.
// Only one window is visible at a time. Windows are displayed in the
// status bar as tabs.
type Window struct {
	ID      WindowID
	Name    string
	Layout  LayoutMode
	paneMgr *paneManager
	created time.Time
}

// WindowManager manages a list of windows within a session. Each window
// has its own pane layout. Only one window is active (visible) at a time.
// All mutations must happen on the SessionManager worker goroutine.
type WindowManager struct {
	windows    map[WindowID]*Window
	order      []WindowID
	activeID   WindowID
	nextID     WindowID
	layoutMode LayoutMode
	width      int
	height     int
	mu         sync.Mutex
}

// NewWindowManager creates a WindowManager with the given layout mode
// and screen dimensions.
func NewWindowManager(mode LayoutMode, width, height int) *WindowManager {
	return &WindowManager{
		windows:    make(map[WindowID]*Window),
		layoutMode: mode,
		width:      width,
		height:     height,
		nextID:     1,
	}
}

// NewWindow creates a new window with the given name and returns its ID.
// The window gets its own paneManager with the default layout mode.
// The first window created automatically becomes active.
func (wm *WindowManager) NewWindow(name string) WindowID {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	id := wm.nextID
	wm.nextID++

	if name == "" {
		name = fmt.Sprintf("window-%d", id)
	}

	w := &Window{
		ID:      id,
		Name:    name,
		Layout:  wm.layoutMode,
		paneMgr: newPaneManager(wm.layoutMode, wm.width, wm.height),
		created: time.Now(),
	}

	wm.windows[id] = w
	wm.order = append(wm.order, id)

	if wm.activeID == 0 {
		wm.activeID = id
	}

	return id
}

// NextWindow switches to the next window in the order. Wraps around.
// Returns the new active WindowID.
func (wm *WindowManager) NextWindow() WindowID {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if len(wm.order) <= 1 {
		return wm.activeID
	}

	idx := wm.windowIndex(wm.activeID)
	if idx < 0 {
		return wm.activeID
	}

	next := (idx + 1) % len(wm.order)
	wm.activeID = wm.order[next]
	return wm.activeID
}

// PrevWindow switches to the previous window in the order. Wraps around.
// Returns the new active WindowID.
func (wm *WindowManager) PrevWindow() WindowID {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if len(wm.order) <= 1 {
		return wm.activeID
	}

	idx := wm.windowIndex(wm.activeID)
	if idx < 0 {
		return wm.activeID
	}

	prev := (idx - 1 + len(wm.order)) % len(wm.order)
	wm.activeID = wm.order[prev]
	return wm.activeID
}

// RenameWindow changes the name of the window with the given ID.
func (wm *WindowManager) RenameWindow(id WindowID, name string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.windows[id]
	if !ok {
		return fmt.Errorf("%w: %d", ErrWindowNotFound, id)
	}
	w.Name = name
	return nil
}

// CloseWindow closes the window with the given ID, removing all its panes.
// If the closed window was active, the next window becomes active.
// Returns an error if this is the last window.
func (wm *WindowManager) CloseWindow(id WindowID) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, ok := wm.windows[id]; !ok {
		return fmt.Errorf("%w: %d", ErrWindowNotFound, id)
	}

	if len(wm.order) <= 1 {
		return fmt.Errorf("%w: cannot close the last window", ErrWindowNotClosable)
	}

	delete(wm.windows, id)
	for i, wid := range wm.order {
		if wid == id {
			wm.order = append(wm.order[:i], wm.order[i+1:]...)
			break
		}
	}

	if wm.activeID == id {
		wm.activeID = wm.order[0]
	}

	return nil
}

// ActiveWindowID returns the currently active window ID, or 0 if no windows exist.
func (wm *WindowManager) ActiveWindowID() WindowID {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return wm.activeID
}

// Window returns the window with the given ID, or nil if not found.
func (wm *WindowManager) Window(id WindowID) *Window {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return wm.windows[id]
}

// Windows returns a snapshot of all windows in order.
func (wm *WindowManager) Windows() []*Window {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	result := make([]*Window, 0, len(wm.order))
	for _, id := range wm.order {
		result = append(result, wm.windows[id])
	}
	return result
}

// ActivePaneManager returns the paneManager of the active window, or nil if no windows exist.
func (wm *WindowManager) ActivePaneManager() *paneManager {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.activeID == 0 {
		return nil
	}
	if w, ok := wm.windows[wm.activeID]; ok {
		return w.paneMgr
	}
	return nil
}

// SetSize updates the screen dimensions for all windows' layout engines.
func (wm *WindowManager) SetSize(width, height int) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.width = width
	wm.height = height
	for _, w := range wm.windows {
		w.paneMgr.setSize(width, height)
	}
}

// windowIndex returns the index of the given window ID in the order slice, or -1.
func (wm *WindowManager) windowIndex(id WindowID) int {
	for i, wid := range wm.order {
		if wid == id {
			return i
		}
	}
	return -1
}

// WindowPanes returns the list of panes in the window with the given ID.
func (wm *WindowManager) WindowPanes(id WindowID) []Pane {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.windows[id]
	if !ok {
		return nil
	}
	return w.paneMgr.Panes()
}

// WindowPaneMgr returns the paneManager for the window with the given ID,
// or nil if the window does not exist.
func (wm *WindowManager) WindowPaneMgr(id WindowID) *paneManager {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	w, ok := wm.windows[id]
	if !ok {
		return nil
	}
	return w.paneMgr
}

// BreakPane removes the active pane from the source window and moves it
// to a brand new window with the same screen dimensions. The new window
// is created with a default name and automatically becomes the target
// for the moved pane. Returns the new WindowID.
//
// The source window is NOT deleted even if it becomes empty.
func (wm *WindowManager) BreakPane(sourceID WindowID) (WindowID, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	source, ok := wm.windows[sourceID]
	if !ok {
		return 0, fmt.Errorf("%w: %d", ErrWindowNotFound, sourceID)
	}

	// Create a new window with the same dimensions.
	newID := wm.nextID
	wm.nextID++
	name := fmt.Sprintf("window-%d", newID)
	newWindow := &Window{
		ID:      newID,
		Name:    name,
		Layout:  wm.layoutMode,
		paneMgr: newPaneManager(wm.layoutMode, wm.width, wm.height),
		created: time.Now(),
	}
	wm.windows[newID] = newWindow
	wm.order = append(wm.order, newID)

	// Transfer the active pane from source to the new window.
	newPaneID := source.paneMgr.transferPaneToWindow(newWindow.paneMgr, SplitRight)
	if newPaneID == 0 {
		// Transfer failed — remove the just-created window.
		delete(wm.windows, newID)
		for i, wid := range wm.order {
			if wid == newID {
				wm.order = append(wm.order[:i], wm.order[i+1:]...)
				break
			}
		}
		return 0, fmt.Errorf("%w: no active pane in window %d", ErrPaneNotFound, sourceID)
	}

	return newID, nil
}

// JoinPane removes the active pane from the source window and moves it
// into the target window. The target window must exist and be different
// from the source. The pane keeps its original SessionID, VTerm, Title, etc.
func (wm *WindowManager) JoinPane(sourceID, targetID WindowID) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	source, ok := wm.windows[sourceID]
	if !ok {
		return fmt.Errorf("%w: %d", ErrWindowNotFound, sourceID)
	}
	target, ok := wm.windows[targetID]
	if !ok {
		return fmt.Errorf("%w: %d", ErrWindowNotFound, targetID)
	}
	if sourceID == targetID {
		return fmt.Errorf("termmux: cannot join pane into the same window")
	}

	// Transfer the active pane from source to target.
	source.paneMgr.transferPaneToWindow(target.paneMgr, SplitRight)

	return nil
}

// WindowInfo is a read-only snapshot of a window's state for external consumers.
type WindowInfo struct {
	ID        WindowID
	Name      string
	Layout    LayoutMode
	PaneCount int
	Active    bool
	Created   time.Time
}
