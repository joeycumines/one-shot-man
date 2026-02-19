package claudemux

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPInstanceConfig manages a per-instance MCP server endpoint and its
// Claude Code configuration file. Each Claude Code instance gets a unique
// MCP server endpoint (TCP on all platforms, Unix socket where supported)
// and a generated config file that tells Claude Code how to connect.
//
// Lifecycle:
//  1. NewMCPInstanceConfig(sessionID) — allocates temp dir
//  2. ListenAndServe(server) — starts HTTP listener, blocks until ready
//  3. WriteConfigFile() — generates JSON config pointing to the endpoint
//  4. (caller spawns Claude Code with SpawnArgs())
//  5. Close() — stops listener, removes temp dir + socket
type MCPInstanceConfig struct {
	SessionID string

	// configDir is the temp directory holding socket + config.
	configDir string

	// listener is the active network listener (TCP or Unix).
	listener net.Listener

	// configPath is the path to the generated config JSON file.
	configPath string

	// httpServer is the HTTP server wrapping the MCP handler.
	httpServer *http.Server

	mu     sync.Mutex
	closed bool
}

var (
	// ErrNotListening is returned when an operation requires an active listener
	// but ListenAndServe has not been called.
	ErrNotListening = errors.New("claudemux: MCP endpoint not listening")

	// ErrAlreadyListening is returned when ListenAndServe is called twice.
	ErrAlreadyListening = errors.New("claudemux: MCP endpoint already listening")

	// ErrInstanceClosed is returned after Close() has been called.
	ErrInstanceClosed = errors.New("claudemux: MCP instance closed")
)

// mcpSessionIDSafe matches only characters safe for filesystem paths.
var mcpSessionIDSafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// NewMCPInstanceConfig creates a new per-instance MCP configuration.
// It allocates a temporary directory for the socket and config file.
// The sessionID is sanitized for filesystem safety.
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

	return &MCPInstanceConfig{
		SessionID:  sessionID,
		configDir:  tmpDir,
		configPath: filepath.Join(tmpDir, "mcp-config.json"),
	}, nil
}

// endpointType returns "unix" on Unix systems and "tcp" on Windows.
func endpointType() string {
	if runtime.GOOS == "windows" {
		return "tcp"
	}
	return "unix"
}

// ListenAndServe starts an HTTP server for the MCP endpoint. On Unix, it
// listens on a Unix domain socket; on Windows, it listens on TCP localhost
// with an auto-assigned port. The function returns once the listener is
// ready (startup sequencing guarantee). The HTTP server runs until Close()
// is called.
func (c *MCPInstanceConfig) ListenAndServe(server *mcp.Server) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrInstanceClosed
	}
	if c.listener != nil {
		c.mu.Unlock()
		return ErrAlreadyListening
	}
	c.mu.Unlock()

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless: true, // Each instance is dedicated; no session tracking needed
		},
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	var ln net.Listener
	var err error
	if endpointType() == "unix" {
		sockPath := filepath.Join(c.configDir, "mcp.sock")
		ln, err = net.Listen("unix", sockPath)
	} else {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return fmt.Errorf("claudemux: failed to listen: %w", err)
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = ln.Close()
		return ErrInstanceClosed
	}
	c.listener = ln
	c.httpServer = &http.Server{Handler: mux}
	c.mu.Unlock()

	// Start serving in background. Serve blocks until the listener is closed.
	go func() {
		_ = c.httpServer.Serve(ln)
	}()

	return nil
}

// Endpoint returns the URL of the active MCP endpoint, or an empty string
// if not listening. The format is suitable for use in MCP client configs.
func (c *MCPInstanceConfig) Endpoint() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listener == nil {
		return ""
	}
	addr := c.listener.Addr()
	switch addr.Network() {
	case "unix":
		return "http+unix://" + addr.String() + "/mcp"
	default:
		return "http://" + addr.String() + "/mcp"
	}
}

// ListenerAddr returns the raw network address of the listener, or nil
// if not listening. Useful for tests that need the actual net.Addr.
func (c *MCPInstanceConfig) ListenerAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.listener == nil {
		return nil
	}
	return c.listener.Addr()
}

// mcpServerEntry is the JSON structure for a single MCP server in the config.
type mcpServerEntry struct {
	URL string `json:"url"`
}

// mcpConfigFile is the JSON structure of the generated config file.
type mcpConfigFile struct {
	MCPServers map[string]mcpServerEntry `json:"mcpServers"`
}

// WriteConfigFile generates the MCP config JSON file at ConfigPath().
// Must be called after ListenAndServe. The generated file tells Claude Code
// how to connect to this instance's MCP server.
func (c *MCPInstanceConfig) WriteConfigFile() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrInstanceClosed
	}
	if c.listener == nil {
		c.mu.Unlock()
		return ErrNotListening
	}
	c.mu.Unlock()

	endpoint := c.Endpoint()
	cfg := mcpConfigFile{
		MCPServers: map[string]mcpServerEntry{
			"osm": {URL: endpoint},
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
// It verifies the listener is active and the config file exists.
func (c *MCPInstanceConfig) Validate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrInstanceClosed
	}
	if c.listener == nil {
		return ErrNotListening
	}
	if _, err := os.Stat(c.configPath); err != nil {
		return fmt.Errorf("claudemux: config file not found: %w", err)
	}
	return nil
}

// Close stops the HTTP server, closes the listener, and removes the temp
// directory containing the socket and config file. Safe to call multiple times.
func (c *MCPInstanceConfig) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	srv := c.httpServer
	ln := c.listener
	dir := c.configDir
	c.mu.Unlock()

	var errs []error
	if srv != nil {
		if err := srv.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close http server: %w", err))
		}
	} else if ln != nil {
		if err := ln.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close listener: %w", err))
		}
	}
	if dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove temp dir: %w", err))
		}
	}
	return errors.Join(errs...)
}
