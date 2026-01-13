package goroutineid

import (
	// import-fix
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGoroutineIDFromStack_Valid(t *testing.T) {
	// Simulate a typical runtime.Stack header
	stack := []byte("goroutine 123 [running]:\n")
	id := parseGoroutineIDFromStack(stack)
	require.Equal(t, int64(123), id)
}

func TestParseGoroutineIDFromStack_Invalid(t *testing.T) {
	// No prefix
	stack := []byte("something else\n")
	id := parseGoroutineIDFromStack(stack)
	require.Equal(t, int64(0), id)
}

func TestGetReturnsNonZero(t *testing.T) {
	id := Get()
	require.Greater(t, id, int64(0))
}
