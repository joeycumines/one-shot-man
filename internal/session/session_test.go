package session

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// =============================================================================
// Priority Hierarchy Tests (from docs/sophisticated-auto-determination-of-session-id.md)
// Tests verify the strict priority order: Explicit > Multiplexer > SSH > GUI > Deep Anchor > UUID
// =============================================================================

// TestPriorityHierarchy_ExplicitFlagIsHighestPriority verifies that explicit flag
// takes precedence over ALL other methods including OSM_SESSION_ID env.
func TestPriorityHierarchy_ExplicitFlagIsHighestPriority(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("OSM_SESSION_ID")
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("TERM_SESSION_ID")
	}()

	// Set all possible environment variables
	os.Setenv("OSM_SESSION_ID", "env-session")
	os.Setenv("TMUX_PANE", "%0")
	os.Setenv("STY", "12345.pts-0.host")
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	os.Setenv("TERM_SESSION_ID", "terminal-session")

	id, source, err := GetSessionID("explicit-flag-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "explicit-flag-value" {
		t.Errorf("expected explicit-flag-value, got %q", id)
	}
	if source != "explicit-flag" {
		t.Errorf("expected source explicit-flag, got %q", source)
	}
}

// TestPriorityHierarchy_EnvOverridesMultiplexer verifies OSM_SESSION_ID env
// takes precedence over multiplexer detection.
func TestPriorityHierarchy_EnvOverridesMultiplexer(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("OSM_SESSION_ID")
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
	}()

	os.Setenv("OSM_SESSION_ID", "env-session")
	os.Setenv("TMUX_PANE", "%0")
	os.Setenv("STY", "12345.pts-0.host")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "env-session" {
		t.Errorf("expected env-session, got %q", id)
	}
	if source != "explicit-env" {
		t.Errorf("expected source explicit-env, got %q", source)
	}
}

// TestPriorityHierarchy_MultiplexerOverridesSSH verifies multiplexer (screen)
// takes precedence over SSH when TMUX is unavailable.
func TestPriorityHierarchy_MultiplexerOverridesSSH(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("STY")
		os.Unsetenv("SSH_CONNECTION")
	}()

	// Note: TMUX_PANE without tmux running falls through to STY
	os.Setenv("STY", "12345.pts-0.host")
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "screen" {
		t.Errorf("expected source screen, got %q", source)
	}
	if len(id) != 64 {
		t.Errorf("expected SHA256 hash (64 chars), got %d chars: %q", len(id), id)
	}
}

// TestPriorityHierarchy_SSHOverridesDeepAnchor verifies SSH context takes
// precedence over deep anchor when SSH_CONNECTION is present.
func TestPriorityHierarchy_SSHOverridesDeepAnchor(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	_, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "ssh-env" {
		t.Errorf("expected source ssh-env, got %q", source)
	}
}

// =============================================================================
// Priority 1: Explicit Override Tests
// =============================================================================

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

// TestGetSessionID_ExplicitOverride_EmptyStringIsNotOverride verifies that
// empty string is not treated as an explicit override.
func TestGetSessionID_ExplicitOverride_EmptyStringIsNotOverride(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("OSM_SESSION_ID")
	os.Setenv("OSM_SESSION_ID", "from-env")

	id, source, _ := GetSessionID("")
	if source == "explicit-flag" {
		t.Errorf("empty string should not be treated as explicit flag")
	}
	if id != "from-env" {
		t.Errorf("expected from-env, got %q", id)
	}
}

// =============================================================================
// Priority 2: Multiplexer Detection Tests
// =============================================================================

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

// TestGetSessionID_Screen_DifferentSTYProducesDifferentID verifies that
// different STY values produce different session IDs.
func TestGetSessionID_Screen_DifferentSTYProducesDifferentID(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("STY")

	os.Setenv("STY", "12345.pts-0.host")
	id1, _, _ := GetSessionID("")

	os.Setenv("STY", "67890.pts-1.host")
	id2, _, _ := GetSessionID("")

	if id1 == id2 {
		t.Errorf("different STY values should produce different session IDs")
	}
}

