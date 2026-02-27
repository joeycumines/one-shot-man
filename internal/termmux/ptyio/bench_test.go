package ptyio

import (
	"context"
	"io"
	"testing"
)

// ── T088: Benchmark BufferedReader throughput ───────────────────────

func BenchmarkBufferedReader(b *testing.B) {
	// Create a pipe to simulate PTY (in-memory for benchmarking).
	pr, pw := io.Pipe()
	br := NewBufferedReader(pr, 16)
	ctx, cancel := context.WithCancel(context.Background())
	go br.ReadLoop(ctx)

	data := make([]byte, 32*1024) // 32KB chunks matching default buffer.
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	// Writer goroutine.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < b.N; i++ {
			if _, err := pw.Write(data); err != nil {
				return
			}
		}
		pw.Close()
	}()

	// Drain output channel.
	for range br.Output() {
	}
	<-done
	cancel()
}
