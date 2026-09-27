package node

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// blockingConn blocks its first Write until released, so a test can hold a
// flush open while a concurrent write is issued.
type blockingConn struct {
	mu          sync.Mutex
	writes      []string
	release     chan struct{}
	blocked     bool
	halfClosed  bool
	releaseOnce sync.Once
}

func (c *blockingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	block := !c.blocked
	c.blocked = true
	c.mu.Unlock()
	if block {
		<-c.release
	}
	c.mu.Lock()
	c.writes = append(c.writes, string(p))
	c.mu.Unlock()
	return len(p), nil
}

func (c *blockingConn) CloseWrite() error {
	c.mu.Lock()
	c.halfClosed = true
	c.mu.Unlock()
	return nil
}

func (c *blockingConn) Close() error {
	c.releaseOnce.Do(func() { close(c.release) })
	return nil
}

func (c *blockingConn) recorded() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.writes...)
}

func (c *blockingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *blockingConn) LocalAddr() net.Addr              { return nil }
func (c *blockingConn) RemoteAddr() net.Addr             { return nil }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

// TestSocketWriteQueueDoesNotBlockEnqueue preserves FIFO order while proving
// callers can enqueue writes during a blocked network write.
func TestSocketWriteQueueDoesNotBlockEnqueue(t *testing.T) {
	conn := &blockingConn{release: make(chan struct{})}
	defer conn.Close()
	queue := newSocketWriteQueue()
	if err := queue.write("a1"); err != nil {
		t.Fatal(err)
	}
	if err := queue.write("a2"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	writerDone := make(chan error, 1)
	go func() { writerDone <- queue.run(ctx) }()
	if err := queue.setConn(conn); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn.mu.Lock()
		blocked := conn.blocked
		conn.mu.Unlock()
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("setConn never reached the first write")
		}
		time.Sleep(time.Millisecond)
	}

	enqueued := make(chan error, 1)
	go func() {
		if err := queue.write("b"); err != nil {
			enqueued <- err
			return
		}
		enqueued <- queue.end("")
	}()
	select {
	case err := <-enqueued:
		if err != nil {
			t.Fatalf("enqueue while Write is blocked: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("enqueue waited for the blocked network write")
	}
	if got := conn.recorded(); len(got) != 0 {
		t.Fatalf("later write reached the connection during the blocked write: %v", got)
	}

	conn.releaseOnce.Do(func() { close(conn.release) })
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("writer: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish after the blocked Write was released")
	}

	got := conn.recorded()
	want := []string{"a1", "a2", "b"}
	if len(got) != len(want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("writes = %v, want %v", got, want)
		}
	}
	conn.mu.Lock()
	halfClosed := conn.halfClosed
	conn.mu.Unlock()
	if !halfClosed {
		t.Fatal("writer did not half-close after the queued end")
	}
}

func TestSocketWriteQueueBuffersUntilConnect(t *testing.T) {
	queue := newSocketWriteQueue()
	conn := &blockingConn{release: make(chan struct{})}
	conn.releaseOnce.Do(func() { close(conn.release) })
	defer conn.Close()
	for _, data := range []string{"one", "two", "three"} {
		if err := queue.write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := queue.end(""); err != nil {
		t.Fatal(err)
	}
	if got := conn.recorded(); len(got) != 0 {
		t.Fatalf("pre-connect writes reached the connection: %v", got)
	}
	writerDone := make(chan error, 1)
	go func() { writerDone <- queue.run(context.Background()) }()
	if err := queue.setConn(conn); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("writer: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish the pre-connect queue")
	}
	got := conn.recorded()
	want := []string{"one", "two", "three"}
	if len(got) != len(want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("writes = %v, want %v", got, want)
		}
	}
	conn.mu.Lock()
	halfClosed := conn.halfClosed
	conn.mu.Unlock()
	if !halfClosed {
		t.Fatal("pre-connect end did not half-close after buffered writes")
	}
}

func TestSocketWriteQueueCancellationUnblocksWriter(t *testing.T) {
	conn := &blockingConn{release: make(chan struct{})}
	defer conn.Close()
	queue := newSocketWriteQueue()
	if err := queue.write("blocked"); err != nil {
		t.Fatal(err)
	}
	if err := queue.setConn(conn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	writerDone := make(chan error, 1)
	go func() { writerDone <- queue.run(ctx) }()
	t.Cleanup(cancel)

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn.mu.Lock()
		blocked := conn.blocked
		conn.mu.Unlock()
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("writer never reached the blocked Write")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	_ = conn.Close()
	select {
	case err := <-writerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("writer cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not stop after connection close")
	}
}
