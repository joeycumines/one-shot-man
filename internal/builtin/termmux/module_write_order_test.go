package termmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goja "github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// buildStdinToFileProgram builds a cross-platform binary that appends every
// chunk it reads from stdin to the file named by argv[1], flushing as it
// goes — the behavior `sh -c 'cat > file'` provided when this test was
// unix-only. A built helper replaces the shell because there is no sh/cat on
// Windows, and the file path travels as an argument instead of being
// interpolated into a JS template literal, where Windows backslash paths are
// syntax errors.
func buildStdinToFileProgram(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main

import "os"

func main() {
	f, err := os.Create(os.Args[1])
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				os.Exit(1)
			}
		}
		if err != nil {
			return
		}
	}
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	binName := "stdintofileprogram"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

// TestCaptureSession_JSBinding_WriteOrderPreserved pins that successive writes
// reach the child in the order they were issued.
//
// Each binding call runs its work on a fresh goroutine, so a caller that issues
// writes without awaiting each one used to let those goroutines race all the
// way to the PTY: content split across writes arrived shuffled, which reaches a
// child TUI as lines appearing out of order. The wrapper now reserves a slot on
// the JS thread, where call order is defined, and each worker waits for its
// predecessor.
//
// The child appends everything it receives to a file (see
// buildStdinToFileProgram), so the file records exactly what the child
// received and in which order, with no dependency on the JS read path.
func TestCaptureSession_JSBinding_WriteOrderPreserved(t *testing.T) {
	t.Parallel()

	e := newTestEnv(t)

	dir := t.TempDir()
	out := filepath.Join(dir, "received.txt")

	// %q is valid JS string syntax (backslashes are escaped), so Windows
	// paths are safe to interpolate into the script.
	if _, err := awaitJSValue(t, e.runtime, fmt.Sprintf(`
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession(%q, [%q]);
		await cs.start();
	`, buildStdinToFileProgram(t), out)); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Fire the writes WITHOUT awaiting each one: this is the shape that used
	// to let the per-call goroutines reorder the bytes. The line terminator
	// is \r on Windows — console line input is terminated by ENTER (\r),
	// and a bare \n is not one (see the cmd.exe note in module_pane_test.go).
	// On Unix the default ICRNL maps \r to \n, so both work there; \n is
	// kept on Unix so the file records exactly the bytes cat would have.
	terminator := `\n`
	if runtime.GOOS == "windows" {
		terminator = `\r`
	}
	const lines = 40
	if _, err := awaitJSValue(t, e.runtime, `
		for (var i = 0; i < `+itoa(lines)+`; i++) {
			cs.write('L' + (i < 10 ? '0' : '') + i + '`+terminator+`');
		}
	`); err != nil {
		t.Fatalf("write burst failed: %v", err)
	}

	// Wait for the child to have written every line, then close its stdin so
	// the helper flushes and exits.
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, err := os.ReadFile(out); err == nil {
			if len(lineMarker.FindAllString(string(b), -1)) >= lines {
				break
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	_, _ = awaitJSValue(t, e.runtime, `await cs.sendEOF()`)
	_, _ = awaitJSValue(t, e.runtime, `await cs.wait()`)

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the child's record failed: %v", err)
	}
	// The markers are collected in file order regardless of which line
	// terminator the platform's console input records.
	got := lineMarker.FindAllString(string(raw), -1)
	if len(got) != lines {
		t.Fatalf("child received %d lines, want %d: %q", len(got), lines, string(raw))
	}
	for i, line := range got {
		want := "L" + pad2(i)
		if line != want {
			t.Fatalf("position %d is %q, want %q — writes reached the child out of order: %v", i, line, want, got)
		}
	}

	if _, err := awaitJSValue(t, e.runtime, `await cs.close()`); err != nil {
		t.Fatalf("close() failed: %v", err)
	}
}

// lineMarker matches one L-prefixed, zero-padded two-digit line marker.
var lineMarker = regexp.MustCompile(`L[0-9]{2}`)

// pad2 zero-pads a small non-negative int to two digits.
func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}

