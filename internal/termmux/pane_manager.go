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
	return &paneManager{
		engine: NewLayoutEngine(mode, width, height),
		panes:  make(map[PaneID]*PaneBinding),
	}
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
			VTerm:      binding.VTerm,
			LastActive: binding.LastActive,
		})
	}

	// Compute geometries.
	geoms := pm.engine.Compute(paneSlice)

	// Merge geometries back.
	result := make([]Pane, len(paneSlice))
	for i := range paneSlice {
		result[i] = paneSlice[i]
		result[i].Geometry = geoms[i]
	}

	return result
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

// transferPaneToWindow removes a pane binding from this paneManager's
// layout engine and adds it to the target paneManager. The target
// receives a fresh PaneID via target.engine.AllocID(). The original binding's
// SessionID, VTerm, Title, LastActive, Exited, and RemainOnExit are
// copied to the new binding. Returns the new PaneID or zero on failure.
func (pm *paneManager) transferPaneToWindow(target *paneManager, dir SplitDirection) PaneID {
	pm.mu.Lock()
	target.mu.Lock()
	defer pm.mu.Unlock()
	defer target.mu.Unlock()

	binding, ok := pm.panes[pm.activePaneID]
	if !ok {
		return 0
	}

	delete(pm.engine.splits, pm.activePaneID)

	srcIdx := -1
	for i, pid := range pm.paneOrder {
		if pid == pm.activePaneID {
			srcIdx = i
			break
		}
	}
	if srcIdx < 0 {
		return 0
	}
	pm.paneOrder = append(pm.paneOrder[:srcIdx], pm.paneOrder[srcIdx+1:]...)
	delete(pm.panes, pm.activePaneID)

	newID := target.engine.AllocID()

	target.engine.splits[newID] = SplitGroup{Direction: dir, Ratio: 1.0}
	target.engine.panes = append(target.engine.panes, newID)

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

	return newID
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

func (pm *paneManager) Binding(id PaneID) *PaneBinding {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.panes[id]
}
