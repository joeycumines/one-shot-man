package termmux

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadManagerState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	want := &PersistedManagerState{
		Version:  persistenceVersion,
		ActiveID: 2,
		TermRows: 40,
		TermCols: 120,
		SavedAt:  time.Now().Truncate(time.Millisecond), // JSON loses nanoseconds
		Sessions: []PersistedSession{
			{
				SessionID:  1,
				Target:     SessionTarget{Name: "claude", Kind: SessionKindPTY},
				State:      SessionRunning,
				PID:        12345,
				Rows:       40,
				Cols:       120,
				LastActive: time.Now().Add(-5 * time.Minute).Truncate(time.Millisecond),
				Command:    "claude",
				Args:       []string{"--chat"},
				Dir:        "/home/user/project",
			},
			{
				SessionID:  2,
				Target:     SessionTarget{Name: "verify", Kind: SessionKindCapture},
				State:      SessionExited,
				PID:        0,
				Rows:       40,
				Cols:       120,
				LastActive: time.Now().Truncate(time.Millisecond),
				Command:    "bash",
				Args:       []string{"-i"},
				Dir:        "/tmp/worktree",
				Env:        map[string]string{"BRANCH": "fix-bug"},
			},
		},
	}

	if err := SaveManagerState(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := LoadManagerState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Compare fields.
	if got.Version != want.Version {
		t.Errorf("version: got %q, want %q", got.Version, want.Version)
	}
	if got.ActiveID != want.ActiveID {
		t.Errorf("activeID: got %d, want %d", got.ActiveID, want.ActiveID)
	}
	if got.TermRows != want.TermRows || got.TermCols != want.TermCols {
		t.Errorf("dims: got %dx%d, want %dx%d", got.TermRows, got.TermCols, want.TermRows, want.TermCols)
	}
	if !got.SavedAt.Equal(want.SavedAt) {
		t.Errorf("savedAt: got %v, want %v", got.SavedAt, want.SavedAt)
	}
	if len(got.Sessions) != len(want.Sessions) {
		t.Fatalf("sessions: got %d, want %d", len(got.Sessions), len(want.Sessions))
	}
	for i, ws := range want.Sessions {
		gs := got.Sessions[i]
		if gs.SessionID != ws.SessionID {
			t.Errorf("session[%d].id: got %d, want %d", i, gs.SessionID, ws.SessionID)
		}
		if gs.Target.Name != ws.Target.Name || gs.Target.Kind != ws.Target.Kind {
			t.Errorf("session[%d].target: got %v, want %v", i, gs.Target, ws.Target)
		}
		if gs.State != ws.State {
			t.Errorf("session[%d].state: got %v, want %v", i, gs.State, ws.State)
		}
		if gs.PID != ws.PID {
			t.Errorf("session[%d].pid: got %d, want %d", i, gs.PID, ws.PID)
		}
		if !gs.LastActive.Equal(ws.LastActive) {
			t.Errorf("session[%d].lastActive: got %v, want %v", i, gs.LastActive, ws.LastActive)
		}
		if gs.Command != ws.Command {
			t.Errorf("session[%d].command: got %q, want %q", i, gs.Command, ws.Command)
		}
		if len(gs.Args) != len(ws.Args) {
			t.Errorf("session[%d].args: got %v, want %v", i, gs.Args, ws.Args)
		}
		if gs.Dir != ws.Dir {
			t.Errorf("session[%d].dir: got %q, want %q", i, gs.Dir, ws.Dir)
		}
		if len(gs.Env) != len(ws.Env) {
			t.Errorf("session[%d].env: got %v, want %v", i, gs.Env, ws.Env)
		}
	}
}

func TestLoadManagerState_NotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	got, err := LoadManagerState(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil state for missing file, got: %+v", got)
	}
}

func TestLoadManagerState_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManagerState(path)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestLoadManagerState_WrongVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.json")
	state := PersistedManagerState{
		Version:  "999",
		Sessions: []PersistedSession{},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManagerState(path)
	if err == nil {
		t.Fatal("expected error for wrong version")
	}
}

func TestSaveManagerState_Nil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nil.json")
	err := SaveManagerState(path, nil)
	if err == nil {
		t.Fatal("expected error for nil state")
	}
}

func TestSaveManagerState_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	state := &PersistedManagerState{
		Version:  persistenceVersion,
		Sessions: []PersistedSession{},
		SavedAt:  time.Now(),
	}
	if err := SaveManagerState(path, state); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify no .tmp file remains.
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp file should not remain after save, exists: %v", err)
	}

	// Verify the written file is valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var parsed PersistedManagerState
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal written file: %v", err)
	}
	if parsed.Version != persistenceVersion {
		t.Errorf("version: got %q, want %q", parsed.Version, persistenceVersion)
	}
}

func TestRemoveManagerState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Remove non-existent file should not error.
	if err := RemoveManagerState(path); err != nil {
		t.Fatalf("remove nonexistent: %v", err)
	}

	// Create and remove.
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveManagerState(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be removed")
	}
}

