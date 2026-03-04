package command

import (
	"encoding/json"
	"testing"
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
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

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
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

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
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

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
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

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
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

	result, err := evalJS(`
		(function() {
			var r = globalThis.prSplit.renderConflictPrompt({
				branchName: 'split/01-types',
				files: ['types.go', 'types_test.go'],
				exitCode: 2,
				errorOutput: 'undefined: Foo',
				sessionId: 'test-session-123'
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
	if !data.ContainsSession {
		t.Error("prompt should contain session ID")
	}
}

func TestChunk09_RenderPrompt_NoTemplate(t *testing.T) {
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

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
	evalJS := loadChunkEngine(t, nil, claudeChunks...)

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
