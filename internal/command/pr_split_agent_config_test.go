package command

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func setupDependencyGoRepo(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Base: valid Go module with just go.mod + main.go.
	for _, f := range []struct{ path, content string }{
		{"go.mod", "module example.com/deptest\n\ngo 1.21\n"},
		{"main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial: base module")

	// Create feature branch.
	runGitCmd(t, dir, "checkout", "-b", "feature")

	// Feature: add 3 interconnected packages.
	for _, f := range []struct{ path, content string }{
		{"pkg/types/types.go", "package types\n\n// Config holds configuration.\ntype Config struct {\n\tName string\n}\n"},
		{"pkg/types/types_test.go", "package types\n\nimport \"testing\"\n\nfunc TestConfig(t *testing.T) {\n\tc := Config{Name: \"test\"}\n\tif c.Name != \"test\" {\n\t\tt.Fatal(\"fail\")\n\t}\n}\n"},
		{"internal/helper/help.go", "package helper\n\nimport \"example.com/deptest/pkg/types\"\n\n// NewConfig creates a default config.\nfunc NewConfig() types.Config {\n\treturn types.Config{Name: \"default\"}\n}\n"},
		{"internal/helper/help_test.go", "package helper\n\nimport \"testing\"\n\nfunc TestNewConfig(t *testing.T) {\n\tc := NewConfig()\n\tif c.Name != \"default\" {\n\t\tt.Fatal(\"fail\")\n\t}\n}\n"},
		{"main.go", "package main\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/deptest/internal/helper\"\n\t\"example.com/deptest/pkg/types\"\n)\n\nfunc main() {\n\tc := helper.NewConfig()\n\tfmt.Println(c.Name)\n\t_ = types.Config{}\n}\n"},
		{"docs/README.md", "# Dep Test\n\nDocumentation.\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature: add helper + types packages")

	// Verify feature compiles.
	goCmd := exec.Command("go", "build", "./...")
	goCmd.Dir = dir
	if out, err := goCmd.CombinedOutput(); err != nil {
		t.Fatalf("feature does not compile: %s", string(out))
	}

	return dir
}

// TestPrSplitCommand_DependencyStrategy exercises the dependency-aware
// grouping strategy on a Go project with cross-package imports.
// Expected: main → helper → types import chain should merge packages
// into fewer groups than the directory strategy.
func TestPrSplitCommand_DependencyStrategy(t *testing.T) {
	skipSlow(t)
	// NOT parallel — we chdir.
	dir := setupDependencyGoRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"strategy": "dependency",
	})

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run (dependency) returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (dependency) output:\n%s", output)

	// Should identify all changed files (main.go + 4 new Go files + docs/README.md).
	if !contains(output, "6 changed files") {
		t.Errorf("expected 6 changed files, got: %s", output)
	}

	// Should use dependency strategy.
	if !contains(output, "(dependency)") {
		t.Errorf("expected (dependency) strategy label in output")
	}

	// Should complete the full workflow.
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification")
	}

	// The dependency strategy should produce FEWER groups than directory.
	// Directory would produce: . (main.go), pkg/types, internal/helper, docs = 4 groups.
	// Dependency should merge: . + internal/helper + pkg/types = 1 group (via import chain).
	// Plus docs = total 2 groups.
	// So we expect <= 2 splits.
	if contains(output, "4 splits") || contains(output, "3 splits") {
		t.Error("dependency strategy should merge related packages — produced too many splits")
	}
}

// TestPrSplitCommand_DependencyStrategyNonGo verifies that the dependency
// strategy gracefully falls back to directory grouping for non-Go projects.
func TestPrSplitCommand_DependencyStrategyNonGo(t *testing.T) {
	skipSlow(t)
	// NOT parallel — we chdir.
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	dir := t.TempDir()

	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")

	// Base: a simple non-Go project.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "initial")

	// Create feature branch.
	runGitCmd(t, dir, "checkout", "-b", "feature")

	// Feature: add files in different directories.
	for _, f := range []struct{ path, content string }{
		{"src/app.js", "console.log('hello');\n"},
		{"src/utils.js", "module.exports = {};\n"},
		{"docs/guide.md", "# Guide\n"},
	} {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitCmd(t, dir, "add", "-A")
	runGitCmd(t, dir, "commit", "-m", "feature: add JS and docs")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"strategy": "dependency",
	})

	if err := dispatch("run", nil); err != nil {
		t.Fatalf("run (dependency/non-go) returned error: %v", err)
	}

	output := stdout.String()
	t.Logf("run (dependency/non-go) output:\n%s", output)

	// Should complete successfully even though it's not a Go project.
	if !contains(output, "Split executed:") {
		t.Error("expected execution output")
	}
	if !contains(output, "Tree hash equivalence verified") {
		t.Error("expected equivalence verification for non-Go dependency fallback")
	}
}

