package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/storage"
)

// SetKeyInFile updates or adds a global option key in the config file.
// It preserves comments and formatting. If the key exists in the global
// section, its line is replaced in-place. If not found, the key is inserted
// before the first section header (or appended at the end if no sections exist).
//
// Only global-section keys are matched; keys inside [section] blocks are
// ignored, ensuring command-specific options are never accidentally overwritten.
func SetKeyInFile(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config file: %w", err)
	}

	var lines []string
	if len(data) > 0 {
		lines = strings.Split(string(data), "\n")
	}

	found := false
	inGlobalSection := true
	insertIndex := len(lines) // default: append at end

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track section boundaries
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inGlobalSection && !found {
				insertIndex = i // insert before first section header
			}
			inGlobalSection = false
			continue
		}

		// Only match keys in the global section
		if !inGlobalSection {
			continue
		}

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Parse key from line: "keyName value..."
		parts := strings.SplitN(trimmed, " ", 2)
		if parts[0] == key {
			if value == "" {
				lines[i] = key
			} else {
				lines[i] = key + " " + value
			}
			found = true
			break
		}
	}

	if !found {
		var newLine string
		if value == "" {
			newLine = key
		} else {
			newLine = key + " " + value
		}
		if insertIndex >= len(lines) {
			// Append: ensure there's a trailing newline context
			if len(lines) > 0 && lines[len(lines)-1] == "" {
				// Insert before the trailing empty line (from trailing newline)
				lines = append(lines[:len(lines)-1], newLine, "")
			} else {
				lines = append(lines, newLine)
			}
		} else {
			// Insert before the section header at insertIndex
			lines = append(lines[:insertIndex+1], lines[insertIndex:]...)
			lines[insertIndex] = newLine
		}
	}

	result := strings.Join(lines, "\n")

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	return storage.AtomicWriteFile(path, []byte(result), 0644)
}
