package termmux

import (
	"testing"
	"time"
)

func TestSessionManager_RemainOnExit_Default(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	if m.RemainOnExit() {
		t.Error("default RemainOnExit should be false")
	}
}

func TestSessionManager_SetRemainOnExit(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	m.SetRemainOnExit(true)
	if !m.RemainOnExit() {
		t.Error("RemainOnExit should be true after setting")
	}

	m.SetRemainOnExit(false)
	if m.RemainOnExit() {
		t.Error("RemainOnExit should be false after clearing")
	}
}

func TestSessionManager_RemainOnExit_KeepsSessionAfterExit(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	m.SetRemainOnExit(true)

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	close(session.readerCh)

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventSessionExited && evt.SessionID == id {
				goto exited
			}
		case <-deadline:
			t.Fatal("timed out waiting for SessionExited")
		}
	}

exited:
	// Session should remain in Exited state (not Closed) and snapshot
	// should still be available.
	snap := m.Snapshot(id)
	if snap == nil {
		t.Error("snapshot should still be available after exit with remain-on-exit")
	}

	// Session should not have been closed — it should still appear in sessions.
	sessions := m.Sessions()
	found := false
	for _, si := range sessions {
		if si.ID == id {
			found = true
			if si.State != SessionExited {
				t.Errorf("session state = %v, want Exited", si.State)
			}
		}
	}
	if !found {
		t.Error("session should still be registered after exit with remain-on-exit")
	}
}

func TestSessionManager_RemainOnExit_Off_ClosesOnExit(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	close(session.readerCh)

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	waitForEventKindCh(t, evtCh, EventSessionClosed, 10*time.Second)
}

func TestSessionManager_RespawnSession(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	m.SetRemainOnExit(true)

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	session.readerCh <- []byte("ready")
	waitForSnapshotContains(t, m, id, "ready", 2*time.Second)

	close(session.readerCh)

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	waitForEventKindCh(t, evtCh, EventSessionExited, 10*time.Second)

	newID, err := m.RespawnSession(id)
	if err != nil {
		t.Fatalf("RespawnSession: %v", err)
	}
	if newID == 0 {
		t.Error("RespawnSession should return a valid session ID")
	}
}

func TestSessionManager_RespawnSession_NotExited(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err = m.RespawnSession(id)
	if err == nil {
		t.Error("expected error when respawning non-exited session")
	}
}
