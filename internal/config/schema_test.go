package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// --- ConfigSchema tests ---

func TestNewSchema(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	if s == nil {
		t.Fatal("NewSchema returned nil")
	}
	if len(s.GlobalOptions()) != 0 {
		t.Fatalf("expected empty global options, got %d", len(s.GlobalOptions()))
	}
}

func TestSchemaRegister(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})
	s.Register(ConfigOption{Key: "pager", Type: TypeString, Section: "help"})

	if !s.IsKnown("", "verbose") {
		t.Error("expected 'verbose' to be known globally")
	}
	if !s.IsKnown("help", "pager") {
		t.Error("expected 'pager' to be known in [help]")
	}
	if s.IsKnown("", "nonexistent") {
		t.Error("expected 'nonexistent' to not be known")
	}
}

func TestSchemaRegisterAll(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "a", Section: ""},
		{Key: "b", Section: ""},
		{Key: "c", Section: "sec"},
	})
	if len(s.GlobalOptions()) != 2 {
		t.Fatalf("expected 2 global options, got %d", len(s.GlobalOptions()))
	}
	if len(s.SectionOptions("sec")) != 1 {
		t.Fatalf("expected 1 section option, got %d", len(s.SectionOptions("sec")))
	}
}

func TestSchemaLookup(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto", Section: ""})
	s.Register(ConfigOption{Key: "pager", Type: TypeString, Section: "help"})

	opt := s.Lookup("", "color")
	if opt == nil || opt.Key != "color" || opt.Default != "auto" {
		t.Fatalf("unexpected Lookup result: %+v", opt)
	}

	opt = s.Lookup("help", "pager")
	if opt == nil || opt.Key != "pager" {
		t.Fatalf("unexpected Lookup result for help.pager: %+v", opt)
	}

	opt = s.Lookup("", "nonexistent")
	if opt != nil {
		t.Fatalf("expected nil for nonexistent, got %+v", opt)
	}

	opt = s.Lookup("nosection", "nokey")
	if opt != nil {
		t.Fatalf("expected nil for nosection.nokey, got %+v", opt)
	}
}

func TestSchemaIsKnown_GlobalFallbackInSection(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})
	s.Register(ConfigOption{Key: "pager", Type: TypeString, Section: "help"})

	// Global key should be known in command sections (fallback).
	if !s.IsKnown("help", "verbose") {
		t.Error("global option 'verbose' should be known in [help] section")
	}
	// Section-specific key known in its section.
	if !s.IsKnown("help", "pager") {
		t.Error("section option 'pager' should be known in [help]")
	}
	// Section key NOT known in a different section (unless also global).
	if s.IsKnown("version", "pager") {
		t.Error("section option 'pager' should not be known in [version]")
	}
}

func TestSchemaGlobalOptions(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "a", Section: ""},
		{Key: "b", Section: "cmd"},
		{Key: "c", Section: ""},
	})
	globals := s.GlobalOptions()
	if len(globals) != 2 {
		t.Fatalf("expected 2 globals, got %d", len(globals))
	}
	keys := []string{globals[0].Key, globals[1].Key}
	if keys[0] != "a" || keys[1] != "c" {
		t.Fatalf("unexpected global keys: %v", keys)
	}
}

func TestSchemaSectionOptions(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "a", Section: "help"},
		{Key: "b", Section: "version"},
		{Key: "c", Section: "help"},
	})
	helpOpts := s.SectionOptions("help")
	if len(helpOpts) != 2 {
		t.Fatalf("expected 2 [help] opts, got %d", len(helpOpts))
	}
	versionOpts := s.SectionOptions("version")
	if len(versionOpts) != 1 {
		t.Fatalf("expected 1 [version] opt, got %d", len(versionOpts))
	}
	emptyOpts := s.SectionOptions("nonexistent")
	if len(emptyOpts) != 0 {
		t.Fatalf("expected 0 opts for nonexistent, got %d", len(emptyOpts))
	}
}

