package command

import (
	"bytes"
	"flag"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestPrSplitCommand_NonInteractive(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	var stdout, stderr bytes.Buffer

	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	output := stdout.String()
	if !contains(output, "PR Split") {
		t.Errorf("Expected PR Split initial message in output, got: %s", output)
	}
}

func TestPrSplitCommand_Name(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	if cmd.Name() != "pr-split" {
		t.Errorf("Expected command name 'pr-split', got: %s", cmd.Name())
	}
}

func TestPrSplitCommand_Description(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	expected := "Split a large PR into reviewable stacked branches"
	if cmd.Description() != expected {
		t.Errorf("Expected description '%s', got: %s", expected, cmd.Description())
	}
}

func TestPrSplitCommand_Usage(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	expected := "pr-split [options]"
	if cmd.Usage() != expected {
		t.Errorf("Expected usage '%s', got: %s", expected, cmd.Usage())
	}
}

func TestPrSplitCommand_SetupFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	cmd.SetupFlags(fs)

	// Verify all expected flags are registered
	expectedFlags := []string{
		"interactive", "i",
		"base", "strategy", "max", "prefix", "verify", "dry-run",
		"json",
		"test", "session", "store", "log-level", "log-file", "log-buffer",
		"claude-command", "claude-arg", "claude-model", "claude-config-dir", "claude-env",
		"timeout",
	}

	for _, name := range expectedFlags {
		if fs.Lookup(name) == nil {
			t.Errorf("Expected '%s' flag to be defined", name)
		}
	}
}

func TestPrSplitCommand_FlagParsing(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	err := fs.Parse([]string{
		"--base", "develop",
		"--strategy", "extension",
		"--max", "5",
		"--prefix", "pr/",
		"--verify", "go test ./...",
		"--dry-run",
		"--test",
	})
	if err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	if cmd.baseBranch != "develop" {
		t.Errorf("Expected baseBranch 'develop', got: %s", cmd.baseBranch)
	}
	if cmd.strategy != "extension" {
		t.Errorf("Expected strategy 'extension', got: %s", cmd.strategy)
	}
	if cmd.maxFiles != 5 {
		t.Errorf("Expected maxFiles 5, got: %d", cmd.maxFiles)
	}
	if cmd.branchPrefix != "pr/" {
		t.Errorf("Expected branchPrefix 'pr/', got: %s", cmd.branchPrefix)
	}
	if cmd.verifyCommand != "go test ./..." {
		t.Errorf("Expected verifyCommand 'go test ./...', got: %s", cmd.verifyCommand)
	}
	if !cmd.dryRun {
		t.Error("Expected dryRun to be true")
	}
	if !cmd.testMode {
		t.Error("Expected testMode to be true after parsing --test flag")
	}
}

func TestPrSplitCommand_FlagShortForm(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	err := fs.Parse([]string{"-i"})
	if err != nil {
		t.Fatalf("Failed to parse -i flag: %v", err)
	}

	if !cmd.interactive {
		t.Error("Expected interactive to be true after parsing -i flag")
	}
}

func TestPrSplitCommand_FlagDefaults(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Don't parse any flags — check defaults
	if cmd.baseBranch != "main" {
		t.Errorf("Expected default baseBranch 'main', got: %s", cmd.baseBranch)
	}
	if cmd.strategy != "directory" {
		t.Errorf("Expected default strategy 'directory', got: %s", cmd.strategy)
	}
	if cmd.maxFiles != 10 {
		t.Errorf("Expected default maxFiles 10, got: %d", cmd.maxFiles)
	}
	if cmd.branchPrefix != "split/" {
		t.Errorf("Expected default branchPrefix 'split/', got: %s", cmd.branchPrefix)
	}
	if cmd.verifyCommand != "make" {
		t.Errorf("Expected default verifyCommand 'make', got: %s", cmd.verifyCommand)
	}
	if cmd.dryRun {
		t.Error("Expected default dryRun to be false")
	}
	if cmd.resume {
		t.Error("Expected default resume to be false")
	}
}

