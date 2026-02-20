package claudemux

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// MCPInstanceConfig manages a per-instance MCP configuration file for
// Claude Code. Each Claude Code instance spawns its own MCP server process
// via the "command" + "args" config format (stdio transport). This replaces
// the previous HTTP-based approach with the standard MCP specification.
//
// Lifecycle:
//  1. NewMCPInstanceConfig(sessionID) — allocates temp dir, resolves binary
//  2. WriteConfigFile() — generates JSON config with command/args
//  3. (caller spawns Claude Code with SpawnArgs())
//  4. Close() — removes temp dir
type MCPInstanceConfig struct {
	SessionID string

	// OsmBinary is the path to the osm binary. Defaults to os.Executable().
	// Can be overridden for testing or custom deployments.
	OsmBinary string

	// configDir is the temp directory holding the config.
	configDir string

	// configPath is the path to the generated config JSON file.
	configPath string

	mu     sync.Mutex
	closed bool
}

var (
	// ErrInstanceClosed is returned after Close() has been called.
	ErrInstanceClosed = errors.New("claudemux: MCP instance closed")
)

// mcpSessionIDSafe matches only characters safe for filesystem paths.
var mcpSessionIDSafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// NewMCPInstanceConfig creates a new per-instance MCP configuration.
// It allocates a temporary directory for the config file and resolves
// the osm binary path via os.Executable().
func NewMCPInstanceConfig(sessionID string) (*MCPInstanceConfig, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("claudemux: session ID is required")
	}

	// Sanitize session ID for use in filesystem paths.
	safe := mcpSessionIDSafe.ReplaceAllString(sessionID, "_")
	if len(safe) > 64 {
		safe = safe[:64]
	}

	tmpDir, err := os.MkdirTemp("", "osm-mcp-"+safe+"-")
	if err != nil {
		return nil, fmt.Errorf("claudemux: failed to create temp dir: %w", err)
	}

	// Resolve the current osm binary path.
	osmBin, err := os.Executable()
	if err != nil {
		// Fall back to "osm" and hope it's on PATH.
		osmBin = "osm"
	}

	return &MCPInstanceConfig{
		SessionID:  sessionID,
		OsmBinary:  osmBin,
		configDir:  tmpDir,
		configPath: filepath.Join(tmpDir, "mcp-config.json"),
	}, nil
}

// mcpServerEntry is the JSON structure for a single MCP server in the config.
// Uses the standard MCP "command" + "args" format for stdio transport.
type mcpServerEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// mcpConfigFile is the JSON structure of the generated config file.
type mcpConfigFile struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

// WriteConfigFile generates the MCP config JSON file at ConfigPath().
// The generated file tells Claude Code to spawn an osm MCP server process
// for this session using stdio transport.
func (c *MCPInstanceConfig) WriteConfigFile() error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrInstanceClosed
	}

	cfg := mcpConfigFile{
		MCPServers: map[string]mcpServerEntry{
			"osm": {
				Command: c.OsmBinary,
				Args:    []string{"mcp-instance", "--session", c.SessionID},
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("claudemux: failed to marshal config: %w", err)
	}

	if err := os.WriteFile(c.configPath, data, 0600); err != nil {
		return fmt.Errorf("claudemux: failed to write config: %w", err)
	}

	return nil
}

// ConfigPath returns the path to the generated config JSON file.
func (c *MCPInstanceConfig) ConfigPath() string {
	return c.configPath
}

// SpawnArgs returns additional CLI arguments for Claude Code to use this
// MCP configuration. The caller should append these to the spawn command.
func (c *MCPInstanceConfig) SpawnArgs() []string {
	return []string{"--mcp-config", c.configPath}
}

// Validate checks that the configuration is usable before spawning Claude.
// It verifies the config file exists and is not closed.
func (c *MCPInstanceConfig) Validate() error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrInstanceClosed
	}
	if _, err := os.Stat(c.configPath); err != nil {
		return fmt.Errorf("claudemux: config file not found: %w", err)
	}
	return nil
}

// Close removes the temp directory containing the config file. Safe to
// call multiple times.
func (c *MCPInstanceConfig) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	dir := c.configDir
	c.mu.Unlock()

	if dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("claudemux: remove temp dir: %w", err)
		}
	}
	return nil
}