func TestSchemaSections(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "a", Section: ""},
		{Key: "b", Section: "help"},
		{Key: "c", Section: "version"},
		{Key: "d", Section: "help"},
	})
	sections := s.Sections()
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d: %v", len(sections), sections)
	}
	if sections[0] != "help" || sections[1] != "version" {
		t.Fatalf("expected [help, version], got %v", sections)
	}
}

func TestSchemaDuplicateOverwrites(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "color", Type: TypeBool, Section: ""})
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto", Section: ""})

	opt := s.Lookup("", "color")
	if opt == nil || opt.Type != TypeString || opt.Default != "auto" {
		t.Fatalf("expected last registration to win, got %+v", opt)
	}
}

// --- ValidateConfig tests ---

func TestValidateConfig_AllValid(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "verbose", Type: TypeBool, Section: ""},
		{Key: "timeout", Type: TypeDuration, Section: ""},
		{Key: "pager", Type: TypeString, Section: "help"},
	})
	c := NewConfig()
	c.SetGlobalOption("verbose", "true")
	c.SetGlobalOption("timeout", "30s")
	c.SetCommandOption("help", "pager", "less")

	issues := ValidateConfig(c, s)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %v", issues)
	}
}

func TestValidateConfig_UnknownGlobal(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})

	c := NewConfig()
	c.SetGlobalOption("verbos", "true") // typo

	issues := ValidateConfig(c, s)
	if len(issues) != 1 || !strings.Contains(issues[0], "unknown global option") {
		t.Fatalf("expected 1 unknown global issue, got: %v", issues)
	}
}

func TestValidateConfig_UnknownCommand(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "pager", Type: TypeString, Section: "help"})

	c := NewConfig()
	c.SetCommandOption("help", "pagr", "less") // typo

	issues := ValidateConfig(c, s)
	if len(issues) != 1 || !strings.Contains(issues[0], "unknown option for command") {
		t.Fatalf("expected 1 unknown command issue, got: %v", issues)
	}
}

func TestValidateConfig_TypeMismatch(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})
	s.Register(ConfigOption{Key: "timeout", Type: TypeDuration, Section: ""})
	s.Register(ConfigOption{Key: "count", Type: TypeInt, Section: ""})

	c := NewConfig()
	c.SetGlobalOption("verbose", "maybe")   // invalid bool
	c.SetGlobalOption("timeout", "notaval") // invalid duration
	c.SetGlobalOption("count", "abc")       // invalid int

	issues := ValidateConfig(c, s)
	if len(issues) != 3 {
		t.Fatalf("expected 3 type issues, got %d: %v", len(issues), issues)
	}
}

func TestValidateConfig_SectionTypeValidation(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "limit", Type: TypeInt, Section: "help"})

	c := NewConfig()
	c.SetCommandOption("help", "limit", "abc")

	issues := ValidateConfig(c, s)
	if len(issues) != 1 || !strings.Contains(issues[0], "expected int") {
		t.Fatalf("expected 1 type validation issue, got: %v", issues)
	}
}

func TestValidateConfig_GlobalInSection(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})

	c := NewConfig()
	c.SetCommandOption("help", "verbose", "true")

	// Global options in sections should be allowed.
	issues := ValidateConfig(c, s)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for global key in section, got: %v", issues)
	}
}

func TestValidateConfig_GlobalInSectionTypeCheck(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})

	c := NewConfig()
	c.SetCommandOption("help", "verbose", "maybe")

	issues := ValidateConfig(c, s)
	if len(issues) != 1 || !strings.Contains(issues[0], "expected bool") {
		t.Fatalf("expected bool type issue, got: %v", issues)
	}
}

func TestValidateConfig_EmptyConfig(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()
	c := NewConfig()

	issues := ValidateConfig(c, s)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for empty config, got: %v", issues)
	}
}

func TestValidateConfig_StringAndPathListAcceptAnything(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "name", Type: TypeString, Section: ""})
	s.Register(ConfigOption{Key: "paths", Type: TypePathList, Section: ""})

	c := NewConfig()
	c.SetGlobalOption("name", "any value at all!!")
	c.SetGlobalOption("paths", "/a:/b:/c")

	issues := ValidateConfig(c, s)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for string/path-list, got: %v", issues)
	}
}

