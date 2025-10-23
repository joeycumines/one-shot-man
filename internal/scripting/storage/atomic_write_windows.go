//go:build windows
// +build windows

package storage

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// atomicRenameWindows performs an atomic file rename on Windows.
func atomicRenameWindows(oldpath, newpath string) error {
	from, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return fmt.Errorf("failed to convert oldpath to UTF16: %w", err)
	}
	to, err := windows.UTF16PtrFromString(newpath)
	if err != nil {
		return fmt.Errorf("failed to convert newpath to UTF16: %w", err)
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING); err != nil {
		return fmt.Errorf("MoveFileEx failed: %w", err)
	}
	return nil
}
