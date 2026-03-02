package command

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// T56: AUTO_FIX_STRATEGIES detect + fix unit tests
// ---------------------------------------------------------------------------

// Helper JS to get strategies by name from the exported array.
const strategiesAccessJS = `
// Helper: find strategy by name.
function getStrategy(name) {
	var arr = globalThis.prSplit.AUTO_FIX_STRATEGIES;
	for (var i = 0; i < arr.length; i++) {
		if (arr[i].name === name) return arr[i];
	}
	return null;
}
`

func TestAutoFixStrategy_GoModTidy_Detect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(strategiesAccessJS); err != nil {
		t.Fatal(err)
	}
	// Mock osmod.fileExists.
	if _, err := evalJS(`
		var _osmod56 = require('osm:os');
		_osmod56.fileExists = function(p) { return !!(globalThis._mockFileExists && globalThis._mockFileExists[p]); };
	`); err != nil {
		t.Fatal(err)
	}

	t.Run("detect_true_when_gomod_exists", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {'go.mod': true};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('go-mod-tidy').detect('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true, got %v", val)
		}
	})

	t.Run("detect_false_when_no_gomod", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('go-mod-tidy').detect('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false, got %v", val)
		}
	})

	t.Run("detect_with_custom_dir", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {'/repo/go.mod': true};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('go-mod-tidy').detect('/repo')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for /repo/go.mod, got %v", val)
		}
	})
}

func TestAutoFixStrategy_GoGenerateSum_Detect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(strategiesAccessJS); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		var _osmod56b = require('osm:os');
		_osmod56b.fileExists = function(p) { return !!(globalThis._mockFileExists && globalThis._mockFileExists[p]); };
	`); err != nil {
		t.Fatal(err)
	}

	t.Run("detect_true_when_gosum_exists", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {'go.sum': true};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('go-generate-sum').detect('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true, got %v", val)
		}
	})

	t.Run("detect_false_when_no_gosum", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('go-generate-sum').detect('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false, got %v", val)
		}
	})
}

func TestAutoFixStrategy_GoBuildMissingImports_Detect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(strategiesAccessJS); err != nil {
		t.Fatal(err)
	}

	t.Run("detect_true_with_undefined_reference", func(t *testing.T) {
		val, err := evalJS(`getStrategy('go-build-missing-imports').detect('.', 'src/foo.go:10: undefined: SomeFunc')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for undefined reference, got %v", val)
		}
	})

	t.Run("detect_true_with_unused_import", func(t *testing.T) {
		val, err := evalJS(`getStrategy('go-build-missing-imports').detect('.', '"fmt" imported and not used')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for unused import, got %v", val)
		}
	})

	t.Run("detect_true_with_could_not_import", func(t *testing.T) {
		val, err := evalJS(`getStrategy('go-build-missing-imports').detect('.', 'could not import github.com/foo/bar')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for could not import, got %v", val)
		}
	})

	t.Run("detect_false_with_unrelated_error", func(t *testing.T) {
		val, err := evalJS(`getStrategy('go-build-missing-imports').detect('.', 'syntax error: unexpected EOF')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false for unrelated error, got %v", val)
		}
	})

	t.Run("detect_false_with_null_output", func(t *testing.T) {
		val, err := evalJS(`getStrategy('go-build-missing-imports').detect('.', null)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false for null output, got %v", val)
		}
	})
}

