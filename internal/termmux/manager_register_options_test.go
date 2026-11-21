package termmux

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestSessionManager_RegisterRemainOnExit(t *testing.T) {
	closeFailure := errors.New("first owner close failed")
	for _, tc := range []struct {
		name          string
		defaultRemain bool
		options       []RegisterOptions
		wantRetained  bool
		closeErr      error
	}{
		{name: "explicit_true", options: []RegisterOptions{{RemainOnExit: true}}, wantRetained: true},
		{name: "explicit_true_close_failure", options: []RegisterOptions{{RemainOnExit: true}}, wantRetained: true, closeErr: closeFailure},
		{name: "explicit_false", defaultRemain: true, options: []RegisterOptions{{RemainOnExit: false}}},
		{name: "inherit_false"},
		{name: "inherit_true", defaultRemain: true, wantRetained: true},
	} {
		for _, output := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/output_%t", tc.name, output), func(t *testing.T) {
				t.Parallel()
				manager, shutdown := startManager(t)
				defer shutdown()
				manager.SetRemainOnExit(tc.defaultRemain)
				events := subscribeTestEvents(t, manager).channel()
				session := &unregisterResultSession{controllableSession: newControllableSession(), closeErr: tc.closeErr}
				id, err := manager.Register(session, SessionTarget{Name: "owned"}, tc.options...)
				if err != nil {
					t.Fatal(err)
				}
				if got := manager.RemainOnExit(); got != tc.defaultRemain {
					t.Fatalf("registration changed manager default to %v", got)
				}
				// The default is captured during registration, not consulted at EOF.
				manager.SetRemainOnExit(!tc.defaultRemain)
				if output {
					session.readerCh <- []byte("final output")
				}
				session.closeReader()
				wantEvent := EventSessionClosed
				if tc.wantRetained {
					wantEvent = EventSessionExited
				}
				event := waitForEventKindCh(t, events, wantEvent, 5*time.Second)
				if event.SessionID != id {
					t.Fatalf("EOF event session = %d, want %d", event.SessionID, id)
				}
				// This public request is also a barrier after the EOF handler.
				registered := manager.Sessions()
				if tc.wantRetained {
					if len(registered) != 1 || registered[0].ID != id || registered[0].State != SessionExited || manager.Snapshot(id) == nil {
						t.Fatalf("EOF did not retain the exited session and snapshot: %+v", registered)
					}
					if calls := session.closeCalls.Load(); calls != 0 {
						t.Fatalf("EOF closed the owner-managed session %d times", calls)
					}
					if err := manager.Unregister(id); !errors.Is(err, tc.closeErr) {
						t.Errorf("Unregister = %v, want first Close result %v", err, tc.closeErr)
					}
					waitForEventKindCh(t, events, EventSessionClosed, 5*time.Second)
				} else {
					if len(registered) != 0 {
						t.Fatalf("ordinary EOF retained sessions: %+v", registered)
					}
					if err := manager.Unregister(id); !errors.Is(err, ErrSessionNotFound) {
						t.Errorf("auto-removed Unregister = %v, want ErrSessionNotFound", err)
					}
				}
				if len(manager.Sessions()) != 0 || manager.Snapshot(id) != nil || manager.ActiveID() != 0 {
					t.Error("closed session bookkeeping was not removed")
				}
				manager.Close()
				if calls := session.closeCalls.Load(); calls != 1 {
					t.Errorf("Close calls = %d, want exactly one across EOF/unregister/shutdown", calls)
				}
			})
		}
	}
}

type registerBarrierSession struct {
	*controllableSession
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *registerBarrierSession) Close() error {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.controllableSession.Close()
}

func TestSessionManager_RegisterCopiesOptionsBeforeEnqueue(t *testing.T) {
	manager, shutdown := startManager(t)
	defer shutdown()
	blocker := &registerBarrierSession{controllableSession: newControllableSession(), entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(blocker.release) }) }
	defer unblock()
	id, err := manager.Register(blocker, SessionTarget{Name: "barrier"})
	if err != nil {
		t.Fatal(err)
	}
	unregistered := make(chan error, 1)
	go func() { unregistered <- manager.Unregister(id) }()
	select {
	case <-blocker.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not enter Close barrier")
	}

	options := []RegisterOptions{{RemainOnExit: true}}
	session := &unregisterResultSession{controllableSession: newControllableSession()}
	registered := make(chan response, 1)
	go func() {
		id, err := manager.Register(session, SessionTarget{Name: "copied"}, options...)
		registered <- response{value: id, err: err}
	}()
	// The real worker is parked in the first session's Close. Hold the next
	// admitted request, mutate the caller's slice, and return that exact request
	// before releasing the worker. This establishes the admission boundary
	// without sleeps or racing a mutation against argument copying.
	var queued request
	select {
	case queued = <-manager.reqChan:
	case <-time.After(5 * time.Second):
		t.Fatal("Register request was not admitted")
	}
	options[0].RemainOnExit = false
	manager.reqChan <- queued
	unblock()
	if err := <-unregistered; err != nil {
		t.Fatal(err)
	}
	result := <-registered
	if result.err != nil {
		t.Fatal(result.err)
	}
	id = result.value.(SessionID)
	events := subscribeTestEvents(t, manager).channel()
	session.closeReader()
	waitForEventKindCh(t, events, EventSessionExited, 5*time.Second)
	if sessions := manager.Sessions(); len(sessions) != 1 || sessions[0].State != SessionExited || session.closeCalls.Load() != 0 {
		t.Fatalf("mutation changed admitted registration policy: %+v, close calls %d", sessions, session.closeCalls.Load())
	}
	if err := manager.Unregister(id); err != nil {
		t.Fatal(err)
	}
}

func TestSessionManager_RegisterRejectsMultipleOptions(t *testing.T) {
	manager, shutdown := startManager(t)
	defer shutdown()
	session := &unregisterResultSession{controllableSession: newControllableSession()}
	id, err := manager.Register(session, SessionTarget{}, RegisterOptions{}, RegisterOptions{RemainOnExit: true})
	if id != 0 || !errors.Is(err, ErrInvalidPayload) || len(manager.Sessions()) != 0 || session.closeCalls.Load() != 0 {
		t.Fatalf("invalid options mutated manager/session: id=%d err=%v close calls=%d", id, err, session.closeCalls.Load())
	}
}
