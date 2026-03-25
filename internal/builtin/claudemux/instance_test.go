package claudemux

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestNewInstanceRegistry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}
	if r.BaseDir() == "" {
		t.Error("BaseDir is empty")
	}
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0", r.Len())
	}
}

func TestNewInstanceRegistry_EmptyDir(t *testing.T) {
	t.Parallel()

	_, err := NewInstanceRegistry("")
	if err == nil {
		t.Error("expected error for empty baseDir")
	}
}

func TestInstanceRegistry_Create(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	inst, err := r.Create("agent-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "agent-1" {
		t.Errorf("ID = %q, want %q", inst.ID, "agent-1")
	}
	if inst.StateDir == "" {
		t.Error("StateDir is empty")
	}
	if inst.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if inst.IsClosed() {
		t.Error("instance should not be closed")
	}

	// State dir should exist.
	if _, err := os.Stat(inst.StateDir); err != nil {
		t.Errorf("StateDir does not exist: %v", err)
	}

	// Logs dir should exist.
	logsDir := filepath.Join(inst.StateDir, "logs")
	if _, err := os.Stat(logsDir); err != nil {
		t.Errorf("logs dir does not exist: %v", err)
	}

	// state.json should exist with correct content.
	data, err := os.ReadFile(filepath.Join(inst.StateDir, "state.json"))
	if err != nil {
		t.Fatalf("ReadFile state.json: %v", err)
	}
	var state InstanceState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}
	if state.ID != "agent-1" {
		t.Errorf("state.ID = %q, want %q", state.ID, "agent-1")
	}
	if state.Status != "active" {
		t.Errorf("state.Status = %q, want %q", state.Status, "active")
	}
}

func TestInstanceRegistry_Create_EmptyID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	_, err = r.Create("")
	if !errors.Is(err, ErrInstanceIDEmpty) {
		t.Errorf("Create empty ID: got %v, want ErrInstanceIDEmpty", err)
	}
}

func TestInstanceRegistry_Create_Duplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	_, err = r.Create("dup")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err = r.Create("dup")
	if !errors.Is(err, ErrInstanceIDExists) {
		t.Errorf("duplicate Create: got %v, want ErrInstanceIDExists", err)
	}
}

func TestInstanceRegistry_Get(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	_, err = r.Create("get-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	inst, ok := r.Get("get-test")
	if !ok || inst == nil {
		t.Fatal("Get returned false")
	}
	if inst.ID != "get-test" {
		t.Errorf("ID = %q, want %q", inst.ID, "get-test")
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("Get nonexistent should return false")
	}
}

func TestInstanceRegistry_Close_Instance(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	_, err = r.Create("close-me")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}

	if err := r.Close("close-me"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if r.Len() != 0 {
		t.Errorf("Len after Close = %d, want 0", r.Len())
	}

	// Get should return false after close.
	_, ok := r.Get("close-me")
	if ok {
		t.Error("Get after Close should return false")
	}
}

func TestInstanceRegistry_Close_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	err = r.Close("ghost")
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("Close ghost: got %v, want ErrInstanceNotFound", err)
	}
}

func TestInstanceRegistry_CloseAll(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	for i := range 5 {
		_, err := r.Create(filepath.Join("agent", string(rune('a'+i))))
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	if r.Len() != 5 {
		t.Errorf("Len = %d, want 5", r.Len())
	}

	if err := r.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	if r.Len() != 0 {
		t.Errorf("Len after CloseAll = %d, want 0", r.Len())
	}
}

func TestInstanceRegistry_List(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	names := []string{"charlie", "alpha", "bravo"}
	for _, name := range names {
		if _, err := r.Create(name); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	// List should be sorted.
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("List len = %d, want 3", len(got))
	}
	if got[0] != "alpha" || got[1] != "bravo" || got[2] != "charlie" {
		t.Errorf("List = %v, want [alpha bravo charlie]", got)
	}
}

func TestInstance_Close(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	inst, err := r.Create("close-inst")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Attach a mock agent.
	mock := &mockHandle{alive: true}
	inst.Agent = mock

	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !inst.IsClosed() {
		t.Error("instance should be closed")
	}
	if mock.alive {
		t.Error("agent should not be alive after Close")
	}

	// state.json should say "closed".
	data, _ := os.ReadFile(filepath.Join(inst.StateDir, "state.json"))
	var state InstanceState
	_ = json.Unmarshal(data, &state)
	if state.Status != "closed" {
		t.Errorf("state.Status = %q, want closed", state.Status)
	}

	// Double close should be safe.
	if err := inst.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestInstance_Close_NilAgent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	inst, err := r.Create("nil-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Close with nil agent should not panic.
	if err := inst.Close(); err != nil {
		t.Fatalf("Close nil agent: %v", err)
	}
}

// TestInstanceRegistry_ConcurrentCreate tests concurrent Create calls
// to verify the sync.Map provides proper isolation.
func TestInstanceRegistry_ConcurrentCreate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := string(rune('A' + idx))
			_, errs[idx] = r.Create(id)
		}(i)
	}
	wg.Wait()

	// All creates should succeed (unique IDs).
	for i, err := range errs {
		if err != nil {
			t.Errorf("Create %d: %v", i, err)
		}
	}
	if r.Len() != n {
		t.Errorf("Len = %d, want %d", r.Len(), n)
	}
}