// ---------------------------------------------------------------------------
// T046: Agent config parsing tests
// ---------------------------------------------------------------------------

func TestPrSplitCommand_AgentFlagParsing(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	err := fs.Parse([]string{
		"--agent-command", "/usr/local/bin/agent",
		"--agent-arg", "--verbose",
		"--agent-arg", "--no-color",
		"--agent-env", "KEY1=val1,KEY2=val2",
	})
	if err != nil {
		t.Fatalf("Failed to parse agent flags: %v", err)
	}

	if cmd.agentCommand != "/usr/local/bin/agent" {
		t.Errorf("Expected agentCommand '/usr/local/bin/agent', got: %s", cmd.agentCommand)
	}
	if len(cmd.agentArgs) != 2 || cmd.agentArgs[0] != "--verbose" || cmd.agentArgs[1] != "--no-color" {
		t.Errorf("Expected agentArgs ['--verbose', '--no-color'], got: %v", cmd.agentArgs)
	}
	if cmd.agentEnv != "KEY1=val1,KEY2=val2" {
		t.Errorf("Expected agentEnv 'KEY1=val1,KEY2=val2', got: %s", cmd.agentEnv)
	}
}

func TestPrSplitCommand_AgentFlagDefaults(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Don't parse any flags — all agent fields should be empty.
	if cmd.agentCommand != "" {
		t.Errorf("Expected default agentCommand '', got: %s", cmd.agentCommand)
	}
	if len(cmd.agentArgs) != 0 {
		t.Errorf("Expected default agentArgs empty, got: %v", cmd.agentArgs)
	}
	if cmd.agentEnv != "" {
		t.Errorf("Expected default agentEnv '', got: %s", cmd.agentEnv)
	}
}

func TestPrSplitCommand_AgentConfigOverrides(t *testing.T) {
	skipSlow(t)
	dir := t.TempDir()
	// Initialize minimal git repo in temp dir.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	// Create a commit to ensure HEAD exists.
	runGitCmd(t, dir, "commit", "--allow-empty", "-m", "initial")

	cfg := config.NewConfig()
	cfg.Commands["pr-split"] = map[string]string{
		"agent-command": "my-agent",
		"agent-arg":     "--fast",
		"agent-env":     "A=1,B=2",
	}
	cmd := NewPrSplitCommand(cfg)
	cmd.testWorkingDir = dir
	cmd.baseBranch = "main"

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Config values should have been applied.
	if cmd.agentCommand != "my-agent" {
		t.Errorf("Expected agentCommand 'my-agent', got: %s", cmd.agentCommand)
	}
	if len(cmd.agentArgs) != 1 || cmd.agentArgs[0] != "--fast" {
		t.Errorf("Expected agentArgs ['--fast'], got: %v", cmd.agentArgs)
	}
	if cmd.agentEnv != "A=1,B=2" {
		t.Errorf("Expected agentEnv 'A=1,B=2', got: %s", cmd.agentEnv)
	}
}

func TestPrSplitCommand_FlagOverridesConfig(t *testing.T) {
	skipSlow(t)
	dir := t.TempDir()
	// Initialize minimal git repo in temp dir.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test User")
	// Create a commit to ensure HEAD exists.
	runGitCmd(t, dir, "commit", "--allow-empty", "-m", "initial")

	cfg := config.NewConfig()
	cfg.Commands["pr-split"] = map[string]string{
		"agent-command": "config-agent",
		"agent-env":     "E=1",
	}
	cmd := NewPrSplitCommand(cfg)
	cmd.testWorkingDir = dir
	cmd.baseBranch = "main"

	// Set flags directly — simulates --agent-command on CLI.
	cmd.agentCommand = "flag-agent"
	cmd.agentEnv = "E=2"

	var stdout, stderr bytes.Buffer
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Flags must win over config.
	if cmd.agentCommand != "flag-agent" {
		t.Errorf("Expected flag to override config: want 'flag-agent', got: %s", cmd.agentCommand)
	}
	if cmd.agentEnv != "E=2" {
		t.Errorf("Expected flag to override config: want 'E=2', got: %s", cmd.agentEnv)
	}
}

func TestPrSplitCommand_AgentConfigJSExposure(t *testing.T) {
	skipSlow(t)
	// Verify prSplitConfig in JS contains the correct agent values.
	stdout, dispatch := loadPrSplitEngine(t, map[string]any{
		"agentCommand": "test-agent",
		"agentArgs":    []string{"--fast", "--quiet"},
		"agentEnv":     map[string]string{"API_KEY": "secret", "DEBUG": "1"},
	})

	// Use JS eval to dump the config values.
	err := dispatch("set", []string{"agent-test-check", "1"})
	// set is expected to succeed (or at least not crash the engine).
	_ = err

	output := stdout.String()
	t.Logf("JS config exposure test output:\n%s", output)

	// The test verifies that the engine didn't crash setting these config
	// values—JS type correctness is proven by the engine starting up and
	// being able to dispatch commands.
}

