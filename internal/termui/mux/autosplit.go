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
	Detail    string // Sub-step progress detail (e.g. "Classifying 15/42 files...")
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

// AutoSplitStepDetailMsg updates the sub-step progress detail for a
// running step (e.g. "Classifying 15/42 files...").
type AutoSplitStepDetailMsg struct {
	Name   string
	Detail string
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

// AutoSplitToggleMsg is sent when the user presses the toggle key (Ctrl+])
// to switch between the auto-split TUI and Claude's terminal.
type AutoSplitToggleMsg struct{}

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

	// Output scroll offset — 0 means "at bottom" (auto-scroll),
	// positive means scrolled up by N lines.
	scrollOffset int

	// Pipeline state.
	done        bool
	doneSummary string
	quitting    bool
	cancelled   bool // set atomically when user cancels; polled by JS

	// Pipeline timer — set on first StepStartMsg.
	pipelineStartedAt time.Time

	// Toggle key (default Ctrl+]) for switching to Claude TUI.
	toggleKey byte
	// Toggle callback — invoked (outside BubbleTea) when toggle is pressed.
	// The callback should block until the user toggles back.
	onToggle func()

	// Internal: the running tea.Program, set after Run() is called.
	// Used by Send* methods to deliver messages from other goroutines.
	program *tea.Program

	// Styles.
	headerStyle    lipgloss.Style
	pendingStyle   lipgloss.Style
	runningStyle   lipgloss.Style
	doneStyle      lipgloss.Style
	failedStyle    lipgloss.Style
	detailStyle    lipgloss.Style
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

// WithAutoSplitToggleKey sets the key used to switch to Claude TUI.
func WithAutoSplitToggleKey(key byte) AutoSplitOption {
	return func(m *AutoSplitModel) {
		m.toggleKey = key
	}
}

// WithAutoSplitOnToggle sets the callback invoked when the toggle key
// is pressed. The callback should block until the user toggles back.
func WithAutoSplitOnToggle(fn func()) AutoSplitOption {
	return func(m *AutoSplitModel) {
		m.onToggle = fn
	}
}

// NewAutoSplitModel creates a new auto-split progress TUI.
func NewAutoSplitModel(opts ...AutoSplitOption) *AutoSplitModel {
	m := &AutoSplitModel{
		width:     80,
		height:    24,
		maxLines:  500,
		toggleKey: DefaultToggleKey,
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
		detailStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true),
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

// send delivers a message to the running BubbleTea program.
// Safe for concurrent use; no-op if the program has not started.
func (m *AutoSplitModel) send(msg tea.Msg) {
	m.mu.Lock()
	p := m.program
	m.mu.Unlock()
	if p != nil {
		p.Send(msg)
	}
}

// SendStepStart notifies the TUI that a pipeline step has started.
// Safe for concurrent use from any goroutine.
func (m *AutoSplitModel) SendStepStart(name string) {
	m.send(AutoSplitStepStartMsg{Name: name})
}

// SendStepDone notifies the TUI that a pipeline step has finished.
// Pass an empty errMsg for success.
func (m *AutoSplitModel) SendStepDone(name, errMsg string, elapsed time.Duration) {
	m.send(AutoSplitStepDoneMsg{Name: name, Err: errMsg, Elapsed: elapsed})
}

// SendOutput appends text to the live output pane.
func (m *AutoSplitModel) SendOutput(text string) {
	m.send(AutoSplitOutputMsg{Text: text})
}

// SendError appends an error line to the output pane.
func (m *AutoSplitModel) SendError(text string) {
	m.send(AutoSplitErrorMsg{Text: text})
}

// SendDone signals that the pipeline is complete.
func (m *AutoSplitModel) SendDone(summary string) {
	m.send(AutoSplitDoneMsg{Summary: summary})
}

// SendStepDetail updates the sub-step progress detail for a running
// step (e.g. "Classifying 15/42 files..."). The detail is displayed
// inline after the step name in the progress view.
func (m *AutoSplitModel) SendStepDetail(name, detail string) {
	m.send(AutoSplitStepDetailMsg{Name: name, Detail: detail})
}

// Cancelled returns true if the user has pressed q/Ctrl+C to cancel
// the pipeline. This is goroutine-safe and designed to be polled by
// the JS pipeline to implement cooperative cancellation.
func (m *AutoSplitModel) Cancelled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancelled
}

// Quit programmatically triggers a clean shutdown of the TUI. The
// cancelled flag is set so that the JS pipeline can detect it.
func (m *AutoSplitModel) Quit() {
	m.mu.Lock()
	m.cancelled = true
	m.mu.Unlock()
	m.send(tea.Quit())
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
		// Cancel / quit.
		//
		// First press: set cancelled flag so the JS pipeline can detect
		// it via cooperative polling. The TUI stays visible showing a
		// "Cancelling…" state. This prevents the alt-screen from
		// being torn down while the pipeline is still running (which
		// would expose the go-prompt beneath in an unresponsive state).
		//
		// Second press: force-quit the BubbleTea program. This is the
		// fallback for pipelines that are truly stuck (e.g. blocked on
		// an unresponsive child process).
		//
		// When the pipeline finishes, it sends AutoSplitDoneMsg which
		// triggers the actual tea.Quit.
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			if m.done {
				// Pipeline has finished — dismiss the TUI.
				m.quitting = true
				return m, tea.Quit
			}
			m.mu.Lock()
			alreadyCancelled := m.cancelled
			m.cancelled = true
			m.mu.Unlock()
			if alreadyCancelled {
				// Second press: force quit.
				m.quitting = true
				return m, tea.Quit
			}
			// First press: stay visible, show "Cancelling…"
			return m, nil
		}

		// Enter key on done screen: dismiss the TUI.
		if msg.Type == tea.KeyEnter && m.done {
			m.quitting = true
			return m, tea.Quit
		}

		// Toggle key (default Ctrl+]) — switch to Claude TUI.
		if (len(msg.Runes) == 1 && byte(msg.Runes[0]) == m.toggleKey) ||
			(msg.Type == tea.KeyCtrlCloseBracket && m.toggleKey == DefaultToggleKey) {
			if m.onToggle != nil {
				// Temporarily release alt-screen so Claude can use the terminal.
				// The onToggle callback blocks until the user toggles back.
				return m, func() tea.Msg {
					m.onToggle()
					return AutoSplitToggleMsg{}
				}
			}
		}

		// Output pane scrolling.
		switch msg.Type {
		case tea.KeyUp:
			m.scrollUp(1)
		case tea.KeyDown:
			m.scrollDown(1)
		case tea.KeyPgUp:
			m.scrollUp(m.outputPaneHeight() / 2)
		case tea.KeyPgDown:
			m.scrollDown(m.outputPaneHeight() / 2)
		case tea.KeyHome:
			maxOffset := len(m.outputLines) - m.outputPaneHeight()
			if maxOffset < 0 {
				maxOffset = 0
			}
			m.scrollOffset = maxOffset
		case tea.KeyEnd:
			m.scrollOffset = 0
		}
		return m, nil

	case AutoSplitStepStartMsg:
		if m.pipelineStartedAt.IsZero() {
			m.pipelineStartedAt = time.Now()
		}
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
				m.steps[i].Detail = "" // Clear detail on completion.
				break
			}
		}
		return m, nil

	case AutoSplitStepDetailMsg:
		m.ensureStep(msg.Name)
		for i := range m.steps {
			if m.steps[i].Name == msg.Name {
				m.steps[i].Detail = msg.Detail
				break
			}
		}
		return m, nil

	case AutoSplitOutputMsg:
		lines := strings.Split(msg.Text, "\n")
		m.outputLines = appendCapped(m.outputLines, lines, m.maxLines)
		// scrollOffset == 0 means "follow the tail" — no adjustment needed.
		// If the user has scrolled up (scrollOffset > 0) the viewport stays
		// where it is; the new content appends below the visible region.
		return m, nil

	case AutoSplitErrorMsg:
		errLine := "ERROR: " + msg.Text
		m.outputLines = appendCapped(m.outputLines, []string{errLine}, m.maxLines)
		return m, nil

	case AutoSplitDoneMsg:
		m.done = true
		m.doneSummary = msg.Summary
		// If the user already pressed cancel, exit immediately now that
		// the pipeline has cleaned up. Otherwise, stay visible and show
		// the summary until the user dismisses with q or Enter.
		m.mu.Lock()
		wasCancelled := m.cancelled
		m.mu.Unlock()
		if wasCancelled {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case AutoSplitToggleMsg:
		// Returned from Claude TUI — refresh display.
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

	layout := m.computeLayout()

	// Render top pane (step list).
	topContent := m.renderSteps(layout.topHeight, m.width)

	// Separator bar.
	separator := m.renderSeparator(m.width)

	// Render bottom pane (live output) with scroll offset.
	viewLines := m.outputLines
	if m.scrollOffset > 0 && len(viewLines) > layout.bottomHeight {
		endIdx := len(viewLines) - m.scrollOffset
		if endIdx < layout.bottomHeight {
			endIdx = layout.bottomHeight
		}
		if endIdx > len(viewLines) {
			endIdx = len(viewLines)
		}
		viewLines = viewLines[:endIdx]
	}
	bottomContent := renderPane(viewLines, layout.bottomHeight, m.width)

	// Help bar.
	helpBar := m.renderHelpBar(m.width)

	return lipgloss.JoinVertical(lipgloss.Left, topContent, separator, bottomContent, helpBar)
}

