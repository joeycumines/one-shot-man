package scripting

import (
	"github.com/joeycumines/one-shot-man/internal/argv"
)

// Helper: length in runes for a string
func currentWord(before string) string { _, cur := argv.BeforeCursor(before); return cur.Text }

// tokenizeCommandLine tokenizes an entire input line into arguments with shell-like rules.
// Supports single/double quotes and backslash escaping. Unclosed quotes are allowed and
// produce a final token up to the end of input. Returned tokens do not include surrounding quotes.
func tokenizeCommandLine(line string) []string { return argv.ParseSlice(line) }

// getInt extracts an integer value from a JavaScript object map.
func getInt(m map[string]interface{}, key string, defaultValue int) int {
	if val, exists := m[key]; exists {
		if i, ok := val.(int); ok {
			return i
		}
		if f, ok := val.(float64); ok {
			return int(f)
		}
	}
	return defaultValue
}

// Helper functions for extracting values from JavaScript objects

func getString(m map[string]interface{}, key, defaultValue string) string {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func getBool(m map[string]interface{}, key string, defaultValue bool) bool {
	if val, exists := m[key]; exists {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

func getStringSlice(m map[string]interface{}, key string) (result []string) {
	if val, exists := m[key]; exists {
		if arr, ok := val.([]interface{}); ok {
			for _, item := range arr {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
		}
	}
	return result
}
