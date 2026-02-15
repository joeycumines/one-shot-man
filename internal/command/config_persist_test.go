package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// --- T065: Schema-aware config set + roundtrip integration tests ---

func TestConfigSet_SchemaValidation_KnownBoolKey_ValidValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"verbose", "true"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("expected no stderr for valid bool value, got: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Set configuration: verbose = true") {
		t.Fatalf("expected confirmation, got: %q", stdout.String())
	}

	// Verify in memory
	if v, ok := cfg.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected in-memory verbose=true, got %q exists=%v", v, ok)
	}

	// Verify on disk
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected disk verbose=true, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_SchemaValidation_KnownBoolKey_InvalidValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"verbose", "banana"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid bool value")
	}
	if !strings.Contains(err.Error(), "invalid value") {
		t.Fatalf("expected 'invalid value' in error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "expected bool") {
		t.Fatalf("expected 'expected bool' in stderr, got: %q", stderr.String())
	}

	// Verify value was NOT set in memory
	if _, ok := cfg.GetGlobalOption("verbose"); ok {
		t.Fatal("expected value NOT to be set in memory for invalid type")
	}

	// Verify file was NOT created
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected config file to not exist, got stat err: %v", err)
	}
}

func TestConfigSet_SchemaValidation_KnownIntKey_InvalidValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"log.buffer-size", "notanumber"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid int value")
	}
	if !strings.Contains(err.Error(), "invalid value") {
		t.Fatalf("expected 'invalid value' in error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "expected int") {
		t.Fatalf("expected 'expected int' in stderr, got: %q", stderr.String())
	}
}

func TestConfigSet_SchemaValidation_KnownDurationKey_InvalidValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"timeout", "notaduration"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid duration value")
	}
	if !strings.Contains(err.Error(), "invalid value") {
		t.Fatalf("expected 'invalid value' in error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "expected duration") {
		t.Fatalf("expected 'expected duration' in stderr, got: %q", stderr.String())
	}
}

func TestConfigSet_SchemaValidation_KnownDurationKey_ValidValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"timeout", "30s"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("expected no stderr for valid duration, got: %q", stderr.String())
	}

	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("timeout"); !ok || v != "30s" {
		t.Fatalf("expected disk timeout=30s, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_SchemaValidation_UnknownKey_Warning(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"my-custom-key", "myvalue"}, &stdout, &stderr); err != nil {
		t.Fatalf("unknown key should not return error: %v", err)
	}

	// Should warn on stderr
	if !strings.Contains(stderr.String(), "not a known configuration key") {
		t.Fatalf("expected unknown key warning in stderr, got: %q", stderr.String())
	}

	// Should still set the value in memory
	if v, ok := cfg.GetGlobalOption("my-custom-key"); !ok || v != "myvalue" {
		t.Fatalf("expected in-memory my-custom-key=myvalue, got %q exists=%v", v, ok)
	}

	// Should still persist to disk
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("my-custom-key"); !ok || v != "myvalue" {
		t.Fatalf("expected disk my-custom-key=myvalue, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_SchemaValidation_KnownStringKey_AnyValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	// String keys accept any value — no type validation
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"color", "rainbow"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("expected no stderr for string key, got: %q", stderr.String())
	}
}

func TestConfigSet_SchemaValidation_KnownIntKey_ValidValue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"log.buffer-size", "2000"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("expected no stderr, got: %q", stderr.String())
	}

	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("log.buffer-size"); !ok || v != "2000" {
		t.Fatalf("expected disk log.buffer-size=2000, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_SchemaValidation_BoolAcceptedValues(t *testing.T) {
	t.Parallel()

	// parseBool in config accepts: true/false/yes/no/1/0/on/off
	accepted := []string{"true", "false", "yes", "no", "1", "0", "on", "off"}
	for _, val := range accepted {
		val := val
		t.Run(val, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config")
			cfg := config.NewConfig()
			cmd := NewConfigCommand(cfg, configPath)

			var stdout, stderr bytes.Buffer
			if err := cmd.Execute([]string{"verbose", val}, &stdout, &stderr); err != nil {
				t.Fatalf("expected valid bool %q to be accepted, got: %v", val, err)
			}
			if stderr.Len() > 0 {
				t.Fatalf("expected no stderr for valid bool %q, got: %q", val, stderr.String())
			}
		})
	}
}

// --- Full roundtrip integration tests ---

