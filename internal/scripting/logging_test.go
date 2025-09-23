package scripting

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// Test newline behavior for non-interactive writer path
func TestPrintToTUI_Writer_Newline(t *testing.T) {
	var buf bytes.Buffer
	l := NewTUILogger(&buf, 10)

	l.PrintToTUI("hello")
	l.PrintToTUI("world\n")

	got := buf.String()
	want := "hello\nworld\n"
	if got != want {
		t.Fatalf("writer output mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// Test newline behavior for interactive sink path
func TestPrintToTUI_Sink_Newline(t *testing.T) {
	var got []string
	var mu sync.Mutex

	l := NewTUILogger(nil, 10)
	l.SetTUISink(func(s string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, s)
	})

	l.PrintToTUI("alpha")
	l.PrintToTUI("beta\n")

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0] != "alpha\n" || got[1] != "beta\n" {
		t.Fatalf("sink output mismatch: %+v", got)
	}
}

// Test atomicity between PrintToTUI and SetTUISink: SetTUISink should block
// until any in-flight PrintToTUI completes while holding the read lock
func TestPrintToTUI_SetTUISink_Atomicity(t *testing.T) {
	var buf bytes.Buffer
	l := NewTUILogger(&buf, 10)

	start := make(chan struct{})
	done := make(chan struct{})
	block := make(chan struct{})

	// Install a sink that blocks until we let it proceed, to simulate a long write
	l.SetTUISink(func(s string) {
		<-block // wait until allowed
		// no-op
	})

	// Goroutine that will print after reading current sink under RLock
	go func() {
		close(start)
		l.PrintToTUI("msg")
		close(done)
	}()

	<-start
	// Call SetTUISink concurrently; it must block until PrintToTUI releases RLock
	setDone := make(chan struct{})
	go func() {
		l.SetTUISink(nil)
		close(setDone)
	}()

	// Give goroutines a moment to contend
	time.Sleep(50 * time.Millisecond)

	select {
	case <-setDone:
		t.Fatalf("SetTUISink returned while PrintToTUI held RLock; expected it to block")
	default:
		// expected to be blocked
	}

	// Unblock the sink, allowing PrintToTUI to complete and release RLock
	close(block)
	<-done
	<-setDone
}

// blockingWriter blocks on Write until unblocked, useful to simulate a slow terminal write
type blockingWriter struct {
	unblk chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.unblk
	return len(p), nil
}

// Ensure SetTUISink blocks while a writer-path PrintToTUI is in progress
func TestPrintToTUI_SetTUISink_Atomicity_WriterPath(t *testing.T) {
	bw := &blockingWriter{unblk: make(chan struct{})}
	l := NewTUILogger(bw, 10)

	started := make(chan struct{})
	printed := make(chan struct{})

	// Kick off a print that will block in the writer
	go func() {
		close(started)
		l.PrintToTUI("x")
		close(printed)
	}()

	<-started
	setDone := make(chan struct{})
	go func() {
		l.SetTUISink(func(string) {})
		close(setDone)
	}()

	// Small delay to allow SetTUISink to attempt acquiring the write lock
	time.Sleep(25 * time.Millisecond)

	select {
	case <-setDone:
		t.Fatalf("SetTUISink returned before writer-path PrintToTUI completed; expected it to block")
	default:
		// expected to be blocked
	}

	// Unblock writer, allowing PrintToTUI to finish and release RLock
	close(bw.unblk)
	<-printed
	<-setDone
}
