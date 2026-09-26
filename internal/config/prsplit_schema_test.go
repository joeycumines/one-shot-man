package config

import "testing"

func TestDefaultSchema_ContainsPrSplitOptions(t *testing.T) {
	t.Parallel()

	schema := DefaultSchema()
	expected := map[string]OptionType{
		"base":               TypeString,
		"strategy":           TypeEnum,
		"max":                TypeInt,
		"prefix":             TypeString,
		"verify":             TypeString,
		"dry-run":            TypeBool,
		"agent-command":      TypeString,
		"agent-arg":          TypeString,
		"agent-env":          TypeString,
		"timeout":            TypeDuration,
		"resume":             TypeBool,
		"cleanup-on-failure": TypeBool,
	}

	for key, wantType := range expected {
		opt := schema.Lookup("pr-split", key)
		if opt == nil {
			t.Errorf("PR-split option %q is not registered", key)
			continue
		}
		if opt.Type != wantType {
			t.Errorf("PR-split option %q type = %q, want %q", key, opt.Type, wantType)
		}
	}
}

func TestValidateConfig_AcceptsPrSplitOptions(t *testing.T) {
	t.Parallel()

	cfg := NewConfig()
	cfg.Commands["pr-split"] = map[string]string{
		"base":               "main",
		"strategy":           "extension",
		"max":                "8",
		"prefix":             "feature/",
		"verify":             "go test ./...",
		"dry-run":            "true",
		"agent-command":      "/usr/local/bin/agent",
		"agent-arg":          "--verbose",
		"agent-env":          "DEBUG=1",
		"timeout":            "30s",
		"resume":             "false",
		"cleanup-on-failure": "true",
	}

	if issues := ValidateConfig(cfg, DefaultSchema()); len(issues) != 0 {
		t.Fatalf("valid PR-split config reported issues: %v", issues)
	}
}

func TestGetCommandBool_UsesCaseInsensitiveSchemaValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"true", "TRUE", "Yes", "on"} {
		cfg := NewConfig()
		cfg.Commands["pr-split"] = map[string]string{"resume": value}
		if !cfg.GetCommandBool("pr-split", "resume") {
			t.Errorf("GetCommandBool(%q) = false, want true", value)
		}
	}
}
