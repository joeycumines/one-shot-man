package mux

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AutoSplitStep tracks the state of a single pipeline step.
type AutoSplitStep struct {
	Name      string
	Status    StepStatus
	Error     string
	StartedAt time.Time
	Elapsed   time.Duration
}

// StepStatus represents the state of a pipeline step.
type StepStatus int

const (
	// StepPending means the step has not started yet.
	StepPending StepStatus = iota
	// StepRunning means the step is currently executing.
	StepRunning
	// StepDone means the step completed successfully.
	StepDone
	// StepFailed means the step completed with an error.
	StepFailed
)

// stepIcon returns the Unicode icon for a step status.
func stepIcon(s StepStatus) string {
	switch s {
	case StepPending:
		return "○"
	case StepRunning:
		return "◉"
	case StepDone:
		return "✓"
	case StepFailed:
		return "✗"
	default:
		return "?"
	}
}

// --- BubbleTea Messages ---

// AutoSplitStepStartMsg signals a new pipeline step is starting.
type AutoSplitStepStartMsg struct {
	Name string
}

// AutoSplitStepDoneMsg signals a pipeline step finished.
type AutoSplitStepDoneMsg struct {
	Name    string
	Err     string
	Elapsed time.Duration
}

// AutoSplitOutputMsg appends text to the live output pane.
type AutoSplitOutputMsg struct {
	Text string
}

// AutoSplitErrorMsg appends an error line to the live output pane.
type AutoSplitErrorMsg struct {
	Text string
}

// AutoSplitDoneMsg signals the entire pipeline has finished.
type AutoSplitDoneMsg struct {
	Summary string
}

// autoSplitTickMsg is an internal tick for elapsed-time updates.
type autoSplitTickMsg time.Time

// --- Model ---

// AutoSplitModel is a BubbleTea model that visualises the auto-split
// pipeline progress. The top pane shows steps with status icons and
// elapsed times. The bottom pane shows live output from the pipeline
// (Claude responses, git operations, errors). A status bar separates
// the two panes.
//
// All public methods that mutate state are goroutine-safe; they send
// messages through the [tea.Program] rather than mutating directly.
type AutoSplitModel struct {
	// Terminal dimensions.
	width  int
	height int

	// Steps registered in the pipeline.
	steps []AutoSplitStep

	// Live output lines (capped ring buffer).
	outputLines []string
	maxLines    int

	// Pipeline state.
	done        bool
	doneSummary string
	quitting    bool

	// Internal: the running tea.Program, set after Run() is called.
	// Used by Send* methods to deliver messages from other goroutines.
	program *tea.Program

	// Styles.
	headerStyle    lipgloss.Style
	pendingStyle   lipgloss.Style
	runningStyle   lipgloss.Style
	doneStyle      lipgloss.Style
	failedStyle    lipgloss.Style
	separatorStyle lipgloss.Style
	outputStyle    lipgloss.Style
	errorStyle     lipgloss.Style

	mu sync.Mutex
}

// AutoSplitOption configures an [AutoSplitModel].
type AutoSplitOption func(*AutoSplitModel)

// WithAutoSplitMaxLines sets the maximum output lines retained.
func WithAutoSplitMaxLines(n int) AutoSplitOption {
	return func(m *AutoSplitModel) {
		if n < 10 {
			n = 10
		}
		m.maxLines = n
	}
}

// NewAutoSplitModel creates a new auto-split progress TUI.
func NewAutoSplitModel(opts ...AutoSplitOption) *AutoSplitModel {
	m := &AutoSplitModel{
		width:    80,
		height:   24,
		maxLines: 500,
		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")),
		pendingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")),
		runningStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")),
		doneStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")),
		failedStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196")),
		separatorStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("240")).
			Foreground(lipgloss.Color("255")).
			Bold(true),
		outputStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")),
		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// --- Public API (goroutine-safe, deliver via tea.Program.Send) ---

// SendStepStart notifies the TUI that a pipeline step has started.
// Safe for concurrent use from any goroutine.
func (m *AutoSplitModel) SendStepStart(name string) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()
	if p != nil {
		p.Send(AutoSplitStepStartMsg{Name: name})
	}
}

