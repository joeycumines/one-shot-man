package mux

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PlanEditorItem represents a split in the plan editor.
type PlanEditorItem struct {
	Name        string
	Files       []string
	BranchName  string
	Description string
}

// PlanEditor is a BubbleTea model for visually editing a split plan.
// It shows splits as a navigable list with file details, supporting
// move, delete, rename, and merge operations.
type PlanEditor struct {
	// Terminal dimensions.
	width  int
	height int

	// Plan data.
	items []PlanEditorItem

	// Navigation state.
	cursor   int  // selected split index
	expanded int  // expanded split index (-1 = none)
	fileCur  int  // file cursor within expanded split
	moveMode bool // in file-move mode (selecting destination split)

	// Edit mode.
	renaming     bool
	renameBuffer string
	renameIdx    int

	// Styles.
	selectedStyle  lipgloss.Style
	normalStyle    lipgloss.Style
	fileStyle      lipgloss.Style
	headerStyle    lipgloss.Style
	moveHintStyle  lipgloss.Style
	statusBarStyle lipgloss.Style

	// Callback — called when plan changes (move/delete/rename/merge).
	onChange func(items []PlanEditorItem)

	// Done — user pressed q or Escape at top level.
	done bool
}

// PlanEditorOption configures a PlanEditor.
type PlanEditorOption func(*PlanEditor)

// WithOnChange sets the callback invoked when the plan changes.
func WithOnChange(fn func(items []PlanEditorItem)) PlanEditorOption {
	return func(p *PlanEditor) {
		p.onChange = fn
	}
}

// NewPlanEditor creates a plan editor BubbleTea model.
func NewPlanEditor(items []PlanEditorItem, opts ...PlanEditorOption) *PlanEditor {
	// Deep copy items so mutations don't affect caller.
	copied := make([]PlanEditorItem, len(items))
	for i, item := range items {
		files := make([]string, len(item.Files))
		copy(files, item.Files)
		copied[i] = PlanEditorItem{
			Name:        item.Name,
			Files:       files,
			BranchName:  item.BranchName,
			Description: item.Description,
		}
	}
	p := &PlanEditor{
		width:    80,
		height:   24,
		items:    copied,
		expanded: -1,
		selectedStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true),
		normalStyle: lipgloss.NewStyle(),
		fileStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		headerStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")),
		moveHintStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true),
		statusBarStyle: lipgloss.NewStyle().
			Background(lipgloss.Color("240")).
			Foreground(lipgloss.Color("255")),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Items returns the current plan items.
func (p *PlanEditor) Items() []PlanEditorItem {
	result := make([]PlanEditorItem, len(p.items))
	for i, item := range p.items {
		files := make([]string, len(item.Files))
		copy(files, item.Files)
		result[i] = PlanEditorItem{
			Name:        item.Name,
			Files:       files,
			BranchName:  item.BranchName,
			Description: item.Description,
		}
	}
	return result
}

// Done reports if the editor has been dismissed.
func (p *PlanEditor) Done() bool {
	return p.done
}

// Run starts the BubbleTea program and blocks until the user exits.
// Returns the final items after all edits.
func (p *PlanEditor) Run() ([]PlanEditorItem, error) {
	prog := tea.NewProgram(p, tea.WithAltScreen())
	_, err := prog.Run()
	if err != nil {
		return nil, err
	}
	return p.Items(), nil
}

// Init implements tea.Model.
func (p *PlanEditor) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (p *PlanEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		return p, nil

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p *PlanEditor) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Rename mode — capture input.
	if p.renaming {
		return p.handleRenameKey(msg)
	}

	// Move mode — select destination split.
	if p.moveMode {
		return p.handleMoveKey(msg)
	}

	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if p.expanded >= 0 {
			p.fileCur--
			if p.fileCur < 0 {
				p.fileCur = 0
			}
		} else {
			p.cursor--
			if p.cursor < 0 {
				p.cursor = 0
			}
		}

	case tea.KeyDown, tea.KeyCtrlN:
		if p.expanded >= 0 {
			maxFile := len(p.items[p.expanded].Files) - 1
			if maxFile < 0 {
				maxFile = 0
			}
			p.fileCur++
			if p.fileCur > maxFile {
				p.fileCur = maxFile
			}
		} else {
			p.cursor++
			if p.cursor >= len(p.items) {
				p.cursor = len(p.items) - 1
			}
		}

	case tea.KeyEnter:
		if p.expanded == p.cursor {
			// Collapse.
			p.expanded = -1
			p.fileCur = 0
		} else if p.cursor >= 0 && p.cursor < len(p.items) {
			// Expand.
			p.expanded = p.cursor
			p.fileCur = 0
		}

	case tea.KeyEscape:
		if p.expanded >= 0 {
			p.expanded = -1
			p.fileCur = 0
		} else {
			p.done = true
			return p, tea.Quit
		}

	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'q':
				p.done = true
				return p, tea.Quit

			case 'd':
				// Delete split at cursor.
				if len(p.items) > 1 && p.cursor >= 0 && p.cursor < len(p.items) {
					p.items = append(p.items[:p.cursor], p.items[p.cursor+1:]...)
					if p.cursor >= len(p.items) {
						p.cursor = len(p.items) - 1
					}
					if p.expanded >= len(p.items) {
						p.expanded = -1
					}
					p.notifyChange()
				}

			case 'r':
				// Rename split at cursor.
				if p.cursor >= 0 && p.cursor < len(p.items) {
					p.renaming = true
					p.renameIdx = p.cursor
					p.renameBuffer = p.items[p.cursor].Name
				}

			case 'm':
				// Move file from expanded split.
				if p.expanded >= 0 && len(p.items[p.expanded].Files) > 0 {
					p.moveMode = true
				}

			case 'M':
				// Merge current split into next.
				if p.cursor >= 0 && p.cursor < len(p.items)-1 {
					dst := p.cursor + 1
					p.items[dst].Files = append(p.items[dst].Files, p.items[p.cursor].Files...)
					p.items = append(p.items[:p.cursor], p.items[p.cursor+1:]...)
					if p.cursor >= len(p.items) {
						p.cursor = len(p.items) - 1
					}
					p.expanded = -1
					p.notifyChange()
				}
			}
		}
	}
	return p, nil
}

