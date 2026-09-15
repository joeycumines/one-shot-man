package gateway

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func listen(t *testing.T) (net.Listener, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	return listener, listener.Addr().(*net.TCPAddr).Port
}

func TestProbeDistinguishesListeningFromRefused(t *testing.T) {
	listener, port := listen(t)
	if err := Probe(context.Background(), "127.0.0.1", port, time.Second); err != nil {
		t.Fatalf("Probe against a listener: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := Probe(context.Background(), "127.0.0.1", port, 250*time.Millisecond); err == nil {
		t.Fatal("Probe against a closed port: want an error, so a caller cannot read a refused connection as ready")
	}
}

func TestProbeRejectsAnUnusablePort(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		if err := Probe(context.Background(), "127.0.0.1", port, time.Second); err == nil {
			t.Fatalf("Probe(%d): want an error", port)
		}
	}
}

func TestWaitForPollsUntilThePortAppears(t *testing.T) {
	listener, port := listen(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Bind the same port again shortly after the wait begins, which only a
	// polling wait can observe.
	ready := make(chan net.Listener, 1)
	go func() {
		time.Sleep(300 * time.Millisecond)
		rebound, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			close(ready)
			return
		}
		ready <- rebound
	}()

	if err := WaitFor(context.Background(), "127.0.0.1", port, time.Now().Add(5*time.Second), 50*time.Millisecond); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if rebound, ok := <-ready; ok && rebound != nil {
		rebound.Close()
	}
}

func TestWaitForReportsTheAddressAndTheLastFailure(t *testing.T) {
	listener, port := listen(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := WaitFor(context.Background(), "127.0.0.1", port, time.Now().Add(200*time.Millisecond), 50*time.Millisecond)
	if err == nil {
		t.Fatal("WaitFor: want a timeout error")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:"+strconv.Itoa(port)) {
		t.Fatalf("error: got %v, want it to name the address", err)
	}
	if !strings.Contains(err.Error(), "without a connection") {
		t.Fatalf("error: got %v, want it to say readiness never arrived", err)
	}
}

func TestWaitForHonoursCancellation(t *testing.T) {
	listener, port := listen(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := WaitFor(ctx, "127.0.0.1", port, time.Now().Add(10*time.Second), 50*time.Millisecond)
	if err == nil {
		t.Fatal("WaitFor: want a cancellation error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %s, want it honoured promptly", elapsed)
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error: got %v, want it to report cancellation", err)
	}
}
