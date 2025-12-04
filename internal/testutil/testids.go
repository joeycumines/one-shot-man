package testutil

import (
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
	return fmt.Sprintf("%s-%s-%d", prefix, safeName, ID)
}
