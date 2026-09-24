package node

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// blockingConn blocks its first Write until released, so a test can hold a
// flush open while a concurrent write is issued.
type blockingConn struct {
	mu      sync.Mutex
	writes  []string
	release chan struct{}
	blocked bool
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

func (c *blockingConn) recorded() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.writes...)
}

func (c *blockingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *blockingConn) Close() error                     { return nil }
func (c *blockingConn) LocalAddr() net.Addr              { return nil }
func (c *blockingConn) RemoteAddr() net.Addr             { return nil }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

// TestSocketWriteQueueSerializesAcrossConnect pins the connect-race invariant:
// writes issued before the connection establishes are flushed in order, and a
// write issued while that flush is in progress waits for it rather than
// overtaking the buffered bytes or being dropped.
func TestSocketWriteQueueSerializesAcrossConnect(t *testing.T) {
	conn := &blockingConn{release: make(chan struct{})}
	queue := &socketWriteQueue{}
	if err := queue.write("a1"); err != nil {
		t.Fatal(err)
	}
	if err := queue.write("a2"); err != nil {
		t.Fatal(err)
	}

	flushed := make(chan error, 1)
	go func() { flushed <- queue.setConn(conn) }()

	// Wait until setConn is inside the first Write, then issue a write.
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
	written := make(chan error, 1)
	go func() { written <- queue.write("b") }()

	// The concurrent write must not reach the connection while the flush holds
	// the queue's lock.
	time.Sleep(50 * time.Millisecond)
	if got := conn.recorded(); len(got) != 0 {
		t.Fatalf("concurrent write reached the connection during the flush: %v", got)
	}

	close(conn.release)
	if err := <-flushed; err != nil {
		t.Fatalf("setConn: %v", err)
	}
	if err := <-written; err != nil {
		t.Fatalf("write: %v", err)
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
}

// TestSocketWriteQueueBuffersUntilConnect covers the plain case: writes before
// setConn are buffered (nothing reaches the connection) and flush in order.
func TestSocketWriteQueueBuffersUntilConnect(t *testing.T) {
	queue := &socketWriteQueue{}
	conn := &blockingConn{release: make(chan struct{})}
	close(conn.release)
	for _, data := range []string{"one", "two", "three"} {
		if err := queue.write(data); err != nil {
			t.Fatal(err)
		}
	}
	if got := conn.recorded(); len(got) != 0 {
		t.Fatalf("pre-connect writes reached the connection: %v", got)
	}
	if err := queue.setConn(conn); err != nil {
		t.Fatal(err)
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
	if err := queue.write("four"); err != nil {
		t.Fatal(err)
	}
	if got := conn.recorded(); len(got) != 4 || got[3] != "four" {
		t.Fatalf("post-connect write = %v, want the append", got)
	}
}
