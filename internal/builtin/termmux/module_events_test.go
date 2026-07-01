package termmux

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// chanStringIO is a test double for parent.StringIO that reads from a
// buffered channel. Closing the channel makes Receive return io.EOF.
type chanStringIO struct {
	ch   chan string
	sent []string
}

func newChanStringIO() (*chanStringIO, chan string) {
	ch := make(chan string, 16)
	return &chanStringIO{ch: ch}, ch
}

func (s *chanStringIO) Send(input string) error {
	s.sent = append(s.sent, input)
	return nil
}

func (s *chanStringIO) Receive() (string, error) {
	v, ok := <-s.ch
	if !ok {
		return "", io.EOF
	}
	return v, nil
}

func (s *chanStringIO) Close() error { return nil }

// waitForEvents drains the event loop until the named JS array has at least
// wantCount entries or the deadline expires.
func waitForEvents(t *testing.T, runtime *goja.Runtime, varName string, wantCount int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, _ = runJS(t, runtime, `queueMicrotask(() => {})`)
		v, err := runJS(t, runtime, varName+`.length`)
		if err != nil {
			t.Fatalf("check %s.length: %v", varName, err)
		}
		if int(v.ToInteger()) >= wantCount {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s to reach %d events", varName, wantCount)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEventBridge_UnknownEventRejected(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	mgr := parent.NewSessionManager()
	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runtime.RunString(`mux.on('not-a-real-event', function() {})`)
	if err == nil {
		t.Fatal("expected error for unknown event")
	}
	if !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("expected unknown event error, got: %v", err)
	}
}

