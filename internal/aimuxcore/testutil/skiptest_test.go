package testutil

import (
	"testing"
)

func SkipSlow(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}
}
