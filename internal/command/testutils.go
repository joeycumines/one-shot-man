//go:build unix

package command

import "regexp"

// ansiRegex matches ANSI escape sequences - shared across test files for cross-platform compatibility
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