// --- Typed getter tests ---

func TestGetString(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("color", "auto")

	if v := c.GetString("color"); v != "auto" {
		t.Fatalf("expected auto, got %q", v)
	}
	if v := c.GetString("nonexistent"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestGetStringDefault(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("color", "auto")

	if v := c.GetStringDefault("color", "never"); v != "auto" {
		t.Fatalf("expected auto, got %q", v)
	}
	if v := c.GetStringDefault("nonexistent", "fallback"); v != "fallback" {
		t.Fatalf("expected fallback, got %q", v)
	}
}

func TestGetBool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
		{"yes", true},
		{"no", false},
		{"on", true},
		{"off", false},
		{"TRUE", true},
		{"invalid", false},
	}
	for _, tc := range tests {
		c := NewConfig()
		c.SetGlobalOption("flag", tc.value)
		if got := c.GetBool("flag"); got != tc.expected {
			t.Errorf("GetBool(%q) = %v, want %v", tc.value, got, tc.expected)
		}
	}

	// Not set should return false.
	c := NewConfig()
	if c.GetBool("notset") != false {
		t.Error("expected false for unset key")
	}
}

func TestGetInt(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("count", "42")
	c.SetGlobalOption("bad", "abc")

	if v := c.GetInt("count"); v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
	if v := c.GetInt("bad"); v != 0 {
		t.Fatalf("expected 0 for bad int, got %d", v)
	}
	if v := c.GetInt("notset"); v != 0 {
		t.Fatalf("expected 0 for unset key, got %d", v)
	}
}

func TestGetDuration(t *testing.T) {
	t.Parallel()
	c := NewConfig()
	c.SetGlobalOption("timeout", "30s")
	c.SetGlobalOption("bad", "notduration")

	if v := c.GetDuration("timeout"); v != 30*time.Second {
		t.Fatalf("expected 30s, got %v", v)
	}
	if v := c.GetDuration("bad"); v != 0 {
		t.Fatalf("expected 0 for bad duration, got %v", v)
	}
	if v := c.GetDuration("notset"); v != 0 {
		t.Fatalf("expected 0 for unset key, got %v", v)
	}
}

func TestGetWithEnv(t *testing.T) {
	dir := t.TempDir() // avoid parallel due to Setenv
	_ = dir

	c := NewConfig()
	c.SetGlobalOption("editor", "vim")

	// Env not set: falls back to config.
	if v := c.GetWithEnv("editor", "OSM_TEST_EDITOR_XYZ"); v != "vim" {
		t.Fatalf("expected vim, got %q", v)
	}

	// Env set: takes precedence.
	t.Setenv("OSM_TEST_EDITOR_XYZ", "nano")
	if v := c.GetWithEnv("editor", "OSM_TEST_EDITOR_XYZ"); v != "nano" {
		t.Fatalf("expected nano from env, got %q", v)
	}

	// Env set to empty string: still takes precedence.
	t.Setenv("OSM_TEST_EDITOR_XYZ", "")
	if v := c.GetWithEnv("editor", "OSM_TEST_EDITOR_XYZ"); v != "" {
		t.Fatalf("expected empty from env, got %q", v)
	}

	// Empty envVar means no env check.
	if v := c.GetWithEnv("editor", ""); v != "vim" {
		t.Fatalf("expected vim with empty envVar, got %q", v)
	}

	// Env set but for a different key; key not in config.
	os.Unsetenv("OSM_TEST_EDITOR_XYZ")
	if v := c.GetWithEnv("nonexistent", "OSM_TEST_EDITOR_XYZ"); v != "" {
		t.Fatalf("expected empty for unset env and unset config, got %q", v)
	}
}

// --- DefaultSchema tests ---