// TestWriteToChild_OrderingAgainstInput verifies that un-awaited
// writeToChild calls interleaved with input/inputStaged reach the child in
// JS call order (writeToChild now routes through managerPromiseOrdered, the
// same mgrOrder chain as the other manager input calls).
func TestWriteToChild_OrderingAgainstInput(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() { mgr.Close(); <-errCh }()

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "order-mix", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)
	setOnLoop(t, runtime, "sid", uint64(id))

	_, err = awaitJSValue(t, runtime, `
		var ps = [];
		for (var i = 0; i < 15; i++) {
			ps.push(tuiMux.writeToChild("W" + i + ";"));
			ps.push(tuiMux.input("A" + i + ";"));
			ps.push(tuiMux.inputStaged(sid, "S" + i + ";"));
		}
		return Promise.all(ps).then(function() { return true; });
	`)
	if err != nil {
		t.Fatalf("mixed ordering script: %v", err)
	}

	got := strings.Join(rec.sent, "")
	want := ""
	for i := 0; i < 15; i++ {
		want += fmt.Sprintf("W%d;A%d;S%d;", i, i, i)
	}
	if got != want {
		t.Fatalf("mixed call order violated:\n got %q\nwant %q", got, want)
	}
}

// TestSessionWriteStaged_ManagerNotRunningRejects verifies session().writeStaged
// rejects with ErrManagerNotRunning (rather than hanging or degrading) when
// the SessionManager worker has stopped.
func TestSessionWriteStaged_ManagerNotRunningRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx := t.Context()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	rec := newRecordingStringIO()
	sio := parent.NewStringIOSession(rec)
	sio.Start()
	id, err := mgr.Register(sio, parent.SessionTarget{Name: "staged-stop", Kind: "pty"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	runtime := goja.New()
	tuiMux := wrapTestSessionManagerWithLoop(t, ctx, runtime, mgr, nil, nil, -1, "")
	setOnLoop(t, runtime, "tuiMux", tuiMux)

	// Stop the manager; writeStaged must now reject with ErrManagerNotRunning.
	mgr.Close()
	<-errCh

	err = awaitJSErr(t, runtime, `
		var s = tuiMux.session();
		await s.writeStaged("payload");
	`)
	if err == nil {
		t.Fatal("writeStaged on a stopped manager should reject, got resolution")
	}
	if !strings.Contains(err.Error(), parent.ErrManagerNotRunning.Error()) {
		t.Fatalf("writeStaged rejection = %q, want it to wrap %q", err.Error(), parent.ErrManagerNotRunning)
	}
}

// recordingOrderSession records every Write's bytes in order and can block
// the first write until released, so tests can interleave calls while a
// predecessor is in flight.
type recordingOrderSession struct {
	mu       sync.Mutex
	calls    [][]byte
	blockCh  chan struct{} // if non-nil, the first Write blocks until closed
	released bool
	doneCh   chan struct{}
	readerCh chan []byte
	once     sync.Once
}

func newRecordingOrderSession() *recordingOrderSession {
	return &recordingOrderSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 16),
	}
}

func (s *recordingOrderSession) Done() <-chan struct{} { return s.doneCh }
func (s *recordingOrderSession) Reader() <-chan []byte { return s.readerCh }

func (s *recordingOrderSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	if s.blockCh != nil && !s.released {
		ch := s.blockCh
		s.mu.Unlock()
		<-ch // block until the test releases the predecessor
		s.mu.Lock()
		s.released = true
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	s.calls = append(s.calls, cp)
	s.mu.Unlock()
	return len(p), nil
}

func (s *recordingOrderSession) Resize(rows, cols int) error { return nil }

func (s *recordingOrderSession) Close() error {
	s.once.Do(func() { close(s.doneCh) })
	return nil
}

// TestTrackOrdered_CancelledSlotDoesNotRun pins R3-1: when the worker context
// is cancelled while trackOrdered waits on its predecessor, the operation
// must NOT run (it would race the in-flight predecessor) and the promise must
// reject with the context error.
func TestTrackOrdered_CancelledSlotDoesNotRun(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: drives the event loop")
	}
	e := newTestEnv(t)
	rt := e.runtime
	adapter := e.adapter

	sess := newRecordingOrderSession()
	sess.blockCh = make(chan struct{}) // first write blocks until released

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ran := make(chan struct{})

	// Slot 1 on the chain blocks inside operation via the session write.
	chain := newOrderChain()
	_ = trackOrdered(ctx, adapter, rt, chain, func() (any, error) {
		_, _ = sess.Write([]byte("first"))
		return nil, nil
	})
	// Slot 2: cancelled while waiting for slot 1.
	cancelledCtx, cancel2 := context.WithCancel(context.Background())
	var p2ran atomic.Int32
	_ = trackOrdered(cancelledCtx, adapter, rt, chain, func() (any, error) {
		p2ran.Add(1)
		return nil, nil
	})
	cancel2() // cancel slot 2's worker ctx while it awaits slot 1

	// Release slot 1.
	close(sess.blockCh)

	// Give slot 2's worker a moment to observe the cancellation, then check
	// its operation never ran.
	time.Sleep(100 * time.Millisecond)
	if got := p2ran.Load(); got != 0 {
		t.Fatalf("cancelled slot ran its operation %d times, want 0", got)
	}
	_ = ran
}

