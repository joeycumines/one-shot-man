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

// ---------------------------------------------------------------------------
// parseClaudeMuxOption
// ---------------------------------------------------------------------------

func TestParseClaudeMuxOption_StringOptions(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{EnvVars: make(map[string]string)}

	pairs := []struct {
		key, value string
	}{
		{"provider", "myai"},
		{"model", "gpt-4"},
		{"work-dir", "/tmp/work"},
		{"env-profile", "staging"},
		{"pre-spawn-hook", "/path/hook.js"},
		{"provider-command", "/usr/bin/ai"},
		{"mcp-servers", "srv1,srv2"},
	}
	for _, p := range pairs {
		if err := parseClaudeMuxOption(&oc, p.key, p.value); err != nil {
			t.Fatalf("parseClaudeMuxOption(%q, %q): %v", p.key, p.value, err)
		}
	}
	if oc.Provider != "myai" {
		t.Fatalf("Provider = %q; want %q", oc.Provider, "myai")
	}
	if oc.Model != "gpt-4" {
		t.Fatalf("Model = %q; want %q", oc.Model, "gpt-4")
	}
	if oc.WorkDir != "/tmp/work" {
		t.Fatalf("WorkDir = %q; want %q", oc.WorkDir, "/tmp/work")
	}
	if oc.EnvProfile != "staging" {
		t.Fatalf("EnvProfile = %q; want %q", oc.EnvProfile, "staging")
	}
	if oc.PreSpawnHook != "/path/hook.js" {
		t.Fatalf("PreSpawnHook = %q; want %q", oc.PreSpawnHook, "/path/hook.js")
	}
	if oc.ProviderCommand != "/usr/bin/ai" {
		t.Fatalf("ProviderCommand = %q; want %q", oc.ProviderCommand, "/usr/bin/ai")
	}
	if oc.MCPServers != "srv1,srv2" {
		t.Fatalf("MCPServers = %q; want %q", oc.MCPServers, "srv1,srv2")
	}
}

func TestParseClaudeMuxOption_EnvValid(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "env", "API_KEY=secret123"); err != nil {
		t.Fatal(err)
	}
	if oc.EnvVars["API_KEY"] != "secret123" {
		t.Fatalf("EnvVars[API_KEY] = %q; want %q", oc.EnvVars["API_KEY"], "secret123")
	}
}

func TestParseClaudeMuxOption_EnvMissingEquals(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "env", "NOEQUALS"); err == nil {
		t.Fatal("expected error for env without = separator")
	}
}

func TestParseClaudeMuxOption_EnvInheritBool(t *testing.T) {
	t.Parallel()
	// Start with true to avoid zero-value vacuity.
	oc := ClaudeMuxConfig{EnvInherit: true}
	if err := parseClaudeMuxOption(&oc, "env-inherit", "false"); err != nil {
		t.Fatal(err)
	}
	if oc.EnvInherit {
		t.Fatal("EnvInherit should be false after setting to false")
	}
	// Flip back to true.
	if err := parseClaudeMuxOption(&oc, "env-inherit", "true"); err != nil {
		t.Fatal(err)
	}
	if !oc.EnvInherit {
		t.Fatal("EnvInherit should be true after setting to true")
	}
}

func TestParseClaudeMuxOption_EnvInheritInvalid(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "env-inherit", "nah"); err == nil {
		t.Fatal("expected error for invalid boolean")
	}
}

func TestParseClaudeMuxOption_PermissionPolicyValid(t *testing.T) {
	t.Parallel()
	for _, val := range []string{"reject", "ask"} {
		oc := ClaudeMuxConfig{}
		if err := parseClaudeMuxOption(&oc, "permission-policy", val); err != nil {
			t.Fatalf("unexpected error for %q: %v", val, err)
		}
		if oc.PermissionPolicy != val {
			t.Fatalf("PermissionPolicy = %q; want %q", oc.PermissionPolicy, val)
		}
	}
}

func TestParseClaudeMuxOption_PermissionPolicyInvalid(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "permission-policy", "allow"); err == nil {
		t.Fatal("expected error for invalid permission-policy")
	}
}

func TestParseClaudeMuxOption_IntegerOptions(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "rate-limit-backoff-sec", "60"); err != nil {
		t.Fatal(err)
	}
	if oc.RateLimitBackoffSec != 60 {
		t.Fatalf("RateLimitBackoffSec = %d; want 60", oc.RateLimitBackoffSec)
	}
	if err := parseClaudeMuxOption(&oc, "max-agents", "8"); err != nil {
		t.Fatal(err)
	}
	if oc.MaxAgents != 8 {
		t.Fatalf("MaxAgents = %d; want 8", oc.MaxAgents)
	}
	if err := parseClaudeMuxOption(&oc, "pty-rows", "40"); err != nil {
		t.Fatal(err)
	}
	if oc.PTYRows != 40 {
		t.Fatalf("PTYRows = %d; want 40", oc.PTYRows)
	}
	if err := parseClaudeMuxOption(&oc, "pty-cols", "120"); err != nil {
		t.Fatal(err)
	}
	if oc.PTYCols != 120 {
		t.Fatalf("PTYCols = %d; want 120", oc.PTYCols)
	}
}

func TestParseClaudeMuxOption_IntegerInvalid(t *testing.T) {
	t.Parallel()
	intOpts := []string{"rate-limit-backoff-sec", "max-agents", "pty-rows", "pty-cols"}
	for _, opt := range intOpts {
		t.Run(opt, func(t *testing.T) {
			t.Parallel()
			oc := ClaudeMuxConfig{}
			if err := parseClaudeMuxOption(&oc, opt, "notanumber"); err == nil {
				t.Fatalf("expected error for non-integer %s", opt)
			}
		})
	}
}

func TestParseClaudeMuxOption_UnknownOption(t *testing.T) {
	t.Parallel()
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "nonexistent", "value"); err == nil {
		t.Fatal("expected error for unknown claude-mux option")
	}
}

func TestParseClaudeMuxOption_EnvNilMapInit(t *testing.T) {
	t.Parallel()
	// EnvVars starts as nil; parseClaudeMuxOption should auto-init the map.
	oc := ClaudeMuxConfig{}
	if err := parseClaudeMuxOption(&oc, "env", "K=V"); err != nil {
		t.Fatal(err)
	}
	if oc.EnvVars == nil {
		t.Fatal("EnvVars should be initialized")
	}
	if oc.EnvVars["K"] != "V" {
		t.Fatalf("EnvVars[K] = %q; want %q", oc.EnvVars["K"], "V")
	}
}