func TestConfigSet_FullRoundtrip_SetReadValidate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	// Start from a realistic config file
	initial := "# osm configuration\nverbose false\n\n[help]\npager less\n"
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	// Set a known key
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"verbose", "true"}, &stdout, &stderr); err != nil {
		t.Fatalf("set verbose: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	// Read back via get
	stdout.Reset()
	stderr.Reset()
	if err := cmd.Execute([]string{"verbose"}, &stdout, &stderr); err != nil {
		t.Fatalf("get verbose: %v", err)
	}
	if !strings.Contains(stdout.String(), "verbose: true") {
		t.Fatalf("expected 'verbose: true', got: %q", stdout.String())
	}

	// Validate
	stdout.Reset()
	stderr.Reset()
	if err := cmd.Execute([]string{"validate"}, &stdout, &stderr); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !strings.Contains(stdout.String(), "Configuration is valid") {
		t.Fatalf("expected valid, got: %q", stdout.String())
	}

	// Verify file roundtrip: reload from disk
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if v, ok := reloaded.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected disk verbose=true, got %q exists=%v", v, ok)
	}
	// Verify section data preserved
	if v, ok := reloaded.GetCommandOption("help", "pager"); !ok || v != "less" {
		t.Fatalf("expected help.pager=less preserved, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_FullRoundtrip_MultipleKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	// Set multiple keys in sequence
	keys := []struct{ key, value string }{
		{"verbose", "true"},
		{"color", "always"},
		{"log.buffer-size", "500"},
		{"editor", "vim -u NONE"},
		{"timeout", "1m30s"},
	}

	for _, kv := range keys {
		var stdout, stderr bytes.Buffer
		if err := cmd.Execute([]string{kv.key, kv.value}, &stdout, &stderr); err != nil {
			t.Fatalf("set %s=%s: %v", kv.key, kv.value, err)
		}
		if stderr.Len() > 0 {
			t.Fatalf("unexpected stderr for %s: %q", kv.key, stderr.String())
		}
	}

	// Reload from disk and verify all keys
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	for _, kv := range keys {
		if v, ok := reloaded.GetGlobalOption(kv.key); !ok || v != kv.value {
			t.Fatalf("expected %s=%s on disk, got %q exists=%v", kv.key, kv.value, v, ok)
		}
	}

	// Validate config is clean
	issues := config.ValidateConfig(reloaded, config.DefaultSchema())
	if len(issues) > 0 {
		t.Fatalf("expected valid config after set, got issues: %v", issues)
	}
}

func TestConfigSet_FullRoundtrip_UpdateExistingKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	// Write initial config
	if err := os.WriteFile(configPath, []byte("color auto\nverbose false\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	// Update an existing key
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"color", "never"}, &stdout, &stderr); err != nil {
		t.Fatalf("set color=never: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	// Verify the update on disk (not a duplicate)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "color") != 1 {
		t.Fatalf("expected exactly one 'color' line, got:\n%s", data)
	}
	if !strings.Contains(string(data), "color never") {
		t.Fatalf("expected 'color never', got:\n%s", data)
	}

	// Verify other keys preserved
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := reloaded.GetGlobalOption("verbose"); !ok || v != "false" {
		t.Fatalf("expected verbose=false preserved, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_InvalidType_DoesNotPersist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	// Write initial config with a valid value
	if err := os.WriteFile(configPath, []byte("verbose true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	// Try to set invalid bool value
	var stdout, stderr bytes.Buffer
	err = cmd.Execute([]string{"verbose", "banana"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}

	// Verify disk was NOT changed
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := reloaded.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected original verbose=true preserved on disk, got %q exists=%v", v, ok)
	}
}

func TestConfigSet_PathListKey_AcceptsColonSeparated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	// PathList type accepts any value (validation is pass-through like string)
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"script.paths", "/usr/local/scripts:/home/user/scripts"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("expected no stderr for path-list, got: %q", stderr.String())
	}

	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := reloaded.GetGlobalOption("script.paths"); !ok || v != "/usr/local/scripts:/home/user/scripts" {
		t.Fatalf("expected path-list preserved, got %q exists=%v", v, ok)
	}
}

// --- ValidateOptionValue unit tests ---

func TestValidateOptionValue_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		optType   config.OptionType
		value     string
		wantError bool
	}{
		// string accepts anything
		{"string-any", config.TypeString, "literally anything", false},
		{"string-empty", config.TypeString, "", false},

		// bool
		{"bool-true", config.TypeBool, "true", false},
		{"bool-false", config.TypeBool, "false", false},
		{"bool-yes", config.TypeBool, "yes", false},
		{"bool-no", config.TypeBool, "no", false},
		{"bool-1", config.TypeBool, "1", false},
		{"bool-0", config.TypeBool, "0", false},
		{"bool-on", config.TypeBool, "on", false},
		{"bool-off", config.TypeBool, "off", false},
		{"bool-invalid", config.TypeBool, "banana", true},
		{"bool-empty", config.TypeBool, "", true},

		// int
		{"int-valid", config.TypeInt, "42", false},
		{"int-negative", config.TypeInt, "-1", false},
		{"int-zero", config.TypeInt, "0", false},
		{"int-invalid", config.TypeInt, "notanumber", true},
		{"int-float", config.TypeInt, "3.14", true},

		// duration
		{"duration-30s", config.TypeDuration, "30s", false},
		{"duration-5m", config.TypeDuration, "5m", false},
		{"duration-1h30m", config.TypeDuration, "1h30m", false},
		{"duration-invalid", config.TypeDuration, "notaduration", true},

		// path-list accepts anything
		{"pathlist-any", config.TypePathList, "foo:bar:baz", false},
		{"pathlist-empty", config.TypePathList, "", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := config.ValidateOptionValue(tt.optType, tt.value)
			if tt.wantError && err == nil {
				t.Fatalf("expected error for type=%s value=%q", tt.optType, tt.value)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error for type=%s value=%q: %v", tt.optType, tt.value, err)
			}
		})
	}
}
