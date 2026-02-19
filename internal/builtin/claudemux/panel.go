package claudemux

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PanelState represents the lifecycle state of the panel.
type PanelState int

const (
	PanelIdle    PanelState = iota // Not yet started
	PanelActive                    // Active and rendering
	PanelClosed                    // Shut down
)

// PanelStateName returns a human-readable name for a PanelState.
func PanelStateName(s PanelState) string {
	switch s {
	case PanelIdle:
		return "Idle"
	case PanelActive:
		return "Active"
	case PanelClosed:
		return "Closed"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// PaneHealth tracks health indicators for a single instance pane.
type PaneHealth struct {
	State      string    `json:"state"`      // "running", "error", "idle", "stopped"
	ErrorCount int64     `json:"errorCount"`
	TaskCount  int64     `json:"taskCount"`
	LastUpdate time.Time `json:"lastUpdate,omitempty"`
}

// Pane represents a single instance's view state within the panel.
// Each pane has an independent scrollback buffer and scroll position.
type Pane struct {
	ID         string
	Title      string
	Scrollback []string
	MaxLines   int
	ScrollPos  int // 0 = bottom (latest), positive = scrolled up
	Health     PaneHealth
}

// PanelConfig configures the multi-instance panel.
type PanelConfig struct {
	MaxPanes       int // Maximum number of panes (1-9, default 9)
	ScrollbackSize int // Max lines per pane scrollback (default 10000)
}

// DefaultPanelConfig returns production-ready panel configuration.
func DefaultPanelConfig() PanelConfig {
	return PanelConfig{
		MaxPanes:       9,
		ScrollbackSize: 10000,
	}
}

// Panel manages multiple instance views with Alt+N switching,
// input routing, per-pane scrollback, and health indicators.
//
// Panel is safe for concurrent use from multiple goroutines.
type Panel struct {
	config PanelConfig

	mu        sync.Mutex
	panes     []*Pane
	activeIdx int
	state     PanelState
}

// NewPanel creates a panel with the given configuration.
func NewPanel(config PanelConfig) *Panel {
	if config.MaxPanes < 1 {
		config.MaxPanes = 1
	}
	if config.MaxPanes > 9 {
		config.MaxPanes = 9
	}
	if config.ScrollbackSize < 100 {
		config.ScrollbackSize = 100
	}
	return &Panel{
		config: config,
		panes:  make([]*Pane, 0, config.MaxPanes),
		state:  PanelIdle,
	}
}

// Start transitions the panel from Idle to Active.
func (p *Panel) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != PanelIdle {
		return fmt.Errorf("claudemux: panel cannot start from state %s",
			PanelStateName(p.state))
	}
	p.state = PanelActive
	return nil
}

// AddPane adds a new instance pane to the panel. Returns the pane
// index (0-based, corresponding to Alt+1..9).
func (p *Panel) AddPane(id, title string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PanelClosed {
		return -1, fmt.Errorf("claudemux: panel is closed")
	}
	if len(p.panes) >= p.config.MaxPanes {
		return -1, fmt.Errorf("claudemux: panel full (max %d panes)", p.config.MaxPanes)
	}

	// Check duplicate ID.
	for _, pane := range p.panes {
		if pane.ID == id {
			return -1, fmt.Errorf("claudemux: pane %q already exists", id)
		}
	}

	pane := &Pane{
		ID:         id,
		Title:      title,
		Scrollback: make([]string, 0, 256),
		MaxLines:   p.config.ScrollbackSize,
		Health: PaneHealth{
			State: "idle",
		},
	}
	p.panes = append(p.panes, pane)
	idx := len(p.panes) - 1

	// If this is the first pane, make it active.
	if len(p.panes) == 1 {
		p.activeIdx = 0
	}

	return idx, nil
}

// RemovePane removes a pane by ID and adjusts the active index.
func (p *Panel) RemovePane(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, pane := range p.panes {
		if pane.ID == id {
			p.panes = append(p.panes[:i], p.panes[i+1:]...)

			// Adjust active index.
			if len(p.panes) == 0 {
				p.activeIdx = 0
			} else if p.activeIdx >= len(p.panes) {
				p.activeIdx = len(p.panes) - 1
			}
			return nil
		}
	}
	return fmt.Errorf("claudemux: pane %q not found", id)
}

// InputResult describes the outcome of routing an input event.
type InputResult struct {
	TargetPaneID string // Which pane should receive the input ("" if consumed by panel)
	Consumed     bool   // Whether the panel consumed the input (e.g., pane switch)
	Action       string // "switch", "scroll-up", "scroll-down", "forward", "none"
}

// RouteInput processes a keyboard input event and determines where it
// should go. Alt+1..9 switches panes, other input routes to the active pane.
//
// Key format follows BubbleTea conventions:
//   - "alt+1" through "alt+9" for pane switching
//   - "pgup"/"pgdown" for scrollback navigation
//   - Everything else forwards to the active pane
func (p *Panel) RouteInput(key string) InputResult {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != PanelActive || len(p.panes) == 0 {
		return InputResult{Action: "none"}
	}

	// Alt+1..9 switches panes.
	if len(key) == 5 && key[:4] == "alt+" && key[4] >= '1' && key[4] <= '9' {
		idx := int(key[4]-'1')
		if idx < len(p.panes) {
			p.activeIdx = idx
			return InputResult{
				Consumed: true,
				Action:   "switch",
			}
		}
		return InputResult{Action: "none"}
	}

	// PgUp/PgDown for scrollback navigation on active pane.
	if key == "pgup" {
		pane := p.panes[p.activeIdx]
		maxScroll := len(pane.Scrollback)
		if pane.ScrollPos < maxScroll {
			pane.ScrollPos += 20
			if pane.ScrollPos > maxScroll {
				pane.ScrollPos = maxScroll
			}
		}
		return InputResult{
			TargetPaneID: pane.ID,
			Consumed:     true,
			Action:       "scroll-up",
		}
	}
	if key == "pgdown" {
		pane := p.panes[p.activeIdx]
		if pane.ScrollPos > 0 {
			pane.ScrollPos -= 20
			if pane.ScrollPos < 0 {
				pane.ScrollPos = 0
			}
		}
		return InputResult{
			TargetPaneID: pane.ID,
			Consumed:     true,
			Action:       "scroll-down",
		}
	}

	// Forward to active pane.
	pane := p.panes[p.activeIdx]
	return InputResult{
		TargetPaneID: pane.ID,
		Action:       "forward",
	}
}

// SetActive changes the active pane by index (0-based).
func (p *Panel) SetActive(index int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if index < 0 || index >= len(p.panes) {
		return fmt.Errorf("claudemux: pane index %d out of range (0-%d)",
			index, len(p.panes)-1)
	}
	p.activeIdx = index
	return nil
}

// ActivePane returns the currently active pane, or nil if none.
func (p *Panel) ActivePane() *Pane {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.panes) == 0 {
		return nil
	}
	return p.panes[p.activeIdx]
}

