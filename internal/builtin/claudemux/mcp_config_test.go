package claudemux

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestMCPInstanceConfig_Endpoint_BeforeListen(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("ep-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	if ep := c.Endpoint(); ep != "" {
		t.Errorf("Endpoint before listen = %q, want empty", ep)
	}
	if addr := c.ListenerAddr(); addr != nil {
		t.Errorf("ListenerAddr before listen = %v, want nil", addr)
	}
}

func testMCPServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
}

func TestMCPInstanceConfig_ListenAndServe(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("listen-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	ep := c.Endpoint()
	if ep == "" {
		t.Fatal("Endpoint is empty after ListenAndServe")
	}
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(ep, "http://127.0.0.1:") {
			t.Errorf("Endpoint = %q, want http://127.0.0.1:... on Windows", ep)
		}
	} else {
		if !strings.HasPrefix(ep, "http+unix://") {
			t.Errorf("Endpoint = %q, want http+unix://... on Unix", ep)
		}
	}
	if !strings.HasSuffix(ep, "/mcp") {
		t.Errorf("Endpoint = %q, want to end with /mcp", ep)
	}

	addr := c.ListenerAddr()
	if addr == nil {
		t.Fatal("ListenerAddr is nil after ListenAndServe")
	}
}

func TestMCPInstanceConfig_ListenAndServe_Double(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("double-listen")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("first ListenAndServe: %v", err)
	}

	err = c.ListenAndServe(server)
	if !errors.Is(err, ErrAlreadyListening) {
		t.Errorf("second ListenAndServe: got %v, want ErrAlreadyListening", err)
	}
}

func TestMCPInstanceConfig_ListenAndServe_AfterClose(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("closed-listen")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	_ = c.Close()

	server := testMCPServer()
	err = c.ListenAndServe(server)
	if !errors.Is(err, ErrInstanceClosed) {
		t.Errorf("ListenAndServe after Close: got %v, want ErrInstanceClosed", err)
	}
}

func TestMCPInstanceConfig_WriteConfigFile(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("config-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

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
	if osm.URL == "" {
		t.Error("osm URL is empty")
	}
	if osm.URL != c.Endpoint() {
		t.Errorf("osm URL = %q, want %q", osm.URL, c.Endpoint())
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

func TestMCPInstanceConfig_WriteConfigFile_BeforeListen(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("no-listen")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	err = c.WriteConfigFile()
	if !errors.Is(err, ErrNotListening) {
		t.Errorf("WriteConfigFile before listen: got %v, want ErrNotListening", err)
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

	// Validate before listen should fail.
	if err := c.Validate(); !errors.Is(err, ErrNotListening) {
		t.Errorf("Validate before listen: got %v, want ErrNotListening", err)
	}

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	// Validate without config file should fail.
	if err := c.Validate(); err == nil {
		t.Error("Validate without config file: expected error")
	}

	if err := c.WriteConfigFile(); err != nil {
		t.Fatalf("WriteConfigFile: %v", err)
	}

	// Validate with listener + config should pass.
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestMCPInstanceConfig_Close(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("close-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}
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

func TestMCPInstanceConfig_Close_BeforeListen(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("close-no-listen")
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

func TestMCPInstanceConfig_HTTPEndpointResponds(t *testing.T) {
	t.Parallel()

	c, err := NewMCPInstanceConfig("http-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	// The MCP HTTP endpoint should be reachable.
	addr := c.ListenerAddr()
	if addr == nil {
		t.Fatal("no listener address")
	}

	var url string
	switch addr.Network() {
	case "unix":
		// For Unix sockets, use a custom dialer.
		sockPath := addr.String()
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return net.Dial("unix", sockPath)
				},
			},
		}
		resp, err := client.Get("http://unix/mcp")
		if err != nil {
			t.Fatalf("GET unix socket: %v", err)
		}
		_ = resp.Body.Close()
		// StreamableHTTPHandler returns a non-500 response for GET; the exact
		// status varies by SDK version (400, 405, etc.). We just verify the
		// server is reachable and doesn't crash.
		if resp.StatusCode >= 500 {
			t.Errorf("GET /mcp status = %d, want non-5xx (server is reachable)", resp.StatusCode)
		}
		return
	default:
		url = "http://" + addr.String() + "/mcp"
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	_ = resp.Body.Close()
	// See above: we only check the server is reachable, not the specific status.
	if resp.StatusCode >= 500 {
		t.Errorf("GET /mcp status = %d, want non-5xx (server is reachable)", resp.StatusCode)
	}
}

func TestEndpointType(t *testing.T) {
	t.Parallel()

	et := endpointType()
	switch runtime.GOOS {
	case "windows":
		if et != "tcp" {
			t.Errorf("endpointType on windows = %q, want tcp", et)
		}
	default:
		if et != "unix" {
			t.Errorf("endpointType on %s = %q, want unix", runtime.GOOS, et)
		}
	}
}

func TestMCPInstanceConfig_SocketPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets not available on Windows")
	}
	t.Parallel()

	c, err := NewMCPInstanceConfig("socket-test")
	if err != nil {
		t.Fatalf("NewMCPInstanceConfig: %v", err)
	}
	defer func() { _ = c.Close() }()

	server := testMCPServer()
	if err := c.ListenAndServe(server); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	addr := c.ListenerAddr()
	if addr.Network() != "unix" {
		t.Errorf("network = %q, want unix", addr.Network())
	}

	sockPath := addr.String()
	if !strings.Contains(sockPath, "mcp.sock") {
		t.Errorf("socket path = %q, want to contain mcp.sock", sockPath)
	}

	// Verify socket file exists.
	if _, err := os.Stat(sockPath); err != nil {
		t.Errorf("socket file should exist: %v", err)
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
