package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigParsing(t *testing.T) {
	configContent := `# Global options
verbose true
color auto

[help]
pager less
format detailed

[version]
format short`

	config, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Test global options
	if value, ok := config.GetGlobalOption("verbose"); !ok || value != "true" {
		t.Errorf("Expected verbose=true, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetGlobalOption("color"); !ok || value != "auto" {
		t.Errorf("Expected color=auto, got %s (exists: %v)", value, ok)
	}

	// Test command-specific options
	if value, ok := config.GetCommandOption("help", "pager"); !ok || value != "less" {
		t.Errorf("Expected help.pager=less, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetCommandOption("help", "format"); !ok || value != "detailed" {
		t.Errorf("Expected help.format=detailed, got %s (exists: %v)", value, ok)
	}

	// Test fallback to global options
	if value, ok := config.GetCommandOption("help", "verbose"); !ok || value != "true" {
		t.Errorf("Expected help.verbose=true (fallback), got %s (exists: %v)", value, ok)
	}

	// Test non-existent option
	if value, ok := config.GetCommandOption("nonexistent", "option"); ok {
		t.Errorf("Expected nonexistent option to not exist, but got %s", value)
	}
}

func TestEmptyConfig(t *testing.T) {
	config, err := LoadFromReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Failed to load empty config: %v", err)
	}

	if len(config.Global) != 0 {
		t.Errorf("Expected empty global config, got %v", config.Global)
	}

	if len(config.Commands) != 0 {
		t.Errorf("Expected empty commands config, got %v", config.Commands)
	}
}

func TestConfigWithComments(t *testing.T) {
	configContent := `# This is a comment
verbose true
# Another comment
color auto
# Command section
[help]
# Command option comment
pager less`

	config, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("Failed to load config with comments: %v", err)
	}

	if value, ok := config.GetGlobalOption("verbose"); !ok || value != "true" {
		t.Errorf("Expected verbose=true, got %s (exists: %v)", value, ok)
	}

	if value, ok := config.GetCommandOption("help", "pager"); !ok || value != "less" {
		t.Errorf("Expected help.pager=less, got %s (exists: %v)", value, ok)
	}
}

func TestSetGlobalAndCommandOptions(t *testing.T) {
	cfg := NewConfig()

	cfg.SetGlobalOption("color", "auto")
	if got, ok := cfg.GetGlobalOption("color"); !ok || got != "auto" {
		t.Fatalf("expected global option color=auto, got %q exists=%v", got, ok)
	}

	cfg.SetCommandOption("script", "timeout", "30s")
	if got, ok := cfg.GetCommandOption("script", "timeout"); !ok || got != "30s" {
		t.Fatalf("expected command option script.timeout=30s, got %q exists=%v", got, ok)
	}

	// ensure command-specific values take precedence over globals
	cfg.SetGlobalOption("timeout", "10s")
	if got, ok := cfg.GetCommandOption("script", "timeout"); !ok || got != "30s" {
		t.Fatalf("expected command option script.timeout to shadow global, got %q exists=%v", got, ok)
	}
}

func TestLoadFromPathMissing(t *testing.T) {
	path := t.TempDir() + "/missing-config"

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("expected no error loading missing config, got %v", err)
	}

	if len(cfg.Global) != 0 || len(cfg.Commands) != 0 {
		t.Fatalf("expected empty config for missing file, got %+v", cfg)
	}
}

func TestLoadFromPathExisting(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	contents := "verbose true\n[help]\npager less"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("expected load success, got %v", err)
	}

	if got, ok := cfg.GetGlobalOption("verbose"); !ok || got != "true" {
		t.Fatalf("expected verbose global option, got %q exists=%v", got, ok)
	}

	if got, ok := cfg.GetCommandOption("help", "pager"); !ok || got != "less" {
		t.Fatalf("expected help pager option, got %q exists=%v", got, ok)
	}
}

func TestLoadUsesConfigPathEnv(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	if err := os.WriteFile(path, []byte("color auto"), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("ONESHOTMAN_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected load success, got %v", err)
	}

	if got, ok := cfg.GetGlobalOption("color"); !ok || got != "auto" {
		t.Fatalf("expected color option from env-config, got %q exists=%v", got, ok)
	}
}

func TestLoadNoFileReturnsEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	t.Setenv("ONESHOTMAN_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected load success, got %v", err)
	}

	if len(cfg.Global) != 0 || len(cfg.Commands) != 0 {
		t.Fatalf("expected empty config when file missing, got %+v", cfg)
	}
}
