package scripting

import (
	"fmt"
	"sync/atomic"
	"testing"
)

var testSessionCounter int64

// newTestSessionID generates a deterministic, process-local unique session ID
// for tests to avoid collisions that can happen when using time.Now().UnixNano().
func newTestSessionID(t *testing.T, prefix string) string {
	id := atomic.AddInt64(&testSessionCounter, 1)
	return fmt.Sprintf("%s-%s-%d", prefix, t.Name(), id)
}
