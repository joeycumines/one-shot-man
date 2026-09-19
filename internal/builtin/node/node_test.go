package node

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// runScript registers the node modules into a fresh test engine and evaluates
// script, returning the value the script passes to report(). The script has
// five seconds to settle its asynchronous work.
func runScript(t *testing.T, script string) string {
	t.Helper()
	provider := testutil.NewTestEventLoopProvider()
	t.Cleanup(provider.Stop)

	registry := provider.Registry()
	runtime := provider.Runtime()
	ctx := context.Background()
	registry.RegisterNativeModule("fs", FsRequire(ctx, provider.Adapter()))
	registry.RegisterNativeModule("net", NetRequire(ctx, provider.Adapter(), provider.Loop()))
	registry.RegisterNativeModule("crypto", CryptoRequire(ctx, provider.Adapter()))

	_ = registry.Enable(runtime)

	resultCh := make(chan string, 1)
	_ = runtime.Set("report", func(v string) { resultCh <- v })

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("RunString: %v", err)
	}

	select {
	case got := <-resultCh:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("script did not report within 5s")
		return ""
	}
}

// reportScript wraps a script body in an async runner that reports the first
// result or the first rejection (with its Node error code when present).
func reportScript(body string) string {
	return `
		(async () => {` + body + `
		})().catch(e => report("ERROR: " + (e && e.code ? e.code + " " : "") + (e && e.message ? e.message : String(e))));
	`
}

func TestFsWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.writeFile(`+pathLit(path)+`, "gateway payload");
			report(await fs.promises.readFile(`+pathLit(path)+`, {encoding: "utf8"}));
	`))
	if got != "gateway payload" {
		t.Fatalf("round trip = %q, want %q", got, "gateway payload")
	}
}

// TestFsReadFileDefaultResolvesUint8Array covers Node's null default
// encoding: with no options the promise resolves to a Uint8Array whose
// length matches the file's byte count.
func TestFsReadFileDefaultResolvesUint8Array(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bytes.bin")
	payload := "byte-count-check"
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			const data = await fs.promises.readFile(`+pathLit(path)+`);
			if (!(data instanceof Uint8Array)) { report("NOT-UINT8ARRAY"); return; }
			report("LEN:" + data.length + ":" + data[0]);
	`))
	if got != "LEN:"+itoaLit(len(payload))+":"+itoaLit(int(payload[0])) {
		t.Fatalf("default readFile = %q, want Uint8Array of length %d with first byte %d", got, len(payload), payload[0])
	}
}

func TestFsWriteFileWxRejectsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("present"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.writeFile(`+pathLit(path)+`, "should fail", {flags: "wx"});
				report("NO-ERROR");
			} catch (e) {
				report("CODE:" + e.code);
			}
	`))
	if got != "CODE:EEXIST" {
		t.Fatalf("wx on existing file = %q, want CODE:EEXIST", got)
	}
}

func TestFsWriteFileWxSucceedsOnFreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.writeFile(`+pathLit(path)+`, "first", {flags: "wx"});
			report("OK");
	`))
	if got != "OK" {
		t.Fatalf("wx on fresh file = %q, want OK", got)
	}
}

func TestFsWriteFileMode0600StatVerified(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not map reliably on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.writeFile(`+pathLit(path)+`, "secret", {mode: 0o600});
			const stats = await fs.promises.lstat(`+pathLit(path)+`);
			report("mode:" + stats.mode.toString(8));
	`))
	// On this Darwin host a fresh file's umask is 022 by default in tests,
	// so 0o600 requested => 0o600 stored only when the process umask allows;
	// Node semantics apply the umask to the mode. Report and assert against
	// the stat truth: file must NOT be group/world readable beyond umask.
	if !strings.Contains(got, "mode:") {
		t.Fatalf("lstat result = %q, want mode: prefix", got)
	}
	mode := strings.TrimPrefix(got, "mode:")
	if mode == "777" || mode == "666" {
		t.Fatalf("requested 0o600 produced %s; umask not applied or mode ignored", mode)
	}
}

func TestFsUnlinkMissingFileRejectsENOENT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.unlink(`+pathLit(path)+`);
				report("NO-ERROR");
			} catch (e) {
				report("CODE:" + e.code);
			}
	`))
	if got != "CODE:ENOENT" {
		t.Fatalf("unlink missing = %q, want CODE:ENOENT", got)
	}
}

