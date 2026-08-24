//go:build unix

package termmux

import (
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

func runOnEnvLoop(t *testing.T, env *testEnv, script string) (goja.Value, error) {
	t.Helper()
	type result struct {
		v   goja.Value
		err error
	}
	ch := make(chan result, 1)
	if err := env.loop.Submit(func() {
		v, err := env.runtime.RunString(script)
		ch <- result{v, err}
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(5 * time.Second):
		t.Fatalf("runOnEnvLoop timeout")
		return nil, nil
	}
}

// ---------------------------------------------------------------------------
// T004/T056: CaptureSession JS binding completeness tests
//
// Validates that all 17 methods exposed by WrapCaptureSession are callable
// from JS and return the expected types. Uses real PTY (requires unix).
//
// The four methods called by runVerifyBranch/pollVerifySession:
//   isDone()    → boolean
//   exitCode()  → number
//   close()     → void
//   interrupt() → void
//
// Additional methods:
//   start()     → void
//   resize(r,c) → void
//   wait()      → {code, error?}
//   write(data) → void
//   sendEOF()   → void
//   pid()       → number
//   kill()      → void
//   pause()     → void
//   resume()    → void
//   isPaused()  → boolean
//   passthrough(cfg?) → {reason, error?}
//
// Task 49: output() and screen() removed (VTerm elimination).
// Task 56: isRunning(), target(), setTarget() removed from CaptureSession;
//          all JS call sites use SessionManager wrappers instead.
// ---------------------------------------------------------------------------

func TestCaptureSession_JSBinding_AllMethods(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)
	go e.loop.Run(e.ctx)
	t.Cleanup(func() { e.stop() })
	rt := e.runtime
	_ = rt

	// Create a CaptureSession that runs `echo "hello T004"` and exits.
	v, err := runOnEnvLoop(t, e, `
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('echo', ['hello T004']);

		// Verify all 17 methods exist and are functions.
		var methods = [
			'start', 'interrupt', 'kill',
			'pause', 'resume', 'isPaused',
			'resize', 'wait', 'write', 'sendEOF', 'close', 'pid', 'exitCode', 'isDone',
			'passthrough',
			'reader', 'readAvailable'
		];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof cs[methods[i]] !== 'function') {
				missing.push(methods[i] + ':' + typeof cs[methods[i]]);
			}
		}
		JSON.stringify(missing);
	`)
	if err != nil {
		t.Fatalf("JS setup failed: %v", err)
	}
	missingStr := v.String()
	if missingStr != "[]" {
		t.Fatalf("missing methods on CaptureSession: %s", missingStr)
	}

	// Start the session and wait for completion.
	_, err = runOnEnvLoop(t, e, `cs.start()`)
	if err != nil {
		t.Fatalf("cs.start() failed: %v", err)
	}

	// pid() should return a positive integer.
	v, err = runOnEnvLoop(t, e, `cs.pid()`)
	if err != nil {
		t.Fatalf("pid() failed: %v", err)
	}
	pid := v.ToInteger()
	if pid <= 0 {
		t.Errorf("expected positive pid, got %d", pid)
	}

	// wait() now returns Promise<{code,error}> — await it.
	_, err = runOnEnvLoop(t, e, `globalThis.__waitDone = false; globalThis.__waitResult = null; cs.wait().then(r => { globalThis.__waitResult = JSON.stringify(r); globalThis.__waitDone = true; }).catch(e => { globalThis.__waitResult = JSON.stringify({error: e.message}); globalThis.__waitDone = true; })`)
	if err != nil {
		t.Fatalf("cs.wait() failed: %v", err)
	}
	waitResult := ""
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		v, _ := runOnEnvLoop(t, e, `globalThis.__waitDone`)
		if v.ToBoolean() {
			v2, _ := runOnEnvLoop(t, e, `globalThis.__waitResult`)
			waitResult = v2.String()
			break
		}
	}
	if waitResult == "" {
		t.Fatalf("wait() promise did not settle")
	}
	if !strings.Contains(waitResult, `"code"`) {
		t.Errorf("wait() result should contain 'code', got %q", waitResult)
	}
	if !strings.Contains(waitResult, `"code":0`) {
		t.Errorf("echo should exit with code 0, got %q", waitResult)
	}

	// After wait(), isDone() must be true.
	v, err = runOnEnvLoop(t, e, `cs.isDone()`)
	if err != nil {
		t.Fatalf("isDone() failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isDone() should be true after wait()")
	}

	// exitCode() should return 0.
	v, err = runOnEnvLoop(t, e, `cs.exitCode()`)
	if err != nil {
		t.Fatalf("exitCode() failed: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Errorf("exitCode() = %d, want 0", v.ToInteger())
	}

	// output() and screen() were removed in Task 49 — screen reads go
	// through SessionManager snapshots. Verify they are absent.
	v, err = runOnEnvLoop(t, e, `typeof cs.output`)
	if err != nil {
		t.Fatalf("typeof cs.output check failed: %v", err)
	}
	if v.String() != "undefined" {
		t.Errorf("output should be undefined after VTerm removal, got %q", v.String())
	}
	v, err = runOnEnvLoop(t, e, `typeof cs.screen`)
	if err != nil {
		t.Fatalf("typeof cs.screen check failed: %v", err)
	}
	if v.String() != "undefined" {
		t.Errorf("screen should be undefined after VTerm removal, got %q", v.String())
	}

	// Task 56: isRunning(), target(), setTarget() removed — all call sites
	// use SessionManager wrappers. Verify they are absent.
	for _, removed := range []string{"isRunning", "target", "setTarget"} {
		v, err = runOnEnvLoop(t, e, `typeof cs.` + removed)
		if err != nil {
			t.Fatalf("typeof cs.%s check failed: %v", removed, err)
		}
		if v.String() != "undefined" {
			t.Errorf("%s should be undefined after Task 56 removal, got %q", removed, v.String())
		}
	}

	// close() should not error on completed session (idempotent).
	_, err = runOnEnvLoop(t, e, `cs.close()`)
	if err != nil {
		t.Fatalf("close() failed: %v", err)
	}

	// Double close should also not error.
	_, err = runOnEnvLoop(t, e, `cs.close()`)
	if err != nil {
		t.Fatalf("double close() failed: %v", err)
	}
}

func TestCaptureSession_JSBinding_Interrupt(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)
	go e.loop.Run(e.ctx)
	t.Cleanup(func() { e.stop() })
	rt := e.runtime
	_ = rt

	// Start a long-running sleep process and interrupt it.
	_, err := runOnEnvLoop(t, e, `
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('sleep', ['60']);
		cs.start();
	`)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Give process a moment to start.
	time.Sleep(50 * time.Millisecond)

	// interrupt() should not error.
	_, err = runOnEnvLoop(t, e, `cs.interrupt()`)
	if err != nil {
		t.Fatalf("interrupt() failed: %v", err)
	}

	// Wait should complete (signal causes exit) — await Promise.
	_, err = runOnEnvLoop(t, e, `globalThis.__waitDone2 = false; cs.wait().then(() => { globalThis.__waitDone2 = true; }).catch(() => { globalThis.__waitDone2 = true; })`)
	if err != nil {
		t.Fatalf("wait() after interrupt failed: %v", err)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		v2, _ := runOnEnvLoop(t, e, `globalThis.__waitDone2`)
		if v2.ToBoolean() {
			break
		}
		if i == 19 {
			t.Fatalf("wait() promise did not settle after interrupt")
		}
	}

	// isDone should be true.
	v, err := runOnEnvLoop(t, e, `cs.isDone()`)
	if err != nil {
		t.Fatalf("isDone() after interrupt failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isDone() should be true after interrupt + wait")
	}

	_, err = runOnEnvLoop(t, e, `cs.close()`)
	if err != nil {
		t.Fatalf("close() after interrupt failed: %v", err)
	}
}

