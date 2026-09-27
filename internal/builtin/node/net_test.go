package node

import (
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

func TestNetConnectFiresConnectAndDeliversBytes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		accepted <- string(buf[:n])
		_, _ = conn.Write([]byte("pong"))
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			let received = "";
			socket.on("connect", () => socket.write("ping"));
			socket.on("data", (chunk) => { received += chunk; socket.end(); report("GOT:" + received); });
			socket.on("error", (e) => report("ERROR: " + e.message));
	`))
	if !strings.HasPrefix(got, "GOT:pong") {
		t.Fatalf("net exchange = %q, want GOT:pong", got)
	}
	select {
	case written := <-accepted:
		if written != "ping" {
			t.Fatalf("server received %q, want ping", written)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never received the client's bytes")
	}
}

func TestNetWriteAfterFailedDialIsRejected(t *testing.T) {
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: 0});
			socket.on("error", () => {});
			socket.on("close", () => {
				try {
					socket.write("late");
					report("WRITE-ACCEPTED");
				} catch (err) {
					report("WRITE-REJECTED");
				}
			});
	`))
	if got != "WRITE-REJECTED" {
		t.Fatalf("write after failed dial = %q, want WRITE-REJECTED", got)
	}
}

func TestNetConnectionErrorExposesNodeFields(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const socket = require("net").connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			socket.on("error", (err) => report([err.code, err.syscall, err.address, err.port].join(":")));
	`))
	want := "ECONNREFUSED:connect:127.0.0.1:" + strconv.Itoa(port)
	if got != want {
		t.Fatalf("connection error fields = %q, want %q", got, want)
	}
}

func TestNodeNetErrorMapsDNSLookupFailures(t *testing.T) {
	tests := []struct {
		name     string
		err      *net.DNSError
		wantCode string
		wantSys  string
		wantHost string
	}{
		{
			name:     "not found",
			err:      &net.DNSError{Err: "no such host", Name: "missing.invalid", IsNotFound: true},
			wantCode: "ENOTFOUND",
			wantSys:  "getaddrinfo",
			wantHost: "missing.invalid",
		},
		{
			name:     "temporary failure",
			err:      &net.DNSError{Err: "temporary failure", Name: "slow.invalid", IsTemporary: true},
			wantCode: "EAI_AGAIN",
			wantSys:  "getaddrinfo",
			wantHost: "slow.invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := goja.New()
			got := nodeNetError(rt, tt.err)
			if code := got.Get("code").String(); code != tt.wantCode {
				t.Fatalf("error code = %q, want %q", code, tt.wantCode)
			}
			if syscall := got.Get("syscall").String(); syscall != tt.wantSys {
				t.Fatalf("error syscall = %q, want %q", syscall, tt.wantSys)
			}
			if hostname := got.Get("hostname").String(); hostname != tt.wantHost {
				t.Fatalf("error hostname = %q, want %q", hostname, tt.wantHost)
			}
		})
	}
}

// TestNetWriteBeforeConnectFlushesAfterConnect covers Node's buffered-write
// semantics: bytes written before the "connect" event fires must reach the
// server, not drop. No connect handler is registered; the write happens
// synchronously right after net.connect returns.
func TestNetWriteBeforeConnectFlushesAfterConnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		accepted <- string(buf[:n])
		_, _ = conn.Write([]byte("ack"))
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			// No connect handler: the write is buffered pre-connect.
			socket.write("early-bird");
			let received = "";
			socket.on("data", (chunk) => { received += chunk; socket.end(); report("GOT:" + received); });
			socket.on("error", (e) => report("ERROR: " + e.message));
	`))
	if !strings.HasPrefix(got, "GOT:ack") {
		t.Fatalf("net exchange = %q, want GOT:ack", got)
	}
	select {
	case written := <-accepted:
		if written != "early-bird" {
			t.Fatalf("server received %q, want the pre-connect write flushed", written)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server never received the pre-connect bytes")
	}
}

func TestNetEndBeforeConnectHalfClosesAndReceivesResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	request := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			request <- "accept error: " + err.Error()
			return
		}
		defer conn.Close()
		payload, err := io.ReadAll(conn)
		if err != nil {
			request <- "read error: " + err.Error()
			return
		}
		request <- string(payload)
		_, _ = conn.Write([]byte("response"))
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			socket.write("request");
			socket.end();
			let received = "";
			socket.on("data", (chunk) => { received += chunk; });
			socket.on("close", () => report("RESPONSE:" + received));
			socket.on("error", (e) => report("ERROR:" + e.message));
	`))
	if got != "RESPONSE:response" {
		t.Fatalf("half-close exchange = %q, want RESPONSE:response", got)
	}
	select {
	case got := <-request:
		if got != "request" {
			t.Fatalf("server received %q, want request", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive the request before the client's half-close")
	}
}

func TestNetDestroyClosesAndJoinsSocketWorkers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			socket.on("connect", () => {
				socket.write("x".repeat(1024 * 1024));
				socket.destroy();
			});
			socket.on("close", () => report("CLOSED"));
			socket.on("error", (e) => report("ERROR:" + e.message));
	`))
	if got != "CLOSED" {
		t.Fatalf("destroy result = %q, want CLOSED", got)
	}
	select {
	case <-serverDone:
	case <-time.After(5 * time.Second):
		t.Fatal("server stayed blocked after socket destroy")
	}
}

