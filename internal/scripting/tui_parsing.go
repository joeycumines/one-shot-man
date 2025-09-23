package scripting

import (
	"fmt"
	"github.com/joeycumines/one-shot-man/internal/argv"
)

// Helper: length in runes for a string
func currentWord(before string) string { _, cur := argv.BeforeCursor(before); return cur.Text }

// tokenizeCommandLine tokenizes an entire input line into arguments with shell-like rules.
// Supports single/double quotes and backslash escaping. Unclosed quotes are allowed and
// produce a final token up to the end of input. Returned tokens do not include surrounding quotes.
func tokenizeCommandLine(line string) []string { return argv.ParseSlice(line) }

// getInt extracts an integer value from a JavaScript object map.
func getInt(m map[string]interface{}, key string, defaultValue int) (int, error) {
	if val, exists := m[key]; exists {
		switch v := val.(type) {
		case int:
			return v, nil
		case int32:
			return int(v), nil
		case int64:
			return int(v), nil
		case float64:
			return int(v), nil
		case float32:
			return int(v), nil
		default:
			return 0, fmt.Errorf("value for key '%s' is not a number: got %T", key, val)
		}
	}
	return defaultValue, nil
}

// Helper functions for extracting values from JavaScript objects

func getString(m map[string]interface{}, key, defaultValue string) (string, error) {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str, nil
		}
		return "", fmt.Errorf("value for key '%s' is not a string: got %T", key, val)
	}
	return defaultValue, nil
}

func getBool(m map[string]interface{}, key string, defaultValue bool) (bool, error) {
	if val, exists := m[key]; exists {
		if b, ok := val.(bool); ok {
			return b, nil
		}
		return false, fmt.Errorf("value for key '%s' is not a bool: got %T", key, val)
	}
	return defaultValue, nil
}

func getStringSlice(m map[string]interface{}, key string) ([]string, error) {
	if val, exists := m[key]; exists {
		if arr, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(arr))
			for i, item := range arr {
				str, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("value for key '%s' at index %d is not a string: got %T", key, i, item)
				}
				result = append(result, str)
			}
			return result, nil
		}
		return nil, fmt.Errorf("value for key '%s' is not an array: got %T", key, val)
	}
	return nil, nil
}
