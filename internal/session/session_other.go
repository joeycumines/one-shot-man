//go:build !linux && !windows && !darwin

package session

import (
	"fmt"
	"runtime"
)

// resolveDeepAnchor returns an error on unsupported platforms.
func resolveDeepAnchor() (*SessionContext, error) {
	return nil, fmt.Errorf("deep anchor detection not supported on %s", runtime.GOOS)
}
