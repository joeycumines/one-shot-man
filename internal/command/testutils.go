package command

import "regexp"

// ansiRegex matches ANSI escape sequences — shared across test files for cross-platform compatibility.
// Handles:
//   - Standard CSI sequences:      \x1b[K, \x1b[18;73H, \x1b[>4m
//   - DEC private mode sequences:  \x1b[?1049h (alt screen), \x1b[?1006h (SGR mouse),
//     \x1b[?2004h (bracketed paste). These have an intermediate '?' between the CSI
//     introducer and parameters: ESC [ ? Ps... Letter
//
// NOTE: This regex is used to STRIP sequences from PTY buffers in test helpers.
// It is intentionally aggressive — it must NOT leave partial characters (e.g., stripping
// \x1b[?1049h to "1049h") since artifacts corrupt the parseDebugJSON normalization.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\[[0-9;]*\?[0-9;]*[a-zA-Z]`)
