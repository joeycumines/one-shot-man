package termpane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

const termPaneScriptTimeout = 5 * time.Second

type termPaneTestEnv struct {
	loop    *goeventloop.Loop
	ctx     context.Context
	done    chan struct{}
	mu      sync.Mutex
	started bool
}

var termPaneTestEnvs sync.Map

type termPaneScriptValue struct {
	value any
}

func (v termPaneScriptValue) Export() any {
	return v.value
}

func newTermPaneRuntime(t *testing.T) (*goja.Runtime, *gojaeventloop.Adapter) {
	t.Helper()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}
	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create event loop adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("bind event loop adapter: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	env := &termPaneTestEnv{
		loop: loop,
		ctx:  ctx,
		done: make(chan struct{}),
	}
	termPaneTestEnvs.Store(runtime, env)
	t.Cleanup(func() {
		termPaneTestEnvs.Delete(runtime)
		env.mu.Lock()
		started := env.started
		env.mu.Unlock()

		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), termPaneScriptTimeout)
		defer shutdownCancel()
		_ = loop.Shutdown(shutdownCtx)
		if started {
			select {
			case <-env.done:
			case <-shutdownCtx.Done():
				t.Errorf("event loop did not stop: %v", shutdownCtx.Err())
			}
		}
	})
	return runtime, adapter
}

func termPaneEnvFor(t *testing.T, runtime *goja.Runtime) *termPaneTestEnv {
	t.Helper()
	value, ok := termPaneTestEnvs.Load(runtime)
	if !ok {
		t.Fatal("term pane runtime is not registered")
	}
	return value.(*termPaneTestEnv)
}

func startTermPaneRuntime(t *testing.T, runtime *goja.Runtime) *termPaneTestEnv {
	t.Helper()
	env := termPaneEnvFor(t, runtime)
	env.mu.Lock()
	if !env.started {
		env.started = true
		go func() {
			_ = env.loop.Run(env.ctx)
			close(env.done)
		}()
	}
	env.mu.Unlock()
	return env
}

type termPaneScriptResult struct {
	value termPaneScriptValue
	err   error
}

func runTermPaneScript(t *testing.T, runtime *goja.Runtime, script string) (termPaneScriptValue, error) {
	t.Helper()
	env := startTermPaneRuntime(t, runtime)
	resultCh := make(chan termPaneScriptResult, 1)
	if err := env.loop.Submit(func() {
		value, err := runtime.RunString(script)
		if err != nil {
			resultCh <- termPaneScriptResult{err: err}
			return
		}
		var exported any
		if value != nil {
			exported = value.Export()
		}
		resultCh <- termPaneScriptResult{value: termPaneScriptValue{value: exported}}
	}); err != nil {
		return termPaneScriptValue{}, err
	}

	select {
	case result := <-resultCh:
		return result.value, result.err
	case <-time.After(termPaneScriptTimeout):
		return termPaneScriptValue{}, errors.New("term pane script timed out")
	}
}

func awaitTermPaneScript(t *testing.T, runtime *goja.Runtime, script string) (termPaneScriptValue, error) {
	t.Helper()
	env := startTermPaneRuntime(t, runtime)
	resultCh := make(chan termPaneScriptResult, 1)
	if err := env.loop.Submit(func() {
		var once sync.Once
		send := func(result termPaneScriptResult) {
			once.Do(func() { resultCh <- result })
		}
		_ = runtime.Set("__termPanePromiseDone", func(value goja.Value) {
			var exported any
			if value != nil {
				exported = value.Export()
			}
			send(termPaneScriptResult{value: termPaneScriptValue{value: exported}})
		})
		_ = runtime.Set("__termPanePromiseFailed", func(value goja.Value) {
			message := "term pane promise rejected"
			if value != nil && !goja.IsUndefined(value) {
				message = value.String()
			}
			send(termPaneScriptResult{err: errors.New(message)})
		})
		wrapped := `(async function() {` + script + `})().then(function(value) { __termPanePromiseDone(value); }, function(error) { __termPanePromiseFailed(error); });`
		if _, err := runtime.RunString(wrapped); err != nil {
			send(termPaneScriptResult{err: err})
		}
	}); err != nil {
		return termPaneScriptValue{}, err
	}

	select {
	case result := <-resultCh:
		return result.value, result.err
	case <-time.After(termPaneScriptTimeout):
		return termPaneScriptValue{}, errors.New("term pane promise timed out")
	}
}

type blockingSession struct {
	controllableSession
	writeStarted chan struct{}
	releaseWrite chan struct{}
	startOnce    sync.Once
	releaseOnce  sync.Once
}

func (s *blockingSession) Write(p []byte) (int, error) {
	s.startOnce.Do(func() { close(s.writeStarted) })
	<-s.releaseWrite
	return len(p), nil
}

func (s *blockingSession) release() {
	s.releaseOnce.Do(func() { close(s.releaseWrite) })
}

