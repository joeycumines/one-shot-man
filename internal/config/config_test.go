package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigParsing(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

	t.Setenv("OSM_CONFIG", path)

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
	t.Setenv("OSM_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected load success, got %v", err)
	}

	if len(cfg.Global) != 0 || len(cfg.Commands) != 0 {
		t.Fatalf("expected empty config when file missing, got %+v", cfg)
	}
}

func TestMissingConfigFiles(t *testing.T) {
	t.Parallel()

	t.Run("ConfigFileIsDirectory", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "configdir")
		if err := os.MkdirAll(configPath, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		if err == nil {
			t.Fatalf("expected error when config file is a directory, got config: %+v", cfg)
		}
	})

	t.Run("ConfigFilePathTooLong", func(t *testing.T) {
		dir := t.TempDir()

		// Create a path that exceeds typical OS limits (255+ chars for filename)
		longName := strings.Repeat("a", 300)
		configPath := filepath.Join(dir, longName)

		cfg, err := LoadFromPath(configPath)
		if err == nil {
			t.Fatalf("expected error when config path is too long, got config: %+v", cfg)
		}
	})

	t.Run("ConfigFilePathWithControlCharacters", func(t *testing.T) {
		dir := t.TempDir()
		// Some systems may not allow control characters in paths
		configPath := filepath.Join(dir, "config\nwith\ttabs")

		cfg, err := LoadFromPath(configPath)
		if err == nil {
			// If it succeeded, verify the config is empty (file shouldn't exist)
			if len(cfg.Global) != 0 || len(cfg.Commands) != 0 {
				t.Fatalf("expected empty config for non-existent path, got %+v", cfg)
			}
		}
	})
}

