package command

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpMakeDefaultTimeout is the maximum time a make invocation may run.
const mcpMakeDefaultTimeout = 5 * time.Minute

// MCPMakeCommand starts an MCP server over stdio that exposes GNU Make
// as a tool. It detects the appropriate make binary (gmake on macOS,
// make elsewhere), validates Makefile presence, and provides two tools:
//
//   - make: Execute a make target with optional working directory.
//   - make_help: Display the Makefile's help target output.
//
// Usage: osm mcp-make [--workdir <dir>] [--file <path>]
type MCPMakeCommand struct {
	*BaseCommand
	workdir string // default working directory (empty = cwd)
	file    string // Makefile path override (empty = auto-detect)
	timeout time.Duration

	// makeBinaryOverride allows tests to inject a mock make binary.
	makeBinaryOverride string
}

// NewMCPMakeCommand creates a new mcp-make command.
func NewMCPMakeCommand() *MCPMakeCommand {
	return &MCPMakeCommand{
		BaseCommand: NewBaseCommand(
			"mcp-make",
			"Start MCP server exposing GNU Make tools (stdio transport)",
			"osm mcp-make [--workdir <dir>] [--file <path>]",
		),
		timeout: mcpMakeDefaultTimeout,
	}
}

// SetupFlags configures the flags for the mcp-make command.
func (c *MCPMakeCommand) SetupFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.workdir, "workdir", "", "Default working directory for make invocations")
	fs.StringVar(&c.file, "file", "", "Path to Makefile (overrides auto-detection)")
}

// Execute starts the MCP server on stdio, blocking until the client disconnects.
func (c *MCPMakeCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("mcp-make: unexpected arguments: %v", args)
	}

	makeBin, err := c.detectMakeBinary()
	if err != nil {
		return fmt.Errorf("mcp-make: %w", err)
	}

	server := c.newMakeServer(makeBin)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// detectMakeBinary finds the appropriate make binary for the platform.
// On macOS, GNU Make is typically installed as gmake (via Homebrew),
// while the system make is BSD make. On other platforms, make is usually
// GNU Make.
func (c *MCPMakeCommand) detectMakeBinary() (string, error) {
	if c.makeBinaryOverride != "" {
		return c.makeBinaryOverride, nil
	}

	candidates := []string{"make"}
	if runtime.GOOS == "darwin" {
		// Prefer gmake on macOS (GNU Make via Homebrew).
		candidates = []string{"gmake", "make"}
	}

	for _, bin := range candidates {
		path, err := exec.LookPath(bin)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("make binary not found; install GNU Make (macOS: brew install make)")
}

// --- MCP tool input types ---

type mcpMakeInput struct {
	Target  string `json:"target" jsonschema:"Make target to execute"`
	Workdir string `json:"workdir,omitempty" jsonschema:"Working directory for make execution (optional, uses server default if empty)"`
	File    string `json:"file,omitempty" jsonschema:"Path to Makefile (optional)"`
}

type mcpMakeHelpInput struct {
	Workdir string `json:"workdir,omitempty" jsonschema:"Working directory (optional, uses server default if empty)"`
}

// newMakeServer creates a configured MCP server with make tools.
// Unexported for testability via InMemoryTransport.
func (c *MCPMakeCommand) newMakeServer(makeBin string) *mcp.Server {
	var (
		mu          sync.Mutex
		targetCache map[string]targetCacheEntry
	)
	targetCache = make(map[string]targetCacheEntry)

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "osm-make",
			Version: "0.1.0",
		},
		nil,
	)

	// --- make ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "make",
		Description: "Execute make command on a Makefile",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpMakeInput) (*mcp.CallToolResult, any, error) {
		if input.Target == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("target is required"))
			return result, nil, nil
		}

		// Resolve working directory.
		workdir, err := c.resolveWorkdir(input.Workdir)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(err)
			return result, nil, nil
		}

		// Build make args.
		var makeArgs []string
		makeFile := input.File
		if makeFile == "" {
			makeFile = c.file
		}
		if makeFile != "" {
			makeArgs = append(makeArgs, "-f", makeFile)
		}
		makeArgs = append(makeArgs, input.Target)

		output, err := c.runMake(ctx, makeBin, workdir, makeArgs)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("make %s: %w\n%s", input.Target, err, output))
			return result, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: output}},
		}, nil, nil
	})

	// --- make_help ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "make_help",
		Description: "Display help from the Makefile's help target",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpMakeHelpInput) (*mcp.CallToolResult, any, error) {
		workdir, err := c.resolveWorkdir(input.Workdir)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(err)
			return result, nil, nil
		}

		// Check cache.
		cacheKey := workdir + ":" + c.file
		mu.Lock()
		entry, ok := targetCache[cacheKey]
		if ok && time.Since(entry.fetched) < 5*time.Minute {
			mu.Unlock()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: entry.output}},
			}, nil, nil
		}
		mu.Unlock()

		var makeArgs []string
		if c.file != "" {
			makeArgs = append(makeArgs, "-f", c.file)
		}
		makeArgs = append(makeArgs, "help")

		output, err := c.runMake(ctx, makeBin, workdir, makeArgs)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("make help: %w\n%s", err, output))
			return result, nil, nil
		}

		// Cache the result.
		mu.Lock()
		targetCache[cacheKey] = targetCacheEntry{
			output:  output,
			fetched: time.Now(),
		}
		mu.Unlock()

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: output}},
		}, nil, nil
	})

	return server
}

// targetCacheEntry caches make help output to avoid repeated invocations.
type targetCacheEntry struct {
	output  string
	fetched time.Time
}

// resolveWorkdir determines the working directory for a make invocation.
// Priority: explicit input > command-level default > current working directory.
func (c *MCPMakeCommand) resolveWorkdir(inputWorkdir string) (string, error) {
	dir := inputWorkdir
	if dir == "" {
		dir = c.workdir
	}
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("cannot determine working directory: %w", err)
		}
	}

	// Resolve to absolute path and verify it exists.
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("invalid working directory %q: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("working directory %q: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working directory %q: not a directory", abs)
	}
	return abs, nil
}

// runMake executes a make invocation with timeout and returns combined output.
func (c *MCPMakeCommand) runMake(ctx context.Context, makeBin, workdir string, args []string) (string, error) {
	timeout := c.timeout
	if timeout == 0 {
		timeout = mcpMakeDefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, makeBin, args...)
	cmd.Dir = workdir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()

	// Trim trailing whitespace from output.
	result := strings.TrimRight(output.String(), "\n\r\t ")
	return result, err
}
