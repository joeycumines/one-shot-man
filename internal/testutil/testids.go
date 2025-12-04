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
	id := atomic.AddInt64(&sessionCounter, 1)
	return fmt.Sprintf("%s-%s-%d", prefix, strings.ReplaceAll(tname, `/`, `-_-`), id)
}