func TestAutoFixStrategy_NpmInstall_Detect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(strategiesAccessJS); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(`
		var _osmod56c = require('osm:os');
		_osmod56c.fileExists = function(p) { return !!(globalThis._mockFileExists && globalThis._mockFileExists[p]); };
	`); err != nil {
		t.Fatal(err)
	}

	t.Run("detect_true_when_packagejson_exists", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {'package.json': true};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('npm-install').detect('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true, got %v", val)
		}
	})

	t.Run("detect_false_when_no_packagejson", func(t *testing.T) {
		if _, err := evalJS(`globalThis._mockFileExists = {};`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`getStrategy('npm-install').detect('.')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false, got %v", val)
		}
	})
}

func TestAutoFixStrategy_AddMissingFiles_Detect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(strategiesAccessJS); err != nil {
		t.Fatal(err)
	}

	t.Run("detect_true_no_such_file", func(t *testing.T) {
		val, err := evalJS(`getStrategy('add-missing-files').detect('.', 'open foo.go: no such file or directory')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for 'no such file', got %v", val)
		}
	})

	t.Run("detect_true_cannot_find", func(t *testing.T) {
		val, err := evalJS(`getStrategy('add-missing-files').detect('.', 'error: cannot find module bar')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for 'cannot find', got %v", val)
		}
	})

	t.Run("detect_true_file_not_found", func(t *testing.T) {
		val, err := evalJS(`getStrategy('add-missing-files').detect('.', 'compilation: file not found')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != true {
			t.Errorf("expected true for 'file not found', got %v", val)
		}
	})

	t.Run("detect_false_for_unrelated", func(t *testing.T) {
		val, err := evalJS(`getStrategy('add-missing-files').detect('.', 'test failure: assertion error')`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false for unrelated error, got %v", val)
		}
	})

	t.Run("detect_false_for_null", func(t *testing.T) {
		val, err := evalJS(`getStrategy('add-missing-files').detect('.', null)`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false for null, got %v", val)
		}
	})
}

func TestAutoFixStrategy_ClaudeFix_Detect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(strategiesAccessJS); err != nil {
		t.Fatal(err)
	}

	// Without claudeExecutor, should be false.
	t.Run("detect_false_without_executor", func(t *testing.T) {
		val, err := evalJS(`getStrategy('claude-fix').detect()`)
		if err != nil {
			t.Fatal(err)
		}
		if val != false {
			t.Errorf("expected false without executor, got %v", val)
		}
	})
}

// ---------------------------------------------------------------------------
// T57: ClaudeCodeExecutor.resolve auto-detection tests
// ---------------------------------------------------------------------------

