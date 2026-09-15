package command

import (
	"strings"
	"testing"
)

// TestSubstitutePlaceholders pins the behaviour the reviewers asked to be able
// to demonstrate: a placeholder resolves to the materialized directory, and a
// placeholder with no directory is refused instead of silently becoming empty.
func TestSubstitutePlaceholders(t *testing.T) {
	key := placeholderNames[0]
	cases := []struct {
		name      string
		value     string
		directory string
		want      string
		wantErr   string
	}{
		{
			name:      "no placeholder is passed through",
			value:     "/plain/path",
			directory: "",
			want:      "/plain/path",
		},
		{
			name:      "placeholder resolves to the materialized directory",
			value:     "${" + key + "}/config.json",
			directory: "/tmp/launch",
			want:      "/tmp/launch/config.json",
		},
		{
			name:      "placeholder with no directory is refused",
			value:     "${" + key + "}/config.json",
			directory: "",
			wantErr:   "materialized no files",
		},
		{
			name:      "unknown placeholder is refused",
			value:     "${NOT_A_KNOWN_PLACEHOLDER}/config.json",
			directory: "/tmp/launch",
			wantErr:   "unresolved placeholder",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := substitutePlaceholders("EXAMPLE_VAR", testCase.value, testCase.directory)
			if testCase.wantErr != "" {
				if err == nil {
					t.Fatalf("expected an error containing %q, got value %q and no error", testCase.wantErr, got)
				}
				if !strings.Contains(err.Error(), testCase.wantErr) {
					t.Fatalf("expected an error containing %q, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Fatalf("got %q, want %q", got, testCase.want)
			}
		})
	}
}
