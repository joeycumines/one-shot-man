package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// session.go — formatSessionID constrained namespace paths (96.3%)
// ==========================================================================

// TestFormatSessionID_ConstrainedNamespace triggers the namespace truncation
// path (CASE 2 constrained block) by using a namespace > 61 chars.
// With MaxSessionIDLength=80, NamespaceDelimiter="--" (2), fullSuffixLen=17:
// allowedNS = 80 - 2 - 17 = 61
// A namespace of 65 chars yields maxPayload = 80-65-2 = 13 < 17 → constrained.
func TestFormatSessionID_ConstrainedNamespace(t *testing.T) {
	t.Parallel()

	// A namespace of 65 chars and a payload that needs sanitization
	// (contains "/") to force CASE 2 entry.
	longNS := strings.Repeat("n", 65)
	payload := "some/path/payload"

	result := formatSessionID(longNS, payload)

	// Namespace should be truncated to allowedNS (61) chars.
	assert.True(t, len(result) <= MaxSessionIDLength,
		"result length %d should be <= MaxSessionIDLength %d", len(result), MaxSessionIDLength)
	assert.True(t, strings.HasPrefix(result, strings.Repeat("n", 61)+"--"),
		"result should start with truncated namespace + delimiter")
}

// TestFormatSessionID_VeryLongNamespace exercises the hash-only payload path
// where availForSanitized <= 0 after namespace truncation.
func TestFormatSessionID_VeryLongNamespace(t *testing.T) {
	t.Parallel()

	// Namespace of 100 chars → truncated to 61 → maxPayload = 17 → avail = 0
	// → hash-only payload.
	longNS := strings.Repeat("x", 100)
	payload := "dirty/payload!"

	result := formatSessionID(longNS, payload)

	// Result must fit within the limit.
	assert.True(t, len(result) <= MaxSessionIDLength,
		"result length %d should be <= %d", len(result), MaxSessionIDLength)

	// Should contain the truncated namespace.
	prefix := strings.Repeat("x", 61) + NamespaceDelimiter
	assert.True(t, strings.HasPrefix(result, prefix),
		"result should start with truncated namespace")

	// After the namespace delimiter, the payload should be a hash-only string
	// (hex characters only).
	payloadPart := result[len(prefix):]
	for _, c := range payloadPart {
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		assert.True(t, isHex,
			"hash-only payload should contain only hex chars, got %q in %q", string(c), payloadPart)
	}
}

// TestFormatSessionID_SanitizedWithFullSuffix exercises CASE 2 where the
// payload needs sanitization but there's room for sanitized content + suffix.
func TestFormatSessionID_SanitizedWithFullSuffix(t *testing.T) {
	t.Parallel()

	result := formatSessionID("test", "path/with/slashes")

	// Should have the namespace prefix.
	assert.True(t, strings.HasPrefix(result, "test--"),
		"result should start with namespace + delimiter")

	// Should contain the full suffix (16 hex chars).
	assert.True(t, len(result) <= MaxSessionIDLength,
		"result length %d should be <= %d", len(result), MaxSessionIDLength)

	// The sanitized payload should have "/" replaced.
	payloadPart := result[len("test--"):]
	assert.NotContains(t, payloadPart, "/",
		"sanitized payload should not contain slashes")
}

// TestFormatSessionID_TruncatedSanitizedPayload exercises CASE 2 where
// the sanitized payload is too long and gets truncated.
func TestFormatSessionID_TruncatedSanitizedPayload(t *testing.T) {
	t.Parallel()

	// Long payload that's safe (no sanitization needed) but exceeds
	// maxPayload when miniSuffix is included, forcing full suffix + truncation.
	longPayload := strings.Repeat("a", 100)

	result := formatSessionID("ns", longPayload)

	assert.True(t, len(result) <= MaxSessionIDLength,
		"result length %d should be <= %d", len(result), MaxSessionIDLength)
	assert.True(t, strings.HasPrefix(result, "ns--"),
		"result should start with namespace + delimiter")
}

// ==========================================================================
// session.go — formatScreenID
// ==========================================================================

// TestFormatScreenID_Deterministic verifies that formatScreenID produces
// consistent results for the same input.
func TestFormatScreenID_Deterministic(t *testing.T) {
	t.Parallel()

	id1 := formatScreenID("12345.pts-0.hostname")
	id2 := formatScreenID("12345.pts-0.hostname")

	require.Equal(t, id1, id2, "same STY should produce same ID")
	assert.True(t, strings.HasPrefix(id1, NamespaceScreen+NamespaceDelimiter),
		"should have screen namespace prefix")
}

// TestFormatScreenID_DifferentInputsDifferentOutput verifies collision resistance.
func TestFormatScreenID_DifferentInputsDifferentOutput(t *testing.T) {
	t.Parallel()

	id1 := formatScreenID("12345.pts-0.host1")
	id2 := formatScreenID("67890.pts-1.host2")

	assert.NotEqual(t, id1, id2, "different STY values should produce different IDs")
}

// ==========================================================================
// session.go — formatSSHID
// ==========================================================================

// TestFormatSSHID_FullTuple verifies the standard 4-field SSH_CONNECTION.
func TestFormatSSHID_FullTuple(t *testing.T) {
	t.Parallel()

	id := formatSSHID("192.168.1.100 12345 10.0.0.1 22")

	assert.True(t, strings.HasPrefix(id, NamespaceSSH+NamespaceDelimiter),
		"should have ssh namespace prefix")
	assert.True(t, len(id) <= MaxSessionIDLength,
		"ID length %d should be <= %d", len(id), MaxSessionIDLength)
}

// TestFormatSSHID_MalformedConnection verifies the fallback for non-4-field values.
func TestFormatSSHID_MalformedConnection(t *testing.T) {
	t.Parallel()

	id := formatSSHID("malformed")

	assert.True(t, strings.HasPrefix(id, NamespaceSSH+NamespaceDelimiter),
		"should have ssh namespace prefix")
}

// TestFormatSSHID_EmptyString verifies handling of empty SSH_CONNECTION.
func TestFormatSSHID_EmptyString(t *testing.T) {
	t.Parallel()

	id := formatSSHID("")

	assert.True(t, strings.HasPrefix(id, NamespaceSSH+NamespaceDelimiter),
		"should have ssh namespace prefix")
}
