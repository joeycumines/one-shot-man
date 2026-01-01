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
	t.Parallel()
	cfg := config.NewConfig()
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

	// set new key
	stdout.Reset()
	if err := cmd.Execute([]string{"theme", "dark"}, &stdout, &stderr); err != nil {
		t.Fatalf("config execute returned error on set: %v", err)
	}
	if !strings.Contains(stdout.String(), "Set configuration: theme = dark") {
		t.Fatalf("expected confirmation message, got %q", stdout.String())
	}
	if value, ok := cfg.GetGlobalOption("theme"); !ok || value != "dark" {
		t.Fatalf("expected theme option to be set, got %q exists=%v", value, ok)
	}

	// invalid arg count
	stdout.Reset()
	stderr.Reset()
	err := cmd.Execute([]string{"too", "many", "args"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error for invalid arguments")
	}
	if !strings.Contains(stderr.String(), "Invalid number of arguments") {
		t.Fatalf("expected invalid argument message, got %q", stderr.String())
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
