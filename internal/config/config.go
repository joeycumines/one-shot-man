package config

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config represents the application configuration.
type Config struct {
	// Global options that apply to all commands
	Global map[string]string
	// Command-specific options
	Commands map[string]map[string]string
	// Sessions configuration controls automatic session cleanup and retention.
	Sessions SessionConfig
	// Warnings contains any warnings generated during config loading
	Warnings []string
}

// NewConfig creates a new empty configuration.
func NewConfig() *Config {
	return &Config{
		Global:   make(map[string]string),
		Commands: make(map[string]map[string]string),
		Sessions: SessionConfig{
			MaxAgeDays:           90,
			MaxCount:             100,
			MaxSizeMB:            500,
			AutoCleanupEnabled:   true,
			CleanupIntervalHours: 24,
		},
		Warnings: make([]string, 0),
	}
}

// SessionConfig controls session lifecycle and cleanup behavior.
type SessionConfig struct {
	MaxAgeDays int `json:"maxAgeDays" default:"90"`
	MaxCount   int `json:"maxCount" default:"100"`
	MaxSizeMB  int `json:"maxSizeMb" default:"500"`
	// TODO: AutoCleanupEnabled is parsed and validated but no automatic cleanup
	// scheduler exists yet. Reserved for future auto-cleanup scheduler implementation.
	AutoCleanupEnabled bool `json:"autoCleanupEnabled" default:"true"`
	// TODO: CleanupIntervalHours is parsed and validated but no automatic cleanup
	// scheduler exists yet. Reserved for future auto-cleanup scheduler implementation.
	CleanupIntervalHours int `json:"cleanupIntervalHours" default:"24"`
}

// Load loads configuration from the default config file path.
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	return LoadFromPath(configPath)
}

// LoadFromPath loads configuration from the specified file path.
// The file uses dnsmasq-style format: optionName remainingLineIsTheValue
//
// SECURITY: This function rejects symlinks to prevent symlink attacks
// that could read sensitive files through symlink traversal.
func LoadFromPath(path string) (*Config, error) {
	// Security: Lstat checks the final path component for symlinks.
	// This prevents symlink-to-file attacks (e.g., config -> /etc/passwd).
	// Intermediate directory symlinks are NOT checked, by design:
	// the threat model targets direct file symlink substitution.
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return NewConfig(), nil
		}
		return nil, fmt.Errorf("failed to stat config file: %w", err)
	}

	// Reject symlinks to prevent reading sensitive files through symlink attacks
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlink not allowed in config path: %s", path)
	}

	// Open the file (symlinks already rejected by Lstat check above)
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	return LoadFromReader(file)
}

// LoadFromReader loads configuration from an io.Reader.
func LoadFromReader(r io.Reader) (*Config, error) {
	config := NewConfig()
	scanner := bufio.NewScanner(r)

	var currentCommand string
	var inSessionsSection bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header [section_name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			if sectionName == "sessions" {
				inSessionsSection = true
				currentCommand = ""
			} else {
				inSessionsSection = false
				currentCommand = sectionName
				if config.Commands[currentCommand] == nil {
					config.Commands[currentCommand] = make(map[string]string)
				}
			}
			continue
		}

		// Parse option line: optionName remainingLineIsTheValue
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 1 {
			continue
		}

		optionName := parts[0]
		var value string
		if len(parts) > 1 {
			value = parts[1]
		}

		// Store in appropriate section
		if inSessionsSection {
			// Session configuration option
			if err := parseSessionOption(&config.Sessions, optionName, value); err != nil {
				return nil, fmt.Errorf("invalid session option %q: %w", optionName, err)
			}
		} else if currentCommand == "" {
			// Global option
			config.Global[optionName] = value
		} else {
			// Command-specific option
			config.Commands[currentCommand][optionName] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config: %w", err)
	}

	// Validate config against schema: detect unknown options and type mismatches.
	for _, issue := range ValidateConfig(config, DefaultSchema()) {
		config.addWarning("%s", issue)
	}

	return config, nil
}

// addWarning adds a warning to the config's warnings list.
func (c *Config) addWarning(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	c.Warnings = append(c.Warnings, msg)
	slog.Warn("[Config] " + msg)
}

// parseSessionOption parses a session configuration option and updates the SessionConfig.
// Supported options:
//   - maxAgeDays <int>: Maximum age of sessions in days (default: 90)
//   - maxCount <int>: Maximum number of sessions to keep (default: 100)
//   - maxSizeMB <int>: Maximum total size of sessions in MB (default: 500)
//   - autoCleanupEnabled <bool>: Whether automatic cleanup is enabled (default: true)
//   - cleanupIntervalHours <int>: Hours between cleanup runs (default: 24)
func parseSessionOption(sc *SessionConfig, name, value string) error {
	switch name {
	case "maxAgeDays":
		age, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		if age < 0 {
			return fmt.Errorf("maxAgeDays cannot be negative: %d", age)
		}
		sc.MaxAgeDays = age

	case "maxCount":
		count, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		if count < 0 {
			return fmt.Errorf("maxCount cannot be negative: %d", count)
		}
		sc.MaxCount = count

	case "maxSizeMB":
		size, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		if size < 0 {
			return fmt.Errorf("maxSizeMB cannot be negative: %d", size)
		}
		sc.MaxSizeMB = size

	case "autoCleanupEnabled":
		enabled, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value %q: %w", value, err)
		}
		sc.AutoCleanupEnabled = enabled

	case "cleanupIntervalHours":
		interval, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		if interval < 1 {
			return fmt.Errorf("cleanupIntervalHours must be at least 1: %d", interval)
		}
		sc.CleanupIntervalHours = interval

	default:
		return fmt.Errorf("unknown session option: %s", name)
	}
	return nil
}

// parseBool parses a boolean value from string.
// Accepts: true, false, 1, 0, yes, no (case-insensitive)
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", s)
	}
}

// GetGlobalOption returns a global configuration option.
func (c *Config) GetGlobalOption(name string) (string, bool) {
	value, exists := c.Global[name]
	return value, exists
}

// GetCommandOption returns a command-specific configuration option.
// It first checks command-specific options, then falls back to global options.
func (c *Config) GetCommandOption(command, name string) (string, bool) {
	if cmdOptions, exists := c.Commands[command]; exists {
		if value, exists := cmdOptions[name]; exists {
			return value, true
		}
	}

	// Fall back to global options
	return c.GetGlobalOption(name)
}

// SetGlobalOption sets a global configuration option.
func (c *Config) SetGlobalOption(name, value string) {
	c.Global[name] = value
}

// SetCommandOption sets a command-specific configuration option.
func (c *Config) SetCommandOption(command, name, value string) {
	if c.Commands[command] == nil {
		c.Commands[command] = make(map[string]string)
	}
	c.Commands[command][name] = value
}

// GetWarnings returns any warnings generated during config loading.
func (c *Config) GetWarnings() []string {
	return c.Warnings
}

// HasWarnings returns true if there are any warnings.
func (c *Config) HasWarnings() bool {
	return len(c.Warnings) > 0
}
