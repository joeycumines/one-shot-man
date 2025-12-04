package session

import (
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
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
	// Explicit values get namespaced with "ex--" prefix
	// Safe user-provided payloads get MANDATORY MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^ex--explicit-flag-value_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Errorf("expected ex--explicit-flag-value_XX (mini suffix for safe payload), got %q", id)
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
	// Explicit env values get namespaced with "ex--" prefix
	// Safe user-provided payloads get MANDATORY MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^ex--env-session_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Errorf("expected ex--env-session_XX (mini suffix for safe payload), got %q", id)
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
	// New format: screen--{16-char-hash}
	if !strings.HasPrefix(id, "screen--") {
		t.Errorf("expected screen-- prefix, got %q", id)
	}
	// Total length: "screen--" (8) + 16 = 24
	if len(id) != 24 {
		t.Errorf("expected 24 chars (screen-- + 16 hash), got %d chars: %q", len(id), id)
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
	// Explicit values get namespaced with "ex--" prefix
	// Safe user-provided payloads get MANDATORY MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^ex--explicit-override_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Fatalf("expected ex--explicit-override_XX (mini suffix for safe payload), got %q", id)
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
	// Explicit env values get namespaced with "ex--" prefix
	// Safe user-provided payloads get MANDATORY MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^ex--from-env_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Fatalf("expected ex--from-env_XX (mini suffix for safe payload), got %q", id)
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
	// Explicit flag values get namespaced with "ex--" prefix
	// Safe user-provided payloads get MANDATORY MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^ex--from-flag_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Fatalf("expected ex--from-flag_XX (mini suffix for safe payload), got %q", id)
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
	// Explicit env values get namespaced with "ex--" prefix
	// Safe user-provided payloads get MANDATORY MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^ex--from-env_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Errorf("expected ex--from-env_XX (mini suffix for safe payload), got %q", id)
	}
}

