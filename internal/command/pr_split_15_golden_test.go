package command

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T346: Golden file tests for PR Split TUI visual regression
// ---------------------------------------------------------------------------

var updateGolden = flag.Bool("update-golden", false, "update golden files")

func testGolden(t *testing.T, name string, got string) {
	t.Helper()
	goldenFile := filepath.Join("testdata", "golden", name+".golden")

	if *updateGolden {
		dir := filepath.Dir(goldenFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("updated %s", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	if os.IsNotExist(err) {
		t.Fatalf("golden file %s does not exist; run with -update-golden to create", goldenFile)
	}
	if err != nil {
		t.Fatal(err)
	}

	if got != string(want) {
		t.Errorf("golden mismatch for %s:\n--- want ---\n%s\n--- got ---\n%s", name, string(want), got)
	}
}

// TestGolden_VerifyPane_Running renders the verify pane with an active
// session in running state and compares against the stored golden file.
func TestGolden_VerifyPane_Running(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderVerifyPane({
		verifyScreen: 'Running tests...\ntest_utils.go:15: ok\ntest_main.go:42: ok\ntest_api.go:8: FAIL',
		activeVerifySession: true,
		splitViewFocus: 'claude',
		splitViewTab: 'verify',
		verifyPaused: false,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		activeVerifyBranch: 'split/01-types',
		verifyElapsedMs: 12500
	}, 80, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	testGolden(t, "verify-pane-running", raw.(string))
}

// TestGolden_VerifyPane_Paused renders the verify pane in paused state
// and compares against the stored golden file.
func TestGolden_VerifyPane_Paused(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderVerifyPane({
		verifyScreen: 'Running tests...\ntest_utils.go:15: ok\ntest_main.go:42: ok\ntest_api.go:8: FAIL\ntest_db.go:99: ok',
		activeVerifySession: true,
		splitViewFocus: 'wizard',
		splitViewTab: 'verify',
		verifyPaused: true,
		verifyViewportOffset: 0,
		verifyAutoScroll: true,
		activeVerifyBranch: 'split/02-impl',
		verifyElapsedMs: 45200
	}, 80, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	testGolden(t, "verify-pane-paused", raw.(string))
}

// TestGolden_ShellPane_Active renders the shell pane with an active shell
// session and compares against the stored golden file.
func TestGolden_ShellPane_Active(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderShellPane({
		shellSession: true,
		shellScreen: '$ go test ./...\nok  \tpkg/types\t0.012s\nok  \tpkg/impl\t0.034s\nFAIL\tpkg/api\t0.056s\n$ ',
		splitViewFocus: 'claude',
		splitViewTab: 'shell',
		shellViewOffset: 0,
		activeVerifyWorktree: '/tmp/worktrees/split-01-types'
	}, 80, 20)`)
	if err != nil {
		t.Fatal(err)
	}
	testGolden(t, "shell-pane-active", raw.(string))
}
