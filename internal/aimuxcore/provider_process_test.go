package aimuxcore

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

func TestCaptureAgentHandle_RawOutputIsLosslessWithoutConsumer(t *testing.T) {
	t.Parallel()
	cs := termmux.NewCaptureSession(termmux.CaptureConfig{})
	h := &captureAgentHandle{cs: cs, eventsCh: make(chan LineEvent, 16), rawWake: make(chan struct{}, 1), ready: make(chan struct{})}
	src := make(chan []byte)
	done := make(chan struct{})
	go func() { h.forwardOutput(src); close(done) }()
	for i := range 2048 {
		src <- []byte(fmt.Sprintf("chunk-%d\\n", i))
	}
	close(src)
	<-done
	for i := range 2048 {
		got, err := h.Receive()
		if err != nil || got != fmt.Sprintf("chunk-%d\\n", i) {
			t.Fatalf("raw chunk %d = %q, %v", i, got, err)
		}
	}
}

func TestCaptureAgentHandle_EmptyRawChunkIsPreserved(t *testing.T) {
	t.Parallel()
	cs := termmux.NewCaptureSession(termmux.CaptureConfig{})
	h := &captureAgentHandle{
		cs:       cs,
		eventsCh: make(chan LineEvent, 4),
		rawWake:  make(chan struct{}, 1),
		ready:    make(chan struct{}),
	}
	src := make(chan []byte)
	done := make(chan struct{})
	go func() { h.forwardOutput(src); close(done) }()
	empty := []byte{}
	src <- empty
	close(src)
	<-done

	got, err := h.Receive()
	if err != nil {
		t.Fatalf("Receive returned error for empty chunk: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty chunk, got %q", got)
	}
}

func TestCaptureAgentHandle_WaitReady_DoesNotConsumeFirstChunk(t *testing.T) {
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

	chunk, err := h.Receive()
	if err != nil {
		t.Fatalf("receive failed: %v", err)
	}
	if !strings.Contains(chunk, "go version") {
		t.Fatalf("expected first chunk to contain 'go version', got %q", chunk)
	}
}
