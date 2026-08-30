package termmux

import (
	"fmt"
	"strconv"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// MouseDragState is a snapshot of an active mouse drag operation. It is safe
// to read from any goroutine, but mutation is owned by MouseDrag. Fields use
// the JS-friendly string representation for PaneID.
type MouseDragState struct {
	Active         bool
	Button         tea.MouseButton
	StartX, StartY int
	LastX, LastY   int
	PaneID, Edge   string
}

// MouseDrag tracks a button-down / motion / release lifecycle for dragging
// pane dividers in a termmux layout. It consumes BubbleTea mouse events and
// calls SessionManager.ResizePane to adjust split ratios during motion.
type MouseDrag struct {
	mu    sync.Mutex
	state MouseDragState
}

// NewMouseDrag creates an inactive MouseDrag state machine.
func NewMouseDrag() *MouseDrag {
	return &MouseDrag{}
}

// Handle consumes BubbleTea mouse messages and updates the dragging state.
// It returns true for consumed events. Unrecognized events or events that do
// not interact with a divider return false so callers can forward them.
func (d *MouseDrag) Handle(msg tea.MouseMsg, mgr *SessionManager) (handled bool, cmd tea.Cmd, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	switch m := msg.(type) {
	case tea.MouseClickMsg:
		return d.handleMouseDown(m.Mouse(), mgr)
	case tea.MouseMotionMsg:
		return d.handleMouseMotion(m.Mouse(), mgr)
	case tea.MouseReleaseMsg:
		return d.handleMouseRelease(m.Mouse())
	default:
		return false, nil, nil
	}
}

func (d *MouseDrag) handleMouseDown(mouse tea.Mouse, mgr *SessionManager) (bool, tea.Cmd, error) {
	if d.state.Active {
		return false, nil, nil
	}
	if mouse.Button != tea.MouseLeft {
		return false, nil, nil
	}
	panes := mgr.Panes()
	if len(panes) < 2 {
		return false, nil, nil
	}
	id, edge := dividerEdgeAt(panes, mouse.X, mouse.Y)
	if id == 0 {
		return false, nil, nil
	}
	d.state = MouseDragState{
		Active: true,
		Button: mouse.Button,
		StartX: mouse.X,
		StartY: mouse.Y,
		LastX:  mouse.X,
		LastY:  mouse.Y,
		PaneID: fmt.Sprintf("%d", id),
		Edge:   edge,
	}
	return true, nil, nil
}

func (d *MouseDrag) handleMouseMotion(mouse tea.Mouse, mgr *SessionManager) (bool, tea.Cmd, error) {
	if !d.state.Active || mouse.Button != d.state.Button {
		return false, nil, nil
	}
	dx := mouse.X - d.state.LastX
	dy := mouse.Y - d.state.LastY
	if dx == 0 && dy == 0 {
		return true, nil, nil
	}

	id, err := parsePaneID(d.state.PaneID)
	if err != nil {
		d.state.Active = false
		return true, nil, nil
	}

	panes := mgr.Panes()
	geom, ok := paneGeometry(panes, id)
	if !ok {
		d.state.Active = false
		return true, nil, nil
	}

	var delta, newDim, total int
	switch d.state.Edge {
	case "bottom":
		delta = d.state.LastY - mouse.Y
		newDim = geom.Rows + delta
		total = totalRows(panes)
	case "right":
		delta = d.state.LastX - mouse.X
		newDim = geom.Cols + delta
		total = totalCols(panes)
	default:
		d.state.Active = false
		return true, nil, nil
	}
	if total <= 0 || newDim <= 0 {
		d.state.LastX = mouse.X
		d.state.LastY = mouse.Y
		return true, nil, nil
	}

	ratio := clamp(float64(newDim)/float64(total), 0.1, 0.9)
	err = mgr.ResizePane(id, ratio)

	d.state.LastX = mouse.X
	d.state.LastY = mouse.Y
	return true, nil, err
}

func (d *MouseDrag) handleMouseRelease(mouse tea.Mouse) (bool, tea.Cmd, error) {
	if !d.state.Active {
		return false, nil, nil
	}
	d.state.Active = false
	return true, nil, nil
}

// dividerEdgeAt returns the ID and edge of a pane whose boundary is a divider
// at the given screen coordinate, or (0, "") when no divider is present. Only
// horizontal (bottom) and vertical (right) divider edges are reported because
// they unambiguously identify the adjacent pair of panes.
func dividerEdgeAt(panes []Pane, x, y int) (PaneID, string) {
	for i, p := range panes {
		g := p.Geometry
		if g.Rows == 0 || g.Cols == 0 {
			continue
		}
		// Bottom edge of pane i shared with another pane's top edge.
		if y == g.Row+g.Rows && x >= g.Col && x < g.Col+g.Cols {
			for j, q := range panes {
				if i == j {
					continue
				}
				qg := q.Geometry
				if y == qg.Row &&
					x >= max(g.Col, qg.Col) &&
					x < min(g.Col+g.Cols, qg.Col+qg.Cols) {
					return p.ID, "bottom"
				}
			}
		}
		// Right edge of pane i shared with another pane's left edge.
		if x == g.Col+g.Cols && x >= g.Col && x < g.Col+g.Cols {
			for j, q := range panes {
				if i == j {
					continue
				}
				qg := q.Geometry
				if x == qg.Col &&
					y >= max(g.Row, qg.Row) &&
					y < min(g.Row+g.Rows, qg.Row+qg.Rows) {
					return p.ID, "right"
				}
			}
		}
	}
	return 0, ""
}

func paneGeometry(panes []Pane, id PaneID) (PaneGeometry, bool) {
	for _, p := range panes {
		if p.ID == id {
			return p.Geometry, true
		}
	}
	return PaneGeometry{}, false
}

func totalRows(panes []Pane) int {
	total := 0
	for _, p := range panes {
		total += p.Geometry.Rows
	}
	return total
}

func totalCols(panes []Pane) int {
	total := 0
	for _, p := range panes {
		total += p.Geometry.Cols
	}
	return total
}

// parsePaneID parses a decimal PaneID string.
func parsePaneID(s string) (PaneID, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid pane id %q: %w", s, err)
	}
	return PaneID(v), nil
}
