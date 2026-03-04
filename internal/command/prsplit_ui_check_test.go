package command

import tea "github.com/charmbracelet/bubbletea"

// Compile-time interface checks.
var (
	_ tea.Model = (*AutoSplitModel)(nil)
	_ tea.Model = (*PlanEditor)(nil)
)
