package command

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// ---------------------------------------------------------------------------
// containsGlobMeta — zero direct tests prior to this file
// ---------------------------------------------------------------------------

func TestContainsGlobMeta(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "empty_string", path: "", want: false},
		{name: "plain_path", path: "/usr/local/bin", want: false},
		{name: "asterisk", path: "/tmp/prompts/*.md", want: true},
		{name: "question_mark", path: "/tmp/prompts/?.md", want: true},
		{name: "open_bracket", path: "/tmp/prompts/[ab].md", want: true},
		{name: "close_bracket_only", path: "/tmp/prompts/foo].md", want: false},
		{name: "double_star", path: "/tmp/**/goals", want: true},
		{name: "mid_path_star", path: "/home/*/goals", want: true},
		{name: "all_three_metas", path: "/a/*b?[c", want: true},
		{name: "no_meta_with_dots", path: "/path/to/file.prompt.md", want: false},
		{name: "tilde_not_meta", path: "~/goals", want: false},
		{name: "curly_brace_not_meta", path: "/tmp/{a,b}", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := containsGlobMeta(tc.path)
			if got != tc.want {
				t.Errorf("containsGlobMeta(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// configKeys — zero isolation tests prior to this file
// ---------------------------------------------------------------------------

func TestConfigKeys_Sorted(t *testing.T) {
	t.Parallel()
	keys := configKeys()
	if len(keys) == 0 {
		t.Fatal("configKeys() returned empty slice; DefaultSchema must define at least one global option")
	}
	if !sort.StringsAreSorted(keys) {
		t.Errorf("configKeys() returned unsorted keys: %v", keys)
	}
}

func TestConfigKeys_ContainsExpectedKeys(t *testing.T) {
	t.Parallel()
	keys := configKeys()
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}

	// Spot-check a few well-known global config keys.
	expected := []string{"verbose", "color", "debug", "session.id"}
	for _, k := range expected {
		if !set[k] {
			t.Errorf("configKeys() missing expected key %q; got %v", k, keys)
		}
	}
}

func TestConfigKeys_NoDuplicates(t *testing.T) {
	t.Parallel()
	keys := configKeys()
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		if seen[k] {
			t.Errorf("configKeys() contains duplicate key %q", k)
		}
		seen[k] = true
	}
}

// ---------------------------------------------------------------------------
// Session clean / purge confirm-abort — no tests supply "N" via stdin
// ---------------------------------------------------------------------------

func TestSessionClean_ConfirmAbort(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	// Simulate user typing "N\n" at the prompt.
	cmd.stdin = strings.NewReader("N\n")

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"clean"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for aborted clean, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected 'aborted' in output, got: %q", stdout.String())
	}
}

func TestSessionClean_ConfirmAbort_EmptyInput(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	// Empty stdin (EOF) should also abort.
	cmd.stdin = strings.NewReader("")

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"clean"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for EOF abort on clean, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected 'aborted' in output, got: %q", stdout.String())
	}
}

func TestSessionPurge_ConfirmAbort(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	// Simulate user typing "no\n" at the prompt.
	cmd.stdin = strings.NewReader("no\n")

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"purge"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for aborted purge, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected 'aborted' in output, got: %q", stdout.String())
	}
}

func TestSessionPurge_ConfirmAbort_EmptyInput(t *testing.T) {
	t.Parallel()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	// Empty stdin (EOF) should also abort.
	cmd.stdin = strings.NewReader("")

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"purge"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for EOF abort on purge, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("expected 'aborted' in output, got: %q", stdout.String())
	}
}
