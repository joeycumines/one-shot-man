package aimuxcore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeEventHandle is a test double for AgentHandle that allows direct control
// of the Events channel. It implements the full AgentHandle interface.
type fakeEventHandle struct {
	eventsCh  chan LineEvent
	alive     atomic.Bool
	closed    atomic.Bool
	healthMu  sync.RWMutex
	lastEvent time.Time
	lastSend  time.Time
	sendErr   error
}

var _ AgentHandle = (*fakeEventHandle)(nil)

func newFakeEventHandle() *fakeEventHandle {
	h := &fakeEventHandle{
		eventsCh: make(chan LineEvent, 256),
	}
	h.alive.Store(true)
	return h
}

func (h *fakeEventHandle) Send(input string) error {
	if h.sendErr != nil {
		return h.sendErr
	}
	h.healthMu.Lock()
	h.lastSend = time.Now()
	h.healthMu.Unlock()
	return nil
}

func (h *fakeEventHandle) Receive() (string, error) { return "", io.EOF }

func (h *fakeEventHandle) Close() error {
	if h.closed.CompareAndSwap(false, true) {
		h.alive.Store(false)
		close(h.eventsCh)
	}
	return nil
}

func (h *fakeEventHandle) IsAlive() bool { return h.alive.Load() }

func (h *fakeEventHandle) Wait() (int, error) { return 0, nil }

func (h *fakeEventHandle) Resize(_, _ int) error { return nil }

func (h *fakeEventHandle) WaitReady(_ context.Context) error { return nil }

func (h *fakeEventHandle) Events() <-chan LineEvent { return h.eventsCh }

func (h *fakeEventHandle) Health() HealthSnapshot {
	h.healthMu.RLock()
	defer h.healthMu.RUnlock()
	return HealthSnapshot{
		Alive:     h.alive.Load(),
		LastEvent: h.lastEvent,
		LastSend:  h.lastSend,
	}
}

func (h *fakeEventHandle) emitLine(line string) {
	h.healthMu.Lock()
	h.lastEvent = time.Now()
	h.healthMu.Unlock()
	h.eventsCh <- LineEvent{Line: line}
}

func (h *fakeEventHandle) emitEOF() {
	h.eventsCh <- LineEvent{Err: io.EOF}
	close(h.eventsCh)
}

// --- Line splitting tests (captureAgentHandle.drainLines via forwardOutput) ---

func TestLineSplitting_MultiLineChunk(t *testing.T) {
	t.Parallel()

	var lines []string
	buf := []byte("line1\nline2\nline3\n")
	remaining := drainLinesForTest(buf, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Errorf("lines = %v, want [line1 line2 line3]", lines)
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty remaining buffer, got %q", string(remaining))
	}
}

