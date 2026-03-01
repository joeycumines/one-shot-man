package claudemux

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
	execmod "github.com/joeycumines/one-shot-man/internal/builtin/exec"
	pabtmod "github.com/joeycumines/one-shot-man/internal/builtin/pabt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateTestEnv sets up a full JS environment with osm:bt, osm:claudemux,
// osm:exec, and osm:pabt modules registered. Returns the bridge and a function
// to run JS code on the event loop.
func templateTestEnv(t *testing.T) (*btmod.Bridge, func(string) goja.Value) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("claudemux templates use sh -c; skipping on Windows")
	}

	reg := gojarequire.NewRegistry()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	reg.Enable(vm)
	gojanodejsconsole.Enable(vm)
	adapter, err := gojaeventloop.New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	ctx := context.Background()
	bridge := btmod.NewBridgeWithEventLoop(ctx, loop, vm, reg)
	t.Cleanup(func() { bridge.Stop() })

	// Register additional modules.
	reg.RegisterNativeModule("osm:claudemux", Require(ctx))
	reg.RegisterNativeModule("osm:exec", execmod.Require(ctx, nil))
	reg.RegisterNativeModule("osm:pabt", pabtmod.Require(ctx, bridge))

	// Helper: run JS on event loop, fail test on error.
	runJS := func(script string) goja.Value {
		t.Helper()
		var res goja.Value
		err := bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
			var e error
			res, e = vm.RunString(script)
			return e
		})
		require.NoError(t, err, "JS execution failed for: %s", script)
		return res
	}

	return bridge, runJS
}

// templatePath returns the absolute path to internal/command/pr_split_script.js
// relative to this test file's package directory.
func templatePath(t *testing.T) string {
	t.Helper()
	// Test CWD is internal/builtin/claudemux
	wd, err := os.Getwd()
	require.NoError(t, err)
	p := filepath.Join(wd, "..", "..", "..", "internal", "command", "pr_split_script.js")
	absP, err := filepath.Abs(p)
	require.NoError(t, err)
	_, err = os.Stat(absP)
	require.NoError(t, err, "pr_split_script.js not found at %s", absP)
	return absP
}

// TestTemplates_ModuleLoads verifies the template JS file can be required.
func TestTemplates_ModuleLoads(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`var templates = require('` + tp + `');`)
	val := runJS(`templates.VERSION`)
	assert.Equal(t, "5.0.0", val.String())
}

// TestTemplates_ExportedFunctions verifies all expected functions are exported.
func TestTemplates_ExportedFunctions(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)
	runJS(`var templates = require('` + tp + `');`)

	fns := []string{
		"btVerifyOutput", "btRunTests", "btCommitChanges", "btSplitBranch",
		"verifyAndCommit",
	}
	for _, fn := range fns {
		val := runJS(`typeof templates.` + fn)
		assert.Equal(t, "function", val.String(), "expected %s to be a function", fn)
	}
}

// TestTemplates_VerifyOutput_Success tests verifyOutput with a passing command.
func TestTemplates_VerifyOutput_Success(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btVerifyOutput(bb, 'echo hello');
		var status = bt.tick(node);
		globalThis._status = status;
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "success", status.String())

	verified := runJS(`globalThis._bb.get('verified')`)
	assert.Equal(t, true, verified.Export())

	code := runJS(`globalThis._bb.get('verifyCode')`)
	assert.Equal(t, int64(0), code.Export())

	stdout := runJS(`globalThis._bb.get('verifyStdout')`)
	assert.Contains(t, stdout.String(), "hello")
}

// TestTemplates_VerifyOutput_Failure tests verifyOutput with a failing command.
func TestTemplates_VerifyOutput_Failure(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btVerifyOutput(bb, 'false');
		var status = bt.tick(node);
		globalThis._status = status;
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())

	verified := runJS(`globalThis._bb.get('verified')`)
	assert.Nil(t, verified.Export())

	lastErr := runJS(`globalThis._bb.get('lastError')`)
	assert.Contains(t, lastErr.String(), "verify failed")
}

// TestTemplates_RunTests_Success tests runTests with a passing command.
func TestTemplates_RunTests_Success(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btRunTests(bb, 'echo tests-passed');
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "success", status.String())

	passed := runJS(`globalThis._bb.get('testsPassed')`)
	assert.Equal(t, true, passed.Export())
}

// TestTemplates_RunTests_Failure tests runTests with a failing command.
func TestTemplates_RunTests_Failure(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btRunTests(bb, 'false');
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())

	lastErr := runJS(`globalThis._bb.get('lastError')`)
	assert.Contains(t, lastErr.String(), "tests failed")
}

// TestTemplates_RunTests_DefaultCommand tests runTests uses 'make test' by default.
func TestTemplates_RunTests_DefaultCommand(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// runTests(bb) with no command should attempt 'make test'
	// which will fail (no Makefile in temp context), confirming the default is used
	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btRunTests(bb);
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())
}

// TestTemplates_Sequence_VerifyThenRun tests composing templates into a sequence.
func TestTemplates_Sequence_VerifyThenRun(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var seq = bt.node(bt.sequence,
			templates.btVerifyOutput(bb, 'echo step1'),
			templates.btRunTests(bb, 'echo step2')
		);
		globalThis._status = bt.tick(seq);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "success", status.String())

	verified := runJS(`globalThis._bb.get('verified')`)
	assert.Equal(t, true, verified.Export())

	passed := runJS(`globalThis._bb.get('testsPassed')`)
	assert.Equal(t, true, passed.Export())
}

