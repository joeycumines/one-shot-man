package exec

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// TestPipeQueueReportsCutShortStream checks cut-short terminal contract.
func TestPipeQueueReportsCutShortStream(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
	child.stdout.push(readChunk{data: []byte("parent-done\n")}, nil)
	if !child.stdout.closeTruncated() {
		t.Fatal("closeTruncated on an open queue reported it was already closed")
	}

	data, done, err := child.ReadStdoutContext(context.Background())
	if err != nil || done || data != "parent-done\n" {
		t.Fatalf("first read = (%q, %v, %v), want the queued data", data, done, err)
	}
	data, done, err = child.ReadStdoutContext(context.Background())
	if !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal read = (%q, %v, %v), want errOutputStreamCutShort", data, done, err)
	}
}

// TestPipeQueueCleanCloseStaysClean checks clean close stays clean.
func TestPipeQueueCleanCloseStaysClean(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
	child.stdout.push(readChunk{data: []byte("complete\n")}, nil)
	child.stdout.close()
	if child.stdout.closeTruncated() {
		t.Fatal("closeTruncated re-marked a stream that had already ended")
	}

	if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "complete\n" {
		t.Fatalf("first read = (%q, %v, %v), want the queued data", data, done, err)
	}
	if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || !done || data != "" {
		t.Fatalf("terminal read = (%q, %v, %v), want a clean end", data, done, err)
	}
}

// TestSettlePumpsMarksStreamsItCut checks bounded drain cuts streams.
func TestSettlePumpsMarksStreamsItCut(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
	child.stdout.push(readChunk{data: []byte("parent-done\n")}, nil)
	pumpDone := make(chan struct{}) // never closes: the pumps are blocked
	stdoutPipe := &trackingReadCloser{closed: make(chan struct{})}
	stderrPipe := &trackingReadCloser{closed: make(chan struct{})}

	settled := make(chan struct{})
	go func() {
		child.settlePumps(5*time.Millisecond, nil, pumpDone, stdoutPipe, stderrPipe)
		close(settled)
	}()

	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("settlePumps did not settle on a pump that cannot finish")
	}

	if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "parent-done\n" {
		t.Fatalf("first read = (%q, %v, %v), want the buffered prefix", data, done, err)
	}
	if _, _, err := child.ReadStdoutContext(context.Background()); !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal stdout read error = %v, want errOutputStreamCutShort", err)
	}
	if _, _, err := child.ReadStderrContext(context.Background()); !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal stderr read error = %v, want errOutputStreamCutShort", err)
	}
	select {
	case <-stdoutPipe.closed:
	default:
		t.Fatal("settlePumps did not close the stdout pipe of the stream it cut")
	}
	select {
	case <-stderrPipe.closed:
	default:
		t.Fatal("settlePumps did not close the stderr pipe of the stream it cut")
	}
}

// TestSettlePumpsKeepsQueuedPrefixAndCleanEOF preserves every admitted byte
// when forced settlement closes an incomplete stream. A second stream that
// has already reached EOF must keep its complete verdict.
func TestSettlePumpsKeepsQueuedPrefixAndCleanEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

		child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
		// stdout: one real chunk, then the pipe stays open (an escaped writer holds
		// the write end) until settlePumps closes it; the pump's read then fails
		// with os.ErrClosed, which is not io.EOF, so the pump marks the queue
		// truncated itself after pushing the chunk.
		stdoutPipe := newBlockingReadPipe(t, "parent-done\n")
		// stderr: the pump reaches a clean io.EOF on its own before settlePumps
		// runs, so its queue closes cleanly and must stay untruncated.
		stderrPipe := &eofReadCloser{}

		var pumpWg sync.WaitGroup
		pumpWg.Add(2)
		pumpDone := make(chan struct{})
		go func() {
			defer pumpWg.Done()
			child.pumpPipe(stdoutPipe, &child.stdout)
		}()
		go func() {
			defer pumpWg.Done()
			child.pumpPipe(stderrPipe, &child.stderr)
		}()
		go func() {
			pumpWg.Wait()
			close(pumpDone)
		}()

		// Wait until stdout has admitted its prefix and stderr has reached EOF.
		// The fake-clock ceiling then cuts only the blocked stdout reader.
		synctest.Wait()
		child.settlePumps(50*time.Millisecond, nil, pumpDone, stdoutPipe, stderrPipe)

		if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "parent-done\n" {
			t.Fatalf("first stdout read = (%q, %v, %v), want the admitted prefix", data, done, err)
		}
		if _, _, err := child.ReadStdoutContext(context.Background()); !errors.Is(err, errOutputStreamCutShort) {
			t.Fatalf("terminal stdout read error = %v, want errOutputStreamCutShort", err)
		}
		if _, done, err := child.ReadStderrContext(context.Background()); err != nil || !done {
			t.Fatalf("terminal stderr read = (%v, %v), want a clean end, not a truncation", done, err)
		}

	})
}