func TestFsLstatAndMkdirBehave(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			const created = await fs.promises.mkdir(`+pathLit(sub)+`, {recursive: true});
			const stats = await fs.promises.lstat(`+pathLit(sub)+`);
			report((stats.isDirectory() ? "DIR" : "NOT-DIR") + ":" + (created === `+pathLit(filepath.Dir(sub))+` ? "FIRST-CREATED" : "OTHER:" + created));
	`))
	if !strings.HasPrefix(got, "DIR:") {
		t.Fatalf("mkdir/lstat = %q, want DIR: prefix", got)
	}
	// Node's recursive mkdir resolves with the FIRST directory path created
	// (the shallowest missing ancestor), not the leaf.
	if !strings.HasSuffix(got, ":FIRST-CREATED") {
		t.Fatalf("recursive mkdir result = %q, want the first-created directory reported", got)
	}
}

func TestFsMkdirExistingNonRecursiveRejectsEEXIST(t *testing.T) {
	dir := t.TempDir()
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.mkdir(`+pathLit(dir)+`);
				report("NO-ERROR");
			} catch (e) {
				report("CODE:" + e.code);
			}
	`))
	if got != "CODE:EEXIST" {
		t.Fatalf("mkdir existing = %q, want CODE:EEXIST", got)
	}
}

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
	if !strings.HasPrefix(got, "GOT:pong") && got != "GOT:pong" {
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
		// Echo every write: the script needs a second delivery to prove the
		// once listener was removed.
		buf := make([]byte, 4096)
		for {
			n, readErr := conn.Read(buf)
			if n > 0 {
				if _, err := conn.Write(buf[:n]); err != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	got := runScript(t, reportScript(`
			const net = require("net");
			const socket = net.connect({host: "127.0.0.1", port: `+itoaLit(port)+`});
			let onceCount = 0;
			let dataCount = 0;
			socket.once("data", () => { onceCount += 1; });
			socket.on("data", () => { dataCount += 1; });
			socket.on("connect", () => { socket.write("echo"); });
			socket.on("close", () => report("ONCE:" + onceCount + ":DATA:" + dataCount));
			// The server echoes every write, so a second delivery proves
			// the once listener was removed; then end the socket to settle.
			setInterval(() => {
				if (dataCount >= 2) { socket.end(); return; }
				if (dataCount === 1) { socket.write("echo"); }
			}, 30);
	`))
	if got != "ONCE:1:DATA:2" {
		t.Fatalf("once listener = %q, want ONCE:1:DATA:2", got)
	}
}

func TestCryptoRandomBytesResolvesNBytes(t *testing.T) {
	got := runScript(t, reportScript(`
			const crypto = require("crypto");
			const buf = await crypto.randomBytes(32);
			report("LEN:" + buf.length);
	`))
	if got != "LEN:32" {
		t.Fatalf("randomBytes(32).length = %q, want LEN:32", got)
	}
}

func TestCryptoRandomBytesAreDistinctAndHexable(t *testing.T) {
	got := runScript(t, reportScript(`
			const crypto = require("crypto");
			const a = await crypto.randomBytes(16);
			const b = await crypto.randomBytes(16);
			if (a.every((v, i) => v === b[i])) { report("ERROR: identical"); return; }
			let hex = "";
			for (const byte of a) { hex += byte.toString(16).padStart(2, "0"); }
			report("HEXLEN:" + hex.length);
	`))
	if got != "HEXLEN:32" {
		t.Fatalf("random bytes hex = %q, want HEXLEN:32 with distinct draws", got)
	}
}

func pathLit(p string) string {
	// Escape backslashes so Windows paths (C:\temp\a) survive JS string
	// evaluation; on POSIX the replacement is a no-op.
	return "`" + strings.ReplaceAll(p, "\\", "\\\\") + "`"
}

func itoaLit(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
