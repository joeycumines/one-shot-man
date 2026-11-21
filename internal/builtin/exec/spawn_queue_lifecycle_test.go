package exec

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestPumpPipe_ReapKeepsFullQueueBlockedUntilRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const payload = "AAAABBBBCCCC"
		child := &ChildProcess{stdout: newPipeQueue(4), done: make(chan struct{})}
		pumpDone := make(chan struct{})
		go func() {
			child.pumpPipe(io.NopCloser(strings.NewReader(payload)), &child.stdout)
			close(pumpDone)
		}()
		synctest.Wait()
		child.recordExit(nil, nil)
		close(child.done)
		// A closed reap channel must not turn the capacity wait into a retry
		// loop. Wait returns only when the pump is durably blocked again.
		synctest.Wait()
		if code, err := child.WaitContext(context.Background()); code != 0 || err != nil {
			t.Fatalf("wait with unread output = (%d, %v), want (0, nil)", code, err)
		}
		select {
		case <-pumpDone:
			t.Fatal("pump abandoned output instead of waiting for a reader")
		default:
		}
		var captured strings.Builder
		for {
			if n := child.stdout.retainedBytes(); n > 4 {
				t.Fatalf("retained=%d, cap=4", n)
			}
			data, done, err := child.ReadStdoutContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			captured.WriteString(data)
			if done {
				break
			}
			synctest.Wait()
		}
		<-pumpDone
		if captured.String() != payload {
			t.Fatalf("capture=%q, want %q", captured.String(), payload)
		}
	})
}

// lastReadEOF exercises the allowed io.Reader result (n > 0, io.EOF): those
// final bytes still need admission before the queue can be called complete.
type lastReadEOF struct{ reader *strings.Reader }

func (r *lastReadEOF) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if r.reader.Len() == 0 {
		err = io.EOF
	}
	return n, err
}
func (*lastReadEOF) Close() error { return nil }

func TestPumpPipe_AbortedAdmissionCannotBecomeCleanEOF(t *testing.T) {
	for _, simultaneousEOF := range []bool{false, true} {
		name := "separate EOF"
		if simultaneousEOF {
			name = "data and EOF together"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				child := &ChildProcess{stdout: newPipeQueue(4), abort: make(chan struct{})}
				var reader io.ReadCloser = io.NopCloser(strings.NewReader("AAAABBBB"))
				if simultaneousEOF {
					reader = &lastReadEOF{reader: strings.NewReader("AAAABBBB")}
				}
				pumpDone := make(chan struct{})
				go func() { child.pumpPipe(reader, &child.stdout); close(pumpDone) }()
				synctest.Wait()
				child.AbortDrain()
				synctest.Wait()
				<-pumpDone
				if data, done, err := child.ReadStdoutContext(context.Background()); data != "AAAA" || done || err != nil {
					t.Fatalf("retained prefix = (%q,%v,%v), want (AAAA,false,nil)", data, done, err)
				}
				if _, done, err := child.ReadStdoutContext(context.Background()); !done || !errors.Is(err, errOutputStreamCutShort) {
					t.Fatalf("lost BBBB but terminal read = (%v,%v)", done, err)
				}
			})
		})
	}
}

func TestPumpPipe_AbortedButFullyRetainedOutputStaysComplete(t *testing.T) {
	child := &ChildProcess{stdout: newPipeQueue(8), abort: make(chan struct{})}
	child.AbortDrain()
	child.pumpPipe(&lastReadEOF{reader: strings.NewReader("complete")}, &child.stdout)
	if data, done, err := child.ReadStdoutContext(context.Background()); data != "complete" || done || err != nil {
		t.Fatalf("read = (%q,%v,%v), want the complete payload", data, done, err)
	}
	if _, done, err := child.ReadStdoutContext(context.Background()); !done || err != nil {
		t.Fatalf("terminal read = (%v,%v), want clean EOF", done, err)
	}
}

func TestPipeQueue_CapacityDoesNotNarrowToMachineInt(t *testing.T) {
	for _, capacity := range []int64{-1, 0, 1, 1 << 32, math.MaxInt64} {
		q := newPipeQueue(capacity)
		want := capacity
		if want <= 0 {
			want = DefaultQueueCapBytes
		}
		if q.capBytes != want {
			t.Fatalf("configured cap %d became %d, want %d", capacity, q.capBytes, want)
		}
		child := &ChildProcess{}
		child.pumpPipe(io.NopCloser(strings.NewReader("x")), &q)
		chunk, ok, _, _ := q.pop()
		if !ok || string(chunk.data) != "x" || q.cutShort() {
			t.Fatalf("cap %d: read=%q ok=%v truncated=%v", capacity, chunk.data, ok, q.cutShort())
		}
	}
}

func TestPipeQueue_TerminalVerdictIsImmutable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for range 100 {
			q := newPipeQueue(4)
			q.push(readChunk{data: []byte("kept")}, nil)
			truncatedFirst := make(chan bool, 1)
			go q.close()
			go func() { truncatedFirst <- q.closeTruncated() }()
			synctest.Wait()
			wantTruncated := <-truncatedFirst
			q.close()
			q.closeTruncated()
			if q.cutShort() != wantTruncated {
				t.Fatalf("terminal verdict changed: truncated=%v, first closer truncated=%v", q.cutShort(), wantTruncated)
			}
			chunk, ok, _, _ := q.pop()
			if !ok || string(chunk.data) != "kept" {
				t.Fatalf("terminal transition discarded an admitted prefix: %q", chunk.data)
			}
		}
	})
}

type eofOnClosePipe struct {
	closed chan struct{}
	once   sync.Once
}

func (r *eofOnClosePipe) Read([]byte) (int, error) {
	<-r.closed
	return 0, io.EOF
}
func (r *eofOnClosePipe) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestSettlePumps_ForcedCutoffWakesQueueAndKeepsVerdict(t *testing.T) {
	for _, blockedRead := range []bool{false, true} {
		name := "full queue"
		if blockedRead {
			name = "EOF induced by Close"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				child := &ChildProcess{stdout: newPipeQueue(4), stderr: newPipeQueue(4)}
				child.stdout.push(readChunk{data: []byte("kept")}, nil)
				var stdout io.ReadCloser = io.NopCloser(strings.NewReader("lost"))
				if blockedRead {
					stdout = &eofOnClosePipe{closed: make(chan struct{})}
				}
				stderr := &eofReadCloser{}
				pumpDone := make(chan struct{})
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); child.pumpPipe(stdout, &child.stdout) }()
				go func() { defer wg.Done(); child.pumpPipe(stderr, &child.stderr) }()
				go func() { wg.Wait(); close(pumpDone) }()
				synctest.Wait()
				start := time.Now()
				child.settlePumps(time.Second, nil, pumpDone, stdout, stderr)
				if elapsed := time.Since(start); elapsed != time.Second {
					t.Fatalf("settlement elapsed=%v, want exact one-second fake-clock cutoff", elapsed)
				}
				<-pumpDone
				if data, done, err := child.ReadStdoutContext(context.Background()); data != "kept" || done || err != nil {
					t.Fatalf("admitted prefix = (%q,%v,%v), want kept", data, done, err)
				}
				if _, done, err := child.ReadStdoutContext(context.Background()); !done || !errors.Is(err, errOutputStreamCutShort) {
					t.Fatalf("forced cutoff became clean: (%v,%v)", done, err)
				}
				if _, done, err := child.ReadStderrContext(context.Background()); !done || err != nil {
					t.Fatalf("previous clean EOF became incomplete: (%v,%v)", done, err)
				}
			})
		})
	}
}
