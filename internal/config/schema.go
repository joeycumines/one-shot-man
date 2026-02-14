package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OptionType represents the expected type of a configuration option value.
type OptionType string

const (
	// TypeString is a plain string value (the default for all config values).
	TypeString OptionType = "string"
	// TypeBool is a boolean value (true/false/yes/no/1/0/on/off).
	TypeBool OptionType = "bool"
	// TypeInt is an integer value.
	TypeInt OptionType = "int"
	// TypeDuration is a Go time.Duration value (e.g. "30s", "5m", "1h").
	TypeDuration OptionType = "duration"
	// TypePathList is a colon-separated (or semicolon on Windows) list of paths.
	TypePathList OptionType = "path-list"
)

// ConfigOption declares a single configuration option with its type, default,
// documentation, and environment variable override.
type ConfigOption struct {
	// Key is the option name as it appears in the config file (kebab-case).
	Key string
	// Type is the expected value type for validation.
	Type OptionType
	// Default is the default value as a string, or "" for no default.
	Default string
	// Description is a human-readable description of the option.
	Description string
	// Section is "" for global options, or a command/section name.
	Section string
	// EnvVar is the environment variable that overrides this option, or "".
	EnvVar string
}

// ConfigSchema declares the expected configuration options for the application.
// It is used for validation, documentation, typed getters, and env var mapping.
type ConfigSchema struct {
	options []*ConfigOption
	// byKey indexes global options by key for fast lookup.
	byKey map[string]*ConfigOption
	// bySection indexes command/section options by section then key.
	bySection map[string]map[string]*ConfigOption
}

// NewSchema creates a new empty ConfigSchema.
func NewSchema() *ConfigSchema {
	return &ConfigSchema{
		byKey:     make(map[string]*ConfigOption),
		bySection: make(map[string]map[string]*ConfigOption),
	}
}

// Register adds a ConfigOption to the schema. Duplicate keys within the same
// section are silently overwritten (last registration wins).
func (s *ConfigSchema) Register(opt ConfigOption) {
	ref := new(ConfigOption)
	*ref = opt
	s.options = append(s.options, ref)
	if opt.Section == "" {
		s.byKey[opt.Key] = ref
	} else {
		if s.bySection[opt.Section] == nil {
			s.bySection[opt.Section] = make(map[string]*ConfigOption)
		}
		s.bySection[opt.Section][opt.Key] = ref
	}
}

// RegisterAll adds multiple ConfigOptions to the schema.
func (s *ConfigSchema) RegisterAll(opts []ConfigOption) {
	for _, opt := range opts {
		s.Register(opt)
	}
}

// Lookup returns the ConfigOption for a key in a given section ("" for global).
// Returns nil if the key is not registered.
func (s *ConfigSchema) Lookup(section, key string) *ConfigOption {
	if section == "" {
		return s.byKey[key]
	}
	if sec, ok := s.bySection[section]; ok {
		return sec[key]
	}
	return nil
}

// IsKnown returns true if the key is registered in the given section.
// For command sections, global keys are also considered known (they can
// appear in command sections and fall back to the global value).
func (s *ConfigSchema) IsKnown(section, key string) bool {
	if section == "" {
		return s.byKey[key] != nil
	}
	// Command section: check section-specific, then global.
	if sec, ok := s.bySection[section]; ok {
		if sec[key] != nil {
			return true
		}
	}
	return s.byKey[key] != nil
}

// GlobalOptions returns all registered global options (Section == "").
func (s *ConfigSchema) GlobalOptions() []ConfigOption {
	var out []ConfigOption
	for _, o := range s.options {
		if o.Section == "" {
			out = append(out, *o)
		}
	}
	return out
}

// SectionOptions returns all registered options for a specific section.
func (s *ConfigSchema) SectionOptions(section string) []ConfigOption {
	var out []ConfigOption
	for _, o := range s.options {
		if o.Section == section {
			out = append(out, *o)
		}
	}
	return out
}

// Sections returns a sorted list of all registered non-empty section names.
func (s *ConfigSchema) Sections() []string {
	seen := make(map[string]bool)
	for sec := range s.bySection {
		seen[sec] = true
	}
	out := make([]string, 0, len(seen))
	for sec := range seen {
		out = append(out, sec)
	}
	sort.Strings(out)
	return out
}