// --- Internal helpers ---

// scrollUp scrolls the output pane up (into history) by n lines.
func (m *AutoSplitModel) scrollUp(n int) {
	maxOffset := len(m.outputLines) - m.outputPaneHeight()
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.scrollOffset += n
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

// scrollDown scrolls the output pane towards the bottom by n lines.
// An offset of 0 means "follow the tail" (auto-scroll).
func (m *AutoSplitModel) scrollDown(n int) {
	m.scrollOffset -= n
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// autoSplitLayout holds the computed pane dimensions for a single render.
type autoSplitLayout struct {
	topHeight    int // Height of the step list pane (including header).
	bottomHeight int // Height of the output pane.
}

// computeLayout calculates the split between the step pane (top) and
// the output pane (bottom), reserving 1 line each for the separator
// and help bar. The top pane is capped at 40% of the terminal.
func (m *AutoSplitModel) computeLayout() autoSplitLayout {
	availableHeight := m.height
	if availableHeight < 5 {
		availableHeight = 5
	}
	topMax := availableHeight * 2 / 5
	topNeeded := len(m.steps) + 1 // +1 for header
	if topNeeded > topMax {
		topNeeded = topMax
	}
	if topNeeded < 2 {
		topNeeded = 2
	}
	bottomHeight := availableHeight - topNeeded - 2 // -1 separator, -1 help bar
	if bottomHeight < 1 {
		bottomHeight = 1
	}
	return autoSplitLayout{topHeight: topNeeded, bottomHeight: bottomHeight}
}

// outputPaneHeight calculates the current height of the bottom (output) pane.
func (m *AutoSplitModel) outputPaneHeight() int {
	return m.computeLayout().bottomHeight
}

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

	headerText := "  Auto-Split Pipeline"
	if !m.pipelineStartedAt.IsZero() {
		elapsed := time.Since(m.pipelineStartedAt)
		// If done, freeze the timer at the last step's end time.
		if m.done && len(m.steps) > 0 {
			var lastEnd time.Duration
			for _, s := range m.steps {
				if s.StartedAt.IsZero() {
					continue // Skip steps that were never started.
				}
				if e := s.StartedAt.Sub(m.pipelineStartedAt) + s.Elapsed; e > lastEnd {
					lastEnd = e
				}
			}
			if lastEnd > 0 {
				elapsed = lastEnd
			}
		}
		headerText += fmt.Sprintf("  ⏱ %s", formatDuration(elapsed))
	}
	header := m.headerStyle.Render(headerText)
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

		// Show step counter (1-indexed).
		counter := fmt.Sprintf("%d/%d", i+1, len(steps))
		line := style.Render(fmt.Sprintf("  %s %s %s%s", icon, counter, s.Name, elapsed))
		if s.Status == StepFailed && s.Error != "" {
			line += m.failedStyle.Render(fmt.Sprintf(" — %s", s.Error))
		}
		// Show sub-step detail for running steps (e.g. "Classifying 15/42 files...").
		if s.Detail != "" && s.Status == StepRunning {
			line += m.detailStyle.Render(fmt.Sprintf(" · %s", s.Detail))
		}

		b.WriteString(truncate(line, width))
		if i < len(steps)-1 {
			b.WriteByte('\n')
		}
	}

	// Pad remaining lines.
	for j := len(steps) - startIdx; j < slotsForSteps; j++ {
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

	// Left side: status message.
	m.mu.Lock()
	isCancelled := m.cancelled
	m.mu.Unlock()

	var left string
	switch {
	case m.done && isCancelled:
		left = " ⏹ Cancelled"
	case m.done:
		left = " ✓ Complete — press q to dismiss"
	case isCancelled:
		left = " ⏳ Cancelling… (q again to force)"
	case currentStep != "":
		left = fmt.Sprintf(" ◉ %s", currentStep)
	default:
		left = " Auto-Split"
	}

	var right string
	if total > 0 {
		right = fmt.Sprintf(" %d/%d ", doneCount+failCount, total)
		if failCount > 0 {
			right = fmt.Sprintf(" %d/%d (%d failed) ", doneCount+failCount, total, failCount)
		}
	}

	// Show scroll indicator when user has scrolled up.
	if m.scrollOffset > 0 {
		right += fmt.Sprintf("▲%d ", m.scrollOffset)
	}

	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}
	bar := left + strings.Repeat("─", padding) + right

	// Use a warning style for the separator when cancelling.
	if isCancelled && !m.done {
		cancelStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("208")).
			Foreground(lipgloss.Color("0")).
			Bold(true)
		return cancelStyle.Width(width).Render(bar)
	}

	return m.separatorStyle.Width(width).Render(bar)
}

// renderHelpBar builds the contextual help bar at the bottom.
func (m *AutoSplitModel) renderHelpBar(width int) string {
	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)

	m.mu.Lock()
	isCancelled := m.cancelled
	m.mu.Unlock()

	var help string
	switch {
	case m.done:
		help = keyStyle.Render("q") + helpStyle.Render("/") +
			keyStyle.Render("enter") + helpStyle.Render(" dismiss")
	case isCancelled:
		help = keyStyle.Render("q") + helpStyle.Render(" force quit")
	default:
		help = keyStyle.Render("q") + helpStyle.Render(" cancel  ") +
			keyStyle.Render("ctrl+]") + helpStyle.Render(" claude  ") +
			keyStyle.Render("↑↓") + helpStyle.Render(" scroll  ") +
			keyStyle.Render("home/end") + helpStyle.Render(" jump")
	}

	// Pad to width.
	padding := width - lipgloss.Width(help) - 1
	if padding < 0 {
		padding = 0
	}
	return " " + help + strings.Repeat(" ", padding)
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
