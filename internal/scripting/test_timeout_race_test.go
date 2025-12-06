//go:build race

package scripting

import "time"

// defaultTimeout is larger for race-enabled builds because the runtime may
// slow down scheduling and IO.
//
//lint:ignore U1000 Unused depending on env.
var defaultTimeout = 70 * time.Second