// Resolve returns the effective value for a global config key by checking,
// in order: (1) the environment variable declared in the schema for this key,
// (2) the config value, (3) the schema default. Returns "" if the key is not
// found anywhere.
func (s *ConfigSchema) Resolve(c *Config, key string) string {
	opt := s.Lookup("", key)
	// Check env var override from schema.
	if opt != nil && opt.EnvVar != "" {
		if v, ok := os.LookupEnv(opt.EnvVar); ok {
			return v
		}
	}
	// Check config value.
	v, ok := c.GetGlobalOption(key)
	if ok {
		return v
	}
	// Fall back to schema default.
	if opt != nil {
		return opt.Default
	}
	return ""
}

// ValidateConfig checks a loaded Config against the schema and returns a list
// of human-readable issues (empty if the config is valid). Validation includes:
//   - Unknown global options (not in schema)
//   - Unknown command options (not in schema for that section, and not global)
//   - Type mismatches for options with declared types
func ValidateConfig(c *Config, s *ConfigSchema) []string {
	var issues []string

	// Validate global options.
	for key, value := range c.Global {
		opt := s.Lookup("", key)
		if opt == nil {
			issues = append(issues, fmt.Sprintf("unknown global option: %q (value: %q)", key, value))
			continue
		}
		if err := validateType(opt.Type, value); err != nil {
			issues = append(issues, fmt.Sprintf("global option %q: %v", key, err))
		}
	}

	// Validate command-section options.
	for section, opts := range c.Commands {
		for key, value := range opts {
			if !s.IsKnown(section, key) {
				issues = append(issues, fmt.Sprintf("unknown option for command %q: %q (value: %q)", section, key, value))
				continue
			}
			// Find the option definition (section-specific or global fallback).
			opt := s.Lookup(section, key)
			if opt == nil {
				opt = s.Lookup("", key)
			}
			if opt != nil {
				if err := validateType(opt.Type, value); err != nil {
					issues = append(issues, fmt.Sprintf("option %q in [%s]: %v", key, section, err))
				}
			}
		}
	}

	sort.Strings(issues)
	return issues
}

// validateType checks that a string value matches the expected OptionType.
func validateType(t OptionType, value string) error {
	switch t {
	case TypeString, TypePathList, "":
		// Anything is valid for string and path-list.
		return nil
	case TypeBool:
		if _, err := parseBool(value); err != nil {
			return fmt.Errorf("expected bool, got %q", value)
		}
	case TypeInt:
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("expected int, got %q", value)
		}
	case TypeDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("expected duration, got %q", value)
		}
	default:
		return fmt.Errorf("unknown option type %q", t)
	}
	return nil
}

// --- Typed getter methods on Config ---

// GetString returns the global option value for key, or "" if not set.
func (c *Config) GetString(key string) string {
	v, _ := c.GetGlobalOption(key)
	return v
}

// GetStringDefault returns the global option value for key, or defaultValue if
// not set.
func (c *Config) GetStringDefault(key, defaultValue string) string {
	v, ok := c.GetGlobalOption(key)
	if !ok {
		return defaultValue
	}
	return v
}

// GetBool returns the global option value for key parsed as a boolean. Returns
// false if the key is not set or the value cannot be parsed.
func (c *Config) GetBool(key string) bool {
	v, ok := c.GetGlobalOption(key)
	if !ok {
		return false
	}
	b, err := parseBool(v)
	if err != nil {
		return false
	}
	return b
}

