package ptyio

import (
	"context"
	"io"
	"runtime"
	"testing"
	"time"
)

func TestBufferedReader_ReadsChunks(t *testing.T) {
	t.Parallel()
	pr, pw := io.Pipe()
	br := NewBufferedReader(pr, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go br.ReadLoop(ctx)

	pw.Write([]byte("Hello"))
	chunk := <-br.Output()
	if string(chunk) != "Hello" {
		t.Fatalf("got %q; want %q", chunk, "Hello")
	}
	pw.Close()
	<-br.done
}

func TestBufferedReader_DoneOnClose(t *testing.T) {
	t.Parallel()
	pr, pw := io.Pipe()
	br := NewBufferedReader(pr, 1)
	go br.ReadLoop(context.Background())
	pw.Close()
	select {
	case <-br.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Done() not closed after pipe close")
	}
}

func TestBufferedReader_ContextCancel(t *testing.T) {
	t.Parallel()
	pr, _ := io.Pipe()
	br := NewBufferedReader(pr, 1)
	ctx, cancel := context.WithCancel(context.Background())

	go br.ReadLoop(ctx)
	cancel()
	// The reader should eventually exit (after its current blocking Read returns).
	// Since io.Pipe doesn't unblock on context cancel, close the reader.
	pr.Close()
	select {
	case <-br.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Done() not closed after cancel")
	}
}

func TestBufferedReader_NoGoroutineLeak(t *testing.T) {
	t.Parallel()
	before := runtime.NumGoroutine()
	pr, pw := io.Pipe()
	br := NewBufferedReader(pr, 4)
	go br.ReadLoop(context.Background())

	pw.Write([]byte("test"))
	<-br.Output()
	pw.Close()
	<-br.done

	// Allow goroutines to settle.
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("possible goroutine leak: before=%d, after=%d", before, after)
	}
}

func TestBufferedReader_Backpressure(t *testing.T) {
	t.Parallel()
	pr, pw := io.Pipe()
	// Channel capacity of 1 — second write should block.
	br := NewBufferedReader(pr, 1)
	go br.ReadLoop(context.Background())

	// First write fills the channel.
	pw.Write([]byte("first"))
	// Give reader time to fill channel.
	time.Sleep(50 * time.Millisecond)

	// Second write should cause reader to block on channel send.
	pw.Write([]byte("second"))
	time.Sleep(50 * time.Millisecond)

	// Drain the channel.
	chunk1 := <-br.Output()
	chunk2 := <-br.Output()
	if string(chunk1) != "first" {
		t.Errorf("chunk1 = %q; want %q", chunk1, "first")
	}
	if string(chunk2) != "second" {
		t.Errorf("chunk2 = %q; want %q", chunk2, "second")
	}

	pw.Close()
	<-br.done
}
