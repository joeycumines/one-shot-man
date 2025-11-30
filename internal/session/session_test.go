package session

import (
	"os"
	"strings"
	"testing"
)

func TestGetSessionID_ExplicitOverride(t *testing.T) {
	os.Clearenv()
	id, source, err := GetSessionID("explicit-override")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "explicit-override" {
		t.Fatalf("expected explicit-override, got %q", id)
	}
	if source != "explicit-flag" {
		t.Fatalf("expected source explicit-flag, got %q", source)
	}
}

func TestGetSessionID_EnvVarOverride(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("OSM_SESSION_ID")
	os.Setenv("OSM_SESSION_ID", "from-env")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "from-env" {
		t.Fatalf("expected from-env, got %q", id)
	}
	if source != "explicit-env" {
		t.Fatalf("expected source explicit-env, got %q", source)
	}
}

func TestGetSessionID_ExplicitFlagOverridesEnv(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("OSM_SESSION_ID")
	os.Setenv("OSM_SESSION_ID", "from-env")

	id, source, err := GetSessionID("from-flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "from-flag" {
		t.Fatalf("expected from-flag, got %q", id)
	}
	if source != "explicit-flag" {
		t.Fatalf("expected source explicit-flag, got %q", source)
	}
}

func TestGetSessionID_Screen(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("STY")
	os.Setenv("STY", "12345.pts-0.host")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "screen" {
		t.Fatalf("expected source screen, got %q", source)
	}
	// ID should be a hash
	if len(id) != 64 { // SHA256 hex output length
		t.Fatalf("expected SHA256 hash (64 chars), got %d chars: %q", len(id), id)
	}
}

func TestGetSessionID_SSH(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "ssh-env" {
		t.Fatalf("expected source ssh-env, got %q", source)
	}
	// ID should be a hash
	if len(id) != 64 {
		t.Fatalf("expected SHA256 hash (64 chars), got %d chars: %q", len(id), id)
	}
}

func TestGetSessionID_SSHDifferentPorts(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	// Session 1 with port 12345
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	id1, _, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session 2 with port 12346 (different tab from same host)
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12346 192.168.1.1 22")
	id2, _, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Different client ports should produce different session IDs
	if id1 == id2 {
		t.Fatalf("expected different session IDs for different client ports, both got: %q", id1)
	}
}

func TestSessionContext_GenerateHash(t *testing.T) {
	ctx := &SessionContext{
		BootID:      "test-boot-id",
		ContainerID: "pid:[1234]",
		AnchorPID:   5678,
		StartTime:   123456789,
		TTYName:     "/dev/pts/0",
	}

	hash := ctx.GenerateHash()

	// Should be a valid SHA256 hex string (64 characters)
	if len(hash) != 64 {
		t.Fatalf("expected 64 character hash, got %d: %q", len(hash), hash)
	}

	// Hash should be consistent
	hash2 := ctx.GenerateHash()
	if hash != hash2 {
		t.Fatalf("hash not consistent: %q != %q", hash, hash2)
	}

	// Different context should produce different hash
	ctx2 := &SessionContext{
		BootID:      "different-boot-id",
		ContainerID: "pid:[1234]",
		AnchorPID:   5678,
		StartTime:   123456789,
		TTYName:     "/dev/pts/0",
	}
	hash3 := ctx2.GenerateHash()
	if hash == hash3 {
		t.Fatalf("different context should produce different hash")
	}
}

func TestSessionContext_GenerateHash_ColonDelimiterSafety(t *testing.T) {
	// Test that concatenation collisions are prevented by colon delimiters
	ctx1 := &SessionContext{
		BootID:      "AB",
		ContainerID: "C",
		AnchorPID:   1,
		StartTime:   1,
		TTYName:     "",
	}

	ctx2 := &SessionContext{
		BootID:      "A",
		ContainerID: "BC",
		AnchorPID:   1,
		StartTime:   1,
		TTYName:     "",
	}

	hash1 := ctx1.GenerateHash()
	hash2 := ctx2.GenerateHash()

	if hash1 == hash2 {
		t.Fatalf("colon delimiter should prevent concatenation collisions")
	}
}

func TestHashString(t *testing.T) {
	hash := hashString("test-input")

	// Should be a valid SHA256 hex string (64 characters)
	if len(hash) != 64 {
		t.Fatalf("expected 64 character hash, got %d: %q", len(hash), hash)
	}

	// Hash should be consistent
	hash2 := hashString("test-input")
	if hash != hash2 {
		t.Fatalf("hash not consistent: %q != %q", hash, hash2)
	}

	// Different input should produce different hash
	hash3 := hashString("different-input")
	if hash == hash3 {
		t.Fatalf("different input should produce different hash")
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid1, err := generateUUID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UUID should have the right format (8-4-4-4-12 hex characters)
	parts := strings.Split(uuid1, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts in UUID, got %d: %q", len(parts), uuid1)
	}

	uuid2, err := generateUUID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UUIDs should be unique
	if uuid1 == uuid2 {
		t.Fatalf("UUIDs should be unique, both got: %q", uuid1)
	}
}