// TestSettlePumpsAbortsPromptly pins the abort channel: a kill, timeout,
// base-context cancellation, or failed tree close must settle the pumps in
// milliseconds, not wait out the full ceiling.
func TestSettlePumpsAbortsPromptly(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
	pumpDone := make(chan struct{}) // never closes: the pumps are blocked
	stdoutPipe := &trackingReadCloser{closed: make(chan struct{})}
	stderrPipe := &trackingReadCloser{closed: make(chan struct{})}
	abort := make(chan struct{})

	settled := make(chan struct{})
	start := time.Now()
	go func() {
		child.settlePumps(outputEOFCeiling, abort, pumpDone, stdoutPipe, stderrPipe)
		close(settled)
	}()
	// Simulate Kill()/baseCtx cancellation firing long before the ceiling.
	time.Sleep(20 * time.Millisecond)
	close(abort)

	select {
	case <-settled:
	case <-time.After(2 * time.Second):
		t.Fatal("settlePumps ignored the abort channel and waited for the ceiling")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("settlePumps took %s after abort, want prompt settlement", elapsed)
	}
	if _, _, err := child.ReadStdoutContext(context.Background()); !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal stdout read error = %v, want errOutputStreamCutShort after abort", err)
	}
}

// TestSettlePumpsLeavesFinishedPumpsAlone keeps the normal path unchanged: a
// pump that reached EOF is not marked truncated, and its pipe is left to the
// pump.
func TestSettlePumpsLeavesFinishedPumpsAlone(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
	child.stdout.close()
	child.stderr.close()
	pumpDone := make(chan struct{})
	close(pumpDone)
	pipe := &trackingReadCloser{closed: make(chan struct{})}

	child.settlePumps(5*time.Millisecond, nil, pumpDone, pipe, pipe)

	if _, done, err := child.ReadStdoutContext(context.Background()); err != nil || !done {
		t.Fatalf("terminal read = (%v, %v), want a clean end", done, err)
	}
	select {
	case <-pipe.closed:
		t.Fatal("settlePumps closed the pipe of a stream that had finished")
	default:
	}
}

// TestSettlePumpsPrefersFinishedPumpsOverAbort pins the ready-check: when the
// pumps have already finished AND the abort channel is closed at the same
// instant, settlePumps must take the pumpDone path deterministically and leave
// the pipes alone, instead of letting the select pick abort at random and cut
// streams that reached EOF.
func TestSettlePumpsPrefersFinishedPumpsOverAbort(t *testing.T) {
	t.Parallel()

	for i := range 50 {
		child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
		child.stdout.close()
		child.stderr.close()
		pumpDone := make(chan struct{})
		close(pumpDone)
		abort := make(chan struct{})
		close(abort)
		pipe := &trackingReadCloser{closed: make(chan struct{})}

		child.settlePumps(time.Hour, abort, pumpDone, pipe, pipe)

		if _, done, err := child.ReadStdoutContext(context.Background()); err != nil || !done {
			t.Fatalf("iteration %d: terminal read = (%v, %v), want a clean end", i, done, err)
		}
		select {
		case <-pipe.closed:
			t.Fatalf("iteration %d: settlePumps closed the pipe despite pumpDone being ready", i)
		default:
		}
	}
}

