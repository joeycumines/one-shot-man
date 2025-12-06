package storage

import (
	"time"
)

// Session represents the complete, persisted state of a single osm TUI instance.
// This is the top-level object serialized to a file.
// Future enhancements may include history pruning settings (e.g., max entry count) to manage file size.
type Session struct {
	Version     string                            `json:"version"`      // Schema version, e.g., "1.0.0", for forward-compatibility and migration logic.
	ID          string                            `json:"id"`           // Unique identifier for this session.
	CreatedAt   time.Time                         `json:"created_at"`   // Timestamp of session creation.
	UpdatedAt   time.Time                         `json:"updated_at"`   // Timestamp of the last state modification.
	History     []HistoryEntry                    `json:"history"`      // A chronological log of all commands.
	ScriptState map[string]map[string]interface{} `json:"script_state"` // Per-command state: outer key is command name, inner map is command's local state.
	SharedState map[string]interface{}            `json:"shared_state"` // Global shared state accessible across all commands (indexed by shared symbol name).
}

// HistoryEntry is an immutable record of a command and the complete state after its execution.
type HistoryEntry struct {
	EntryID    string    `json:"entry_id"` // Unique ID for the entry (e.g., nanosecond timestamp).
	ModeID     string    `json:"mode_id"`  // The name of the mode where the command was run.
	Command    string    `json:"command"`
	Timestamp  time.Time `json:"timestamp"`
	FinalState string    `json:"finalState"` // Complete serialized JSON of unified application state.
}
