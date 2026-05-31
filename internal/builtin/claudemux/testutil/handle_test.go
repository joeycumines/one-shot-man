package testutil

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux"
)

// Compile-time interface compliance checks.
var (
	_ claudemux.AgentHandle = (*MockHandle)(nil)
	_ claudemux.AgentHandle = (*ChannelHandle)(nil)
	_ claudemux.AgentHandle = (*ScriptedHandle)(nil)
)

func TestMockHandle_BasicOperations(t *testing.T) {
	t.Parallel()
	h := NewMockHandle()

	if !h.IsAlive() {
		t.Fatal("new MockHandle should be alive")
	}

	if err := h.Send("hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if h.Input.String() != "hello" {
		t.Errorf("Input = %q, want %q", h.Input.String(), "hello")
	}

	// Pre-fill output and receive.
	h.Output.WriteString("world\n")
	out, err := h.Receive()
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if out != "world\n" {
		t.Errorf("Receive() = %q, want %q", out, "world\n")
	}

	// Second receive should be EOF (output was consumed).
	_, err = h.Receive()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF after consuming output, got %v", err)
	}

	// Close.
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if h.IsAlive() {
		t.Error("should not be alive after close")
	}
	if !h.Closed {
		t.Error("Closed flag should be true")
	}

	// Send after close should fail.
	if err := h.Send("test"); err == nil {
		t.Error("Send after close should fail")
	}

	// Receive after close should return EOF.
	_, err = h.Receive()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF after close, got %v", err)
	}
}

func TestMockHandle_Wait(t *testing.T) {
	t.Parallel()
	h := NewMockHandle()
	code, err := h.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if h.IsAlive() {
		t.Error("should not be alive after wait")
	}
}

func TestMockHandle_Resize(t *testing.T) {
	t.Parallel()
	h := NewMockHandle()
	if err := h.Resize(50, 120); err != nil {
		t.Fatalf("Resize: %v", err)
	}
}

func TestMockHandle_WaitReady(t *testing.T) {
	t.Parallel()
	h := NewMockHandle()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestChannelHandle_ConcurrentIO(t *testing.T) {
	t.Parallel()
	h := NewChannelHandle()
	defer h.Close()

	const numWrites = 50
	var wg sync.WaitGroup

	// Concurrent output writer (simulates agent producing output).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range numWrites {
			h.WriteOutput(strings.Repeat("x", i+1))
		}
	}()

	// Concurrent output reader (simulates consumer reading output).
	wg.Add(1)
	go func() {
		defer wg.Done()
		received := 0
		for received < numWrites {
			_, err := h.Receive()
			if err != nil {
				return
			}
			received++
		}
	}()

	wg.Wait()
}