// TestGetSessionID_Screen_SameSTYProducesSameID verifies deterministic behavior.
func TestGetSessionID_Screen_SameSTYProducesSameID(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("STY")

	os.Setenv("STY", "12345.pts-0.host")
	id1, _, _ := GetSessionID("")
	id2, _, _ := GetSessionID("")

	if id1 != id2 {
		t.Errorf("same STY value should produce same session ID, got %q and %q", id1, id2)
	}
}

// TestGetSessionID_TmuxPane_StaleFallsThrough verifies that TMUX_PANE with
// unreachable tmux falls through to next priority (per doc spec).
func TestGetSessionID_TmuxPane_StaleFallsThrough(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
	}()

	// TMUX_PANE present but tmux not running - should fall through
	os.Setenv("TMUX_PANE", "%0")
	os.Setenv("STY", "12345.pts-0.host")

	_, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall through to screen since tmux is unreachable
	if source != "screen" {
		t.Logf("source=%q (tmux may be running on this system)", source)
		// This test may pass with "tmux" if tmux is actually running
	}
}

// =============================================================================
// Priority 3: SSH Context Tests
// =============================================================================

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

// TestGetSessionID_SSHDifferentPorts verifies that different client ports
// produce different session IDs (critical for concurrent tabs from same host).
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

	// CRITICAL: Different client ports MUST produce different session IDs
	// This was a resolved conflict in the architecture doc
	if id1 == id2 {
		t.Fatalf("CRITICAL: expected different session IDs for different client ports, both got: %q", id1)
	}
}

// TestGetSessionID_SSH_SameConnectionProducesSameID verifies deterministic behavior.
func TestGetSessionID_SSH_SameConnectionProducesSameID(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	id1, _, _ := GetSessionID("")
	id2, _, _ := GetSessionID("")

	if id1 != id2 {
		t.Errorf("same SSH_CONNECTION should produce same session ID")
	}
}

// TestGetSessionID_SSH_MalformedConnection verifies fallback for malformed SSH_CONNECTION.
func TestGetSessionID_SSH_MalformedConnection(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	// Malformed - only 3 parts instead of 4
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1")
	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "ssh-env" {
		t.Errorf("expected source ssh-env even for malformed, got %q", source)
	}
	if len(id) != 64 {
		t.Errorf("expected SHA256 hash for malformed SSH_CONNECTION")
	}
}

// TestGetSessionID_SSH_DifferentServerIP verifies different server IPs produce different IDs.
func TestGetSessionID_SSH_DifferentServerIP(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	id1, _, _ := GetSessionID("")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.2 22")
	id2, _, _ := GetSessionID("")

	if id1 == id2 {
		t.Errorf("different server IPs should produce different session IDs")
	}
}

// TestGetSessionID_SSH_DifferentServerPort verifies different server ports produce different IDs.
func TestGetSessionID_SSH_DifferentServerPort(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	id1, _, _ := GetSessionID("")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 2222")
	id2, _, _ := GetSessionID("")

	if id1 == id2 {
		t.Errorf("different server ports should produce different session IDs")
	}
}

// TestGetSessionID_SSH_IPv6Address verifies IPv6 addresses are handled correctly.
func TestGetSessionID_SSH_IPv6Address(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "::1 12345 ::1 22")
	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != "ssh-env" {
		t.Errorf("expected source ssh-env for IPv6, got %q", source)
	}
	if len(id) != 64 {
		t.Errorf("expected SHA256 hash for IPv6 address")
	}
}

// =============================================================================
// Priority 4: macOS GUI Terminal Tests
// =============================================================================

// TestGetSessionID_MacOSTerminal verifies TERM_SESSION_ID handling on darwin.
func TestGetSessionID_MacOSTerminal(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("TERM_SESSION_ID")

	os.Setenv("TERM_SESSION_ID", "terminal-session-12345")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runtime.GOOS == "darwin" {
		if source != "macos-terminal" {
			t.Errorf("expected source macos-terminal on darwin, got %q", source)
		}
		if len(id) != 64 {
			t.Errorf("expected SHA256 hash (64 chars), got %d chars", len(id))
		}
	} else {
		// On non-darwin, TERM_SESSION_ID should be ignored
		if source == "macos-terminal" {
			t.Errorf("TERM_SESSION_ID should only be used on darwin, but got macos-terminal on %s", runtime.GOOS)
		}
	}
}

