package config

import (
	"testing"
)

// ---------------------------------------------------------------------------
// parseBool
// ---------------------------------------------------------------------------

func TestParseBool(t *testing.T) {
	t.Parallel()

	truthy := []string{"true", "TRUE", "True", "1", "yes", "YES", "Yes", "on", "ON", "On"}
	for _, s := range truthy {
		t.Run("truthy/"+s, func(t *testing.T) {
			t.Parallel()
			got, err := parseBool(s)
			if err != nil {
				t.Fatalf("parseBool(%q): unexpected error: %v", s, err)
			}
			if !got {
				t.Fatalf("parseBool(%q) = false; want true", s)
			}
		})
	}

	falsy := []string{"false", "FALSE", "False", "0", "no", "NO", "No", "off", "OFF", "Off"}
	for _, s := range falsy {
		t.Run("falsy/"+s, func(t *testing.T) {
			t.Parallel()
			got, err := parseBool(s)
			if err != nil {
				t.Fatalf("parseBool(%q): unexpected error: %v", s, err)
			}
			if got {
				t.Fatalf("parseBool(%q) = true; want false", s)
			}
		})
	}

	invalid := []string{"", " ", "maybe", "2", "-1", "t", "f", "y", "n", "oui"}
	for _, s := range invalid {
		t.Run("invalid/"+s, func(t *testing.T) {
			t.Parallel()
			_, err := parseBool(s)
			if err == nil {
				t.Fatalf("parseBool(%q): expected error, got nil", s)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseHotSnippetLine
// ---------------------------------------------------------------------------

func TestParseHotSnippetLine_EmptyName(t *testing.T) {
	t.Parallel()
	var snippets []HotSnippet
	err := parseHotSnippetLine(&snippets, "", "some text")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestParseHotSnippetLine_NewSnippet(t *testing.T) {
	t.Parallel()
	var snippets []HotSnippet
	if err := parseHotSnippetLine(&snippets, "greet", "hello world"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(snippets))
	}
	if snippets[0].Name != "greet" {
		t.Fatalf("name = %q; want %q", snippets[0].Name, "greet")
	}
	if snippets[0].Text != "hello world" {
		t.Fatalf("text = %q; want %q", snippets[0].Text, "hello world")
	}
}

func TestParseHotSnippetLine_LiteralNewlineConversion(t *testing.T) {
	t.Parallel()
	var snippets []HotSnippet
	if err := parseHotSnippetLine(&snippets, "multi", `line1\nline2\nline3`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "line1\nline2\nline3"
	if snippets[0].Text != want {
		t.Fatalf("text = %q; want %q", snippets[0].Text, want)
	}
}

func TestParseHotSnippetLine_Description(t *testing.T) {
	t.Parallel()
	snippets := []HotSnippet{{Name: "greet", Text: "hello"}}
	if err := parseHotSnippetLine(&snippets, "greet.description", "A greeting snippet"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snippets[0].Description != "A greeting snippet" {
		t.Fatalf("description = %q; want %q", snippets[0].Description, "A greeting snippet")
	}
}

func TestParseHotSnippetLine_DescriptionMissing(t *testing.T) {
	t.Parallel()
	var snippets []HotSnippet
	err := parseHotSnippetLine(&snippets, "nosuch.description", "desc")
	if err == nil {
		t.Fatal("expected error for description of missing snippet")
	}
}

func TestParseHotSnippetLine_MultipleSnippets(t *testing.T) {
	t.Parallel()
	var snippets []HotSnippet
	if err := parseHotSnippetLine(&snippets, "a", "text-a"); err != nil {
		t.Fatal(err)
	}
	if err := parseHotSnippetLine(&snippets, "b", "text-b"); err != nil {
		t.Fatal(err)
	}
	if len(snippets) != 2 {
		t.Fatalf("expected 2 snippets, got %d", len(snippets))
	}
	if snippets[0].Name != "a" || snippets[1].Name != "b" {
		t.Fatalf("unexpected snippet names: %v", snippets)
	}
	if snippets[0].Text != "text-a" {
		t.Fatalf("snippet[0].Text = %q; want %q", snippets[0].Text, "text-a")
	}
	if snippets[1].Text != "text-b" {
		t.Fatalf("snippet[1].Text = %q; want %q", snippets[1].Text, "text-b")
	}
}

func TestParseHotSnippetLine_DescriptionTargetsLastMatch(t *testing.T) {
	t.Parallel()
	snippets := []HotSnippet{
		{Name: "dup", Text: "first"},
		{Name: "dup", Text: "second"},
	}
	if err := parseHotSnippetLine(&snippets, "dup.description", "desc"); err != nil {
		t.Fatal(err)
	}
	// Should set on the LAST snippet named "dup" (index 1)
	if snippets[0].Description != "" {
		t.Fatalf("first snippet should have no description, got %q", snippets[0].Description)
	}
	if snippets[1].Description != "desc" {
		t.Fatalf("second snippet description = %q; want %q", snippets[1].Description, "desc")
	}
}

// ---------------------------------------------------------------------------
// parseSessionOption
// ---------------------------------------------------------------------------

func TestParseSessionOption_ValidOptions(t *testing.T) {
	t.Parallel()

	sc := SessionConfig{}
	if err := parseSessionOption(&sc, "maxAgeDays", "30"); err != nil {
		t.Fatal(err)
	}
	if sc.MaxAgeDays != 30 {
		t.Fatalf("MaxAgeDays = %d; want 30", sc.MaxAgeDays)
	}
	if err := parseSessionOption(&sc, "maxCount", "50"); err != nil {
		t.Fatal(err)
	}
	if sc.MaxCount != 50 {
		t.Fatalf("MaxCount = %d; want 50", sc.MaxCount)
	}
	if err := parseSessionOption(&sc, "maxSizeMB", "200"); err != nil {
		t.Fatal(err)
	}
	if sc.MaxSizeMB != 200 {
		t.Fatalf("MaxSizeMB = %d; want 200", sc.MaxSizeMB)
	}
	// Test autoCleanupEnabled with "true" to avoid zero-value vacuity.
	if err := parseSessionOption(&sc, "autoCleanupEnabled", "true"); err != nil {
		t.Fatal(err)
	}
	if !sc.AutoCleanupEnabled {
		t.Fatal("AutoCleanupEnabled should be true")
	}
	// Now flip to false and verify the field actually changed.
	if err := parseSessionOption(&sc, "autoCleanupEnabled", "false"); err != nil {
		t.Fatal(err)
	}
	if sc.AutoCleanupEnabled {
		t.Fatal("AutoCleanupEnabled should be false after setting to false")
	}
	if err := parseSessionOption(&sc, "cleanupIntervalHours", "12"); err != nil {
		t.Fatal(err)
	}
	if sc.CleanupIntervalHours != 12 {
		t.Fatalf("CleanupIntervalHours = %d; want 12", sc.CleanupIntervalHours)
	}
}

func TestParseSessionOption_NegativeValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, value string
	}{
		{"maxAgeDays", "-1"},
		{"maxCount", "-5"},
		{"maxSizeMB", "-100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sc := SessionConfig{}
			err := parseSessionOption(&sc, tc.name, tc.value)
			if err == nil {
				t.Fatalf("expected error for negative %s", tc.name)
			}
		})
	}
}

func TestParseSessionOption_InvalidInteger(t *testing.T) {
	t.Parallel()
	sc := SessionConfig{}
	if err := parseSessionOption(&sc, "maxAgeDays", "abc"); err == nil {
		t.Fatal("expected error for non-integer value")
	}
}

func TestParseSessionOption_CleanupIntervalZero(t *testing.T) {
	t.Parallel()
	sc := SessionConfig{}
	if err := parseSessionOption(&sc, "cleanupIntervalHours", "0"); err == nil {
		t.Fatal("expected error for cleanupIntervalHours < 1")
	}
}

func TestParseSessionOption_UnknownOption(t *testing.T) {
	t.Parallel()
	sc := SessionConfig{}
	if err := parseSessionOption(&sc, "unknownField", "value"); err == nil {
		t.Fatal("expected error for unknown session option")
	}
}

func TestParseSessionOption_InvalidBool(t *testing.T) {
	t.Parallel()
	sc := SessionConfig{}
	if err := parseSessionOption(&sc, "autoCleanupEnabled", "maybe"); err == nil {
		t.Fatal("expected error for invalid boolean in autoCleanupEnabled")
	}
}
