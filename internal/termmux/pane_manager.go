package termmux

import (
	"fmt"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// PaneBinding associates a PaneID with its SessionID and VTerm.
// It is the internal bookkeeping record used by paneManager.
type PaneBinding struct {
	PaneID       PaneID
	SessionID    SessionID
	VTerm        *vt.VTerm
	Title        string
	LastActive   time.Time
	Exited       bool
	RemainOnExit bool
	geometry     *PaneGeometry
}

// paneManager implements the PaneManager interface. It uses a LayoutEngine
// for geometry computation and maintains a map of PaneID → PaneBinding.
// All mutations must happen on the SessionManager worker goroutine.
type paneManager struct {
	engine       *LayoutEngine
	panes        map[PaneID]*PaneBinding
	paneOrder    []PaneID // insertion order for layout
	activePaneID PaneID
	synchronize  bool       // when true, input is sent to all panes
	remainOnExit bool       // default for new panes
	mu           sync.Mutex // protects concurrent reads for Panes() snapshot
}

// newPaneManager creates a paneManager with the given layout mode and
// screen dimensions.
func newPaneManager(mode LayoutMode, width, height int) *paneManager {
	pm := &paneManager{
		engine: NewLayoutEngine(mode, width, height),
		panes:  make(map[PaneID]*PaneBinding),
	}
	pm.engine.paneMgr = pm
	return pm
}

// Create adds a new pane associated with the given sessionID, splitting
// from the active pane in the given direction. Returns the new PaneID.
func (pm *paneManager) Create(sessionID SessionID, direction SplitDirection) (PaneID, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Split in the layout engine — it allocates and returns the new PaneID.
	id := pm.engine.Split(pm.activePaneID, direction)

	pm.panes[id] = &PaneBinding{
		PaneID:       id,
		SessionID:    sessionID,
		Title:        fmt.Sprintf("pane-%d", id),
		LastActive:   time.Now(),
		RemainOnExit: pm.remainOnExit,
	}
	pm.paneOrder = append(pm.paneOrder, id)

	// First pane becomes active automatically.
	if pm.activePaneID == 0 {
		pm.activePaneID = id
	}

	return id, nil
}

func (pm *paneManager) Remove(id PaneID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	binding, ok := pm.panes[id]
	if !ok {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	_ = binding.SessionID

	pm.engine.Remove(id)

	delete(pm.panes, id)
	for i, pid := range pm.paneOrder {
		if pid == id {
			pm.paneOrder = append(pm.paneOrder[:i], pm.paneOrder[i+1:]...)
			break
		}
	}

	if pm.activePaneID == id {
		pm.activePaneID = 0
		if len(pm.paneOrder) > 0 {
			pm.activePaneID = pm.paneOrder[0]
		}
	}

	return nil
}

// Focus switches the active pane to the given ID.
func (pm *paneManager) Focus(id PaneID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.panes[id]; !ok {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	pm.activePaneID = id
	if binding, ok := pm.panes[id]; ok {
		binding.LastActive = time.Now()
	}

	return nil
}

// Resize adjusts the split ratio for the given pane.
func (pm *paneManager) Resize(id PaneID, ratio float64) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.panes[id]; !ok {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	if !pm.engine.Resize(id, ratio) {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	return nil
}

func (pm *paneManager) paneSlice() []Pane {
	panes := make([]Pane, 0, len(pm.paneOrder))
	for _, pid := range pm.paneOrder {
		panes = append(panes, Pane{ID: pid})
	}
	return panes
}

func (pm *paneManager) FocusAt(row, col int) (PaneID, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id, ok := pm.engine.PaneAt(pm.paneSlice(), row, col)
	if !ok {
		return 0, fmt.Errorf("%w: no pane at row %d col %d", ErrPaneNotFound, row, col)
	}

	pm.activePaneID = id
	if binding, ok := pm.panes[id]; ok {
		binding.LastActive = time.Now()
	}
	return id, nil
}

func (pm *paneManager) ResizePaneAt(row, col int, ratio float64) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id, ok := pm.engine.DividerAt(pm.paneSlice(), row, col)
	if !ok {
		return fmt.Errorf("%w: no divider at row %d col %d", ErrPaneNotFound, row, col)
	}

	if !pm.engine.Resize(id, ratio) {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}
	return nil
}

// Panes returns a snapshot of all panes with their computed geometries.
func (pm *paneManager) Panes() []Pane {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.paneOrder) == 0 {
		return nil
	}

	// Build ordered Pane slice for geometry computation.
	paneSlice := make([]Pane, 0, len(pm.paneOrder))
	for _, pid := range pm.paneOrder {
		binding := pm.panes[pid]
		paneSlice = append(paneSlice, Pane{
			ID:         binding.PaneID,
			SessionID:  binding.SessionID,
			Title:      binding.Title,
			Focus:      binding.PaneID == pm.activePaneID,
			Exited:     binding.Exited,
			VTerm:      binding.VTerm,
			LastActive: binding.LastActive,
		})
	}

	// Compute geometries.
	geoms := pm.engine.Compute(paneSlice)

	result := make([]Pane, len(paneSlice))
	for i := range paneSlice {
		result[i] = paneSlice[i]
		if binding := pm.panes[paneSlice[i].ID]; binding.geometry != nil {
			result[i].Geometry = *binding.geometry
		} else {
			result[i].Geometry = geoms[i]
		}
	}

	return result
}

// ResizePaneDelta returns a new PaneGeometry for the given pane by applying a
// directional cell delta. The override must be committed separately
// (paneManager.setGeometry) so callers can first propagate the new size to the
// child PTY/VTerm.
func (pm *paneManager) ResizePaneDelta(id PaneID, direction string, delta int) (PaneGeometry, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.panes[id]; !ok {
		return PaneGeometry{}, fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	panes := make([]Pane, 0, len(pm.paneOrder))
	baseGeoms := pm.engine.Compute(pm.paneSlice())
	for i, pid := range pm.paneOrder {
		geo := baseGeoms[i]
		if binding := pm.panes[pid]; binding.geometry != nil {
			geo = *binding.geometry
		}
		panes = append(panes, Pane{ID: pid, Geometry: geo})
	}

	return pm.engine.ResizePaneDelta(panes, id, direction, delta)
}

func (pm *paneManager) setGeometry(id PaneID, g PaneGeometry) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	binding, ok := pm.panes[id]
	if !ok {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}
	gCopy := g
	binding.geometry = &gCopy
	return nil
}

// ActivePaneID returns the currently focused pane ID, or 0 if no panes exist.
func (pm *paneManager) ActivePaneID() PaneID {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.activePaneID
}

// FocusNext moves focus to the adjacent pane in the given direction.
// Returns the new active PaneID.
func (pm *paneManager) FocusNext(direction NavigationDirection) PaneID {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.activePaneID == 0 {
		return 0
	}

	nextID := pm.engine.FocusNext(pm.activePaneID, direction)
	if nextID != 0 && nextID != pm.activePaneID {
		pm.activePaneID = nextID
		if binding, ok := pm.panes[nextID]; ok {
			binding.LastActive = time.Now()
		}
	}

	return pm.activePaneID
}

// HasPanes reports whether any panes exist.
func (pm *paneManager) HasPanes() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return len(pm.panes) > 0
}

// FocusLeft moves focus to the pane on the left.
func (pm *paneManager) FocusLeft() { pm.FocusNext(NavLeft) }

// FocusRight moves focus to the pane on the right.
func (pm *paneManager) FocusRight() { pm.FocusNext(NavRight) }

// FocusDown moves focus to the pane below.
func (pm *paneManager) FocusDown() { pm.FocusNext(NavDown) }

// FocusUp moves focus to the pane above.
func (pm *paneManager) FocusUp() { pm.FocusNext(NavUp) }

// Close shuts down the pane manager, releasing all resources.
func (pm *paneManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.panes = make(map[PaneID]*PaneBinding)
	pm.paneOrder = nil
	pm.activePaneID = 0
	return nil
}

// setVTerm sets the VTerm for a pane binding. Called after the worker
// creates the VTerm during session registration.
func (pm *paneManager) setVTerm(id PaneID, v *vt.VTerm) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if binding, ok := pm.panes[id]; ok {
		binding.VTerm = v
	}
}

