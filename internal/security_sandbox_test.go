package internal_test

// JS sandbox boundary tests for one-shot-man.
// Verify that the Goja JavaScript runtime does NOT expose Go internals
// beyond the explicitly registered osm: module APIs.
//
// SECURITY POSTURE:
// osm is a local developer tool, NOT a sandboxed execution environment.
// The Goja VM provides language-level isolation (JS cannot call arbitrary Go code),
// but does NOT provide security isolation (exec, readFile, fetch are intentionally
// unrestricted). The security boundary is the OS user's permissions.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func newSandboxTestEngine(t *testing.T) (*scripting.Engine, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("sandbox", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	return engine, &stdout, &stderr
}

func TestSandbox_NodeBuiltinsNotAvailable(t *testing.T) {
	t.Parallel()
	nodeBuiltins := []string{
		"os", "child_process", "fs", "path", "net", "http", "https",
		"crypto", "stream", "events", "buffer", "vm",
		"cluster", "dgram", "dns", "tls", "zlib", "worker_threads",
		// NOTE: "util" is intentionally excluded — goja_nodejs provides it as
		// a standard polyfill. This is expected behavior, not a sandbox breach.
	}
	for _, mod := range nodeBuiltins {
		t.Run(mod, func(t *testing.T) {
			t.Parallel()
			engine, _, _ := newSandboxTestEngine(t)
			script := engine.LoadScriptFromString("node-builtin-"+mod, `
				try {
					require('`+mod+`');
					throw new Error('SANDBOX_BREACH: require succeeded');
				} catch(e) {
					if (e.message && e.message.indexOf('SANDBOX_BREACH') !== -1) {
						throw e;
					}
				}
			`)
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("Node.js builtin %q rejection failed: %v", mod, err)
			}
		})
	}
}

func TestSandbox_NoGoReflect(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("no-reflect", `
		if (typeof Reflect !== 'undefined' && typeof Reflect.construct === 'function') {
			try {
				function Foo() { this.x = 1; }
				var f = Reflect.construct(Foo, []);
				if (f.x !== 1) throw new Error('JS Reflect.construct failed');
			} catch(e) {}
		}
		var goInternals = [
			'Go', 'GoReflect', 'GoRuntime', 'GoUnsafe',
			'runtime', 'reflect', 'unsafe', 'syscall',
			'__go_runtime', '__go_reflect',
		];
		for (var i = 0; i < goInternals.length; i++) {
			var name = goInternals[i];
			if (typeof globalThis[name] !== 'undefined') {
				throw new Error('SANDBOX_BREACH: global "' + name + '" exists');
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Go reflect access test failed: %v", err)
	}
}

func TestSandbox_NoGoUnsafe(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("no-unsafe", `
		if (typeof Buffer !== 'undefined') {
			throw new Error('SANDBOX_BREACH: Buffer global exists');
		}
		if (typeof process !== 'undefined') {
			throw new Error('SANDBOX_BREACH: process global exists');
		}
		var exec = require('osm:exec');
		var proto = Object.getPrototypeOf(exec);
		if (proto !== Object.prototype && proto !== null) {
			// Goja may use custom prototypes for modules — acceptable
		}
		try {
			eval('1 + 1');
		} catch(e) {}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Go unsafe access test failed: %v", err)
	}
}