func TestEventBridge_Title(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio, ch := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var titleEvents = [];
		mux.addEventListener('title', function(evt) { titleEvents.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup title listener: %v", err)
	}

	id, err := mgr.Register(sess, parent.SessionTarget{Name: "title-test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	ch <- "\x1b]0;My Title\x07"
	close(ch)

	waitForEvents(t, runtime, "titleEvents", 1)

	sidJSON := sessionIDJSON(uint64(id))
	_, err = runJS(t, runtime, fmt.Sprintf(`
		var got = titleEvents[0];
		if (!got) { throw new Error("no title event"); }
		if (got.sessionId !== %s) { throw new Error("sessionId mismatch"); }
		if (got.data !== "My Title") { throw new Error("data mismatch"); }
	`, sidJSON))
	if err != nil {
		t.Errorf("title event fields: %v", err)
	}

	cancel()
	<-errCh
}

func TestEventBridge_WorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio, ch := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var cwdEvents = [];
		mux.addEventListener('cwd', function(evt) { cwdEvents.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup cwd listener: %v", err)
	}

	id, err := mgr.Register(sess, parent.SessionTarget{Name: "cwd-test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	ch <- "\x1b]7;file:///home/user\x07"
	close(ch)

	waitForEvents(t, runtime, "cwdEvents", 1)

	sidJSON := sessionIDJSON(uint64(id))
	_, err = runJS(t, runtime, fmt.Sprintf(`
		var got = cwdEvents[0];
		if (got.sessionId !== %s) { throw new Error("sessionId mismatch"); }
		if (got.data !== "file:///home/user") { throw new Error("data mismatch"); }
	`, sidJSON))
	if err != nil {
		t.Errorf("cwd event fields: %v", err)
	}

	cancel()
	<-errCh
}

func TestEventBridge_Clipboard(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio, ch := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var clipboardEvents = [];
		mux.addEventListener('clipboard', function(evt) { clipboardEvents.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup clipboard listener: %v", err)
	}

	id, err := mgr.Register(sess, parent.SessionTarget{Name: "clipboard-test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	ch <- "\x1b]52;c;SGVsbG8=\x07"
	close(ch)

	waitForEvents(t, runtime, "clipboardEvents", 1)

	v, err := runJS(t, runtime, `
		(function() {
			var e = clipboardEvents[0];
			return JSON.stringify({ sessionId: e.sessionId, data: e.data });
		})()
	`)
	if err != nil {
		t.Fatalf("stringify clipboard event: %v", err)
	}
	got := v.String()
	want := `{"sessionId":` + sessionIDJSON(uint64(id)) + `,"data":"c;SGVsbG8="}`
	if got != want {
		t.Errorf("clipboard event = %s, want %s", got, want)
	}

	cancel()
	<-errCh
}

func TestEventBridge_Activity(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	activeSIO, activeCh := newChanStringIO()
	activeSess := parent.NewStringIOSession(activeSIO)
	activeSess.Start()

	bgSIO, bgCh := newChanStringIO()
	bgSess := parent.NewStringIOSession(bgSIO)
	bgSess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var activityEvents = [];
		mux.addEventListener('activity', function(evt) { activityEvents.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup activity listener: %v", err)
	}

	activeID, err := mgr.Register(activeSess, parent.SessionTarget{Name: "active", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register active: %v", err)
	}
	if err := mgr.Activate(activeID); err != nil {
		t.Fatalf("Activate active: %v", err)
	}

	bgID, err := mgr.Register(bgSess, parent.SessionTarget{Name: "background", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register background: %v", err)
	}
	if err := mgr.SetMonitorConfig(bgID, parent.MonitorConfig{
		Activity:          true,
		ActivityThreshold: 0,
	}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	close(activeCh)
	bgCh <- "hello"
	close(bgCh)

	waitForEvents(t, runtime, "activityEvents", 1)

	v, err := runJS(t, runtime, `JSON.stringify(activityEvents[0])`)
	if err != nil {
		t.Fatalf("stringify activity event: %v", err)
	}
	got := v.String()
	want := `{"sessionId":` + sessionIDJSON(uint64(bgID)) + `}`
	if got != want {
		t.Errorf("activity event = %s, want %s", got, want)
	}

	cancel()
	<-errCh
}

func TestEventBridge_Silence(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio, ch := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var silenceEvents = [];
		mux.addEventListener('silence', function(evt) { silenceEvents.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup silence listener: %v", err)
	}

	id, err := mgr.Register(sess, parent.SessionTarget{Name: "silence-test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.SetMonitorConfig(id, parent.MonitorConfig{
		Silence:          true,
		SilenceThreshold: 1,
	}); err != nil {
		t.Fatalf("SetMonitorConfig: %v", err)
	}

	// Wait for the silence threshold to elapse. NewMonitorState sets
	// LastOutputAt = time.Now(), so the silence timer starts from config
	// time, not from session creation.
	time.Sleep(1100 * time.Millisecond)

	count := mgr.CheckSilenceMonitors()
	if count == 0 {
		t.Fatal("CheckSilenceMonitors emitted no silence events")
	}

	waitForEvents(t, runtime, "silenceEvents", 1)

	v, err := runJS(t, runtime, `JSON.stringify(silenceEvents[0])`)
	if err != nil {
		t.Fatalf("stringify silence event: %v", err)
	}
	got := v.String()
	want := `{"sessionId":` + sessionIDJSON(uint64(id)) + `}`
	if got != want {
		t.Errorf("silence event = %s, want %s", got, want)
	}

	close(ch)
	cancel()
	<-errCh
}

func TestEventBridge_MultipleListeners(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	sio, ch := newChanStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var titleA = [];
		var titleB = [];
		mux.addEventListener('title', function(evt) { titleA.push(evt.detail); });
		mux.addEventListener('title', function(evt) { titleB.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup listeners: %v", err)
	}

	id, err := mgr.Register(sess, parent.SessionTarget{Name: "multi-test", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	ch <- "\x1b]2;XTerm Title\x07"
	close(ch)

	waitForEvents(t, runtime, "titleA", 1)
	waitForEvents(t, runtime, "titleB", 1)

	sidJSON := sessionIDJSON(uint64(id))
	_, err = runJS(t, runtime, fmt.Sprintf(`
		var wantSessionId = %s;
		var wantData = "XTerm Title";
		if (!titleA[0]) { throw new Error("titleA empty"); }
		if (!titleB[0]) { throw new Error("titleB empty"); }
		if (titleA[0].sessionId !== wantSessionId) { throw new Error("titleA sessionId mismatch"); }
		if (titleA[0].data !== wantData) { throw new Error("titleA data mismatch"); }
		if (titleB[0].sessionId !== wantSessionId) { throw new Error("titleB sessionId mismatch"); }
		if (titleB[0].data !== wantData) { throw new Error("titleB data mismatch"); }
	`, sidJSON))
	if err != nil {
		t.Errorf("multiple listener detail fields: %v", err)
	}

	cancel()
	<-errCh
}

func TestEventBridge_CustomEventDetail(t *testing.T) {
	ctx := t.Context()

	mgr := parent.NewSessionManager()
	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runtime.RunString(`
		var detail = null;
		mux.addEventListener('registered', function(evt) {
			detail = evt.detail;
		});
		mux.addEventListener('registered', function(evt) {
			detail = evt.detail;
		});
		mux.dispatchEvent(new CustomEvent('registered', { detail: { sessionId: 42 } }));
	`)
	if err != nil {
		t.Fatalf("setup and dispatch custom event: %v", err)
	}

	v, err := runtime.RunString(`JSON.stringify(detail)`)
	if err != nil {
		t.Fatalf("read detail: %v", err)
	}
	if got, want := v.String(), `{"sessionId":42}`; got != want {
		t.Errorf("detail = %s, want %s", got, want)
	}
}

func TestEventBridge_OnOffCompatibility(t *testing.T) {
	ctx := t.Context()

	mgr := parent.NewSessionManager()
	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runtime.RunString(`
		var events = [];
		var handle = mux.on('registered', function(evt) { events.push(evt.detail); });
		mux.dispatchEvent(new CustomEvent('registered', { detail: { id: 1 } }));
		mux.off(handle);
		mux.dispatchEvent(new CustomEvent('registered', { detail: { id: 2 } }));
	`)
	if err != nil {
		t.Fatalf("run on/off test: %v", err)
	}

	v, err := runtime.RunString(`events.length`)
	if err != nil {
		t.Fatalf("events.length: %v", err)
	}
	if got, want := int(v.ToInteger()), 1; got != want {
		t.Errorf("events.length = %d, want %d", got, want)
	}
}

func TestEventBridge_PollEventsIsNoOp(t *testing.T) {
	ctx := t.Context()

	mgr := parent.NewSessionManager()
	runtime := goja.New()
	wrapper := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	v, err := runtime.RunString(`mux.pollEvents()`)
	if err != nil {
		t.Fatalf("pollEvents: %v", err)
	}
	if got, want := int(v.ToInteger()), 0; got != want {
		t.Errorf("pollEvents = %d, want %d", got, want)
	}
}

// TestEventBridge_NonLoopGoroutineDelivery verifies that an event emitted from a
// non-JS goroutine is delivered asynchronously through the event loop.
func TestEventBridge_NonLoopGoroutineDelivery(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := parent.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	wrapper := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", wrapper)

	_, err := runJS(t, runtime, `
		var events = [];
		mux.addEventListener('registered', function(evt) { events.push(evt.detail); });
	`)
	if err != nil {
		t.Fatalf("setup listener: %v", err)
	}

	sio := newRecordingStringIO()
	sess := parent.NewStringIOSession(sio)
	sess.Start()
	_, _ = mgr.Register(sess, parent.SessionTarget{Name: "async-test", Kind: "pty"})

	waitForEvents(t, runtime, "events", 1)

	v, err := runJS(t, runtime, `JSON.stringify(events[0])`)
	if err != nil {
		t.Fatalf("read events[0]: %v", err)
	}
	if !strings.Contains(v.String(), `"sessionId":`) {
		t.Errorf("events[0] missing sessionId: %s", v.String())
	}

	cancel()
	<-errCh
}

// sessionIDJSON returns a JSON-compatible number string for the given uint64.
func sessionIDJSON(u uint64) string {
	return fmt.Sprintf("%d", u)
}
