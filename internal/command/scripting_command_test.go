package command

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

type terminalFunc func()

func (f terminalFunc) Run() {
	f()
}

func TestNewScriptingCommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewScriptingCommand(cfg)

	if got := cmd.Name(); got != "script" {
		t.Fatalf("expected command name 'script', got %q", got)
	}
	if got := cmd.Description(); got != "Execute JavaScript scripts with deferred/declarative API" {
		t.Fatalf("unexpected description: %q", got)
	}
	if got := cmd.Usage(); got != "script [options] [script-file]" {
		t.Fatalf("unexpected usage: %q", got)
	}
	if cmd.config != cfg {
		t.Fatalf("expected config to be retained")
	}
	// engineFactory is intentionally nil - Execute() creates it with correct session/storage params
	if cmd.terminalFactory == nil {
		t.Fatalf("expected terminalFactory to be initialized")
	}
}

func TestScriptingCommand_SetupFlags(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	fs := flag.NewFlagSet("scripting", flag.ContinueOnError)

	cmd.SetupFlags(fs)

	for _, name := range []string{"interactive", "i", "script", "e", "test"} {
		if fs.Lookup(name) == nil {
			t.Fatalf("expected flag %q to be registered", name)
		}
	}
}

func TestScriptingCommand_FlagParsing(t *testing.T) {
	t.Run("long forms", func(t *testing.T) {
		t.Parallel()
		cmd := NewScriptingCommand(config.NewConfig())
		fs := flag.NewFlagSet("scripting", flag.ContinueOnError)
		cmd.SetupFlags(fs)

		if err := fs.Parse([]string{"--interactive", "--script", "ctx.log('hi')", "--test"}); err != nil {
			t.Fatalf("parse failed: %v", err)
		}

		if !cmd.interactive {
			t.Fatalf("expected interactive to be true")
		}
		if cmd.script != "ctx.log('hi')" {
			t.Fatalf("unexpected script: %q", cmd.script)
		}
		if !cmd.testMode {
			t.Fatalf("expected testMode to be true")
		}
	})

	t.Run("short forms", func(t *testing.T) {
		t.Parallel()
		cmd := NewScriptingCommand(config.NewConfig())
		fs := flag.NewFlagSet("scripting", flag.ContinueOnError)
		cmd.SetupFlags(fs)

		if err := fs.Parse([]string{"-i", "-e", "ctx.log('short')"}); err != nil {
			t.Fatalf("parse failed: %v", err)
		}

		if !cmd.interactive {
			t.Fatalf("expected interactive to be true")
		}
		if cmd.script != "ctx.log('short')" {
			t.Fatalf("unexpected script: %q", cmd.script)
		}
	})
}

func TestScriptingCommand_Execute_NoScript(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	var stdout, stderr bytes.Buffer

	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil || err.Error() != "no script specified" {
		t.Fatalf("expected no script specified error, got %v", err)
	}

	msg := stderr.String()
	if !strings.Contains(msg, "No script file specified") {
		t.Fatalf("expected guidance in stderr, got %q", msg)
	}
}

func TestScriptingCommand_Execute_ScriptFileNotFound(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	var stdout, stderr bytes.Buffer

	err := cmd.Execute([]string{"missing-script.js"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "script file not found") {
		t.Fatalf("expected script missing error, got %v", err)
	}
}

func TestScriptingCommand_Execute_ScriptFileSuccess(t *testing.T) {
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.testMode = true

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "simple.js")
	if err := os.WriteFile(scriptPath, []byte("ctx.log('ran');"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldwd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"simple.js"}, &stdout, &stderr); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if out := stdout.String(); !strings.Contains(out, "[simple.js] ran") {
		t.Fatalf("expected script log, got %q", out)
	}
}

func TestScriptingCommand_Execute_ScriptFromScriptsDirectory(t *testing.T) {
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.testMode = true

	tmpDir := t.TempDir()
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	scriptPath := filepath.Join(scriptsDir, "nested.js")
	if err := os.WriteFile(scriptPath, []byte("ctx.log('nested');"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldwd)
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"nested.js"}, &stdout, &stderr); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if out := stdout.String(); !strings.Contains(out, "[nested.js] nested") {
		t.Fatalf("expected script log, got %q", out)
	}
}

func TestScriptingCommand_Execute_InlineScript(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.testMode = true
	cmd.script = "ctx.log('inline');"

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if out := stdout.String(); !strings.Contains(out, "[command-line] inline") {
		t.Fatalf("expected inline log, got %q", out)
	}
}

func TestScriptingCommand_Execute_Interactive(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.Global = map[string]string{
		"prompt.color.input":  "green",
		"prompt.color.prefix": "cyan",
		"other":               "ignored",
	}

	cmd := NewScriptingCommand(cfg)
	cmd.interactive = true

	var ran bool
	cmd.terminalFactory = func(ctx context.Context, engine *scripting.Engine) terminalRunner {
		return terminalFunc(func() {
			ran = true
		})
	}

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("execute interactive: %v", err)
	}

	if !ran {
		t.Fatalf("expected terminal Run to be invoked")
	}
}

func TestScriptingCommand_Execute_EngineError(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.engineFactory = func(context.Context, io.Writer, io.Writer) (*scripting.Engine, error) {
		return nil, errors.New("boom")
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "failed to create scripting engine") {
		t.Fatalf("expected engine creation error, got %v", err)
	}
}

func TestScriptingCommand_Execute_InlineScriptFailure(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.script = "throw new Error('kaboom')"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "script execution failed") {
		t.Fatalf("expected script failure, got %v", err)
	}
}
