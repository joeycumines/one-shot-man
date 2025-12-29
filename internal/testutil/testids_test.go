package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

// TestNewTestSessionID_Structure verifies the output format and UUID validity.
// It ensures the output strictly adheres to "encoded_uuid-prefix-safeName".
func TestNewTestSessionID_Structure(t *testing.T) {
	prefix := "integration-"
	tname := "SimpleTest"

	id := NewTestSessionID(prefix, tname)

	// Format: UUID (32 hex chars) + "-" + prefix + safeName
	// Note: The implementation concatenates prefix and safeName directly after the separator.
	// UUID is 32 chars (hex encoded, no dashes from hex.EncodeToString).
	if len(id) < 33 {
		t.Fatalf("ID too short to contain UUID: %q", id)
	}

	uuidPart := id[:32]
	separator := id[32:33]
	remainder := id[33:]

	// 1. Verify UUID part is valid hex
	if matched, _ := regexp.MatchString("^[0-9a-f]{32}$", uuidPart); !matched {
		t.Errorf("UUID part of ID is malformed: %s", uuidPart)
	}

	// 2. Verify separator
	if separator != "-" {
		t.Errorf("Expected separator '-', got %q", separator)
	}

	// 3. Verify suffix structure
	expectedSuffix := prefix + tname
	if remainder != expectedSuffix {
		t.Errorf("Expected suffix %q, got %q", expectedSuffix, remainder)
	}
}

// TestNewTestSessionID_Sanitization verifies that special characters are replaced
// with dashes, while allowing alphanumeric characters, dashes, and underscores.
func TestNewTestSessionID_Sanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // The expected suffix part (safeName)
	}{
		{
			name:     "Standard Alphanumeric",
			input:    "TestAlpha123",
			expected: "TestAlpha123",
		},
		{
			name:     "Existing Separators",
			input:    "Test-With_Underscore",
			expected: "Test-With_Underscore",
		},
		{
			name:     "Subtest Slashes",
			input:    "TestGroup/SubTest",
			expected: "TestGroup-SubTest",
		},
		{
			name:     "Windows Paths Backslashes",
			input:    "Test\\Path",
			expected: "Test-Path",
		},
		{
			name:     "Colons and Spaces",
			input:    "Test: Case One",
			expected: "Test--Case-One",
		},
		{
			name:     "Complex Symbols",
			input:    "Test!@#$%",
			expected: "Test-----",
		},
	}

	prefix := "safe-"

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := NewTestSessionID(prefix, tc.input)

			// Strip the UUID and prefix to verify the sanitization logic specifically
			// Format: UUID(32) + "-" + prefix + safeName
			expectedStart := 32 + 1 + len(prefix)
			if len(out) < expectedStart {
				t.Fatalf("Output too short: %s", out)
			}

			actualSafeName := out[expectedStart:]
			if actualSafeName != tc.expected {
				t.Errorf("Sanitization failed.\nInput:    %q\nExpected: %q\nGot:      %q", tc.input, tc.expected, actualSafeName)
			}
		})
	}
}

