package exec

import (
	"context"
	"errors"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// TestPipeQueueReportsCutShortStream pins the terminal contract of a stream the
// exec layer closed instead of the writer: what was queued is delivered first,
// and the end of the stream is an explicit cut-short, never a clean EOF.
func TestPipeQueueReportsCutShortStream(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(), stderr: newPipeQueue()}
	child.stdout.push(readChunk{data: []byte("parent-done\n")})
	if !child.stdout.closeTruncated() {
		t.Fatal("closeTruncated on an open queue reported it was already closed")
	}

	data, done, err := child.ReadStdout()
	if err != nil || done || data != "parent-done\n" {
		t.Fatalf("first read = (%q, %v, %v), want the queued data", data, done, err)
	}
	data, done, err = child.ReadStdout()
	if !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal read = (%q, %v, %v), want errOutputStreamCutShort", data, done, err)
	}
}

// TestPipeQueueCleanCloseStaysClean makes sure the truncation flag is not set by
// a stream that ended on its own, and that the queued data still arrives before
// the terminal state.
func TestPipeQueueCleanCloseStaysClean(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(), stderr: newPipeQueue()}
	child.stdout.push(readChunk{data: []byte("complete\n")})
	child.stdout.close()
	if child.stdout.closeTruncated() {
		t.Fatal("closeTruncated re-marked a stream that had already ended")
	}

	if data, done, err := child.ReadStdout(); err != nil || done || data != "complete\n" {
		t.Fatalf("first read = (%q, %v, %v), want the queued data", data, done, err)
	}
	if data, done, err := child.ReadStdout(); err != nil || !done || data != "" {
		t.Fatalf("terminal read = (%q, %v, %v), want a clean end", data, done, err)
	}
}

// TestSettlePumpsMarksStreamsItCut pins the bounded drain: a pump that cannot
// finish is cut, its stream is marked truncated so a consumer can tell its
// prefix from the whole output, and the handle still settles instead of hanging
// on a writer the platform could not kill.
func TestSettlePumpsMarksStreamsItCut(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(), stderr: newPipeQueue()}
	child.stdout.push(readChunk{data: []byte("parent-done\n")})
	pumpDone := make(chan struct{}) // never closes: the pumps are blocked
	stdoutPipe := &trackingReadCloser{closed: make(chan struct{})}

	settled := make(chan struct{})
	go func() {
		child.settlePumps(5*time.Millisecond, pumpDone, stdoutPipe, stdoutPipe)
		close(settled)
	}()

	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("settlePumps did not settle on a pump that cannot finish")
	}

	if data, done, err := child.ReadStdout(); err != nil || done || data != "parent-done\n" {
		t.Fatalf("first read = (%q, %v, %v), want the buffered prefix", data, done, err)
	}
	if _, _, err := child.ReadStdout(); !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("terminal read error = %v, want errOutputStreamCutShort", err)
	}
	select {
	case <-stdoutPipe.closed:
	default:
		t.Fatal("settlePumps did not close the pipe of the stream it cut")
	}
}

// TestSettlePumpsLeavesFinishedPumpsAlone keeps the normal path unchanged: a
// pump that reached EOF is not marked truncated, and its pipe is left to the
// pump.
func TestSettlePumpsLeavesFinishedPumpsAlone(t *testing.T) {
	t.Parallel()

	child := &ChildProcess{stdout: newPipeQueue(), stderr: newPipeQueue()}
	child.stdout.close()
	child.stderr.close()
	pumpDone := make(chan struct{})
	close(pumpDone)
	pipe := &trackingReadCloser{closed: make(chan struct{})}

	child.settlePumps(5*time.Millisecond, pumpDone, pipe, pipe)

	if _, done, err := child.ReadStdout(); err != nil || !done {
		t.Fatalf("terminal read = (%v, %v), want a clean end", done, err)
	}
	select {
	case <-pipe.closed:
		t.Fatal("settlePumps closed the pipe of a stream that had finished")
	default:
	}
}

// newJSTestRuntime builds a bound goja runtime and event loop for JS-surface
// tests. The loop runs until test cleanup.
func newJSTestRuntime(t *testing.T) (*goja.Runtime, *gojaeventloop.Adapter, *goeventloop.Loop) {
	t.Helper()
	runtime := goja.New()
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
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
	})
	return runtime, adapter, loop
}

// evalOnLoop runs script on the loop and returns the object it hands to the
// __got callback, so a JS promise can be observed without polling.
func evalOnLoop(t *testing.T, runtime *goja.Runtime, loop *goeventloop.Loop, script string) map[string]any {
	t.Helper()
	got := make(chan map[string]any, 1)
	if err := runtime.Set("__got", func(value map[string]any) { got <- value }); err != nil {
		t.Fatalf("set callback: %v", err)
	}
	scriptErr := make(chan error, 1)
	if err := loop.Submit(func() {
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
		read          func() (string, bool, error)
		wantTruncated bool
	}{
		{
			name:          "cut short",
			read:          func() (string, bool, error) { return "", true, errOutputStreamCutShort },
			wantTruncated: true,
		},
		{
			name: "clean end",
			read: func() (string, bool, error) { return "", true, nil },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, adapter, loop := newJSTestRuntime(t)
			stream := wrapReadableStream(context.Background(), runtime, adapter, loop, test.read)
			if err := runtime.Set("stream", stream); err != nil {
				t.Fatalf("set stream: %v", err)
			}
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