// ActiveIndex returns the 0-based index of the active pane.
func (p *Panel) ActiveIndex() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.activeIdx
}

// AppendOutput adds a line to a pane's scrollback buffer. Trims
// the buffer if it exceeds MaxLines. Resets scroll position to bottom
// if the pane is currently at the bottom.
func (p *Panel) AppendOutput(paneID string, line string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pane := p.findPaneLocked(paneID)
	if pane == nil {
		return fmt.Errorf("claudemux: pane %q not found", paneID)
	}

	pane.Scrollback = append(pane.Scrollback, line)

	// Trim if over max.
	if len(pane.Scrollback) > pane.MaxLines {
		excess := len(pane.Scrollback) - pane.MaxLines
		pane.Scrollback = pane.Scrollback[excess:]
		// Adjust scroll position.
		if pane.ScrollPos > 0 {
			pane.ScrollPos -= excess
			if pane.ScrollPos < 0 {
				pane.ScrollPos = 0
			}
		}
	}

	return nil
}

// UpdateHealth updates the health indicators for a pane.
func (p *Panel) UpdateHealth(paneID string, health PaneHealth) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	pane := p.findPaneLocked(paneID)
	if pane == nil {
		return fmt.Errorf("claudemux: pane %q not found", paneID)
	}

	pane.Health = health
	return nil
}