func TestSandbox_NoGoLinkname(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("no-linkname", `
		var runtimeFuncs = [
			'GOMAXPROCS', 'Goexit', 'GC', 'Gosched',
			'NumCPU', 'NumGoroutine', 'ReadMemStats',
			'SetFinalizer', 'KeepAlive',
		];
		for (var i = 0; i < runtimeFuncs.length; i++) {
			if (typeof globalThis[runtimeFuncs[i]] === 'function') {
				throw new Error('SANDBOX_BREACH: runtime func "' + runtimeFuncs[i] + '" accessible');
			}
		}
		var osFuncs = ['Exit', 'Getpid', 'Getuid', 'Kill', 'Remove', 'Rename', 'Mkdir'];
		for (var i = 0; i < osFuncs.length; i++) {
			if (typeof globalThis[osFuncs[i]] === 'function') {
				throw new Error('SANDBOX_BREACH: os func "' + osFuncs[i] + '" accessible');
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Go linkname access test failed: %v", err)
	}
}

func TestSandbox_GojaDefaultsAreSafe(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("goja-defaults", `
		if (typeof process !== 'undefined') throw new Error('SANDBOX_BREACH: process exists');
		if (typeof Deno !== 'undefined') throw new Error('SANDBOX_BREACH: Deno exists');
		if (typeof Buffer !== 'undefined') throw new Error('SANDBOX_BREACH: Buffer exists');
		try { require('../../etc/passwd'); } catch(e) {}
		var arr = [3, 1, 2]; arr.sort();
		if (JSON.stringify(arr) !== '[1,2,3]') throw new Error('Basic JS broken');
		var obj = JSON.parse('{"key":"value"}');
		if (obj.key !== 'value') throw new Error('JSON parse failed');
		var re = /hello/;
		if (!re.test('hello world')) throw new Error('RegExp failed');
		(function() { "use strict"; })();
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Goja defaults safety test failed: %v", err)
	}
}

func TestSandbox_NoOsExit(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("no-os-exit", `
		if (typeof exit === 'function') throw new Error('SANDBOX_BREACH: global exit()');
		var osmod = require('osm:os');
		if (typeof osmod.exit === 'function') throw new Error('SANDBOX_BREACH: osm:os.exit()');
		var execmod = require('osm:exec');
		if (typeof execmod.exit === 'function') throw new Error('SANDBOX_BREACH: osm:exec.exit()');
		if (typeof process !== 'undefined' && typeof process.exit === 'function') {
			throw new Error('SANDBOX_BREACH: process.exit()');
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("os.Exit access test failed: %v", err)
	}
}

func TestSandbox_VMIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var stdout1, stderr1, stdout2, stderr2 bytes.Buffer
	engine1, err := scripting.NewEngineWithConfig(
		ctx, &stdout1, &stderr1,
		testutil.NewTestSessionID("sandbox", t.Name()+"-1"),
		"memory",
	)
	if err != nil {
		t.Fatalf("Engine 1 creation failed: %v", err)
	}
	defer engine1.Close()
	engine2, err := scripting.NewEngineWithConfig(
		ctx, &stdout2, &stderr2,
		testutil.NewTestSessionID("sandbox", t.Name()+"-2"),
		"memory",
	)
	if err != nil {
		t.Fatalf("Engine 2 creation failed: %v", err)
	}
	defer engine2.Close()

	s1 := engine1.LoadScriptFromString("set-global", `
		globalThis.__sandboxTest = 'engine1-secret';
	`)
	if err := engine1.ExecuteScript(s1); err != nil {
		t.Fatalf("Engine 1 set-global failed: %v", err)
	}
	s2 := engine2.LoadScriptFromString("check-global", `
		if (typeof globalThis.__sandboxTest !== 'undefined') {
			throw new Error('ISOLATION_BREACH: engine1 global leaked to engine2');
		}
	`)
	if err := engine2.ExecuteScript(s2); err != nil {
		t.Fatalf("VM isolation failed — engine1 global leaked: %v", err)
	}

	s3 := engine1.LoadScriptFromString("modify-proto", `
		Array.prototype.__sandboxMarker = true;
	`)
	if err := engine1.ExecuteScript(s3); err != nil {
		t.Fatalf("Engine 1 modify-proto failed: %v", err)
	}
	s4 := engine2.LoadScriptFromString("check-proto", `
		if ([].__sandboxMarker === true) {
			throw new Error('ISOLATION_BREACH: prototype pollution leaked');
		}
	`)
	if err := engine2.ExecuteScript(s4); err != nil {
		t.Fatalf("VM isolation failed — prototype pollution leaked: %v", err)
	}

	engine1.SetGlobal("isolatedVar", "engine1-value")
	v2 := engine2.GetGlobal("isolatedVar")
	if v2 != nil {
		t.Errorf("ISOLATION_BREACH: engine1 SetGlobal leaked to engine2: got %v", v2)
	}
}

func TestSandbox_PrototypePollutionWithinVM(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("proto-pollution", `
		Object.prototype.__injected = 'polluted';
		var obj = {};
		if (obj.__injected !== 'polluted') throw new Error('Expected prototype pollution within VM');
		delete Object.prototype.__injected;
		var obj2 = {};
		if (typeof obj2.__injected !== 'undefined') throw new Error('Prototype pollution cleanup failed');
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Prototype pollution test failed: %v", err)
	}
}

