//go:build unix

package termmux

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// ---------------------------------------------------------------------------
// T004: CaptureSession JS binding completeness tests
//
// Validates that all 18 methods exposed by WrapCaptureSession are callable
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
//   isRunning() → boolean
//   resize(r,c) → void
//   wait()      → {code, error?}
//   write(data) → void
//   sendEOF()   → void
//   pid()       → number
//   kill()      → void
//   pause()     → void
//   resume()    → void
//   isPaused()  → boolean
//   target()    → object
//   setTarget() → void
//   passthrough(cfg?) → {reason, error?}
//
// Note: output() and screen() were removed in Task 49 (VTerm elimination).
// Screen reads now go through SessionManager snapshots.
// ---------------------------------------------------------------------------

func TestCaptureSession_JSBinding_AllMethods(t *testing.T) {
	t.Parallel()

	rt, _ := testRequire(t)

	// Create a CaptureSession that runs `echo "hello T004"` and exits.
	v, err := rt.RunString(`
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('echo', ['hello T004']);

		// Verify all 20 methods exist and are functions.
		var methods = [
			'start', 'isRunning', 'interrupt', 'kill',
			'pause', 'resume', 'isPaused',
			'resize', 'wait', 'write', 'sendEOF', 'close', 'pid', 'exitCode', 'isDone',
			'target', 'setTarget', 'passthrough',
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
	_, err = rt.RunString(`cs.start()`)
	if err != nil {
		t.Fatalf("cs.start() failed: %v", err)
	}

	// isRunning should be true right after start (before wait).
	// Note: fast commands may exit before we check, so we only verify the type.
	v, err = rt.RunString(`typeof cs.isRunning()`)
	if err != nil {
		t.Fatalf("isRunning() failed: %v", err)
	}
	if v.String() != "boolean" {
		t.Errorf("isRunning() should return boolean, got %q", v.String())
	}

	// pid() should return a positive integer.
	v, err = rt.RunString(`cs.pid()`)
	if err != nil {
		t.Fatalf("pid() failed: %v", err)
	}
	pid := v.ToInteger()
	if pid <= 0 {
		t.Errorf("expected positive pid, got %d", pid)
	}

	// wait() should return an object with {code: number}.
	v, err = rt.RunString(`JSON.stringify(cs.wait())`)
	if err != nil {
		t.Fatalf("cs.wait() failed: %v", err)
	}
	waitResult := v.String()
	if !strings.Contains(waitResult, `"code"`) {
		t.Errorf("wait() result should contain 'code', got %q", waitResult)
	}
	if !strings.Contains(waitResult, `"code":0`) {
		t.Errorf("echo should exit with code 0, got %q", waitResult)
	}

	// After wait(), isDone() must be true.
	v, err = rt.RunString(`cs.isDone()`)
	if err != nil {
		t.Fatalf("isDone() failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isDone() should be true after wait()")
	}

	// exitCode() should return 0.
	v, err = rt.RunString(`cs.exitCode()`)
	if err != nil {
		t.Fatalf("exitCode() failed: %v", err)
	}
	if v.ToInteger() != 0 {
		t.Errorf("exitCode() = %d, want 0", v.ToInteger())
	}

	// output() and screen() were removed in Task 49 — screen reads go
	// through SessionManager snapshots. Verify they are absent.
	v, err = rt.RunString(`typeof cs.output`)
	if err != nil {
		t.Fatalf("typeof cs.output check failed: %v", err)
	}
	if v.String() != "undefined" {
		t.Errorf("output should be undefined after VTerm removal, got %q", v.String())
	}
	v, err = rt.RunString(`typeof cs.screen`)
	if err != nil {
		t.Fatalf("typeof cs.screen check failed: %v", err)
	}
	if v.String() != "undefined" {
		t.Errorf("screen should be undefined after VTerm removal, got %q", v.String())
	}

	// target() should return metadata with at least kind information.
	v, err = rt.RunString(`JSON.stringify(cs.target())`)
	if err != nil {
		t.Fatalf("target() failed: %v", err)
	}
	if !strings.Contains(v.String(), `"kind":"capture"`) {
		t.Errorf("target() should default to capture kind, got %q", v.String())
	}

	// setTarget() should accept a metadata object and round-trip it via target().
	_, err = rt.RunString(`cs.setTarget({ id: 'shell-1', name: 'shell', kind: 'pty' })`)
	if err != nil {
		t.Fatalf("setTarget() failed: %v", err)
	}
	v, err = rt.RunString(`JSON.stringify(cs.target())`)
	if err != nil {
		t.Fatalf("target() after setTarget() failed: %v", err)
	}
	if !strings.Contains(v.String(), `"shell"`) || !strings.Contains(v.String(), `"pty"`) {
		t.Errorf("target() after setTarget() = %q, want shell/pty metadata", v.String())
	}

	// close() should not error on completed session (idempotent).
	_, err = rt.RunString(`cs.close()`)
	if err != nil {
		t.Fatalf("close() failed: %v", err)
	}

	// Double close should also not error.
	_, err = rt.RunString(`cs.close()`)
	if err != nil {
		t.Fatalf("double close() failed: %v", err)
	}
}

func TestCaptureSession_JSBinding_Interrupt(t *testing.T) {
	t.Parallel()

	rt, _ := testRequire(t)

	// Start a long-running sleep process and interrupt it.
	_, err := rt.RunString(`
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
	_, err = rt.RunString(`cs.interrupt()`)
	if err != nil {
		t.Fatalf("interrupt() failed: %v", err)
	}

	// Wait should complete (signal causes exit).
	_, err = rt.RunString(`cs.wait()`)
	if err != nil {
		t.Fatalf("wait() after interrupt failed: %v", err)
	}

	// isDone should be true.
	v, err := rt.RunString(`cs.isDone()`)
	if err != nil {
		t.Fatalf("isDone() after interrupt failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isDone() should be true after interrupt + wait")
	}

	_, err = rt.RunString(`cs.close()`)
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

	rt, _ := testRequire(t)

	// Use cat which reads stdin and echoes to stdout.
	_, err := rt.RunString(`
		var tm = require('osm:termmux');
		var cs = tm.newCaptureSession('cat', []);
		cs.start();
	`)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// write() should not error.
	_, err = rt.RunString(`cs.write('hello from JS\n')`)
	if err != nil {
		t.Fatalf("write() failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// sendEOF() should close stdin, causing cat to exit.
	_, err = rt.RunString(`cs.sendEOF()`)
	if err != nil {
		t.Fatalf("sendEOF() failed: %v", err)
	}

	// Wait should complete (cat exits on EOF).
	v, err := rt.RunString(`JSON.stringify(cs.wait())`)
	if err != nil {
		t.Fatalf("wait() after sendEOF failed: %v", err)
	}
	if !strings.Contains(v.String(), `"code":0`) {
		t.Errorf("cat should exit 0, got %s", v.String())
	}

	// isDone should be true after wait.
	v, err = rt.RunString(`cs.isDone()`)
	if err != nil {
		t.Fatalf("isDone() after wait failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("isDone() should be true after wait")
	}

	_, err = rt.RunString(`cs.close()`)
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
