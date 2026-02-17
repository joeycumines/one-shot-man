package scripting

import (
	"os"
	"path/filepath"
	"strings"
)

// parseHistoryConfig parses history configuration from JavaScript config.
func parseHistoryConfig(configMap map[string]interface{}) (historyConfig, error) {
	config := historyConfig{
		Enabled: false,
		File:    "",
		Size:    1000,
	}

	if historyRaw, exists := configMap["history"]; exists {
		if historyMap, ok := historyRaw.(map[string]interface{}); ok {
			if v, err := getBool(historyMap, "enabled", false); err != nil {
				return historyConfig{}, err
			} else {
				config.Enabled = v
			}
			if v, err := getString(historyMap, "file", ""); err != nil {
				return historyConfig{}, err
			} else {
				config.File = v
			}
			if v, err := getInt(historyMap, "size", 1000); err != nil {
				return historyConfig{}, err
			} else {
				config.Size = v
			}
		}
	}

	return config, nil
}

// loadHistory loads history from a file.
func loadHistory(filename string) []string {
	if filename == "" {
		return []string{}
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		return []string{}
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	var history []string
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			history = append(history, line)
		}
	}

	return history
}

// saveHistory persists history entries to a file, deduplicating consecutive
// identical entries and keeping only the last maxEntries entries.
// Parent directories are created if needed. Returns nil on empty filename.
func saveHistory(filename string, entries []string, maxEntries int) error {
	if filename == "" {
		return nil
	}

	// Deduplicate consecutive identical entries
	var deduped []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if len(deduped) == 0 || entry != deduped[len(deduped)-1] {
			deduped = append(deduped, entry)
		}
	}

	// Trim to maxEntries (keep most recent)
	if maxEntries > 0 && len(deduped) > maxEntries {
		deduped = deduped[len(deduped)-maxEntries:]
	}

	// Ensure parent directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	content := strings.Join(deduped, "\n") + "\n"
	return os.WriteFile(filename, []byte(content), 0o600)
}
