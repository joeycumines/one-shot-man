//go:build unix

package gateway

import "syscall"

// openNoFollow refuses to follow a symlink left where the discovery
// advertisement belongs. Only Unix exposes O_NOFOLLOW; other platforms open
// without the flag (see openNoFollow's other definition).
const openNoFollow = syscall.O_NOFOLLOW
