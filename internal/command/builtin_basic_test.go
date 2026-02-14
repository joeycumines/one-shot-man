package command

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestHelpCommandListsBuiltins(t *testing.T) {
	t.Parallel()
	registry := &Registry{commands: make(map[string]Command)}
	version := NewVersionCommand("1.2.3")
	helper := NewHelpCommand(registry)
	registry.Register(helper)
	registry.Register(version)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := helper.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("help execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Available commands") {
		t.Fatalf("expected help output to list available commands, got %q", output)
	}

	if !strings.Contains(output, "version") {
		t.Fatalf("expected help output to include version command, got %q", output)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}

func TestHelpCommandSpecificCommand(t *testing.T) {
	t.Parallel()
	registry := &Registry{commands: make(map[string]Command)}
	helper := NewHelpCommand(registry)
	version := NewVersionCommand("9.9.9")
	registry.Register(helper)
	registry.Register(version)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := helper.Execute([]string{"version"}, &stdout, &stderr); err != nil {
		t.Fatalf("help execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Command: version") {
		t.Fatalf("expected detailed help for version command, got %q", output)
	}
}

func TestHelpCommandShowsCommandFlags(t *testing.T) {
	t.Parallel()
	registry := &Registry{commands: make(map[string]Command)}
	helper := NewHelpCommand(registry)
	cfg := config.NewConfig()
	configCmd := NewConfigCommand(cfg)
	registry.Register(helper)
	registry.Register(configCmd)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := helper.Execute([]string{"config"}, &stdout, &stderr); err != nil {
		t.Fatalf("help execute returned error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Flags:") || !strings.Contains(out, "-global") || !strings.Contains(out, "-all") {
		t.Fatalf("expected config help to mention flags, got %q", out)
	}
}

func TestHelpCommandUnknownCommand(t *testing.T) {
	t.Parallel()
	registry := &Registry{commands: make(map[string]Command)}
	helper := NewHelpCommand(registry)
	registry.Register(helper)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := helper.Execute([]string{"missing"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error for unknown command")
	}

	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", stdout.String())
	}

	if !strings.Contains(stderr.String(), "Unknown command: missing") {
		t.Fatalf("expected stderr to mention missing command, got %q", stderr.String())
	}
}

func TestVersionCommandExecute(t *testing.T) {
	t.Parallel()
	cmd := NewVersionCommand("0.0.1-test")
	var stdout bytes.Buffer

	if err := cmd.Execute(nil, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("version execute returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "one-shot-man version 0.0.1-test") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}

func TestVersionCommandRejectsUnexpectedArgs(t *testing.T) {
	t.Parallel()
	cmd := NewVersionCommand("1.0.0")
	var stdout, stderr bytes.Buffer

	err := cmd.Execute([]string{"extra", "args"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unexpected arguments")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("expected 'unexpected arguments' error, got %q", err.Error())
	}
	if !strings.Contains(stderr.String(), "unexpected arguments") {
		t.Fatalf("expected stderr to mention unexpected arguments, got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", stdout.String())
	}
}

func TestConfigCommandShowAll(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("color", "auto")
	cfg.SetCommandOption("help", "pager", "less")

	cmd := NewConfigCommand(cfg)
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse([]string{"--all"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	var stdout bytes.Buffer
	if err := cmd.Execute(fs.Args(), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("config execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Global configuration:") {
		t.Fatalf("expected global configuration header, got %q", output)
	}
	if !strings.Contains(output, "color: auto") {
		t.Fatalf("expected global option output, got %q", output)
	}
	if !strings.Contains(output, "[help]") {
		t.Fatalf("expected command-specific header, got %q", output)
	}
}

func TestConfigCommandGetAndSet(t *testing.T) {
	// Note: not parallel because we need to control OSM_CONFIG env var
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	t.Setenv("OSM_CONFIG", configPath)

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	cfg.SetGlobalOption("color", "auto")
	cmd := NewConfigCommand(cfg)
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// get existing key
	if err := cmd.Execute([]string{"color"}, &stdout, &stderr); err != nil {
		t.Fatalf("config execute returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "color: auto") {
		t.Fatalf("expected get output for color, got %q", stdout.String())
	}

	// set new key with valid global option
	stdout.Reset()
	if err := cmd.Execute([]string{"debug", "true"}, &stdout, &stderr); err != nil {
		t.Fatalf("config execute returned error on set: %v", err)
	}
	if !strings.Contains(stdout.String(), "Set configuration: debug = true") {
		t.Fatalf("expected confirmation message, got %q", stdout.String())
	}
	if value, ok := cfg.GetGlobalOption("debug"); !ok || value != "true" {
		t.Fatalf("expected debug option to be set, got %q exists=%v", value, ok)
	}

	// invalid arg count
	stdout.Reset()
	stderr.Reset()
	err = cmd.Execute([]string{"too", "many", "args"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error for invalid arguments")
	}
	if !strings.Contains(stderr.String(), "Invalid number of arguments") {
		t.Fatalf("expected invalid argument message, got %q", stderr.String())
	}
}

func TestConfigCommandPersistsToDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	// Seed with existing content
	initial := "verbose true\n"
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"color", "always"}, &stdout, &stderr); err != nil {
		t.Fatalf("config set returned error: %v", err)
	}

	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Set configuration: color = always") {
		t.Fatalf("expected confirmation, got %q", stdout.String())
	}

	// Verify the file was written
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("color"); !ok || v != "always" {
		t.Fatalf("expected color=always on disk, got %q exists=%v", v, ok)
	}
	if v, ok := reloaded.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected verbose=true preserved, got %q exists=%v", v, ok)
	}
}

func TestConfigCommandPersistsNewFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "config")

	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"editor", "nano"}, &stdout, &stderr); err != nil {
		t.Fatalf("config set returned error: %v", err)
	}

	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("editor"); !ok || v != "nano" {
		t.Fatalf("expected editor=nano on disk, got %q exists=%v", v, ok)
	}
}

func TestConfigCommandValidate(t *testing.T) {
	t.Parallel()

	t.Run("ValidConfig", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.SetGlobalOption("verbose", "true")
		cfg.SetGlobalOption("color", "auto")
		cmd := NewConfigCommand(cfg)

		var stdout, stderr bytes.Buffer
		if err := cmd.Execute([]string{"validate"}, &stdout, &stderr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout.String(), "Configuration is valid") {
			t.Fatalf("expected valid config message, got %q", stdout.String())
		}
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.SetGlobalOption("verbose", "notabool")
		cfg.SetGlobalOption("unknownkey", "value")
		cmd := NewConfigCommand(cfg)

		var stdout, stderr bytes.Buffer
		if err := cmd.Execute([]string{"validate"}, &stdout, &stderr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		output := stdout.String()
		if !strings.Contains(output, "issue(s)") {
			t.Fatalf("expected issue count, got %q", output)
		}
		if !strings.Contains(output, "expected bool") {
			t.Fatalf("expected type mismatch in output, got %q", output)
		}
		if !strings.Contains(output, "unknown") {
			t.Fatalf("expected unknown option in output, got %q", output)
		}
	})
}

func TestConfigCommandSchema(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"schema"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Global Options:") {
		t.Fatalf("expected 'Global Options:' in schema output, got %q", output)
	}
	if !strings.Contains(output, "verbose") {
		t.Fatalf("expected 'verbose' in schema output, got %q", output)
	}
	if !strings.Contains(output, "[help] Options:") {
		t.Fatalf("expected '[help] Options:' in schema output, got %q", output)
	}
}

func TestConfigCommandResolve(t *testing.T) {
	t.Parallel()

	t.Run("ResolveWithDefault", func(t *testing.T) {
		// "verbose" has Default: "false" in schema, not set in config.
		cfg := config.NewConfig()
		cmd := NewConfigCommand(cfg)

		var stdout, stderr bytes.Buffer
		if err := cmd.Execute([]string{"verbose"}, &stdout, &stderr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should resolve to schema default "false".
		if !strings.Contains(stdout.String(), "verbose: false") {
			t.Fatalf("expected schema default, got %q", stdout.String())
		}
	})

	t.Run("ResolveWithConfigValue", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.SetGlobalOption("color", "always")
		cmd := NewConfigCommand(cfg)

		var stdout, stderr bytes.Buffer
		if err := cmd.Execute([]string{"color"}, &stdout, &stderr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout.String(), "color: always") {
			t.Fatalf("expected config value, got %q", stdout.String())
		}
	})

	t.Run("ResolveUnknownKey", func(t *testing.T) {
		cfg := config.NewConfig()
		cmd := NewConfigCommand(cfg)

		var stdout, stderr bytes.Buffer
		if err := cmd.Execute([]string{"nonexistent"}, &stdout, &stderr); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(stdout.String(), "not found") {
			t.Fatalf("expected 'not found', got %q", stdout.String())
		}
	})
}

func TestConfigCommandUsageShowsSubcommands(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "validate") {
		t.Fatalf("expected 'validate' in usage, got %q", output)
	}
	if !strings.Contains(output, "schema") {
		t.Fatalf("expected 'schema' in usage, got %q", output)
	}
}

func TestInitCommandExistingConfigWithoutForce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("existing"), 0600); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}
	t.Setenv("OSM_CONFIG", configPath)

	cmd := NewInitCommand()
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	var stdout bytes.Buffer
	if err := cmd.Execute(fs.Args(), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("init execute returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Configuration already exists") {
		t.Fatalf("expected notice about existing config, got %q", output)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("expected config file to remain unchanged, got %q", string(data))
	}
}

func TestInitCommandRejectsUnexpectedArgs(t *testing.T) {
	t.Parallel()
	cmd := NewInitCommand()
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	var stdout, stderr bytes.Buffer

	err := cmd.Execute([]string{"unexpected"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unexpected arguments")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("expected 'unexpected arguments' error, got %q", err.Error())
	}
	if !strings.Contains(stderr.String(), "unexpected arguments") {
		t.Fatalf("expected stderr to mention unexpected arguments, got %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout output, got %q", stdout.String())
	}
}

func TestInitCommandForceCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "config")
	t.Setenv("OSM_CONFIG", configPath)

	cmd := NewInitCommand()
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	if err := fs.Parse([]string{"--force"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := cmd.Execute(fs.Args(), &stdout, &stderr); err != nil {
		t.Fatalf("init execute returned error: %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Initialized one-shot-man configuration") {
		t.Fatalf("expected initialization message, got %q", output)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected generated config to be non-empty")
	}
}
