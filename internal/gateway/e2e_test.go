package gateway

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// stubShaper writes a shell script that records the environment it was started
// with and then occupies the bind port, so readiness behaves like the real
// shaper's without any upstream involvement.
func stubShaper(t *testing.T, dir string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is unavailable, so a stub shaper cannot occupy the port")
	}
	environmentPath := filepath.Join(dir, "child-environment.txt")
	path := filepath.Join(dir, "stub-shaper")
	script := "#!/bin/sh\n" +
		"env > " + environmentPath + "\n" +
		"port=\"\"\n" +
		"for arg in \"$@\"; do case \"$arg\" in -bind=*:*) port=\"${arg##*:}\";; esac; done\n" +
		"[ -n \"$port\" ] || exit 3\n" +
		"exec python3 -c 'import socket,sys\n" +
		"s=socket.socket()\n" +
		"s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)\n" +
		"s.bind((\"127.0.0.1\", int(sys.argv[1])))\n" +
		"s.listen(8)\n" +
		"while True:\n" +
		"    conn,_ = s.accept()\n" +
		"    conn.close()' \"$port\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing the stub shaper: %v", err)
	}
	return path, environmentPath
}

// freePort asks the kernel for a port the stub can bind, by binding and closing
// one, so the test never collides with a real shaper.
func freePort(t *testing.T) int {
	t.Helper()
	listener, port := listen(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return port
}

func TestGatewayAgainstAStubShaper(t *testing.T) {
	dir := t.TempDir()
	stub, environmentPath := stubShaper(t, dir)
	mounts := umansMounts()
	credentials := map[string]string{"UMANS_API_KEY": "stub-secret-value"}

	runOnce := func() (Discovery, string, int) {
		port := freePort(t)
		discoveryPath := filepath.Join(dir, "run-"+strconv.Itoa(port)+".discovery")
		cfg := Config{
			ShaperBinary:  stub,
			ShaperArgs:    []string{"-bind=127.0.0.1:" + strconv.Itoa(port)},
			Host:          "127.0.0.1",
			Port:          port,
			DiscoveryPath: discoveryPath,
			ReadyTimeout:  10 * time.Second,
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		var code int
		var runErr error
		go func() {
			code, runErr = Run(ctx, cfg, mounts, credentials, DefaultDependencies(cfg))
			close(done)
		}()

		waitForFile(t, discoveryPath)
		info, err := os.Stat(discoveryPath)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("discovery mode: got %o, want 600", mode)
		}
		read, err := ReadDiscovery(discoveryPath)
		if err != nil {
			t.Fatalf("ReadDiscovery: %v", err)
		}

		// Cancelling is the SIGTERM path: the advertisement must go and the
		// child must be reaped.
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Run did not return after cancellation")
		}
		if runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
		if _, err := os.Stat(discoveryPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("the advertisement survived shutdown: %v", err)
		}
		return read, discoveryPath, code
	}

	first, firstPath, _ := runOnce()
	second, secondPath, _ := runOnce()

	if first.Version != DiscoveryVersion || first.Host != "127.0.0.1" || first.PID == 0 {
		t.Fatalf("discovery fields: got %+v", first)
	}
	if first.Prefixes["umans-shaper"] != "/umans" {
		t.Fatalf("prefixes: got %v", first.Prefixes)
	}
	if first.Token == second.Token {
		t.Fatal("two runs produced the same token, want a per-start token")
	}
	if first.Port == second.Port {
		t.Log("note: the two runs happened to reuse a port, which is not a failure")
	}

	// The child saw the credential under the shaper's own spelling, and no file
	// the gateway produced carries the value.
	environment, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatalf("reading the child environment: %v", err)
	}
	if !strings.Contains(string(environment), "SHAPER_PROVIDER_UMANS_API_KEY=stub-secret-value") {
		t.Fatalf("the child environment does not carry the shaper credential: %s", environment)
	}
	for _, path := range []string{firstPath, secondPath, environmentPath} {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(data), "stub-secret-value") && path != environmentPath {
			t.Fatalf("%s carries the credential value", path)
		}
	}
}