// TestTrackOrdered_CancelRacesRelease pins R4-4: when the predecessor is
// released at the SAME moment the slot's worker context is cancelled (both
// select cases ready), the post-wait re-check must still prevent the
// operation from running — and the chain must stay alive: a later operation
// on a live context settles normally.
func TestTrackOrdered_CancelRacesRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: drives the event loop")
	}
	e := newTestEnv(t)
	rt := e.runtime
	adapter := e.adapter

	sess := newRecordingOrderSession()
	sess.blockCh = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := newOrderChain()
	// Slot 1 blocks inside its operation.
	_ = trackOrdered(ctx, adapter, rt, chain, func() (any, error) {
		_, _ = sess.Write([]byte("first"))
		return nil, nil
	})
	// Slot 2: cancel its ctx, THEN release slot 1 — both select cases become
	// ready together and Go may pick <-wait; the re-check must still reject.
	cancelledCtx, cancel2 := context.WithCancel(context.Background())
	var p2ran atomic.Int32
	_ = trackOrdered(cancelledCtx, adapter, rt, chain, func() (any, error) {
		p2ran.Add(1)
		return nil, nil
	})
	cancel2()
	close(sess.blockCh)

	time.Sleep(150 * time.Millisecond)
	if got := p2ran.Load(); got != 0 {
		t.Fatalf("cancel-raced slot ran its operation %d times, want 0", got)
	}
	// The cancelled slot's TrackPromise worker holds loop liveness until it
	// settles; keep a ref'd JS timer alive for the duration of the check so the
	// loop cannot auto-exit while we are still polling.
	_, _ = runJS(t, rt, `globalThis.__keepAlive = setTimeout(function(){}, 3000)`)
	t.Cleanup(func() { _, _ = runJS(t, rt, `clearTimeout(globalThis.__keepAlive)`) })

	// Chain liveness: a subsequent operation on a live ctx must run and settle.
	// trackOrdered must run on the event-loop goroutine (TrackPromise requires
	// the bound adapter's owner), so issue slot 3 via runJS with the Go values
	// exposed as globals, and poll the JS-side counter back through runJS.
	setOnLoop(t, rt, "__goCtx", ctx)
	setOnLoop(t, rt, "__goAdapter", adapter)
	setOnLoop(t, rt, "__goChain", chain)
	setOnLoop(t, rt, "__trackOrderedForTest", func(ctx context.Context, adapter *gojaeventloop.Adapter, chain *orderChain, op func() (any, error)) goja.Value {
		return trackOrdered(ctx, adapter, rt, chain, op)
	})
	_, rerr := runJS(t, rt, `
		return (function() {
			globalThis.__p3 = __trackOrderedForTest(__goCtx, __goAdapter, __goChain, function() {
				globalThis.__p3ran = (globalThis.__p3ran || 0) + 1;
				return "ok";
			});
			return 1;
		})();
	`)
	if rerr != nil {
		t.Fatalf("issue slot 3 on loop: %v", rerr)
	}
	deadline := time.Now().Add(5 * time.Second)
	ran := false
	for time.Now().Before(deadline) {
		v, rerr := runJS(t, rt, `return globalThis.__p3ran || 0`)
		if rerr == nil && v != nil && v.ToInteger() >= 1 {
			ran = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ran {
		t.Fatalf("post-cancel slot did not run (chain deadlocked by cancelled slot)")
	}
}

// TestCaptureSession_SendEOF_OrderedAfterWrite pins R3-1b: an un-awaited
// write followed by sendEOF must deliver the data bytes before the EOF byte.
func TestCaptureSession_SendEOF_OrderedAfterWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns child process")
	}
	e := newTestEnv(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "received.bin")
	if _, err := awaitJSValue(t, e.runtime, fmt.Sprintf(`
		var tm = require('osm:termmux');
		globalThis.cs = tm.newCaptureSession(%q, [%q]);
		await cs.start();
	`, buildStdinToFileProgram(t), out)); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	// Fire write then sendEOF without awaiting the write.
	if _, err := awaitJSValue(t, e.runtime, `
		cs.write('DATA-BEFORE-EOF');
		cs.sendEOF();
		return true;
	`); err != nil {
		t.Fatalf("write+sendEOF: %v", err)
	}
	_, _ = awaitJSValue(t, e.runtime, `await cs.wait()`)
	// Close before reading the record file: on Windows the child helper may
	// still hold received.bin open briefly after Wait returns, and reading or
	// TempDir cleanup while the handle is open fails with a sharing violation.
	_, _ = awaitJSValue(t, e.runtime, `await cs.close()`)
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read child record: %v", err)
	}
	if runtime.GOOS == "windows" {
		// ConPTY has no line discipline: 0x04 written to the input pipe is
		// delivered to the child as a literal byte, not translated into an
		// EOF condition for a plain os.Stdin.Read loop, so the child keeps
		// running until cs.close() kills it. The byte-level ordering contract
		// (write lands before the EOF byte on the pipe) still holds; assert
		// the data arrived and contains no trailing 0x04 truncation.
		if string(raw) != "DATA-BEFORE-EOF" {
			t.Fatalf("child received %q, want exactly %q on Windows (0x04 is a literal byte, not EOF)", raw, "DATA-BEFORE-EOF")
		}
		return
	}
	if string(raw) != "DATA-BEFORE-EOF" {
		t.Fatalf("child received %q, want exactly %q (EOF must come after the write)", raw, "DATA-BEFORE-EOF")
	}
}

