package termmux

import "testing"

func TestSessionLock_LockUnlock(t *testing.T) {
	var l SessionLock
	if l.IsLocked() {
		t.Error("new lock should not be locked")
	}
	if err := l.Lock("secret"); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if !l.IsLocked() {
		t.Error("after Lock should be locked")
	}
	if l.Unlock("wrong") {
		t.Error("wrong password should not unlock")
	}
	if !l.IsLocked() {
		t.Error("still locked after wrong password")
	}
	if !l.Unlock("secret") {
		t.Error("correct password should unlock")
	}
	if l.IsLocked() {
		t.Error("should be unlocked after correct password")
	}
}

func TestSessionLock_UnlockNotLocked(t *testing.T) {
	var l SessionLock
	if !l.Unlock("anything") {
		t.Error("Unlock on unlocked session should return true")
	}
}

func TestSessionManager_LockSession(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if m.IsLocked(id) {
		t.Error("new session should not be locked")
	}

	if err := m.LockSession(id, "mypassword"); err != nil {
		t.Fatalf("LockSession: %v", err)
	}

	if !m.IsLocked(id) {
		t.Error("after LockSession should be locked")
	}

	if m.UnlockSession(id, "wrongpass") {
		t.Error("wrong password should not unlock")
	}

	if !m.IsLocked(id) {
		t.Error("should still be locked after wrong password")
	}

	if !m.UnlockSession(id, "mypassword") {
		t.Error("correct password should unlock")
	}

	if m.IsLocked(id) {
		t.Error("should be unlocked after correct password")
	}
}

func TestSessionManager_LockSession_NotFound(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	err := m.LockSession(999, "test")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSessionManager_UnlockSession_NotFound(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	if m.UnlockSession(999, "test") {
		t.Error("expected false for nonexistent session")
	}
}
