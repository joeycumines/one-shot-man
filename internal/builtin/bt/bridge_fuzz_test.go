package bt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBridge_LoadScript_FuzzInputs(t *testing.T) {
	t.Parallel()

	bridge := testBridge(t)
	cases := []struct {
		name        string
		src         string
		expectError bool
	}{
		{"empty", "", false},
		{"simple", "var a = 1;", false},
		{"function", "function f(){return 1;}", false},
		{"large_string", "var s = \"" + strings.Repeat("A", 10000) + "\";", false},
		{"throw", "throw new Error('test')", true},
		{"odd_syntax", "0..toString()", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := bridge.LoadScript("fuzz", tc.src)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
