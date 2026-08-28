package aimuxcore

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

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