func TestDefaultSchema_ContainsAllLegacyGlobalOptions(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	// These are the original knownGlobalOptions keys that must be present.
	legacyGlobals := []string{
		"verbose", "color", "pager", "format", "timeout",
		"session.id", "output", "editor", "debug", "quiet",
	}
	for _, key := range legacyGlobals {
		if !s.IsKnown("", key) {
			t.Errorf("legacy global option %q not in DefaultSchema", key)
		}
	}
}

func TestDefaultSchema_ContainsAllLegacyCommandOptions(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	legacyCommands := map[string][]string{
		"help":    {"pager", "format", "output"},
		"version": {"format", "output"},
		"prompt":  {"template", "output", "editor", "add-context"},
		"session": {"list", "delete", "export", "import"},
	}
	for section, keys := range legacyCommands {
		for _, key := range keys {
			if !s.IsKnown(section, key) {
				t.Errorf("legacy command option [%s] %q not in DefaultSchema", section, key)
			}
		}
	}
}

func TestDefaultSchema_ContainsDiscoveryOptions(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	discoveryKeys := []string{
		"script.autodiscovery", "script.git-traversal",
		"script.max-traversal-depth", "script.paths",
		"script.path-patterns", "script.disable-standard-paths",
		"script.debug-discovery", "script.module-paths",
		"goal.autodiscovery", "goal.disable-standard-paths",
		"goal.max-traversal-depth", "goal.paths",
		"goal.path-patterns", "goal.debug-discovery",
	}
	for _, key := range discoveryKeys {
		if !s.IsKnown("", key) {
			t.Errorf("discovery option %q not in DefaultSchema", key)
		}
	}
}

func TestDefaultSchema_ContainsOrchestratorOptions(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	// Global orchestrator options
	orchestratorGlobals := []string{
		"orchestrator.provider",
		"orchestrator.model",
		"orchestrator.work-dir",
		"orchestrator.env-inherit",
		"orchestrator.env-profile",
		"orchestrator.pre-spawn-hook",
		"orchestrator.permission-policy",
		"orchestrator.rate-limit-backoff-sec",
		"orchestrator.max-agents",
		"orchestrator.pty-rows",
		"orchestrator.pty-cols",
		"orchestrator.provider-command",
		"orchestrator.mcp-servers",
	}
	for _, key := range orchestratorGlobals {
		if !s.IsKnown("", key) {
			t.Errorf("orchestrator global option %q not in DefaultSchema", key)
		}
	}

	// Section options for [orchestrator]
	orchestratorSection := []string{
		"provider", "model", "work-dir", "env-inherit", "env",
		"env-profile", "pre-spawn-hook", "permission-policy",
		"rate-limit-backoff-sec", "max-agents", "pty-rows",
		"pty-cols", "provider-command", "mcp-servers",
	}
	for _, key := range orchestratorSection {
		if !s.IsKnown("orchestrator", key) {
			t.Errorf("orchestrator section option %q not in DefaultSchema", key)
		}
	}
}

func TestDefaultSchema_OrchestratorOptionTypes(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	checks := map[string]OptionType{
		"orchestrator.env-inherit":            TypeBool,
		"orchestrator.rate-limit-backoff-sec": TypeInt,
		"orchestrator.max-agents":             TypeInt,
		"orchestrator.pty-rows":               TypeInt,
		"orchestrator.pty-cols":               TypeInt,
		"orchestrator.provider":               TypeString,
		"orchestrator.model":                  TypeString,
	}
	for key, wantType := range checks {
		opt := s.Lookup("", key)
		if opt == nil {
			t.Errorf("option %q not found", key)
			continue
		}
		if opt.Type != wantType {
			t.Errorf("option %q type = %q, want %q", key, opt.Type, wantType)
		}
	}
}

func TestDefaultSchema_OptionTypes(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	// Spot-check types.
	checks := map[string]OptionType{
		"verbose":                    TypeBool,
		"color":                      TypeString,
		"timeout":                    TypeDuration,
		"script.max-traversal-depth": TypeInt,
		"script.paths":               TypePathList,
	}
	for key, wantType := range checks {
		opt := s.Lookup("", key)
		if opt == nil {
			t.Errorf("option %q not found", key)
			continue
		}
		if opt.Type != wantType {
			t.Errorf("option %q type = %q, want %q", key, opt.Type, wantType)
		}
	}
}

