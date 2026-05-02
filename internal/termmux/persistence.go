package termmux

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// persistenceVersion is the schema version for the persisted state file.
// Bump this when the PersistedManagerState schema changes incompatibly.
const persistenceVersion = "1"

// PersistedSession captures the metadata of a single managed session for
// serialization. This includes everything needed to detect whether a session
// can be resumed (PID liveness) and what command to restart if not.
type PersistedSession struct {
	// SessionID is the monotonic identifier assigned by SessionManager.
	SessionID uint64 `json:"sessionId"`

	// Target is the session's metadata (name, kind, stable ID).
	Target SessionTarget `json:"target"`

	// State is the lifecycle state at the time of persistence.
	State SessionState `json:"state"`

	// PID is the child process ID, or 0 if not applicable.
	PID int `json:"pid,omitempty"`

	// Rows and Cols are the terminal dimensions at persistence time.
	Rows int `json:"rows"`
	Cols int `json:"cols"`

	// LastActive is the last time this session was the active input target.
	LastActive time.Time `json:"lastActive"`

	// Command, Args, Dir, and Env describe how to restart the session.
	// These are only populated for sessions whose underlying type
	// implements [SessionConfigProvider].
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Dir     string            `json:"dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// PersistedManagerState is the top-level structure written to disk by
// [SaveManagerState]. It captures all sessions and the manager's terminal
// dimensions at a point in time.
type PersistedManagerState struct {
	// Version is the persistence schema version.
	Version string `json:"version"`

	// ActiveID is the currently active session's ID.
	ActiveID uint64 `json:"activeId"`

	// Sessions lists all managed sessions at the time of persistence.
	Sessions []PersistedSession `json:"sessions"`

	// TermRows and TermCols are the manager's terminal dimensions.
	TermRows int `json:"termRows"`
	TermCols int `json:"termCols"`

	// SavedAt is the timestamp of this persistence snapshot.
	SavedAt time.Time `json:"savedAt"`
}

// SessionPIDProvider is an optional interface implemented by session types
// that can report their child process PID (e.g., [CaptureSession]).
type SessionPIDProvider interface {
	Pid() int
}

// SessionConfigProvider is an optional interface implemented by session
// types that can export their creation configuration for restart purposes
// (e.g., [CaptureSession]).
type SessionConfigProvider interface {
	ExportConfig() CaptureConfig
}

// ExportState captures a read-only snapshot of all managed sessions for
// persistence. This method sends a request to the worker goroutine to
// ensure thread-safe access to worker-owned state.
func (m *SessionManager) ExportState() (*PersistedManagerState, error) {
	resp := m.sendRequest(reqExportState, nil)
	if resp.err != nil {
		return nil, resp.err
	}
	return resp.value.(*PersistedManagerState), nil
}

// RestoreFromState rehydrates the SessionManager from a previously persisted
// state snapshot. For each session in the state, the factory function is
// called to construct an [InteractiveSession]. Successfully created sessions
// are registered with the manager; the active session is set to the one
// recorded in the persisted state (if it was restored).
//
// The factory function receives a [PersistedSession] containing the session's
// command, args, environment, and other metadata. It returns the constructed
// session or an error. A common factory implementation creates a
// [*CaptureSession] from [PersistedSession.Command] and related fields:
//
//	mgr.RestoreFromState(state, func(ps termmux.PersistedSession) (termmux.InteractiveSession, error) {
//	    cs := termmux.NewCaptureSession(termmux.CaptureConfig{
//	        Command: ps.Command,
//	        Args:    ps.Args,
//	        Dir:     ps.Dir,
//	        Env:     ps.Env,
//	        Rows:    ps.Rows,
//	        Cols:    ps.Cols,
//	    })
//	    return cs, nil
//	})
//
// RestoreFromState must be called while the manager is running (after [Run]).
// It must not be called concurrently with other mutating operations. If the
// manager already has registered sessions, the restored sessions are added
// alongside them (IDs are assigned sequentially and will NOT match the
// persisted IDs).
//
// Sessions whose factory call fails are recorded in [RestoreResult.Failed].
func (m *SessionManager) RestoreFromState(
	state *PersistedManagerState,
	factory func(PersistedSession) (InteractiveSession, error),
) (*RestoreResult, error) {
	if state == nil {
		return nil, errors.New("termmux: nil state")
	}
	if factory == nil {
		return nil, errors.New("termmux: nil factory")
	}
	if state.Version != persistenceVersion {
		return nil, fmt.Errorf("termmux: unsupported persistence version %q (expected %q)", state.Version, persistenceVersion)
	}
	resp := m.sendRequest(reqRestoreState, &restoreStatePayload{
		state:   state,
		factory: factory,
	})
	if resp.err != nil {
		return nil, resp.err
	}
	return resp.value.(*RestoreResult), nil
}

// handleExportState builds a [PersistedManagerState] snapshot from the
// worker-owned session map. Runs exclusively on the worker goroutine.
func (m *SessionManager) handleExportState() response {
	state := &PersistedManagerState{
		Version:  persistenceVersion,
		ActiveID: uint64(m.activeID),
		TermRows: m.termRows,
		TermCols: m.termCols,
		SavedAt:  time.Now(),
		Sessions: make([]PersistedSession, 0, len(m.sessions)),
	}
	for id, ms := range m.sessions {
		ps := PersistedSession{
			SessionID:  uint64(id),
			Target:     ms.target,
			State:      ms.state,
			Rows:       m.termRows,
			Cols:       m.termCols,
			LastActive: ms.lastActive,
		}
		// Extract PID if the underlying session supports it.
		if pp, ok := ms.session.(SessionPIDProvider); ok {
			ps.PID = pp.Pid()
		}
		// Extract restart config if the underlying session supports it.
		if cp, ok := ms.session.(SessionConfigProvider); ok {
			cfg := cp.ExportConfig()
			ps.Command = cfg.Command
			ps.Args = cfg.Args
			ps.Dir = cfg.Dir
			ps.Env = cfg.Env
		}
		state.Sessions = append(state.Sessions, ps)
	}
	return response{value: state}
}

// handleRestoreState processes a reqRestoreState request on the worker
// goroutine. It iterates over persisted sessions, calls the factory for each,
// and registers the resulting InteractiveSession.
func (m *SessionManager) handleRestoreState(p *restoreStatePayload) response {
	result := &RestoreResult{
		Restored: make([]SessionID, 0, len(p.state.Sessions)),
		Failed:   nil,
	}

	// Track the mapping from persisted session ID to new SessionID
	// so we can set the active session correctly.
	idMap := make(map[uint64]SessionID) // persisted SessionID → new SessionID

	for _, ps := range p.state.Sessions {
		session, err := p.factory(ps)
		if err != nil {
			result.Failed = append(result.Failed, RestoreFailure{
				SessionID: SessionID(ps.SessionID),
				Error:     err,
			})
			continue
		}

		target := ps.Target
		newID := m.nextID
		m.nextID++
		m.snapshotGen++

		v := vt.NewVTerm(m.termRows, m.termCols)
		v.BellFn = func() {
			m.eventBus.emit(EventBell, newID)
		}

		ms := &managedSession{
			session:    session,
			vterm:      v,
			state:      SessionCreated,
			target:     target,
			lastActive: ps.LastActive,
		}

		snap := &ScreenSnapshot{
			Gen:       m.snapshotGen,
			Rows:      m.termRows,
			Cols:      m.termCols,
			Timestamp: time.Now(),
		}
		ms.snapshot.Store(snap)
		m.sessions[newID] = ms

		// Spawn reader goroutine for the restored session.
		m.startReaderGoroutine(newID, session)

		idMap[ps.SessionID] = newID
		result.Restored = append(result.Restored, newID)

		slog.Info("restored session", "persistedId", ps.SessionID, "newId", newID, "target", target)
	}

	// Set the active session if the persisted active ID was restored
	// and no session is currently active.
	if newActiveID, ok := idMap[p.state.ActiveID]; ok && m.activeID == 0 {
		m.activeID = newActiveID
	}

	// Emit registration events for restored sessions.
	for _, newID := range result.Restored {
		m.eventBus.emit(EventSessionRegistered, newID)
	}

	return response{value: result}
}

// SaveManagerState atomically writes the persisted state to path. The parent
// directory is created if it does not exist. A temporary file + rename
// strategy prevents partial writes.
func SaveManagerState(path string, state *PersistedManagerState) error {
	if state == nil {
		return errors.New("persistence: nil state")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("persistence: create directory: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("persistence: marshal: %w", err)
	}
	// Atomic write: temp file + rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("persistence: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("persistence: rename: %w", err)
	}
	return nil
}

// LoadManagerState reads and decodes a persisted state file. Returns
// (nil, nil) if the file does not exist.
func LoadManagerState(path string) (*PersistedManagerState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("persistence: read: %w", err)
	}
	var state PersistedManagerState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("persistence: unmarshal: %w", err)
	}
	if state.Version != persistenceVersion {
		return nil, fmt.Errorf("persistence: unsupported version %q (expected %q)", state.Version, persistenceVersion)
	}
	return &state, nil
}

// RemoveManagerState deletes the persisted state file. Returns nil if the
// file does not exist.
func RemoveManagerState(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("persistence: remove: %w", err)
	}
	return nil
}

// ProcessAlive reports whether a process with the given PID exists and is
// reachable. The implementation is platform-specific: on Unix, it sends
// signal 0 (null signal); on Windows, it calls OpenProcess with
// PROCESS_QUERY_LIMITED_INFORMATION.
//
// Returns false for pid <= 0.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return processAlive(pid)
}
