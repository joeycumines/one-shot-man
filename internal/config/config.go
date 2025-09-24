package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Config represents the application configuration.
type Config struct {
	// Global options that apply to all commands
	Global map[string]string
	// Command-specific options
	Commands map[string]map[string]string
}

// NewConfig creates a new empty configuration.
func NewConfig() *Config {
	return &Config{
		Global:   make(map[string]string),
		Commands: make(map[string]map[string]string),
	}
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
func LoadFromPath(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty config if file doesn't exist
			return NewConfig(), nil
		}
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

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for command section header [command_name]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentCommand = strings.Trim(line, "[]")
			if config.Commands[currentCommand] == nil {
				config.Commands[currentCommand] = make(map[string]string)
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
		if currentCommand == "" {
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

	return config, nil
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

// GetScriptConfig returns script-specific configuration options for a command.
// It extracts options with the given prefix and returns them as a map with the prefix stripped.
func (c *Config) GetScriptConfig(command, prefix string) map[string]string {
	result := make(map[string]string)
	
	// Check command-specific options first
	if cmdOptions, exists := c.Commands[command]; exists {
		for key, value := range cmdOptions {
			if strings.HasPrefix(key, prefix) {
				strippedKey := strings.TrimPrefix(key, prefix)
				if strippedKey != "" {
					result[strippedKey] = value
				}
			}
		}
	}
	
	// Check global options as fallback
	for key, value := range c.Global {
		if strings.HasPrefix(key, prefix) {
			strippedKey := strings.TrimPrefix(key, prefix)
			if strippedKey != "" && result[strippedKey] == "" {
				result[strippedKey] = value
			}
		}
	}
	
	return result
}

// GetTemplateOverride checks if there's a custom template configured for a command.
// It returns the template content and true if found, or empty string and false if not.
func (c *Config) GetTemplateOverride(command string) (string, bool) {
	// Check for template file path first
	if templatePath, exists := c.GetCommandOption(command, "template.file"); exists {
		if content, err := os.ReadFile(templatePath); err == nil {
			return string(content), true
		}
	}
	
	// Check for inline template content
	if templateContent, exists := c.GetCommandOption(command, "template.content"); exists {
		return templateContent, true
	}
	
	return "", false
}