// TestNetWriteOrderingIsPreserved covers the end-to-end contract over a real
// socket: writes issued before the connection establishes are buffered and
// flushed on connect, and post-connect writes follow, so the server receives
// every write exactly once and in issue order. (The deterministic coverage of
// the connect race itself is TestSocketWriteQueueDoesNotBlockEnqueue,
// which fails on the old unlocked-flush shape.)
func TestNetWriteOrderingIsPreserved(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	// The server reads until the terminator and reports the raw wire order.
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Delay the read so the pre-connect flush blocks on the socket
		// buffer; that is the window in which a post-connect write used to
		// reach the wire ahead of the buffered bytes.
		time.Sleep(300 * time.Millisecond)
		buf := make([]byte, 0, 1<<20)
		tmp := make([]byte, 4096)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, readErr := conn.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if strings.Contains(string(buf), "\n") {
				break
			}
			if readErr != nil {
				break
			}
		}
		_, _ = conn.Write([]byte("ack"))
		received <- string(buf)
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			let issued = "";
			const w = (chunk) => { issued += chunk; socket.write(chunk); };
			// A pre-connect burst larger than the socket buffer, so the flush
			// blocks until the server reads ...
			for (let i = 0; i < 64; i++) { w("a".repeat(4096)); }
			// ... while timer writes straddle the connect.
			let ticks = 0;
			const timer = setInterval(() => {
				w("b");
				if (++ticks >= 60) { clearInterval(timer); setTimeout(() => { socket.write("\n"); }, 150); }
			}, 0);
			socket.on("connect", () => { w("c"); w("c"); w("c"); });
			socket.on("data", (chunk) => { socket.end(); report("ISSUED:" + issued); });
			socket.on("error", (e) => report("ERROR: " + e.message));
	`))
	if !strings.HasPrefix(got, "ISSUED:") {
		t.Fatalf("net exchange = %q, want ISSUED:", got)
	}
	issued := strings.TrimPrefix(got, "ISSUED:")
	select {
	case wire := <-received:
		wire = strings.TrimSuffix(wire, "\n")
		if wire != issued {
			t.Fatalf("wire order differs from issue order (wire %d bytes, issued %d bytes): first difference at %d",
				len(wire), len(issued), firstDifference(wire, issued))
		}
	case <-time.After(11 * time.Second):
		t.Fatal("server never received the writes")
	}
}

// firstDifference returns the index of the first differing byte, or the length
// of the shorter string when one is a prefix of the other.
func firstDifference(a, b string) int {
	n := min(len(b), len(a))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// TestNetOnceListenerFiresExactlyOnce verifies real once semantics: a once
// listener runs on its first event and is removed, so a second emission
// does not re-run it.
func TestNetOnceListenerFiresExactlyOnce(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err := conn.Write([]byte("first")); err != nil {
			return
		}
		ack := make([]byte, 1)
		if _, err := io.ReadFull(conn, ack); err != nil {
			return
		}
		if _, err := conn.Write([]byte("second")); err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, conn)
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			let onceCount = 0;
			let dataCount = 0;
			socket.once("data", () => { onceCount += 1; });
			socket.on("data", () => {
				if (++dataCount === 1) { socket.write("a"); }
				else { socket.end(); }
			});
			socket.on("close", () => report("ONCE:" + onceCount + ":DATA:" + dataCount));
	`))
	if got != "ONCE:1:DATA:2" {
		t.Fatalf("once listener = %q, want ONCE:1:DATA:2", got)
	}
}

func TestNetListenerAddedDuringEmitReceivesNextEvent(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		if _, err := conn.Write([]byte("first")); err != nil {
			serverDone <- err
			return
		}
		ack := make([]byte, 1)
		if _, err := io.ReadFull(conn, ack); err != nil {
			serverDone <- err
			return
		}
		_, err = conn.Write([]byte("second"))
		serverDone <- err
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const socket = require("net").connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			let eventCount = 0;
			let addedCount = 0;
			socket.on("data", () => {
				if (++eventCount === 1) {
					socket.on("data", () => { addedCount++; });
					socket.write("a");
				} else {
					socket.end();
				}
			});
			socket.on("close", () => report("EVENTS:" + eventCount + ":ADDED:" + addedCount));
			socket.on("error", (err) => report("ERROR:" + err.code));
	`))
	if got != "EVENTS:2:ADDED:1" {
		t.Fatalf("listener added during emit = %q, want EVENTS:2:ADDED:1", got)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server exchange: %v", err)
	}
}
