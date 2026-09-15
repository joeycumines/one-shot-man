package gateway

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func deadPID() func(int) bool { return func(int) bool { return false } }

func livePID() func(int) bool { return func(int) bool { return true } }

func TestDiscoveryRoundTripAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "gateway.discovery")
	written := NewDiscovery(4242, 11239, "127.0.0.1", map[string]string{"umans-shaper": "/umans", "zen-shaper": "/zen"}, "ephemeral-token", time.Unix(1700000000, 0))
	if err := WriteDiscovery(path, written, deadPID()); err != nil {
		t.Fatalf("WriteDiscovery: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode: got %o, want 600", mode)
	}

	read, err := ReadDiscovery(path)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if read.PID != written.PID || read.Port != written.Port || read.Token != written.Token {
		t.Fatalf("round trip: got %+v, want %+v", read, written)
	}
	if read.StartedAt != "2023-11-14T22:13:20Z" {
		t.Fatalf("started_at: got %q", read.StartedAt)
	}
	if len(read.Prefixes) != 2 || read.Prefixes["zen-shaper"] != "/zen" {
		t.Fatalf("prefixes: got %v", read.Prefixes)
	}
}

func TestDiscoveryRefusesALiveProcessAndTakesOverADeadOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.discovery")
	first := NewDiscovery(os.Getpid(), 11239, "127.0.0.1", map[string]string{"a": "/a"}, "token-one", time.Now())
	if err := WriteDiscovery(path, first, deadPID()); err != nil {
		t.Fatalf("WriteDiscovery: %v", err)
	}

	second := NewDiscovery(os.Getpid(), 11240, "127.0.0.1", map[string]string{"a": "/a"}, "token-two", time.Now())
	if err := WriteDiscovery(path, second, livePID()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("WriteDiscovery over a live advertisement: got %v, want ErrAlreadyRunning", err)
	}
	// The refused write must not have disturbed the advertisement.
	still, err := ReadDiscovery(path)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if still.Token != "token-one" {
		t.Fatalf("token after a refused write: got %q, want token-one", still.Token)
	}

	if err := WriteDiscovery(path, second, deadPID()); err != nil {
		t.Fatalf("WriteDiscovery over a dead advertisement: %v", err)
	}
	taken, err := ReadDiscovery(path)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if taken.Token != "token-two" || taken.Port != 11240 {
		t.Fatalf("takeover: got %+v, want the second advertisement", taken)
	}
}

func TestDiscoveryRejectsUnsupportedDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.discovery")

	unsupported := NewDiscovery(1, 11239, "127.0.0.1", nil, "token", time.Now())
	unsupported.Version = DiscoveryVersion + 1
	if err := WriteDiscovery(path, unsupported, deadPID()); err == nil {
		t.Fatal("WriteDiscovery: want an error for an unsupported version")
	}

	if err := os.WriteFile(path, []byte(`{"version":99,"pid":1,"token":"t"}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := ReadDiscovery(path); err == nil {
		t.Fatal("ReadDiscovery: want an error for an unsupported version")
	}

	if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// An unreadable document cannot be attributed to a live process, so the next
	// start takes it over rather than failing.
	fresh := NewDiscovery(1, 11239, "127.0.0.1", map[string]string{"a": "/a"}, "recovered", time.Now())
	if err := WriteDiscovery(path, fresh, livePID()); err != nil {
		t.Fatalf("WriteDiscovery over an unreadable document: %v", err)
	}
	read, err := ReadDiscovery(path)
	if err != nil {
		t.Fatalf("ReadDiscovery: %v", err)
	}
	if read.Token != "recovered" {
		t.Fatalf("token: got %q, want recovered", read.Token)
	}
}

func TestDiscoveryRemovalIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.discovery")
	if err := RemoveDiscovery(path); err != nil {
		t.Fatalf("RemoveDiscovery on a missing file: %v", err)
	}
	if err := WriteDiscovery(path, NewDiscovery(1, 11239, "127.0.0.1", map[string]string{"a": "/a"}, "t", time.Now()), deadPID()); err != nil {
		t.Fatalf("WriteDiscovery: %v", err)
	}
	if err := RemoveDiscovery(path); err != nil {
		t.Fatalf("RemoveDiscovery: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat after removal: got %v, want not-exist", err)
	}
}

func TestDiscoveryDocumentShape(t *testing.T) {
	// The launcher reads these exact keys, so the encoding is pinned here.
	encoded, err := json.Marshal(NewDiscovery(7, 11239, "127.0.0.1", map[string]string{"a": "/a"}, "t", time.Unix(0, 0)))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"version", "pid", "host", "port", "prefixes", "token", "started_at"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("the discovery document is missing %q", key)
		}
	}
	if len(fields) != 7 {
		t.Errorf("the discovery document has %d fields, want exactly the seven the spec names", len(fields))
	}
}
