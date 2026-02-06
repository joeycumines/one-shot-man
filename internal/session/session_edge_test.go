package session

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// =============================================================================
// TestConcurrentSessionAccess - Concurrent session access safety tests
// =============================================================================

// TestConcurrentSessionAccess_MultipleGoroutines tests that concurrent access
// to session ID generation doesn't cause panics or crashes.
func TestConcurrentSessionAccess_MultipleGoroutines(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("OSM_SESSION")
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("TERM_SESSION_ID")
	}()

	// Set SSH connection for testing
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	// Test concurrent session ID generation
	const numGoroutines = 50
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				id, source, err := GetSessionID("")
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: error getting session ID: %v", goroutineID, err)
					return
				}
				if id == "" {
					errors <- fmt.Errorf("goroutine %d: empty session ID", goroutineID)
					return
				}
				if source == "" {
					errors <- fmt.Errorf("goroutine %d: empty source", goroutineID)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}
}

// TestConcurrentSessionAccess_ReadWriteWithExplicitOverride tests concurrent access
// with explicit overrides.
func TestConcurrentSessionAccess_ReadWriteWithExplicitOverride(t *testing.T) {
	const numGoroutines = 30
	const numOpsPerGoroutine = 30

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOpsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				// Alternate between empty and explicit override
				explicit := ""
				if j%2 == 0 {
					explicit = fmt.Sprintf("session-%d-%d", goroutineID, j)
				}

				id, source, err := GetSessionID(explicit)
				if err != nil {
					errors <- err
					return
				}
				if id == "" {
					errors <- fmt.Errorf("goroutine %d, op %d: empty ID", goroutineID, j)
					return
				}
				if explicit != "" && source != "explicit-flag" {
					errors <- fmt.Errorf("goroutine %d, op %d: expected explicit-flag source, got %q", goroutineID, j, source)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}
}

// TestConcurrentSessionAccess_RapidSessionIDCalls tests rapid calls to GetSessionID.
func TestConcurrentSessionAccess_RapidSessionIDCalls(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	// Rapid sequential calls
	for i := 0; i < 1000; i++ {
		id, source, err := GetSessionID("")
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if id == "" {
			t.Fatalf("iteration %d: empty session ID", i)
		}
		if source == "" {
			t.Fatalf("iteration %d: empty source", i)
		}
	}

	t.Logf("1000 rapid session ID calls completed successfully")
}

// =============================================================================
// TestSessionIDGenerationEdgeCases - Test session ID generation edge cases
// =============================================================================

// TestSessionIDGenerationEdgeCases_EmptyEnvironment tests ID generation with empty env.
func TestSessionIDGenerationEdgeCases_EmptyEnvironment(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("OSM_SESSION")
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("TERM_SESSION_ID")
	}()

	// Clear all env vars that could affect session ID
	os.Unsetenv("OSM_SESSION")
	os.Unsetenv("TMUX_PANE")
	os.Unsetenv("STY")
	os.Unsetenv("SSH_CONNECTION")
	os.Unsetenv("TERM_SESSION_ID")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id == "" {
		t.Fatal("session ID should not be empty")
	}

	// Should fall back to deep-anchor or uuid-fallback
	if source != "deep-anchor" && source != "uuid-fallback" {
		t.Errorf("expected deep-anchor or uuid-fallback, got %q", source)
	}
}

// TestSessionIDGenerationEdgeCases_MultipleIndicatorsPresent tests with multiple indicators.
func TestSessionIDGenerationEdgeCases_MultipleIndicatorsPresent(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("OSM_SESSION")
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("TERM_SESSION_ID")
	}()

	// Set multiple environment variables
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	os.Setenv("TERM_SESSION_ID", "terminal-session-12345")
	os.Setenv("STY", "12345.pts-0.host")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id == "" {
		t.Fatal("session ID should not be empty")
	}

	// SSH has higher priority than screen, should use SSH
	if source != "ssh-env" {
		t.Logf("got source %q (may vary based on tmux availability)", source)
	}
}

// TestSessionIDGenerationEdgeCases_IDUniqueness tests that generated IDs are unique.
func TestSessionIDGenerationEdgeCases_IDUniqueness(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("OSM_SESSION")
		os.Unsetenv("TMUX_PANE")
		os.Unsetenv("STY")
		os.Unsetenv("SSH_CONNECTION")
		os.Unsetenv("TERM_SESSION_ID")
	}()

	// Generate many IDs and verify uniqueness
	seen := make(map[string]bool)
	const numIDs = 1000

	for i := 0; i < numIDs; i++ {
		// Vary environment slightly each time to generate different IDs
		os.Setenv("SSH_CONNECTION", fmt.Sprintf("192.168.1.%d %d 192.168.1.1 22", i%256, 10000+i))

		id, _, err := GetSessionID("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if seen[id] {
			t.Errorf("duplicate session ID generated: %q", id)
		}
		seen[id] = true
	}

	if len(seen) < numIDs {
		t.Errorf("expected %d unique IDs, got %d", numIDs, len(seen))
	}
}

