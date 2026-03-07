package config

import (
	"bufio"
	"errors"
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
	// ClaudeMux controls Claude-Mux (agent orchestration, PTY, MCP) behavior.
	ClaudeMux ClaudeMuxConfig
	// HotSnippets are user-configured text snippets that can be copied to
	// clipboard from interactive modes. Parsed from the [hot-snippets]
	// config section.
	HotSnippets []HotSnippet
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
		ClaudeMux: ClaudeMuxConfig{
			Provider:            "claude-code",
			EnvInherit:          true,
			PermissionPolicy:    "reject",
			RateLimitBackoffSec: 30,
			MaxAgents:           4,
			PTYRows:             24,
			PTYCols:             80,
			EnvVars:             make(map[string]string),
		},
		HotSnippets: make([]HotSnippet, 0),
		Warnings:    make([]string, 0),
	}
}

// HotSnippet represents a named text snippet that users can quickly copy
// to the clipboard from interactive modes.
type HotSnippet struct {
	Name        string `json:"name"`
	Text        string `json:"text"`
	Description string `json:"description,omitempty"`
}

// ClaudeMuxConfig controls Claude-Mux (claude-code orchestration) behavior.
type ClaudeMuxConfig struct {
	// Provider is the default AI provider name (e.g., "claude-code").
	Provider string `json:"provider" default:"claude-code"`
	// Model is the default model identifier.
	Model string `json:"model"`
	// WorkDir is the default working directory for agents (empty = CWD).
	WorkDir string `json:"workDir"`
	// EnvInherit controls whether agents inherit the parent environment.
	EnvInherit bool `json:"envInherit" default:"true"`
	// EnvVars are additional environment variables for all agents.
	// Parsed from KEY=VALUE entries in the [claude-mux] config section.
	EnvVars map[string]string `json:"envVars"`
	// EnvProfile is the active environment variable profile name.
	EnvProfile string `json:"envProfile"`
	// PreSpawnHook is a path to a JS file executed before agent spawn.
	// Used for credential injection (e.g., op plugin, aws-vault wrappers).
	PreSpawnHook string `json:"preSpawnHook"`
	// PermissionPolicy is the default permission handling: "reject" (default) or "ask".
	PermissionPolicy string `json:"permissionPolicy" default:"reject"`
	// RateLimitBackoffSec is the initial rate limit backoff in seconds.
	RateLimitBackoffSec int `json:"rateLimitBackoffSec" default:"30"`
	// MaxAgents is the maximum number of concurrent agents.
	MaxAgents int `json:"maxAgents" default:"4"`
	// PTYRows is the default PTY row count.
	PTYRows int `json:"ptyRows" default:"24"`
	// PTYCols is the default PTY column count.
	PTYCols int `json:"ptyCols" default:"80"`
	// ProviderCommand overrides the provider's default executable path.
	ProviderCommand string `json:"providerCommand"`
	// MCPServers is a comma-separated list of MCP server commands to attach.
	MCPServers string `json:"mcpServers"`
}

