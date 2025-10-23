package storage

import (
	"time"
)

// Session represents the complete, persisted state of a single osm TUI instance.
// This is the top-level object serialized to a file.
// Future enhancements may include history pruning settings (e.g., max entry count) to manage file size.
type Session struct {
	Version     string               `json:"version"`      // Schema version, e.g., "1.0.0", for forward-compatibility and migration logic.
	SessionID   string               `json:"session_id"`   // Unique identifier for this session.
	CreatedAt   time.Time            `json:"created_at"`   // Timestamp of session creation.
	UpdatedAt   time.Time            `json:"updated_at"`   // Timestamp of the last state modification.
	Contracts   []ContractDefinition `json:"contracts"`    // All registered state contracts for validation.
	History     []HistoryEntry       `json:"history"`      // A chronological log of all commands.
	LatestState map[string]ModeState `json:"latest_state"` // Fast-lookup map of the most recent state for each mode. Reserved key "__shared__" for global shared state.
}

// ContractDefinition stores serializable contract metadata.
type ContractDefinition struct {
	ModeName string         `json:"mode_name"`
	IsShared bool           `json:"is_shared"`
	Keys     map[string]any `json:"keys"`    // Map of Symbol description to default value.
	Schemas  map[string]any `json:"schemas"` // Map of Symbol description to schema information (e.g., type name or JSON schema).
}

// ModeState encapsulates a mode's serialized state and its contract hash.
type ModeState struct {
	ModeName     string `json:"mode_name"`
	ContractHash string `json:"contract_hash"` // SHA256 hash of the contract keys for compatibility validation.
	StateJSON    string `json:"state_json"`    // The complete, serialized state for this mode.
}

// HistoryEntry is an immutable record of a command and the complete state after its execution.
type HistoryEntry struct {
	EntryID        string            `json:"entry_id"` // Unique ID for the entry (e.g., nanosecond timestamp).
	ModeID         string            `json:"mode_id"`  // The name of the mode where the command was run.
	Command        string            `json:"command"`
	Timestamp      time.Time         `json:"timestamp"`
	FinalState     string            `json:"finalState"`      // Complete serialized JSON of application state in namespaced format, e.g., {"__shared__":{...}, "flow":{...}}.
	ContractHashes map[string]string `json:"contract_hashes"` // Maps each active contract's name to its deterministic hash.
}
