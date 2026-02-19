package claudemux

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
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

	for i := 0; i < 5; i++ {
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

func TestInstance_WithMCPConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := NewInstanceRegistry(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("NewInstanceRegistry: %v", err)
	}

	inst, err := r.Create("mcp-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Attach MCP config.
	mcpCfg, err := NewMCPInstanceConfig("mcp-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	inst.MCP = mcpCfg

	// Close should clean up MCP too.
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// MCP temp dir should be removed.
	if _, err := os.Stat(mcpCfg.configDir); !os.IsNotExist(err) {
		t.Errorf("MCP configDir should be removed, stat: %v", err)
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

	for i := 0; i < n; i++ {
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

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Create("same-id")
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
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