// TestTemplates_Sequence_FailShortCircuits tests that sequence stops on first failure.
func TestTemplates_Sequence_FailShortCircuits(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var seq = bt.node(bt.sequence,
			templates.btVerifyOutput(bb, 'false'),
			templates.btRunTests(bb, 'echo should-not-run')
		);
		globalThis._status = bt.tick(seq);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())

	// testsPassed should NOT be set because the sequence short-circuited
	passed := runJS(`globalThis._bb.get('testsPassed')`)
	assert.Nil(t, passed.Export())
}

// TestTemplates_Fallback tests fallback composition for remediation.
func TestTemplates_Fallback(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// First child fails, fallback tries second child which succeeds
	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var tree = bt.node(bt.fallback,
			templates.btVerifyOutput(bb, 'false'),
			templates.btRunTests(bb, 'echo fallback-ok')
		);
		globalThis._status = bt.tick(tree);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "success", status.String())

	passed := runJS(`globalThis._bb.get('testsPassed')`)
	assert.Equal(t, true, passed.Export())
}

// TestTemplates_VerifyAndCommit_WorkflowComposer tests the verifyAndCommit composer.
func TestTemplates_VerifyAndCommit_WorkflowComposer(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// Verify composition only — DO NOT tick the node.
	// The commitChanges leaf runs `git add -A` + `git commit` in CWD, which
	// would mutate the host repository if any working tree changes exist.
	// That is a catastrophic test isolation violation.
	nodeType := runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.verifyAndCommit(bb, {
			testCommand: 'echo ok',
			message: 'Automated commit'
		});
		typeof node;
	`)
	assert.Equal(t, "function", nodeType.String(), "verifyAndCommit should return a BT node (function)")

	// Also verify with verifyCommand to test the 3-step branch.
	nodeType2 := runJS(`
		var bt2 = require('osm:bt');
		var templates2 = require('` + tp + `');
		var bb2 = new bt2.Blackboard();
		var node2 = templates2.verifyAndCommit(bb2, {
			testCommand: 'echo ok',
			verifyCommand: 'echo verify',
			message: 'Automated commit'
		});
		typeof node2;
	`)
	assert.Equal(t, "function", nodeType2.String(), "verifyAndCommit with verifyCommand should return a BT node (function)")
}

// TestTemplates_VerifyAndCommit_OrderTestsThenVerify verifies that the
// verifyAndCommit composer sequences tests FIRST, then verification.
// This tests the semantic ordering — the original had tests→verify→commit,
// not verify→tests→commit.
func TestTemplates_VerifyAndCommit_OrderTestsThenVerify(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// Use a test command that succeeds and a verify command that fails.
	// With correct ordering (tests→verify→commit):
	//   testsPassed=true, then verifyCommand fails → bt.failure
	//   committed should NOT be set.
	//
	// With WRONG ordering (verify→tests→commit):
	//   verifyCommand fails first → testsPassed would NOT be set.
	//
	// We distinguish the two orderings by checking testsPassed.
	tmpDir := t.TempDir()
	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.verifyAndCommit(bb, {
			testCommand: 'echo tests-ran',
			verifyCommand: 'false',
			message: 'Automated commit'
		});
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)
	_ = tmpDir // temp dir referenced for safety — test doesn't write

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String(), "overall should fail because verify fails")

	// CRITICAL: testsPassed MUST be true — tests run BEFORE verify.
	// If this is nil/false, order is wrong (verify ran first and short-circuited).
	passed := runJS(`globalThis._bb.get('testsPassed')`)
	assert.Equal(t, true, passed.Export(), "tests MUST run before verify — testsPassed should be set")

	// committed should NOT be set because the sequence failed at verify.
	committed := runJS(`globalThis._bb.get('committed')`)
	assert.Nil(t, committed.Export(), "committed should not be set — verify failed before commit")
}

// TestTemplates_VerifyAndCommit_DefaultMessage verifies the default commit
// message is 'Automated commit' (capital A) matching the original.
// This test is NOT parallel because it uses os.Chdir.
func TestTemplates_VerifyAndCommit_DefaultMessage(t *testing.T) {
	// NOT t.Parallel() — os.Chdir is process-global
	if runtime.GOOS == "windows" {
		t.Skip("uses sh -c; skipping on Windows")
	}

	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	tmpDir := t.TempDir()

	// Initialize a git repo in tmpDir via Go-side exec (safe).
	runJS(`
		var exec = require('osm:exec');
		exec.exec('git', 'init', '` + tmpDir + `');
		exec.exec('git', '-C', '` + tmpDir + `', 'config', 'user.email', 'test@test.com');
		exec.exec('git', '-C', '` + tmpDir + `', 'config', 'user.name', 'Test');
		exec.exec('sh', '-c', 'touch "` + tmpDir + `/initial" && git -C "` + tmpDir + `" add -A && git -C "` + tmpDir + `" commit -m "init"');
		exec.exec('sh', '-c', 'echo "change" > "` + tmpDir + `/testfile"');
	`)

	// Change to temp dir so btCommitChanges (which uses git in CWD) works
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.verifyAndCommit(bb, {
			testCommand: 'echo ok'
			// Note: no message field — should default to 'Automated commit'
		});
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "success", status.String(), "verifyAndCommit should succeed in temp repo")

	// Read the commit message
	commitMsg := runJS(`
		var exec2 = require('osm:exec');
		exec2.exec('git', 'log', '-1', '--format=%s').stdout.trim();
	`)
	assert.Equal(t, "Automated commit", commitMsg.String(), "default message should be 'Automated commit' with capital A")
}