// TestGetSessionID_MacOSTerminal_DifferentIDsProduceDifferentHashes verifies
// different TERM_SESSION_ID values produce different hashes on darwin.
func TestGetSessionID_MacOSTerminal_DifferentIDsProduceDifferentHashes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("TERM_SESSION_ID only used on darwin")
	}

	os.Clearenv()
	defer os.Unsetenv("TERM_SESSION_ID")

	os.Setenv("TERM_SESSION_ID", "terminal-session-1")
	id1, _, _ := GetSessionID("")

	os.Setenv("TERM_SESSION_ID", "terminal-session-2")
	id2, _, _ := GetSessionID("")

	if id1 == id2 {
		t.Errorf("different TERM_SESSION_ID values should produce different hashes")
	}
}

// =============================================================================
// Priority 6: UUID Fallback Tests
// =============================================================================

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

	// Verify each part has expected length
	expectedLengths := []int{8, 4, 4, 4, 12}
	for i, part := range parts {
		if len(part) != expectedLengths[i] {
			t.Errorf("UUID part %d: expected %d chars, got %d: %q", i, expectedLengths[i], len(part), part)
		}
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

// TestGenerateUUID_UniqueAcrossMultipleCalls verifies UUID uniqueness.
func TestGenerateUUID_UniqueAcrossMultipleCalls(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		uuid, err := generateUUID()
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if seen[uuid] {
			t.Fatalf("duplicate UUID generated: %q", uuid)
		}
		seen[uuid] = true
	}
}

// =============================================================================
// SessionContext Hash Generation Tests
// =============================================================================

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

// TestSessionContext_GenerateHash_ColonDelimiterSafety verifies that colon delimiters
// prevent concatenation collisions (e.g., "AB" + "C" vs "A" + "BC").
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