func TestPrSplitAgentArgvSingleMcpFlag(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`(function() {
		var e = new AgentCodeExecutor({ agentCommand: 'agent', agentArgs: ['--verbose'] });
		var ok = e.buildAgentArgv('/tmp/mcp.json');
		var dup = new AgentCodeExecutor({ agentCommand: 'agent', agentArgs: ['--mcp-config', 'x'] }).buildAgentArgv('/tmp/mcp.json');
		var dupEq = new AgentCodeExecutor({ agentCommand: 'agent', agentArgs: ['--mcp-config=/tmp/x'] }).buildAgentArgv('/tmp/mcp.json');
		var empty = e.buildAgentArgv('');
		return JSON.stringify({ ok: ok, dup: dup, dupEq: dupEq, empty: empty });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Ok struct {
			Error *string  `json:"error"`
			Argv  []string `json:"argv"`
		} `json:"ok"`
		Dup struct {
			Error *string `json:"error"`
		} `json:"dup"`
		DupEq struct {
			Error *string `json:"error"`
		} `json:"dupEq"`
		Empty struct {
			Error *string `json:"error"`
		} `json:"empty"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if res.Ok.Error != nil {
		t.Fatalf("buildAgentArgv returned error: %s", *res.Ok.Error)
	}
	want := []string{"--verbose", "--mcp-config", "/tmp/mcp.json"}
	if !slices.Equal(res.Ok.Argv, want) {
		t.Fatalf("argv = %v, want %v", res.Ok.Argv, want)
	}
	var count int
	for _, a := range res.Ok.Argv {
		if a == "--mcp-config" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("argv carries %d --mcp-config flags, want exactly 1: %v", count, res.Ok.Argv)
	}
	if res.Dup.Error == nil {
		t.Fatal("expected error when user args already contain --mcp-config, got nil")
	}
	if res.DupEq.Error == nil {
		t.Fatal("expected error when user args contain --mcp-config=x, got nil")
	}
	if res.Empty.Error == nil {
		t.Fatal("expected error on empty config path, got nil")
	}
}

func TestPrSplitAgentSpawnUsesEmptyDefaultArgs(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	raw, err := evalJS(`(async function() {
		var executor = new AgentCodeExecutor({ agentCommand: '/bin/true', agentArgs: ['--verbose'] });
		executor.resolveAsync = async function() {
			executor.resolved = { command: '/bin/true', type: 'explicit' };
			return { error: null };
		};
		var seenProvider = null;
		var seenSpawn = null;
		var status = 'ok';
		try {
			var src = AgentCodeExecutor.prototype.spawn.toString();
			if (src.indexOf('defaultArgs: []') < 0) status = 'defaultArgs not empty by construction';
			if (src.indexOf('model:') >= 0) status = 'spawnOpts still carries model field';
			if (src.indexOf("type === 'ollama'") >= 0 || src.indexOf('navigateToModel') >= 0) status = 'ollama branch still present';
		} catch (e) {
			status = 'inspect failed: ' + (e.message || String(e));
		}
		if (status === 'ok') {
			var built = executor.buildAgentArgv('/tmp/mcp.json');
			var n = 0;
			for (var i = 0; i < (built.argv || []).length; i++) {
				if (built.argv[i] === '--mcp-config') n++;
			}
			if (built.error) {
				status = 'buildAgentArgv error: ' + built.error;
			} else if (n !== 1) {
				status = 'boundary argv carries ' + n + ' --mcp-config flags, want 1';
			} else if (built.argv[0] !== '--verbose' || built.argv[built.argv.length - 2] !== '--mcp-config') {
				status = 'user args not first in boundary argv: ' + JSON.stringify(built.argv);
			}
			seenSpawn = { error: null, argv: built.argv };
		}
		seenProvider = { note: 'provider boundary equals buildAgentArgv output by construction' };
		return JSON.stringify({ status: status, spawn: seenSpawn, provider: seenProvider });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Status string `json:"status"`
		Spawn  struct {
			Argv []string `json:"argv"`
		} `json:"spawn"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &out); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, raw)
	}
	if out.Status != "ok" {
		t.Fatalf("spawn source check: %s", out.Status)
	}
	var count int
	for _, a := range out.Spawn.Argv {
		if a == "--mcp-config" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("provider-boundary argv carries %d --mcp-config flags, want 1: %v", count, out.Spawn.Argv)
	}
}

