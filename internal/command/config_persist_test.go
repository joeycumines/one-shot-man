package command

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
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
	if err := cmd.Execute([]string{"config.schema-version", "1"}, &stdout, &stderr); err != nil {
		t.Fatalf("set config.schema-version: %v", err)
	}

	// Set a known key
	stdout.Reset()
	stderr.Reset()
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

// --- T119: config list and config diff subcommands ---

func TestConfigList_ShowsAllKeys(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"list"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Must contain the header
	if !strings.Contains(out, "KEY") || !strings.Contains(out, "VALUE") || !strings.Contains(out, "SOURCE") {
		t.Fatalf("expected table header, got:\n%s", out)
	}

	// Check that all global schema keys appear
	schema := config.DefaultSchema()
	for _, opt := range schema.GlobalOptions() {
		if !strings.Contains(out, opt.Key) {
			t.Errorf("expected key %q in list output", opt.Key)
		}
	}
}

func TestConfigList_ShowsDefaultSource(t *testing.T) {
	// Unset all schema env vars that could interfere.
	// Can't use t.Parallel() because we modify environment directly.
	envVars := []string{"OSM_SESSION_ID", "EDITOR", "OSM_LOG_FILE", "OSM_LOG_LEVEL"}
	saved := make(map[string]string)
	for _, e := range envVars {
		if v, ok := os.LookupEnv(e); ok {
			saved[e] = v
		}
		os.Unsetenv(e)
	}
	t.Cleanup(func() {
		for _, e := range envVars {
			if v, ok := saved[e]; ok {
				os.Setenv(e, v)
			} else {
				os.Unsetenv(e)
			}
		}
	})

	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"list"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// With no customizations and no env vars, everything should be "default"
	for line := range strings.SplitSeq(out, "\n") {
		if line == "" || strings.HasPrefix(line, "KEY") {
			continue
		}
		if !strings.Contains(line, "default") {
			t.Errorf("expected 'default' source in line: %q", line)
		}
	}
}

func TestConfigList_ShowsConfigSource(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("color", "always")
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"list"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Find the line with "color" — it should say "config"
	found := false
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "color") {
			found = true
			if !strings.Contains(line, "config") {
				t.Errorf("expected 'config' source for color, got: %q", line)
			}
			if !strings.Contains(line, "always") {
				t.Errorf("expected value 'always' for color, got: %q", line)
			}
		}
	}
	if !found {
		t.Fatal("expected 'color' key in list output")
	}
}

func TestConfigList_ShowsEnvSource(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	t.Setenv("OSM_SESSION_ID", "test-session-42")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"list"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	found := false
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "session.id") {
			found = true
			if !strings.Contains(line, "env") {
				t.Errorf("expected 'env' source for session.id, got: %q", line)
			}
			if !strings.Contains(line, "test-session-42") {
				t.Errorf("expected value 'test-session-42' for session.id, got: %q", line)
			}
		}
	}
	if !found {
		t.Fatal("expected 'session.id' key in list output")
	}
}

func TestConfigList_RejectsExtraArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"list", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !errors.Is(err, ErrUnexpectedArguments) {
		t.Fatalf("expected ErrUnexpectedArguments, got: %v", err)
	}
}

func TestConfigDiff_ShowsNonDefaults(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("verbose", "true")
	cfg.SetGlobalOption("color", "never")
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"diff"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	// Header must be present
	if !strings.Contains(out, "KEY") {
		t.Fatalf("expected table header, got:\n%s", out)
	}
	// verbose and color should appear
	if !strings.Contains(out, "verbose") {
		t.Error("expected 'verbose' in diff output")
	}
	if !strings.Contains(out, "color") {
		t.Error("expected 'color' in diff output")
	}
	// Keys that are at default should NOT appear — pick one that's definitely default
	if strings.Contains(out, "pager") {
		t.Error("expected default key 'pager' to NOT appear in diff output")
	}
}

func TestConfigDiff_AllDefaults(t *testing.T) {
	// Unset all schema env vars that could make diff show non-defaults.
	envVars := []string{"OSM_SESSION_ID", "EDITOR", "OSM_LOG_FILE", "OSM_LOG_LEVEL"}
	saved := make(map[string]string)
	for _, e := range envVars {
		if v, ok := os.LookupEnv(e); ok {
			saved[e] = v
		}
		os.Unsetenv(e)
	}
	t.Cleanup(func() {
		for _, e := range envVars {
			if v, ok := saved[e]; ok {
				os.Setenv(e, v)
			} else {
				os.Unsetenv(e)
			}
		}
	})

	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"diff"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "All values are at their defaults.") {
		t.Fatalf("expected defaults message, got:\n%s", out)
	}
}

