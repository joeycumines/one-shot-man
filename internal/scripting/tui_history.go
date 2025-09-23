package scripting

import (
	"os"
	"strings"
)

// parseHistoryConfig parses history configuration from JavaScript config.
func parseHistoryConfig(configMap map[string]interface{}) HistoryConfig {
	config := HistoryConfig{
		Enabled: false,
		File:    "",
		Size:    1000,
	}

	if historyRaw, exists := configMap["history"]; exists {
		if historyMap, ok := historyRaw.(map[string]interface{}); ok {
			config.Enabled = getBool(historyMap, "enabled", false)
			config.File = getString(historyMap, "file", "")
			config.Size = getInt(historyMap, "size", 1000)
		}
	}

	return config
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
