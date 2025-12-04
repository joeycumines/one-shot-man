package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
)

var sessionCounter int64

// NewTestSessionID generates a deterministic, process-local unique session ID
// for tests. Pass in t.Name() from the caller to make IDs traceable per-test.
func NewTestSessionID(prefix, tname string) string {
	ID := atomic.AddInt64(&sessionCounter, 1)
	// Replace forward slashes and colons (common in subtests) with safe separators.
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, tname)

	// Ensure the safeName isn't excessively long (filesystem name limits exist).
	const maxSafeBytes = 64
	if len(safeName) > maxSafeBytes {
		// Compute a short hash suffix so we retain a stable identifier.
		h := sha256.Sum256([]byte(safeName))
		hashSuffix := "-" + hex.EncodeToString(h[:])[:8]

		// Reserve space for the hash suffix within the byte limit.
		keep := maxSafeBytes - len(hashSuffix)
		// keep must be positive for the truncation+hash strategy to work.
		// This is an invariant based on the constants above; if it ever fails
		// panic loudly so tests/CI reveal the misconfiguration quickly.
		if keep <= 0 {
			panic("maxSafeBytes too small for hashSuffix; update constants")
		}

		// Take the last `keep` bytes of safeName.
		start := len(safeName) - keep
		if start < 0 {
			start = 0
		}
		safeName = safeName[start:] + hashSuffix
	}

	return fmt.Sprintf("%s-%s-%d", prefix, safeName, ID)
}
