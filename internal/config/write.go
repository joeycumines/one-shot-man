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

// DeleteKeyInFile removes a global option key from the config file.
// It preserves comments, formatting, and section content. Only global-section
// keys are matched; keys inside [section] blocks are left untouched.
// If the key is not found or the file does not exist, no error is returned.
func DeleteKeyInFile(path, key string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to delete
		}
		return fmt.Errorf("reading config file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	inGlobalSection := true
	deleteIndex := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track section boundaries.
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inGlobalSection = false
			continue
		}

		if !inGlobalSection {
			continue
		}

		// Skip comments and empty lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		parts := strings.SplitN(trimmed, " ", 2)
		if parts[0] == key {
			deleteIndex = i
			break
		}
	}

	if deleteIndex < 0 {
		return nil // key not found, nothing to do
	}

	// Remove the line.
	lines = append(lines[:deleteIndex], lines[deleteIndex+1:]...)

	result := strings.Join(lines, "\n")

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	return storage.AtomicWriteFile(path, []byte(result), 0644)
}

// DeleteAllGlobalKeysInFile removes all global option key lines from the
// config file. Comments, empty lines, section headers, and section contents
// are preserved. Returns the number of keys removed.
// If the file does not exist, returns (0, nil).
func DeleteAllGlobalKeysInFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading config file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	inGlobalSection := true
	kept := make([]string, 0, len(lines))
	removed := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track section boundaries.
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inGlobalSection = false
			kept = append(kept, line)
			continue
		}

		// Preserve everything outside the global section.
		if !inGlobalSection {
			kept = append(kept, line)
			continue
		}

		// Preserve comments and empty lines in the global section.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}

		// This is a global key line — remove it.
		removed++
	}

	if removed == 0 {
		return 0, nil
	}

	result := strings.Join(kept, "\n")

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, fmt.Errorf("creating config directory: %w", err)
	}

	return removed, storage.AtomicWriteFile(path, []byte(result), 0644)
}