// TestSettlePumpsAbortGraceLetsPumpsReachEOF pins the timeout grace: when abort
// fires while a pump is mid-drain, settlePumps first gives the pump a grace
// window to hit natural EOF (the process-group writers are dead by then), and
// only force-closes the pipe if the pump is still stuck. A pump that reaches
// EOF inside the grace must end clean, not truncated.
func TestSettlePumpsAbortGraceLetsPumpsReachEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

		abort := make(chan struct{})
		close(abort)
		child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0), abort: abort}
		// stdout pump serves one chunk, then reaches a clean EOF shortly after
		// abort fires (simulating a dead in-group writer whose buffered output the
		// pump has not read yet).
		stdoutPipe := newDelayedEOFPipe("final-chunk\n", 30*time.Millisecond)
		stderrPipe := &eofReadCloser{}

		var pumpWg sync.WaitGroup
		pumpWg.Add(2)
		pumpDone := make(chan struct{})
		go func() {
			defer pumpWg.Done()
			child.pumpPipe(stdoutPipe, &child.stdout)
		}()
		go func() {
			defer pumpWg.Done()
			child.pumpPipe(stderrPipe, &child.stderr)
		}()
		go func() {
			pumpWg.Wait()
			close(pumpDone)
		}()

		child.settlePumps(time.Hour, abort, pumpDone, stdoutPipe, stderrPipe)

		if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "final-chunk\n" {
			t.Fatalf("first stdout read = (%q, %v, %v), want the final chunk", data, done, err)
		}
		if _, done, err := child.ReadStdoutContext(context.Background()); err != nil || !done {
			t.Fatalf("terminal stdout read = (%v, %v), want a clean EOF within the abort grace, not a truncation", done, err)
		}

	})
}

// TestSettlePumpsAbortStillCutsEscapedWriter makes sure the abort grace does not
// become unbounded: a pump whose writer escaped the process group stays blocked
// past the grace, so settlePumps closes the pipe and marks the stream truncated.
func TestSettlePumpsAbortStillCutsEscapedWriter(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0), stderr: newPipeQueue(0)}
	child.stdout.push(readChunk{data: []byte("parent-done\n")}, nil)
	pumpDone := make(chan struct{}) // never closes: the escaped writer holds the pipe
	stdoutPipe := &trackingReadCloser{closed: make(chan struct{})}
	stderrPipe := &trackingReadCloser{closed: make(chan struct{})}
	abort := make(chan struct{})
	close(abort)

	settled := make(chan struct{})
	start := time.Now()
	go func() {
		child.settlePumps(time.Hour, abort, pumpDone, stdoutPipe, stderrPipe)
		close(settled)
	}()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("settlePumps did not cut the escaped writer within the abort grace")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("settlePumps took %s after abort with an escaped writer, want ~grace", elapsed)
	}
	if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "parent-done\n" {
		t.Fatalf("first stdout read = (%q, %v, %v), want the queued chunk", data, done, err)
	}
	if _, done, err := child.ReadStdoutContext(context.Background()); !errors.Is(err, errOutputStreamCutShort) || !done {
		t.Fatalf("terminal stdout read = (%v, %v), want (errOutputStreamCutShort, true)", done, err)
	}
}

// Kill must release a pending drain even after the direct child has exited.
// It does not signal the reaped process again.
func TestKillAfterReapAbortsPendingDrain(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{
		done:    make(chan struct{}),
		settled: make(chan struct{}),
		abort:   make(chan struct{}),
	}
	close(child.done)
	child.mu.Lock()
	child.closed = true
	child.mu.Unlock()

	if err := child.Kill(); err != nil {
		t.Fatalf("Kill after reap = %v, want nil", err)
	}
	select {
	case <-child.abort:
	default:
		t.Fatal("Kill on a reaped child did not abort its pending drain")
	}
}

// delayedEOFPipe serves payload, then after delay returns a clean io.EOF —
// modeling a pipe whose in-group writer died and whose kernel buffer the pump
// drains slightly after the abort fired.
type delayedEOFPipe struct {
	payload []byte
	delay   time.Duration
	once    sync.Once
	closed  chan struct{}
}

func newDelayedEOFPipe(payload string, delay time.Duration) *delayedEOFPipe {
	return &delayedEOFPipe{payload: []byte(payload), delay: delay, closed: make(chan struct{})}
}

func (p *delayedEOFPipe) Read(buf []byte) (int, error) {
	if len(p.payload) > 0 {
		n := copy(buf, p.payload)
		p.payload = p.payload[n:]
		return n, nil
	}
	timer := time.NewTimer(p.delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return 0, io.EOF
	case <-p.closed:
		return 0, os.ErrClosed
	}
}

func (p *delayedEOFPipe) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

// TestReadPipeNilContextDoesNotPanic pins the nil-ctx guard shared with
// WriteStdinContext/CloseStdinContext: ReadStdoutContext(nil) must behave like a
// background context, not panic on ctx.Done().
func TestReadPipeNilContextDoesNotPanic(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0)}
	child.stdout.push(readChunk{data: []byte("data")}, nil)
	child.stdout.close()

	data, done, err := child.ReadStdoutContext(nil) //lint:ignore SA1012 nil ctx is the contract under test
	if err != nil || done || data != "data" {
		t.Fatalf("first read = (%q, %v, %v), want the queued data", data, done, err)
	}
	//lint:ignore SA1012 nil ctx is the contract under test
	if _, done, err := child.ReadStdoutContext(nil); err != nil || !done {
		t.Fatalf("terminal read = (%v, %v), want a clean end", done, err)
	}
}

