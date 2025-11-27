//go:build !race

package scripting

import "time"

// defaultTimeout is used by tests which need to wait for interactive
// components to complete. Non-race builds are quicker so keep it shorter.
var defaultTimeout = 20 * time.Second