// TestNewTestSessionID_Truncation verifies that names exceeding maxSafeBytes (32)
// are correctly truncated and appended with a SHA256 hash suffix.
func TestNewTestSessionID_Truncation(t *testing.T) {
	// Create a string significantly longer than 32 bytes.
	// Length: 60 chars.
	longName := "Test_With_A_Very_Long_Name_That_Exceeds_Thirty_Two_Bytes_Limit"
	prefix := "" // Empty prefix to simplify length calculations

	id := NewTestSessionID(prefix, longName)

	// Extract the safeName part (after UUID and dash)
	// UUID is 32 chars + 1 dash = 33 chars offset
	if len(id) <= 33 {
		t.Fatalf("ID generated is too short: %s", id)
	}
	safeName := id[33:]

	// 1. Verify exact length constraint
	// The logic mandates the result matches maxSafeBytes (32) exactly when truncated.
	if len(safeName) != 32 {
		t.Errorf("Expected truncated safeName length 32, got %d. Value: %s", len(safeName), safeName)
	}

	// 2. Verify Hash Suffix Logic
	// Logic: "-" + hex(sha256(original_safe_name))[:8]
	// First, we must replicate the sanitization (which is identity for longName above)
	h := sha256.Sum256([]byte(longName))
	expectedHashSuffix := "-" + hex.EncodeToString(h[:])[:8]

	if !strings.HasSuffix(safeName, expectedHashSuffix) {
		t.Errorf("Truncated name does not end with expected hash suffix.\nExpected suffix: %s\nGot: %s", expectedHashSuffix, safeName)
	}

	// 3. Verify content preservation
	// Logic: It keeps the *last* 'keep' bytes.
	// keep = 32 - len(hashSuffix) = 32 - 9 = 23.
	// It should preserve the *end* of the string.
	expectedContent := longName[len(longName)-23:]
	actualContent := safeName[:23]

	if actualContent != expectedContent {
		t.Errorf("Truncation did not preserve the end of the string.\nExpected start: %s\nGot start:      %s", expectedContent, actualContent)
	}
}

// TestNewTestSessionID_Uniqueness tracks uniqueness across multiple calls
// and ensures consistent suffix generation for identical inputs.
func TestNewTestSessionID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	iterations := 100
	tname := "TestConcurrency"
	prefix := "uid-"

	// Capture the first suffix to ensure stability for same inputs
	firstID := NewTestSessionID(prefix, tname)
	firstSuffix := firstID[32:] // Extract -prefix+name part

	seen[firstID] = true

	for i := 0; i < iterations; i++ {
		id := NewTestSessionID(prefix, tname)

		// 1. Verify Uniqueness
		if seen[id] {
			t.Fatalf("Generated duplicate ID: %s", id)
		}
		seen[id] = true

		// 2. Verify Suffix Stability
		// Even though UUID changes, the generated name part must remain identical for the same input
		currentSuffix := id[32:]
		if currentSuffix != firstSuffix {
			t.Errorf("Suffix instability detected. Expected %q, got %q", firstSuffix, currentSuffix)
		}
	}
}

// TestNewTestSessionID_EdgeCases covers boundary conditions and empty inputs.
func TestNewTestSessionID_EdgeCases(t *testing.T) {
	t.Run("Empty Inputs", func(t *testing.T) {
		id := NewTestSessionID("", "")
		// Should be UUID + "-"
		if len(id) != 33 {
			t.Errorf("Expected length 33 for empty inputs, got %d", len(id))
		}
		if !strings.HasSuffix(id, "-") {
			t.Error("Expected ID to end in dash for empty inputs")
		}
	})

	t.Run("Boundary 32 Bytes", func(t *testing.T) {
		// Exactly 32 bytes - should NOT trigger hashing
		input := "12345678901234567890123456789012"
		id := NewTestSessionID("", input)
		safeName := id[33:]

		if safeName != input {
			t.Errorf("Input of exactly 32 bytes was modified unexpectedly: %s", safeName)
		}
		// Verify no hash suffix format (quick heuristic check for the dash at position 23)
		if strings.Contains(safeName, "-") {
			t.Error("Unexpected dash found in non-truncated 32-byte string")
		}
	})

	t.Run("Boundary 33 Bytes", func(t *testing.T) {
		// Exactly 33 bytes - MUST trigger hashing
		input := "123456789012345678901234567890123"
		id := NewTestSessionID("", input)
		safeName := id[33:]

		if len(safeName) != 32 {
			t.Errorf("Expected truncation to 32 bytes, got %d", len(safeName))
		}
		// Should have hash suffix
		// The original string has no dashes. If the result has a dash, it came from the hash suffix logic.
		if !strings.Contains(safeName, "-") {
			t.Error("Expected hash suffix (indicated by dash) for 33-byte input")
		}
	})

	t.Run("All Special Characters", func(t *testing.T) {
		input := "/////*****"
		id := NewTestSessionID("pfx-", input)
		if !strings.HasSuffix(id, "-pfx-----------") {
			t.Errorf("Failed to sanitize string of only special characters: %s", id)
		}
	})
}