func TestConfigDiff_IncludesEnvOverrides(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	t.Setenv("OSM_LOG_LEVEL", "debug")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"diff"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "log.level") {
		t.Error("expected 'log.level' in diff output when env overrides default")
	}
	if !strings.Contains(out, "debug") {
		t.Error("expected value 'debug' in diff output")
	}
	if !strings.Contains(out, "env") {
		t.Error("expected 'env' source in diff output")
	}
}

func TestConfigDiff_RejectsExtraArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"diff", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !errors.Is(err, ErrUnexpectedArguments) {
		t.Fatalf("expected ErrUnexpectedArguments, got: %v", err)
	}
}

func TestConfigUsage_IncludesListAndDiff(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "config list") {
		t.Error("expected usage to mention 'config list'")
	}
	if !strings.Contains(out, "config diff") {
		t.Error("expected usage to mention 'config diff'")
	}
	if !strings.Contains(out, "config reset") {
		t.Error("expected usage to mention 'config reset'")
	}
}

// --- T206: config reset subcommand ---

func TestConfigReset_SingleKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	if err := os.WriteFile(configPath, []byte("verbose true\ncolor always\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"reset", "color"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Reset color to default:") {
		t.Fatalf("expected reset confirmation, got: %q", out)
	}

	// Verify in-memory removal.
	if _, ok := cfg.GetGlobalOption("color"); ok {
		t.Fatal("expected 'color' removed from in-memory config")
	}
	// Other keys should be preserved.
	if v, ok := cfg.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected verbose=true preserved, got %q exists=%v", v, ok)
	}

	// Verify on disk.
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.GetGlobalOption("color"); ok {
		t.Fatal("expected 'color' removed from disk config")
	}
	if v, ok := reloaded.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatalf("expected verbose=true preserved on disk, got %q exists=%v", v, ok)
	}
}

func TestConfigReset_SingleKey_UnknownKey(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("unknown-key", "foo")
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "unknown-key"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown configuration key") {
		t.Fatalf("expected 'unknown configuration key' in error, got: %v", err)
	}
}

func TestConfigReset_SingleKey_NotSet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	if err := os.WriteFile(configPath, []byte("verbose true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	// Reset a key that exists in schema but is not set in config.
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"reset", "color"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Reset color to default:") {
		t.Fatalf("expected reset confirmation, got: %q", out)
	}
}

func TestConfigReset_AllKeys_RequiresForce(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("verbose", "true")
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--all"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error without --force")
	}
	if !strings.Contains(err.Error(), "requires --force") {
		t.Fatalf("expected 'requires --force' in error, got: %v", err)
	}

	// Verify value was NOT cleared.
	if v, ok := cfg.GetGlobalOption("verbose"); !ok || v != "true" {
		t.Fatal("expected value preserved when --force is missing")
	}
}

func TestConfigReset_AllKeys_WithForce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	if err := os.WriteFile(configPath, []byte("verbose true\ncolor always\neditor vim\n\n[help]\npager less\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"reset", "--all", "--force"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Reset") && !strings.Contains(out, "key(s) to defaults") {
		t.Fatalf("expected reset confirmation, got: %q", out)
	}

	// Verify in-memory: no global keys.
	if len(cfg.Global) != 0 {
		t.Fatalf("expected no global keys in memory, got %v", cfg.Global)
	}

	// Verify on disk: global keys removed, section preserved.
	reloaded, err := config.LoadFromPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Global) != 0 {
		t.Fatalf("expected no global keys on disk, got %v", reloaded.Global)
	}
	if v, ok := reloaded.GetCommandOption("help", "pager"); !ok || v != "less" {
		t.Fatalf("expected help.pager=less preserved on disk, got %q exists=%v", v, ok)
	}
}

func TestConfigReset_NoArgs_ShowsUsage(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"reset"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("expected usage text, got: %q", out)
	}
	if !strings.Contains(out, "config reset <key>") {
		t.Fatalf("expected single key usage, got: %q", out)
	}
	if !strings.Contains(out, "config reset --all") {
		t.Fatalf("expected --all usage, got: %q", out)
	}
}

func TestConfigReset_UnknownFlag(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected 'unknown flag' in error, got: %v", err)
	}
}

func TestConfigReset_AllAndKey_Conflict(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--all", "verbose"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for --all with key")
	}
	if !strings.Contains(err.Error(), "cannot specify both") {
		t.Fatalf("expected 'cannot specify both' in error, got: %v", err)
	}
}

