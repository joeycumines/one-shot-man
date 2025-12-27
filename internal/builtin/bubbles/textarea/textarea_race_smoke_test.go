//go:build race

package textarea

import "testing"

// TestTextarea_RaceSmoke is a minimal smoke test to ensure the package
// compiles and is included when building/running with -race.
func TestTextarea_RaceSmoke(t *testing.T) {
	// empty smoke test
}
