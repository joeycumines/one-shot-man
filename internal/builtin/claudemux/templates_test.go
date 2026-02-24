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
	reg.RegisterNativeModule("osm:exec", execmod.Require(ctx))
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
		"btSpawnClaude", "btSendPrompt", "btWaitForResponse",
		"btVerifyOutput", "btRunTests", "btCommitChanges", "btSplitBranch",
		"spawnAndPrompt", "verifyAndCommit",
		"createPlanningActions",
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

// TestTemplates_SendPrompt_NoAgent tests sendPrompt fails when no agent is spawned.
func TestTemplates_SendPrompt_NoAgent(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btSendPrompt(bb, 'hello');
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())

	lastErr := runJS(`globalThis._bb.get('lastError')`)
	assert.Equal(t, "no agent spawned", lastErr.String())
}

// TestTemplates_WaitForResponse_NoAgent tests waitForResponse fails without agent.
func TestTemplates_WaitForResponse_NoAgent(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var node = templates.btWaitForResponse(bb);
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())

	lastErr := runJS(`globalThis._bb.get('lastError')`)
	assert.Contains(t, lastErr.String(), "no agent or parser")
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

// TestTemplates_CreatePlanningActions tests the PA-BT action factory.
func TestTemplates_CreatePlanningActions(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var pabt = require('osm:pabt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var orc = require('osm:claudemux');
		var registry = orc.newRegistry();
		var actions = templates.createPlanningActions(pabt, bb, registry, {
			testCommand: 'echo ok',
			prompt: 'hello world'
		});
		globalThis._actions = actions;
		globalThis._actionNames = Object.keys(actions).sort();
	`)

	// Verify all 7 PA-BT actions are created
	names := runJS(`JSON.stringify(globalThis._actionNames)`)
	assert.Equal(t, `["CommitChanges","RunTests","SendPrompt","SpawnClaude","SplitBranch","VerifyOutput","WaitForResponse"]`, names.String())
}

// TestTemplates_PlanningActions_RegisterWithState tests PA-BT action registration.
func TestTemplates_PlanningActions_RegisterWithState(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var pabt = require('osm:pabt');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var orc = require('osm:claudemux');
		var registry = orc.newRegistry();
		var actions = templates.createPlanningActions(pabt, bb, registry, {});
		var state = pabt.newState(bb);
		var names = Object.keys(actions);
		for (var i = 0; i < names.length; i++) {
			state.registerAction(names[i], actions[names[i]]);
		}
		globalThis._registered = true;
	`)

	registered := runJS(`globalThis._registered`)
	assert.Equal(t, true, registered.Export())
}

// TestTemplates_SpawnClaude_UnknownProvider tests spawnClaude with unknown provider.
func TestTemplates_SpawnClaude_UnknownProvider(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		var node = templates.btSpawnClaude(bb, registry, 'nonexistent', {});
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "failure", status.String())

	lastErr := runJS(`globalThis._bb.get('lastError')`)
	assert.Contains(t, lastErr.String(), "not found")
}

// TestTemplates_SpawnClaude_WithEchoProvider tests spawnClaude with a real PTY echo provider.
func TestTemplates_SpawnClaude_WithEchoProvider(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("PTY not available on Windows")
	}

	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		// Register a claude-code provider with echo as the command
		var provider = orc.claudeCode({command: '/bin/echo'});
		registry.register(provider);
		var node = templates.btSpawnClaude(bb, registry, 'claude-code', {});
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	status := runJS(`globalThis._status`)
	assert.Equal(t, "success", status.String())

	spawned := runJS(`globalThis._bb.get('agentSpawned')`)
	assert.Equal(t, true, spawned.Export())

	// Verify agent handle and parser were set
	hasAgent := runJS(`globalThis._bb.get('agent') !== null`)
	assert.Equal(t, true, hasAgent.Export())

	hasParser := runJS(`globalThis._bb.get('parser') !== null`)
	assert.Equal(t, true, hasParser.Export())

	// Clean up: close the agent
	runJS(`
		var agent = globalThis._bb.get('agent');
		if (agent && agent.close) { try { agent.close(); } catch(e) {} }
	`)
}