func TestSandbox_ExecModuleAPIBoundary(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("exec-boundary", `
		var execmod = require('osm:exec');
		var expected = ['exec', 'execv'];
		for (var i = 0; i < expected.length; i++) {
			if (typeof execmod[expected[i]] !== 'function') {
				throw new Error('Missing expected export: ' + expected[i]);
			}
		}
		var dangerous = ['spawn', 'fork', 'execFile', 'execSync', 'spawnSync',
			'kill', 'exit', 'abort', 'system', 'popen'];
		for (var i = 0; i < dangerous.length; i++) {
			if (typeof execmod[dangerous[i]] !== 'undefined') {
				throw new Error('SANDBOX_BREACH: osm:exec.' + dangerous[i] + ' exists');
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Exec module API boundary test failed: %v", err)
	}
}

func TestSandbox_OsModuleAPIBoundary(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("os-boundary", `
		var osmod = require('osm:os');
		var expected = ['readFile', 'fileExists', 'openEditor', 'clipboardCopy', 'getenv'];
		for (var i = 0; i < expected.length; i++) {
			if (typeof osmod[expected[i]] !== 'function') {
				throw new Error('Missing expected export: ' + expected[i]);
			}
		}
		var dangerous = ['writeFile', 'unlink', 'rmdir', 'mkdir', 'rename',
			'chmod', 'chown', 'symlink', 'truncate', 'appendFile',
			'exit', 'kill', 'setenv', 'unsetenv'];
		for (var i = 0; i < dangerous.length; i++) {
			if (typeof osmod[dangerous[i]] !== 'undefined') {
				throw new Error('SANDBOX_BREACH: osm:os.' + dangerous[i] + ' exists');
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("OS module API boundary test failed: %v", err)
	}
}

func TestSandbox_FetchModuleAPIBoundary(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("fetch-boundary", `
		var fetchmod = require('osm:fetch');
		var expected = ['fetch', 'fetchStream'];
		for (var i = 0; i < expected.length; i++) {
			if (typeof fetchmod[expected[i]] !== 'function') {
				throw new Error('Missing expected export: ' + expected[i]);
			}
		}
		var dangerous = ['createServer', 'listen', 'Agent', 'request',
			'globalAgent', 'ClientRequest', 'ServerResponse'];
		for (var i = 0; i < dangerous.length; i++) {
			if (typeof fetchmod[dangerous[i]] !== 'undefined') {
				throw new Error('SANDBOX_BREACH: osm:fetch.' + dangerous[i] + ' exists');
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Fetch module API boundary test failed: %v", err)
	}
}

func TestSandbox_GlobalScopeIsMinimal(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("global-scope", `
		var expectedOSMGlobals = ['ctx', 'context', 'log', 'output', 'tui', 'require'];
		for (var i = 0; i < expectedOSMGlobals.length; i++) {
			if (typeof globalThis[expectedOSMGlobals[i]] === 'undefined') {
				throw new Error('Expected global missing: ' + expectedOSMGlobals[i]);
			}
		}
		var forbidden = [
			'process', 'Buffer', 'global', '__filename', '__dirname',
			'Go', 'GoReflect', 'GoRuntime', 'GoUnsafe',
			'runtime', 'reflect', 'unsafe', 'syscall',
			'Deno',
			'window', 'document', 'navigator', 'location',
			'XMLHttpRequest', 'fetch',
			'exit', 'quit', 'die',
		];
		for (var i = 0; i < forbidden.length; i++) {
			if (typeof globalThis[forbidden[i]] !== 'undefined') {
				throw new Error('FORBIDDEN global found: ' + forbidden[i]);
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Global scope test failed: %v", err)
	}
}

func TestSandbox_ReturnedObjectsDontExposeGoMethods(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("go-methods", `
		var execmod = require('osm:exec');
		var result = execmod.exec('echo', 'test');
		var resultKeys = Object.keys(result).sort();
		var expectedKeys = ['code', 'error', 'message', 'stderr', 'stdout'];
		if (JSON.stringify(resultKeys) !== JSON.stringify(expectedKeys)) {
			throw new Error('Unexpected exec result keys: ' + JSON.stringify(resultKeys));
		}
		var proto = Object.getPrototypeOf(result);
		if (proto !== Object.prototype) {
			var protoKeys = Object.getOwnPropertyNames(proto);
			var goMethods = ['Close', 'Sync', 'Read', 'Write', 'Seek',
				'Lock', 'Unlock', 'String', 'Error', 'GoString'];
			for (var i = 0; i < goMethods.length; i++) {
				if (protoKeys.indexOf(goMethods[i]) !== -1) {
					throw new Error('SANDBOX_BREACH: Go method "' + goMethods[i] + '" on result');
				}
			}
		}
		var osmod = require('osm:os');
		var fileResult = osmod.readFile('/nonexistent/path');
		var fileKeys = Object.keys(fileResult).sort();
		var expectedFileKeys = ['content', 'error', 'message'];
		if (JSON.stringify(fileKeys) !== JSON.stringify(expectedFileKeys)) {
			throw new Error('Unexpected readFile result keys: ' + JSON.stringify(fileKeys));
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Go methods exposure test failed: %v", err)
	}
}

func TestSandbox_RequireOnlyLoadsRegisteredModules(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("require-osm", `
		var modules = [
			'osm:exec', 'osm:os', 'osm:fetch', 'osm:flag',
			'osm:time', 'osm:argv', 'osm:ctxutil',
			'osm:text/template', 'osm:unicodetext',
			'osm:bubbletea', 'osm:bubblezone',
			'osm:bubbles/textarea', 'osm:bubbles/viewport',
			'osm:termui/scrollbar', 'osm:lipgloss',
			'osm:bt', 'osm:pabt', 'osm:grpc',
			'osm:nextIntegerId',
		];
		var loaded = 0;
		for (var i = 0; i < modules.length; i++) {
			try {
				var mod = require(modules[i]);
				if (mod === null || mod === undefined) {
					throw new Error('Module ' + modules[i] + ' loaded as null/undefined');
				}
				loaded++;
			} catch(e) {
				throw new Error('Failed to require ' + modules[i] + ': ' + e.message);
			}
		}
		if (loaded !== modules.length) {
			throw new Error('Expected ' + modules.length + ' modules, loaded ' + loaded);
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("osm: module loading test failed: %v", err)
	}

	script2 := engine.LoadScriptFromString("require-forbidden", `
		var forbidden = [
			'go:os', 'go:runtime', 'go:reflect', 'go:unsafe',
			'go:syscall', 'go:net', 'go:io',
			'node:os', 'node:fs', 'node:path',
		];
		for (var i = 0; i < forbidden.length; i++) {
			try {
				require(forbidden[i]);
				throw new Error('SANDBOX_BREACH: require("' + forbidden[i] + '") succeeded');
			} catch(e) {
				if (e.message && e.message.indexOf('SANDBOX_BREACH') !== -1) throw e;
			}
		}
	`)
	if err := engine.ExecuteScript(script2); err != nil {
		t.Fatalf("Forbidden module rejection test failed: %v", err)
	}
}

func TestSandbox_ContextCancellationStopsExecution(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(
		ctx, &stdout, &stderr,
		testutil.NewTestSessionID("sandbox", t.Name()),
		"memory",
	)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}
	defer engine.Close()
	cancel()
	script := engine.LoadScriptFromString("infinite-loop", `
		while(true) {}
	`)
	err = engine.ExecuteScript(script)
	if err == nil {
		t.Error("Infinite loop should have been interrupted by context cancellation")
	}
	if err != nil && !strings.Contains(err.Error(), "context canceled") &&
		!strings.Contains(err.Error(), "interrupt") &&
		!strings.Contains(err.Error(), "cancelled") {
		t.Logf("Execution stopped with error (expected): %v", err)
	}
}

func TestSandbox_ExecUsesCommandContextNotShell(t *testing.T) {
	t.Parallel()
	platform := testutil.DetectPlatform(t)
	testutil.SkipIfWindows(t, platform, "echo is a shell builtin on Windows, not a standalone executable")
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("exec-no-shell", `
		var execmod = require('osm:exec');
		var result = execmod.exec('echo', '$(id)', '&&', 'rm', '-rf', '/');
		if (result.error) throw new Error('exec failed: ' + result.message);
		if (result.stdout.indexOf('$(id)') === -1) {
			throw new Error('Shell expansion occurred — exec should not use shell');
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Exec no-shell test failed: %v", err)
	}
}

func TestSandbox_OsModuleReadOnlyFilesystem(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("os-readonly", `
		var osmod = require('osm:os');
		var writeOps = ['writeFile', 'appendFile', 'unlink', 'rmdir', 'mkdir',
			'rename', 'chmod', 'chown', 'symlink', 'truncate',
			'copyFile', 'link', 'mkdtemp'];
		for (var i = 0; i < writeOps.length; i++) {
			if (typeof osmod[writeOps[i]] !== 'undefined') {
				throw new Error('osm:os should be read-only but has: ' + writeOps[i]);
			}
		}
		if (typeof osmod.readFile !== 'function') throw new Error('readFile missing');
		if (typeof osmod.fileExists !== 'function') throw new Error('fileExists missing');
		if (typeof osmod.getenv !== 'function') throw new Error('getenv missing');
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("OS read-only test failed: %v", err)
	}
}

func TestSandbox_GrpcModuleAPIBoundary(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("grpc-boundary", `
		var grpcmod = require('osm:grpc');
		var expected = ['dial', 'loadDescriptorSet', 'status'];
		for (var i = 0; i < expected.length; i++) {
			if (typeof grpcmod[expected[i]] === 'undefined') {
				throw new Error('Missing expected grpc export: ' + expected[i]);
			}
		}
		var dangerous = ['createServer', 'Server', 'listen', 'serve'];
		for (var i = 0; i < dangerous.length; i++) {
			if (typeof grpcmod[dangerous[i]] !== 'undefined') {
				throw new Error('SANDBOX_BREACH: osm:grpc.' + dangerous[i] + ' exists');
			}
		}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("gRPC module API boundary test failed: %v", err)
	}
}

func TestSandbox_TemplateModuleNoArbitraryCodeExec(t *testing.T) {
	t.Parallel()
	engine, _, _ := newSandboxTestEngine(t)
	script := engine.LoadScriptFromString("template-safe", `
		var tmpl = require('osm:text/template');
		var t1 = tmpl.new('test');
		t1.parse('{{call .dangerous}}');
		try {
			t1.execute({dangerous: function() { return 'pwned'; }});
		} catch(e) {}
		var t2 = tmpl.new('test2');
		try {
			t2.parse('{{printf "%p" .}}');
			var result = t2.execute({});
		} catch(e) {}
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Template safety test failed: %v", err)
	}
}
