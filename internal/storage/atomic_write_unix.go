//go:build !windows

package storage

import (
	"fmt"
)

// atomicRenameWindows is a stub for non-Windows platforms.
// It should never be called on non-Windows systems.
func atomicRenameWindows(oldpath, newpath string) error {
	return fmt.Errorf("atomicRenameWindows called on non-Windows platform")
}