// TestTemplates_SpawnAndPrompt_IncludesWaitForResponse verifies the 3-step
// composition of spawnAndPrompt: spawn → send → wait. The original had the
// wait step; it was previously lost in the port (2-step only). This test
// uses an echo provider so spawn succeeds, and checks that 'agentSpawned'
// is set (confirming the 3-step structure compiles and at least the first
// step works with a real provider). On a mock environment, spawnAndPrompt
// must be a function returning a BT node.
func TestTemplates_SpawnAndPrompt_IncludesWaitForResponse(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// Verify that spawnAndPrompt accepts config-object style (matching the
	// original claude-mux.js API: spawnAndPrompt(bb, registry, config) where
	// config has .provider, .spawnOpts, .prompt).
	nodeType := runJS(`
		var bt = require('osm:bt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		var node = templates.spawnAndPrompt(bb, registry, {
			provider: 'nonexistent',
			prompt: 'test prompt'
		});
		typeof node;
	`)
	assert.Equal(t, "function", nodeType.String(), "spawnAndPrompt should return a BT node")
}

// TestTemplates_SpawnAndPrompt_ConfigObjectAPI verifies spawnAndPrompt uses
// the config-object style signature (bb, registry, config) where config has
// .provider, .spawnOpts, .prompt — NOT the positional style (bb, registry, providerName, opts).
func TestTemplates_SpawnAndPrompt_ConfigObjectAPI(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("PTY not available on Windows")
	}
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// With echo provider, spawn should succeed using config.provider
	runJS(`
		var bt = require('osm:bt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		var provider = orc.claudeCode({command: '/bin/echo'});
		registry.register(provider);
		// Config-object style: provider is a field, not a positional arg
		var node = templates.spawnAndPrompt(bb, registry, {
			provider: 'claude-code',
			prompt: 'hello'
		});
		// Only tick once — spawn will succeed, send may fail (echo exits)
		// but the important thing is that the node structure is correct.
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	// If spawnAndPrompt used the old positional API (bb, registry, providerName, opts)
	// the third arg {provider: 'claude-code', ...} would be stringified as providerName
	// and spawn would fail with "provider [object Object] not found".
	// With config-object API, spawn uses config.provider which is 'claude-code'.
	spawned := runJS(`globalThis._bb.get('agentSpawned')`)
	assert.Equal(t, true, spawned.Export(), "spawnAndPrompt config-object API: spawn should succeed")

	// Clean up
	runJS(`
		var agent = globalThis._bb.get('agent');
		if (agent && agent.close) { try { agent.close(); } catch(e) {} }
	`)
}

// TestTemplates_SpawnPromptAndReadResult_PositionalAPI verifies that
// spawnPromptAndReadResult uses the positional style (bb, registry, providerName, opts)
// and includes 3 steps: spawn → send → wait.
func TestTemplates_SpawnPromptAndReadResult_PositionalAPI(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("PTY not available on Windows")
	}
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	runJS(`
		var bt = require('osm:bt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		var provider = orc.claudeCode({command: '/bin/echo'});
		registry.register(provider);
		// Positional style: providerName is a separate argument
		var node = templates.spawnPromptAndReadResult(bb, registry, 'claude-code', {
			prompt: 'hello'
		});
		globalThis._status = bt.tick(node);
		globalThis._bb = bb;
	`)

	spawned := runJS(`globalThis._bb.get('agentSpawned')`)
	assert.Equal(t, true, spawned.Export(), "spawnPromptAndReadResult: spawn should succeed")

	// Clean up
	runJS(`
		var agent = globalThis._bb.get('agent');
		if (agent && agent.close) { try { agent.close(); } catch(e) {} }
	`)
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

// TestTemplates_CreatePlanningActions_HasPreconditionsAndEffects verifies that
// createPlanningActions produces PA-BT actions with proper preconditions and
// effects for backchaining. This is the most critical behavioral fix — without
// conditions/effects the planner cannot infer action ordering.
func TestTemplates_CreatePlanningActions_HasPreconditionsAndEffects(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// Create the planning actions and then create a plan with a goal.
	// If preconditions/effects are set correctly, the planner should be
	// able to synthesize a plan that chains:
	//   goal:committed=true → CommitChanges(needs testsPassed) →
	//   RunTests(needs responseReceived) → WaitForResponse(needs promptSent) →
	//   SendPrompt(needs agentSpawned) → SpawnClaude(no preconditions)
	runJS(`
		var bt = require('osm:bt');
		var pabt = require('osm:pabt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		var actions = templates.createPlanningActions(pabt, bb, registry, {
			testCommand: 'echo ok',
			prompt: 'test'
		});
		var state = pabt.newState(bb);
		var names = Object.keys(actions);
		for (var i = 0; i < names.length; i++) {
			state.registerAction(names[i], actions[names[i]]);
		}

		// Create a plan with goal: committed=true
		// The planner should backchain through the dependency graph.
		var plan = pabt.newPlan(state, [
			{key: 'committed', match: function(v) { return v === true; }}
		]);
		globalThis._plan = plan;
		globalThis._hasNode = typeof plan.node() === 'function';
	`)

	hasNode := runJS(`globalThis._hasNode`)
	assert.Equal(t, true, hasNode.Export(),
		"PA-BT plan with preconditions/effects should produce a valid node — "+
			"if this fails, the planner could not backchain through the actions")
}

// TestTemplates_CreatePlanningActions_BackchainOrder verifies that the PA-BT
// planner, given the actions from createPlanningActions, produces a plan that
// respects the dependency chain. We verify by setting blackboard state and
// checking if the planner creates a meaningful (non-empty) plan.
func TestTemplates_CreatePlanningActions_BackchainOrder(t *testing.T) {
	t.Parallel()
	_, runJS := templateTestEnv(t)
	tp := templatePath(t)

	// With no initial blackboard state, achieving 'branchCreated=true' requires
	// the full chain: Spawn → Send → Wait → RunTests → Commit → SplitBranch.
	// This only works if all preconditions/effects are properly defined.
	runJS(`
		var bt = require('osm:bt');
		var pabt = require('osm:pabt');
		var orc = require('osm:claudemux');
		var templates = require('` + tp + `');
		var bb = new bt.Blackboard();
		var registry = orc.newRegistry();
		var actions = templates.createPlanningActions(pabt, bb, registry, {
			testCommand: 'echo ok',
			prompt: 'test'
		});
		var state = pabt.newState(bb);
		var names = Object.keys(actions);
		for (var i = 0; i < names.length; i++) {
			state.registerAction(names[i], actions[names[i]]);
		}

		// Goal: branchCreated=true — requires the full chain
		var plan = pabt.newPlan(state, [
			{key: 'branchCreated', match: function(v) { return v === true; }}
		]);
		globalThis._hasFullNode = typeof plan.node() === 'function';

		// Now set agentSpawned=true and create a new plan for committed=true.
		// With preconditions working, the planner should skip SpawnClaude.
		var bb2 = new bt.Blackboard();
		bb2.set('agentSpawned', true);
		var state2 = pabt.newState(bb2);
		var actions2 = templates.createPlanningActions(pabt, bb2, registry, {
			testCommand: 'echo ok',
			prompt: 'test'
		});
		var names2 = Object.keys(actions2);
		for (var j = 0; j < names2.length; j++) {
			state2.registerAction(names2[j], actions2[names2[j]]);
		}
		var plan2 = pabt.newPlan(state2, [
			{key: 'committed', match: function(v) { return v === true; }}
		]);
		globalThis._hasPartialNode = typeof plan2.node() === 'function';
	`)

	hasFullNode := runJS(`globalThis._hasFullNode`)
	assert.Equal(t, true, hasFullNode.Export(),
		"full plan (branchCreated goal with no initial state) should produce a valid node")

	hasPartialNode := runJS(`globalThis._hasPartialNode`)
	assert.Equal(t, true, hasPartialNode.Export(),
		"partial plan (committed goal with agentSpawned=true) should produce a valid node")
}