func TestInvalidConfigurationValues(t *testing.T) {
	t.Parallel()

	t.Run("EmptySessionID", func(t *testing.T) {
		// Config can have empty values - this is allowed by the parser
		// Empty values should be stored and retrievable
		configContent := "session.id "
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error for empty value, got: %v", err)
		}

		value, ok := cfg.GetGlobalOption("session.id")
		if !ok {
			t.Fatalf("expected empty session.id option to be stored")
		}
		if value != "" {
			t.Fatalf("expected empty string value, got %q", value)
		}
	})

	t.Run("SessionIDWithSpecialCharacters", func(t *testing.T) {
		// Config parser should accept special characters in values
		configContent := "session.id test-session_2024.01+user@host"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error for special chars, got: %v", err)
		}

		value, ok := cfg.GetGlobalOption("session.id")
		if !ok {
			t.Fatalf("expected session.id option to be stored")
		}
		if value != "test-session_2024.01+user@host" {
			t.Fatalf("expected full value, got %q", value)
		}
	})

	t.Run("NegativeValues", func(t *testing.T) {
		// Config parser accepts negative numeric values
		configContent := "timeout -30\nretries -5"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error for negative values, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("timeout"); !ok || value != "-30" {
			t.Errorf("expected timeout=-30, got %s (exists: %v)", value, ok)
		}
	})

	t.Run("VeryLargeValues", func(t *testing.T) {
		// Config parser should accept very large values
		largeValue := strings.Repeat("x", 10000)
		configContent := "data " + largeValue

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error for large values, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("data"); !ok || value != largeValue {
			t.Fatalf("expected large value to be stored correctly")
		}
	})

	t.Run("UnicodeValues", func(t *testing.T) {
		// Config parser should accept unicode characters
		configContent := "description Hello 世界 🌍 Ñoño"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error for unicode, got: %v", err)
		}

		expected := "Hello 世界 🌍 Ñoño"
		if value, ok := cfg.GetGlobalOption("description"); !ok || value != expected {
			t.Fatalf("expected unicode value %q, got %q", expected, value)
		}
	})

	t.Run("ValuesWithNewlines", func(t *testing.T) {
		// Values with embedded newlines should be handled
		// The parser treats each line as a separate entry
		configContent := `option1 line1
option2 line2
option3 line3`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if _, ok := cfg.GetGlobalOption("option1"); !ok || cfg.Global["option1"] != "line1" {
			t.Errorf("option1 not parsed correctly")
		}
		if _, ok := cfg.GetGlobalOption("option2"); !ok || cfg.Global["option2"] != "line2" {
			t.Errorf("option2 not parsed correctly")
		}
		if _, ok := cfg.GetGlobalOption("option3"); !ok || cfg.Global["option3"] != "line3" {
			t.Errorf("option3 not parsed correctly")
		}
	})
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	// Note: Cannot use t.Parallel() here because t.Setenv is used

	t.Run("OSMConfigPointsToMissingFile", func(t *testing.T) {
		dir := t.TempDir()
		missingPath := filepath.Join(dir, "nonexistent-config")
		t.Setenv("OSM_CONFIG", missingPath)

		// Should return empty config, not an error
		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error when OSM_CONFIG points to missing file, got: %v", err)
		}

		if len(cfg.Global) != 0 || len(cfg.Commands) != 0 {
			t.Fatalf("expected empty config, got %+v", cfg)
		}
	})

	t.Run("OSMConfigPointsToDirectory", func(t *testing.T) {
		dir := t.TempDir()
		configDir := filepath.Join(dir, "configdir")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		t.Setenv("OSM_CONFIG", configDir)

		cfg, err := Load()
		if err == nil {
			t.Fatalf("expected error when OSM_CONFIG points to directory, got config: %+v", cfg)
		}
	})

	t.Run("OSMConfigWithInvalidPathCharacters", func(t *testing.T) {
		dir := t.TempDir()
		// Path with quotes and special shell chars - should be treated as literal path
		invalidPath := filepath.Join(dir, "config\"with'quotes")
		if err := os.WriteFile(invalidPath, []byte("test true"), 0600); err != nil {
			// If we can't create the file, that's okay - we're testing path handling
			t.Logf("could not create file with quotes in name: %v", err)
			return
		}
		t.Setenv("OSM_CONFIG", invalidPath)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error loading config with quotes in path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("test"); !ok || value != "true" {
			t.Fatalf("expected test=true, got %s (exists: %v)", value, ok)
		}
	})

	t.Run("OSMConfigWithSpaces", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "my config file")
		if err := os.WriteFile(configPath, []byte("color auto"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}
		t.Setenv("OSM_CONFIG", configPath)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error with spaces in path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("color"); !ok || value != "auto" {
			t.Fatalf("expected color=auto, got %s (exists: %v)", value, ok)
		}
	})

	t.Run("OSMConfigWithUnicodePath", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "配置ファイル")
		if err := os.WriteFile(configPath, []byte("test true"), 0600); err != nil {
			t.Skip("unicode path not supported on this system")
		}
		t.Setenv("OSM_CONFIG", configPath)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected no error with unicode path, got: %v", err)
		}

		if _, ok := cfg.GetGlobalOption("test"); !ok {
			t.Fatalf("expected test option to exist")
		}
	})

	t.Run("OSMConfigOverriddenByEnvironment", func(t *testing.T) {
		// Test that OSM_CONFIG environment variable takes precedence
		dir1 := t.TempDir()
		dir2 := t.TempDir()

		path1 := filepath.Join(dir1, "config1")
		path2 := filepath.Join(dir2, "config2")

		if err := os.WriteFile(path1, []byte("source first"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}
		if err := os.WriteFile(path2, []byte("source second"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		// Set OSM_CONFIG to path2
		t.Setenv("OSM_CONFIG", path2)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected load success, got: %v", err)
		}

		// Should load from path2, not path1
		if value, ok := cfg.GetGlobalOption("source"); !ok || value != "second" {
			t.Fatalf("expected source=second (from OSM_CONFIG), got %s (exists: %v)", value, ok)
		}
	})
}

func TestConfigFilePathResolutionEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("RelativePath", func(t *testing.T) {
		// Create a config file in current directory
		relPath := "./test-config"
		if err := os.WriteFile(relPath, []byte("test relative"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}
		defer os.Remove(relPath)

		cfg, err := LoadFromPath(relPath)
		if err != nil {
			t.Fatalf("expected load success with relative path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("test"); !ok || value != "relative" {
			t.Fatalf("expected test=relative, got %s (exists: %v)", value, ok)
		}
	})

	t.Run("PathWithParentDirectoryComponents", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "level1", "level2")
		if err := os.MkdirAll(nested, 0755); err != nil {
			t.Fatalf("failed to create nested directory: %v", err)
		}

		configPath := filepath.Join(nested, "config")
		if err := os.WriteFile(configPath, []byte("test nested"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		// Access config using a path that goes up and back down
		// This tests that the path resolution handles ".." components
		relPath := filepath.Join(nested, "..", "level2", "config")

		cfg, err := LoadFromPath(relPath)
		if err != nil {
			t.Fatalf("expected load success with .. in path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("test"); !ok || value != "nested" {
			t.Fatalf("expected test=nested, got %s (exists: %v)", value, ok)
		}
	})

	t.Run("PathWithSymlink", func(t *testing.T) {
		// This test verifies that a symlink in an INTERMEDIATE directory
		// component does NOT block config loading. Only the final path
		// component (the config file itself) is checked for symlinks.
		dir := t.TempDir()
		realDir := filepath.Join(dir, "real")
		linkDir := filepath.Join(dir, "link")

		if err := os.MkdirAll(realDir, 0755); err != nil {
			t.Fatalf("failed to create real directory: %v", err)
		}

		// Create symlink
		if err := os.Symlink(realDir, linkDir); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		configPath := filepath.Join(linkDir, "config")
		if err := os.WriteFile(configPath, []byte("test symlink"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		cfg, err := LoadFromPath(configPath)
		if err != nil {
			t.Fatalf("expected load success with symlink path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("test"); !ok || value != "symlink" {
			t.Fatalf("expected test=symlink, got %s (exists: %v)", value, ok)
		}
	})

	// T60: Direct symlink attack tests — verify LoadFromPath() rejects
	// config files that are themselves symlinks (not just intermediary dirs).
	t.Run("DirectSymlink_Rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create a real config file, then a symlink pointing to it.
		realFile := filepath.Join(dir, "real-config")
		if err := os.WriteFile(realFile, []byte("test secret"), 0600); err != nil {
			t.Fatalf("write real config: %v", err)
		}
		symlinkPath := filepath.Join(dir, "symlinked-config")
		if err := os.Symlink(realFile, symlinkPath); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		_, err := LoadFromPath(symlinkPath)
		if err == nil {
			t.Fatal("expected error for direct symlink config, got nil")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected error to mention 'symlink', got: %v", err)
		}
	})

	t.Run("DirectSymlink_ToSensitiveFile", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Symlink pointing to /etc/passwd (or any existing file).
		// LoadFromPath must reject before opening.
		target := "/etc/passwd"
		if _, err := os.Stat(target); err != nil {
			t.Skip("target file not available on this platform")
		}
		symlinkPath := filepath.Join(dir, "evil-config")
		if err := os.Symlink(target, symlinkPath); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		_, err := LoadFromPath(symlinkPath)
		if err == nil {
			t.Fatal("expected error for symlink to sensitive file, got nil")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected error to mention 'symlink', got: %v", err)
		}
	})

	t.Run("PathWithSpecialCharacters", func(t *testing.T) {
		dir := t.TempDir()
		// Note: Some characters may not be allowed in filenames on some platforms
		specialNames := []string{
			"config-with-dashes",
			"config_with_underscores",
			"config.with.dots",
			"config_with_numbers_123",
		}

		for _, name := range specialNames {
			configPath := filepath.Join(dir, name)
			if err := os.WriteFile(configPath, []byte("test "+name), 0600); err != nil {
				t.Logf("could not create file %s: %v", name, err)
				continue
			}

			cfg, err := LoadFromPath(configPath)
			if err != nil {
				t.Errorf("expected load success with %s, got: %v", name, err)
				continue
			}

			if value, ok := cfg.GetGlobalOption("test"); !ok || value != name {
				t.Errorf("expected test=%s, got %s (exists: %v)", name, value, ok)
			}
		}
	})

	t.Run("PathWithWhitespaceOnly", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, "   ")
		if err := os.WriteFile(configPath, []byte("test whitespace"), 0600); err != nil {
			t.Skip("whitespace-only filenames not supported")
		}

		cfg, err := LoadFromPath(configPath)
		if err != nil {
			t.Fatalf("expected load success with whitespace path, got: %v", err)
		}

		if _, ok := cfg.GetGlobalOption("test"); !ok {
			t.Fatalf("expected test option to exist")
		}
	})

	t.Run("AbsolutePathResolution", func(t *testing.T) {
		dir := t.TempDir()
		absPath := filepath.Join(dir, "absolute-config")
		if err := os.WriteFile(absPath, []byte("test absolute"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}

		cfg, err := LoadFromPath(absPath)
		if err != nil {
			t.Fatalf("expected load success with absolute path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("test"); !ok || value != "absolute" {
			t.Fatalf("expected test=absolute, got %s (exists: %v)", value, ok)
		}
	})

	t.Run("EmptyPath", func(t *testing.T) {
		// Empty path handling - behavior depends on os.Open behavior
		// On most systems, empty path returns an error or IsNotExist
		cfg, err := LoadFromPath("")
		if err != nil {
			// Expected on most systems
			t.Logf("got expected error for empty path: %v", err)
			return
		}
		// If no error, it returns empty config (file doesn't exist)
		if len(cfg.Global) != 0 || len(cfg.Commands) != 0 {
			t.Fatalf("expected empty config for empty path, got %+v", cfg)
		}
	})

	t.Run("CurrentDirectoryPath", func(t *testing.T) {
		// Using "." as path should work
		dotConfig := "./.config"
		if err := os.WriteFile(dotConfig, []byte("test dot"), 0600); err != nil {
			t.Fatalf("failed to create config file: %v", err)
		}
		defer os.Remove(dotConfig)

		cfg, err := LoadFromPath("./.config")
		if err != nil {
			t.Fatalf("expected load success with ./path, got: %v", err)
		}

		if value, ok := cfg.GetGlobalOption("test"); !ok || value != "dot" {
			t.Fatalf("expected test=dot, got %s (exists: %v)", value, ok)
		}
	})
}

func TestSessionConfigParsing_M2(t *testing.T) {
	t.Parallel()

	t.Run("ValidSessionConfig", func(t *testing.T) {
		configContent := `[sessions]
maxAgeDays 30
maxCount 50
maxSizeMB 200
autoCleanupEnabled false
cleanupIntervalHours 12`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Check session config values
		if cfg.Sessions.MaxAgeDays != 30 {
			t.Errorf("expected MaxAgeDays=30, got %d", cfg.Sessions.MaxAgeDays)
		}
		if cfg.Sessions.MaxCount != 50 {
			t.Errorf("expected MaxCount=50, got %d", cfg.Sessions.MaxCount)
		}
		if cfg.Sessions.MaxSizeMB != 200 {
			t.Errorf("expected MaxSizeMB=200, got %d", cfg.Sessions.MaxSizeMB)
		}
		if cfg.Sessions.AutoCleanupEnabled != false {
			t.Errorf("expected AutoCleanupEnabled=false, got %v", cfg.Sessions.AutoCleanupEnabled)
		}
		if cfg.Sessions.CleanupIntervalHours != 12 {
			t.Errorf("expected CleanupIntervalHours=12, got %d", cfg.Sessions.CleanupIntervalHours)
		}
	})

	t.Run("SessionConfigWithOtherSections", func(t *testing.T) {
		configContent := `verbose true

[sessions]
maxAgeDays 60

[help]
pager less`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Check global option
		if value, ok := cfg.GetGlobalOption("verbose"); !ok || value != "true" {
			t.Errorf("expected verbose=true, got %s", value)
		}

		// Check session config
		if cfg.Sessions.MaxAgeDays != 60 {
			t.Errorf("expected MaxAgeDays=60, got %d", cfg.Sessions.MaxAgeDays)
		}

		// Check command option
		if value, ok := cfg.GetCommandOption("help", "pager"); !ok || value != "less" {
			t.Errorf("expected help.pager=less, got %s", value)
		}
	})

	t.Run("InvalidIntegerValues", func(t *testing.T) {
		invalidValues := []struct {
			name  string
			input string
		}{
			{"invalidMaxAgeDays", "maxAgeDays notanumber"},
			{"invalidMaxCount", "maxCount abc"},
			{"invalidMaxSizeMB", "maxSizeMB 12.5.3"},
			{"invalidCleanupInterval", "cleanupIntervalHours xyz"},
		}

		for _, tc := range invalidValues {
			t.Run(tc.name, func(t *testing.T) {
				configContent := "[sessions]\n" + tc.input
				_, err := LoadFromReader(strings.NewReader(configContent))
				if err == nil {
					t.Errorf("expected error for invalid value %q", tc.input)
				}
			})
		}
	})

	t.Run("InvalidBooleanValues", func(t *testing.T) {
		configContent := "[sessions]\nautoCleanupEnabled maybe"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Error("expected error for invalid boolean value")
		}
	})

	t.Run("NegativeIntegerValues", func(t *testing.T) {
		negativeValues := []struct {
			name  string
			input string
		}{
			{"negativeMaxAgeDays", "maxAgeDays -1"},
			{"negativeMaxCount", "maxCount -10"},
			{"negativeMaxSizeMB", "maxSizeMB -100"},
		}

		for _, tc := range negativeValues {
			t.Run(tc.name, func(t *testing.T) {
				configContent := "[sessions]\n" + tc.input
				_, err := LoadFromReader(strings.NewReader(configContent))
				if err == nil {
					t.Errorf("expected error for negative value %q", tc.input)
				}
			})
		}
	})

	t.Run("ZeroCleanupInterval", func(t *testing.T) {
		configContent := "[sessions]\ncleanupIntervalHours 0"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Error("expected error for cleanupIntervalHours=0")
		}
	})

	t.Run("UnknownSessionOption", func(t *testing.T) {
		configContent := "[sessions]\nunknownOption value"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Error("expected error for unknown session option")
		}
	})

	t.Run("BooleanTrueVariations", func(t *testing.T) {
		trueValues := []string{"true", "1", "yes", "on", "TRUE", "Yes", "ON"}

		for _, val := range trueValues {
			configContent := "[sessions]\nautoCleanupEnabled " + val
			cfg, err := LoadFromReader(strings.NewReader(configContent))
			if err != nil {
				t.Errorf("expected no error for %q, got: %v", val, err)
				continue
			}
			if !cfg.Sessions.AutoCleanupEnabled {
				t.Errorf("expected AutoCleanupEnabled=true for value %q", val)
			}
		}
	})

	t.Run("BooleanFalseVariations", func(t *testing.T) {
		falseValues := []string{"false", "0", "no", "off", "FALSE", "No", "OFF"}

		for _, val := range falseValues {
			configContent := "[sessions]\nautoCleanupEnabled " + val
			cfg, err := LoadFromReader(strings.NewReader(configContent))
			if err != nil {
				t.Errorf("expected no error for %q, got: %v", val, err)
				continue
			}
			if cfg.Sessions.AutoCleanupEnabled {
				t.Errorf("expected AutoCleanupEnabled=false for value %q", val)
			}
		}
	})

	t.Run("EmptySessionsSection", func(t *testing.T) {
		configContent := "[sessions]\n\n[global]\nverbose true"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		// Should have default values
		if cfg.Sessions.MaxAgeDays != 90 {
			t.Errorf("expected default MaxAgeDays=90, got %d", cfg.Sessions.MaxAgeDays)
		}
	})

	t.Run("PartialSessionConfig", func(t *testing.T) {
		configContent := `[sessions]
maxAgeDays 45
autoCleanupEnabled true`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Check specified values
		if cfg.Sessions.MaxAgeDays != 45 {
			t.Errorf("expected MaxAgeDays=45, got %d", cfg.Sessions.MaxAgeDays)
		}
		if !cfg.Sessions.AutoCleanupEnabled {
			t.Errorf("expected AutoCleanupEnabled=true, got %v", cfg.Sessions.AutoCleanupEnabled)
		}

		// Check default values for unspecified options
		if cfg.Sessions.MaxCount != 100 {
			t.Errorf("expected default MaxCount=100, got %d", cfg.Sessions.MaxCount)
		}
		if cfg.Sessions.MaxSizeMB != 500 {
			t.Errorf("expected default MaxSizeMB=500, got %d", cfg.Sessions.MaxSizeMB)
		}
	})

	t.Run("DefaultValuesWhenNoSessionsSection", func(t *testing.T) {
		configContent := "verbose true"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Should have all default values
		if cfg.Sessions.MaxAgeDays != 90 {
			t.Errorf("expected default MaxAgeDays=90, got %d", cfg.Sessions.MaxAgeDays)
		}
		if cfg.Sessions.MaxCount != 100 {
			t.Errorf("expected default MaxCount=100, got %d", cfg.Sessions.MaxCount)
		}
		if cfg.Sessions.MaxSizeMB != 500 {
			t.Errorf("expected default MaxSizeMB=500, got %d", cfg.Sessions.MaxSizeMB)
		}
		if !cfg.Sessions.AutoCleanupEnabled {
			t.Errorf("expected default AutoCleanupEnabled=true, got %v", cfg.Sessions.AutoCleanupEnabled)
		}
		if cfg.Sessions.CleanupIntervalHours != 24 {
			t.Errorf("expected default CleanupIntervalHours=24, got %d", cfg.Sessions.CleanupIntervalHours)
		}
	})
}

func TestConfigSchemaValidation_M1(t *testing.T) {
	t.Parallel()

	t.Run("UnknownGlobalOption", func(t *testing.T) {
		configContent := "verbos true" // typo of "verbose"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if !cfg.HasWarnings() {
			t.Error("expected warnings for unknown global option")
		}

		warnings := cfg.GetWarnings()
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "unknown global option") && strings.Contains(w, "verbos") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about 'verbos', got: %v", warnings)
		}
	})

	t.Run("UnknownCommandOption", func(t *testing.T) {
		configContent := `[help]
pagr less` // typo of "pager"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if !cfg.HasWarnings() {
			t.Error("expected warnings for unknown command option")
		}

		warnings := cfg.GetWarnings()
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "unknown option") && strings.Contains(w, "pagr") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about 'pagr', got: %v", warnings)
		}
	})

	t.Run("KnownGlobalOption", func(t *testing.T) {
		configContent := "verbose true"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if cfg.HasWarnings() {
			t.Errorf("expected no warnings for known option, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("KnownCommandOption", func(t *testing.T) {
		configContent := `[help]
pager less`
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if cfg.HasWarnings() {
			t.Errorf("expected no warnings for known option, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("GlobalOptionInCommandSection", func(t *testing.T) {
		// Global options should be valid in command sections too
		configContent := `[help]
verbose true`
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if cfg.HasWarnings() {
			t.Errorf("expected no warnings when using global option in command section, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("MultipleWarnings", func(t *testing.T) {
		configContent := `verbos true
colr auto

[help]
pagr less
formt detailed`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		warnings := cfg.GetWarnings()
		if len(warnings) < 4 {
			t.Errorf("expected at least 4 warnings, got %d: %v", len(warnings), warnings)
		}

		// Check for specific typos
		warningText := strings.Join(warnings, " ")
		if !strings.Contains(warningText, "verbos") {
			t.Error("expected warning about 'verbos'")
		}
		if !strings.Contains(warningText, "colr") {
			t.Error("expected warning about 'colr'")
		}
		if !strings.Contains(warningText, "pagr") {
			t.Error("expected warning about 'pagr'")
		}
		if !strings.Contains(warningText, "formt") {
			t.Error("expected warning about 'formt'")
		}
	})

	t.Run("UnknownCommandSection", func(t *testing.T) {
		// Options in unknown command sections should still generate warnings
		configContent := `[unknowncmd]
weirdoption value`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if !cfg.HasWarnings() {
			t.Error("expected warnings for options in unknown command section")
		}

		warnings := cfg.GetWarnings()
		found := false
		for _, w := range warnings {
			if strings.Contains(w, "weirdoption") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about 'weirdoption', got: %v", warnings)
		}
	})

	t.Run("NoWarningsForEmptyConfig", func(t *testing.T) {
		cfg, err := LoadFromReader(strings.NewReader(""))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if cfg.HasWarnings() {
			t.Errorf("expected no warnings for empty config, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("WarningsPreservedAcrossNewConfig", func(t *testing.T) {
		// Each NewConfig should start with empty warnings
		cfg1 := NewConfig()
		if cfg1.HasWarnings() {
			t.Error("new config should not have warnings")
		}

		// Loading config with unknown option should add warnings
		configContent := "unknownoption value"
		cfg2, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if !cfg2.HasWarnings() {
			t.Error("expected warnings after loading config with unknown options")
		}
	})
}

// TestLoadFromReader_TypeValidation verifies that LoadFromReader produces type
// mismatch warnings (not just unknown-option warnings) via the post-parse
// ValidateConfig integration.
func TestLoadFromReader_TypeValidation(t *testing.T) {
	t.Parallel()

	t.Run("InvalidBool", func(t *testing.T) {
		cfg, err := LoadFromReader(strings.NewReader("verbose notabool"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.HasWarnings() {
			t.Fatal("expected type warning for invalid bool")
		}
		found := false
		for _, w := range cfg.GetWarnings() {
			if strings.Contains(w, "expected bool") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'expected bool' warning, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("InvalidInt", func(t *testing.T) {
		cfg, err := LoadFromReader(strings.NewReader("script.max-traversal-depth abc"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.HasWarnings() {
			t.Fatal("expected type warning for invalid int")
		}
		found := false
		for _, w := range cfg.GetWarnings() {
			if strings.Contains(w, "expected int") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'expected int' warning, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("InvalidDuration", func(t *testing.T) {
		cfg, err := LoadFromReader(strings.NewReader("timeout notaduration"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.HasWarnings() {
			t.Fatal("expected type warning for invalid duration")
		}
		found := false
		for _, w := range cfg.GetWarnings() {
			if strings.Contains(w, "expected duration") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected 'expected duration' warning, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("ValidTypes", func(t *testing.T) {
		cfg, err := LoadFromReader(strings.NewReader("verbose true\nscript.max-traversal-depth 5\ntimeout 30s"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HasWarnings() {
			t.Errorf("expected no warnings for valid types, got: %v", cfg.GetWarnings())
		}
	})

	t.Run("MixedUnknownAndTypeMismatch", func(t *testing.T) {
		cfg, err := LoadFromReader(strings.NewReader("unknownkey hello\nverbose maybe"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		warnings := cfg.GetWarnings()
		if len(warnings) < 2 {
			t.Fatalf("expected at least 2 warnings (unknown + type), got %d: %v", len(warnings), warnings)
		}
		warningText := strings.Join(warnings, " ")
		if !strings.Contains(warningText, "unknown") {
			t.Error("expected unknown option warning")
		}
		if !strings.Contains(warningText, "expected bool") {
			t.Error("expected type mismatch warning")
		}
	})
}

func TestHotSnippetConfigParsing(t *testing.T) {
	t.Run("BasicSnippet", func(t *testing.T) {
		input := "[hot-snippets]\nfollowup Continue with the same context."
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 1 {
			t.Fatalf("expected 1 snippet, got %d", len(cfg.HotSnippets))
		}
		s := cfg.HotSnippets[0]
		if s.Name != "followup" {
			t.Errorf("name = %q, want %q", s.Name, "followup")
		}
		if s.Text != "Continue with the same context." {
			t.Errorf("text = %q, want %q", s.Text, "Continue with the same context.")
		}
		if s.Description != "" {
			t.Errorf("description = %q, want empty", s.Description)
		}
	})

	t.Run("SnippetWithDescription", func(t *testing.T) {
		input := "[hot-snippets]\nfollowup Continue with the same context.\nfollowup.description Follow-up prompt"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 1 {
			t.Fatalf("expected 1 snippet, got %d", len(cfg.HotSnippets))
		}
		s := cfg.HotSnippets[0]
		if s.Name != "followup" {
			t.Errorf("name = %q, want %q", s.Name, "followup")
		}
		if s.Text != "Continue with the same context." {
			t.Errorf("text = %q, want %q", s.Text, "Continue with the same context.")
		}
		if s.Description != "Follow-up prompt" {
			t.Errorf("description = %q, want %q", s.Description, "Follow-up prompt")
		}
	})

	t.Run("MultipleSnippets", func(t *testing.T) {
		input := "[hot-snippets]\nfollowup Continue with the same context.\nkickoff You are an expert software engineer.\nkickoff.description Kickoff prompt"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 2 {
			t.Fatalf("expected 2 snippets, got %d", len(cfg.HotSnippets))
		}
		if cfg.HotSnippets[0].Name != "followup" {
			t.Errorf("snippet 0 name = %q, want %q", cfg.HotSnippets[0].Name, "followup")
		}
		if cfg.HotSnippets[1].Name != "kickoff" {
			t.Errorf("snippet 1 name = %q, want %q", cfg.HotSnippets[1].Name, "kickoff")
		}
		if cfg.HotSnippets[1].Description != "Kickoff prompt" {
			t.Errorf("snippet 1 description = %q, want %q", cfg.HotSnippets[1].Description, "Kickoff prompt")
		}
	})

	t.Run("EscapedNewlines", func(t *testing.T) {
		input := "[hot-snippets]\nmultiline First line\\nSecond line\\nThird line"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 1 {
			t.Fatalf("expected 1 snippet, got %d", len(cfg.HotSnippets))
		}
		want := "First line\nSecond line\nThird line"
		if cfg.HotSnippets[0].Text != want {
			t.Errorf("text = %q, want %q", cfg.HotSnippets[0].Text, want)
		}
	})

	t.Run("EmptySection", func(t *testing.T) {
		input := "[hot-snippets]\n\n[prompt-flow]\ntemplate default"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 0 {
			t.Errorf("expected 0 snippets, got %d", len(cfg.HotSnippets))
		}
	})

	t.Run("DescriptionWithoutSnippet", func(t *testing.T) {
		input := "[hot-snippets]\nnonexistent.description This should fail"
		_, err := LoadFromReader(strings.NewReader(input))
		if err == nil {
			t.Fatal("expected error for description targeting nonexistent snippet")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %q, want to contain 'not found'", err.Error())
		}
	})

	t.Run("SnippetNameOnly", func(t *testing.T) {
		// A snippet with a name but no text
		input := "[hot-snippets]\nemptytext"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 1 {
			t.Fatalf("expected 1 snippet, got %d", len(cfg.HotSnippets))
		}
		if cfg.HotSnippets[0].Text != "" {
			t.Errorf("text = %q, want empty", cfg.HotSnippets[0].Text)
		}
	})

	t.Run("MixedWithOtherSections", func(t *testing.T) {
		input := "verbose true\n\n[hot-snippets]\nsnip1 hello world\n\n[sessions]\nmaxAgeDays 30\n\n[prompt-flow]\ntemplate default"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 1 {
			t.Fatalf("expected 1 snippet, got %d", len(cfg.HotSnippets))
		}
		if cfg.HotSnippets[0].Name != "snip1" {
			t.Errorf("name = %q, want %q", cfg.HotSnippets[0].Name, "snip1")
		}
		if cfg.HotSnippets[0].Text != "hello world" {
			t.Errorf("text = %q, want %q", cfg.HotSnippets[0].Text, "hello world")
		}
		// Verify other sections still work
		if cfg.Sessions.MaxAgeDays != 30 {
			t.Errorf("maxAgeDays = %d, want 30", cfg.Sessions.MaxAgeDays)
		}
		if cfg.GetString("verbose") != "true" {
			t.Errorf("verbose = %q, want %q", cfg.GetString("verbose"), "true")
		}
	})

	t.Run("SnippetsNotInCommands", func(t *testing.T) {
		// Verify [hot-snippets] section is NOT stored in Commands map
		input := "[hot-snippets]\nsnip1 text"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, exists := cfg.Commands["hot-snippets"]; exists {
			t.Error("hot-snippets should not appear in Commands map")
		}
	})

	t.Run("NewConfigInitializesHotSnippets", func(t *testing.T) {
		cfg := NewConfig()
		if cfg.HotSnippets == nil {
			t.Error("HotSnippets should be initialized, not nil")
		}
		if len(cfg.HotSnippets) != 0 {
			t.Errorf("expected empty HotSnippets, got %d", len(cfg.HotSnippets))
		}
	})

	t.Run("DuplicateSnippetNames", func(t *testing.T) {
		// Duplicate names are allowed — both are added (contextManager handles dedup if needed)
		input := "[hot-snippets]\ndup First text\ndup Second text"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 2 {
			t.Fatalf("expected 2 snippets, got %d", len(cfg.HotSnippets))
		}
		if cfg.HotSnippets[0].Text != "First text" {
			t.Errorf("snippet 0 text = %q, want %q", cfg.HotSnippets[0].Text, "First text")
		}
		if cfg.HotSnippets[1].Text != "Second text" {
			t.Errorf("snippet 1 text = %q, want %q", cfg.HotSnippets[1].Text, "Second text")
		}
	})

	t.Run("DescriptionAppliesToLastMatch", func(t *testing.T) {
		// When there are duplicates, .description applies to the last one with that name
		input := "[hot-snippets]\ndup First\ndup Second\ndup.description Applies to second"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.HotSnippets[0].Description != "" {
			t.Errorf("first dup should have no description, got %q", cfg.HotSnippets[0].Description)
		}
		if cfg.HotSnippets[1].Description != "Applies to second" {
			t.Errorf("second dup description = %q, want %q", cfg.HotSnippets[1].Description, "Applies to second")
		}
	})

	t.Run("CommentsInHotSnippetsSection", func(t *testing.T) {
		input := "[hot-snippets]\n# This is a comment\nsnip1 text\n# Another comment"
		cfg, err := LoadFromReader(strings.NewReader(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.HotSnippets) != 1 {
			t.Fatalf("expected 1 snippet, got %d", len(cfg.HotSnippets))
		}
	})
}

func TestClaudeMuxConfigParsing(t *testing.T) {
	t.Parallel()

	t.Run("ValidClaudeMuxConfig", func(t *testing.T) {
		t.Parallel()
		configContent := `[claude-mux]
provider claude-code
model claude-sonnet-4-20250514
work-dir /tmp/agents
env-inherit false
env API_KEY=secret123
env REGION=us-west-2
env-profile production
pre-spawn-hook /path/to/hook.js
permission-policy ask
rate-limit-backoff-sec 60
max-agents 8
pty-rows 40
pty-cols 120
provider-command /usr/local/bin/claude
mcp-servers mcp-server-github,mcp-server-filesystem`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		oc := cfg.ClaudeMux
		if oc.Provider != "claude-code" {
			t.Errorf("Provider = %q, want %q", oc.Provider, "claude-code")
		}
		if oc.Model != "claude-sonnet-4-20250514" {
			t.Errorf("Model = %q, want %q", oc.Model, "claude-sonnet-4-20250514")
		}
		if oc.WorkDir != "/tmp/agents" {
			t.Errorf("WorkDir = %q, want %q", oc.WorkDir, "/tmp/agents")
		}
		if oc.EnvInherit != false {
			t.Errorf("EnvInherit = %v, want false", oc.EnvInherit)
		}
		if len(oc.EnvVars) != 2 {
			t.Fatalf("EnvVars length = %d, want 2", len(oc.EnvVars))
		}
		if oc.EnvVars["API_KEY"] != "secret123" {
			t.Errorf("EnvVars[API_KEY] = %q, want %q", oc.EnvVars["API_KEY"], "secret123")
		}
		if oc.EnvVars["REGION"] != "us-west-2" {
			t.Errorf("EnvVars[REGION] = %q, want %q", oc.EnvVars["REGION"], "us-west-2")
		}
		if oc.EnvProfile != "production" {
			t.Errorf("EnvProfile = %q, want %q", oc.EnvProfile, "production")
		}
		if oc.PreSpawnHook != "/path/to/hook.js" {
			t.Errorf("PreSpawnHook = %q, want %q", oc.PreSpawnHook, "/path/to/hook.js")
		}
		if oc.PermissionPolicy != "ask" {
			t.Errorf("PermissionPolicy = %q, want %q", oc.PermissionPolicy, "ask")
		}
		if oc.RateLimitBackoffSec != 60 {
			t.Errorf("RateLimitBackoffSec = %d, want 60", oc.RateLimitBackoffSec)
		}
		if oc.MaxAgents != 8 {
			t.Errorf("MaxAgents = %d, want 8", oc.MaxAgents)
		}
		if oc.PTYRows != 40 {
			t.Errorf("PTYRows = %d, want 40", oc.PTYRows)
		}
		if oc.PTYCols != 120 {
			t.Errorf("PTYCols = %d, want 120", oc.PTYCols)
		}
		if oc.ProviderCommand != "/usr/local/bin/claude" {
			t.Errorf("ProviderCommand = %q, want %q", oc.ProviderCommand, "/usr/local/bin/claude")
		}
		if oc.MCPServers != "mcp-server-github,mcp-server-filesystem" {
			t.Errorf("MCPServers = %q, want %q", oc.MCPServers, "mcp-server-github,mcp-server-filesystem")
		}
	})

	t.Run("ClaudeMuxDefaults", func(t *testing.T) {
		t.Parallel()
		cfg := NewConfig()
		oc := cfg.ClaudeMux

		if oc.Provider != "claude-code" {
			t.Errorf("default Provider = %q, want %q", oc.Provider, "claude-code")
		}
		if oc.Model != "" {
			t.Errorf("default Model = %q, want empty", oc.Model)
		}
		if oc.WorkDir != "" {
			t.Errorf("default WorkDir = %q, want empty", oc.WorkDir)
		}
		if oc.EnvInherit != true {
			t.Errorf("default EnvInherit = %v, want true", oc.EnvInherit)
		}
		if oc.EnvVars == nil {
			t.Error("default EnvVars should not be nil")
		}
		if len(oc.EnvVars) != 0 {
			t.Errorf("default EnvVars should be empty, got %d entries", len(oc.EnvVars))
		}
		if oc.EnvProfile != "" {
			t.Errorf("default EnvProfile = %q, want empty", oc.EnvProfile)
		}
		if oc.PreSpawnHook != "" {
			t.Errorf("default PreSpawnHook = %q, want empty", oc.PreSpawnHook)
		}
		if oc.PermissionPolicy != "reject" {
			t.Errorf("default PermissionPolicy = %q, want %q", oc.PermissionPolicy, "reject")
		}
		if oc.RateLimitBackoffSec != 30 {
			t.Errorf("default RateLimitBackoffSec = %d, want 30", oc.RateLimitBackoffSec)
		}
		if oc.MaxAgents != 4 {
			t.Errorf("default MaxAgents = %d, want 4", oc.MaxAgents)
		}
		if oc.PTYRows != 24 {
			t.Errorf("default PTYRows = %d, want 24", oc.PTYRows)
		}
		if oc.PTYCols != 80 {
			t.Errorf("default PTYCols = %d, want 80", oc.PTYCols)
		}
		if oc.ProviderCommand != "" {
			t.Errorf("default ProviderCommand = %q, want empty", oc.ProviderCommand)
		}
		if oc.MCPServers != "" {
			t.Errorf("default MCPServers = %q, want empty", oc.MCPServers)
		}
	})

	t.Run("ClaudeMuxEnvVars", func(t *testing.T) {
		t.Parallel()
		configContent := `[claude-mux]
env FOO=bar
env BAZ=qux=quux
env EMPTY=`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		oc := cfg.ClaudeMux
		if len(oc.EnvVars) != 3 {
			t.Fatalf("EnvVars length = %d, want 3", len(oc.EnvVars))
		}
		if oc.EnvVars["FOO"] != "bar" {
			t.Errorf("EnvVars[FOO] = %q, want %q", oc.EnvVars["FOO"], "bar")
		}
		// Second = is part of the value
		if oc.EnvVars["BAZ"] != "qux=quux" {
			t.Errorf("EnvVars[BAZ] = %q, want %q", oc.EnvVars["BAZ"], "qux=quux")
		}
		// Empty value after =
		if oc.EnvVars["EMPTY"] != "" {
			t.Errorf("EnvVars[EMPTY] = %q, want empty", oc.EnvVars["EMPTY"])
		}
	})

	t.Run("ClaudeMuxEnvVarInvalid", func(t *testing.T) {
		t.Parallel()
		configContent := "[claude-mux]\nenv NOEQUALSSIGN"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Fatal("expected error for env without = sign")
		}
		if !strings.Contains(err.Error(), "KEY=VALUE") {
			t.Errorf("error = %q, want to contain 'KEY=VALUE'", err.Error())
		}
	})

	t.Run("ClaudeMuxInvalidPermissionPolicy", func(t *testing.T) {
		t.Parallel()
		configContent := "[claude-mux]\npermission-policy allow"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Fatal("expected error for invalid permission-policy")
		}
		if !strings.Contains(err.Error(), "reject") || !strings.Contains(err.Error(), "ask") {
			t.Errorf("error = %q, want to mention 'reject' and 'ask'", err.Error())
		}
	})

	t.Run("ClaudeMuxInvalidIntOptions", func(t *testing.T) {
		t.Parallel()
		intOptions := []struct {
			name  string
			input string
		}{
			{"invalidRateLimitBackoff", "rate-limit-backoff-sec abc"},
			{"invalidMaxAgents", "max-agents notanumber"},
			{"invalidPtyRows", "pty-rows 12.5"},
			{"invalidPtyCols", "pty-cols xyz"},
		}

		for _, tc := range intOptions {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				configContent := "[claude-mux]\n" + tc.input
				_, err := LoadFromReader(strings.NewReader(configContent))
				if err == nil {
					t.Errorf("expected error for invalid value %q", tc.input)
				}
			})
		}
	})

	t.Run("ClaudeMuxPartialConfig", func(t *testing.T) {
		t.Parallel()
		configContent := `[claude-mux]
model gpt-4o
max-agents 2`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		oc := cfg.ClaudeMux
		// Specified values
		if oc.Model != "gpt-4o" {
			t.Errorf("Model = %q, want %q", oc.Model, "gpt-4o")
		}
		if oc.MaxAgents != 2 {
			t.Errorf("MaxAgents = %d, want 2", oc.MaxAgents)
		}

		// Default values for unspecified options
		if oc.Provider != "claude-code" {
			t.Errorf("default Provider = %q, want %q", oc.Provider, "claude-code")
		}
		if oc.EnvInherit != true {
			t.Errorf("default EnvInherit = %v, want true", oc.EnvInherit)
		}
		if oc.PermissionPolicy != "reject" {
			t.Errorf("default PermissionPolicy = %q, want %q", oc.PermissionPolicy, "reject")
		}
		if oc.RateLimitBackoffSec != 30 {
			t.Errorf("default RateLimitBackoffSec = %d, want 30", oc.RateLimitBackoffSec)
		}
		if oc.PTYRows != 24 {
			t.Errorf("default PTYRows = %d, want 24", oc.PTYRows)
		}
		if oc.PTYCols != 80 {
			t.Errorf("default PTYCols = %d, want 80", oc.PTYCols)
		}
	})

	t.Run("ClaudeMuxWithOtherSections", func(t *testing.T) {
		t.Parallel()
		configContent := `verbose true

[claude-mux]
provider openai
max-agents 6

[sessions]
maxAgeDays 45

[help]
pager less`

		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		// Check claude-mux
		if cfg.ClaudeMux.Provider != "openai" {
			t.Errorf("ClaudeMux.Provider = %q, want %q", cfg.ClaudeMux.Provider, "openai")
		}
		if cfg.ClaudeMux.MaxAgents != 6 {
			t.Errorf("ClaudeMux.MaxAgents = %d, want 6", cfg.ClaudeMux.MaxAgents)
		}

		// Check other sections still work
		if cfg.Sessions.MaxAgeDays != 45 {
			t.Errorf("Sessions.MaxAgeDays = %d, want 45", cfg.Sessions.MaxAgeDays)
		}
		if cfg.GetString("verbose") != "true" {
			t.Errorf("verbose = %q, want %q", cfg.GetString("verbose"), "true")
		}
		if v, ok := cfg.GetCommandOption("help", "pager"); !ok || v != "less" {
			t.Errorf("help.pager = %q, want %q", v, "less")
		}
	})

	t.Run("ClaudeMuxNotInCommands", func(t *testing.T) {
		t.Parallel()
		configContent := "[claude-mux]\nprovider test-provider"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if _, exists := cfg.Commands["claude-mux"]; exists {
			t.Error("claude-mux should not appear in Commands map")
		}
	})

	t.Run("ClaudeMuxUnknownOption", func(t *testing.T) {
		t.Parallel()
		configContent := "[claude-mux]\nunknown-option value"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Fatal("expected error for unknown claude-mux option")
		}
		if !strings.Contains(err.Error(), "unknown claude-mux option") {
			t.Errorf("error = %q, want to contain 'unknown claude-mux option'", err.Error())
		}
	})

	t.Run("ClaudeMuxPermissionPolicyReject", func(t *testing.T) {
		t.Parallel()
		configContent := "[claude-mux]\npermission-policy reject"
		cfg, err := LoadFromReader(strings.NewReader(configContent))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if cfg.ClaudeMux.PermissionPolicy != "reject" {
			t.Errorf("PermissionPolicy = %q, want %q", cfg.ClaudeMux.PermissionPolicy, "reject")
		}
	})

	t.Run("ClaudeMuxBooleanVariations", func(t *testing.T) {
		t.Parallel()
		trueValues := []string{"true", "1", "yes", "on", "TRUE", "Yes", "ON"}
		for _, val := range trueValues {
			configContent := "[claude-mux]\nenv-inherit " + val
			cfg, err := LoadFromReader(strings.NewReader(configContent))
			if err != nil {
				t.Errorf("expected no error for %q, got: %v", val, err)
				continue
			}
			if !cfg.ClaudeMux.EnvInherit {
				t.Errorf("expected EnvInherit=true for value %q", val)
			}
		}

		falseValues := []string{"false", "0", "no", "off", "FALSE", "No", "OFF"}
		for _, val := range falseValues {
			configContent := "[claude-mux]\nenv-inherit " + val
			cfg, err := LoadFromReader(strings.NewReader(configContent))
			if err != nil {
				t.Errorf("expected no error for %q, got: %v", val, err)
				continue
			}
			if cfg.ClaudeMux.EnvInherit {
				t.Errorf("expected EnvInherit=false for value %q", val)
			}
		}
	})

	t.Run("ClaudeMuxInvalidBooleanValue", func(t *testing.T) {
		t.Parallel()
		configContent := "[claude-mux]\nenv-inherit maybe"
		_, err := LoadFromReader(strings.NewReader(configContent))
		if err == nil {
			t.Fatal("expected error for invalid boolean value")
		}
	})
}
