package termmux

// ConfiguredTermSize returns the dimensions supplied at construction without
// sending a request to the manager worker. It is safe before Run starts.
func (m *SessionManager) ConfiguredTermSize() (rows, cols int) {
	return int(m.configuredTermRows.Load()), int(m.configuredTermCols.Load())
}
