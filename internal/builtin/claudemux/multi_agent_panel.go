package claudemux

import (
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// AgentOutputMsg carries a line of output from an agent.
type AgentOutputMsg struct {
	AgentID string
	Line    string
}

// AgentStatusMsg carries a health update for an agent.
type AgentStatusMsg struct {
	AgentID string
	Health  PaneHealth
}

// AgentJoinedMsg signals that a new agent has joined the panel.
type AgentJoinedMsg struct {
	AgentID string
	Title   string
	Role    string
}

// AgentLeftMsg signals that an agent has left the panel.
type AgentLeftMsg struct {
	AgentID string
}

// agentInfo tracks per-agent metadata within the multi-agent panel.
type agentInfo struct {
	agentID string
	title   string
	role    string
}

// MultiAgentPanel wraps the existing Panel with Bubble Tea integration
// and live agent output streaming. It implements tea.Model.
type MultiAgentPanel struct {
	panel    *Panel
	bus      *CoordinationBus
	registry *AgentRegistry

	width  int
	height int

	showDashboard bool

	agents map[string]*agentInfo

	mu sync.RWMutex
}

// NewMultiAgentPanel creates a MultiAgentPanel backed by the given Panel,
// CoordinationBus, and AgentRegistry.
func NewMultiAgentPanel(panel *Panel, bus *CoordinationBus, registry *AgentRegistry) *MultiAgentPanel {
	return &MultiAgentPanel{
		panel:    panel,
		bus:      bus,
		registry: registry,
		agents:   make(map[string]*agentInfo),
	}
}

// AddAgent adds a new agent to the panel, creating a corresponding pane
// and registering it in the internal agent map.
func (m *MultiAgentPanel) AddAgent(agentID, title, role string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[agentID]; exists {
		return fmt.Errorf("claudemux: agent %q already in multi-agent panel", agentID)
	}

	if _, err := m.panel.AddPane(agentID, title); err != nil {
		return err
	}

	m.agents[agentID] = &agentInfo{
		agentID: agentID,
		title:   title,
		role:    role,
	}

	return nil
}

// RemoveAgent removes an agent from the panel and the internal agent map.
func (m *MultiAgentPanel) RemoveAgent(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[agentID]; !exists {
		return fmt.Errorf("claudemux: agent %q not in multi-agent panel", agentID)
	}

	if err := m.panel.RemovePane(agentID); err != nil {
		return err
	}

	delete(m.agents, agentID)
	return nil
}

// SetSize updates the terminal dimensions for rendering.
func (m *MultiAgentPanel) SetSize(width, height int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.width = width
	m.height = height
}

// ToggleDashboard flips the dashboard visibility flag.
func (m *MultiAgentPanel) ToggleDashboard() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.showDashboard = !m.showDashboard
}

// Snapshot delegates to the underlying Panel's Snapshot method.
func (m *MultiAgentPanel) Snapshot() PanelSnapshot {
	return m.panel.Snapshot()
}

// SubscribeToAgent subscribes to agent output via the CoordinationBus.
// Incoming messages for the given agent are forwarded as AgentOutputMsg
// values through the returned channel. The caller is responsible for
// feeding those messages into the Bubble Tea Update loop.
func (m *MultiAgentPanel) SubscribeToAgent(agentID string) <-chan AgentOutputMsg {
	ch := make(chan AgentOutputMsg, 64)
	m.bus.Subscribe(agentID, func(msg CoordinationMessage) {
		ch <- AgentOutputMsg{
			AgentID: msg.From,
			Line:    string(msg.Payload),
		}
	})
	return ch
}

// Init implements tea.Model. Returns nil (no initial command).
func (m *MultiAgentPanel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Processes Bubble Tea messages and
// updates internal state accordingly.
func (m *MultiAgentPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case AgentOutputMsg:
		_ = m.panel.AppendOutput(msg.AgentID, msg.Line)
		return m, nil

	case AgentStatusMsg:
		_ = m.panel.UpdateHealth(msg.AgentID, msg.Health)
		return m, nil

	case AgentJoinedMsg:
		_ = m.AddAgent(msg.AgentID, msg.Title, msg.Role)
		return m, nil

	case AgentLeftMsg:
		_ = m.RemoveAgent(msg.AgentID)
		return m, nil

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		// Delegate key handling to the underlying panel via RouteInput.
		// Convert tea.KeyMsg to the string key format Panel expects.
		key := keyString(msg)
		if key != "" {
			m.panel.RouteInput(key)
		}
		return m, nil
	}

	return m, nil
}

// View implements tea.Model. Renders the status bar, optional dashboard,
// and the active pane's visible lines.
func (m *MultiAgentPanel) View() tea.View {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var b strings.Builder

	// Status bar.
	b.WriteString(m.panel.StatusBar())
	b.WriteByte('\n')

	// Optional dashboard.
	if m.showDashboard {
		b.WriteString(m.renderDashboard())
		b.WriteByte('\n')
	}

	// Active pane content.
	active := m.panel.ActivePane()
	if active != nil {
		height := m.height
		if height <= 0 {
			height = 24
		}
		// Reserve 1 line for status bar, 1 for optional separator.
		contentHeight := height - 1
		if m.showDashboard {
			contentHeight--
		}
		if contentHeight < 1 {
			contentHeight = 1
		}

		lines, _ := m.panel.GetVisibleLines(active.ID, contentHeight)
		for _, line := range lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	return tea.NewView(b.String())
}

// renderDashboard produces a summary of all agents with their health and roles.
// Caller must hold m.mu (at least RLock).
func (m *MultiAgentPanel) renderDashboard() string {
	var b strings.Builder
	b.WriteString("── Agents ──")

	// Collect agents in stable order from the registry.
	agentIDs := m.registry.List()
	if len(agentIDs) == 0 {
		// Fall back to local agent map if registry is empty.
		for id := range m.agents {
			agentIDs = append(agentIDs, id)
		}
	}

	for _, id := range agentIDs {
		info, ok := m.agents[id]
		if !ok {
			continue
		}
		_ = info // info is used below
		snap := m.panel.Snapshot()
		var ph PaneHealth
		for _, ps := range snap.Panes {
			if ps.ID == id {
				ph = ps.Health
				break
			}
		}
		indicator := healthIndicator(ph.State)
		b.WriteByte('\n')
		fmt.Fprintf(&b, "  %s%s [%s] %s", indicator, info.title, info.role, ph.State)
	}

	return b.String()
}

// keyString converts a tea.KeyMsg to the string format expected by Panel.RouteInput.
func keyString(msg tea.KeyMsg) string {
	// tea.KeyMsg in bubbletea v2 has a String() method that produces
	// representations like "alt+1", "pgup", "pgdown", etc.
	s := msg.String()
	// Normalize: bubbletea v2 may produce different formats; the Panel
	// expects "alt+1" through "alt+9", "pgup", "pgdown".
	return s
}