// setSize updates the layout engine's screen dimensions.
func (pm *paneManager) setSize(width, height int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.engine.SetSize(width, height)
}

// setMode changes the layout engine's current layout mode.
func (pm *paneManager) setMode(mode LayoutMode) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.engine.SetMode(mode)
}

// mode returns the layout engine's current layout mode.
func (pm *paneManager) mode() LayoutMode {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	return pm.engine.Mode()
}

// activeSessionID returns the SessionID of the active pane, or 0 if none.
func (pm *paneManager) activeSessionID() SessionID {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.activePaneID == 0 {
		return 0
	}
	if binding, ok := pm.panes[pm.activePaneID]; ok {
		return binding.SessionID
	}
	return 0
}

// Verify paneManager implements PaneManager at compile time.
func (pm *paneManager) removeSessionID(id PaneID) SessionID {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	binding, ok := pm.panes[id]
	if !ok {
		return 0
	}
	return binding.SessionID
}

// transferPaneToWindow moves the pane with the given id from this paneManager
// to target. The pane receives a fresh PaneID in the target layout engine.
// The original binding's SessionID, VTerm, Title, LastActive, Exited, and
// RemainOnExit are copied to the new binding.
//
// On success the source pane is removed, the source window's active pane is
// refocused to a remaining pane (if the moved pane was active), and the target
// window's active pane is set to the newly created pane. The moved SessionID is
// returned alongside the new PaneID so callers can update global active state.
func (pm *paneManager) transferPaneToWindow(id PaneID, target *paneManager, dir SplitDirection) (PaneID, SessionID, error) {
	pm.mu.Lock()
	target.mu.Lock()
	defer pm.mu.Unlock()
	defer target.mu.Unlock()

	binding, ok := pm.panes[id]
	if !ok {
		return 0, 0, fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	pm.engine.Remove(id)

	srcIdx := -1
	for i, pid := range pm.paneOrder {
		if pid == id {
			srcIdx = i
			break
		}
	}
	if srcIdx >= 0 {
		pm.paneOrder = append(pm.paneOrder[:srcIdx], pm.paneOrder[srcIdx+1:]...)
	}
	delete(pm.panes, id)

	wasActive := pm.activePaneID == id
	if wasActive {
		pm.activePaneID = 0
		if len(pm.paneOrder) > 0 {
			pm.activePaneID = pm.paneOrder[0]
		}
	}

	newID := target.engine.Split(target.activePaneID, dir)

	target.paneOrder = append(target.paneOrder, newID)
	target.panes[newID] = &PaneBinding{
		PaneID:       newID,
		SessionID:    binding.SessionID,
		VTerm:        binding.VTerm,
		Title:        binding.Title,
		LastActive:   binding.LastActive,
		Exited:       binding.Exited,
		RemainOnExit: binding.RemainOnExit,
	}

	target.activePaneID = newID

	return newID, binding.SessionID, nil
}

var _ PaneManager = (*paneManager)(nil)

func (pm *paneManager) SetSynchronize(v bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.synchronize = v
}

func (pm *paneManager) Synchronize() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.synchronize
}