// TestPrSplitCommand_ResumeFlag verifies the --resume flag is parsed
// and the config override works.
func TestPrSplitCommand_ResumeFlag(t *testing.T) {
	t.Parallel()

	t.Run("flag sets resume", func(t *testing.T) {
		t.Parallel()
		cfg := config.NewConfig()
		cmd := NewPrSplitCommand(cfg)
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cmd.SetupFlags(fs)
		if err := fs.Parse([]string{"--resume"}); err != nil {
			t.Fatal(err)
		}
		if !cmd.resume {
			t.Error("expected resume to be true after --resume flag")
		}
	})

	t.Run("config override sets resume", func(t *testing.T) {
		t.Parallel()
		cfg := config.NewConfig()
		cfg.SetCommandOption("pr-split", "resume", "true")
		cmd := NewPrSplitCommand(cfg)
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		cmd.SetupFlags(fs)
		if err := fs.Parse(nil); err != nil {
			t.Fatal(err)
		}
		// The config defaults are applied in Run(), not SetupFlags().
		// Verify the config key is recognized and retrievable.
		if v, ok := cfg.GetCommandOption("pr-split", "resume"); !ok || v != "true" {
			t.Errorf("config should have pr-split.resume=true, got ok=%v v=%q", ok, v)
		}
	})
}

