package scripting

import (
	"os"
	"strings"
)

// parseHistoryConfig parses history configuration from JavaScript config.
func parseHistoryConfig(configMap map[string]interface{}) (HistoryConfig, error) {
	config := HistoryConfig{
		Enabled: false,
		File:    "",
		Size:    1000,
	}

	if historyRaw, exists := configMap["history"]; exists {
		if historyMap, ok := historyRaw.(map[string]interface{}); ok {
			if v, err := getBool(historyMap, "enabled", false); err != nil {
				return HistoryConfig{}, err
			} else {
				config.Enabled = v
			}
			if v, err := getString(historyMap, "file", ""); err != nil {
				return HistoryConfig{}, err
			} else {
				config.File = v
			}
			if v, err := getInt(historyMap, "size", 1000); err != nil {
				return HistoryConfig{}, err
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

// saveHistory saves history to a file.
func saveHistory(filename string, history []string) error {
	if filename == "" {
		return nil // Silent no-op if no file specified
	}

	if len(history) == 0 {
		return nil // Nothing to save
	}

	// Create file content
	content := strings.Join(history, "\n")
	if content != "" {
		content += "\n" // Ensure trailing newline
	}

	// Write to file with proper permissions
	return os.WriteFile(filename, []byte(content), 0644)
}