func (p *PlanEditor) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		if p.renameIdx >= 0 && p.renameIdx < len(p.items) && p.renameBuffer != "" {
			p.items[p.renameIdx].Name = p.renameBuffer
			p.notifyChange()
		}
		p.renaming = false
		p.renameBuffer = ""

	case tea.KeyEscape:
		p.renaming = false
		p.renameBuffer = ""

	case tea.KeyBackspace:
		if len(p.renameBuffer) > 0 {
			p.renameBuffer = p.renameBuffer[:len(p.renameBuffer)-1]
		}

	case tea.KeyRunes:
		p.renameBuffer += string(msg.Runes)
	}
	return p, nil
}

func (p *PlanEditor) handleMoveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		p.moveMode = false

	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			idx := int(msg.Runes[0] - '1') // 1-based selection
			if idx >= 0 && idx < len(p.items) && idx != p.expanded {
				// Move file from expanded split to target.
				src := p.expanded
				if p.fileCur >= 0 && p.fileCur < len(p.items[src].Files) {
					file := p.items[src].Files[p.fileCur]
					p.items[src].Files = append(
						p.items[src].Files[:p.fileCur],
						p.items[src].Files[p.fileCur+1:]...,
					)
					p.items[idx].Files = append(p.items[idx].Files, file)

					// Adjust file cursor.
					if p.fileCur >= len(p.items[src].Files) && len(p.items[src].Files) > 0 {
						p.fileCur = len(p.items[src].Files) - 1
					}
					p.notifyChange()
				}
				p.moveMode = false
			}
		}
	}
	return p, nil
}

func (p *PlanEditor) notifyChange() {
	if p.onChange != nil {
		p.onChange(p.Items())
	}
}

// View implements tea.Model.
func (p *PlanEditor) View() string {
	if p.done {
		return ""
	}

	var b strings.Builder

	// Header.
	header := p.headerStyle.Render(fmt.Sprintf("╔═ Plan Editor (%d splits) ═╗", len(p.items)))
	b.WriteString(header)
	b.WriteByte('\n')

	// Splits list.
	for i, item := range p.items {
		prefix := "  "
		style := p.normalStyle
		if i == p.cursor {
			prefix = "▶ "
			style = p.selectedStyle
		}

		line := fmt.Sprintf("%s%d. %s (%d files)", prefix, i+1, item.Name, len(item.Files))
		b.WriteString(style.Render(line))
		b.WriteByte('\n')

		// Expanded files.
		if i == p.expanded {
			for j, file := range item.Files {
				fPrefix := "    "
				fStyle := p.fileStyle
				if j == p.fileCur {
					fPrefix = "  → "
					fStyle = p.selectedStyle
				}
				b.WriteString(fStyle.Render(fPrefix + file))
				b.WriteByte('\n')
			}
			if len(item.Files) == 0 {
				b.WriteString(p.fileStyle.Render("    (empty)"))
				b.WriteByte('\n')
			}
		}
	}

	// Status bar / hints.
	b.WriteByte('\n')
	if p.renaming {
		b.WriteString(p.moveHintStyle.Render(fmt.Sprintf("Rename: %s█", p.renameBuffer)))
	} else if p.moveMode {
		b.WriteString(p.moveHintStyle.Render("Move to split: press 1-9 | Esc cancel"))
	} else {
		hints := "↑↓ navigate | Enter expand | d delete | r rename | m move | M merge | q quit"
		b.WriteString(p.statusBarStyle.Width(p.width).Render(hints))
	}

	return b.String()
}
