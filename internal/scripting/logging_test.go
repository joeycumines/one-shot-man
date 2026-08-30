package scripting

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// Test newline behavior for non-interactive writer path
func TestPrintToTUI_Writer_Newline(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := NewTUILogger(&buf, nil, 10, slog.LevelInfo)

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
	t.Parallel()
	var got []string
	var mu sync.Mutex

	l := NewTUILogger(nil, nil, 10, slog.LevelInfo)
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
	t.Parallel()
	var buf bytes.Buffer
	l := NewTUILogger(&buf, nil, 10, slog.LevelInfo)

	entered := make(chan struct{})
	block := make(chan struct{})
	done := make(chan struct{})

	// Install a sink that signals when entered and blocks until allowed
	l.SetTUISink(func(s string) {
		close(entered)
		<-block // wait until allowed
	})

	// Goroutine that will print
	go func() {
		l.PrintToTUI("msg")
		close(done)
	}()

	// Wait until PrintToTUI has acquired RLock and entered the sink
	<-entered

	// Call SetTUISink concurrently; it must block until PrintToTUI releases RLock
	setDone := make(chan struct{})
	go func() {
		l.SetTUISink(nil)
		close(setDone)
	}()

	// SetTUISink should be blocked by the RLock held in PrintToTUI
	select {
	case <-setDone:
		t.Fatalf("SetTUISink returned while PrintToTUI held RLock; expected it to block")
	case <-time.After(100 * time.Millisecond):
		// expected timeout - SetTUISink is properly blocked
	}

	// Unblock the sink, allowing PrintToTUI to complete and release RLock
	close(block)
	<-done
	<-setDone
}

// blockingWriter blocks on Write until unblocked, useful to simulate a slow terminal write
type blockingWriter struct {
	entered chan struct{}
	unblk   chan struct{}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	if w.entered != nil {
		close(w.entered)
		w.entered = nil
	}
	<-w.unblk
	return len(p), nil
}

// Ensure SetTUISink blocks while a writer-path PrintToTUI is in progress
func TestPrintToTUI_SetTUISink_Atomicity_WriterPath(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	bw := &blockingWriter{entered: entered, unblk: make(chan struct{})}
	l := NewTUILogger(bw, nil, 10, slog.LevelInfo)

	printed := make(chan struct{})

	// Kick off a print that will block in the writer
	go func() {
		l.PrintToTUI("x")
		close(printed)
	}()

	// Wait until Write is called, meaning PrintToTUI holds the RLock
	<-entered

	// Try to set the sink - should block until PrintToTUI completes
	setDone := make(chan struct{})
	go func() {
		l.SetTUISink(func(string) {})
		close(setDone)
	}()

	// SetTUISink should be blocked by the RLock held in PrintToTUI
	select {
	case <-setDone:
		t.Fatalf("SetTUISink returned before writer-path PrintToTUI completed; expected it to block")
	case <-time.After(100 * time.Millisecond):
		// expected timeout - SetTUISink is properly blocked
	}

	// Unblock writer, allowing PrintToTUI to finish and release RLock
	close(bw.unblk)
	<-printed
	<-setDone
}

// TestTUILogger_FileLogging verifies that logs are written to the provided file writer in JSON format
func TestTUILogger_FileLogging(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := NewTUILogger(nil, &buf, 10, slog.LevelInfo)

	l.Info("test message", slog.String("key", "value"))

	got := buf.String()
	if got == "" {
		t.Fatal("expected output in file buffer, got empty")
	}

	// Verify JSON structure
	if !bytes.Contains(buf.Bytes(), []byte(`"level":"INFO"`)) {
		t.Errorf("output missing log level: %s", got)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"msg":"test message"`)) {
		t.Errorf("output missing message: %s", got)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"key":"value"`)) {
		t.Errorf("output missing attributes: %s", got)
	}
}

func TestJsNormalizePrintfFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"%s", "%v"},
		{"hello %s world", "hello %v world"},
		{"%d %s %f", "%d %v %f"},
		{"%%", "%%"},
		{"100%% %s", "100%% %v"},
		{"%10s", "%10v"},
		{"%-10s", "%-10v"},
		{"%.5s", "%.5v"},
		{"%[1]s", "%[1]v"},
		{"%[2]s %s", "%[2]v %v"},
		{"%#s", "%#v"},
		{"%+s", "%+v"},
		{"%0s", "%0v"},
		{"%*s", "%*v"},
		{"%.*s", "%.*v"},
		{"%3.*s", "%3.*v"},
		{"no format", "no format"},
		{"%d %f %v %t %x", "%d %f %v %t %x"},
		{"%s %s", "%v %v"},
		{"%%s", "%%s"},
		{"trail%", "trail%"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := jsNormalizePrintfFormat(tt.input)
			if got != tt.want {
				t.Errorf("jsNormalizePrintfFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
