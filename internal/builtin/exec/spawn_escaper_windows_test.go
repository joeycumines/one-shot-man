//go:build windows

package exec

import "runtime"

// runEscaperHelper is a no-op on Windows: the re-exec helper only ever runs
// on POSIX (escaperScript panics on Windows and its callers skip there), so
// the Windows test build never needs syscall.Setsid.
func runEscaperHelper() {
	if runtime.GOOS == "windows" {
		return
	}
	panic("unreachable: escaper helper runs only on unix (see spawn_escaper_unix_test.go)")
}