func TestProcessAlive_Self(t *testing.T) {
	// Our own PID should be alive.
	if !ProcessAlive(os.Getpid()) {
		t.Error("expected own process to be alive")
	}
}

func TestProcessAlive_Invalid(t *testing.T) {
	if ProcessAlive(0) {
		t.Error("expected pid 0 to be not alive")
	}
	if ProcessAlive(-1) {
		t.Error("expected pid -1 to be not alive")
	}
	// PID that's very unlikely to exist.
	if ProcessAlive(4_000_000) {
		t.Log("warning: PID 4000000 exists on this system (unexpected but not fatal)")
	}
}

func TestCaptureSession_ExportConfig(t *testing.T) {
	cfg := CaptureConfig{
		Name:    "test",
		Kind:    SessionKindCapture,
		Command: "echo",
		Args:    []string{"hello", "world"},
		Dir:     "/tmp",
		Env:     map[string]string{"FOO": "bar"},
		Rows:    30,
		Cols:    100,
	}
	cs := NewCaptureSession(cfg)

	exported := cs.ExportConfig()
	if exported.Command != cfg.Command {
		t.Errorf("command: got %q, want %q", exported.Command, cfg.Command)
	}
	if len(exported.Args) != len(cfg.Args) {
		t.Fatalf("args: got %v, want %v", exported.Args, cfg.Args)
	}
	for i := range cfg.Args {
		if exported.Args[i] != cfg.Args[i] {
			t.Errorf("args[%d]: got %q, want %q", i, exported.Args[i], cfg.Args[i])
		}
	}
	if exported.Dir != cfg.Dir {
		t.Errorf("dir: got %q, want %q", exported.Dir, cfg.Dir)
	}
	if exported.Env["FOO"] != "bar" {
		t.Errorf("env: got %v, want FOO=bar", exported.Env)
	}

	// Verify deep copy — mutating exported should not affect original.
	exported.Args[0] = "MUTATED"
	exported.Env["FOO"] = "MUTATED"
	second := cs.ExportConfig()
	if second.Args[0] != "hello" {
		t.Error("ExportConfig should deep-copy args")
	}
	if second.Env["FOO"] != "bar" {
		t.Error("ExportConfig should deep-copy env")
	}
}

func TestExportState_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, cleanup := startManager(t)
	defer cleanup()

	// Register a session with a controllableSession (doesn't implement PID/Config).
	sess := newControllableSession()
	id, err := mgr.Register(sess, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate: %v", err)
	}

	state, err := mgr.ExportState()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if state.Version != persistenceVersion {
		t.Errorf("version: got %q, want %q", state.Version, persistenceVersion)
	}
	if state.ActiveID != uint64(id) {
		t.Errorf("activeID: got %d, want %d", state.ActiveID, uint64(id))
	}
	if len(state.Sessions) != 1 {
		t.Fatalf("sessions: got %d, want 1", len(state.Sessions))
	}

	ps := state.Sessions[0]
	if ps.SessionID != uint64(id) {
		t.Errorf("session.id: got %d, want %d", ps.SessionID, uint64(id))
	}
	if ps.Target.Name != "test" {
		t.Errorf("session.target.name: got %q, want %q", ps.Target.Name, "test")
	}
	// controllableSession doesn't implement PID or Config providers.
	if ps.PID != 0 {
		t.Errorf("session.pid: got %d, want 0 (mock has no PID)", ps.PID)
	}
	if ps.Command != "" {
		t.Errorf("session.command: got %q, want empty (mock has no config)", ps.Command)
	}
}

func TestRestoreFromState_NilState(t *testing.T) {
	mgr, cleanup := startManager(t)
	defer cleanup()

	_, err := mgr.RestoreFromState(nil, func(ps PersistedSession) (InteractiveSession, error) {
		return newControllableSession(), nil
	})
	if err == nil {
		t.Fatal("expected error for nil state")
	}
}

func TestRestoreFromState_NilFactory(t *testing.T) {
	mgr, cleanup := startManager(t)
	defer cleanup()

	state := &PersistedManagerState{Version: persistenceVersion}
	_, err := mgr.RestoreFromState(state, nil)
	if err == nil {
		t.Fatal("expected error for nil factory")
	}
}

func TestRestoreFromState_WrongVersion(t *testing.T) {
	mgr, cleanup := startManager(t)
	defer cleanup()

	state := &PersistedManagerState{Version: "999"}
	_, err := mgr.RestoreFromState(state, func(ps PersistedSession) (InteractiveSession, error) {
		return newControllableSession(), nil
	})
	if err == nil {
		t.Fatal("expected error for wrong version")
	}
}

