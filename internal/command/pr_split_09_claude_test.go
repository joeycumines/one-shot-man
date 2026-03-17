package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  Chunk 09: Claude — ClaudeCodeExecutor, prompts, detectLanguage
//  Tests use mock exec since we cannot invoke real Claude/Ollama in unit tests.
// ---------------------------------------------------------------------------

var claudeChunks = []string{
	"00_core", "01_analysis", "02_grouping", "03_planning",
	"04_validation", "05_execution", "06_verification", "07_prcreation",
	"08_conflict", "09_claude",
}

func TestChunk09_ClaudeCodeExecutor_Construct(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var ex = new globalThis.prSplit.ClaudeCodeExecutor({
				claudeCommand: '/usr/bin/test-claude',
				claudeArgs: ['--foo'],
				claudeModel: 'gpt4',
				claudeConfigDir: '/tmp/conf',
				claudeEnv: { KEY: 'val' }
			});
			return JSON.stringify({
				command: ex.command,
				argsLen: ex.args.length,
				model: ex.model,
				configDir: ex.configDir,
				hasEnv: !!ex.env.KEY,
				handleNull: ex.handle === null,
				sessionIdNull: ex.sessionId === null
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Command       string `json:"command"`
		ArgsLen       int    `json:"argsLen"`
		Model         string `json:"model"`
		ConfigDir     string `json:"configDir"`
		HasEnv        bool   `json:"hasEnv"`
		HandleNull    bool   `json:"handleNull"`
		SessionIdNull bool   `json:"sessionIdNull"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Command != "/usr/bin/test-claude" {
		t.Errorf("command = %q", data.Command)
	}
	if data.ArgsLen != 1 {
		t.Errorf("argsLen = %d", data.ArgsLen)
	}
	if data.Model != "gpt4" {
		t.Errorf("model = %q", data.Model)
	}
	if !data.HasEnv {
		t.Error("expected env.KEY")
	}
	if !data.HandleNull || !data.SessionIdNull {
		t.Error("expected handle and sessionId null before spawn")
	}
}

func TestChunk09_ClaudeCodeExecutor_Resolve_ExplicitNotFound(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var origExecv = globalThis.prSplit._modules.exec.execv;
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'which') {
					return { code: 1, stdout: '', stderr: 'not found' };
				}
				return origExecv(args);
			};
			var ex = new globalThis.prSplit.ClaudeCodeExecutor({ claudeCommand: 'nonexistent' });
			var r = ex.resolve();
			globalThis.prSplit._modules.exec.execv = origExecv;
			return JSON.stringify({ error: r.error || '' });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error == "" {
		t.Error("expected error when command not found")
	}
}

func TestChunk09_ClaudeCodeExecutor_Resolve_ExplicitFound(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var origExecv = globalThis.prSplit._modules.exec.execv;
			globalThis.prSplit._modules.exec.execv = function(args) {
				if (args[0] === 'which' && args[1] === 'my-claude') {
					return { code: 0, stdout: '/usr/bin/my-claude\n', stderr: '' };
				}
				return origExecv(args);
			};
			var ex = new globalThis.prSplit.ClaudeCodeExecutor({ claudeCommand: 'my-claude' });
			var r = ex.resolve();
			globalThis.prSplit._modules.exec.execv = origExecv;
			return JSON.stringify({
				error: r.error,
				resolvedType: ex.resolved ? ex.resolved.type : '',
				resolvedCommand: ex.resolved ? ex.resolved.command : ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error           *string `json:"error"`
		ResolvedType    string  `json:"resolvedType"`
		ResolvedCommand string  `json:"resolvedCommand"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error != nil {
		t.Errorf("unexpected error: %v", *data.Error)
	}
	if data.ResolvedType != "explicit" {
		t.Errorf("expected type 'explicit', got %q", data.ResolvedType)
	}
	if data.ResolvedCommand != "my-claude" {
		t.Errorf("expected command 'my-claude', got %q", data.ResolvedCommand)
	}
}

func TestChunk09_DetectLanguage(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var dl = globalThis.prSplit.detectLanguage;
			return JSON.stringify({
				go: dl(['main.go', 'util.go', 'readme.md']),
				js: dl(['app.js', 'index.ts', 'style.css']),
				empty: dl([]),
				nil: dl(null),
				unknown: dl(['data.csv', 'config.yaml'])
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Go      string `json:"go"`
		JS      string `json:"js"`
		Empty   string `json:"empty"`
		Nil     string `json:"nil"`
		Unknown string `json:"unknown"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Go != "Go" {
		t.Errorf("expected 'Go' for go files, got %q", data.Go)
	}
	if data.JS != "JavaScript" && data.JS != "TypeScript" {
		t.Errorf("expected 'JavaScript' or 'TypeScript' for js/ts files, got %q", data.JS)
	}
	if data.Empty != "unknown" {
		t.Errorf("expected 'unknown' for empty files, got %q", data.Empty)
	}
	if data.Nil != "unknown" {
		t.Errorf("expected 'unknown' for null, got %q", data.Nil)
	}
	if data.Unknown != "unknown" {
		t.Errorf("expected 'unknown' for unrecognized extensions, got %q", data.Unknown)
	}
}

func TestChunk09_RenderConflictPrompt(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.renderConflictPrompt({
				branchName: 'split/01-types',
				files: ['types.go', 'types_test.go'],
				exitCode: 2,
				errorOutput: 'undefined: Foo'
			});
			return JSON.stringify({
				hasText: r.text.length > 0,
				error: r.error,
				containsBranch: r.text.indexOf('split/01-types') >= 0,
				containsError: r.text.indexOf('undefined: Foo') >= 0,
				containsSession: r.text.indexOf('test-session-123') >= 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		HasText         bool    `json:"hasText"`
		Error           *string `json:"error"`
		ContainsBranch  bool    `json:"containsBranch"`
		ContainsError   bool    `json:"containsError"`
		ContainsSession bool    `json:"containsSession"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error != nil {
		t.Fatalf("render error: %v", *data.Error)
	}
	if !data.HasText {
		t.Error("expected non-empty prompt text")
	}
	if !data.ContainsBranch {
		t.Error("prompt should contain branch name")
	}
	if !data.ContainsError {
		t.Error("prompt should contain error output")
	}
	// T34: session IDs removed from prompts.
	if data.ContainsSession {
		t.Error("prompt must NOT contain session ID (removed per T34)")
	}
}

// ---- T13: ClaudeCodeExecutor model-not-available scenarios ----------------

func TestChunk09_ClaudeCodeExecutor_ModelNotAvailable(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	t.Run("CommandNotFound", func(t *testing.T) {
		result, err := evalJS(`
			(function() {
				var origExecv = globalThis.prSplit._modules.exec.execv;
				globalThis.prSplit._modules.exec.execv = function(args) {
					if (args[0] === 'which') return { code: 1, stdout: '', stderr: 'not found' };
					return origExecv(args);
				};
				var ex = new globalThis.prSplit.ClaudeCodeExecutor({ claudeCommand: '/opt/nonexistent-claude' });
				var r = ex.resolve();
				globalThis.prSplit._modules.exec.execv = origExecv;
				return JSON.stringify({ error: r.error || '', resolved: !!ex.resolved });
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Error    string `json:"error"`
			Resolved bool   `json:"resolved"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
			t.Fatal(err)
		}
		if data.Error == "" {
			t.Fatal("expected error for command not found")
		}
		if !strings.Contains(data.Error, "/opt/nonexistent-claude") {
			t.Errorf("error should mention the command, got %q", data.Error)
		}
		if data.Resolved {
			t.Error("resolved should be null/false")
		}
	})

	t.Run("ClaudeVersionCheckFails", func(t *testing.T) {
		result, err := evalJS(`
			(function() {
				var origExecv = globalThis.prSplit._modules.exec.execv;
				globalThis.prSplit._modules.exec.execv = function(args) {
					if (args[0] === 'which' && args[1] === 'claude') {
						return { code: 0, stdout: '/usr/bin/claude\n', stderr: '' };
					}
					if (args[0] === 'claude' && args[1] === '--version') {
						return { code: 1, stdout: '', stderr: 'segfault' };
					}
					// No ollama.
					if (args[0] === 'which') return { code: 1, stdout: '', stderr: '' };
					return origExecv(args);
				};
				var ex = new globalThis.prSplit.ClaudeCodeExecutor({});
				var r = ex.resolve();
				globalThis.prSplit._modules.exec.execv = origExecv;
				return JSON.stringify({ error: r.error || '' });
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
			t.Fatal(err)
		}
		if data.Error == "" {
			t.Fatal("expected error for version check failure")
		}
		if !strings.Contains(data.Error, "version check failed") {
			t.Errorf("error should mention version check, got %q", data.Error)
		}
	})

	t.Run("OllamaModelNotListed", func(t *testing.T) {
		result, err := evalJS(`
			(function() {
				var origExecv = globalThis.prSplit._modules.exec.execv;
				globalThis.prSplit._modules.exec.execv = function(args) {
					// No claude on PATH.
					if (args[0] === 'which' && args[1] === 'claude') {
						return { code: 1, stdout: '', stderr: '' };
					}
					// Ollama found.
					if (args[0] === 'which' && args[1] === 'ollama') {
						return { code: 0, stdout: '/usr/bin/ollama\n', stderr: '' };
					}
					// ollama list succeeds but model not in output.
					if (args[0] === 'ollama' && args[1] === 'list') {
						return { code: 0, stdout: 'NAME    SIZE    MODIFIED\nllama3  4.7G    2 weeks ago\n', stderr: '' };
					}
					return origExecv(args);
				};
				var ex = new globalThis.prSplit.ClaudeCodeExecutor({ claudeModel: 'nonexistent-model-42' });
				var r = ex.resolve();
				globalThis.prSplit._modules.exec.execv = origExecv;
				return JSON.stringify({ error: r.error || '' });
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
			t.Fatal(err)
		}
		if data.Error == "" {
			t.Fatal("expected error for model not listed")
		}
		if !strings.Contains(data.Error, "nonexistent-model-42") {
			t.Errorf("error should mention the model name, got %q", data.Error)
		}
		if !strings.Contains(data.Error, "not available") {
			t.Errorf("error should say model not available, got %q", data.Error)
		}
	})

	t.Run("NoBinaryFound", func(t *testing.T) {
		result, err := evalJS(`
			(function() {
				var origExecv = globalThis.prSplit._modules.exec.execv;
				globalThis.prSplit._modules.exec.execv = function(args) {
					if (args[0] === 'which') return { code: 1, stdout: '', stderr: '' };
					return origExecv(args);
				};
				var ex = new globalThis.prSplit.ClaudeCodeExecutor({});
				var r = ex.resolve();
				globalThis.prSplit._modules.exec.execv = origExecv;
				return JSON.stringify({ error: r.error || '' });
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
			t.Fatal(err)
		}
		if data.Error == "" {
			t.Fatal("expected error for no binary found")
		}
		if !strings.Contains(data.Error, "Claude") || !strings.Contains(data.Error, "Ollama") {
			t.Errorf("error should suggest both Claude and Ollama, got %q", data.Error)
		}
	})

	t.Run("OllamaResolveSuccess", func(t *testing.T) {
		result, err := evalJS(`
			(function() {
				var origExecv = globalThis.prSplit._modules.exec.execv;
				globalThis.prSplit._modules.exec.execv = function(args) {
					if (args[0] === 'which' && args[1] === 'claude') return { code: 1, stdout: '', stderr: '' };
					if (args[0] === 'which' && args[1] === 'ollama') return { code: 0, stdout: '/usr/bin/ollama\n', stderr: '' };
					if (args[0] === 'ollama' && args[1] === 'list') {
						return { code: 0, stdout: 'NAME     SIZE    MODIFIED\nllama3   4.7G    2w ago\n', stderr: '' };
					}
					return origExecv(args);
				};
				var ex = new globalThis.prSplit.ClaudeCodeExecutor({ claudeModel: 'llama3' });
				var r = ex.resolve();
				globalThis.prSplit._modules.exec.execv = origExecv;
				return JSON.stringify({
					error: r.error,
					type: ex.resolved ? ex.resolved.type : '',
					command: ex.resolved ? ex.resolved.command : ''
				});
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Error   *string `json:"error"`
			Type    string  `json:"type"`
			Command string  `json:"command"`
		}
		if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
			t.Fatal(err)
		}
		if data.Error != nil {
			t.Fatalf("unexpected error: %v", *data.Error)
		}
		if data.Type != "ollama" {
			t.Errorf("expected type 'ollama', got %q", data.Type)
		}
		if data.Command != "ollama" {
			t.Errorf("expected command 'ollama', got %q", data.Command)
		}
	})
}

func TestChunk09_RenderPrompt_NoTemplate(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	// Temporarily nullify template module to test error path.
	result, err := evalJS(`
		(function() {
			var origTemplate = globalThis.prSplit._modules.template;
			globalThis.prSplit._modules.template = null;
			var r = globalThis.prSplit.renderPrompt('hello {{.Name}}', { Name: 'world' });
			globalThis.prSplit._modules.template = origTemplate;
			return JSON.stringify({ text: r.text, error: r.error || '' });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Text  string `json:"text"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error == "" {
		t.Error("expected error when template module unavailable")
	}
	if data.Text != "" {
		t.Error("expected empty text on error")
	}
}

func TestChunk09_ClaudeCodeExecutor_Close(t *testing.T) {
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var ex = new globalThis.prSplit.ClaudeCodeExecutor({});
			ex.handle = { close: function() {} };
			ex.sessionId = 'test';
			ex.resolved = { command: 'test', type: 'test' };
			ex.close();
			return JSON.stringify({
				handleNull: ex.handle === null,
				sessionIdNull: ex.sessionId === null,
				resolvedNull: ex.resolved === null
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		HandleNull    bool `json:"handleNull"`
		SessionIdNull bool `json:"sessionIdNull"`
		ResolvedNull  bool `json:"resolvedNull"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.HandleNull {
		t.Error("expected handle null after close")
	}
	if !data.SessionIdNull {
		t.Error("expected sessionId null after close")
	}
	if !data.ResolvedNull {
		t.Error("expected resolved null after close")
	}
}

// ---------------------------------------------------------------------------
//  T025: Crash recovery — restart() and captureDiagnostic()
// ---------------------------------------------------------------------------

func TestChunk09_ClaudeCodeExecutor_CaptureDiagnostic(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var ex = new globalThis.prSplit.ClaudeCodeExecutor({});
			// No handle — should return empty string.
			var d1 = ex.captureDiagnostic();

			// Handle with receive returning data.
			ex.handle = {
				receive: function() { return 'error: segfault at 0x0\npanic: runtime error'; }
			};
			var d2 = ex.captureDiagnostic();

			// Handle with receive that throws (simulating dead process EOF).
			ex.handle = {
				receive: function() { throw new Error('EOF'); }
			};
			var d3 = ex.captureDiagnostic();

			// Handle without receive method.
			ex.handle = {};
			var d4 = ex.captureDiagnostic();

			return JSON.stringify({
				noHandle: d1,
				withData: d2,
				throwsEOF: d3,
				noReceive: d4
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		NoHandle  string `json:"noHandle"`
		WithData  string `json:"withData"`
		ThrowsEOF string `json:"throwsEOF"`
		NoReceive string `json:"noReceive"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.NoHandle != "" {
		t.Errorf("captureDiagnostic with no handle should be empty, got %q", data.NoHandle)
	}
	if data.WithData != "error: segfault at 0x0\npanic: runtime error" {
		t.Errorf("captureDiagnostic with data got %q", data.WithData)
	}
	if data.ThrowsEOF != "" {
		t.Errorf("captureDiagnostic with EOF throw should be empty, got %q", data.ThrowsEOF)
	}
	if data.NoReceive != "" {
		t.Errorf("captureDiagnostic with no receive should be empty, got %q", data.NoReceive)
	}
}

func TestChunk09_ClaudeCodeExecutor_Restart(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(async function() {
			var closeCalled = false;
			var resolveCalled = false;
			var spawnCalled = false;
			var spawnOpts = null;

			var ex = new globalThis.prSplit.ClaudeCodeExecutor({
				claudeCommand: '/usr/bin/test-claude'
			});
			ex.handle = { close: function() { closeCalled = true; } };
			ex.sessionId = 'old-session';
			ex.resolved = { command: '/usr/bin/test-claude', type: 'explicit' };

			// Mock resolveAsync and spawn.
			var origResolve = ex.resolve;
			ex.resolveAsync = async function() {
				resolveCalled = true;
				ex.resolved = { command: '/usr/bin/test-claude', type: 'explicit' };
				return { error: null };
			};
			var origSpawn = ex.spawn;
			ex.spawn = function(sid, opts) {
				spawnCalled = true;
				spawnOpts = opts;
				ex.handle = { isAlive: function() { return true; } };
				ex.sessionId = 'new-session';
				return { error: null, sessionId: 'new-session' };
			};

			var result = await ex.restart(null, { mcpConfigPath: '/tmp/mcp.json' });
			return JSON.stringify({
				closeCalled: closeCalled,
				resolveCalled: resolveCalled,
				spawnCalled: spawnCalled,
				error: result.error,
				sessionId: result.sessionId,
				mcpConfigPath: spawnOpts ? spawnOpts.mcpConfigPath : ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		CloseCalled   bool    `json:"closeCalled"`
		ResolveCalled bool    `json:"resolveCalled"`
		SpawnCalled   bool    `json:"spawnCalled"`
		Error         *string `json:"error"`
		SessionId     string  `json:"sessionId"`
		McpConfigPath string  `json:"mcpConfigPath"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if !data.CloseCalled {
		t.Error("restart should call close()")
	}
	if !data.ResolveCalled {
		t.Error("restart should call resolve()")
	}
	if !data.SpawnCalled {
		t.Error("restart should call spawn()")
	}
	if data.Error != nil {
		t.Errorf("restart error should be null, got %v", *data.Error)
	}
	if data.SessionId != "new-session" {
		t.Errorf("restart sessionId should be 'new-session', got %q", data.SessionId)
	}
	if data.McpConfigPath != "/tmp/mcp.json" {
		t.Errorf("restart should pass mcpConfigPath, got %q", data.McpConfigPath)
	}
}

func TestChunk09_ClaudeCodeExecutor_Restart_ResolveFails(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(async function() {
			var ex = new globalThis.prSplit.ClaudeCodeExecutor({});
			ex.handle = { close: function() {} };

			// Mock resolveAsync to fail.
			ex.resolveAsync = async function() {
				return { error: 'binary not found' };
			};

			var r = await ex.restart();
			return JSON.stringify({ error: r.error });
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}

	var data struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &data); err != nil {
		t.Fatal(err)
	}
	if data.Error == "" {
		t.Error("restart with failed resolve should return error")
	}
	if !strings.Contains(data.Error, "resolve failed") {
		t.Errorf("error should mention resolve failed, got %q", data.Error)
	}
}