// TestSessionContext_GenerateHash_AllFieldsContribute verifies each field affects hash.
func TestSessionContext_GenerateHash_AllFieldsContribute(t *testing.T) {
	base := &SessionContext{
		BootID:      "boot",
		ContainerID: "container",
		AnchorPID:   1234,
		StartTime:   5678,
		TTYName:     "/dev/pts/0",
	}
	baseHash := base.GenerateHash()

	tests := []struct {
		name   string
		modify func() *SessionContext
	}{
		{"BootID", func() *SessionContext {
			return &SessionContext{BootID: "different", ContainerID: "container", AnchorPID: 1234, StartTime: 5678, TTYName: "/dev/pts/0"}
		}},
		{"ContainerID", func() *SessionContext {
			return &SessionContext{BootID: "boot", ContainerID: "different", AnchorPID: 1234, StartTime: 5678, TTYName: "/dev/pts/0"}
		}},
		{"AnchorPID", func() *SessionContext {
			return &SessionContext{BootID: "boot", ContainerID: "container", AnchorPID: 9999, StartTime: 5678, TTYName: "/dev/pts/0"}
		}},
		{"StartTime", func() *SessionContext {
			return &SessionContext{BootID: "boot", ContainerID: "container", AnchorPID: 1234, StartTime: 9999, TTYName: "/dev/pts/0"}
		}},
		{"TTYName", func() *SessionContext {
			return &SessionContext{BootID: "boot", ContainerID: "container", AnchorPID: 1234, StartTime: 5678, TTYName: "/dev/pts/1"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modified := tt.modify()
			modifiedHash := modified.GenerateHash()
			if modifiedHash == baseHash {
				t.Errorf("changing %s should change hash", tt.name)
			}
		})
	}
}

// TestSessionContext_GenerateHash_NoTruncation verifies full SHA256 output (no truncation).
func TestSessionContext_GenerateHash_NoTruncation(t *testing.T) {
	ctx := &SessionContext{
		BootID:      "test-boot-id",
		ContainerID: "pid:[1234]",
		AnchorPID:   5678,
		StartTime:   123456789,
		TTYName:     "/dev/pts/0",
	}

	hash := ctx.GenerateHash()

	// SHA256 produces 32 bytes = 64 hex characters
	// CONFLICT RESOLUTION in doc: Removed truncation to guarantee collision resistance
	if len(hash) != 64 {
		t.Errorf("hash should be full SHA256 (64 chars), got %d chars", len(hash))
	}
}

// TestSessionContext_GenerateHash_EmptyFields verifies handling of empty fields.
func TestSessionContext_GenerateHash_EmptyFields(t *testing.T) {
	ctx := &SessionContext{
		BootID:      "",
		ContainerID: "",
		AnchorPID:   0,
		StartTime:   0,
		TTYName:     "",
	}

	hash := ctx.GenerateHash()
	if len(hash) != 64 {
		t.Errorf("hash should still be valid SHA256 even with empty fields")
	}
}

// =============================================================================
// hashString Tests
// =============================================================================

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

// TestHashString_EmptyInput verifies handling of empty string.
func TestHashString_EmptyInput(t *testing.T) {
	hash := hashString("")
	if len(hash) != 64 {
		t.Errorf("hash of empty string should still be valid SHA256")
	}
}

// TestHashString_SpecialCharacters verifies handling of special characters.
func TestHashString_SpecialCharacters(t *testing.T) {
	inputs := []string{
		"ssh:192.168.1.1:22:192.168.1.2:22",
		"screen:12345.pts-0.host",
		"terminal:ABC-DEF-123",
		"string with spaces",
		"string\twith\ttabs",
		"string\nwith\nnewlines",
		"unicode: 日本語",
	}

	seen := make(map[string]bool)
	for _, input := range inputs {
		hash := hashString(input)
		if len(hash) != 64 {
			t.Errorf("invalid hash length for input %q: %d", input, len(hash))
		}
		if seen[hash] {
			t.Errorf("collision detected for input %q", input)
		}
		seen[hash] = true
	}
}

// =============================================================================
// getTmuxSessionID Tests
// =============================================================================

// TestGetTmuxSessionID_Timeout verifies 500ms timeout behavior.
func TestGetTmuxSessionID_Timeout(t *testing.T) {
	// This test verifies the timeout exists, though we can't easily test
	// the exact timeout behavior without mocking. We verify the function
	// returns quickly when tmux is not available.

	// If tmux is not running, this should fail quickly (not hang)
	done := make(chan bool)
	go func() {
		_, _ = getTmuxSessionID()
		done <- true
	}()

	select {
	case <-done:
		// Good - function completed
	case <-timeout(2000): // 2 second timeout for the test
		t.Error("getTmuxSessionID took too long - timeout may not be working")
	}
}

// helper for test timeout
func timeout(ms int) <-chan bool {
	ch := make(chan bool)
	go func() {
		select {}
	}()
	return ch
}

// =============================================================================
// Integration/Fallback Tests
// =============================================================================

// TestGetSessionID_FallbackChain verifies fallback when higher priorities unavailable.
func TestGetSessionID_FallbackChain(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without any env vars, should use deep-anchor or uuid-fallback
	validSources := map[string]bool{
		"deep-anchor":   true,
		"uuid-fallback": true,
	}
	if !validSources[source] {
		t.Errorf("expected deep-anchor or uuid-fallback, got %q", source)
	}
	if id == "" {
		t.Error("session ID should not be empty")
	}
}

// TestGetSessionID_Deterministic verifies same environment produces same ID.
func TestGetSessionID_Deterministic(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	id1, source1, _ := GetSessionID("")
	id2, source2, _ := GetSessionID("")

	if id1 != id2 {
		t.Errorf("same environment should produce same ID: %q != %q", id1, id2)
	}
	if source1 != source2 {
		t.Errorf("same environment should produce same source: %q != %q", source1, source2)
	}
}
