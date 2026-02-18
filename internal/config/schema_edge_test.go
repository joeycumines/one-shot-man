package config

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// 1. Out-of-Range Values
// =============================================================================

func TestValidateType_IntOverflow(t *testing.T) {
	t.Parallel()
	// A value larger than max int64 should fail strconv.Atoi.
	huge := "99999999999999999999"
	err := ValidateOptionValue(TypeInt, huge)
	if err == nil {
		t.Fatalf("expected error for int overflow %q, got nil", huge)
	}
	if !strings.Contains(err.Error(), "expected int") {
		t.Errorf("expected 'expected int' error, got: %v", err)
	}
}

func TestValidateType_IntNegativeIsValid(t *testing.T) {
	t.Parallel()
	if err := ValidateOptionValue(TypeInt, "-1"); err != nil {
		t.Fatalf("expected -1 to be valid int, got error: %v", err)
	}
}

func TestValidateType_IntMaxInt64(t *testing.T) {
	t.Parallel()
	// max int64 as string — strconv.Atoi handles it on 64-bit platforms.
	maxStr := strconv.FormatInt(math.MaxInt64, 10)
	if err := ValidateOptionValue(TypeInt, maxStr); err != nil {
		t.Fatalf("expected max int64 to be valid, got: %v", err)
	}
}

func TestValidateType_IntMinInt64(t *testing.T) {
	t.Parallel()
	minStr := strconv.FormatInt(math.MinInt64, 10)
	if err := ValidateOptionValue(TypeInt, minStr); err != nil {
		t.Fatalf("expected min int64 to be valid, got: %v", err)
	}
}

func TestValidateType_DurationAbsurdlyLarge(t *testing.T) {
	t.Parallel()
	// 876000h = 100 years. time.ParseDuration should handle this.
	if err := ValidateOptionValue(TypeDuration, "876000h"); err != nil {
		t.Fatalf("expected 876000h to parse as duration, got: %v", err)
	}
}

func TestValidateType_DurationZero(t *testing.T) {
	t.Parallel()
	if err := ValidateOptionValue(TypeDuration, "0s"); err != nil {
		t.Fatalf("expected 0s to parse as duration, got: %v", err)
	}
}

func TestValidateType_DurationNanosecond(t *testing.T) {
	t.Parallel()
	if err := ValidateOptionValue(TypeDuration, "1ns"); err != nil {
		t.Fatalf("expected 1ns to parse as duration, got: %v", err)
	}
}

func TestValidateType_DurationNegative(t *testing.T) {
	t.Parallel()
	if err := ValidateOptionValue(TypeDuration, "-5m"); err != nil {
		t.Fatalf("expected -5m to parse as duration, got: %v", err)
	}
}

func TestValidateConfig_BoundaryIntValues(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "count", Type: TypeInt, Section: ""})

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"zero", "0", true},
		{"negative", "-42", true},
		{"max_int64", strconv.FormatInt(math.MaxInt64, 10), true},
		{"min_int64", strconv.FormatInt(math.MinInt64, 10), true},
		{"overflow", "99999999999999999999", false},
		{"negative_overflow", "-99999999999999999999", false},
		{"float", "3.14", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := NewConfig()
			c.SetGlobalOption("count", tc.value)
			issues := ValidateConfig(c, s)
			if tc.valid && len(issues) != 0 {
				t.Errorf("expected valid for %q, got issues: %v", tc.value, issues)
			}
			if !tc.valid && len(issues) == 0 {
				t.Errorf("expected issues for %q, got none", tc.value)
			}
		})
	}
}

// =============================================================================
// 2. Special Characters in Config Values
// =============================================================================