// TestSpawnedChildSettlesPumpsPromptlyAfterKill checks drain settles after kill.
func TestSpawnedChildSettlesPumpsPromptlyAfterKill(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX shell background semantics")
	}

	child, err := SpawnChild(context.Background(), SpawnConfig{
		Command: "sh",
		// Descendant escapes via setsid helper; leader waits for readiness.
		Args: []string{"-c", escaperScript(t.TempDir() + "/ready")},
	})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}

	// The leader exits only after the escaped descendant has left the process
	// group and keeps the stdout write end open, so the pumps cannot reach EOF.
	if _, err := child.WaitContext(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	child.AbortDrain() // what Kill/timeout/baseCtx/tree-close-failure trigger

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		for {
			_, _, err := child.ReadStdoutContext(context.Background())
			if err != nil {
				done <- err
				return
			}
		}
	}()
	select {
	case err := <-done:
		if !errors.Is(err, errOutputStreamCutShort) {
			t.Fatalf("terminal read error = %v, want errOutputStreamCutShort", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("ReadStdout blocked %s after AbortDrain — the drain was not aborted promptly", time.Since(start))
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("drain took %s after abort, want well under the 30s ceiling", elapsed)
	}
}

// blockingReadPipe serves payload once and then blocks in Read until Close,
// like a pipe whose write end an escaped process still holds.
type blockingReadPipe struct {
	payload []byte
	closed  chan struct{}
	once    sync.Once
}

func newBlockingReadPipe(t *testing.T, payload string) *blockingReadPipe {
	t.Helper()
	p := &blockingReadPipe{payload: []byte(payload), closed: make(chan struct{})}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func (p *blockingReadPipe) Read(buf []byte) (int, error) {
	if len(p.payload) > 0 {
		n := copy(buf, p.payload)
		p.payload = p.payload[n:]
		return n, nil
	}
	<-p.closed
	return 0, os.ErrClosed
}

func (p *blockingReadPipe) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

// eofReadCloser reports a clean io.EOF on the first read: the pump finishes on
// its own and its queue must never be marked truncated.
type eofReadCloser struct{}

func (*eofReadCloser) Read([]byte) (int, error) { return 0, io.EOF }
func (*eofReadCloser) Close() error             { return nil }

// TestReadPipeUnblocksOnContext pins the cancellation bound on a blocked read:
// a consumer waiting on a queue that never changes must return ctx.Err() when
// the context is cancelled, and must still observe a terminal state that races
// the cancellation.
func TestReadPipeUnblocksOnContext(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(0)}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, _, err := child.ReadStdoutContext(ctx)
		done <- err
	}()
	// Let the reader block on the queue's notify channel.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked read error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked read did not return on context cancellation")
	}

	// A terminal state arriving with the cancellation is still delivered.
	child2 := &ChildProcess{stdout: newPipeQueue(0)}
	child2.stdout.closeTruncated()
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if _, _, err := child2.ReadStdoutContext(ctx2); !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal read after cancel = %v, want errOutputStreamCutShort", err)
	}
}