func TestCaptureSession_JSBinding_Kill(t *testing.T) {
	t.Parallel()

	rt, _ := testRequire(t)

	// Start a long-running process and kill it.
	_, err := rt.RunString(`
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('sleep', ['60']);
		cs.start();
	`)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// kill() should not error.
	_, err = rt.RunString(`cs.kill()`)
	if err != nil {
		t.Fatalf("kill() failed: %v", err)
	}

	// Wait should complete (SIGKILL causes immediate exit).
	_, err = rt.RunString(`cs.wait()`)
	if err != nil {
		t.Fatalf("wait() after kill failed: %v", err)
	}

	// exitCode() after kill — should be non-zero.
	v, err := rt.RunString(`cs.exitCode()`)
	if err != nil {
		t.Fatalf("exitCode() after kill failed: %v", err)
	}
	if v.ToInteger() == 0 {
		t.Error("exitCode() should be non-zero after kill")
	}

	_, err = rt.RunString(`cs.close()`)
	if err != nil {
		t.Fatalf("close() after kill failed: %v", err)
	}
}

func TestCaptureSession_JSBinding_Resize(t *testing.T) {
	t.Parallel()

	rt, _ := testRequire(t)

	_, err := rt.RunString(`
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('sleep', ['60']);
		cs.start();
	`)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// resize() should not error.
	_, err = rt.RunString(`cs.resize(40, 100)`)
	if err != nil {
		t.Fatalf("resize() failed: %v", err)
	}

	// Clean up.
	_, err = rt.RunString(`cs.kill(); cs.wait(); cs.close()`)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}