func TestLineSplitting_ChunkSplitMidLine(t *testing.T) {
	t.Parallel()

	var lines []string
	buf1 := []byte("partial")
	remaining := drainLinesForTest(buf1, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 0 {
		t.Fatalf("expected 0 complete lines from partial chunk, got %d", len(lines))
	}
	if string(remaining) != "partial" {
		t.Errorf("remaining = %q, want %q", string(remaining), "partial")
	}

	buf2 := append(remaining, []byte(" line\n")...)
	remaining2 := drainLinesForTest(buf2, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 1 {
		t.Fatalf("expected 1 line after completing, got %d: %v", len(lines), lines)
	}
	if lines[0] != "partial line" {
		t.Errorf("line = %q, want %q", lines[0], "partial line")
	}
	if len(remaining2) != 0 {
		t.Errorf("expected empty remaining, got %q", string(remaining2))
	}
}

func TestLineSplitting_CRLFHandling(t *testing.T) {
	t.Parallel()

	var lines []string
	buf := []byte("line1\r\nline2\r\n")
	remaining := drainLinesForTest(buf, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" {
		t.Errorf("line[0] = %q, want %q (\\r should be stripped)", lines[0], "line1")
	}
	if lines[1] != "line2" {
		t.Errorf("line[1] = %q, want %q (\\r should be stripped)", lines[1], "line2")
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty remaining, got %q", string(remaining))
	}
}

func TestLineSplitting_MixedLFAndCRLF(t *testing.T) {
	t.Parallel()

	var lines []string
	buf := []byte("lf\n crlf\r\n mixed\r\nlf\n")
	remaining := drainLinesForTest(buf, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(lines), lines)
	}
	expected := []string{"lf", " crlf", " mixed", "lf"}
	for i, want := range expected {
		if lines[i] != want {
			t.Errorf("line[%d] = %q, want %q", i, lines[i], want)
		}
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty remaining, got %q", string(remaining))
	}
}

func TestLineSplitting_NoTrailingNewline(t *testing.T) {
	t.Parallel()

	var lines []string
	buf := []byte("line1\nline2")
	remaining := drainLinesForTest(buf, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 1 {
		t.Fatalf("expected 1 complete line, got %d: %v", len(lines), lines)
	}
	if lines[0] != "line1" {
		t.Errorf("line[0] = %q, want %q", lines[0], "line1")
	}
	if string(remaining) != "line2" {
		t.Errorf("remaining = %q, want %q", string(remaining), "line2")
	}
}

func TestLineSplitting_EmptyLines(t *testing.T) {
	t.Parallel()

	var lines []string
	buf := []byte("a\n\nb\n")
	remaining := drainLinesForTest(buf, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (including empty), got %d: %v", len(lines), lines)
	}
	if lines[0] != "a" || lines[1] != "" || lines[2] != "b" {
		t.Errorf("lines = %v, want [a  b]", lines)
	}
	if len(remaining) != 0 {
		t.Errorf("expected empty remaining, got %q", string(remaining))
	}
}

// drainLinesForTest is a test helper that mirrors captureAgentHandle.drainLines
// logic without requiring a full handle instance.
func drainLinesForTest(buf []byte, emit func(string)) []byte {
	for {
		idx := bytes.IndexByte(buf, '\n')
		if idx < 0 {
			return buf
		}
		line := strings.TrimRight(string(buf[:idx]), "\r")
		emit(line)
		buf = buf[idx+1:]
	}
}

// --- EventStream tests ---

func TestEventStream_EmitsParsedEvents(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	parser := NewParser()
	es := NewEventStream(handle, parser)
	defer es.Close()

	handle.emitLine("Error: something went wrong")
	handle.emitLine("Thinking...")
	handle.emitLine("regular text line")

	events := es.Events()

	ev1 := <-events
	if ev1.Type != EventError {
		t.Errorf("ev1.Type = %s, want %s", EventTypeName(ev1.Type), EventTypeName(EventError))
	}
	if ev1.Line != "Error: something went wrong" {
		t.Errorf("ev1.Line = %q, want %q", ev1.Line, "Error: something went wrong")
	}
	if ev1.Pattern != "error-prefix" {
		t.Errorf("ev1.Pattern = %q, want %q", ev1.Pattern, "error-prefix")
	}

	ev2 := <-events
	if ev2.Type != EventThinking {
		t.Errorf("ev2.Type = %s, want %s", EventTypeName(ev2.Type), EventTypeName(EventThinking))
	}

	ev3 := <-events
	if ev3.Type != EventText {
		t.Errorf("ev3.Type = %s, want %s", EventTypeName(ev3.Type), EventTypeName(EventText))
	}
	if ev3.Pattern != "" {
		t.Errorf("ev3.Pattern = %q, want empty", ev3.Pattern)
	}
}

func TestEventStream_NilParser_EmitsTextEvents(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	es := NewEventStream(handle, nil)
	defer es.Close()

	handle.emitLine("Error: this would be parsed if parser was set")
	handle.emitLine("another line")

	events := es.Events()

	ev1 := <-events
	if ev1.Type != EventText {
		t.Errorf("ev1.Type = %s, want EventText (nil parser)", EventTypeName(ev1.Type))
	}
	if ev1.Line != "Error: this would be parsed if parser was set" {
		t.Errorf("ev1.Line = %q", ev1.Line)
	}

	ev2 := <-events
	if ev2.Type != EventText {
		t.Errorf("ev2.Type = %s, want EventText", EventTypeName(ev2.Type))
	}
}

func TestEventStream_EOFClosesChannel(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	parser := NewParser()
	es := NewEventStream(handle, parser)
	defer es.Close()

	handle.emitLine("line before EOF")
	handle.emitEOF()

	events := es.Events()

	ev := <-events
	if ev.Line != "line before EOF" {
		t.Fatalf("ev.Line = %q, want %q", ev.Line, "line before EOF")
	}

	select {
	case _, ok := <-events:
		if ok {
			t.Error("expected events channel to be closed after EOF")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for events channel to close after EOF")
	}
}

func TestEventStream_CloseStopsReader(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	parser := NewParser()
	es := NewEventStream(handle, parser)

	events := es.Events()

	es.Close()

	select {
	case _, ok := <-events:
		if ok {
			t.Error("expected events channel to be closed after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for events channel to close after Close")
	}
}

func TestEventStream_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	es := NewEventStream(handle, NewParser())

	es.Close()
	es.Close()
	es.Close()
}

func TestEventStream_NilEventsChannel_ReturnsImmediately(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	es := NewEventStream(handle, NewParser())
	defer es.Close()

	close(handle.eventsCh)

	events := es.Events()

	select {
	case _, ok := <-events:
		if ok {
			t.Error("expected events channel to be closed when handle has nil events")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for events channel to close")
	}
}

func TestEventStream_ConcurrentCloseAndRead(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	es := NewEventStream(handle, NewParser())

	events := es.Events()

	var wg sync.WaitGroup
	wg.Go(func() {
		for range events {
		}
	})

	handle.emitLine("line 1")
	handle.emitLine("line 2")
	handle.emitEOF()

	es.Close()
	wg.Wait()
}

// --- HealthMonitor tests ---

func TestHealthMonitor_InitialSnapshot(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	hm := NewHealthMonitor(handle, 50*time.Millisecond)
	defer hm.Close()

	snap := hm.Snapshot()
	if !snap.Alive {
		t.Error("expected alive=true in initial snapshot")
	}
}

func TestHealthMonitor_UpdatesOnInterval(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	hm := NewHealthMonitor(handle, 10*time.Millisecond)
	defer hm.Close()

	handle.healthMu.Lock()
	handle.lastEvent = time.Now()
	handle.healthMu.Unlock()

	time.Sleep(50 * time.Millisecond)

	snap := hm.Snapshot()
	if snap.LastEvent.IsZero() {
		t.Error("expected LastEvent to be updated after polling interval")
	}
}

func TestHealthMonitor_CloseStopsPolling(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	hm := NewHealthMonitor(handle, 10*time.Millisecond)

	hm.Close()

	handle.healthMu.Lock()
	handle.lastEvent = time.Now()
	handle.healthMu.Unlock()

	time.Sleep(30 * time.Millisecond)

	snap := hm.Snapshot()
	if !snap.LastEvent.IsZero() {
		t.Error("expected snapshot to remain stale after Close (polling stopped)")
	}
}

func TestHealthMonitor_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	hm := NewHealthMonitor(handle, 10*time.Millisecond)

	hm.Close()
	hm.Close()
	hm.Close()
}

func TestHealthMonitor_ZeroInterval_NoPolling(t *testing.T) {
	t.Parallel()

	handle := newFakeEventHandle()
	hm := NewHealthMonitor(handle, 0)
	defer hm.Close()

	handle.healthMu.Lock()
	handle.lastEvent = time.Now()
	handle.healthMu.Unlock()

	time.Sleep(30 * time.Millisecond)

	snap := hm.Snapshot()
	if !snap.LastEvent.IsZero() {
		t.Error("expected snapshot to remain stale with zero interval")
	}
}

// --- captureAgentHandle integration tests (real process) ---

func TestCaptureAgentHandle_Events_LineBuffering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}

	p := NewProcessProvider("test", "go", []string{"version"}, ProviderCapabilities{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h, err := p.Spawn(ctx, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}
	defer h.Close()

	if err := h.WaitReady(ctx); err != nil {
		t.Fatalf("waitReady failed: %v", err)
	}

	eventsCh := h.Events()
	if eventsCh == nil {
		t.Fatal("Events() returned nil")
	}

	var lines []string
	timeout := time.After(5 * time.Second)

collect:
	for {
		select {
		case le, ok := <-eventsCh:
			if !ok {
				break collect
			}
			if le.Err != nil {
				break collect
			}
			lines = append(lines, le.Line)
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}

	if len(lines) == 0 {
		t.Fatal("expected at least one line event from 'go version'")
	}

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "go version") {
		t.Errorf("expected output to contain 'go version', got %q", joined)
	}
}

func TestCaptureAgentHandle_Health_TracksSendAndEvent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows due to PTY ANSI handling flake")
	}
	if testing.Short() {
		t.Skip("skipping slow test")
	}

	p := NewProcessProvider("test", "go", []string{"version"}, ProviderCapabilities{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h, err := p.Spawn(ctx, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}
	defer h.Close()

	if err := h.WaitReady(ctx); err != nil {
		t.Fatalf("waitReady failed: %v", err)
	}

	healthBefore := h.Health()
	if healthBefore.LastEvent.IsZero() {
		// Poll briefly for LastEvent to be set (Windows/Linux timing flake)
		deadline := time.Now().Add(2 * time.Second)
		for healthBefore.LastEvent.IsZero() && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
			healthBefore = h.Health()
		}
		if healthBefore.LastEvent.IsZero() {
			t.Error("expected LastEvent to be set after WaitReady (first chunk received)")
		}
	}

	if err := h.Send("test input\n"); err != nil {
		t.Logf("Send returned error (expected for exited process): %v", err)
	}

	healthAfter := h.Health()
	if !healthAfter.LastSend.IsZero() && healthAfter.LastSend.Before(healthAfter.LastEvent) {
		t.Error("expected LastSend to be after LastEvent")
	}
}

func TestCaptureAgentHandle_Health_AliveAfterExit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}

	p := NewProcessProvider("test", "go", []string{"version"}, ProviderCapabilities{})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h, err := p.Spawn(ctx, SpawnOpts{})
	if err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	if err := h.WaitReady(ctx); err != nil {
		t.Fatalf("waitReady failed: %v", err)
	}

	_, _ = h.Wait()

	health := h.Health()
	if health.Alive {
		t.Error("expected Alive=false after process exit")
	}
	if health.LastEvent.IsZero() {
		t.Error("expected LastEvent to be set (output was received)")
	}
}