// StatusBar returns a formatted status bar string showing all panes
// with their health state. The active pane is highlighted with [brackets].
func (p *Panel) StatusBar() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.statusBarLocked()
}

// healthIndicator returns a unicode character for health state.
func healthIndicator(state string) string {
	switch state {
	case "running":
		return "● " // green dot (caller can colorize)
	case "error":
		return "✖ " // red x
	case "idle":
		return "○ " // empty dot
	case "stopped":
		return "■ " // square
	default:
		return "? "
	}
}

// PanelSnapshot holds the complete panel state for rendering.
type PanelSnapshot struct {
	State     PanelState `json:"state"`
	StateName string     `json:"stateName"`
	ActiveIdx int        `json:"activeIdx"`
	Panes     []PaneSnapshot `json:"panes"`
	StatusBar string     `json:"statusBar"`
}

// PaneSnapshot holds a pane's state for rendering.
type PaneSnapshot struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Lines      int        `json:"lines"`
	ScrollPos  int        `json:"scrollPos"`
	Health     PaneHealth `json:"health"`
	IsActive   bool       `json:"isActive"`
}

// Snapshot returns the complete panel state.
func (p *Panel) Snapshot() PanelSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	snap := PanelSnapshot{
		State:     p.state,
		StateName: PanelStateName(p.state),
		ActiveIdx: p.activeIdx,
		Panes:     make([]PaneSnapshot, len(p.panes)),
		StatusBar: p.statusBarLocked(),
	}

	for i, pane := range p.panes {
		snap.Panes[i] = PaneSnapshot{
			ID:        pane.ID,
			Title:     pane.Title,
			Lines:     len(pane.Scrollback),
			ScrollPos: pane.ScrollPos,
			Health:    pane.Health,
			IsActive:  i == p.activeIdx,
		}
	}

	return snap
}

// Close transitions the panel to Closed state.
func (p *Panel) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = PanelClosed
	p.panes = nil
	p.activeIdx = 0
}

// Config returns a copy of the panel configuration.
func (p *Panel) Config() PanelConfig {
	return p.config
}

// PaneCount returns the number of panes.
func (p *Panel) PaneCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.panes)
}

// GetVisibleLines returns the visible scrollback lines for a pane,
// taking into account the scroll position. Returns up to `height` lines.
func (p *Panel) GetVisibleLines(paneID string, height int) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	pane := p.findPaneLocked(paneID)
	if pane == nil {
		return nil, fmt.Errorf("claudemux: pane %q not found", paneID)
	}

	total := len(pane.Scrollback)
	if total == 0 || height <= 0 {
		return nil, nil
	}

	// Calculate the window based on scroll position.
	end := total - pane.ScrollPos
	if end < 0 {
		end = 0
	}
	if end > total {
		end = total
	}
	start := end - height
	if start < 0 {
		start = 0
	}

	result := make([]string, end-start)
	copy(result, pane.Scrollback[start:end])
	return result, nil
}

// --- internal helpers ---

// findPaneLocked finds a pane by ID. Caller must hold p.mu.
func (p *Panel) findPaneLocked(id string) *Pane {
	for _, pane := range p.panes {
		if pane.ID == id {
			return pane
		}
	}
	return nil
}

// statusBarLocked generates the status bar. Caller must hold p.mu.
func (p *Panel) statusBarLocked() string {
	if len(p.panes) == 0 {
		return "[no panes]"
	}

	var parts []string
	for i, pane := range p.panes {
		indicator := healthIndicator(pane.Health.State)
		label := fmt.Sprintf("%d:%s%s", i+1, indicator, pane.Title)
		if i == p.activeIdx {
			label = "[" + label + "]"
		} else {
			label = " " + label + " "
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "|")
}
