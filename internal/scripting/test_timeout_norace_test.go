//go:build !race

package scripting

import "time"

// defaultTimeout is used by tests which need to wait for interactive
// components to complete. Non-race builds are quicker so keep it shorter.
//
//lint:ignore U1000 Unused depending on env.
var defaultTimeout = 50 * time.Second