// TestGetSessionID_ExplicitOverride_AlreadyNamespaced verifies that
// already-namespaced values are sanitized and returned.
func TestGetSessionID_ExplicitOverride_AlreadyNamespaced(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("custom--my-value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Already namespaced values should have namespace sanitized and payload sanitized
	// "custom" and "my-value" are both safe, so MINI suffix (_XX) is added
	re := regexp.MustCompile(`^custom--my-value_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Errorf("expected custom--my-value_XX (mini suffix for safe payload), got %q", id)
	}
	if source != "explicit-flag" {
		t.Errorf("expected source explicit-flag, got %q", source)
	}
}

func TestFormatSessionID_SanitizationAppendsHash(t *testing.T) {
	// Different raw inputs that sanitize to the same string must not collide.
	idA := formatSessionID(NamespaceExplicit, "user/name") // Requires sanitization (/ -> _)
	idB := formatSessionID(NamespaceExplicit, "user_name") // Already safe, no change

	if idA == idB {
		t.Fatalf("expected different IDs for inputs that sanitize to same value, both got %q", idA)
	}

	// idA was sanitized (user/name -> user_name), so it MUST have a FULL suffix (16 hex)
	reA := regexp.MustCompile(`^ex--user_name_[0-9a-f]{16}$`)
	if !reA.MatchString(idA) {
		t.Fatalf("expected idA (sanitized) to have _16hex suffix, got %q", idA)
	}

	// idB was NOT sanitized (user_name is already safe), but gets MINI suffix (2 hex) for mimicry prevention
	reB := regexp.MustCompile(`^ex--user_name_[0-9a-f]{2}$`)
	if !reB.MatchString(idB) {
		t.Fatalf("expected idB (already safe) to have _2hex mini suffix, got %q", idB)
	}
}

func TestFormatSessionID_NoSuffixWhenUnchanged(t *testing.T) {
	// If payload is already safe and unchanged by sanitization, MINI suffix is added
	// to prevent mimicry attacks (where attacker crafts payload matching suffixed output)
	input := "safe-name_123"
	id := formatSessionID(NamespaceExplicit, input)

	// Safe payloads get mandatory mini suffix (_XX) for mimicry prevention
	re := regexp.MustCompile(`^ex--safe-name_123_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Fatalf("Safe payload should get mini suffix: expected ex--safe-name_123_XX, got %q", id)
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
	// New format: screen--{16-char-hash}
	if !strings.HasPrefix(id, "screen--") {
		t.Fatalf("expected screen-- prefix, got %q", id)
	}
	// Total length: "screen--" (8) + 16 = 24
	if len(id) != 24 {
		t.Fatalf("expected 24 chars (screen-- + 16 hash), got %d chars: %q", len(id), id)
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
	// New format: ssh--{16-char-hash}
	if !strings.HasPrefix(id, "ssh--") {
		t.Fatalf("expected ssh-- prefix, got %q", id)
	}
	// Total length: "ssh--" (5) + 16 = 21
	if len(id) != 21 {
		t.Fatalf("expected 21 chars (ssh-- + 16 hash), got %d chars: %q", len(id), id)
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
	// New format: ssh--{16-char-hash}
	if !strings.HasPrefix(id, "ssh--") || len(id) != 21 {
		t.Errorf("expected ssh-- prefix with 21 total chars, got %q", id)
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
	// New format: ssh--{16-char-hash}
	if !strings.HasPrefix(id, "ssh--") || len(id) != 21 {
		t.Errorf("expected ssh-- prefix with 21 total chars for IPv6, got %q", id)
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
		// New format: terminal--{16-char-hash}
		if !strings.HasPrefix(id, "terminal--") {
			t.Errorf("expected terminal-- prefix, got %q", id)
		}
		// Total length: "terminal--" (10) + 16 = 26
		if len(id) != 26 {
			t.Errorf("expected 26 chars (terminal-- + 16 hash), got %d chars", len(id))
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
	case <-time.After(2 * time.Second): // 2 second timeout for the test
		t.Error("getTmuxSessionID took too long - timeout may not be working")
	}
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

// =============================================================================
// Namespaced Format Tests
// =============================================================================

// TestExtractTmuxServerPID verifies extraction of server PID from TMUX env var.
func TestExtractTmuxServerPID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard format", "/tmp/tmux-1000/default,12345,0", "12345"},
		{"different PID", "/tmp/tmux-501/default,98765,1", "98765"},
		{"long path", "/var/folders/abc/def/T/tmux-501/default,54321,2", "54321"},
		{"single digit PID", "/tmp/tmux-1000/default,1,0", "1"},
		{"large PID", "/tmp/tmux-1000/default,4294967295,0", "4294967295"},
		{"empty string", "", ""},
		{"no commas", "/tmp/tmux-1000/default", ""},
		{"one comma only", "/tmp/tmux-1000/default,12345", ""},
		{"non-numeric PID", "/tmp/tmux-1000/default,abc,0", ""},
		{"trailing comma", "/tmp/tmux-1000/default,12345,", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTmuxServerPID(tt.input)
			if result != tt.expected {
				t.Errorf("extractTmuxServerPID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetTmuxSessionID_Format verifies the new tmux ID format uses TMUX_PANE + server PID.
func TestGetTmuxSessionID_Format(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("TMUX")
	}()

	// Set up tmux environment
	os.Setenv("TMUX_PANE", "%5")
	os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	id, err := getTmuxSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Format should be: tmux--{paneNum}_{serverPID}
	// Pane %5 -> 5, server PID 12345
	// Both are integers (safe), so NO suffix needed
	expected := "tmux--5_12345"
	if id != expected {
		t.Errorf("expected %q, got %q", expected, id)
	}
}

// TestGetTmuxSessionID_MissingTMUX verifies error when TMUX env var is missing.
func TestGetTmuxSessionID_MissingTMUX(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("TMUX_PANE")

	os.Setenv("TMUX_PANE", "%0")
	// TMUX not set

	_, err := getTmuxSessionID()
	if err == nil {
		t.Error("expected error when TMUX env var is missing")
	}
}

// TestGetTmuxSessionID_MissingTMUX_PANE verifies error when TMUX_PANE is missing.
func TestGetTmuxSessionID_MissingTMUX_PANE(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("TMUX")

	os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	// TMUX_PANE not set

	_, err := getTmuxSessionID()
	if err == nil {
		t.Error("expected error when TMUX_PANE env var is missing")
	}
}

// TestFormatSessionID_LengthBound verifies session IDs are bounded.
func TestFormatSessionID_LengthBound(t *testing.T) {
	// Very long payload should be truncated
	longPayload := strings.Repeat("a", 200)
	result := formatSessionID("test", longPayload)

	if len(result) > MaxSessionIDLength {
		t.Errorf("session ID should be <= %d chars, got %d: %q", MaxSessionIDLength, len(result), result)
	}

	if !strings.HasPrefix(result, "test--") {
		t.Errorf("should preserve namespace prefix, got %q", result)
	}
}

// TestFormatSessionID_ShortPayload verifies short safe payloads get MINI suffix.
func TestFormatSessionID_ShortPayload(t *testing.T) {
	result := formatSessionID("test", "short")
	// "short" is already safe (no sanitization needed), but user-provided payloads
	// get mandatory MINI suffix (_XX) to prevent mimicry attacks
	re := regexp.MustCompile(`^test--short_[0-9a-f]{2}$`)
	if !re.MatchString(result) {
		t.Errorf("formatSessionID(test, short) = %q, want test--short_XX (mini suffix)", result)
	}
}

// TestFormatSessionID_ExplicitShortHex_StillGetsSuffix verifies that user-controlled
// namespaces do NOT get to bypass the suffix logic, even if the payload
// happens to look like an internal 16-char lowercase hex hash.
func TestFormatSessionID_ExplicitShortHex_StillGetsSuffix(t *testing.T) {
	payload := "0123456789abcdef" // 16 lowercase hex chars
	result := formatSessionID(NamespaceExplicit, payload)

	// Even though the payload matches the internal-short-hex pattern, explicit
	// namespaces must still carry at least the mini suffix to enforce the
	// "all user-provided payloads are suffixed" guarantee.
	re := regexp.MustCompile(`^ex--0123456789abcdef_[0-9a-f]{2}$`)
	if !re.MatchString(result) {
		t.Fatalf("expected explicit short-hex payload to get mini suffix, got %q", result)
	}
}

// Prevent mimicry: a payload that looks like an already-hashed ID must not collide
// with a different pre-sanitization input that produced that appearance.
func TestFormatSessionID_PreventMimicry(t *testing.T) {
	// Input A sanitizes and gets suffix
	idA := formatSessionID(NamespaceExplicit, "foo/bar")

	// Input B is the literal payload that matches idA's payload portion
	parts := strings.SplitN(idA, "--", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected id format: %q", idA)
	}
	payload := parts[1]
	idB := formatSessionID(NamespaceExplicit, payload)

	if idA == idB {
		t.Fatalf("mimicry allowed: %q == %q", idA, idB)
	}
}

// Ensure very long namespace doesn't cause slice bounds or panics and remains bounded
func TestFormatSessionID_NamespaceTooLong_NoPanic(t *testing.T) {
	longNS := strings.Repeat("n", 200)
	id := formatSessionID(longNS, "payload")

	if len(id) > MaxSessionIDLength {
		t.Fatalf("expected id <= %d chars, got %d: %q", MaxSessionIDLength, len(id), id)
	}
}

// TestSessionContext_FormatSessionID verifies the namespaced session ID format.
func TestSessionContext_FormatSessionID(t *testing.T) {
	ctx := &SessionContext{
		BootID:      "test-boot-id",
		ContainerID: "pid:[1234]",
		AnchorPID:   5678,
		StartTime:   123456789,
		TTYName:     "/dev/pts/0",
	}

	id := ctx.FormatSessionID()

	// Should have anchor-- prefix
	if !strings.HasPrefix(id, "anchor--") {
		t.Errorf("expected anchor-- prefix, got %q", id)
	}

	// Total length: "anchor--" (8) + 16 = 24
	if len(id) != 24 {
		t.Errorf("expected 24 chars (anchor-- + 16 hash), got %d: %q", len(id), id)
	}

	// Should be deterministic
	id2 := ctx.FormatSessionID()
	if id != id2 {
		t.Errorf("FormatSessionID should be deterministic: %q != %q", id, id2)
	}
}

// TestNamespaceUniqueness verifies all namespace prefixes are distinct.
func TestNamespaceUniqueness(t *testing.T) {
	namespaces := []string{
		NamespaceExplicit,
		NamespaceTmux,
		NamespaceScreen,
		NamespaceSSH,
		NamespaceTerminal,
		NamespaceAnchor,
		NamespaceUUID,
	}

	seen := make(map[string]bool)
	for _, ns := range namespaces {
		if seen[ns] {
			t.Errorf("duplicate namespace: %q", ns)
		}
		seen[ns] = true
	}
}

// TestMaxSessionIDLength verifies the constant is reasonable.
func TestMaxSessionIDLength(t *testing.T) {
	// Should be filesystem-safe (most systems support 255 bytes)
	if MaxSessionIDLength > 255 {
		t.Errorf("MaxSessionIDLength %d is too large for most filesystems", MaxSessionIDLength)
	}
	// Should be reasonable (at least 40 to fit namespace + some payload)
	if MaxSessionIDLength < 40 {
		t.Errorf("MaxSessionIDLength %d is too small", MaxSessionIDLength)
	}
}

// =============================================================================
// Filesystem Safety Tests (Negative Tests)
// =============================================================================

// TestSanitizePayload_PathSeparators verifies path separators are sanitized.
func TestSanitizePayload_PathSeparators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"forward slash", "user/name", "user_name"},
		{"backslash", "user\\name", "user_name"},
		{"mixed slashes", "path/to\\file", "path_to_file"},
		{"multiple slashes", "a/b/c/d", "a_b_c_d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePayload(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizePayload(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSanitizePayload_WindowsReservedChars verifies Windows reserved chars are sanitized.
func TestSanitizePayload_WindowsReservedChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"colon", "C:drive", "C_drive"},
		{"asterisk", "file*name", "file_name"},
		{"question mark", "what?why", "what_why"},
		{"quotes", "say\"hello\"", "say_hello_"},
		{"less than", "a<b", "a_b"},
		{"greater than", "a>b", "a_b"},
		{"pipe", "a|b", "a_b"},
		{"all reserved", ":*?\"<>|", "_______"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePayload(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizePayload(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSanitizePayload_Whitelist verifies only whitelisted chars are preserved.
func TestSanitizePayload_Whitelist(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "abcxyz", "abcxyz"},
		{"uppercase", "ABCXYZ", "ABCXYZ"},
		{"digits", "0123456789", "0123456789"},
		{"dot", "file.txt", "file.txt"},
		{"hyphen", "my-file", "my-file"},
		{"underscore", "my_file", "my_file"},
		{"mixed safe", "My-File_v1.0", "My-File_v1.0"},
		{"spaces", "hello world", "hello_world"},
		{"special chars", "a@b#c$d%e", "a_b_c_d_e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePayload(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizePayload(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestExplicitID_PathTraversal verifies path traversal attacks are prevented.
func TestExplicitID_PathTraversal(t *testing.T) {
	os.Clearenv()

	// Attack vector: user supplies path traversal in session ID
	// Input: "user/name--hack" is parsed as namespace="user/name", payload="hack"
	// Namespace "/" is sanitized to "_": "user_name"
	// Payload "hack" is already safe → mini suffix (2 hex)
	id, _, err := GetSessionID("user/name--hack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The path separator must be sanitized
	if strings.Contains(id, "/") {
		t.Errorf("SECURITY: path separator not sanitized: %q", id)
	}
	// Namespace was sanitized (user/name -> user_name), but payload "hack" was safe
	// Since payload needed no sanitization, it gets mini suffix (2 hex)
	// Format: {sanitized_namespace}--{safe_payload}_{2hex}
	re := regexp.MustCompile(`^user_name--hack_[0-9a-f]{2}$`)
	if !re.MatchString(id) {
		t.Errorf("expected user_name--hack_XX (mini suffix - payload was safe), got %q", id)
	}
}

// TestExplicitID_WindowsPathTraversal verifies Windows path traversal is prevented.
func TestExplicitID_WindowsPathTraversal(t *testing.T) {
	os.Clearenv()

	// Attack vector: Windows-style path
	id, _, err := GetSessionID("C:\\Users\\hack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The backslash and colon must be sanitized
	if strings.ContainsAny(id, "\\:") {
		t.Errorf("SECURITY: Windows path chars not sanitized: %q", id)
	}
}

// TestTmuxSessionID_WindowsSafe verifies tmux session IDs are Windows-safe.
// The new format uses pane number + server PID (both integers), which is always safe.
func TestTmuxSessionID_WindowsSafe(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("TMUX")
	}()

	os.Setenv("TMUX_PANE", "%0")
	os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	id, err := getTmuxSessionID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must not contain any Windows-unsafe characters
	unsafeChars := "/\\:*?\"<>|"
	for _, c := range unsafeChars {
		if strings.ContainsRune(id, c) {
			t.Errorf("SECURITY: Windows-unsafe char %q in tmux ID: %q", c, id)
		}
	}

	// Should be prefixed correctly
	if !strings.HasPrefix(id, "tmux--") {
		t.Errorf("expected tmux-- prefix, got %q", id)
	}
}

// TestExplicitID_NaiveTruncationCollision verifies hash-suffix prevents collisions.
func TestExplicitID_NaiveTruncationCollision(t *testing.T) {
	os.Clearenv()

	// Create two long IDs that differ only at the end
	base := strings.Repeat("a", 100)
	id1Input := "custom--" + base + "A"
	id2Input := "custom--" + base + "B"

	id1, _, _ := GetSessionID(id1Input)
	id2, _, _ := GetSessionID(id2Input)

	// CRITICAL: These must NOT collide after truncation
	if id1 == id2 {
		t.Errorf("COLLISION: different inputs produced same output:\n  input1: %q\n  input2: %q\n  output: %q",
			id1Input, id2Input, id1)
	}

	// Both should be within length limit
	if len(id1) > MaxSessionIDLength {
		t.Errorf("id1 exceeds max length: %d > %d", len(id1), MaxSessionIDLength)
	}
	if len(id2) > MaxSessionIDLength {
		t.Errorf("id2 exceeds max length: %d > %d", len(id2), MaxSessionIDLength)
	}
}

// TestIsFilenameSafe verifies the whitelist function.
func TestIsFilenameSafe(t *testing.T) {
	safe := []rune{'a', 'z', 'A', 'Z', '0', '9', '.', '-', '_'}
	for _, r := range safe {
		if !isFilenameSafe(r) {
			t.Errorf("expected %q to be safe", r)
		}
	}

	unsafe := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|', ' ', '@', '#', '$', '%', '&'}
	for _, r := range unsafe {
		if isFilenameSafe(r) {
			t.Errorf("expected %q to be unsafe", r)
		}
	}
}
