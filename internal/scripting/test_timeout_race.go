//go:build race

package scripting

import "time"

// defaultTimeout is larger for race-enabled builds because the runtime may
// slow down scheduling and IO.
var defaultTimeout = 60 * time.Second
