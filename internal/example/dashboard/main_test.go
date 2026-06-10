//go:build unix

package dashboard

import (
	"os"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestMain builds the osm binary once for all dashboard PTY integration tests.
// Environment mutation (PATH, OSM_SYNC_PROTOCOL) is confined to a subprocess
// via testutil.RunPTYSuite so the parent process is never polluted.
func TestMain(m *testing.M) {
	testutil.RunPTYSuite(m, testutil.PTYSuiteConfig{})
}

func buildTestBinary(tb testing.TB) string {
	tb.Helper()
	return testutil.BuildTestBinary(tb)
}

func isUnixPlatform() bool {
	_, err := os.Stat("/bin/sh")
	return err == nil
}