func TestChannelHandle_WriteOutputReadInput(t *testing.T) {
	t.Parallel()
	h := NewChannelHandle()
	defer h.Close()

	// WriteOutput → Receive.
	h.WriteOutput("hello from agent")

	out, err := h.Receive()
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if out != "hello from agent" {
		t.Errorf("Receive() = %q, want %q", out, "hello from agent")
	}

	// Send → ReadInput.
	if err := h.Send("user input"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	input, ok := h.ReadInput()
	if !ok {
		t.Fatal("ReadInput should return ok=true")
	}
	if input != "user input" {
		t.Errorf("ReadInput() = %q, want %q", input, "user input")
	}
}

func TestChannelHandle_Close(t *testing.T) {
	t.Parallel()
	h := NewChannelHandle()

	if !h.IsAlive() {
		t.Fatal("should be alive initially")
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if h.IsAlive() {
		t.Error("should not be alive after close")
	}

	// Receive after close should return EOF.
	_, err := h.Receive()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF after close, got %v", err)
	}

	// Send after close should fail.
	if err := h.Send("test"); err == nil {
		t.Error("Send after close should fail")
	}

	// Double close should be safe.
	if err := h.Close(); err != nil {
		t.Fatalf("double Close: %v", err)
	}
}

func TestChannelHandle_Wait(t *testing.T) {
	t.Parallel()
	h := NewChannelHandle()

	waitDone := make(chan struct{})
	go func() {
		code, err := h.Wait()
		if err != nil {
			t.Errorf("Wait error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		close(waitDone)
	}()

	// Close should unblock Wait.
	h.Close()

	select {
	case <-waitDone:
		// Success.
	case <-time.After(time.Second):
		t.Fatal("Wait should unblock after Close")
	}
}

func TestChannelHandle_ResizeAndWaitReady(t *testing.T) {
	t.Parallel()
	h := NewChannelHandle()
	defer h.Close()

	if err := h.Resize(50, 120); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestScriptedHandle_Replay(t *testing.T) {
	t.Parallel()

	chunks := []OutputChunk{
		{Text: "line 1\n"},
		{Text: "line 2\n", Delay: 10 * time.Millisecond},
		{Text: "line 3\n"},
	}

	h := NewScriptedHandle(chunks, nil)
	defer h.Close()

	// Receive all initial chunks in order.
	var received strings.Builder
	for i := 0; i < len(chunks); i++ {
		out, err := h.Receive()
		if err != nil {
			t.Fatalf("Receive %d: %v", i, err)
		}
		received.WriteString(out)
	}

	expected := "line 1\nline 2\nline 3\n"
	if received.String() != expected {
		t.Errorf("received = %q, want %q", received.String(), expected)
	}

	// After initial chunks, next Receive should block (no more output).
	recvCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := h.Receive()
		if err != nil {
			errCh <- err
			return
		}
		recvCh <- out
	}()

	select {
	case <-recvCh:
		t.Error("should not receive more output without input")
	case err := <-errCh:
		// EOF from Close is expected.
		if !errors.Is(err, io.EOF) {
			t.Logf("Receive after initial chunks: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		// Still blocking — expected behavior.
	}
}

func TestScriptedHandle_InputResponse(t *testing.T) {
	t.Parallel()

	initial := []OutputChunk{
		{Text: "ready\n"},
	}

	handler := func(input string) []OutputChunk {
		return []OutputChunk{
			{Text: "echo: " + input + "\n"},
		}
	}

	h := NewScriptedHandle(initial, handler)
	defer h.Close()

	// Receive initial output.
	out, err := h.Receive()
	if err != nil {
		t.Fatalf("Receive initial: %v", err)
	}
	if out != "ready\n" {
		t.Errorf("initial = %q, want %q", out, "ready\n")
	}

	// Send input.
	if err := h.Send("hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Receive response.
	out, err = h.Receive()
	if err != nil {
		t.Fatalf("Receive response: %v", err)
	}
	if out != "echo: hello\n" {
		t.Errorf("response = %q, want %q", out, "echo: hello\n")
	}
}

func TestScriptedHandle_Close(t *testing.T) {
	t.Parallel()

	chunks := []OutputChunk{
		{Text: "starting...\n", Delay: 5 * time.Minute}, // Long delay.
	}

	h := NewScriptedHandle(chunks, nil)

	if !h.IsAlive() {
		t.Fatal("should be alive initially")
	}

	// Close during replay should stop the goroutine.
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if h.IsAlive() {
		t.Error("should not be alive after close")
	}

	// Receive should return EOF.
	_, err := h.Receive()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF after close, got %v", err)
	}

	// Double close should be safe.
	if err := h.Close(); err != nil {
		t.Fatalf("double Close: %v", err)
	}
}

func TestScriptedHandle_NilHandler(t *testing.T) {
	t.Parallel()

	chunks := []OutputChunk{{Text: "init\n"}}
	h := NewScriptedHandle(chunks, nil)
	defer h.Close()

	// Receive initial.
	out, err := h.Receive()
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if out != "init\n" {
		t.Errorf("got %q, want %q", out, "init\n")
	}

	// Send with nil handler — input is consumed but produces no output.
	if err := h.Send("ignored"); err != nil {
		t.Fatalf("Send with nil handler: %v", err)
	}

	// No output should be produced. Verify with timeout.
	recvCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		out, err := h.Receive()
		if err != nil {
			errCh <- err
			return
		}
		recvCh <- out
	}()

	select {
	case <-recvCh:
		t.Error("nil handler should not produce output")
	case err := <-errCh:
		if !errors.Is(err, io.EOF) {
			t.Logf("Receive with nil handler: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		// Blocking as expected.
	}
}

func TestScriptedHandle_Wait(t *testing.T) {
	t.Parallel()

	h := NewScriptedHandle([]OutputChunk{{Text: "hi\n"}}, nil)

	waitDone := make(chan struct{})
	go func() {
		code, err := h.Wait()
		if err != nil {
			t.Errorf("Wait error: %v", err)
		}
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
		close(waitDone)
	}()

	h.Close()

	select {
	case <-waitDone:
		// Success.
	case <-time.After(time.Second):
		t.Fatal("Wait should unblock after Close")
	}
}

func TestScriptedHandle_ResizeAndWaitReady(t *testing.T) {
	t.Parallel()

	h := NewScriptedHandle(nil, nil)
	defer h.Close()

	if err := h.Resize(50, 120); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}
