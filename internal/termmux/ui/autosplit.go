package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termmux"
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

// --- Per-Branch Verification Messages ---

// BranchVerifyStatus represents the verification state of a single branch.
type BranchVerifyStatus int

const (
	// BranchPending means verification has not started.
	BranchPending BranchVerifyStatus = iota
	// BranchRunning means verification is in progress.
	BranchRunning
	// BranchPassed means verification succeeded.
	BranchPassed
	// BranchFailed means verification failed.
	BranchFailed
	// BranchSkipped means verification was skipped (dependency failure).
	BranchSkipped
	// BranchPreExistingFailure means the baseline check failed (T25).
	BranchPreExistingFailure
)

// branchIcon returns the Unicode icon for a branch verification status.
func branchIcon(s BranchVerifyStatus) string {
	switch s {
	case BranchPending:
		return "○"
	case BranchRunning:
		return "⟳"
	case BranchPassed:
		return "✓"
	case BranchFailed:
		return "✗"
	case BranchSkipped:
		return "⊘"
	case BranchPreExistingFailure:
		return "⚠"
	default:
		return "?"
	}
}

// BranchVerifyState tracks the verification state of a single branch.
type BranchVerifyState struct {
	Name     string
	Status   BranchVerifyStatus
	ExitCode int
	Error    string
	Elapsed  time.Duration
	Output   []string // captured verification output lines (T38)
}

// maxBranchOutputLines is the maximum number of output lines stored per branch.
const maxBranchOutputLines = 200

// AutoSplitBranchVerifyStartMsg signals that verification of a branch started.
type AutoSplitBranchVerifyStartMsg struct {
	Branch string
}

// AutoSplitBranchVerifyOutputMsg carries a single line of verification
// output for a specific branch.
type AutoSplitBranchVerifyOutputMsg struct {
	Branch string
	Line   string
}

// AutoSplitBranchVerifyDoneMsg signals that verification of a branch completed.
type AutoSplitBranchVerifyDoneMsg struct {
	Branch   string
	Passed   bool
	ExitCode int
	Error    string
	Elapsed  time.Duration
	Skipped  bool // dependency failure
	PreExist bool // pre-existing failure (T25)
}

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
	cancelled   bool // set on first q/Ctrl+C; polled by JS
	forceCancel bool // set on second q/Ctrl+C; signals JS to kill children immediately
	paused      bool // T40: set on Ctrl-P; polled by JS at step boundaries

	// Pipeline timer — set on first StepStartMsg.
	pipelineStartedAt time.Time

	// Per-branch verification state (T36).
	branches []BranchVerifyState

	// T38: Expanded branch detail view.
	// -1 means no branch is expanded; otherwise index into branches.
	expandedBranch int
	// Scroll offset within the expanded branch output.
	branchScrollOffset int
	// branchCursor is the currently highlighted branch index (-1 = none).
	branchCursor int

	// Toggle key (default Ctrl+]) for switching to Claude TUI.
	toggleKey byte
	// Toggle callback — invoked (outside BubbleTea) when toggle is pressed.
	// The callback should block until the user toggles back.
	onToggle func()

	// T17: Bell-based attention indicator. Set to true when Claude pane
	// emits a bell while in the background; cleared when user toggles to
	// Claude pane and returns.
	needsAttention bool

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
		width:          80,
		height:         24,
		maxLines:       500,
		expandedBranch: -1,
		toggleKey:      termmux.DefaultToggleKey,
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

// SendBranchVerifyStart notifies the TUI that verification of a branch started.
func (m *AutoSplitModel) SendBranchVerifyStart(branch string) {
	m.send(AutoSplitBranchVerifyStartMsg{Branch: branch})
}

// SendBranchVerifyOutput sends a single line of verification output for a branch.
func (m *AutoSplitModel) SendBranchVerifyOutput(branch, line string) {
	m.send(AutoSplitBranchVerifyOutputMsg{Branch: branch, Line: line})
}

// SendBranchVerifyDone notifies the TUI that verification of a branch completed.
func (m *AutoSplitModel) SendBranchVerifyDone(msg AutoSplitBranchVerifyDoneMsg) {
	m.send(msg)
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

// ForceCancelled returns true if the user has pressed q/Ctrl+C twice,
// signalling that child processes should be killed immediately rather
// than waiting for graceful shutdown.
func (m *AutoSplitModel) ForceCancelled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.forceCancel
}

// Paused returns true if the user has pressed Ctrl-P to request a
// pause. The JS pipeline should save a checkpoint and exit cleanly.
func (m *AutoSplitModel) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

