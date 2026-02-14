package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetKeyInFile_NewKeyEmptyFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	if err := SetKeyInFile(path, "color", "auto"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	if got := strings.TrimSpace(string(data)); got != "color auto" {
		t.Fatalf("expected 'color auto', got %q", got)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("color"); !ok || v != "auto" {
		t.Fatalf("expected color=auto after round-trip, got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_NewKeyExistingFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	if err := os.WriteFile(path, []byte("verbose true\n"), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "color", "auto"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "verbose true") {
		t.Fatalf("expected existing key to be preserved, got %q", content)
	}
	if !strings.Contains(content, "color auto") {
		t.Fatalf("expected new key to be added, got %q", content)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected verbose=true, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetGlobalOption("color"); !ok || v != "auto" {
		t.Fatalf("expected color=auto, got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_UpdateExistingKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	if err := os.WriteFile(path, []byte("verbose true\ncolor auto\n"), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "color", "never"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if strings.Count(content, "color") != 1 {
		t.Fatalf("expected exactly one 'color' line, got %q", content)
	}
	if !strings.Contains(content, "color never") {
		t.Fatalf("expected updated value, got %q", content)
	}
	if !strings.Contains(content, "verbose true") {
		t.Fatalf("expected other keys preserved, got %q", content)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("color"); !ok || v != "never" {
		t.Fatalf("expected color=never, got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_PreservesComments(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	initial := "# This is a configuration file\n# Global options\nverbose true\n\n# Color settings\ncolor auto\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "color", "always"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# This is a configuration file") {
		t.Fatalf("expected first comment preserved, got %q", content)
	}
	if !strings.Contains(content, "# Global options") {
		t.Fatalf("expected second comment preserved, got %q", content)
	}
	if !strings.Contains(content, "# Color settings") {
		t.Fatalf("expected third comment preserved, got %q", content)
	}
	if !strings.Contains(content, "color always") {
		t.Fatalf("expected updated value, got %q", content)
	}
}

func TestSetKeyInFile_InsertsBeforeFirstSection(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	initial := "verbose true\n\n[help]\npager less\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "color", "auto"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	colorIdx := strings.Index(content, "color auto")
	sectionIdx := strings.Index(content, "[help]")
	if colorIdx < 0 || sectionIdx < 0 {
		t.Fatalf("expected both 'color auto' and '[help]' in content, got %q", content)
	}
	if colorIdx > sectionIdx {
		t.Fatalf("expected 'color auto' to appear before '[help]', got %q", content)
	}

	if !strings.Contains(content, "pager less") {
		t.Fatalf("expected section options preserved, got %q", content)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("color"); !ok || v != "auto" {
		t.Fatalf("expected color=auto, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetCommandOption("help", "pager"); !ok || v != "less" {
		t.Fatalf("expected help.pager=less, got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_DoesNotMatchKeyInSection(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	initial := "verbose true\n\n[version]\nformat short\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "format", "json"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if strings.Count(content, "format") != 2 {
		t.Fatalf("expected exactly two 'format' lines (global + section), got %q", content)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("format"); !ok || v != "json" {
		t.Fatalf("expected global format=json, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetCommandOption("version", "format"); !ok || v != "short" {
		t.Fatalf("expected version.format=short, got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_CreatesParentDirectory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "deep", "config")

	if err := SetKeyInFile(path, "color", "auto"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	if got := strings.TrimSpace(string(data)); got != "color auto" {
		t.Fatalf("expected 'color auto', got %q", got)
	}
}

func TestSetKeyInFile_AtomicWrite(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	if err := os.WriteFile(path, []byte("verbose true\n"), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "color", "auto"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file should exist after write: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("config file should not be empty after write")
	}

	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-session-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestSetKeyInFile_ValueWithSpaces(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	if err := SetKeyInFile(path, "editor", "vim -u NONE"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("editor"); !ok || v != "vim -u NONE" {
		t.Fatalf("expected editor='vim -u NONE', got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_EmptyValue(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	if err := SetKeyInFile(path, "session.id", ""); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	if got := strings.TrimSpace(string(data)); got != "session.id" {
		t.Fatalf("expected 'session.id', got %q", got)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if v, ok := cfg.GetGlobalOption("session.id"); !ok || v != "" {
		t.Fatalf("expected session.id='', got %q exists=%v", v, ok)
	}
}

func TestSetKeyInFile_MultipleSequentialWrites(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	keys := []struct {
		key, value string
	}{
		{"verbose", "true"},
		{"color", "auto"},
		{"editor", "nano"},
	}

	for _, kv := range keys {
		if err := SetKeyInFile(path, kv.key, kv.value); err != nil {
			t.Fatalf("SetKeyInFile(%q, %q) returned error: %v", kv.key, kv.value, err)
		}
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}

	for _, kv := range keys {
		if v, ok := cfg.GetGlobalOption(kv.key); !ok || v != kv.value {
			t.Fatalf("expected %s=%s, got %q exists=%v", kv.key, kv.value, v, ok)
		}
	}
}

func TestSetKeyInFile_ComplexConfigRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config")

	initial := "# one-shot-man configuration file\n" +
		"# Format: optionName remainingLineIsTheValue\n" +
		"\n" +
		"# Global options\n" +
		"verbose false\n" +
		"color auto\n" +
		"\n" +
		"# Prompt color overrides\n" +
		"# prompt.color.input green\n" +
		"\n" +
		"[help]\n" +
		"pager less\n" +
		"\n" +
		"[version]\n" +
		"format full\n" +
		"\n" +
		"[sessions]\n" +
		"maxAgeDays 90\n" +
		"maxCount 100\n"

	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := SetKeyInFile(path, "color", "always"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	if err := SetKeyInFile(path, "editor", "vim"); err != nil {
		t.Fatalf("SetKeyInFile returned error: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}

	if v, ok := cfg.GetGlobalOption("verbose"); !ok || v != "false" {
		t.Fatalf("expected verbose=false, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetGlobalOption("color"); !ok || v != "always" {
		t.Fatalf("expected color=always, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetGlobalOption("editor"); !ok || v != "vim" {
		t.Fatalf("expected editor=vim, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetCommandOption("help", "pager"); !ok || v != "less" {
		t.Fatalf("expected help.pager=less, got %q exists=%v", v, ok)
	}
	if v, ok := cfg.GetCommandOption("version", "format"); !ok || v != "full" {
		t.Fatalf("expected version.format=full, got %q exists=%v", v, ok)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# one-shot-man configuration file") {
		t.Fatalf("expected header comment preserved, got %q", content)
	}
	if !strings.Contains(content, "# prompt.color.input green") {
		t.Fatalf("expected commented-out option preserved, got %q", content)
	}
}