func TestConfigReset_DuplicateKeyArg(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "verbose", "color"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for multiple keys")
	}
	if !errors.Is(err, ErrUnexpectedArguments) {
		t.Fatalf("expected ErrUnexpectedArguments, got: %v", err)
	}
}

// --- T222: config validate schema version tests ---

func TestConfigValidate_SchemaVersion_Current(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("config.schema-version", "1")
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"validate"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Configuration is valid") {
		t.Fatalf("expected valid config when schema version is current, got: %q", out)
	}
	if strings.Contains(out, "schema version") {
		t.Fatalf("expected no schema version warning for current version, got: %q", out)
	}
}

func TestConfigValidate_SchemaVersion_Old(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	// No config.schema-version → version 0 (outdated)

	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"validate"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "outdated") {
		t.Fatalf("expected 'outdated' warning for old schema version, got: %q", out)
	}
}

func TestConfigValidate_SchemaVersion_Future(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("config.schema-version", "999")
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"validate"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "newer than supported") {
		t.Fatalf("expected 'newer than supported' warning for future version, got: %q", out)
	}
}

// --- T222: config schema --json tests ---

func TestConfigSchema_JSON(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"schema", "--json"}, &stdout, &stderr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	var entries []config.SchemaEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("schema --json output is not valid JSON: %v\noutput: %s", err, out)
	}

	if len(entries) == 0 {
		t.Fatal("expected non-empty schema entries")
	}

	// Verify config.schema-version is present.
	found := false
	for _, e := range entries {
		if e.Key == "config.schema-version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected config.schema-version in JSON schema output")
	}
}

func TestConfigSchema_JSON_ExtraArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"schema", "--json", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args after --json")
	}
	if !errors.Is(err, ErrUnexpectedArguments) {
		t.Fatalf("expected ErrUnexpectedArguments, got: %v", err)
	}
}

// --- T76: config reset disk error path tests ---

func TestConfigReset_SingleKey_DiskWriteError(t *testing.T) {
	t.Parallel()

	// Skip on Windows: ENOTDIR path trick for disk write error simulation
	// behaves differently. Windows returns ENOENT for this case, which is
	// caught by the early "file doesn't exist" check, preventing the error
	// path from being tested. The test still provides value on Unix platforms.
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR path trick for disk write error simulation behaves differently on Windows")
	}

	// Create a config path that triggers a non-IsNotExist error in
	// DeleteKeyInFile. Using a regular file as a directory component
	// causes ENOTDIR on all platforms.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(blocker, "config") // blocker is a file, not a dir

	cfg := config.NewConfig()
	cfg.SetGlobalOption("color", "always")
	cmd := NewConfigCommand(cfg, badPath)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "color"}, &stdout, &stderr)

	// Should succeed (returns nil) despite disk error.
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Warning should be on stderr.
	if !strings.Contains(stderr.String(), "Warning: failed to persist reset to disk") {
		t.Fatalf("expected disk write warning in stderr, got: %q", stderr.String())
	}

	// Stdout should still have the reset confirmation.
	if !strings.Contains(stdout.String(), "Reset color to default:") {
		t.Fatalf("expected reset confirmation in stdout, got: %q", stdout.String())
	}
}

func TestConfigReset_AllKeys_DiskWriteError(t *testing.T) {
	t.Parallel()

	// Skip on Windows: ENOTDIR path trick for disk write error simulation
	// behaves differently. Windows returns ENOENT for this case, which is
	// caught by the early "file doesn't exist" check, preventing the error
	// path from being tested. The test still provides value on Unix platforms.
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR path trick for disk write error simulation behaves differently on Windows")
	}

	// Same strategy: use a file as a directory component.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(blocker, "config")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("verbose", "true")
	cfg.SetGlobalOption("color", "always")
	cmd := NewConfigCommand(cfg, badPath)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--all", "--force"}, &stdout, &stderr)

	// Should succeed despite disk error.
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	// Warning should be on stderr.
	if !strings.Contains(stderr.String(), "Warning: failed to persist reset to disk") {
		t.Fatalf("expected disk write warning in stderr, got: %q", stderr.String())
	}

	// Stdout should have reset count.
	if !strings.Contains(stdout.String(), "Reset") {
		t.Fatalf("expected reset confirmation in stdout, got: %q", stdout.String())
	}

	// In-memory config should be cleared.
	if len(cfg.Global) != 0 {
		t.Fatalf("expected empty global config after reset --all, got: %v", cfg.Global)
	}
}