func TestTermPaneUpdateQueuePreservesOrder(t *testing.T) {
	queue := newTermpaneUpdateQueue()
	previousFirst, finishFirst := queue.enqueue()
	previousSecond, finishSecond := queue.enqueue()
	defer finishSecond()

	select {
	case <-previousSecond:
		t.Fatal("second operation became runnable before the first completed")
	default:
	}
	finishFirst()
	select {
	case <-previousSecond:
	case <-time.After(time.Second):
		t.Fatal("second operation did not become runnable after the first completed")
	}
	select {
	case <-previousFirst:
	default:
		t.Fatal("first operation completion signal was not observable")
	}
}

// TestTermPaneViewStaysLiveForPassivePane exercises the JavaScript view
// binding for a pane nobody drives with BubbleTea messages. The deterministic
// guard for the saturation path itself lives in the core package
// (TestRefreshSnapshot_AfterOutputQueueSaturation), which can observe the
// output queue; here the burst is simply larger than that queue so the pane
// must still render current content when JS asks for a view.
func TestTermPaneViewStaysLiveForPassivePane(t *testing.T) {
	skipSlow(t)

	mgr, mgrCleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer mgrCleanup()

	session := &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 256),
	}
	sessionID, err := mgr.Register(session, termmux.SessionTarget{Name: "burst"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	rt, adapter := newTermPaneRuntime(t)
	rt.Set("_mgr", wrapManager(rt, mgr))
	rt.Set("_sessionID", uint64(sessionID))
	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).String() == "osm:termui/termpane" {
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(t.Context(), adapter)(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})
	if _, err := awaitTermPaneScript(t, rt, `
		const tp = require('osm:termui/termpane');
		globalThis.testPane = tp.termpane({ manager: _mgr, sessionId: _sessionID, bounds: {x: 0, y: 0, width: 80, height: 24} });
		return 'ready';
	`); err != nil {
		t.Fatalf("pane setup failed: %v", err)
	}

	// The pane is already subscribed before this burst, and the burst is far
	// larger than the 64-slot output queue, so a passive pane has dropped
	// most of it by the time JS renders.
	go func() {
		for i := range 200 {
			session.readerCh <- []byte(fmt.Sprintf("line%03d\\n", i))
		}
		session.readerCh <- []byte("TAILMARKER\\n")
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		capture, captureErr := mgr.CaptureScreen(sessionID, termmux.CaptureOptions{Kind: termmux.CapturePlain})
		if captureErr == nil && capture != nil && strings.Contains(capture.Text, "TAILMARKER") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("manager snapshot did not reach marker: %v", captureErr)
		}
		time.Sleep(time.Millisecond)
	}

	value, err := awaitTermPaneScript(t, rt, `
		return testPane.view().content.indexOf('TAILMARKER') >= 0 ? 'ok' : 'missing';
	`)
	if err != nil {
		t.Fatalf("view script failed: %v", err)
	}
	if value.Export() != "ok" {
		t.Fatalf("passive pane view served stale content: %v", value.Export())
	}
}

func TestTermpane_Update_DoesNotBlockEventLoop(t *testing.T) {
	skipSlow(t)

	mgr, mgrCleanup := startManager(t, termmux.WithTermSize(24, 80))
	defer mgrCleanup()

	session := &blockingSession{
		doneCh:       make(chan struct{}),
		readerCh:     make(chan []byte, 16),
		writeStarted: make(chan struct{}),
		releaseWrite: make(chan struct{}),
	}
	defer session.release()
	sessionID, err := mgr.Register(session, termmux.SessionTarget{Name: "test"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}

	rt, adapter := newTermPaneRuntime(t)
	mgrObj := wrapManager(rt, mgr)
	rt.Set("_mgr", mgrObj)
	rt.Set("_sessionID", uint64(sessionID))
	timerFired := make(chan struct{})
	var timerOnce sync.Once
	rt.Set("_timerFired", func() {
		timerOnce.Do(func() { close(timerFired) })
	})
	defer timerOnce.Do(func() { close(timerFired) })

	go func() {
		select {
		case <-session.writeStarted:
		case <-time.After(time.Second):
			return
		}
		select {
		case <-timerFired:
			session.release()
		case <-time.After(time.Second):
		}
	}()

	rt.Set("require", func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).String() == "osm:termui/termpane" {
			mod := rt.NewObject()
			_ = mod.Set("exports", rt.NewObject())
			Require(t.Context(), adapter)(rt, mod)
			return mod.Get("exports")
		}
		return goja.Undefined()
	})

	script := `
		const tp = require('osm:termui/termpane');
		const pane = tp.termpane({
			manager: _mgr,
			sessionId: _sessionID,
			bounds: {x: 0, y: 0, width: 80, height: 24}
		});
		let timerFired = false;
		const updatePromise = pane.update({type: 'Key', key: 'a', text: 'a'});
		setTimeout(function() {
			timerFired = true;
			_timerFired();
		}, 10);
		const res = await updatePromise;
		if (!timerFired) throw new Error('timer did not run during update');
		if (!Array.isArray(res) || res[0] !== pane) throw new Error('update did not resolve to pane result');
		return 'ok';
	`
	val, err := awaitTermPaneScript(t, rt, script)
	if err != nil {
		t.Fatalf("script error: %v", err)
	}
	if val.Export() != "ok" {
		t.Errorf("unexpected result: %v", val.Export())
	}
}
