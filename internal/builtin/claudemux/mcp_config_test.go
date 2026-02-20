package claudemux

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewMCPInstanceConfig(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("test-session-1")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	if c.SessionID != "test-session-1" {
		t.Errorf("SessionID = %q, want %q", c.SessionID, "test-session-1")
	}
	if c.configDir == "" {
		t.Error("configDir is empty")
	}
	if _, err := os.Stat(c.configDir); err != nil {
		t.Errorf("configDir does not exist: %v", err)
	}
	if c.ConfigPath() == "" {
		t.Error("ConfigPath is empty")
	}
	if !strings.HasSuffix(c.ConfigPath(), "mcp-config.json") {
		t.Errorf("ConfigPath = %q, want to end with mcp-config.json", c.ConfigPath())
	}
	if c.OsmBinary == "" {
		t.Error("OsmBinary is empty")
	}
}

func TestNewMCPInstanceConfig_EmptySessionID(t *testing.T) {
	t.Parallel()

	_, err := NewMCPInstanceConfig("")
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestNewMCPInstanceConfig_SanitizesSessionID(t *testing.T) {
	t.Parallel()

	// Session ID with special chars should not cause filesystem errors.
	c, err := NewMCPInstanceConfig("my session/with:special<chars>")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := os.Stat(c.configDir); err != nil {
		t.Errorf("configDir should exist: %v", err)
	}
}

func TestNewMCPInstanceConfig_LongSessionID(t *testing.T) {
	t.Parallel()

	longID := strings.Repeat("x", 200)
	c, err := NewMCPInstanceConfig(longID)
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := os.Stat(c.configDir); err != nil {
		t.Errorf("configDir should exist: %v", err)
	}
}

func TestMCPInstanceConfig_WriteConfigFile(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("config-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Override binary for predictable test output.
	c.OsmBinary = "/usr/local/bin/osm"

	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	// Read and parse the config file.
	data, err := os.ReadFile(c.ConfigPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal config: %v", err)
	}

	osm, ok := cfg.MCPServers["osm"]
	if !ok {
		t.Fatal("config missing 'osm' server entry")
	}
	if osm.Command != "/usr/local/bin/osm" {
		t.Errorf("osm Command = %q, want %q", osm.Command, "/usr/local/bin/osm")
	}
	wantArgs := []string{"mcp-instance", "--session", "config-test"}
	if len(osm.Args) != len(wantArgs) {
		t.Fatalf("osm Args len = %d, want %d", len(osm.Args), len(wantArgs))
	}
	for i, arg := range wantArgs {
		if osm.Args[i] != arg {
			t.Errorf("osm Args[%d] = %q, want %q", i, osm.Args[i], arg)
		}
	}

	// Verify no "url" field in the output.
	if strings.Contains(string(data), `"url"`) {
		t.Errorf("config should not contain url field, got:\n%s", data)
	}

	// Verify file permissions on Unix.
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(c.ConfigPath())
		if err != nil {
			t.Fatalf("Stat config: %v", err)
		}
		perm := fi.Mode().Perm()
		if perm&0077 != 0 {
			t.Errorf("config file permissions = %o, want group/other bits unset", perm)
		}
	}
}

func TestMCPInstanceConfig_WriteConfigFile_AfterClose(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("closed-write")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	_ = c.Close()

	err = c.WriteConfigFile()
	if !errors.Is(err, ErrInstanceClosed) {
		t.Errorf("WriteConfigFile after Close: got %v, want ErrInstanceClosed", err)
	}
}

func TestMCPInstanceConfig_WriteConfigFile_SessionIDInArgs(t *testing.T) {
	t.Parallel()

	// Test that the original session ID is preserved in the args.
	c, err := NewMCPInstanceConfig("session-with-special_chars.123")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	c.OsmBinary = "osm"
	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	data, err := os.ReadFile(c.ConfigPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	osm := cfg.MCPServers["osm"]
	if len(osm.Args) < 3 || osm.Args[2] != "session-with-special_chars.123" {
		t.Errorf("session ID not preserved in args: %v", osm.Args)
	}
}

func TestMCPInstanceConfig_SpawnArgs(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("args-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	args := c.SpawnArgs()
	if len(args) != 2 {
		t.Fatalf("SpawnArgs() len = %d, want 2", len(args))
	}
	if args[0] != "--mcp-config" {
		t.Errorf("SpawnArgs()[0] = %q, want %q", args[0], "--mcp-config")
	}
	if args[1] != c.ConfigPath() {
		t.Errorf("SpawnArgs()[1] = %q, want %q", args[1], c.ConfigPath())
	}
}

func TestMCPInstanceConfig_Validate(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("validate-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Validate without config file should fail.
	if err := c.Validate(); err == nil {
		t.Error("Validate without config file: expected error")
	}

	c.OsmBinary = "osm"
	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	// Validate with config should pass.
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestMCPInstanceConfig_Validate_AfterClose(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("validate-closed")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	_ = c.Close()

	err = c.Validate()
	if !errors.Is(err, ErrInstanceClosed) {
		t.Errorf("Validate after Close: got %v, want ErrInstanceClosed", err)
	}
}

func TestMCPInstanceConfig_Close(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("close-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}

	c.OsmBinary = "osm"
	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	dir := c.configDir
	cfgPath := c.ConfigPath()

	// Close should succeed.
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Temp dir should be removed.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir should be removed after Close, stat: %v", err)
	}

	// Config file should be removed.
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("config file should be removed after Close, stat: %v", err)
	}

	// Double close should be safe.
	if err := c.Close(); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestMCPInstanceConfig_Close_BeforeWrite(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("close-no-write")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}

	dir := c.configDir
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir should be removed, stat: %v", err)
	}
}

func TestMCPInstanceConfig_ConfigDir(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("dir-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	// ConfigPath should be inside configDir.
	if !strings.HasPrefix(c.ConfigPath(), c.configDir) {
		t.Errorf("ConfigPath %q not under configDir %q", c.ConfigPath(), c.configDir)
	}

	// configDir should have the sanitized session ID in its name.
	base := filepath.Base(c.configDir)
	if !strings.Contains(base, "osm-mcp-dir-test") {
		t.Errorf("configDir base = %q, want to contain osm-mcp-dir-test", base)
	}
}

func TestMCPInstanceConfig_ConfigJSON_Structure(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("json-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	c.OsmBinary = "/path/to/osm"
	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	data, err := os.ReadFile(c.ConfigPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Parse as generic JSON to verify exact structure.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	mcpServers, ok := raw["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers object, got: %T", raw["mcpServers"])
	}

	osmEntry, ok := mcpServers["osm"].(map[string]any)
	if !ok {
		t.Fatalf("expected osm entry object, got: %T", mcpServers["osm"])
	}

	// Must have "command" and "args" fields.
	if _, ok := osmEntry["command"]; !ok {
		t.Error("osm entry missing 'command' field")
	}
	if _, ok := osmEntry["args"]; !ok {
		t.Error("osm entry missing 'args' field")
	}

	// Must NOT have "url" field.
	if _, ok := osmEntry["url"]; ok {
		t.Error("osm entry should not have 'url' field")
	}

	// Verify indented output (pretty-printed).
	if !strings.Contains(string(data), "  ") {
		t.Error("config should be indented")
	}
}

func TestMCPInstanceConfig_OsmBinaryOverride(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("binary-override")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Override the binary path.
	c.OsmBinary = "/custom/path/to/osm-dev"
	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	data, err := os.ReadFile(c.ConfigPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var cfg mcpConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if cfg.MCPServers["osm"].Command != "/custom/path/to/osm-dev" {
		t.Errorf("command = %q, want /custom/path/to/osm-dev", cfg.MCPServers["osm"].Command)
	}
}