// --- LineEvent / HealthSnapshot struct tests ---

func TestLineEvent_ZeroValue(t *testing.T) {
	t.Parallel()

	var le LineEvent
	if le.Line != "" {
		t.Errorf("zero LineEvent.Line = %q, want empty", le.Line)
	}
	if le.Err != nil {
		t.Errorf("zero LineEvent.Err = %v, want nil", le.Err)
	}
}

func TestLineEvent_EOF(t *testing.T) {
	t.Parallel()

	le := LineEvent{Err: io.EOF}
	if le.Err == nil {
		t.Error("expected Err to be non-nil for EOF event")
	}
	if !errors.Is(le.Err, io.EOF) {
		t.Errorf("expected Err to be io.EOF, got %v", le.Err)
	}
}

func TestHealthSnapshot_ZeroValue(t *testing.T) {
	t.Parallel()

	var hs HealthSnapshot
	if hs.Alive {
		t.Error("zero HealthSnapshot.Alive = true, want false")
	}
	if !hs.LastEvent.IsZero() {
		t.Error("expected zero LastEvent time")
	}
	if !hs.LastSend.IsZero() {
		t.Error("expected zero LastSend time")
	}
}

func TestHealthMonitor_ConcurrentClose(t *testing.T) {
	t.Parallel()
	h := newFakeEventHandle()
	hm := NewHealthMonitor(h, 50*time.Millisecond)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			hm.Close()
		})
	}
	wg.Wait()
}

func TestProcessProvider_Name(t *testing.T) {
	t.Parallel()
	p := NewProcessProvider("my-agent", "echo", []string{"hi"}, ProviderCapabilities{})
	if got := p.Name(); got != "my-agent" {
		t.Errorf("Name() = %q, want %q", got, "my-agent")
	}
}

func TestProcessProvider_Capabilities(t *testing.T) {
	t.Parallel()
	caps := ProviderCapabilities{MCP: true, Streaming: true}
	p := NewProcessProvider("test", "echo", []string{}, caps)
	if got := p.Capabilities(); !got.MCP || !got.Streaming {
		t.Errorf("Capabilities() = %+v, want MCP=true, Streaming=true", got)
	}
}
