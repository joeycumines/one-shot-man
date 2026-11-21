package termmux

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type unregisterResultSession struct {
	*controllableSession
	closeErr   error
	closeCalls atomic.Int32
}

func (s *unregisterResultSession) Close() error {
	s.closeCalls.Add(1)
	return errors.Join(s.controllableSession.Close(), s.closeErr)
}

// Unregister detaches a session even when Close reports failure. The caller
// still needs that failure: removal from the manager is not proof that the
// underlying child terminated, and must not authorize worktree deletion.
func TestSessionManager_UnregisterReportsCloseFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		closeErr error
	}{
		{name: "close_failure", closeErr: errors.New("native close failed")},
		{name: "ordinary_close"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			manager, shutdown := startManager(t)
			defer shutdown()
			events := subscribeTestEvents(t, manager).channel()
			session := &unregisterResultSession{
				controllableSession: newControllableSession(),
				closeErr:            tc.closeErr,
			}
			id, err := manager.Register(session, SessionTarget{Name: "verify"})
			if err != nil {
				t.Fatal(err)
			}
			if manager.ActiveID() != id || manager.Snapshot(id) == nil {
				t.Fatal("registered session must be active with a snapshot")
			}

			err = manager.Unregister(id)
			if tc.closeErr == nil {
				if err != nil {
					t.Errorf("ordinary Unregister: %v", err)
				}
			} else if !errors.Is(err, tc.closeErr) {
				t.Errorf("Unregister error = %v, want original Close error %v", err, tc.closeErr)
			} else if !strings.Contains(err.Error(), "unregister") {
				t.Errorf("Unregister error lacks operation context: %v", err)
			}
			if calls := session.closeCalls.Load(); calls != 1 {
				t.Errorf("Close calls = %d, want 1", calls)
			}
			if manager.Snapshot(id) != nil || manager.ActiveID() != 0 || len(manager.Sessions()) != 0 {
				t.Error("Unregister did not remove the snapshot/session and clear the active ID")
			}
			if err := manager.Unregister(id); !errors.Is(err, ErrSessionNotFound) {
				t.Errorf("second Unregister = %v, want ErrSessionNotFound", err)
			}
			if err := manager.Unregister(id + 1000); !errors.Is(err, ErrSessionNotFound) {
				t.Errorf("unknown-ID Unregister = %v, want ErrSessionNotFound", err)
			}

			// A subsequent registration event is a FIFO barrier for the public
			// event stream, so counting Closed events needs no timing sleep.
			barrierID, err := manager.Register(newControllableSession(), SessionTarget{Name: "barrier"})
			if err != nil {
				t.Fatal(err)
			}
			closedEvents := 0
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
		readEvents:
			for {
				select {
				case event := <-events:
					if event.SessionID == id && event.Kind == EventSessionClosed {
						closedEvents++
					}
					if event.SessionID == barrierID && event.Kind == EventSessionRegistered {
						break readEvents
					}
				case <-deadline.C:
					t.Fatal("event barrier was not delivered")
				}
			}
			if closedEvents != 1 {
				t.Errorf("Closed events = %d, want exactly 1", closedEvents)
			}
			manager.Close()
			if calls := session.closeCalls.Load(); calls != 1 {
				t.Errorf("detached session was closed again: %d calls", calls)
			}
		})
	}
}
