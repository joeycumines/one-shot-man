//go:build !linux && !windows

package session

import (
	"fmt"
	"runtime"
)

// resolveDeepAnchor returns an error on unsupported platforms.
// On macOS, the system relies on TERM_SESSION_ID (Priority 4) as the primary
// identifier for GUI terminals.
func resolveDeepAnchor() (*SessionContext, error) {
	return nil, fmt.Errorf("deep anchor detection not supported on %s", runtime.GOOS)
}