// SessionConfig controls session lifecycle and cleanup behavior.
type SessionConfig struct {
	MaxAgeDays int `json:"maxAgeDays" default:"90"`
	MaxCount   int `json:"maxCount" default:"100"`
	MaxSizeMB  int `json:"maxSizeMb" default:"500"`
	// AutoCleanupEnabled controls whether commands that create sessions
	// start a background cleanup scheduler. When true (the default),
	// session cleanup runs on command startup and then at the configured
	// interval (CleanupIntervalHours).
	AutoCleanupEnabled bool `json:"autoCleanupEnabled" default:"true"`
	// CleanupIntervalHours is the number of hours between automatic
	// cleanup runs. Only used when AutoCleanupEnabled is true.
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
		if errors.Is(err, os.ErrNotExist) {
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
	var inHotSnippetsSection bool
	var inClaudeMuxSection bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header [section_name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			switch sectionName {
			case "sessions":
				inSessionsSection = true
				inHotSnippetsSection = false
				inClaudeMuxSection = false
				currentCommand = ""
			case "hot-snippets":
				inHotSnippetsSection = true
				inSessionsSection = false
				inClaudeMuxSection = false
				currentCommand = ""
			case "claude-mux":
				inClaudeMuxSection = true
				inSessionsSection = false
				inHotSnippetsSection = false
				currentCommand = ""
			default:
				inSessionsSection = false
				inHotSnippetsSection = false
				inClaudeMuxSection = false
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
		} else if inHotSnippetsSection {
			// Hot-snippet definition
			if err := parseHotSnippetLine(&config.HotSnippets, optionName, value); err != nil {
				return nil, fmt.Errorf("invalid hot-snippet %q: %w", optionName, err)
			}
		} else if inClaudeMuxSection {
			// Claude-Mux configuration option
			if err := parseClaudeMuxOption(&config.ClaudeMux, optionName, value); err != nil {
				return nil, fmt.Errorf("invalid claude-mux option %q: %w", optionName, err)
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

// parseClaudeMuxOption parses a claude-mux configuration option and
// updates the ClaudeMuxConfig. Supported options:
//   - provider <string>: Default AI provider name (default: "claude-code")
//   - model <string>: Default model identifier
//   - work-dir <string>: Default working directory for agents
//   - env-inherit <bool>: Agents inherit parent environment (default: true)
//   - env <KEY=VALUE>: Additional environment variable (can appear multiple times)
//   - env-profile <string>: Active environment variable profile name
//   - pre-spawn-hook <string>: JS file path executed before agent spawn
//   - permission-policy <string>: "reject" (default) or "ask"
//   - rate-limit-backoff-sec <int>: Initial rate limit backoff in seconds (default: 30)
//   - max-agents <int>: Maximum concurrent agents (default: 4)
//   - pty-rows <int>: Default PTY row count (default: 24)
//   - pty-cols <int>: Default PTY column count (default: 80)
//   - provider-command <string>: Override provider executable path
//   - mcp-servers <string>: Comma-separated MCP server commands
func parseClaudeMuxOption(oc *ClaudeMuxConfig, name, value string) error {
	switch name {
	case "provider":
		oc.Provider = value

	case "model":
		oc.Model = value

	case "work-dir":
		oc.WorkDir = value

	case "env-inherit":
		enabled, err := parseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value %q: %w", value, err)
		}
		oc.EnvInherit = enabled

	case "env":
		idx := strings.Index(value, "=")
		if idx < 0 {
			return fmt.Errorf("env requires KEY=VALUE format, got %q", value)
		}
		key := value[:idx]
		val := value[idx+1:]
		if oc.EnvVars == nil {
			oc.EnvVars = make(map[string]string)
		}
		oc.EnvVars[key] = val

	case "env-profile":
		oc.EnvProfile = value

	case "pre-spawn-hook":
		oc.PreSpawnHook = value

	case "permission-policy":
		switch value {
		case "reject", "ask":
			oc.PermissionPolicy = value
		default:
			return fmt.Errorf("permission-policy must be \"reject\" or \"ask\", got %q", value)
		}

	case "rate-limit-backoff-sec":
		sec, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		oc.RateLimitBackoffSec = sec

	case "max-agents":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		oc.MaxAgents = n

	case "pty-rows":
		rows, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		oc.PTYRows = rows

	case "pty-cols":
		cols, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value %q: %w", value, err)
		}
		oc.PTYCols = cols

	case "provider-command":
		oc.ProviderCommand = value

	case "mcp-servers":
		oc.MCPServers = value

	default:
		return fmt.Errorf("unknown claude-mux option: %s", name)
	}
	return nil
}

// parseHotSnippetLine parses a single line from the [hot-snippets] config
// section. Two formats are supported:
//
//	snippetName text of the snippet     → defines a snippet (literal \n → newline)
//	snippetName.description Help text   → sets description on the last snippet named snippetName
//
// The name must not be empty. If a .description suffix targets a name that
// has not yet been defined, an error is returned.
func parseHotSnippetLine(snippets *[]HotSnippet, name, value string) error {
	if name == "" {
		return fmt.Errorf("empty snippet name")
	}

	// Check for .description suffix
	if dotIdx := strings.LastIndex(name, "."); dotIdx > 0 {
		baseName := name[:dotIdx]
		suffix := name[dotIdx+1:]
		if suffix == "description" {
			// Set description on the last snippet with baseName
			for i := len(*snippets) - 1; i >= 0; i-- {
				if (*snippets)[i].Name == baseName {
					(*snippets)[i].Description = value
					return nil
				}
			}
			return fmt.Errorf("snippet %q not found for .description", baseName)
		}
	}

	// Convert literal \n sequences to actual newlines
	text := strings.ReplaceAll(value, `\n`, "\n")
	*snippets = append(*snippets, HotSnippet{Name: name, Text: text})
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