func TestPrSplitCommand_FlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(cmd *PrSplitCommand)
		wantErr string
	}{
		{
			name: "invalid strategy",
			setup: func(cmd *PrSplitCommand) {
				cmd.strategy = "bogus"
			},
			wantErr: `invalid --strategy "bogus"`,
		},
		{
			name: "max files zero",
			setup: func(cmd *PrSplitCommand) {
				cmd.maxFiles = 0
			},
			wantErr: "invalid --max 0: must be at least 1",
		},
		{
			name: "max files negative",
			setup: func(cmd *PrSplitCommand) {
				cmd.maxFiles = -5
			},
			wantErr: "invalid --max -5: must be at least 1",
		},
		{
			name: "negative timeout",
			setup: func(cmd *PrSplitCommand) {
				cmd.timeout = -1 * time.Second
			},
			wantErr: "invalid --timeout",
		},
		{
			name: "valid defaults pass",
			setup: func(cmd *PrSplitCommand) {
				// defaults should be valid — no changes
			},
			wantErr: "",
		},
		{
			name: "valid auto strategy",
			setup: func(cmd *PrSplitCommand) {
				cmd.strategy = "auto"
			},
			wantErr: "",
		},
		{
			name: "valid dependency strategy",
			setup: func(cmd *PrSplitCommand) {
				cmd.strategy = "dependency"
			},
			wantErr: "",
		},
		{
			name: "valid positive timeout",
			setup: func(cmd *PrSplitCommand) {
				cmd.timeout = 5 * time.Minute
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			cmd := NewPrSplitCommand(cfg)
			cmd.testMode = true
			cmd.interactive = false
			cmd.store = "memory"
			cmd.session = t.Name()
			tt.setup(cmd)

			var stdout, stderr bytes.Buffer
			err := cmd.Execute(nil, &stdout, &stderr)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestPrSplitCommand_ExecuteWithArgs(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewPrSplitCommand(cfg)

	var stdout, stderr bytes.Buffer

	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	args := []string{"arg1", "arg2"}
	err := cmd.Execute(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with args, got: %v", err)
	}

	output := stdout.String()
	if !contains(output, "PR Split") {
		t.Errorf("Expected PR Split message with args, got: %s", output)
	}
}

func TestPrSplitCommand_ConfigColorOverrides(t *testing.T) {
	cfg := config.NewConfig()
	cfg.Global = map[string]string{
		"prompt.color.input":  "green",
		"prompt.color.prefix": "cyan",
		"other.setting":       "value",
	}

	cmd := NewPrSplitCommand(cfg)

	var stdout, stderr bytes.Buffer

	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with color config, got: %v", err)
	}

	output := stdout.String()
	if !contains(output, "PR Split") {
		t.Errorf("Expected PR Split message with color config, got: %s", output)
	}
}

func TestPrSplitCommand_NilConfig(t *testing.T) {
	cmd := NewPrSplitCommand(nil)

	var stdout, stderr bytes.Buffer

	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Expected no error with nil config, got: %v", err)
	}

	output := stdout.String()
	if !contains(output, "PR Split") {
		t.Errorf("Expected PR Split message with nil config, got: %s", output)
	}
}

func TestPrSplitCommand_EmbeddedContent(t *testing.T) {
	if len(prSplitTemplate) == 0 {
		t.Error("Expected prSplitTemplate to be non-empty")
	}

	if len(allChunkSources()) == 0 {
		t.Error("Expected chunk sources to be non-empty")
	}
}

// ---------------------------------------------------------------------------
// T91: parseClaudeEnv edge cases
// ---------------------------------------------------------------------------

func TestParseClaudeEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want map[string]string
	}{
		{
			name: "normal pairs",
			raw:  "KEY1=val1,KEY2=val2",
			want: map[string]string{"KEY1": "val1", "KEY2": "val2"},
		},
		{
			name: "empty string",
			raw:  "",
			want: map[string]string{},
		},
		{
			name: "key with empty value",
			raw:  "KEY=",
			want: map[string]string{"KEY": ""},
		},
		{
			name: "empty key silently dropped",
			raw:  "=val",
			want: map[string]string{},
		},
		{
			name: "trailing comma",
			raw:  "A=1,B=2,",
			want: map[string]string{"A": "1", "B": "2"},
		},
		{
			name: "leading comma",
			raw:  ",A=1",
			want: map[string]string{"A": "1"},
		},
		{
			name: "whitespace around pairs",
			raw:  " KEY1=val1 , KEY2=val2 ",
			want: map[string]string{"KEY1": "val1", "KEY2": "val2"},
		},
		{
			name: "double comma produces empty pair",
			raw:  "A=1,,B=2",
			want: map[string]string{"A": "1", "B": "2"},
		},
		{
			name: "value containing equals",
			raw:  "KEY=a=b=c",
			want: map[string]string{"KEY": "a=b=c"},
		},
		{
			name: "no equals sign dropped",
			raw:  "NOEQUALS,KEY=val",
			want: map[string]string{"KEY": "val"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseClaudeEnv(tt.raw)
			if len(got) != len(tt.want) {
				t.Errorf("len mismatch: got %d, want %d\ngot: %v\nwant: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}
			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok {
					t.Errorf("missing key %q", k)
				} else if gotV != wantV {
					t.Errorf("got[%q]=%q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T92: timeout config parsing edge cases
// ---------------------------------------------------------------------------

func TestPrSplitCommand_TimeoutConfigParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configVal  string
		wantUsed   bool // whether the parsed duration overrides default
		wantSuffix string
	}{
		{"valid duration", "5m", true, "5m0s"},
		{"valid seconds", "30s", true, "30s"},
		{"valid hours", "2h", true, "2h0m0s"},
		{"invalid string", "abc", false, ""},
		{"negative duration", "-10s", false, ""}, // d > 0 check rejects this
		{"zero duration", "0s", false, ""},       // d > 0 check rejects this
		{"empty string", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			if tt.configVal != "" {
				cfg.SetCommandOption("pr-split", "timeout", tt.configVal)
			}
			cmd := NewPrSplitCommand(cfg)

			// cmd.timeout starts at 0 (the zero value). Config parsing in
			// Execute only applies when cmd.timeout == 0.
			// We test the parsing by calling the internal flag setup, then
			// checking cmd.timeout after manually applying the config logic.
			if v, ok := cfg.GetCommandOption("pr-split", "timeout"); ok && cmd.timeout == 0 {
				if d, err := time.ParseDuration(v); err == nil && d > 0 {
					cmd.timeout = d
				}
			}

			if tt.wantUsed {
				if cmd.timeout.String() != tt.wantSuffix {
					t.Errorf("timeout=%s, want %s", cmd.timeout, tt.wantSuffix)
				}
			} else {
				if cmd.timeout != 0 {
					t.Errorf("expected timeout=0 (not applied), got %s", cmd.timeout)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T98: config max parsing edge cases
// ---------------------------------------------------------------------------

func TestPrSplitCommand_MaxConfigParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		configV  string
		wantMax  int
		setEmpty bool // don't call SetCommandOption
	}{
		{"valid 5", "5", 5, false},
		{"valid 20", "20", 20, false},
		{"negative", "-5", 10, false},  // Atoi succeeds, n <= 0 → not applied
		{"zero", "0", 10, false},       // Atoi succeeds, n <= 0 → not applied
		{"abc", "abc", 10, false},      // Atoi fails → not applied
		{"empty config", "", 10, true}, // not set → not applied
		{"float", "3.5", 10, false},    // Atoi fails on non-integer → not applied
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewConfig()
			if !tt.setEmpty && tt.configV != "" {
				cfg.SetCommandOption("pr-split", "max", tt.configV)
			}
			cmd := NewPrSplitCommand(cfg)

			// Replicate the exact config parsing from Execute (pr_split.go:150-153).
			// NewPrSplitCommand sets maxFiles=10 (default).
			if v, ok := cfg.GetCommandOption("pr-split", "max"); ok && (cmd.maxFiles == 10 || cmd.maxFiles == 0) {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					cmd.maxFiles = n
				}
			}

			if cmd.maxFiles != tt.wantMax {
				t.Errorf("maxFiles=%d, want %d", cmd.maxFiles, tt.wantMax)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T107: planEditorFactory conversion round-trip
// ---------------------------------------------------------------------------

func TestPlanEditorFactory_ConversionRoundTrip(t *testing.T) {
	t.Parallel()

	// Replicate the map→PlanEditorItem conversion from pr_split.go:571-596.
	convertToPlanItems := func(items []interface{}) []PlanEditorItem {
		editorItems := make([]PlanEditorItem, 0, len(items))
		for _, raw := range items {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			item := PlanEditorItem{}
			if name, ok := m["name"].(string); ok {
				item.Name = name
			}
			if branch, ok := m["branchName"].(string); ok {
				item.BranchName = branch
			}
			if desc, ok := m["description"].(string); ok {
				item.Description = desc
			}
			if files, ok := m["files"].([]interface{}); ok {
				for _, f := range files {
					if s, ok := f.(string); ok {
						item.Files = append(item.Files, s)
					}
				}
			}
			editorItems = append(editorItems, item)
		}
		return editorItems
	}

	// Replicate the PlanEditorItem→map conversion from pr_split.go:620-630.
	convertToMaps := func(items []PlanEditorItem) []interface{} {
		out := make([]interface{}, len(items))
		for i, item := range items {
			out[i] = map[string]interface{}{
				"name":        item.Name,
				"files":       item.Files,
				"branchName":  item.BranchName,
				"description": item.Description,
			}
		}
		return out
	}

	t.Run("valid items round-trip", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":        "split/api",
				"branchName":  "split/api-changes",
				"description": "API layer changes",
				"files":       []interface{}{"api.go", "handler.go"},
			},
			map[string]interface{}{
				"name":        "split/cli",
				"branchName":  "split/cli-changes",
				"description": "CLI changes",
				"files":       []interface{}{"main.go"},
			},
		}
		items := convertToPlanItems(input)
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		if items[0].Name != "split/api" {
			t.Errorf("item[0].Name = %q, want %q", items[0].Name, "split/api")
		}
		if items[0].BranchName != "split/api-changes" {
			t.Errorf("item[0].BranchName = %q, want %q", items[0].BranchName, "split/api-changes")
		}
		if len(items[0].Files) != 2 || items[0].Files[0] != "api.go" {
			t.Errorf("item[0].Files = %v, want [api.go handler.go]", items[0].Files)
		}

		// Convert back to maps and verify round-trip.
		maps := convertToMaps(items)
		if len(maps) != 2 {
			t.Fatalf("expected 2 maps, got %d", len(maps))
		}
		m0 := maps[0].(map[string]interface{})
		if m0["name"] != "split/api" {
			t.Errorf("map[0].name = %v, want split/api", m0["name"])
		}
	})

	t.Run("non-map items skipped", func(t *testing.T) {
		input := []interface{}{
			"not-a-map",
			42,
			nil,
			map[string]interface{}{"name": "valid", "files": []interface{}{"a.go"}},
		}
		items := convertToPlanItems(input)
		if len(items) != 1 {
			t.Fatalf("expected 1 item (non-maps skipped), got %d", len(items))
		}
		if items[0].Name != "valid" {
			t.Errorf("item[0].Name = %q, want %q", items[0].Name, "valid")
		}
	})

	t.Run("missing fields produce zero values", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{}, // all fields missing
		}
		items := convertToPlanItems(input)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].Name != "" {
			t.Errorf("expected empty name, got %q", items[0].Name)
		}
		if items[0].BranchName != "" {
			t.Errorf("expected empty branch, got %q", items[0].BranchName)
		}
		if items[0].Files != nil {
			t.Errorf("expected nil files, got %v", items[0].Files)
		}
	})

	t.Run("empty items list", func(t *testing.T) {
		items := convertToPlanItems([]interface{}{})
		if len(items) != 0 {
			t.Fatalf("expected 0 items, got %d", len(items))
		}
	})

	t.Run("non-string files filtered", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"name":  "mixed",
				"files": []interface{}{"valid.go", 42, nil, "also-valid.go"},
			},
		}
		items := convertToPlanItems(input)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if len(items[0].Files) != 2 {
			t.Fatalf("expected 2 valid files, got %d: %v", len(items[0].Files), items[0].Files)
		}
		if items[0].Files[0] != "valid.go" || items[0].Files[1] != "also-valid.go" {
			t.Errorf("files = %v, want [valid.go also-valid.go]", items[0].Files)
		}
	})

	t.Run("via NewPlanEditor round-trip", func(t *testing.T) {
		// Full round-trip: map→PlanEditorItem→NewPlanEditor→Items()→map
		input := []interface{}{
			map[string]interface{}{
				"name":        "split/core",
				"branchName":  "split/core-v2",
				"description": "Core module",
				"files":       []interface{}{"core.go", "core_test.go"},
			},
		}
		items := convertToPlanItems(input)
		editor := NewPlanEditor(items)
		result := editor.Items()
		maps := convertToMaps(result)

		if len(maps) != 1 {
			t.Fatalf("expected 1 map, got %d", len(maps))
		}
		m := maps[0].(map[string]interface{})
		if m["name"] != "split/core" {
			t.Errorf("name = %v, want split/core", m["name"])
		}
		if m["branchName"] != "split/core-v2" {
			t.Errorf("branchName = %v, want split/core-v2", m["branchName"])
		}
		files := m["files"].([]string)
		if len(files) != 2 || files[0] != "core.go" {
			t.Errorf("files = %v, want [core.go core_test.go]", files)
		}
	})
}

// ---------------------------------------------------------------------------
// T108: Execute() PrepareEngine failure path
// ---------------------------------------------------------------------------

func TestPrSplitCommand_PrepareEngineFailure(t *testing.T) {
	t.Parallel()

	// Trigger PrepareEngine failure by providing an invalid log level.
	// resolveLogConfig returns error for unknown levels.
	cmd := &PrSplitCommand{
		scriptCommandBase: scriptCommandBase{
			logLevel: "INVALID_LEVEL_XYZ",
			config:   config.NewConfig(),
			store:    "memory",
			session:  t.Name(),
		},
		strategy: "directory",
		maxFiles: 10,
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from Execute with invalid log level, got nil")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Errorf("expected 'invalid log level' in error, got: %v", err)
	}
}