func TestClaudeCodeExecutor_Resolve_ExplicitCommand(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	t.Run("explicit_command_found", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57 = require('osm:exec');
			execMod57.execv = function(argv) {
				if (argv[0] === 'which' && argv[1] === 'my-claude') {
					return {stdout: '/usr/bin/my-claude', stderr: '', code: 0};
				}
				return {stdout: '', stderr: 'not found', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({claudeCommand: 'my-claude'});
			var result = exe.resolve();
			JSON.stringify({error: result.error, resolved: exe.resolved})
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if strings.Contains(s, `"error":"`) {
			t.Errorf("expected no error, got: %s", s)
		}
		if !strings.Contains(s, `"type":"explicit"`) {
			t.Errorf("expected type explicit, got: %s", s)
		}
	})

	t.Run("explicit_command_not_found", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57b = require('osm:exec');
			execMod57b.execv = function(argv) {
				return {stdout: '', stderr: 'not found', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({claudeCommand: 'nonexistent'});
			var result = exe.resolve();
			JSON.stringify(result)
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if !strings.Contains(s, "not found") {
			t.Errorf("expected 'not found' error, got: %s", s)
		}
	})
}

func TestClaudeCodeExecutor_Resolve_AutoDetect(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	t.Run("claude_autodetected_version_ok", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57c = require('osm:exec');
			execMod57c.execv = function(argv) {
				if (argv[0] === 'which' && argv[1] === 'claude') {
					return {stdout: '/usr/local/bin/claude', stderr: '', code: 0};
				}
				if (argv[0] === 'claude' && argv[1] === '--version') {
					return {stdout: 'Claude Code v2.1.0', stderr: '', code: 0};
				}
				return {stdout: '', stderr: 'not found', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({});
			var result = exe.resolve();
			JSON.stringify({error: result.error, resolved: exe.resolved})
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if !strings.Contains(s, `"type":"claude-code"`) {
			t.Errorf("expected claude-code type, got: %s", s)
		}
	})

	t.Run("claude_found_version_fails", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57d = require('osm:exec');
			execMod57d.execv = function(argv) {
				if (argv[0] === 'which' && argv[1] === 'claude') {
					return {stdout: '/usr/local/bin/claude', stderr: '', code: 0};
				}
				if (argv[0] === 'claude' && argv[1] === '--version') {
					return {stdout: '', stderr: 'segfault', code: 139};
				}
				return {stdout: '', stderr: 'not found', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({});
			var result = exe.resolve();
			JSON.stringify(result)
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if !strings.Contains(s, "version check failed") {
			t.Errorf("expected version check failure, got: %s", s)
		}
	})

	t.Run("ollama_autodetected_model_available", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57e = require('osm:exec');
			execMod57e.execv = function(argv) {
				if (argv[0] === 'which' && argv[1] === 'claude') {
					return {stdout: '', stderr: '', code: 1};
				}
				if (argv[0] === 'which' && argv[1] === 'ollama') {
					return {stdout: '/usr/local/bin/ollama', stderr: '', code: 0};
				}
				if (argv[0] === 'ollama' && argv[1] === 'list') {
					return {stdout: 'NAME\nllama3:latest\nmistral:latest\n', stderr: '', code: 0};
				}
				return {stdout: '', stderr: '', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({claudeModel: 'llama3'});
			var result = exe.resolve();
			JSON.stringify({error: result.error, resolved: exe.resolved})
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if !strings.Contains(s, `"type":"ollama"`) {
			t.Errorf("expected ollama type, got: %s", s)
		}
	})

	t.Run("ollama_model_missing", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57f = require('osm:exec');
			execMod57f.execv = function(argv) {
				if (argv[0] === 'which' && argv[1] === 'claude') {
					return {stdout: '', stderr: '', code: 1};
				}
				if (argv[0] === 'which' && argv[1] === 'ollama') {
					return {stdout: '/usr/local/bin/ollama', stderr: '', code: 0};
				}
				if (argv[0] === 'ollama' && argv[1] === 'list') {
					return {stdout: 'NAME\nmistral:latest\n', stderr: '', code: 0};
				}
				return {stdout: '', stderr: '', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({claudeModel: 'llama3'});
			var result = exe.resolve();
			JSON.stringify(result)
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if !strings.Contains(s, "not available") {
			t.Errorf("expected model not available error, got: %s", s)
		}
	})

	t.Run("nothing_found", func(t *testing.T) {
		if _, err := evalJS(`
			var execMod57g = require('osm:exec');
			execMod57g.execv = function(argv) {
				return {stdout: '', stderr: '', code: 1};
			};
		`); err != nil {
			t.Fatal(err)
		}
		val, err := evalJS(`
			var exe = new globalThis.prSplit.ClaudeCodeExecutor({});
			var result = exe.resolve();
			JSON.stringify(result)
		`)
		if err != nil {
			t.Fatal(err)
		}
		s := val.(string)
		if !strings.Contains(s, "No Claude-compatible binary") {
			t.Errorf("expected no binary found error, got: %s", s)
		}
	})
}

// ---------------------------------------------------------------------------
// T58: validateSplitPlan duplicate file detection test
// ---------------------------------------------------------------------------

func TestValidateSplitPlan_DuplicateFiles(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`
		var stages = [
			{name: 'split-a', files: ['foo.go', 'bar.go']},
			{name: 'split-b', files: ['baz.go', 'foo.go']}
		];
		var result = globalThis.prSplit.validateSplitPlan(stages);
		JSON.stringify(result)
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, "duplicate") || !strings.Contains(s, "foo.go") {
		t.Errorf("expected duplicate file error mentioning foo.go, got: %s", s)
	}
}

func TestValidateSplitPlan_NoDuplicates(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(execMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`
		var stages = [
			{name: 'split-a', files: ['foo.go', 'bar.go']},
			{name: 'split-b', files: ['baz.go', 'qux.go']}
		];
		var result = globalThis.prSplit.validateSplitPlan(stages);
		JSON.stringify(result)
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if strings.Contains(s, "duplicate") {
		t.Errorf("expected no duplicate errors, got: %s", s)
	}
}
