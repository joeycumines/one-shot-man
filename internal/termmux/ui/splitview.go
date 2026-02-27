package ui

import (
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// SplitPane identifies which pane of the split view is active.
type SplitPane int

const (
	// PaneOsm is the top pane showing osm output.
	PaneOsm SplitPane = iota
	// PaneClaude is the bottom pane showing Claude output.
	PaneClaude
)

// SplitView is a BubbleTea model that renders osm and Claude output in
// vertically stacked panes. The top pane shows osm's TUI, the bottom
// pane shows Claude's output, separated by a status bar.
//
// Input is routed to the active pane (toggled with Ctrl+]).
// Terminal resizes are propagated to both panes.
type SplitView struct {
	// Terminal dimensions.
	width  int
	height int

	// Active pane receives keyboard input.
	activePane SplitPane

	// Pane content — ring buffers of recent output lines.
	osmLines    []string
	claudeLines []string

	// Maximum number of lines to retain per pane.
	maxLines int

	// Claude status for the separator bar.
	claudeStatus string

	// Toggle key — matches parent mux.
	toggleKey byte

	// Split ratio — fraction allocated to top pane (0.0–1.0).
	splitRatio float64

	// Styles.
	separatorStyle lipgloss.Style
	activeStyle    lipgloss.Style
	inactiveStyle  lipgloss.Style

	// Mutex for concurrent output writes.
	mu sync.Mutex

	// Callback to forward input to Claude child PTY.
	claudeWriter func([]byte) error

	// quitting signals the program should exit.
	quitting bool
}

// SplitViewOption configures a SplitView.
type SplitViewOption func(*SplitView)

// WithSplitRatio sets the top/bottom split ratio (0.0–1.0).
func WithSplitRatio(ratio float64) SplitViewOption {
	return func(s *SplitView) {
		if ratio < 0.1 {
			ratio = 0.1
		}
		if ratio > 0.9 {
			ratio = 0.9
		}
		s.splitRatio = ratio
	}
}

// WithMaxLines sets the maximum number of output lines retained per pane.
func WithMaxLines(n int) SplitViewOption {
	return func(s *SplitView) {
		if n < 10 {
			n = 10
		}
		s.maxLines = n
	}
}

// WithToggleKey sets the toggle key for switching active pane.
func WithToggleKey(key byte) SplitViewOption {
	return func(s *SplitView) {
		s.toggleKey = key
	}
}

// WithClaudeWriter sets the callback for forwarding input to Claude.
func WithClaudeWriter(fn func([]byte) error) SplitViewOption {
	return func(s *SplitView) {
		s.claudeWriter = fn
	}
}

// NewSplitView creates a SplitView BubbleTea model with the given options.
func NewSplitView(opts ...SplitViewOption) *SplitView {
	s := &SplitView{
		width:        80,
		height:       24,
		activePane:   PaneOsm,
		maxLines:     500,
		toggleKey:    termmux.DefaultToggleKey,
		splitRatio:   0.5,
		claudeStatus: "idle",
		separatorStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("240")).
			Foreground(lipgloss.Color("255")).
			Bold(true),
		activeStyle: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("86")),
		inactiveStyle: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// AppendOsmOutput adds lines to the osm pane. Safe for concurrent use.
func (s *SplitView) AppendOsmOutput(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := strings.Split(text, "\n")
	s.osmLines = appendCapped(s.osmLines, lines, s.maxLines)
}

// AppendClaudeOutput adds lines to the Claude pane. Safe for concurrent use.
func (s *SplitView) AppendClaudeOutput(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := strings.Split(text, "\n")
	s.claudeLines = appendCapped(s.claudeLines, lines, s.maxLines)
}

// SetClaudeStatus updates the Claude status in the separator bar.
func (s *SplitView) SetClaudeStatus(status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claudeStatus = status
}

// ActivePane returns which pane currently receives input.
func (s *SplitView) ActivePane() SplitPane {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activePane
}

// SetSplitRatio updates the split ratio without recreating the view.
// Safe for concurrent use.
func (s *SplitView) SetSplitRatio(ratio float64) {
	if ratio < 0.1 {
		ratio = 0.1
	}
	if ratio > 0.9 {
		ratio = 0.9
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.splitRatio = ratio
}

// Run starts the BubbleTea program and blocks until exit.
func (s *SplitView) Run() error {
	p := tea.NewProgram(s, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// splitViewOutputMsg is sent when output is appended from goroutines.
type splitViewOutputMsg struct{}

// Init implements tea.Model.
func (s *SplitView) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (s *SplitView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		return s, nil

	case tea.KeyMsg:
		// Ctrl+] toggles active pane.
		if msg.Type == tea.KeyCtrlCloseBracket {
			s.mu.Lock()
			if s.activePane == PaneOsm {
				s.activePane = PaneClaude
			} else {
				s.activePane = PaneOsm
			}
			s.mu.Unlock()
			return s, nil
		}

		// Ctrl+C quits.
		if msg.Type == tea.KeyCtrlC {
			s.quitting = true
			return s, tea.Quit
		}

		// Forward input to active pane.
		if s.activePane == PaneClaude && s.claudeWriter != nil {
			data := keyMsgToBytes(msg)
			if len(data) > 0 {
				_ = s.claudeWriter(data)
			}
		}
		return s, nil

	case splitViewOutputMsg:
		return s, nil
	}
	return s, nil
}

// View implements tea.Model.
func (s *SplitView) View() string {
	if s.quitting {
		return ""
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	availableHeight := s.height - 1
	if availableHeight < 2 {
		availableHeight = 2
	}
	topHeight := int(float64(availableHeight) * s.splitRatio)
	if topHeight < 1 {
		topHeight = 1
	}
	bottomHeight := availableHeight - topHeight
	if bottomHeight < 1 {
		bottomHeight = 1
		topHeight = availableHeight - 1
	}

	contentWidth := s.width
	if contentWidth < 2 {
		contentWidth = 2
	}

	topContent := renderPane(s.osmLines, topHeight, contentWidth)
	bottomContent := renderPane(s.claudeLines, bottomHeight, contentWidth)

	separator := s.renderSeparator(contentWidth)

	var topStyle, bottomStyle lipgloss.Style
	if s.activePane == PaneOsm {
		topStyle = s.activeStyle
		bottomStyle = s.inactiveStyle
	} else {
		topStyle = s.inactiveStyle
		bottomStyle = s.activeStyle
	}

	top := topStyle.Width(contentWidth).Height(topHeight).Render(topContent)
	bottom := bottomStyle.Width(contentWidth).Height(bottomHeight).Render(bottomContent)

	return lipgloss.JoinVertical(lipgloss.Left, top, separator, bottom)
}

func (s *SplitView) renderSeparator(width int) string {
	active := "osm"
	if s.activePane == PaneClaude {
		active = "Claude"
	}
	left := fmt.Sprintf(" [%s] %s ", active, s.claudeStatus)
	right := " Ctrl+] toggle "
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 0 {
		padding = 0
	}
	bar := left + strings.Repeat("─", padding) + right
	return s.separatorStyle.Width(width).Render(bar)
}

// renderPane renders the last N lines of content to fill the pane height.
func renderPane(lines []string, height, width int) string {
	if len(lines) == 0 {
		return strings.Repeat("\n", height-1)
	}

	start := len(lines) - height
	if start < 0 {
		start = 0
	}
	visible := lines[start:]

	var b strings.Builder
	for i, line := range visible {
		if len(line) > width {
			line = line[:width]
		}
		b.WriteString(line)
		if i < len(visible)-1 {
			b.WriteByte('\n')
		}
	}

	for i := len(visible); i < height; i++ {
		b.WriteByte('\n')
	}
	return b.String()
}

// appendCapped appends lines and trims to max capacity.
func appendCapped(existing, newLines []string, max int) []string {
	existing = append(existing, newLines...)
	if len(existing) > max {
		existing = existing[len(existing)-max:]
	}
	return existing
}

// keyMsgToBytes converts a BubbleTea key message to raw bytes for PTY forwarding.
func keyMsgToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyEscape:
		return []byte{0x1b}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case tea.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case tea.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case tea.KeyLeft:
		return []byte{0x1b, '[', 'D'}
	case tea.KeyCtrlA:
		return []byte{0x01}
	case tea.KeyCtrlB:
		return []byte{0x02}
	case tea.KeyCtrlD:
		return []byte{0x04}
	case tea.KeyCtrlE:
		return []byte{0x05}
	case tea.KeyCtrlF:
		return []byte{0x06}
	case tea.KeyCtrlK:
		return []byte{0x0b}
	case tea.KeyCtrlL:
		return []byte{0x0c}
	case tea.KeyCtrlN:
		return []byte{0x0e}
	case tea.KeyCtrlP:
		return []byte{0x10}
	case tea.KeyCtrlU:
		return []byte{0x15}
	case tea.KeyCtrlW:
		return []byte{0x17}
	default:
		return nil
	}
}
