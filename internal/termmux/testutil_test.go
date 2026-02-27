package termmux

import "time"

// waitTimeout waits for ch to close or sends within timeout.
// Returns true if ch fired, false on timeout.
func waitTimeout(ch <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}