func TestLoadConfig_SpecialCharsInValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		key      string
		expected string
	}{
		{
			name:     "tab_in_value",
			input:    "color auto\ttinted",
			key:      "color",
			expected: "auto\ttinted",
		},
		{
			name:     "equals_in_value",
			input:    "color key=value=other",
			key:      "color",
			expected: "key=value=other",
		},
		{
			name:     "colons_in_value",
			input:    "color /usr/bin:/usr/local/bin:/opt/bin",
			key:      "color",
			expected: "/usr/bin:/usr/local/bin:/opt/bin",
		},
		{
			name:     "hash_mid_value",
			input:    "color hello#world",
			key:      "color",
			expected: "hello#world",
		},
		{
			name:     "unicode_emoji",
			input:    "color 🚀🎉✨",
			key:      "color",
			expected: "🚀🎉✨",
		},
		{
			name:     "unicode_cjk",
			input:    "color 日本語テスト",
			key:      "color",
			expected: "日本語テスト",
		},
		{
			name:     "mixed_unicode_ascii",
			input:    "color hello-世界-🌍",
			key:      "color",
			expected: "hello-世界-🌍",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := LoadFromReader(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("LoadFromReader error: %v", err)
			}
			got, ok := cfg.GetGlobalOption(tc.key)
			if !ok {
				t.Fatalf("key %q not found in config", tc.key)
			}
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestLoadConfig_DottedKeys(t *testing.T) {
	t.Parallel()
	// Keys with dots are commonly used (goal.autodiscovery, script.paths, etc.)
	input := "goal.autodiscovery true\nscript.max-traversal-depth 5"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}

	val, ok := cfg.GetGlobalOption("goal.autodiscovery")
	if !ok || val != "true" {
		t.Errorf("expected goal.autodiscovery=true, got %q (exists: %v)", val, ok)
	}
	val, ok = cfg.GetGlobalOption("script.max-traversal-depth")
	if !ok || val != "5" {
		t.Errorf("expected script.max-traversal-depth=5, got %q (exists: %v)", val, ok)
	}
}

func TestLoadConfig_WhitespaceOnlyValue(t *testing.T) {
	t.Parallel()
	// "key   " after TrimSpace becomes "key", so SplitN yields only key, value=""
	input := "verbose   "
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	val, ok := cfg.GetGlobalOption("verbose")
	if !ok {
		t.Fatal("expected 'verbose' key to exist")
	}
	if val != "" {
		t.Errorf("expected empty value for whitespace-only trailing, got %q", val)
	}
}

func TestLoadConfig_EmptyValue(t *testing.T) {
	t.Parallel()
	// Explicit key with no value (just the key name on the line)
	input := "editor"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	val, ok := cfg.GetGlobalOption("editor")
	if !ok {
		t.Fatal("expected 'editor' key to exist")
	}
	if val != "" {
		t.Errorf("expected empty value, got %q", val)
	}
}

// =============================================================================
// 3. Long Values
// =============================================================================

func TestLoadConfig_LongValue10KB(t *testing.T) {
	t.Parallel()
	// Build a ~10KB path list value
	var paths []string
	for i := 0; i < 500; i++ {
		paths = append(paths, "/very/long/path/segment/number/"+strconv.Itoa(i))
	}
	longValue := strings.Join(paths, ":")
	if len(longValue) < 10000 {
		t.Fatalf("test setup: expected >= 10KB value, got %d bytes", len(longValue))
	}

	input := "script.paths " + longValue
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error on 10KB value: %v", err)
	}
	got, ok := cfg.GetGlobalOption("script.paths")
	if !ok {
		t.Fatal("expected script.paths key to exist")
	}
	if got != longValue {
		t.Errorf("long value mismatch: expected len=%d, got len=%d", len(longValue), len(got))
	}
}

func TestLoadConfig_ValueWithManyColons(t *testing.T) {
	t.Parallel()
	// Path list with many colons
	parts := make([]string, 100)
	for i := range parts {
		parts[i] = "/path/" + strconv.Itoa(i)
	}
	pathList := strings.Join(parts, ":")

	input := "script.paths " + pathList
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	got, ok := cfg.GetGlobalOption("script.paths")
	if !ok {
		t.Fatal("expected script.paths to exist")
	}
	if got != pathList {
		t.Errorf("path list mismatch: expected %d chars, got %d chars", len(pathList), len(got))
	}
	// Verify it splits correctly
	gotParts := strings.Split(got, ":")
	if len(gotParts) != 100 {
		t.Errorf("expected 100 path segments, got %d", len(gotParts))
	}
}

// =============================================================================
// 4. Non-UTF8 / Binary Content
// =============================================================================

func TestLoadConfig_Latin1Characters(t *testing.T) {
	t.Parallel()
	// Latin-1 characters in the 0x80-0xFF range (not valid UTF-8 on their own
	// but Go strings are byte sequences, so bufio.Scanner reads them).
	latin1Value := "caf\xe9 na\xefve" // café naïve in Latin-1
	input := "editor " + latin1Value
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error on Latin-1: %v", err)
	}
	got, ok := cfg.GetGlobalOption("editor")
	if !ok {
		t.Fatal("expected 'editor' key to exist")
	}
	if got != latin1Value {
		t.Errorf("Latin-1 value mismatch: expected %q, got %q", latin1Value, got)
	}
}