func TestCaptureSession_JSBinding_WriteAndSendEOF(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)
	go e.loop.Run(e.ctx)
	t.Cleanup(func() { e.stop() })
	rt := e.runtime
	_ = rt

	// Use cat which reads stdin and echoes to stdout.
	_, err := runOnEnvLoop(t, e, `
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('cat', []);
		cs.start();
	`)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// write() should not error.
	_, err = runOnEnvLoop(t, e, `cs.write('hello from JS\n')`)
	if err != nil {
		t.Fatalf("write() failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// sendEOF() should close stdin, causing cat to exit.
	_, err = runOnEnvLoop(t, e, `cs.sendEOF()`)
	if err != nil {
		t.Fatalf("sendEOF() failed: %v", err)
	}

	// Wait should complete (cat exits on EOF) — await Promise.
	_, err = runOnEnvLoop(t, e, `globalThis.__waitDone3 = false; globalThis.__waitResult3 = null; cs.wait().then(r => { globalThis.__waitResult3 = JSON.stringify(r); globalThis.__waitDone3 = true; }).catch(e => { globalThis.__waitResult3 = JSON.stringify({error: e.message}); globalThis.__waitDone3 = true; })`)
	if err != nil {
		t.Fatalf("wait() after sendEOF failed: %v", err)
	}
	waitResult3 := ""
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		v2, _ := runOnEnvLoop(t, e, `globalThis.__waitDone3`)
		if v2.ToBoolean() {
			v3, _ := runOnEnvLoop(t, e, `globalThis.__waitResult3`)
			waitResult3 = v3.String()
			break
		}
	}
	if waitResult3 == "" {
		t.Fatalf("wait() promise did not settle after sendEOF")
	}
	if !strings.Contains(waitResult3, `"code":0`) {
		t.Errorf("cat should exit 0, got %s", waitResult3)
	}

	// isDone should be true after wait.
	v, err := runOnEnvLoop(t, e, `cs.isDone()`)
	if err != nil {
		t.Fatalf("isDone() after wait failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isDone() should be true after wait")
	}

	_, err = runOnEnvLoop(t, e, `cs.close()`)
	if err != nil {
		t.Fatalf("close() failed: %v", err)
	}
}

func TestCaptureSession_JSBinding_isDoneBeforeStart(t *testing.T) {
	t.Parallel()

	rt, _ := testRequire(t)

	// isDone() before start() should be false.
	v, err := rt.RunString(`
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('echo', ['test']);
		cs.isDone();
	`)
	if err != nil {
		t.Fatalf("isDone() before start failed: %v", err)
	}
	if v.ToBoolean() {
		t.Error("isDone() should be false before start()")
	}
}

// T059: Test pause/resume/isPaused JS bindings on a real CaptureSession.
func TestCaptureSession_JSBinding_PauseResume(t *testing.T) {
	t.Parallel()

	rt, ctx := testRequire(t)

	val, err := rt.RunString(`
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('sh', ['-c', 'i=0; while true; do echo "line$i"; i=$((i+1)); sleep 0.1; done']);
		cs.start();
		cs;
	`)
	if err != nil {
		t.Fatalf("start CaptureSession: %v", err)
	}
	_ = ctx

	// Let it produce output.
	time.Sleep(500 * time.Millisecond)

	// isPaused() should be false initially.
	v, err := rt.RunString(`cs.isPaused()`)
	if err != nil {
		t.Fatalf("isPaused: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("expected isPaused()=false initially")
	}

	// pause() should succeed.
	_, err = rt.RunString(`cs.pause()`)
	if err != nil {
		t.Fatalf("pause: %v", err)
	}

	// isPaused() should be true.
	v, err = rt.RunString(`cs.isPaused()`)
	if err != nil {
		t.Fatalf("isPaused after pause: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected isPaused()=true after pause")
	}

	// resume() should succeed.
	_, err = rt.RunString(`cs.resume()`)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	// isPaused() should be false again.
	v, err = rt.RunString(`cs.isPaused()`)
	if err != nil {
		t.Fatalf("isPaused after resume: %v", err)
	}
	if v.ToBoolean() {
		t.Fatal("expected isPaused()=false after resume")
	}

	// Clean up.
	_, _ = rt.RunString(`cs.kill()`)
	time.Sleep(200 * time.Millisecond)
	_, _ = rt.RunString(`cs.close()`)

	_ = val
}

func TestCaptureSession_JSBinding_NewCaptureSessionError(t *testing.T) {
	t.Parallel()

	rt, _ := testRequire(t)

	// Missing command should throw TypeError.
	_, err := rt.RunString(`
		var tm = require('osm:termmux');
		tm.newCaptureSession();
	`)
	if err == nil {
		t.Fatal("expected error for newCaptureSession with no args")
	}
	var jsErr *goja.Exception
	if e, ok := err.(*goja.Exception); ok {
		jsErr = e
	}
	if jsErr == nil || !strings.Contains(jsErr.Error(), "command") {
		t.Errorf("expected TypeError mentioning 'command', got %v", err)
	}

	// Empty string command should also throw.
	_, err = rt.RunString(`
		var tm = require('osm:termmux');
		tm.newCaptureSession('');
	`)
	if err == nil {
		t.Fatal("expected error for empty command string")
	}
}
