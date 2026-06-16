package termmux

import (
	"testing"
	"time"
)

func TestSessionManager_PaneExited_RemainOnExit(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	m.SetRemainOnExit(true)

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "test", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	pm := m.activePaneManager()
	binding := pm.Binding(paneID)
	if binding == nil {
		t.Fatal("missing pane binding")
	}
	id := binding.SessionID

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	close(session.readerCh)
	waitForSessionExited(t, m, id, 10*time.Second)

	if !m.PaneExited(paneID) {
		t.Errorf("PaneExited(%d) = false, want true", paneID)
	}
}

func TestSessionManager_PaneExited_ClosePane_CleansBinding(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	m.SetRemainOnExit(true)

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "test", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	pm := m.activePaneManager()
	binding := pm.Binding(paneID)
	if binding == nil {
		t.Fatal("missing pane binding")
	}
	id := binding.SessionID

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	close(session.readerCh)
	waitForSessionExited(t, m, id, 10*time.Second)

	if err := m.ClosePane(paneID); err != nil {
		t.Fatalf("ClosePane(%d): %v", paneID, err)
	}

	if m.PaneExited(paneID) {
		t.Errorf("PaneExited(%d) = true after ClosePane, want false", paneID)
	}
	if _, err := m.PaneRemainOnExit(paneID); err == nil {
		t.Errorf("PaneRemainOnExit(%d) expected error after ClosePane", paneID)
	}
}

func TestSessionManager_PaneExited_NoRemainOnExit(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "test", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	pm := m.activePaneManager()
	binding := pm.Binding(paneID)
	if binding == nil {
		t.Fatal("missing pane binding")
	}
	id := binding.SessionID

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	close(session.readerCh)
	_, closedCh := m.Subscribe(16)
	waitForEventKindCh(t, closedCh, EventSessionClosed, 10*time.Second)

	if m.PaneExited(paneID) {
		t.Errorf("PaneExited(%d) = true without remain-on-exit, want false", paneID)
	}

	if err := m.ClosePane(paneID); err != nil {
		t.Fatalf("ClosePane(%d): %v", paneID, err)
	}
	if _, err := m.PaneRemainOnExit(paneID); err == nil {
		t.Errorf("PaneRemainOnExit(%d) expected error after ClosePane", paneID)
	}
}

func TestSessionManager_RespawnSession_ClearsPaneExited(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	m.SetRemainOnExit(true)

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "test", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	pm := m.activePaneManager()
	binding := pm.Binding(paneID)
	if binding == nil {
		t.Fatal("missing pane binding")
	}
	id := binding.SessionID

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)
	if err := m.SetPaneRemainOnExit(paneID, true); err != nil {
		t.Fatalf("SetPaneRemainOnExit: %v", err)
	}

	close(session.readerCh)
	waitForSessionExited(t, m, id, 10*time.Second)

	if !m.PaneExited(paneID) {
		t.Fatalf("PaneExited(%d) = false before respawn, want true", paneID)
	}

	newID, err := m.RespawnSession(id)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}
	if newID == 0 || newID == id {
		t.Fatalf("RespawnSession returned unexpected id: %d", newID)
	}

	if active := m.ActivePaneID(); active != paneID {
		t.Errorf("active pane changed from %d to %d after respawn", paneID, active)
	}
	if m.PaneExited(paneID) {
		t.Errorf("PaneExited(%d) = true after respawn, want false", paneID)
	}
	binding = pm.Binding(paneID)
	if binding.SessionID != newID {
		t.Errorf("pane session id = %d after respawn, want %d", binding.SessionID, newID)
	}
}