// SendStepDone notifies the TUI that a pipeline step has finished.
// Pass an empty errMsg for success.
func (m *AutoSplitModel) SendStepDone(name, errMsg string, elapsed time.Duration) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()
	if p != nil {
		p.Send(AutoSplitStepDoneMsg{Name: name, Err: errMsg, Elapsed: elapsed})
	}
}

// SendOutput appends text to the live output pane.
func (m *AutoSplitModel) SendOutput(text string) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()
	if p != nil {
		p.Send(AutoSplitOutputMsg{Text: text})
	}
}

// SendError appends an error line to the output pane.
func (m *AutoSplitModel) SendError(text string) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()
	if p != nil {
		p.Send(AutoSplitErrorMsg{Text: text})
	}
}

// SendDone signals that the pipeline is complete.
func (m *AutoSplitModel) SendDone(summary string) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()
	if p != nil {
		p.Send(AutoSplitDoneMsg{Summary: summary})
	}
}

// Run starts the BubbleTea program (alt-screen) and blocks until quit.
func (m *AutoSplitModel) Run() error {
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.mu.Lock()
	m.program = p
	m.mu.Unlock()
	_, err := p.Run()
	return err
}

// Program returns the underlying tea.Program, or nil if Run has not
// been called. Primarily for testing.
func (m *AutoSplitModel) Program() *tea.Program {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.program
}

// --- tea.Model implementation ---

// Init implements tea.Model. Starts the elapsed-time ticker.
func (m *AutoSplitModel) Init() tea.Cmd {
	return tickCmd()
}

// Update implements tea.Model.
func (m *AutoSplitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case AutoSplitStepStartMsg:
		m.ensureStep(msg.Name)
		for i := range m.steps {
			if m.steps[i].Name == msg.Name {
				m.steps[i].Status = StepRunning
				m.steps[i].StartedAt = time.Now()
				break
			}
		}
		return m, nil

	case AutoSplitStepDoneMsg:
		m.ensureStep(msg.Name)
		for i := range m.steps {
			if m.steps[i].Name == msg.Name {
				if msg.Err != "" {
					m.steps[i].Status = StepFailed
					m.steps[i].Error = msg.Err
				} else {
					m.steps[i].Status = StepDone
				}
				m.steps[i].Elapsed = msg.Elapsed
				break
			}
		}
		return m, nil

	case AutoSplitOutputMsg:
		lines := strings.Split(msg.Text, "\n")
		m.outputLines = appendCapped(m.outputLines, lines, m.maxLines)
		return m, nil

	case AutoSplitErrorMsg:
		errLine := "ERROR: " + msg.Text
		m.outputLines = appendCapped(m.outputLines, []string{errLine}, m.maxLines)
		return m, nil

	case AutoSplitDoneMsg:
		m.done = true
		m.doneSummary = msg.Summary
		return m, nil

	case autoSplitTickMsg:
		// Re-render to update elapsed times on running steps.
		if m.quitting {
			return m, nil
		}
		return m, tickCmd()
	}

	return m, nil
}

// View implements tea.Model.
func (m *AutoSplitModel) View() string {
	if m.quitting {
		return ""
	}

	// Layout: top = step list, separator, bottom = live output.
	// Reserve 1 line for separator, 1 for header.
	availableHeight := m.height
	if availableHeight < 4 {
		availableHeight = 4
	}

	// Top pane: header line + one line per step, capped at 40% of terminal.
	topMax := availableHeight * 2 / 5
	stepCount := len(m.steps)
	topNeeded := stepCount + 1 // +1 for header
	if topNeeded > topMax {
		topNeeded = topMax
	}
	if topNeeded < 2 {
		topNeeded = 2
	}

	// Bottom pane: remaining height minus separator.
	bottomHeight := availableHeight - topNeeded - 1
	if bottomHeight < 1 {
		bottomHeight = 1
	}

	// Render top pane (step list).
	topContent := m.renderSteps(topNeeded, m.width)

	// Separator bar.
	separator := m.renderSeparator(m.width)

	// Render bottom pane (live output).
	bottomContent := renderPane(m.outputLines, bottomHeight, m.width)

	return lipgloss.JoinVertical(lipgloss.Left, topContent, separator, bottomContent)
}