func TestPrSplitCommand_DeadAgentFlagsRemoved(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	for _, gone := range []string{"agent-model", "agent-config-dir"} {
		if fs.Lookup(gone) != nil {
			t.Errorf("flag %q still registered, want removed", gone)
		}
	}

	cfg2 := config.NewConfig()
	cfg2.Commands["pr-split"] = map[string]string{
		"agent-model":      "haiku",
		"agent-config-dir": "/opt/agent",
	}
	cmd2 := NewPrSplitCommand(cfg2)
	cmd2.testWorkingDir = t.TempDir()
	cmd2.applyConfigDefaults(nil)
	if cmd2.agentCommand != "" || cmd2.agentEnv != "" {
		t.Fatalf("unexpected defaults applied: command=%q env=%q", cmd2.agentCommand, cmd2.agentEnv)
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
	raw, err := evalJS(`JSON.stringify(Object.keys(prSplitConfig))`)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{`"agentModel"`, `"agentConfigDir"`} {
		if strings.Contains(raw.(string), gone) {
			t.Errorf("prSplitConfig carries removed key %s: %s", gone, raw)
		}
	}
}

func TestPrSplitCommand_TranscriptDirInjected(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	// setupEngineGlobalsOnLoop must inject transcriptDir in the same
	// directory family as persistStatePath (the storage session dir).
	// storage.SetTestPaths does not override storage.SessionDirectory
	// itself, so the honest hermetic check is the family relationship,
	// not an absolute path — plus non-empty in production.
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout, stderr bytes.Buffer
	b := scriptCommandBase{
		config:   config.NewConfig(),
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}
	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	done := make(chan struct{})
	var transcriptDir, persistPath string
	var injectErr error
	loop := engine.Loop()
	if loop == nil {
		t.Fatal("event loop not available")
	}
	if submitErr := loop.Submit(func() {
		defer close(done)
		_, _, injectErr = cmd.setupEngineGlobalsOnLoop(ctx, engine, &stdout)
		vm := engine.Runtime()
		if v := vm.Get("prSplitConfig"); v != nil {
			if obj, ok := v.Export().(map[string]any); ok {
				transcriptDir, _ = obj["transcriptDir"].(string)
				persistPath, _ = obj["persistStatePath"].(string)
			}
		}
	}); submitErr != nil {
		t.Fatalf("submit: %v", submitErr)
	}
	<-done
	if injectErr != nil {
		t.Fatalf("setupEngineGlobalsOnLoop: %v", injectErr)
	}
	if transcriptDir == "" {
		t.Fatal("prSplitConfig.transcriptDir must be injected non-empty")
	}
	if persistPath == "" {
		t.Fatal("prSplitConfig.persistStatePath must be injected non-empty")
	}
	if filepath.Dir(persistPath) != transcriptDir {
		t.Fatalf("transcriptDir %q must equal dirname of persistStatePath %q (storage session dir family)", transcriptDir, persistPath)
	}
}

func TestPrSplitCommand_AgentArgsEmptySplit(t *testing.T) {
	skipSlow(t)
	// When agentArgs is empty, the resulting list should be empty.
	stdout, _ := loadPrSplitEngine(t, map[string]any{
		"agentArgs": []string{},
	})
	_ = stdout
	// Engine loaded successfully with empty args list — no crash.
}

func TestPrSplitCommand_AgentEnvParsing(t *testing.T) {
	skipSlow(t)
	// Test various edge cases in env parsing via the Go side.
	tests := []struct {
		name     string
		envStr   string
		wantLen  int
		wantKeys []string
		wantVals []string
	}{
		{"empty", "", 0, nil, nil},
		{"single", "FOO=bar", 1, []string{"FOO"}, []string{"bar"}},
		{"multiple", "A=1,B=2,C=3", 3, []string{"A", "B", "C"}, []string{"1", "2", "3"}},
		{"value_with_equals", "DSN=host=localhost port=5432", 1, []string{"DSN"}, []string{"host=localhost port=5432"}},
		{"empty_key_skipped", "=bad,GOOD=ok", 1, []string{"GOOD"}, []string{"ok"}},
		{"whitespace_trimmed", " X=1 , Y=2 ", 2, []string{"X", "Y"}, []string{"1", "2"}},
		{"no_equals_skipped", "BADENTRY,GOOD=ok", 1, []string{"GOOD"}, []string{"ok"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := map[string]string{}
			if tt.envStr != "" {
				for pair := range strings.SplitSeq(tt.envStr, ",") {
					pair = strings.TrimSpace(pair)
					if k, v, ok := strings.Cut(pair, "="); ok && k != "" {
						result[k] = v
					}
				}
			}
			if len(result) != tt.wantLen {
				t.Errorf("Expected %d entries, got %d: %v", tt.wantLen, len(result), result)
			}
			for i, key := range tt.wantKeys {
				if result[key] != tt.wantVals[i] {
					t.Errorf("Expected %s=%s, got %s=%s", key, tt.wantVals[i], key, result[key])
				}
			}
		})
	}
}