// newJSTestRuntime builds a bound goja runtime and event loop for JS-surface
// tests. Auto-exit is OFF: with it on, the loop may commit to termination
// before the first Submit lands (observed on loaded CI runners:
// "submit: eventloop: loop has been terminated"), so the loop runs until
// test cleanup, which already cancels, shuts down, and drains it.
func newJSTestRuntime(t *testing.T) (*goja.Runtime, *gojaeventloop.Adapter, *goeventloop.Loop) {
	t.Helper()
	runtime := goja.New()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("event loop: %v", err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("bind: %v", err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	loopDone := make(chan error, 1)
	go func() { loopDone <- loop.Run(loopCtx) }()
	t.Cleanup(func() {
		loopCancel()
		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
			t.Errorf("loop shutdown: %v", err)
		}
		// Drain the run goroutine so it does not outlive the test.
		<-loopDone
	})
	return runtime, adapter, loop
}

// evalOnLoop runs script on the loop and returns the object it hands to the
// __got callback, so a JS promise can be observed without polling.
func evalOnLoop(t *testing.T, runtime *goja.Runtime, loop *goeventloop.Loop, script string) map[string]any {
	t.Helper()
	got := make(chan map[string]any, 1)
	scriptErr := make(chan error, 1)
	if err := loop.Submit(func() {
		if err := runtime.Set("__got", func(value map[string]any) { got <- value }); err != nil {
			scriptErr <- err
			return
		}
		if _, err := runtime.RunString(script); err != nil {
			scriptErr <- err
		}
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case err := <-scriptErr:
		t.Fatalf("script: %v", err)
	case value := <-got:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("script never settled")
	}
	return nil
}

// TestReadableStreamReportsCutShort pins the JS read() contract: a stream the
// exec layer cut ends with {done: true, truncated: true} instead of rejecting,
// so a streaming consumer can still drain the prefix it received and none can
// mistake the end for a clean EOF. A clean end carries no truncated flag.
func TestReadableStreamReportsCutShort(t *testing.T) {
	skipSlow(t)

	tests := []struct {
		name          string
		read          func(ctx context.Context) (string, bool, error)
		wantTruncated bool
	}{
		{
			name:          "cut short",
			read:          func(ctx context.Context) (string, bool, error) { return "", true, errOutputStreamCutShort },
			wantTruncated: true,
		},
		{
			name: "clean end",
			read: func(ctx context.Context) (string, bool, error) { return "", true, nil },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, adapter, loop := newJSTestRuntime(t)
			execTestOwner(t, loop, func() error {
				stream := wrapReadableStream(context.Background(), runtime, adapter, loop, test.read)
				return runtime.Set("stream", stream)
			})
			value := evalOnLoop(t, runtime, loop, `(async function() {
				__got(await stream.read());
			})()`)
			if done, _ := value["done"].(bool); !done {
				t.Fatalf("done = %v, want true", value["done"])
			}
			truncated, _ := value["truncated"].(bool)
			if truncated != test.wantTruncated {
				t.Fatalf("truncated = %v, want %v (result %#v)", value["truncated"], test.wantTruncated, value)
			}
		})
	}
}

// prefixThenEIOReader serves prefix on the first Read, then fails with a
// non-EOF error — modeling a pipe whose read breaks (EIO) after the child
// was already reaped and some output was captured.
type prefixThenEIOReader struct {
	prefix []byte
	sent   bool
}

func (r *prefixThenEIOReader) Read(buf []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(buf, r.prefix), nil
	}
	return 0, syscall.EIO
}
func (r *prefixThenEIOReader) Close() error { return nil }

// TestPumpPipe_ReadFailureAfterRecordedExitStaysTruncated pins V10: a pump
// read that fails with a NON-EOF error must mark the stream truncated, even
// when the child's exit was already recorded (wait code 0). The recorded exit
// establishes the verdict, but it does not make the capture complete — an
// escaped descendant holding the pipe can still cut it after the reaper ran.
// The terminal read must therefore surface done + errOutputStreamCutShort,
// never a clean EOF.
func TestPumpPipe_ReadFailureAfterRecordedExitStaysTruncated(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{
		stdout: newPipeQueue(0),
		stderr: newPipeQueue(0),
		done:   make(chan struct{}),
	}
	// Publish the real exit record; setting exitCode alone does not exercise
	// the post-reap path.
	child.recordExit(nil, nil)
	close(child.done)

	child.pumpPipe(&prefixThenEIOReader{prefix: []byte("partial\n")}, &child.stdout)

	if data, done, err := child.ReadStdoutContext(context.Background()); err != nil || done || data != "partial\n" {
		t.Fatalf("first stdout read = (%q, %v, %v), want the prefix chunk", data, done, err)
	}
	if _, done, err := child.ReadStdoutContext(context.Background()); !errors.Is(err, errOutputStreamCutShort) || !done {
		t.Fatalf("terminal stdout read = (%v, %v), want (errOutputStreamCutShort, true) — a non-EOF read failure after a recorded exit is still a truncated capture", done, err)
	}
	if !child.stdout.cutShort() {
		t.Fatal("stdout queue not marked cut short after a non-EOF read failure")
	}
	// The recorded exit code is still observable and unaffected.
	if code, waitErr := child.WaitContext(context.Background()); waitErr != nil || code != 0 {
		t.Fatalf("WaitContext = (%d, %v), want (0, nil) — the truncation must not corrupt the recorded exit", code, waitErr)
	}
}
