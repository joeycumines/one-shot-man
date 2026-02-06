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
