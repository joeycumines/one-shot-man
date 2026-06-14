package termmux

import (
	"testing"
	"time"
)

func TestMonitorConfig_Default(t *testing.T) {
	cfg := MonitorConfig{}
	if cfg.Bell {
		t.Error("default Bell should be false")
	}
	if cfg.Activity {
		t.Error("default Activity should be false")
	}
	if cfg.Silence {
		t.Error("default Silence should be false")
	}
}

func TestNewMonitorState_InitializesLastOutputAt(t *testing.T) {
	before := time.Now()
	ms := NewMonitorState(MonitorConfig{Bell: true})
	after := time.Now()
	if ms.LastOutputAt.Before(before) || ms.LastOutputAt.After(after) {
		t.Errorf("LastOutputAt = %v, want between %v and %v", ms.LastOutputAt, before, after)
	}
	if !ms.Config.Bell {
		t.Error("Config.Bell should be true")
	}
}

func TestSessionManager_SetMonitorConfig_NotFound(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	err := m.SetMonitorConfig(999, MonitorConfig{Bell: true})
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestSessionManager_MonitorConfig_Default(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	cfg, err := m.MonitorConfig(id)
	if err != nil {
		t.Fatalf("MonitorConfig: %v", err)
	}
	if cfg.Bell {
		t.Error("default Bell should be false")
	}
}

func TestSessionManager_SetMonitorConfig_RoundTrip(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	want := MonitorConfig{
		Bell:              true,
		Activity:          true,
		ActivityThreshold: 5 * time.Second,
		Silence:           true,
		SilenceThreshold:  30 * time.Second,
	}
	if err := m.SetMonitorConfig(id, want); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	got, err := m.MonitorConfig(id)
	if err != nil {
		t.Fatalf("MonitorConfig: %v", err)
	}
	if got.Bell != want.Bell || got.Activity != want.Activity ||
		got.Silence != want.Silence ||
		got.ActivityThreshold != want.ActivityThreshold ||
		got.SilenceThreshold != want.SilenceThreshold {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSessionManager_VisualBell_NotActive(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	active, err := m.VisualBellActive(id)
	if err != nil {
		t.Fatalf("VisualBellActive: %v", err)
	}
	if active {
		t.Error("visual bell should not be active initially")
	}
}

func TestSessionManager_VisualBell_ActiveOnBell(t *testing.T) {
	m, cleanup := startManager(t, WithVisualBellDuration(500*time.Millisecond))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SetMonitorConfig(id, MonitorConfig{Bell: true}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	session.readerCh <- []byte{0x07}
	waitForEventKindCh(t, evtCh, EventBell, 2*time.Second)

	active, err := m.VisualBellActive(id)
	if err != nil {
		t.Fatalf("VisualBellActive: %v", err)
	}
	if !active {
		t.Error("visual bell should be active after BEL")
	}
}

func TestSessionManager_VisualBell_Expires(t *testing.T) {
	m, cleanup := startManager(t, WithVisualBellDuration(50*time.Millisecond))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SetMonitorConfig(id, MonitorConfig{Bell: true}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	session.readerCh <- []byte{0x07}
	waitForEventKindCh(t, evtCh, EventBell, 2*time.Second)

	time.Sleep(100 * time.Millisecond)

	active, err := m.VisualBellActive(id)
	if err != nil {
		t.Fatalf("VisualBellActive: %v", err)
	}
	if active {
		t.Error("visual bell should have expired")
	}
}

func TestSessionManager_ActivityEvent_BackgroundPane(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	s1 := newControllableSession()
	id1, err := m.Register(s1, SessionTarget{Name: "test1", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register 1: %v", err)
	}

	s2 := newControllableSession()
	id2, err := m.Register(s2, SessionTarget{Name: "test2", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register 2: %v", err)
	}

	if err := m.SetMonitorConfig(id2, MonitorConfig{Activity: true, ActivityThreshold: 0}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	m.Activate(id1)

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	s2.readerCh <- []byte("hello")

	evt := waitForEventKindCh(t, evtCh, EventActivity, 2*time.Second)
	if evt.SessionID != id2 {
		t.Errorf("EventActivity.SessionID = %d, want %d", evt.SessionID, id2)
	}
}

func TestSessionManager_ActivityEvent_NotFiredForActivePane(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id1, err := m.Register(session, SessionTarget{Name: "test1", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SetMonitorConfig(id1, MonitorConfig{Activity: true, ActivityThreshold: 0}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	session.readerCh <- []byte("hello")

	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case evt := <-evtCh:
			if evt.Kind == EventActivity {
				t.Error("EventActivity should not fire for active pane")
				return
			}
		case <-timeout:
			return
		}
	}
}

func TestSessionManager_SilenceEvent(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id1, err := m.Register(session, SessionTarget{Name: "test1", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SetMonitorConfig(id1, MonitorConfig{Silence: true, SilenceThreshold: 100 * time.Millisecond}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	subID, evtCh := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	count := m.CheckSilenceMonitors()
	if count != 1 {
		t.Errorf("CheckSilenceMonitors = %d, want 1", count)
	}

	evt := waitForEventKindCh(t, evtCh, EventSilence, 2*time.Second)
	if evt.SessionID != id1 {
		t.Errorf("EventSilence.SessionID = %d, want %d", evt.SessionID, id1)
	}
}

func TestSessionManager_SilenceEvent_ResetByOutput(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id1, err := m.Register(session, SessionTarget{Name: "test1", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SetMonitorConfig(id1, MonitorConfig{Silence: true, SilenceThreshold: 100 * time.Millisecond}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	count := m.CheckSilenceMonitors()
	if count != 1 {
		t.Fatalf("first CheckSilenceMonitors = %d, want 1", count)
	}

	count = m.CheckSilenceMonitors()
	if count != 0 {
		t.Errorf("second CheckSilenceMonitors = %d, want 0", count)
	}

	session.readerCh <- []byte("hello")
	time.Sleep(50 * time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	count = m.CheckSilenceMonitors()
	if count != 1 {
		t.Errorf("third CheckSilenceMonitors = %d, want 1", count)
	}
}

func TestSessionManager_SilenceEvent_DisabledWhenThresholdZero(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id1, err := m.Register(session, SessionTarget{Name: "test1", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := m.SetMonitorConfig(id1, MonitorConfig{Silence: true, SilenceThreshold: 0}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	count := m.CheckSilenceMonitors()
	if count != 0 {
		t.Errorf("CheckSilenceMonitors = %d, want 0 (threshold=0 disables)", count)
	}
}

// waitForEventKindCh reads from the event channel until an event of the
// given kind is found or the timeout expires.
func waitForEventKindCh(t *testing.T, ch <-chan Event, kind EventKind, timeout time.Duration) Event {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case evt := <-ch:
			if evt.Kind == kind {
				return evt
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s event", kind)
			return Event{}
		}
	}
}

// WithVisualBellDuration sets the duration of a visual bell flash.
func WithVisualBellDuration(d time.Duration) ManagerOption {
	return func(m *SessionManager) {
		m.visualBellDuration = d
	}
}