func TestLoadConfig_HighBytesInValue(t *testing.T) {
	t.Parallel()
	// Bytes 0x80, 0xFF in value
	binaryValue := "start\x80middle\xFFend"
	input := "editor " + binaryValue
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error on high bytes: %v", err)
	}
	got, ok := cfg.GetGlobalOption("editor")
	if !ok {
		t.Fatal("expected 'editor' key to exist")
	}
	if got != binaryValue {
		t.Errorf("binary value mismatch: expected %q, got %q", binaryValue, got)
	}
}

// =============================================================================
// 5. Edge Cases in Parsing
// =============================================================================

func TestLoadConfig_KeyOnly_NoValue(t *testing.T) {
	t.Parallel()
	input := "verbose"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	val, ok := cfg.GetGlobalOption("verbose")
	if !ok {
		t.Fatal("expected 'verbose' key to exist with key-only line")
	}
	// Key-only line results in empty value
	if val != "" {
		t.Errorf("expected empty value for key-only line, got %q", val)
	}
}

func TestLoadConfig_CRLFLineEndings(t *testing.T) {
	t.Parallel()
	// Simulate Windows-style line endings (\r\n)
	input := "verbose true\r\ncolor auto\r\neditor vim\r\n"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error on CRLF: %v", err)
	}

	tests := map[string]string{
		"verbose": "true",
		"color":   "auto",
		"editor":  "vim",
	}
	for key, want := range tests {
		got, ok := cfg.GetGlobalOption(key)
		if !ok {
			t.Errorf("CRLF: key %q not found", key)
			continue
		}
		if got != want {
			t.Errorf("CRLF: key %q = %q, want %q", key, got, want)
		}
	}
}

func TestLoadConfig_MultipleSpacesBetweenKeyAndValue(t *testing.T) {
	t.Parallel()
	// SplitN(line, " ", 2) splits on the first space; remaining spaces are
	// part of the value string.
	input := "color    auto"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	got, ok := cfg.GetGlobalOption("color")
	if !ok {
		t.Fatal("expected 'color' key to exist")
	}
	// The parser uses SplitN(" ", 2), so extra leading spaces remain in value
	if got != "   auto" {
		t.Errorf("expected %q, got %q", "   auto", got)
	}
}

func TestLoadConfig_LeadingTrailingSpacesOnLine(t *testing.T) {
	t.Parallel()
	// TrimSpace removes leading/trailing whitespace from the whole line
	input := "   verbose true   "
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	got, ok := cfg.GetGlobalOption("verbose")
	if !ok {
		t.Fatal("expected 'verbose' key to exist after leading/trailing trim")
	}
	if got != "true" {
		t.Errorf("expected 'true', got %q", got)
	}
}

func TestLoadConfig_SectionNameWithSpecialChars(t *testing.T) {
	t.Parallel()
	input := "[my-cmd-123]\npager less\n\n[foo_bar.baz]\nformat json"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}

	// Check my-cmd-123 section
	val, ok := cfg.GetCommandOption("my-cmd-123", "pager")
	if !ok || val != "less" {
		t.Errorf("expected [my-cmd-123] pager=less, got %q (exists: %v)", val, ok)
	}

	// Check foo_bar.baz section
	val, ok = cfg.GetCommandOption("foo_bar.baz", "format")
	if !ok || val != "json" {
		t.Errorf("expected [foo_bar.baz] format=json, got %q (exists: %v)", val, ok)
	}
}

func TestLoadConfig_EmptyLinesAndCommentsInterspersed(t *testing.T) {
	t.Parallel()
	input := `
# comment at start
verbose true

# another comment

color auto
# mid-comment
editor vim

`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	for _, key := range []string{"verbose", "color", "editor"} {
		if _, ok := cfg.GetGlobalOption(key); !ok {
			t.Errorf("expected key %q to exist", key)
		}
	}
}

func TestLoadConfig_CommentLineNotParsedAsKey(t *testing.T) {
	t.Parallel()
	input := "# verbose true\ncolor auto"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	if _, ok := cfg.GetGlobalOption("verbose"); ok {
		t.Error("commented-out 'verbose' should not appear in config")
	}
	if _, ok := cfg.GetGlobalOption("#"); ok {
		t.Error("'#' should not appear as a key")
	}
	val, ok := cfg.GetGlobalOption("color")
	if !ok || val != "auto" {
		t.Errorf("expected color=auto, got %q (exists: %v)", val, ok)
	}
}

