package command

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// ── recordingStringIO ────────────────────────────────────────────────────────
// A test double implementing [termmux.StringIO] that records all Send() calls.
// Receive() blocks until Close() so the session stays alive for writes.
type recordingStringIO struct {
	sent   []string
	closed chan struct{}
}

func newRecordingStringIO() *recordingStringIO {
	return &recordingStringIO{closed: make(chan struct{})}
}

func (r *recordingStringIO) Send(input string) error {
	r.sent = append(r.sent, input)
	return nil
}

func (r *recordingStringIO) Receive() (string, error) {
	<-r.closed
	return "", fmt.Errorf("closed")
}

func (r *recordingStringIO) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

// ── newPrSplitEvalWithMgr ────────────────────────────────────────────────────
// A variant of [newPrSplitEvalFromFlags] that also returns the SessionManager
// created by setupEngineGlobals. The SessionManager is needed so tests can
// register mock sessions from the Go side.
func newPrSplitEvalWithMgr(t testing.TB) (*termmux.SessionManager, func(string) (any, error)) {
	t.Helper()

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse([]string{"--test", "--store=memory", "--session=" + t.Name()}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	cmd.applyConfigDefaults()
	if err := cmd.validateFlags(); err != nil {
		t.Fatalf("validate flags: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout safeBuffer
	var stderr bytes.Buffer
	engine, cleanup, err := cmd.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("prepare engine: %v", err)
	}
	t.Cleanup(cleanup)

	_, mgr, err := cmd.setupEngineGlobals(ctx, engine, &stdout)
	if err != nil {
		t.Fatalf("setup engine globals: %v", err)
	}

	shim := engine.LoadScriptFromString("pr-split/compat-shim", chunkCompatShim)
	if err := engine.ExecuteScript(shim); err != nil {
		t.Fatalf("compat shim failed: %v", err)
	}

	evalJS := func(js string) (any, error) {
		done := make(chan struct{})
		var result any
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			if strings.Contains(js, "await ") {
				_ = vm.Set("__evalResult", func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set("__evalError", func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := "(async function() {\n\ttry {\n\t\tvar __res = " + js + ";\n\t\tif (__res && typeof __res.then === 'function') { __res = await __res; }\n\t\t__evalResult(__res);\n\t} catch(e) {\n\t\t__evalError(e.message || String(e));\n\t}\n})();"
				if _, runErr := vm.RunString(wrapped); runErr != nil {
					resultErr = runErr
					close(done)
				}
				return
			}

			val, err := vm.RunString(js)
			if err != nil {
				resultErr = err
				close(done)
				return
			}
			result = val.Export()
			close(done)
		})
		if submitErr != nil {
			return nil, submitErr
		}

		select {
		case <-done:
			return result, resultErr
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("eval timeout after 30s")
		}
	}

	return mgr, evalJS
}

// ── TestInlineKeystrokeReachesPTY ────────────────────────────────────────────
// End-to-end test proving the JS → session().write() → SessionManager.Input()
// → InteractiveSession.Write() path works when running through the full
// pr-split command engine setup (PrepareEngine + setupEngineGlobals + chunked
// scripts loaded).
//
// This directly validates the fix for GAP-C01/C02 from the pr-split autopsy:
// the session() wrapper was missing write/resize, causing all inline Claude
// pane keystrokes to silently fail.
func TestInlineKeystrokeReachesPTY(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	mgr, evalJS := newPrSplitEvalWithMgr(t)

	// Register a recording StringIO session as the active Claude session.
	rec := newRecordingStringIO()
	sio := termmux.NewStringIOSession(rec)
	sio.Start()
	t.Cleanup(func() { _ = rec.Close() })

	id, err := mgr.Register(sio, termmux.SessionTarget{
		Name: "claude",
		Kind: termmux.SessionKindPTY,
	})
	if err != nil {
		t.Fatalf("register mock session: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("activate mock session: %v", err)
	}

	// ── Verify session().write() exists and delivers bytes ───────────
	v, err := evalJS(`typeof tuiMux.session().write`)
	if err != nil {
		t.Fatalf("typeof session().write: %v", err)
	}
	if v != "function" {
		t.Fatalf("session().write type = %v, want function", v)
	}

	_, err = evalJS(`tuiMux.session().write('hello')`)
	if err != nil {
		t.Fatalf("session().write('hello'): %v", err)
	}

	if len(rec.sent) != 1 || rec.sent[0] != "hello" {
		t.Errorf("write path: expected sent=['hello'], got %v", rec.sent)
	}

	// ── Verify session().resize() exists and succeeds ────────────────
	v, err = evalJS(`typeof tuiMux.session().resize`)
	if err != nil {
		t.Fatalf("typeof session().resize: %v", err)
	}
	if v != "function" {
		t.Fatalf("session().resize type = %v, want function", v)
	}

	_, err = evalJS(`tuiMux.session().resize(40, 120)`)
	if err != nil {
		t.Fatalf("session().resize(40, 120): %v", err)
	}

	// ── Verify multi-byte sequence round-trip ────────────────────────
	// Send an ANSI escape sequence that simulates a real arrow key press.
	_, err = evalJS(`tuiMux.session().write('\x1b[A')`)
	if err != nil {
		t.Fatalf("session().write(escape): %v", err)
	}
	if len(rec.sent) != 2 || rec.sent[1] != "\x1b[A" {
		t.Errorf("escape round-trip: expected sent[1]='\\x1b[A', got %v", rec.sent)
	}

	// ── Verify all 8 session() methods are present ───────────────────
	v, err = evalJS(`
		var s = tuiMux.session();
		var methods = ['isRunning', 'isDone', 'output', 'screen', 'target', 'setTarget', 'write', 'resize'];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof s[methods[i]] !== 'function') {
				missing.push(methods[i] + ':' + typeof s[methods[i]]);
			}
		}
		JSON.stringify(missing);
	`)
	if err != nil {
		t.Fatalf("method presence check: %v", err)
	}
	got := v.(string)
	if got != "[]" {
		t.Errorf("missing methods on session(): %s", got)
	}
}
