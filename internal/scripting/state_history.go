package scripting

import (
	"time"
)

// HistoryEntry stores a command and the resulting state snapshot.
type HistoryEntry struct {
	// Command is the user-typed command that led to this state.
	Command string `json:"command"`
	// ReadTime is when the command completed and the state was captured.
	ReadTime time.Time `json:"readTime"`
	// FinalState is the full, serialized JSON string of the mode's state.
	FinalState string `json:"finalState"`
}