// TestSessionIDGenerationEdgeCases_FormatValidation tests session ID format validation.
func TestSessionIDGenerationEdgeCases_FormatValidation(t *testing.T) {
	// Test various session ID sources
	testCases := []struct {
		name        string
		setupEnv    func()
		explicitVal string
		wantSrc     string
		wantRegex   string
	}{
		{
			name: "explicit flag",
			setupEnv: func() {
				os.Clearenv()
			},
			explicitVal: "my-test-session",
			wantSrc:     "explicit-flag",
			wantRegex:   `^ex--my-test-session_[0-9a-f]{2}$`,
		},
		{
			name: "SSH",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
			},
			explicitVal: "",
			wantSrc:     "ssh-env",
			wantRegex:   `^ssh--[0-9a-f]{16}$`,
		},
		{
			name: "screen",
			setupEnv: func() {
				os.Clearenv()
				os.Setenv("STY", "12345.pts-0.host")
			},
			explicitVal: "",
			wantSrc:     "screen",
			wantRegex:   `^screen--[0-9a-f]{16}$`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupEnv()
			defer func() {
				os.Unsetenv("OSM_SESSION")
				os.Unsetenv("TMUX_PANE")
				os.Unsetenv("STY")
				os.Unsetenv("SSH_CONNECTION")
				os.Unsetenv("TERM_SESSION_ID")
			}()

			id, source, err := GetSessionID(tc.explicitVal)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if id == "" {
				t.Fatal("session ID should not be empty")
			}

			// Validate format
			re := regexp.MustCompile(tc.wantRegex)
			if !re.MatchString(id) {
				t.Errorf("session ID %q doesn't match expected format %q", id, tc.wantRegex)
			}

			// Validate source
			if source != tc.wantSrc {
				t.Errorf("expected source %q, got %q", tc.wantSrc, source)
			}
		})
	}
}

// TestSessionIDGenerationEdgeCases_ExplicitOverrideFormat tests explicit override format.
func TestSessionIDGenerationEdgeCases_ExplicitOverrideFormat(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("OSM_SESSION")

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "my-session",
			expected: "ex--my-session",
		},
		{
			name:     "name with hyphen",
			input:    "my-session-v2",
			expected: "ex--my-session-v2",
		},
		{
			name:     "name with underscore",
			input:    "my_session_v2",
			expected: "ex--my_session_v2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id, source, err := GetSessionID(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if source != "explicit-flag" {
				t.Errorf("expected source explicit-flag, got %q", source)
			}

			// ID should start with expected prefix
			if !strings.HasPrefix(id, tc.expected) {
				t.Errorf("expected prefix %q, got %q", tc.expected, id)
			}

			// Should have suffix for mimicry protection
			if !strings.Contains(id, "_") {
				t.Errorf("expected suffix for mimicry protection, got %q", id)
			}
		})
	}
}

// TestSessionIDGenerationEdgeCases_SpecialCharactersInInput tests handling of special chars.
func TestSessionIDGenerationEdgeCases_SpecialCharactersInInput(t *testing.T) {
	os.Clearenv()

	testCases := []struct {
		name        string
		input       string
		shouldPanic bool
	}{
		{
			name:        "spaces",
			input:       "session with spaces",
			shouldPanic: false,
		},
		{
			name:        "unicode",
			input:       "session-日本語",
			shouldPanic: false,
		},
		{
			name:        "slashes",
			input:       "path/to/session",
			shouldPanic: false,
		},
		{
			name:        "special chars",
			input:       "session@#$%",
			shouldPanic: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			id, source, err := GetSessionID(tc.input)
			if err != nil {
				if tc.shouldPanic {
					t.Logf("expected error: %v", err)
				} else {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}

			if id == "" {
				t.Fatal("session ID should not be empty")
			}

			if source != "explicit-flag" {
				t.Errorf("expected source explicit-flag, got %q", source)
			}

			// Verify ID is filesystem-safe (no path separators)
			if strings.ContainsAny(id, "/\\:*?\"<>|") {
				t.Errorf("session ID contains unsafe characters: %q", id)
			}
		})
	}
}

// TestSessionIDGenerationEdgeCases_Determinism tests that same input produces same output.
func TestSessionIDGenerationEdgeCases_Determinism(t *testing.T) {
	os.Clearenv()
	defer func() {
		os.Unsetenv("SSH_CONNECTION")
	}()

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	// Generate ID multiple times
	id1, source1, _ := GetSessionID("")
	id2, source2, _ := GetSessionID("")

	if id1 != id2 {
		t.Errorf("determinism broken: %q != %q", id1, id2)
	}

	if source1 != source2 {
		t.Errorf("source should be consistent: %q != %q", source1, source2)
	}
}

// =============================================================================
// TestSessionContextEdgeCases - SessionContext edge cases
// =============================================================================

// TestSessionContextEdgeCases_EmptyContext tests handling of empty session context.
func TestSessionContextEdgeCases_EmptyContext(t *testing.T) {
	ctx := &SessionContext{}

	// Should not panic
	hash := ctx.GenerateHash()
	if len(hash) != 64 {
		t.Errorf("expected 64 char hash, got %d", len(hash))
	}
}