// --- Internal helpers ---

// ensureStep adds a step entry if it doesn't already exist.
func (m *AutoSplitModel) ensureStep(name string) {
	for _, s := range m.steps {
		if s.Name == name {
			return
		}
	}
	m.steps = append(m.steps, AutoSplitStep{
		Name:   name,
		Status: StepPending,
	})
}

// renderSteps renders the step list for the top pane.
func (m *AutoSplitModel) renderSteps(height, width int) string {
	var b strings.Builder

	header := m.headerStyle.Render("  Auto-Split Pipeline")
	b.WriteString(truncate(header, width))
	b.WriteByte('\n')

	// Show last (height-1) steps if there are more than fit.
	steps := m.steps
	startIdx := 0
	slotsForSteps := height - 1
	if len(steps) > slotsForSteps {
		startIdx = len(steps) - slotsForSteps
	}

	for i := startIdx; i < len(steps); i++ {
		s := steps[i]
		icon := stepIcon(s.Status)
		var style lipgloss.Style
		switch s.Status {
		case StepPending:
			style = m.pendingStyle
		case StepRunning:
			style = m.runningStyle
		case StepDone:
			style = m.doneStyle
		case StepFailed:
			style = m.failedStyle
		}

		// Format elapsed time.
		var elapsed string
		switch {
		case s.Status == StepRunning:
			elapsed = fmt.Sprintf(" (%s)", formatDuration(time.Since(s.StartedAt)))
		case s.Elapsed > 0:
			elapsed = fmt.Sprintf(" (%s)", formatDuration(s.Elapsed))
		}

		line := style.Render(fmt.Sprintf("  %s %s%s", icon, s.Name, elapsed))
		if s.Status == StepFailed && s.Error != "" {
			line += m.failedStyle.Render(fmt.Sprintf(" — %s", s.Error))
		}

		b.WriteString(truncate(line, width))
		if i < len(steps)-1 {
			b.WriteByte('\n')
		}
	}

	// Pad remaining lines.
	rendered := startIdx
	for j := len(steps) - startIdx; j < slotsForSteps; j++ {
		_ = rendered // suppress unused
		b.WriteByte('\n')
	}

	return b.String()
}

// renderSeparator builds the status bar between panes.
func (m *AutoSplitModel) renderSeparator(width int) string {
	// Count step statuses.
	var total, doneCount, failCount int
	var currentStep string
	total = len(m.steps)
	for _, s := range m.steps {
		switch s.Status {
		case StepDone:
			doneCount++
		case StepFailed:
			failCount++
		case StepRunning:
			currentStep = s.Name
		}
	}

	left := " Auto-Split"
	if currentStep != "" {
		left = fmt.Sprintf(" ◉ %s", currentStep)
	}
	if m.done {
		left = " ✓ Complete"
	}

	var right string
	if total > 0 {
		right = fmt.Sprintf(" %d/%d ", doneCount+failCount, total)
		if failCount > 0 {
			right = fmt.Sprintf(" %d/%d (%d failed) ", doneCount+failCount, total, failCount)
		}
	}

	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}
	bar := left + strings.Repeat("─", padding) + right
	return m.separatorStyle.Width(width).Render(bar)
}

// formatDuration formats a duration in a human-friendly short form.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// truncate limits a string to width characters (simple byte-level).
func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width < 1 {
		return ""
	}
	return s[:width]
}

// tickCmd returns a command that delivers a tick after 200ms.
func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return autoSplitTickMsg(t)
	})
}
