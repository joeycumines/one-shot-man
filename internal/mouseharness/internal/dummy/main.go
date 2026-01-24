//go:build unix

// Package main provides a dummy TUI program for mouseharness integration tests.
// It displays clickable buttons that respond to mouse events.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	clicked  bool
	scrolled int // positive = up, negative = down
	lastX    int
	lastY    int
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			// Reset state
			m.clicked = false
			m.scrolled = 0
			m.lastX = 0
			m.lastY = 0
		}

	case tea.MouseMsg:
		m.lastX = msg.X
		m.lastY = msg.Y

		switch msg.Button {
		case tea.MouseButtonLeft:
			if msg.Action == tea.MouseActionRelease {
				m.clicked = true
			}
		case tea.MouseButtonWheelUp:
			m.scrolled++
		case tea.MouseButtonWheelDown:
			m.scrolled--
		}
	}

	return m, nil
}

func (m model) View() string {
	var status string
	if m.clicked {
		status = "[Clicked!]"
	} else {
		status = "[Click Me]"
	}

	scrollStatus := fmt.Sprintf("Scroll: %d", m.scrolled)
	posStatus := fmt.Sprintf("Last: (%d,%d)", m.lastX, m.lastY)

	return fmt.Sprintf(`Dummy TUI for mouseharness tests

%s

%s
%s

Press 'q' to quit, 'r' to reset
`, status, scrollStatus, posStatus)
}

func main() {
	// Enable mouse support
	p := tea.NewProgram(model{}, tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