// GetInt returns the global option value for key parsed as an integer. Returns
// 0 if the key is not set or the value cannot be parsed.
func (c *Config) GetInt(key string) int {
	v, ok := c.GetGlobalOption(key)
	if !ok {
		return 0
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return i
}

// GetDuration returns the global option value for key parsed as a
// time.Duration. Returns 0 if the key is not set or the value cannot be parsed.
func (c *Config) GetDuration(key string) time.Duration {
	v, ok := c.GetGlobalOption(key)
	if !ok {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0
	}
	return d
}

// GetWithEnv returns the value for key, checking the environment variable first.
// If envVar is non-empty and the corresponding environment variable is set
// (even to ""), it takes precedence. Otherwise falls back to the global config.
func (c *Config) GetWithEnv(key, envVar string) string {
	if envVar != "" {
		if v, ok := os.LookupEnv(envVar); ok {
			return v
		}
	}
	return c.GetString(key)
}

// --- Help text generation ---

// FormatHelp returns a formatted, human-readable reference of all registered
// options in the schema, grouped by section.
func (s *ConfigSchema) FormatHelp() string {
	var b strings.Builder

	// Global options first.
	globals := s.GlobalOptions()
	if len(globals) > 0 {
		b.WriteString("Global Options:\n")
		for _, o := range globals {
			writeOptionHelp(&b, o)
		}
	}

	// Section options.
	for _, sec := range s.Sections() {
		opts := s.SectionOptions(sec)
		if len(opts) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\n[%s] Options:\n", sec))
		for _, o := range opts {
			writeOptionHelp(&b, o)
		}
	}

	return b.String()
}

func writeOptionHelp(b *strings.Builder, o ConfigOption) {
	b.WriteString(fmt.Sprintf("  %-35s %s", o.Key, o.Description))
	parts := make([]string, 0, 3)
	if o.Type != "" && o.Type != TypeString {
		parts = append(parts, fmt.Sprintf("type: %s", o.Type))
	}
	if o.Default != "" {
		parts = append(parts, fmt.Sprintf("default: %s", o.Default))
	}
	if o.EnvVar != "" {
		parts = append(parts, fmt.Sprintf("env: %s", o.EnvVar))
	}
	if len(parts) > 0 {
		b.WriteString(fmt.Sprintf(" (%s)", strings.Join(parts, ", ")))
	}
	b.WriteString("\n")
}

// --- Default schema for osm ---

// DefaultSchema returns the canonical schema declaring all known osm
// configuration options. This is the single source of truth for option names,
// types, defaults, descriptions, and environment variable overrides.
func DefaultSchema() *ConfigSchema {
	s := NewSchema()
	s.RegisterAll(defaultGlobalOptions())
	s.RegisterAll(defaultCommandOptions())
	return s
}

func defaultGlobalOptions() []ConfigOption {
	return []ConfigOption{
		// Core global options
		{Key: "verbose", Type: TypeBool, Default: "false", Description: "Enable verbose output"},
		{Key: "color", Type: TypeString, Default: "auto", Description: "Color mode: auto, always, never"},
		{Key: "pager", Type: TypeString, Default: "", Description: "Pager program for long output"},
		{Key: "format", Type: TypeString, Default: "", Description: "Default output format"},
		{Key: "timeout", Type: TypeDuration, Default: "", Description: "Default command timeout"},
		{Key: "session.id", Type: TypeString, Default: "", Description: "Override session ID", EnvVar: "OSM_SESSION_ID"},
		{Key: "output", Type: TypeString, Default: "", Description: "Default output destination"},
		{Key: "editor", Type: TypeString, Default: "", Description: "Editor for interactive editing", EnvVar: "EDITOR"},
		{Key: "debug", Type: TypeBool, Default: "false", Description: "Enable debug mode"},
		{Key: "quiet", Type: TypeBool, Default: "false", Description: "Suppress non-essential output"},

		// Script discovery options
		{Key: "script.autodiscovery", Type: TypeBool, Default: "false", Description: "Enable advanced script autodiscovery"},
		{Key: "script.git-traversal", Type: TypeBool, Default: "false", Description: "Traverse git repos for scripts"},
		{Key: "script.max-traversal-depth", Type: TypeInt, Default: "10", Description: "Max directory traversal depth for scripts"},
		{Key: "script.paths", Type: TypePathList, Default: "", Description: "Custom script search paths"},
		{Key: "script.path-patterns", Type: TypePathList, Default: "scripts", Description: "Glob patterns for script directories"},
		{Key: "script.disable-standard-paths", Type: TypeBool, Default: "false", Description: "Disable standard script paths"},
		{Key: "script.debug-discovery", Type: TypeBool, Default: "false", Description: "Debug logging for script discovery"},
		{Key: "script.module-paths", Type: TypePathList, Default: "", Description: "Module search paths for require()"},

		// Goal discovery options
		{Key: "goal.autodiscovery", Type: TypeBool, Default: "true", Description: "Enable goal autodiscovery"},
		{Key: "goal.disable-standard-paths", Type: TypeBool, Default: "false", Description: "Disable standard goal paths"},
		{Key: "goal.max-traversal-depth", Type: TypeInt, Default: "10", Description: "Max directory traversal depth for goals"},
		{Key: "goal.paths", Type: TypePathList, Default: "", Description: "Custom goal search paths"},
		{Key: "goal.path-patterns", Type: TypePathList, Default: "osm-goals,goals", Description: "Patterns for goal directories"},
		{Key: "goal.debug-discovery", Type: TypeBool, Default: "false", Description: "Debug logging for goal discovery"},

		// Sync options (reserved)
		{Key: "sync.repository", Type: TypeString, Default: "", Description: "Git repository URL for sync"},
		{Key: "sync.enabled", Type: TypeBool, Default: "false", Description: "Enable git synchronisation"},
		{Key: "sync.auto-pull", Type: TypeBool, Default: "false", Description: "Auto-pull on startup"},
		{Key: "sync.local-path", Type: TypeString, Default: "", Description: "Local path for sync repository"},

		// Logging options
		{Key: "log.file", Type: TypeString, Default: "", Description: "Default log file path (JSON output)", EnvVar: "OSM_LOG_FILE"},
		{Key: "log.level", Type: TypeString, Default: "info", Description: "Default log level: debug, info, warn, error", EnvVar: "OSM_LOG_LEVEL"},
		{Key: "log.max-size-mb", Type: TypeInt, Default: "10", Description: "Max log file size in MB before rotation"},
		{Key: "log.max-files", Type: TypeInt, Default: "5", Description: "Max number of rotated log backup files"},
		{Key: "log.buffer-size", Type: TypeInt, Default: "1000", Description: "In-memory log buffer size (entries)"},
	}
}

func defaultCommandOptions() []ConfigOption {
	return []ConfigOption{
		// [help] section
		{Key: "pager", Section: "help", Type: TypeString, Default: "", Description: "Pager for help output"},
		{Key: "format", Section: "help", Type: TypeString, Default: "", Description: "Help output format"},
		{Key: "output", Section: "help", Type: TypeString, Default: "", Description: "Help output destination"},

		// [version] section
		{Key: "format", Section: "version", Type: TypeString, Default: "", Description: "Version output format"},
		{Key: "output", Section: "version", Type: TypeString, Default: "", Description: "Version output destination"},

		// [prompt] section
		{Key: "template", Section: "prompt", Type: TypeString, Default: "", Description: "Default prompt template"},
		{Key: "output", Section: "prompt", Type: TypeString, Default: "", Description: "Prompt output destination"},
		{Key: "editor", Section: "prompt", Type: TypeString, Default: "", Description: "Editor for prompt editing"},
		{Key: "add-context", Section: "prompt", Type: TypeString, Default: "", Description: "Auto-add context items"},

		// [session] section
		{Key: "list", Section: "session", Type: TypeString, Default: "", Description: "Session list format"},
		{Key: "delete", Section: "session", Type: TypeString, Default: "", Description: "Session deletion mode"},
		{Key: "export", Section: "session", Type: TypeString, Default: "", Description: "Session export format"},
		{Key: "import", Section: "session", Type: TypeString, Default: "", Description: "Session import format"},

		// [sessions] section — lifecycle and automatic cleanup settings.
		// These options are parsed by the [sessions] config section handler
		// (parseSessionOption) and stored in SessionConfig. The schema entries
		// exist for documentation and the 'config schema' subcommand.
		{Key: "maxAgeDays", Section: "sessions", Type: TypeInt, Default: "90", Description: "Maximum age of sessions in days before cleanup"},
		{Key: "maxCount", Section: "sessions", Type: TypeInt, Default: "100", Description: "Maximum number of sessions to keep"},
		{Key: "maxSizeMB", Section: "sessions", Type: TypeInt, Default: "500", Description: "Maximum total size of sessions in MB"},
		{Key: "autoCleanupEnabled", Section: "sessions", Type: TypeBool, Default: "true", Description: "Enable automatic background session cleanup"},
		{Key: "cleanupIntervalHours", Section: "sessions", Type: TypeInt, Default: "24", Description: "Hours between automatic cleanup runs"},
	}
}
