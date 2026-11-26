package termmux

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// registerOwnedSession separates natural EOF from Close, as a completed
// CaptureSession does. Its first Close outcome must remain observable by the
// owner after both silent and output-producing children have exited.
type registerOwnedSession struct {
	reader     chan []byte
	done       chan struct{}
	doneOnce   sync.Once
	closeErr   error
	closeCalls atomic.Int32
}

func (s *registerOwnedSession) Write(p []byte) (int, error) { return len(p), nil }
func (s *registerOwnedSession) Resize(int, int) error       { return nil }
func (s *registerOwnedSession) Reader() <-chan []byte       { return s.reader }
func (s *registerOwnedSession) Done() <-chan struct{}       { return s.done }
func (s *registerOwnedSession) Close() error {
	s.closeCalls.Add(1)
	s.doneOnce.Do(func() { close(s.done) })
	return s.closeErr
}

func registrationEvents(t *testing.T, manager *parent.SessionManager) (<-chan parent.Event, func()) {
	t.Helper()
	events := manager.SubscribeEvents()
	queued := make(chan parent.Event, 32)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		defer manager.UnsubscribeEvents()
		for {
			select {
			case <-stop:
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				manager.AckEvent()
				select {
				case queued <- event:
				case <-stop:
					return
				}
			}
		}
	}()
	var once sync.Once
	return queued, func() {
		once.Do(func() { close(stop) })
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("registration event subscriber did not stop")
		}
	}
}

// This test intentionally uses only the existing Go binding interfaces. The
// same source can run against a binding that ignores register's third JS
// argument and demonstrate the resulting premature Close and missing ID.
func TestMuxRegisterRemainOnExit(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a JavaScript event loop")
	}
	for _, tc := range []struct {
		name          string
		defaultRemain bool
		options       string
		wantRetained  bool
		closeErr      error
	}{
		{name: "explicit_true", options: "{remainOnExit: true}", wantRetained: true},
		{name: "explicit_true_close_failure", options: "{remainOnExit: true}", wantRetained: true, closeErr: errors.New("first owner close failed")},
		{name: "explicit_false", defaultRemain: true, options: "{remainOnExit: false}"},
		{name: "inherit_false"},
		{name: "inherit_true", defaultRemain: true, wantRetained: true},
		{name: "empty_options", defaultRemain: true, options: "{}", wantRetained: true},
		{name: "null_property", defaultRemain: true, options: "{remainOnExit: null}", wantRetained: true},
		{name: "undefined_property", defaultRemain: true, options: "{remainOnExit: undefined}", wantRetained: true},
	} {
		for _, output := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/output_%t", tc.name, output), func(t *testing.T) {
				ctx := t.Context()
				manager := parent.NewSessionManager()
				managerDone := make(chan struct{})
				go func() {
					defer close(managerDone)
					_ = manager.Run(ctx)
				}()
				select {
				case <-manager.Started():
				case <-time.After(5 * time.Second):
					t.Fatal("manager did not start")
				}
				defer func() {
					manager.Close()
					<-managerDone
				}()
				manager.SetRemainOnExit(tc.defaultRemain)
				events, stopEvents := registrationEvents(t, manager)
				defer stopEvents()

				vm := goja.New()
				mux := wrapTestSessionManagerWithLoop(t, ctx, vm, manager, nil, nil, -1, "")
				setOnLoop(t, vm, "mux", mux)
				session := &registerOwnedSession{reader: make(chan []byte, 1), done: make(chan struct{}), closeErr: tc.closeErr}
				adapter := adapterForRuntime(t, vm)
				setOnLoop(t, vm, "makeSession", func() goja.Value {
					wrapped, _ := wrapInteractiveSessionWithOrder(ctx, adapter, vm, session, parent.SessionKindPTY)
					return wrapped
				})
				registration := `var sessionID = await mux.register(makeSession(), {kind: 'verify', name: 'owned'}); return sessionID;`
				if tc.options != "" {
					registration = `var options = ` + tc.options + `;
						var sessionID = await mux.register(makeSession(), {kind: 'verify', name: 'owned'}, options);
						options.remainOnExit = !options.remainOnExit;
						return sessionID;`
				}
				value, err := awaitJSValue(t, vm, registration)
				if err != nil {
					t.Fatal(err)
				}
				id := parent.SessionID(value.ToInteger())
				if manager.RemainOnExit() != tc.defaultRemain {
					t.Fatal("per-registration option changed the manager default")
				}
				manager.SetRemainOnExit(!tc.defaultRemain)
				if output {
					session.reader <- []byte("final output")
				}
				session.doneOnce.Do(func() { close(session.done) })
				close(session.reader)

				deadline := time.NewTimer(5 * time.Second)
				defer deadline.Stop()
			eof:
				for {
					select {
					case event := <-events:
						if event.SessionID == id && (event.Kind == parent.EventSessionExited || event.Kind == parent.EventSessionClosed) {
							break eof
						}
					case <-deadline.C:
						t.Fatal("manager did not observe natural EOF")
					}
				}
				// Sessions is a public worker barrier after the EOF handler; the
				// Exited event alone precedes default-policy auto-removal.
				registered := manager.Sessions()
				if tc.wantRetained {
					if len(registered) != 1 || registered[0].ID != id || registered[0].State != parent.SessionExited || manager.Snapshot(id) == nil {
						t.Fatalf("natural EOF lost owner registration/snapshot: %+v; Close calls=%d", registered, session.closeCalls.Load())
					}
					if calls := session.closeCalls.Load(); calls != 0 {
						t.Fatalf("natural EOF consumed first owner Close outcome: %d calls", calls)
					}
					_, err := awaitJSValue(t, vm, fmt.Sprintf("await mux.unregister(%d);", id))
					if tc.closeErr == nil {
						if err != nil {
							t.Fatalf("ordinary owner unregister: %v", err)
						}
					} else if err == nil || !strings.Contains(err.Error(), tc.closeErr.Error()) {
						t.Fatalf("owner unregister error=%v, want first Close failure %v", err, tc.closeErr)
					}
				} else if len(registered) != 0 {
					t.Fatalf("default/false registration retained exited session: %+v", registered)
				}
				if len(manager.Sessions()) != 0 || manager.Snapshot(id) != nil || manager.ActiveID() != 0 {
					t.Fatal("closed session bookkeeping remained registered")
				}
				manager.Close()
				if calls := session.closeCalls.Load(); calls != 1 {
					t.Errorf("underlying Close calls=%d, want exactly one across EOF, unregister, and shutdown", calls)
				}
			})
		}
	}
}
