package command

// pr_split_flag_provided_test.go — flag presence detection.
//
// applyConfigDefaults uses flagProvided to tell "the user did not pass this
// flag" (apply the config file value) from "the user passed --flag=false"
// (keep the explicit value). Getting that wrong silently overrides a user's
// explicit command line with their config file.

import "testing"

func TestFlagProvided(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		flag string
		want bool
	}{
		{"absent", []string{"pr-split"}, "--dry-run", false},
		{"present", []string{"pr-split", "--dry-run"}, "--dry-run", true},
		{"explicit false counts as present", []string{"pr-split", "--dry-run=false"}, "--dry-run", true},
		{"explicit true counts as present", []string{"pr-split", "--dry-run=true"}, "--dry-run", true},
		{"other flag does not match", []string{"pr-split", "--json"}, "--dry-run", false},
		{"prefix of another flag does not match", []string{"pr-split", "--dry-run-extra"}, "--dry-run", false},
		{"terminator stops the scan", []string{"pr-split", "--", "--dry-run"}, "--dry-run", false},
		{"only the long form matches", []string{"pr-split", "-dry-run"}, "--dry-run", false},
		{"empty args", nil, "--dry-run", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := flagProvided(tc.args, tc.flag); got != tc.want {
				t.Errorf("flagProvided(%q, %q) = %v, want %v", tc.args, tc.flag, got, tc.want)
			}
		})
	}
}
