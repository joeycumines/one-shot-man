//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd

package scripting

// getTerminalID is a placeholder for Windows.
// A stable terminal identifier is not easily available on Windows without
// significant effort. The system will fall back to other identifiers.
func getTerminalID() string {
	return ""
}