func (pm *paneManager) AllSessionIDs() []SessionID {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	ids := make([]SessionID, 0, len(pm.panes))
	for _, pid := range pm.paneOrder {
		if binding, ok := pm.panes[pid]; ok {
			ids = append(ids, binding.SessionID)
		}
	}
	return ids
}

func (pm *paneManager) SetRemainOnExit(v bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.remainOnExit = v
}

func (pm *paneManager) RemainOnExit() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.remainOnExit
}

func (pm *paneManager) SetPaneRemainOnExit(id PaneID, v bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	binding, ok := pm.panes[id]
	if !ok {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}
	binding.RemainOnExit = v
	return nil
}

func (pm *paneManager) PaneRemainOnExit(id PaneID) (bool, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	binding, ok := pm.panes[id]
	if !ok {
		return false, fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}
	return binding.RemainOnExit, nil
}

func (pm *paneManager) PaneExited(id PaneID) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	binding, ok := pm.panes[id]
	if !ok {
		return false
	}
	return binding.Exited
}

func (pm *paneManager) MarkPaneExited(id PaneID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if binding, ok := pm.panes[id]; ok {
		binding.Exited = true
	}
}

func (pm *paneManager) PaneIDForSession(sid SessionID) PaneID {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, pid := range pm.paneOrder {
		if binding, ok := pm.panes[pid]; ok && binding.SessionID == sid {
			return pid
		}
	}
	return 0
}

func (pm *paneManager) RebindSession(id PaneID, sid SessionID, vterm *vt.VTerm) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	binding, ok := pm.panes[id]
	if !ok {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, id)
	}

	binding.SessionID = sid
	if vterm != nil {
		binding.VTerm = vterm
	}
	binding.Exited = false
	return nil
}

// Swap exchanges all metadata fields (SessionID, VTerm, Title, LastActive,
// Exited, RemainOnExit) between the bindings for id1 and id2 while keeping
// the pane IDs themselves unchanged. The active pane ID is preserved.
func (pm *paneManager) Swap(id1, id2 PaneID) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	b1, ok1 := pm.panes[id1]
	b2, ok2 := pm.panes[id2]
	if !ok1 || !ok2 {
		return fmt.Errorf("%w: one or both panes not found", ErrPaneNotFound)
	}

	swapPaneBindingMetadata(b1, b2)
	return nil
}

func (pm *paneManager) Binding(id PaneID) *PaneBinding {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.panes[id]
}

// swapPaneBindingMetadata exchanges every mutable field of two PaneBinding
// values, leaving their PaneID fields intact.
func swapPaneBindingMetadata(a, b *PaneBinding) {
	a.SessionID, b.SessionID = b.SessionID, a.SessionID
	a.VTerm, b.VTerm = b.VTerm, a.VTerm
	a.Title, b.Title = b.Title, a.Title
	a.LastActive, b.LastActive = b.LastActive, a.LastActive
	a.Exited, b.Exited = b.Exited, a.Exited
	a.RemainOnExit, b.RemainOnExit = b.RemainOnExit, a.RemainOnExit
}