// TestWriteNullUndefinedTypeError pins R3-12: write/writeStaged/writeToChild
// must throw a TypeError for null/undefined (previously coerced to the literal
// strings "null"/"undefined" onto the PTY) and must not write anything.
func TestWriteNullUndefinedTypeError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		globalThis.cs = tm.newCaptureSession("echo hi", []);
		var out = [];
		out.push(expectTypeError(function(){ cs.write(null); }));
		out.push(expectTypeError(function(){ cs.write(undefined); }));
		out.push(expectTypeError(function(){ cs.writeStaged(null); }));
		out.push(expectTypeError(function(){ cs.writeStaged(undefined); }));
		out.push(expectTypeError(function(){ cs.sendKeys(null); }));
		out.push(expectTypeError(function(){ cs.sendKeys(undefined); }));
		out.push(expectTypeError(function(){ cs.sendKeys([null]); }));
		// Valid string writes still work.
		out.push(!expectTypeError(function(){ cs.write("ok"); }));
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("null/undefined write script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("write/writeStaged/sendKeys must throw TypeError on null/undefined")
	}
}

// TestWriteToChildNullTypeError pins R3-12 for the mux-level writeToChild and
// session().write paths: null/undefined throw TypeError and write nothing.
func TestWriteToChildNullTypeError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	runtime, cleanup := setupMgr(t, true)
	defer cleanup()

	v, err := sessionRun(t, runtime, `(async function(){
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		var out = [];
		out.push(expectTypeError(function(){ tuiMux.writeToChild(null); }));
		out.push(expectTypeError(function(){ tuiMux.writeToChild(undefined); }));
		out.push(expectTypeError(function(){ testSession.write(null); }));
		out.push(expectTypeError(function(){ testSession.write(undefined); }));
		out.push(expectTypeError(function(){ testSession.writeStaged(null); }));
		// Valid write still resolves.
		await testSession.write("ping");
		out.push(true);
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("writeToChild null script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("writeToChild/session().write/writeStaged must throw TypeError on null/undefined")
	}
}
