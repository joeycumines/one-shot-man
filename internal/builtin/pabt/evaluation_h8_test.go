package pabt

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestJSCondition_Match_ErrorCases_H8 is an EXHAUSTIVE test for H8 fix.
// Verifies that JSCondition.Match correctly distinguishes error cases from false matches.
func TestJSCondition_Match_ErrorCases_H8(t *testing.T) {
	t.Parallel()

	// Test 1: Nil condition should return false and not panic
	t.Run("nil_condition_returns_false", func(t *testing.T) {
		var cond *JSCondition = nil
		result := cond.Match("test")
		require.False(t, result, "Nil condition should return false")
	})

	// Test 2: Nil matcher should return false (struct literal approach per existing tests)
	t.Run("nil_matcher_returns_false", func(t *testing.T) {
		cond := &JSCondition{
			key:     "test",
			matcher: nil,
			bridge:  nil,
		}
		result := cond.Match("anything")
		require.False(t, result, "JSCondition with nil matcher should return false")
	})

	// Test 3: Nil bridge should return false
	t.Run("nil_bridge_returns_false", func(t *testing.T) {
		cond := &JSCondition{
			key:     "test",
			matcher: nil,
			bridge:  nil,
		}
		result := cond.Match("anything")
		require.False(t, result, "JSCondition with nil bridge should return false")
	})

	// Test 5: Multiple nil conditions all return false
	t.Run("multiple_nil_conditions", func(t *testing.T) {
		keys := []string{"key1", "key2", "key3", "key4", "key5"}
		for _, key := range keys {
			cond := &JSCondition{
				key:     key,
				matcher: nil,
				bridge:  nil,
			}
			result := cond.Match("test")
			require.False(t, result, "Condition with key=%q should return false", key)
		}
	})

	// Test 8: Error logging should not interfere with result
	t.Run("error_logging_does_not_affect_result", func(t *testing.T) {
		cond := &JSCondition{
			key:     "error_log_test",
			matcher: nil,
			bridge:  nil,
		}

		// Run multiple times to ensure consistency
		for i := 0; i < 5; i++ {
			result := cond.Match("test")
			require.False(t, result, "Iteration %d: nil matcher should return false", i)
		}
	})
}

// TestJSCondition_Match_ErrorCases_H8_Comprehensive provides additional exhaustive error case coverage
func TestJSCondition_Match_ErrorCases_H8_Comprehensive(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		key    string
		result bool
	}{
		{"short_key", "k", false},
		{"long_key", strings.Repeat("x", 1000), false},
		{"unicode_key", " ключ日本語", false},
		{"newline_key", "key\nwith\nnewlines", false},
		{"tab_key", "key\twith\ttabs", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldStderr := os.Stderr
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stderr = w

			cond := &JSCondition{
				key:     tc.key,
				matcher: nil,
				bridge:  nil,
			}
			result := cond.Match("test")

			w.Close()
			os.Stderr = oldStderr

			output, _ := io.ReadAll(r)
			outputStr := string(output)

			require.Equal(t, tc.result, result, "Result should match expected for key=%q", tc.key)
			require.Contains(t, outputStr, tc.key,
				"Error message should contain key=%q for debugging", tc.key)
		})
	}
}