// --- FormatHelp tests ---

func TestFormatHelp(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "verbose", Type: TypeBool, Default: "false", Description: "Verbose output", Section: ""},
		{Key: "pager", Type: TypeString, Description: "Pager program", Section: "help"},
	})

	help := s.FormatHelp()
	if !strings.Contains(help, "Global Options:") {
		t.Error("expected 'Global Options:' header in help")
	}
	if !strings.Contains(help, "verbose") {
		t.Error("expected 'verbose' in help")
	}
	if !strings.Contains(help, "type: bool") {
		t.Error("expected 'type: bool' in help")
	}
	if !strings.Contains(help, "default: false") {
		t.Error("expected 'default: false' in help")
	}
	if !strings.Contains(help, "[help] Options:") {
		t.Error("expected '[help] Options:' header in help")
	}
	if !strings.Contains(help, "pager") {
		t.Error("expected 'pager' in help")
	}
}

func TestFormatHelp_WithEnvVar(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{
		Key: "editor", Type: TypeString, EnvVar: "EDITOR",
		Description: "Editor program", Section: "",
	})

	help := s.FormatHelp()
	if !strings.Contains(help, "env: EDITOR") {
		t.Errorf("expected 'env: EDITOR' in help, got:\n%s", help)
	}
}

func TestFormatHelp_Empty(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	help := s.FormatHelp()
	if help != "" {
		t.Fatalf("expected empty help for empty schema, got:\n%s", help)
	}
}

// --- validateType tests ---

func TestValidateType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ   OptionType
		value string
		ok    bool
	}{
		{TypeString, "anything", true},
		{TypeString, "", true},
		{TypePathList, "/a:/b", true},
		{"", "anything", true}, // empty type = string

		{TypeBool, "true", true},
		{TypeBool, "false", true},
		{TypeBool, "yes", true},
		{TypeBool, "no", true},
		{TypeBool, "1", true},
		{TypeBool, "0", true},
		{TypeBool, "on", true},
		{TypeBool, "off", true},
		{TypeBool, "maybe", false},
		{TypeBool, "", false},

		{TypeInt, "42", true},
		{TypeInt, "-1", true},
		{TypeInt, "0", true},
		{TypeInt, "abc", false},
		{TypeInt, "3.14", false},

		{TypeDuration, "30s", true},
		{TypeDuration, "5m", true},
		{TypeDuration, "1h30m", true},
		{TypeDuration, "abc", false},
		{TypeDuration, "30", false},
	}
	for _, tc := range tests {
		err := validateType(tc.typ, tc.value)
		if tc.ok && err != nil {
			t.Errorf("validateType(%q, %q): unexpected error: %v", tc.typ, tc.value, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("validateType(%q, %q): expected error, got nil", tc.typ, tc.value)
		}
	}
}

func TestValidateType_UnknownType(t *testing.T) {
	t.Parallel()
	err := validateType("foobar", "anything")
	if err == nil || !strings.Contains(err.Error(), "unknown option type") {
		t.Fatalf("expected unknown type error, got: %v", err)
	}
}

// --- Schema-aware config loading integration test ---

func TestLoadAndValidateWithSchema(t *testing.T) {
	t.Parallel()
	configContent := `verbose true
color auto
script.autodiscovery true
script.max-traversal-depth 5
timeout 30s

[help]
pager less`

	cfg, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	s := DefaultSchema()
	issues := ValidateConfig(cfg, s)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got: %v", issues)
	}
}

func TestLoadAndValidateWithSchema_InvalidTypes(t *testing.T) {
	t.Parallel()
	configContent := `verbose notbool
script.max-traversal-depth abc`

	cfg, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	s := DefaultSchema()
	issues := ValidateConfig(cfg, s)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d: %v", len(issues), issues)
	}
}

// --- New schema-aware options: discovery keys no longer produce warnings ---

