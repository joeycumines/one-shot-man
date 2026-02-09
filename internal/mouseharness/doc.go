//go:build unix

// Package mouseharness provides test infrastructure for BubbleTea TUI mouse
// interaction testing via PTY (pseudo-terminal).
//
// This package is Unix-only. It depends on PTY support provided by
// github.com/joeycumines/go-prompt/termtest, which is not available on
// Windows. All files in this package (and its internal/dummy sub-package)
// use the //go:build unix constraint, which correctly excludes this package
// from Windows builds.
//
// The package wraps an externally-managed *termtest.Console and adds
// mouse-specific utilities including SGR mouse event generation, terminal
// buffer parsing, and element location finding. It is used exclusively by
// tests — it is not part of the production binary.
package mouseharness