// TestInstanceRegistry_ConcurrentDuplicate tests that concurrent Create
// with the same ID rejects duplicates.
func TestInstanceRegistry_ConcurrentDuplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	const n = 10
	var wg sync.WaitGroup
	successes := 0
	var mu sync.Mutex

	for range n {
		wg.Go(func() {
			_, err := r.Create("same-id")
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("successes = %d, want exactly 1 (only one should win the race)", successes)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
}

func TestInstanceRegistry_SpecialCharSessionID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	inst, err := r.Create("my agent/session:with<special>chars")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(inst.StateDir); err != nil {
		t.Errorf("StateDir should exist: %v", err)
	}
}

// --- Create coverage: truncation and error paths ---

func TestInstanceRegistry_Create_LongSessionID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	// 80-char alphanumeric ID — should be truncated to 64 chars for the
	// directory name, but the Instance.ID should retain the full original value.
	longID := strings.Repeat("a", 80)
	inst, err := r.Create(longID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if inst.ID != longID {
		t.Errorf("ID = %q (len %d), want %q (len %d)", inst.ID, len(inst.ID), longID, len(longID))
	}

	// StateDir basename should be exactly 64 chars.
	base := filepath.Base(inst.StateDir)
	if len(base) != 64 {
		t.Errorf("StateDir basename len = %d, want 64 (truncated)", len(base))
	}

	// State dir must actually exist.
	if _, err := os.Stat(inst.StateDir); err != nil {
		t.Errorf("StateDir should exist: %v", err)
	}
}

func TestInstanceRegistry_Create_StateDirFail(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("invalid path trick not reliable on Windows")
	}

	// Use /dev/null (a regular file, not a directory) as the base dir.
	// MkdirAll under a file path will fail.
	r := &InstanceRegistry{baseDir: "/dev/null/impossible"}

	_, err := r.Create("test-session")
	if err == nil {
		t.Fatal("expected error when base dir is an invalid path")
	}
	if !strings.Contains(err.Error(), "state dir") {
		t.Errorf("error = %q, expected to mention 'state dir'", err)
	}

	// Instance must NOT be registered on failure.
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0 (failed Create should not register)", r.Len())
	}
}

func TestInstanceRegistry_Create_WriteStateFail(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("chmod-based test not reliable on Windows")
	}
	testutil.SkipIfRoot(t, testutil.DetectPlatform(t), "chmod restrictions bypassed by root")

	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	// Pre-create the state directory as read-only, so writeState (which
	// tries to write state.json inside it) will fail.
	sessionID := "readonly-session"
	safe := "readonly-session" // no special chars
	stateDir := filepath.Join(dir, "sessions", safe)
	if err := os.MkdirAll(filepath.Join(stateDir, "logs"), 0700); err != nil {
		t.Fatalf("pre-create stateDir: %v", err)
	}
	// Make stateDir read-only so os.WriteFile inside writeState fails.
	if err := os.Chmod(stateDir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() {
		// Restore write permission for cleanup.
		_ = os.Chmod(stateDir, 0700)
	})

	_, err = r.Create(sessionID)
	if err == nil {
		t.Fatal("expected error when state dir is read-only")
	}
	if !strings.Contains(err.Error(), "write initial state") {
		t.Errorf("error = %q, expected to mention 'write initial state'", err)
	}

	// Instance must NOT be registered on failure.
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0 (failed Create should not register)", r.Len())
	}
}

func TestInstanceRegistry_Create_LogsDirFail(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("file-as-dir trick not reliable on Windows")
	}

	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	// Pre-create the state directory with "logs" as a regular file so that
	// MkdirAll(stateDir + "/logs") fails.
	sessionID := "logs-blocked"
	stateDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatalf("pre-create stateDir: %v", err)
	}
	// Create "logs" as a regular file (not a directory).
	if err := os.WriteFile(filepath.Join(stateDir, "logs"), []byte("blocker"), 0600); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}

	_, err = r.Create(sessionID)
	if err == nil {
		t.Fatal("expected error when logs path is a regular file")
	}
	if !strings.Contains(err.Error(), "logs dir") {
		t.Errorf("error = %q, expected to mention 'logs dir'", err)
	}

	// Instance must NOT be registered on failure.
	if r.Len() != 0 {
		t.Errorf("Len = %d, want 0 (failed Create should not register)", r.Len())
	}
}