func TestDiscoveryKeysNoLongerWarn(t *testing.T) {
	t.Parallel()
	configContent := `script.autodiscovery true
script.git-traversal false
script.max-traversal-depth 10
script.paths /usr/local/share/osm/scripts
script.path-patterns scripts
script.disable-standard-paths false
script.debug-discovery false
script.module-paths /my/modules
goal.autodiscovery true
goal.disable-standard-paths false
goal.max-traversal-depth 10
goal.paths /my/goals
goal.path-patterns osm-goals,goals
goal.debug-discovery false`

	cfg, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.HasWarnings() {
		t.Errorf("expected no warnings for discovery keys, got: %v", cfg.GetWarnings())
	}
}

func TestSyncKeysNoLongerWarn(t *testing.T) {
	t.Parallel()
	configContent := `sync.repository https://github.com/user/config.git
sync.auto-pull false
sync.local-path /home/user/.osm-sync`

	cfg, err := LoadFromReader(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.HasWarnings() {
		t.Errorf("expected no warnings for sync keys, got: %v", cfg.GetWarnings())
	}
}

// --- ValidateConfig via schema (was Validate method, now ValidateConfig only) ---

func TestValidateConfigViaSchema(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Section: ""})

	c := NewConfig()
	c.SetGlobalOption("verbose", "maybe") // invalid bool

	issues := ValidateConfig(c, s)
	if len(issues) != 1 || !strings.Contains(issues[0], "expected bool") {
		t.Fatalf("expected 1 bool type issue, got: %v", issues)
	}

	// Valid config produces no issues.
	c2 := NewConfig()
	c2.SetGlobalOption("verbose", "true")
	if iss := ValidateConfig(c2, s); len(iss) != 0 {
		t.Fatalf("expected no issues, got: %v", iss)
	}
}

// --- FormatHelp tests (DumpHelp alias removed) ---

func TestFormatHelpContainsAllDetails(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{
		Key: "verbose", Type: TypeBool, Default: "false",
		Description: "Verbose output", Section: "",
	})
	s.Register(ConfigOption{
		Key: "pager", Type: TypeString,
		Description: "Pager program", Section: "help",
	})

	help := s.FormatHelp()
	if !strings.Contains(help, "verbose") {
		t.Error("expected 'verbose' in FormatHelp output")
	}
	if !strings.Contains(help, "[help] Options:") {
		t.Error("expected '[help] Options:' in FormatHelp output")
	}
}

// --- Schema.Resolve tests ---

func TestSchemaResolve(t *testing.T) {
	s := NewSchema()
	s.Register(ConfigOption{
		Key: "editor", Type: TypeString, Default: "vi",
		Description: "Editor", EnvVar: "OSM_TEST_RESOLVE_EDITOR",
	})
	s.Register(ConfigOption{
		Key: "color", Type: TypeString, Default: "auto",
		Description: "Color mode",
	})

	c := NewConfig()
	c.SetGlobalOption("editor", "vim")
	c.SetGlobalOption("color", "always")

	// Config value takes effect when env var is not set.
	if v := s.Resolve(c, "editor"); v != "vim" {
		t.Fatalf("expected vim from config, got %q", v)
	}

	// Env var overrides config.
	t.Setenv("OSM_TEST_RESOLVE_EDITOR", "nano")
	if v := s.Resolve(c, "editor"); v != "nano" {
		t.Fatalf("expected nano from env, got %q", v)
	}

	// Env var set to empty still overrides.
	t.Setenv("OSM_TEST_RESOLVE_EDITOR", "")
	if v := s.Resolve(c, "editor"); v != "" {
		t.Fatalf("expected empty from env, got %q", v)
	}

	// Option without EnvVar: just config.
	if v := s.Resolve(c, "color"); v != "always" {
		t.Fatalf("expected always from config, got %q", v)
	}

	// Unset key falls back to schema default.
	c2 := NewConfig()
	os.Unsetenv("OSM_TEST_RESOLVE_EDITOR")
	if v := s.Resolve(c2, "editor"); v != "vi" {
		t.Fatalf("expected vi default, got %q", v)
	}

	// Unknown key returns empty.
	if v := s.Resolve(c2, "nonexistent"); v != "" {
		t.Fatalf("expected empty for unknown key, got %q", v)
	}
}

