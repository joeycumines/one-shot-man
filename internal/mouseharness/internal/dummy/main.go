//go:build unix

// Package main provides a dummy TUI program for mouseharness integration tests.
// It displays clickable buttons that respond to mouse events.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
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
	case tea.KeyPressMsg:
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

	case tea.MouseClickMsg:
		m.lastX = msg.X
		m.lastY = msg.Y

	case tea.MouseReleaseMsg:
		m.lastX = msg.X
		m.lastY = msg.Y
		if msg.Button == tea.MouseLeft {
			m.clicked = true
		}

	case tea.MouseMotionMsg:
		m.lastX = msg.X
		m.lastY = msg.Y

	case tea.MouseWheelMsg:
		m.lastX = msg.X
		m.lastY = msg.Y
		switch msg.Button {
		case tea.MouseWheelUp:
			m.scrolled++
		case tea.MouseWheelDown:
			m.scrolled--
		}
	}

	return m, nil
}

func (m model) View() tea.View {
	var status string
	if m.clicked {
		status = "[Clicked!]"
	} else {
		status = "[Click Me]"
	}

	scrollStatus := fmt.Sprintf("Scroll: %d", m.scrolled)
	posStatus := fmt.Sprintf("Last: (%d,%d)", m.lastX, m.lastY)

	v := tea.NewView(fmt.Sprintf(`Dummy TUI for mouseharness tests

%s

%s
%s

Press 'q' to quit, 'r' to reset
`, status, scrollStatus, posStatus))
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

func main() {
	// Enable mouse support
	p := tea.NewProgram(model{})
	if _, err := p.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
