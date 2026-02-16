package config

import (
	"os"
	"path/filepath"
)

const (
	// DefaultConfigDir is the new default configuration directory name.
	DefaultConfigDir = ".osm"

	// LegacyConfigDir is the legacy configuration directory name.
	// Used as a fallback when the new directory does not exist.
	LegacyConfigDir = ".one-shot-man"
)

// GetConfigPath returns the configuration file path using kubectl-style behavior.
// It first checks the OSM_CONFIG environment variable, then falls back
// to the default location (~/.osm/config). If ~/.osm/ does not exist
// but ~/.one-shot-man/config does, the legacy path is used for backward
// compatibility.
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

	// New default config location: ~/.osm/config
	newConfigDir := filepath.Join(homeDir, DefaultConfigDir)
	newConfigPath := filepath.Join(newConfigDir, "config")

	// If the new directory exists, use it.
	if _, err := os.Stat(newConfigDir); err == nil {
		return newConfigPath, nil
	}

	// Fall back to legacy path if ~/.one-shot-man/config exists.
	legacyConfigPath := filepath.Join(homeDir, LegacyConfigDir, "config")
	if _, err := os.Stat(legacyConfigPath); err == nil {
		return legacyConfigPath, nil
	}

	// Neither exists — return the new default path for new installations.
	return newConfigPath, nil
}

// EnsureConfigDir ensures that the configuration directory exists.
// Creates ~/.osm/ (the new default), not the legacy ~/.one-shot-man/.
func EnsureConfigDir() error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	return os.MkdirAll(configDir, 0755)
}