// --- GlobalOptions returns copies ---

func TestGlobalOptionsReturnsCopy(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "a", Section: ""})

	opts := s.GlobalOptions()
	opts[0].Key = "modified"

	// Original should not be affected.
	original := s.GlobalOptions()
	if original[0].Key != "a" {
		t.Fatal("GlobalOptions() should return a copy, but original was modified")
	}
}

// --- T119: ResolveAll and ResolveDiff tests ---

func TestResolveAll_DefaultsOnly(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto"})

	c := NewConfig()
	resolved := s.ResolveAll(c)

	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved options, got %d", len(resolved))
	}
	for _, ro := range resolved {
		if ro.Source != SourceDefault {
			t.Errorf("expected source=default for %q, got %q", ro.Key, ro.Source)
		}
	}
	if resolved[0].Key != "verbose" || resolved[0].Value != "false" {
		t.Errorf("expected verbose=false, got %s=%s", resolved[0].Key, resolved[0].Value)
	}
	if resolved[1].Key != "color" || resolved[1].Value != "auto" {
		t.Errorf("expected color=auto, got %s=%s", resolved[1].Key, resolved[1].Value)
	}
}

func TestResolveAll_ConfigOverride(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto"})

	c := NewConfig()
	c.SetGlobalOption("color", "never")

	resolved := s.ResolveAll(c)

	// verbose should be default
	if resolved[0].Source != SourceDefault {
		t.Errorf("expected default for verbose, got %q", resolved[0].Source)
	}
	// color should be config
	if resolved[1].Source != SourceConfig {
		t.Errorf("expected config for color, got %q", resolved[1].Source)
	}
	if resolved[1].Value != "never" {
		t.Errorf("expected color=never, got %q", resolved[1].Value)
	}
}

func TestResolveAll_EnvOverride(t *testing.T) {
	s := NewSchema()
	s.Register(ConfigOption{Key: "editor", Type: TypeString, Default: "vi", EnvVar: "OSM_TEST_RESOLVE_ALL_EDITOR"})

	c := NewConfig()
	c.SetGlobalOption("editor", "vim")

	t.Setenv("OSM_TEST_RESOLVE_ALL_EDITOR", "nano")

	resolved := s.ResolveAll(c)
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved option, got %d", len(resolved))
	}
	if resolved[0].Source != SourceEnv {
		t.Errorf("expected env source, got %q", resolved[0].Source)
	}
	if resolved[0].Value != "nano" {
		t.Errorf("expected nano, got %q", resolved[0].Value)
	}
}

func TestResolveAll_EnvPrecedenceOverConfig(t *testing.T) {
	s := NewSchema()
	s.Register(ConfigOption{Key: "level", Type: TypeString, Default: "info", EnvVar: "OSM_TEST_RESOLVE_LEVEL"})

	c := NewConfig()
	c.SetGlobalOption("level", "warn")

	t.Setenv("OSM_TEST_RESOLVE_LEVEL", "debug")

	resolved := s.ResolveAll(c)
	if resolved[0].Source != SourceEnv {
		t.Errorf("expected env source when both config and env are set, got %q", resolved[0].Source)
	}
	if resolved[0].Value != "debug" {
		t.Errorf("expected debug from env, got %q", resolved[0].Value)
	}
}

func TestResolveAll_SkipsCommandSectionOptions(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})
	s.Register(ConfigOption{Key: "pager", Type: TypeString, Section: "help"})

	c := NewConfig()
	resolved := s.ResolveAll(c)
	// Only global option should appear
	if len(resolved) != 1 {
		t.Fatalf("expected 1 resolved option (global only), got %d", len(resolved))
	}
	if resolved[0].Key != "verbose" {
		t.Errorf("expected verbose, got %q", resolved[0].Key)
	}
}

