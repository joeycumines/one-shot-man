package storage

import (
	"encoding/json"
	"time"
)

const CurrentSchemaVersion = "0.2.0"

// Session represents the complete, persisted state of a single osm TUI instance.
// This is the top-level object serialized to a file.
// Future enhancements may include history pruning settings (e.g., max entry count) to manage file size.
type Session struct {
	Version     string                    `json:"version"`     // Schema version, e.g., "1.0.0", for forward-compatibility and migration logic.
	ID          string                    `json:"id"`          // Unique identifier for this session.
	CreateTime  time.Time                 `json:"createTime"`  // Timestamp of session creation.
	UpdateTime  time.Time                 `json:"updateTime"`  // Timestamp of the last state modification.
	History     []HistoryEntry            `json:"history"`     // A chronological log of all commands.
	ScriptState map[string]map[string]any `json:"scriptState"` // Per-command state: outer key is command name, inner map is command's local state.
	SharedState map[string]any            `json:"sharedState"` // Global shared state accessible across all commands (indexed by shared symbol name).
}

// HistoryEntry is an immutable record of a command and the complete state after its execution.
type HistoryEntry struct {
	EntryID    string          `json:"entryId"` // Unique ID for the entry (e.g., nanosecond timestamp).
	ModeID     string          `json:"modeId"`  // The name of the mode where the command was run.
	Command    string          `json:"command"`
	ReadTime   time.Time       `json:"readTime"`
	FinalState json.RawMessage `json:"finalState"` // Complete serialized JSON of unified application state.
}