func TestLoadConfig_SectionWithEmptyBody(t *testing.T) {
	t.Parallel()
	input := "[help]\n\n[version]\nformat json"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	// help section exists but has no options
	if _, ok := cfg.GetCommandOption("help", "pager"); ok {
		t.Error("expected no pager in empty [help] section")
	}
	// version section has an option
	val, ok := cfg.GetCommandOption("version", "format")
	if !ok || val != "json" {
		t.Errorf("expected version.format=json, got %q (exists: %v)", val, ok)
	}
}

func TestLoadConfig_MultipleSectionsBackToBack(t *testing.T) {
	t.Parallel()
	input := "[help]\npager less\n[version]\nformat json\n[prompt]\ntemplate default"
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}
	checks := map[string]map[string]string{
		"help":    {"pager": "less"},
		"version": {"format": "json"},
		"prompt":  {"template": "default"},
	}
	for section, opts := range checks {
		for key, want := range opts {
			got, ok := cfg.GetCommandOption(section, key)
			if !ok || got != want {
				t.Errorf("[%s] %s: expected %q, got %q (exists: %v)", section, key, want, got, ok)
			}
		}
	}
}

// =============================================================================
// Integration: Validation with edge-case values loaded from reader
// =============================================================================

func TestValidateConfig_IntOverflowFromReader(t *testing.T) {
	t.Parallel()
	// Load a config with an overflowing int for a known TypeInt schema option.
	s := NewSchema()
	s.Register(ConfigOption{Key: "count", Type: TypeInt, Section: ""})

	c := NewConfig()
	c.SetGlobalOption("count", "99999999999999999999")

	issues := ValidateConfig(c, s)
	if len(issues) == 0 {
		t.Fatal("expected validation issue for int overflow, got none")
	}
	if !strings.Contains(issues[0], "expected int") {
		t.Errorf("expected 'expected int' issue, got: %v", issues)
	}
}

func TestValidateConfig_DurationBoundaries(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "timeout", Type: TypeDuration, Section: ""})

	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"zero", "0s", true},
		{"nanosecond", "1ns", true},
		{"large", "876000h", true},
		{"negative", "-5m", true},
		{"bare_zero", "0", true},      // "0" is special-cased valid in ParseDuration
		{"bare_number", "100", false}, // "100" without unit fails ParseDuration
		{"empty_string", "", false},   // empty string fails ParseDuration
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := NewConfig()
			c.SetGlobalOption("timeout", tc.value)
			issues := ValidateConfig(c, s)
			if tc.valid && len(issues) != 0 {
				t.Errorf("expected valid for %q, got issues: %v", tc.value, issues)
			}
			if !tc.valid && len(issues) == 0 {
				t.Errorf("expected issues for %q, got none", tc.value)
			}
		})
	}
}

func TestGetDuration_LargeValue(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("timeout", "876000h")
	got := c.GetDuration("timeout")
	want := 876000 * time.Hour
	if got != want {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestGetDuration_ZeroValue(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("timeout", "0s")
	got := c.GetDuration("timeout")
	if got != 0 {
		t.Errorf("expected 0, got %v", got)
	}
}

func TestGetInt_Boundaries(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("max", strconv.FormatInt(math.MaxInt64, 10))
	c.SetGlobalOption("min", strconv.FormatInt(math.MinInt64, 10))
	c.SetGlobalOption("overflow", "99999999999999999999")

	if got := c.GetInt("max"); got != math.MaxInt64 {
		t.Errorf("expected MaxInt64, got %d", got)
	}
	if got := c.GetInt("min"); got != math.MinInt64 {
		t.Errorf("expected MinInt64, got %d", got)
	}
	// Overflow returns 0 (parse error)
	if got := c.GetInt("overflow"); got != 0 {
		t.Errorf("expected 0 for overflow, got %d", got)
	}
}

// =============================================================================
// Validate special char values pass TypeString/TypePathList validation
// =============================================================================

func TestValidateType_StringAcceptsSpecialChars(t *testing.T) {
	t.Parallel()
	specialValues := []string{
		"",
		"hello world",
		"key=value",
		"/a:/b:/c",
		"hello#world",
		"🚀🎉✨",
		"日本語テスト",
		"caf\xe9 na\xefve",
		strings.Repeat("x", 10000),
		"\t\t\t",
		"a\x00b", // null byte
	}
	for _, v := range specialValues {
		if err := ValidateOptionValue(TypeString, v); err != nil {
			t.Errorf("TypeString should accept %q, got error: %v", v, err)
		}
		if err := ValidateOptionValue(TypePathList, v); err != nil {
			t.Errorf("TypePathList should accept %q, got error: %v", v, err)
		}
	}
}
