package ui

import tea "github.com/charmbracelet/bubbletea"

// Compile-time interface checks (T097).
var (
	_ tea.Model = (*AutoSplitModel)(nil)
	_ tea.Model = (*SplitView)(nil)
	_ tea.Model = (*PlanEditor)(nil)
)
