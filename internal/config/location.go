package config

import (
	"os"
	"path/filepath"
)

// GetConfigPath returns the configuration file path using kubectl-style behavior.
// It first checks the OSM_CONFIG environment variable, then falls back
// to the default location (~/.one-shot-man/config).
func GetConfigPath() (string, error) {
	// Check for environment variable override
	if configPath := os.Getenv("OSM_CONFIG"); configPath != "" {
		return configPath, nil
	}

	// Get user home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Default config location: ~/.one-shot-man/config
	configDir := filepath.Join(homeDir, ".one-shot-man")
	configPath := filepath.Join(configDir, "config")

	return configPath, nil
}

// EnsureConfigDir ensures that the configuration directory exists.
func EnsureConfigDir() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	return os.MkdirAll(configDir, 0755)
}
