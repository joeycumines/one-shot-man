package termmux

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionManager_SetPipeFile(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	dir := t.TempDir()
	pipePath := filepath.Join(dir, "output.log")

	if err := m.SetPipeFile(id, pipePath); err != nil {
		t.Fatalf("SetPipeFile: %v", err)
	}

	session.readerCh <- []byte("hello from pipe")
	waitForSnapshotContains(t, m, id, "hello from pipe", 2*time.Second)

	if err := m.ClearPipe(id); err != nil {
		t.Fatalf("ClearPipe: %v", err)
	}

	data, err := os.ReadFile(pipePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello from pipe" {
		t.Errorf("pipe output = %q, want %q", string(data), "hello from pipe")
	}
}

func TestSessionManager_SetPipeFile_NotFound(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	err := m.SetPipeFile(999, "/tmp/nonexistent.log")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSessionManager_ClearPipe_NoPipe(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.ClearPipe(id); err != nil {
		t.Fatalf("ClearPipe should succeed even without active pipe: %v", err)
	}
}

func TestSessionManager_SetPipeFile_InvalidPath(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	err = m.SetPipeFile(id, "/nonexistent/deeply/nested/dir/output.log")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}