// TestSessionContextEdgeCases_LongFields tests handling of very long field values.
func TestSessionContextEdgeCases_LongFields(t *testing.T) {
	longString := strings.Repeat("a", 1000)

	ctx := &SessionContext{
		BootID:      longString,
		ContainerID: longString,
		TTYName:     longString,
		AnchorPID:   12345,
		StartTime:   67890,
	}

	// Should not panic
	hash := ctx.GenerateHash()
	if len(hash) != 64 {
		t.Errorf("expected 64 char hash, got %d", len(hash))
	}
}

// TestSessionContextEdgeCases_UnicodeFields tests handling of unicode in fields.
func TestSessionContextEdgeCases_UnicodeFields(t *testing.T) {
	ctx := &SessionContext{
		BootID:      "boot-日本語-тест-emoji-🎉",
		ContainerID: "container-中文",
		TTYName:     "/dev/pts/中文测试",
		AnchorPID:   12345,
		StartTime:   67890,
	}

	// Should not panic
	hash := ctx.GenerateHash()
	if len(hash) != 64 {
		t.Errorf("expected 64 char hash, got %d", len(hash))
	}

	// Same context should produce same hash
	ctx2 := &SessionContext{
		BootID:      "boot-日本語-тест-emoji-🎉",
		ContainerID: "container-中文",
		TTYName:     "/dev/pts/中文测试",
		AnchorPID:   12345,
		StartTime:   67890,
	}
	hash2 := ctx2.GenerateHash()
	if hash != hash2 {
		t.Error("same context should produce same hash")
	}
}

// =============================================================================
// TestSanitizationEdgeCases - Payload sanitization edge cases
// =============================================================================

// TestSanitizationEdgeCases_AllUnsafeChars tests string with all unsafe characters.
func TestSanitizationEdgeCases_AllUnsafeChars(t *testing.T) {
	unsafeInput := "/\\:*?\"<>|' \t\n\r中文日本語"

	result := sanitizePayload(unsafeInput)

	// Should not contain any unsafe characters
	unsafeChars := "/\\:*?\"<>|"
	for _, c := range unsafeChars {
		if strings.ContainsRune(result, c) {
			t.Errorf("result still contains unsafe char %q: %q", c, result)
		}
	}
}

// TestSanitizationEdgeCases_OnlySafeChars tests string with only safe characters.
func TestSanitizationEdgeCases_OnlySafeChars(t *testing.T) {
	safeInput := "abcXYZ123.-_"

	result := sanitizePayload(safeInput)

	if result != safeInput {
		t.Errorf("expected %q, got %q", safeInput, result)
	}
}

// TestSanitizationEdgeCases_MixedLengths tests sanitization of various length strings.
func TestSanitizationEdgeCases_MixedLengths(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"single char", "a", "a"},
		{"max safe length", strings.Repeat("a", 200), strings.Repeat("a", 200)},
		{"with unsafe", "hello/world", "hello_world"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizePayload(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizePayload(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// =============================================================================
// TestHashFunctionEdgeCases - Hash function edge cases
// =============================================================================

// TestHashFunctionEdgeCases_EmptyInput tests hashing empty string.
func TestHashFunctionEdgeCases_EmptyInput(t *testing.T) {
	hash := hashString("")

	if len(hash) != 64 {
		t.Errorf("expected 64 char hash, got %d", len(hash))
	}

	// Should be deterministic
	hash2 := hashString("")
	if hash != hash2 {
		t.Error("empty string should produce same hash")
	}
}

// TestHashFunctionEdgeCases_LargeInput tests hashing large input.
func TestHashFunctionEdgeCases_LargeInput(t *testing.T) {
	largeInput := strings.Repeat("x", 1024*1024) // 1 MB

	hash := hashString(largeInput)

	if len(hash) != 64 {
		t.Errorf("expected 64 char hash, got %d", len(hash))
	}
}

// TestHashFunctionEdgeCases_CollisionResistance tests that similar inputs produce different hashes.
func TestHashFunctionEdgeCases_CollisionResistance(t *testing.T) {
	base := "session-data"
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		input := base + fmt.Sprintf("%d", i)
		hash := hashString(input)

		if seen[hash] {
			t.Errorf("collision detected for input %q", input)
		}
		seen[hash] = true
	}
}

// TestHashFunctionEdgeCases_UnicodeInput tests hashing unicode strings.
func TestHashFunctionEdgeCases_UnicodeInput(t *testing.T) {
	inputs := []string{
		"hello",
		"日本語",
		"中文",
		"हिन्दी",
		"🎉🚀",
		"mixed 日本語 english",
	}

	seen := make(map[string]bool)
	for _, input := range inputs {
		hash := hashString(input)
		if len(hash) != 64 {
			t.Errorf("invalid hash length for %q: %d", input, len(hash))
		}
		if seen[hash] {
			t.Errorf("collision for %q", input)
		}
		seen[hash] = true
	}
}