func TestRestoreFromState_Empty(t *testing.T) {
	mgr, cleanup := startManager(t)
	defer cleanup()

	state := &PersistedManagerState{
		Version:  persistenceVersion,
		ActiveID: 0,
		Sessions: nil,
	}

	result, err := mgr.RestoreFromState(state, func(ps PersistedSession) (InteractiveSession, error) {
		return newControllableSession(), nil
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(result.Restored) != 0 {
		t.Errorf("restored: got %d, want 0", len(result.Restored))
	}
	if len(result.Failed) != 0 {
		t.Errorf("failed: got %d, want 0", len(result.Failed))
	}
}

func TestRestoreFromState_SingleSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, cleanup := startManager(t)
	defer cleanup()

	state := &PersistedManagerState{
		Version:  persistenceVersion,
		ActiveID: 1,
		TermRows: 24,
		TermCols: 80,
		Sessions: []PersistedSession{
			{
				SessionID: 1,
				Target:    SessionTarget{Name: "shell", Kind: SessionKindPTY},
				State:     SessionCreated,
				Command:   "bash",
				Args:      []string{"-i"},
				Dir:       "/tmp",
				Rows:      24,
				Cols:      80,
			},
		},
	}

	result, err := mgr.RestoreFromState(state, func(ps PersistedSession) (InteractiveSession, error) {
		return newControllableSession(), nil
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	if len(result.Restored) != 1 {
		t.Fatalf("restored: got %d, want 1", len(result.Restored))
	}
	if len(result.Failed) != 0 {
		t.Fatalf("failed: got %d, want 0", len(result.Failed))
	}

	// The restored session should be registered and visible.
	sessions := mgr.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("sessions after restore: got %d, want 1", len(sessions))
	}
	if sessions[0].Target.Name != "shell" {
		t.Errorf("session target name: got %q, want %q", sessions[0].Target.Name, "shell")
	}

	// The persisted active ID (1) should map to the new session ID.
	activeID := mgr.ActiveID()
	if activeID == 0 {
		t.Error("expected active session to be set")
	}
	if activeID != result.Restored[0] {
		t.Errorf("active ID: got %d, want %d (first restored)", activeID, result.Restored[0])
	}
}

func TestRestoreFromState_FactoryFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mgr, cleanup := startManager(t)
	defer cleanup()

	state := &PersistedManagerState{
		Version: persistenceVersion,
		Sessions: []PersistedSession{
			{SessionID: 1, Target: SessionTarget{Name: "good"}, Command: "echo"},
			{SessionID: 2, Target: SessionTarget{Name: "bad"}, Command: "nonexistent"},
			{SessionID: 3, Target: SessionTarget{Name: "also-good"}, Command: "ls"},
		},
	}

	result, err := mgr.RestoreFromState(state, func(ps PersistedSession) (InteractiveSession, error) {
		if ps.Command == "nonexistent" {
			return nil, errors.New("factory: command not found")
		}
		return newControllableSession(), nil
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	if len(result.Restored) != 2 {
		t.Errorf("restored: got %d, want 2", len(result.Restored))
	}
	if len(result.Failed) != 1 {
		t.Fatalf("failed: got %d, want 1", len(result.Failed))
	}
	if result.Failed[0].SessionID != SessionID(2) {
		t.Errorf("failed session ID: got %d, want 2", result.Failed[0].SessionID)
	}

	sessions := mgr.Sessions()
	if len(sessions) != 2 {
		t.Errorf("sessions after restore: got %d, want 2", len(sessions))
	}
}

func TestRestoreFromState_ExportRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Phase 1: Create a manager, register sessions, export state.
	mgr1, cleanup1 := startManager(t)

	sess1 := newControllableSession()
	sess2 := newControllableSession()
	_, err := mgr1.Register(sess1, SessionTarget{Name: "alpha", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("register1: %v", err)
	}
	id2, err := mgr1.Register(sess2, SessionTarget{Name: "beta", Kind: SessionKindCapture})
	if err != nil {
		t.Fatalf("register2: %v", err)
	}
	if err := mgr1.Activate(id2); err != nil {
		t.Fatalf("activate: %v", err)
	}

	state, err := mgr1.ExportState()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	cleanup1()

	// Phase 2: Create a new manager, restore from state.
	mgr2, cleanup2 := startManager(t)
	defer cleanup2()

	result, err := mgr2.RestoreFromState(state, func(ps PersistedSession) (InteractiveSession, error) {
		return newControllableSession(), nil
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}

	if len(result.Restored) != 2 {
		t.Fatalf("restored: got %d, want 2", len(result.Restored))
	}
	if len(result.Failed) != 0 {
		t.Fatalf("failed: got %d, want 0", len(result.Failed))
	}

	// The restored sessions should be visible in the new manager.
	sessions := mgr2.Sessions()
	if len(sessions) != 2 {
		t.Fatalf("sessions: got %d, want 2", len(sessions))
	}

	// The previously-active session (id2) should be active in the new manager.
	// Note: The new IDs won't match the old ones, but the active session
	// should correspond to the session that was active in the persisted state.
	activeID := mgr2.ActiveID()
	if activeID == 0 {
		t.Error("expected active session to be set after restore")
	}

	// The active session should be the one that maps from the persisted active ID.
	// In the persisted state, ActiveID=2 (the "beta" session). The new active
	// session should have target "beta".
	sessions = mgr2.Sessions()
	activeTarget := SessionTarget{}
	for _, s := range sessions {
		if s.ID == activeID {
			activeTarget = s.Target
		}
	}
	if activeTarget.Name != "beta" {
		t.Errorf("active session target name: got %q, want %q", activeTarget.Name, "beta")
	}
}
