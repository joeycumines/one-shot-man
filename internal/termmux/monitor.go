package termmux

import "time"

// MonitorConfig controls per-pane monitoring for bell, activity, and silence.
type MonitorConfig struct {
	// Bell controls whether BEL events trigger a MonitorBell event.
	Bell bool

	// Activity controls whether output from a background (inactive) pane
	// after being idle triggers a MonitorActivity event.
	Activity bool

	// ActivityThreshold is the idle duration after which output from a
	// background pane is considered "activity". Zero means immediate.
	ActivityThreshold time.Duration

	// Silence controls whether a pane that produces no output for the
	// configured duration triggers a MonitorSilence event.
	Silence bool

	// SilenceThreshold is the duration with no output before a silence
	// event fires. Zero disables silence monitoring regardless of the
	// Silence flag.
	SilenceThreshold time.Duration
}

// VisualBellState tracks the state of a visual bell flash for a pane.
type VisualBellState struct {
	// Active is true when a visual bell flash is currently displayed.
	Active bool

	// StartedAt records when the current visual bell flash began.
	StartedAt time.Time
}

// MonitorState tracks per-pane monitoring state. It is owned by the
// SessionManager worker goroutine — no synchronization needed.
type MonitorState struct {
	Config MonitorConfig

	// VisualBell tracks the visual bell flash state for this pane.
	VisualBell VisualBellState

	// LastOutputAt records the last time output was processed for this pane.
	// Used for activity and silence detection.
	LastOutputAt time.Time

	// SilenceFired is true after a silence event has been emitted and
	// not yet reset by new output. This prevents repeated firing.
	SilenceFired bool

	// ActivityFired is true after an activity event has been emitted and
	// not yet reset by the pane becoming active. This prevents repeated
	// firing for the same burst of output.
	ActivityFired bool
}

// NewMonitorState returns a MonitorState with the given config and
// LastOutputAt initialized to now.
func NewMonitorState(cfg MonitorConfig) *MonitorState {
	return &MonitorState{
		Config:       cfg,
		LastOutputAt: time.Now(),
	}
}
