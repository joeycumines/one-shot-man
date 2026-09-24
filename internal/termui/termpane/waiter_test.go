package termpane

import (
	"fmt"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// The event-driven redraw contract: WaitForOutput wakes once per delivered
// session event (coalescing bursts), reports false while another waiter holds
// the pane, and reports false once the pane is closed.

func newWaiterTestPane(t *testing.T) (*Model, *controllableSession) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	mgr, session, sid, cleanup := startTestManager(t)
	t.Cleanup(cleanup)
	m := NewModel(sid, mgr, coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	})
	t.Cleanup(func() { _ = m.Close() })
	return m, session
}

func waitResult(t *testing.T, ch <-chan bool, d time.Duration, what string) bool {
	t.Helper()
	select {
	case ok := <-ch:
		return ok
	case <-time.After(d):
		t.Fatalf("timed out waiting for %s", what)
		return false
	}
}

func waitArmed(t *testing.T, m *Model) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !m.waiting.Load() {
		select {
		case <-deadline:
			t.Fatal("waiter never armed")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestWaitForOutput_WakesOnOutputAndCoalescesBursts(t *testing.T) {
	m, session := newWaiterTestPane(t)

	// A burst of chunks must produce a single wakeup that reports true.
	for i := range 3 {
		session.readerCh <- []byte(fmt.Sprintf("burst-%d\n", i))
	}
	result := make(chan bool, 1)
	go func() { result <- m.WaitForOutput() }()
	if ok := waitResult(t, result, 3*time.Second, "the burst waiter"); !ok {
		t.Fatal("WaitForOutput returned false while the pane is open and output arrived")
	}

	// With nothing pending, the next waiter blocks until a late event arrives.
	late := make(chan bool, 1)
	go func() { late <- m.WaitForOutput() }()
	select {
	case ok := <-late:
		t.Fatalf("WaitForOutput returned (ok=%v) with no pending event", ok)
	case <-time.After(150 * time.Millisecond):
	}
	session.readerCh <- []byte("late\n")
	if ok := waitResult(t, late, 3*time.Second, "the late-event waiter"); !ok {
		t.Fatal("WaitForOutput returned false for a late event on an open pane")
	}
}

func TestWaitForOutput_SingleFlight(t *testing.T) {
	m, session := newWaiterTestPane(t)

	holder := make(chan bool, 1)
	go func() { holder <- m.WaitForOutput() }()
	waitArmed(t, m)

	// While the first waiter owns the pane, a concurrent call must report
	// false without consuming anything.
	if m.WaitForOutput() {
		t.Fatal("a second concurrent waiter must report false while one is active")
	}

	// Releasing the first waiter with an event must also release the guard, so
	// a fresh waiter arms and wakes.
	session.readerCh <- []byte("release\n")
	if ok := waitResult(t, holder, 3*time.Second, "the holding waiter"); !ok {
		t.Fatal("the holding waiter must report true on delivery")
	}
	again := make(chan bool, 1)
	go func() { again <- m.WaitForOutput() }()
	waitArmed(t, m)
	session.readerCh <- []byte("again\n")
	if ok := waitResult(t, again, 3*time.Second, "the fresh waiter"); !ok {
		t.Fatal("the fresh waiter must report true on delivery")
	}
}

func TestWaitForOutput_ClosedPaneReportsFalse(t *testing.T) {
	m, _ := newWaiterTestPane(t)
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if m.WaitForOutput() {
		t.Fatal("WaitForOutput must report false once the pane is closed")
	}
}