func TestResolveDiff_AllDefaults(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto"})

	c := NewConfig()
	diff := s.ResolveDiff(c)
	if len(diff) != 0 {
		t.Fatalf("expected empty diff for all defaults, got %d entries", len(diff))
	}
}

func TestResolveDiff_ConfigOverrides(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto"})
	s.Register(ConfigOption{Key: "pager", Type: TypeString, Default: ""})

	c := NewConfig()
	c.SetGlobalOption("color", "never")

	diff := s.ResolveDiff(c)
	if len(diff) != 1 {
		t.Fatalf("expected 1 diff entry, got %d", len(diff))
	}
	if diff[0].Key != "color" || diff[0].Value != "never" || diff[0].Source != SourceConfig {
		t.Errorf("unexpected diff entry: %+v", diff[0])
	}
}

func TestResolveDiff_EnvOverrides(t *testing.T) {
	s := NewSchema()
	s.Register(ConfigOption{Key: "session.id", Type: TypeString, Default: "", EnvVar: "OSM_TEST_RESOLVE_DIFF_SID"})
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})

	c := NewConfig()
	t.Setenv("OSM_TEST_RESOLVE_DIFF_SID", "my-session")

	diff := s.ResolveDiff(c)
	if len(diff) != 1 {
		t.Fatalf("expected 1 diff entry, got %d", len(diff))
	}
	if diff[0].Key != "session.id" || diff[0].Source != SourceEnv {
		t.Errorf("unexpected diff entry: %+v", diff[0])
	}
}

func TestResolveAll_ConfigValueSameAsDefault(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "verbose", Type: TypeBool, Default: "false"})

	c := NewConfig()
	// Explicitly setting the same value as the default — still "config" source
	c.SetGlobalOption("verbose", "false")

	resolved := s.ResolveAll(c)
	if resolved[0].Source != SourceConfig {
		t.Errorf("expected config source even when value matches default, got %q", resolved[0].Source)
	}
}

func TestResolvedOption_DefaultField(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.Register(ConfigOption{Key: "color", Type: TypeString, Default: "auto"})

	c := NewConfig()
	c.SetGlobalOption("color", "never")

	resolved := s.ResolveAll(c)
	if resolved[0].Default != "auto" {
		t.Errorf("expected Default=auto, got %q", resolved[0].Default)
	}
}

// --- FormatSchemaJSON tests ---

func TestFormatSchemaJSON_ValidJSON(t *testing.T) {
	t.Parallel()
	s := NewSchema()
	s.RegisterAll([]ConfigOption{
		{Key: "verbose", Type: TypeBool, Default: "false", Description: "Verbose output"},
		{Key: "pager", Type: TypeString, Section: "help", Description: "Pager program"},
	})

	data, err := s.FormatSchemaJSON()
	if err != nil {
		t.Fatalf("FormatSchemaJSON returned error: %v", err)
	}

	var entries []SchemaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("FormatSchemaJSON output is not valid JSON: %v\noutput: %s", err, string(data))
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestFormatSchemaJSON_ContainsAllOptions(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()

	data, err := s.FormatSchemaJSON()
	if err != nil {
		t.Fatalf("FormatSchemaJSON returned error: %v", err)
	}

	var entries []SchemaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	globals := s.GlobalOptions()
	var sectionCount int
	for _, sec := range s.Sections() {
		sectionCount += len(s.SectionOptions(sec))
	}
	expectedTotal := len(globals) + sectionCount

	if len(entries) != expectedTotal {
		t.Fatalf("expected %d entries (globals=%d + section=%d), got %d",
			expectedTotal, len(globals), sectionCount, len(entries))
	}
}

func TestDefaultSchema_ContainsSchemaVersion(t *testing.T) {
	t.Parallel()
	s := DefaultSchema()
	opt := s.Lookup("", "config.schema-version")
	if opt == nil {
		t.Fatal("expected config.schema-version option in DefaultSchema")
	}
	if opt.Type != TypeInt {
		t.Errorf("expected TypeInt for config.schema-version, got %q", opt.Type)
	}
	if opt.Default != "1" {
		t.Errorf("expected default '1', got %q", opt.Default)
	}
}
