package termmux

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// buildExitProgram creates a tiny executable that prints "hello respawn"
// to stdout and exits. The binary path is returned.
func buildExitProgram(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("spawns process to build test helper")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main
import (
	"fmt"
	"os"
)
func main() {
	if v := os.Getenv("MARKER"); v != "" {
		fmt.Println(v)
		return
	}
	fmt.Println("hello respawn")
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	binName := "exitprogram"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

func waitForSessionExited(t *testing.T, m *SessionManager, id SessionID, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		sessions := m.Sessions()
		for _, s := range sessions {
			if s.ID == id && s.State == SessionExited {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for session %d to exit", id)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRespawnSession_PreservesCaptureConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns child process")
	}

	bin := buildExitProgram(t)

	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	m.SetRemainOnExit(true)

	cfg := CaptureConfig{
		Command: bin,
		Env:     map[string]string{"RESPAWN_TEST": "ok"},
	}
	cs := NewCaptureSession(cfg)
	if err := cs.Start(m.readerCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	id, err := m.Register(cs, SessionTarget{Name: "respawn-test", Kind: SessionKindCapture})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	waitForSnapshotContains(t, m, id, "hello respawn", 5*time.Second)
	waitForSessionExited(t, m, id, 5*time.Second)

	newID, err := m.RespawnSession(id)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}
	if newID == 0 || newID == id {
		t.Fatalf("RespawnSession returned unexpected id: %d", newID)
	}

	waitForSnapshotContains(t, m, newID, "hello respawn", 5*time.Second)
	sessions := m.Sessions()
	var found bool
	for _, s := range sessions {
		if s.ID == id {
			t.Fatalf("original session %d still present after respawn", id)
		}
		if s.ID == newID {
			found = true
			if s.Target.Name != "respawn-test" {
				t.Errorf("target name = %q, want %q", s.Target.Name, "respawn-test")
			}
		}
	}
	if !found {
		t.Fatalf("respawned session %d not found", newID)
	}
}

func TestRespawnSession_RebindsPane(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns child process")
	}

	bin := buildExitProgram(t)

	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	m.SetRemainOnExit(true)

	cs := NewCaptureSession(CaptureConfig{Command: bin})
	if err := cs.Start(m.readerCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	paneID, err := m.NewPane(cs, SessionTarget{Name: "pane-rebind", Kind: SessionKindCapture}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	panes := m.Panes()
	var oldPane Pane
	for _, p := range panes {
		if p.ID == paneID {
			oldPane = p
		}
	}
	if oldPane.ID == 0 {
		t.Fatalf("pane %d not found", paneID)
	}
	oldID := oldPane.SessionID

	waitForSessionExited(t, m, oldID, 5*time.Second)

	newID, err := m.RespawnSession(oldID)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}

	panes = m.Panes()
	var newPane Pane
	for _, p := range panes {
		if p.ID == paneID {
			newPane = p
		}
	}
	if newPane.ID == 0 {
		t.Fatalf("pane %d missing after respawn", paneID)
	}
	if newPane.SessionID != newID {
		t.Errorf("pane session id = %d, want %d", newPane.SessionID, newID)
	}
	if newPane.VTerm == nil || newPane.VTerm == oldPane.VTerm {
		t.Errorf("pane vterm not updated")
	}
}

func TestRespawnSession_EmitsEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns child process")
	}

	bin := buildExitProgram(t)

	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	m.SetRemainOnExit(true)

	cs := NewCaptureSession(CaptureConfig{Command: bin})
	if err := cs.Start(m.readerCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	subID, evtCh := m.Subscribe(32)
	defer m.Unsubscribe(subID)

	id, err := m.Register(cs, SessionTarget{Name: "event-test", Kind: SessionKindCapture})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	waitForSessionExited(t, m, id, 5*time.Second)

	newID, err := m.RespawnSession(id)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}

	var gotRegistered, gotActivated bool
	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionRegistered && evt.SessionID == newID {
				gotRegistered = true
			}
			if evt.Kind == EventSessionActivated && evt.SessionID == newID {
				gotActivated = true
			}
			if gotRegistered && gotActivated {
				return
			}
		case <-deadline:
			if !gotRegistered {
				t.Errorf("missing EventSessionRegistered for %d", newID)
			}
			if !gotActivated {
				t.Errorf("missing EventSessionActivated for %d", newID)
			}
			return
		}
	}
}

func TestRespawnSession_NotExited(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	cs := NewCaptureSession(CaptureConfig{Command: "go"})
	id, err := m.Register(cs, SessionTarget{Name: "not-exited", Kind: SessionKindCapture})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err = m.RespawnSession(id)
	if err == nil {
		t.Error("expected error when respawning non-exited session")
	}
}

func TestRespawnSession_WindowRebind(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns child process")
	}

	bin := buildExitProgram(t)

	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	m.SetRemainOnExit(true)

	cs := NewCaptureSession(CaptureConfig{Command: bin})
	if err := cs.Start(m.readerCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	paneID, err := m.NewPane(cs, SessionTarget{Name: "window-rebind", Kind: SessionKindCapture}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	panes := m.Panes()
	var oldID SessionID
	for _, p := range panes {
		if p.ID == paneID {
			oldID = p.SessionID
		}
	}
	if oldID == 0 {
		t.Fatalf("session id for pane %d not found", paneID)
	}

	winID, newPaneID, _, err := m.BreakPane(paneID)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}
	_ = newPaneID

	waitForSessionExited(t, m, oldID, 5*time.Second)

	newID, err := m.RespawnSession(oldID)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}

	wp := m.WindowPanes()
	var found bool
	for wid, panes := range wp {
		if wid != winID {
			continue
		}
		for _, p := range panes {
			if p.ID == newPaneID {
				found = true
				if p.SessionID != newID {
					t.Errorf("window pane session id = %d, want %d", p.SessionID, newID)
				}
			}
		}
	}
	if !found {
		t.Fatalf("pane %d not found in window %d after respawn", newPaneID, winID)
	}
}

func TestRespawnSession_PreservesEnvAndDir(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns child process")
	}

	binPath := buildExitProgram(t)
	dir := filepath.Dir(binPath)

	m, cleanup := startManager(t, WithTermSize(10, 40))
	defer cleanup()

	m.SetRemainOnExit(true)

	cfg := CaptureConfig{
		Command: binPath,
		Dir:     dir,
		Env:     map[string]string{"MARKER": "preserved"},
	}
	cs := NewCaptureSession(cfg)
	if err := cs.Start(m.readerCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	id, err := m.Register(cs, SessionTarget{Name: "env-dir", Kind: SessionKindCapture})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	waitForSessionExited(t, m, id, 5*time.Second)

	newID, err := m.RespawnSession(id)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}

	waitForSnapshotContains(t, m, newID, "preserved", 5*time.Second)

}
