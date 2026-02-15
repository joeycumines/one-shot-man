package config

import (
	"strings"
	"testing"
)

// FuzzLoadFromReader ensures the config parser does not panic on arbitrary input.
// The parser must gracefully handle any byte sequence without crashing.
func FuzzLoadFromReader(f *testing.F) {
	// Seed with valid config patterns
	f.Add("")
	f.Add("# comment")
	f.Add("some-option some-value")
	f.Add("[command-section]\noption value")
	f.Add("[sessions]\nstrategy explicit\nid my-session")
	f.Add("[hot-snippets]\nname test\ntext some snippet text\n---")
	f.Add("[hot-snippets]\nname test\ntext some text\nbuiltin true\n---\nname another\ntext more text\n---")
	// Edge cases
	f.Add("[]\nkey value")
	f.Add("[\nkey value")
	f.Add("]\nkey value")
	f.Add("[section]\n[nested]\nkey value")
	f.Add(strings.Repeat("x", 10000))
	f.Add(strings.Repeat("[section]\n", 100))
	f.Add("\x00\x01\x02\x03")
	f.Add("key")        // key without value
	f.Add("key ")       // key with empty value
	f.Add(" key value") // leading whitespace (trimmed)
	f.Add("[hot-snippets]\nname\n---")
	f.Add("[hot-snippets]\n---")
	f.Add("[hot-snippets]\ntext only\n---")
	f.Add("[sessions]\nstrategy")
	f.Add("[sessions]\nunknown-option value")

	f.Fuzz(func(t *testing.T, data string) {
		// Must not panic
		cfg, err := LoadFromReader(strings.NewReader(data))
		if err != nil {
			// Errors are fine — just must not panic
			return
		}
		if cfg == nil {
			t.Fatal("LoadFromReader returned nil config without error")
		}
	})
}