// SetNeedsAttention sets the attention indicator for T17. When set to
// true, a persistent indicator appears in the separator showing that
// Claude needs attention. The indicator is cleared when the user
// toggles to Claude and returns (via AutoSplitToggleMsg).
func (m *AutoSplitModel) SetNeedsAttention(v bool) {
	m.mu.Lock()
	m.needsAttention = v
	m.mu.Unlock()
}

// NeedsAttention returns the current attention indicator state.
func (m *AutoSplitModel) NeedsAttention() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.needsAttention
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

// Steps returns a snapshot of all registered pipeline steps for
// observation by integration tests and diagnostic tools. The returned
// slice is a copy — mutations do not affect the model. Goroutine-safe.
func (m *AutoSplitModel) Steps() []AutoSplitStep {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]AutoSplitStep, len(m.steps))
	copy(out, m.steps)
	return out
}

// OutputLines returns a snapshot of the live output pane contents for
// observation by integration tests. The returned slice is a copy —
// mutations do not affect the model. Goroutine-safe.
func (m *AutoSplitModel) OutputLines() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.outputLines))
	copy(out, m.outputLines)
	return out
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
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			if m.done {
				m.quitting = true
				return m, tea.Quit
			}
			m.mu.Lock()
			alreadyCancelled := m.cancelled
			m.cancelled = true
			if alreadyCancelled {
				m.forceCancel = true
			}
			m.mu.Unlock()
			return m, nil
		}

		// T40: Pause — Ctrl-P saves checkpoint and exits cleanly.
		if msg.Type == tea.KeyCtrlP && !m.done {
			m.mu.Lock()
			m.paused = true
			m.mu.Unlock()
			return m, nil
		}

		// Enter key on done screen: dismiss the TUI (unless branch detail is active).
		if msg.Type == tea.KeyEnter && m.done {
			// T38: if a branch is expanded, collapse it first.
			if m.expandedBranch >= 0 {
				m.expandedBranch = -1
				m.branchScrollOffset = 0
				return m, nil
			}
			// T38: if cursor is on a failed branch, expand it instead of quitting.
			if len(m.branches) > 0 && m.branchCursor >= 0 && m.branchCursor < len(m.branches) {
				br := m.branches[m.branchCursor]
				if br.Status == BranchFailed || br.Status == BranchPreExistingFailure {
					m.expandedBranch = m.branchCursor
					m.branchScrollOffset = 0
					return m, nil
				}
			}
			m.quitting = true
			return m, tea.Quit
		}

		// T38: Branch detail view — collapse on Escape.
		if msg.Type == tea.KeyEsc && m.expandedBranch >= 0 {
			m.expandedBranch = -1
			m.branchScrollOffset = 0
			return m, nil
		}

		// T38: Branch navigation (j/k) and expansion (Enter).
		if len(m.branches) > 0 {
			if m.expandedBranch >= 0 {
				// Inside expanded view: j/k/up/down scroll output.
				switch {
				case msg.String() == "j" || msg.Type == tea.KeyDown:
					m.branchScrollDown(1)
					return m, nil
				case msg.String() == "k" || msg.Type == tea.KeyUp:
					m.branchScrollUp(1)
					return m, nil
				case msg.Type == tea.KeyEnter:
					m.expandedBranch = -1
					m.branchScrollOffset = 0
					return m, nil
				}
			} else {
				// Outside expanded view: j/k moves cursor, Enter expands.
				switch {
				case msg.String() == "j":
					if m.branchCursor < len(m.branches)-1 {
						m.branchCursor++
					}
					return m, nil
				case msg.String() == "k":
					if m.branchCursor > 0 {
						m.branchCursor--
					}
					return m, nil
				case msg.Type == tea.KeyEnter && !m.done:
					// Enter only expands when cursor is on a failed/pre-existing branch.
					if m.branchCursor >= 0 && m.branchCursor < len(m.branches) {
						br := m.branches[m.branchCursor]
						if br.Status == BranchFailed || br.Status == BranchPreExistingFailure {
							m.expandedBranch = m.branchCursor
							m.branchScrollOffset = 0
							return m, nil
						}
					}
				}
			}
		}

		// Toggle key (default Ctrl+]) — switch to Claude TUI.
		if (len(msg.Runes) == 1 && byte(msg.Runes[0]) == m.toggleKey) ||
			(msg.Type == tea.KeyCtrlCloseBracket && m.toggleKey == termmux.DefaultToggleKey) {
			if m.onToggle != nil {
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
				m.steps[i].Detail = ""
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
		return m, nil

	case AutoSplitErrorMsg:
		errLine := "ERROR: " + msg.Text
		m.outputLines = appendCapped(m.outputLines, []string{errLine}, m.maxLines)
		return m, nil

	case AutoSplitDoneMsg:
		m.done = true
		m.doneSummary = msg.Summary
		m.mu.Lock()
		wasCancelled := m.cancelled
		m.mu.Unlock()
		if wasCancelled {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case AutoSplitBranchVerifyStartMsg:
		m.ensureBranch(msg.Branch)
		for i := range m.branches {
			if m.branches[i].Name == msg.Branch {
				m.branches[i].Status = BranchRunning
				m.branches[i].Output = nil // reset on (re-)start
				break
			}
		}
		return m, nil

	case AutoSplitBranchVerifyOutputMsg:
		m.ensureBranch(msg.Branch)
		for i := range m.branches {
			if m.branches[i].Name == msg.Branch {
				if len(m.branches[i].Output) < maxBranchOutputLines {
					m.branches[i].Output = append(m.branches[i].Output, msg.Line)
				}
				break
			}
		}
		return m, nil

	case AutoSplitBranchVerifyDoneMsg:
		m.ensureBranch(msg.Branch)
		for i := range m.branches {
			if m.branches[i].Name == msg.Branch {
				if msg.Skipped {
					m.branches[i].Status = BranchSkipped
				} else if msg.PreExist {
					m.branches[i].Status = BranchPreExistingFailure
				} else if msg.Passed {
					m.branches[i].Status = BranchPassed
				} else {
					m.branches[i].Status = BranchFailed
				}
				m.branches[i].ExitCode = msg.ExitCode
				m.branches[i].Error = msg.Error
				m.branches[i].Elapsed = msg.Elapsed
				break
			}
		}
		return m, nil

	case AutoSplitToggleMsg:
		// T17: User has returned from Claude TUI — clear attention indicator.
		m.mu.Lock()
		m.needsAttention = false
		m.mu.Unlock()
		return m, nil

	case autoSplitTickMsg:
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

	topContent := m.renderSteps(layout.topHeight, m.width)
	separator := m.renderSeparator(m.width)

	var bottomContent string

	// T38: When a branch is expanded, replace the output pane with branch details.
	if m.expandedBranch >= 0 && m.expandedBranch < len(m.branches) {
		bottomContent = m.renderBranchDetail(layout.bottomHeight, m.width)
	} else {
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
		bottomContent = renderPane(viewLines, layout.bottomHeight, m.width)
	}

	helpBar := m.renderHelpBar(m.width)

	return lipgloss.JoinVertical(lipgloss.Left, topContent, separator, bottomContent, helpBar)
}

// --- Internal helpers ---

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

func (m *AutoSplitModel) scrollDown(n int) {
	m.scrollOffset -= n
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// branchScrollUp scrolls the expanded branch output view up (towards older lines).
func (m *AutoSplitModel) branchScrollUp(n int) {
	if m.expandedBranch < 0 || m.expandedBranch >= len(m.branches) {
		return
	}
	lines := len(m.branches[m.expandedBranch].Output)
	viewH := m.branchDetailHeight()
	maxOffset := lines - viewH
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.branchScrollOffset += n
	if m.branchScrollOffset > maxOffset {
		m.branchScrollOffset = maxOffset
	}
}

// branchScrollDown scrolls the expanded branch output view down (towards newer lines).
func (m *AutoSplitModel) branchScrollDown(n int) {
	m.branchScrollOffset -= n
	if m.branchScrollOffset < 0 {
		m.branchScrollOffset = 0
	}
}

// branchDetailHeight returns the number of visible lines for the expanded
// branch detail pane. Defaults to the output pane height.
func (m *AutoSplitModel) branchDetailHeight() int {
	h := m.outputPaneHeight()
	if h < 3 {
		h = 3
	}
	return h
}

type autoSplitLayout struct {
	topHeight    int
	bottomHeight int
}

func (m *AutoSplitModel) computeLayout() autoSplitLayout {
	availableHeight := m.height
	if availableHeight < 5 {
		availableHeight = 5
	}
	topMax := availableHeight * 2 / 5
	topNeeded := len(m.steps) + 1
	if topNeeded > topMax {
		topNeeded = topMax
	}
	if topNeeded < 2 {
		topNeeded = 2
	}
	bottomHeight := availableHeight - topNeeded - 2
	if bottomHeight < 1 {
		bottomHeight = 1
	}
	return autoSplitLayout{topHeight: topNeeded, bottomHeight: bottomHeight}
}

func (m *AutoSplitModel) outputPaneHeight() int {
	return m.computeLayout().bottomHeight
}

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

func (m *AutoSplitModel) ensureBranch(name string) {
	for _, b := range m.branches {
		if b.Name == name {
			return
		}
	}
	m.branches = append(m.branches, BranchVerifyState{
		Name:   name,
		Status: BranchPending,
	})
}

// branchSummaryLine returns a compact summary like "3/5 passed, 1 failed, 1 skipped".
func (m *AutoSplitModel) branchSummaryLine() string {
	var passed, failed, skipped, running, pending int
	for _, b := range m.branches {
		switch b.Status {
		case BranchPassed:
			passed++
		case BranchFailed:
			failed++
		case BranchSkipped, BranchPreExistingFailure:
			skipped++
		case BranchRunning:
			running++
		case BranchPending:
			pending++
		}
	}
	total := len(m.branches)
	parts := []string{fmt.Sprintf("%d/%d passed", passed, total)}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", pending))
	}
	return strings.Join(parts, ", ")
}

func (m *AutoSplitModel) renderSteps(height, width int) string {
	var b strings.Builder

	headerText := "  Auto-Split Pipeline"
	if !m.pipelineStartedAt.IsZero() {
		elapsed := time.Since(m.pipelineStartedAt)
		if m.done && len(m.steps) > 0 {
			var lastEnd time.Duration
			for _, s := range m.steps {
				if s.StartedAt.IsZero() {
					continue
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

		var elapsed string
		switch {
		case s.Status == StepRunning:
			elapsed = fmt.Sprintf(" (%s)", formatDuration(time.Since(s.StartedAt)))
		case s.Elapsed > 0:
			elapsed = fmt.Sprintf(" (%s)", formatDuration(s.Elapsed))
		}

		counter := fmt.Sprintf("%d/%d", i+1, len(steps))
		line := style.Render(fmt.Sprintf("  %s %s %s%s", icon, counter, s.Name, elapsed))
		if s.Status == StepFailed && s.Error != "" {
			line += m.failedStyle.Render(fmt.Sprintf(" — %s", s.Error))
		}
		if s.Detail != "" && s.Status == StepRunning {
			line += m.detailStyle.Render(fmt.Sprintf(" · %s", s.Detail))
		}

		b.WriteString(truncate(line, width))
		if i < len(steps)-1 || len(m.branches) > 0 {
			b.WriteByte('\n')
		}
	}

	// Per-branch verification status (T36).
	if len(m.branches) > 0 {
		summary := m.branchSummaryLine()
		b.WriteString(m.detailStyle.Render("    " + summary))
		b.WriteByte('\n')
		for bi, br := range m.branches {
			icon := branchIcon(br.Status)
			var bStyle lipgloss.Style
			switch br.Status {
			case BranchPending:
				bStyle = m.pendingStyle
			case BranchRunning:
				bStyle = m.runningStyle
			case BranchPassed:
				bStyle = m.doneStyle
			case BranchFailed:
				bStyle = m.failedStyle
			case BranchSkipped:
				bStyle = m.pendingStyle
			case BranchPreExistingFailure:
				bStyle = m.runningStyle
			}
			// T38: Show cursor indicator for the selected branch.
			cursor := "  "
			if bi == m.branchCursor && m.expandedBranch < 0 {
				cursor = "▸ "
			}
			bLine := bStyle.Render(fmt.Sprintf("    %s%s %s", cursor, icon, br.Name))
			if br.Elapsed > 0 {
				bLine += m.detailStyle.Render(fmt.Sprintf(" (%s)", formatDuration(br.Elapsed)))
			}
			if br.Status == BranchFailed && br.Error != "" {
				bLine += m.failedStyle.Render(fmt.Sprintf(" — %s", br.Error))
			}
			if br.Status == BranchPreExistingFailure {
				bLine += m.detailStyle.Render(" (pre-existing)")
			}
			b.WriteString(truncate(bLine, width))
			if bi < len(m.branches)-1 {
				b.WriteByte('\n')
			}
		}
	}

	for j := len(steps) - startIdx; j < slotsForSteps; j++ {
		b.WriteByte('\n')
	}

	return b.String()
}

func (m *AutoSplitModel) renderSeparator(width int) string {
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

	m.mu.Lock()
	isCancelled := m.cancelled
	isPaused := m.paused
	needsAttention := m.needsAttention
	m.mu.Unlock()

	var left string
	switch {
	case m.done && isCancelled:
		left = " ⏹ Cancelled"
	case m.done && isPaused:
		left = " ⏸ Paused — resume with osm pr-split --resume"
	case m.done:
		left = " ✓ Complete — press q to dismiss"
	case isCancelled && m.forceCancel:
		left = " ⚡ Force cancelling… killing processes"
	case isCancelled:
		left = " ⏳ Cancelling… (q again to force kill)"
	case isPaused:
		left = " ⏸ Pausing… saving checkpoint"
	case currentStep != "":
		left = fmt.Sprintf(" ◉ %s", currentStep)
	default:
		left = " Auto-Split"
	}

	// T17: Attention indicator when Claude pane needs attention.
	if needsAttention && !m.done && !isCancelled {
		left += " 🔔"
	}

	var right string
	if total > 0 {
		right = fmt.Sprintf(" %d/%d ", doneCount+failCount, total)
		if failCount > 0 {
			right = fmt.Sprintf(" %d/%d (%d failed) ", doneCount+failCount, total, failCount)
		}
	}

	if m.scrollOffset > 0 {
		right += fmt.Sprintf("▲%d ", m.scrollOffset)
	}

	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}
	bar := left + strings.Repeat("─", padding) + right

	if isCancelled && !m.done {
		bg := lipgloss.Color("208")
		if m.forceCancel {
			bg = lipgloss.Color("196")
		}
		cancelStyle := lipgloss.NewStyle().
			Background(bg).
			Foreground(lipgloss.Color("0")).
			Bold(true)
		return cancelStyle.Width(width).Render(bar)
	}

	return m.separatorStyle.Width(width).Render(bar)
}

// renderBranchDetail renders the expanded branch verification output (T38).
func (m *AutoSplitModel) renderBranchDetail(height, width int) string {
	if m.expandedBranch < 0 || m.expandedBranch >= len(m.branches) {
		return renderPane(nil, height, width)
	}
	br := m.branches[m.expandedBranch]

	// Header line: "▸ branch-name (failed, exit 1, 3.2s)"
	header := m.failedStyle.Render(fmt.Sprintf("▸ %s", br.Name))
	if br.Elapsed > 0 {
		header += m.detailStyle.Render(fmt.Sprintf(" (%s)", formatDuration(br.Elapsed)))
	}
	if br.Error != "" {
		header += m.failedStyle.Render(fmt.Sprintf(" — %s", br.Error))
	}

	lines := br.Output
	viewH := height - 1 // minus 1 for the header line
	if viewH < 1 {
		viewH = 1
	}

	// Apply scroll offset (latest output at bottom like the output pane).
	if m.branchScrollOffset > 0 && len(lines) > viewH {
		endIdx := len(lines) - m.branchScrollOffset
		if endIdx < viewH {
			endIdx = viewH
		}
		if endIdx > len(lines) {
			endIdx = len(lines)
		}
		lines = lines[:endIdx]
	}

	outputContent := renderPane(lines, viewH, width)
	return truncate(header, width) + "\n" + outputContent
}

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
	case isCancelled && m.forceCancel:
		help = helpStyle.Render("⚡ force killing child processes…")
	case isCancelled:
		help = keyStyle.Render("q") + helpStyle.Render(" force kill")
	default:
		branchHint := ""
		if len(m.branches) > 0 {
			if m.expandedBranch >= 0 {
				branchHint = keyStyle.Render("esc") + helpStyle.Render("/") +
					keyStyle.Render("enter") + helpStyle.Render(" collapse  ") +
					keyStyle.Render("j/k") + helpStyle.Render(" scroll  ")
			} else {
				branchHint = keyStyle.Render("j/k") + helpStyle.Render(" branches  ") +
					keyStyle.Render("enter") + helpStyle.Render(" expand  ")
			}
		}
		help = keyStyle.Render("q") + helpStyle.Render(" cancel  ") +
			keyStyle.Render("ctrl+p") + helpStyle.Render(" pause  ") +
			keyStyle.Render("ctrl+]") + helpStyle.Render(" claude  ") +
			branchHint +
			keyStyle.Render("↑↓") + helpStyle.Render(" scroll  ") +
			keyStyle.Render("home/end") + helpStyle.Render(" jump")
	}

	padding := width - lipgloss.Width(help) - 1
	if padding < 0 {
		padding = 0
	}
	return " " + help + strings.Repeat(" ", padding)
}

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

func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width < 1 {
		return ""
	}
	return s[:width]
}

func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return autoSplitTickMsg(t)
	})
}
