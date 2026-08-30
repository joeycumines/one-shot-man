// Command mockagent simulates an agent NDJSON protocol interface.
//
// It reads NDJSON from stdin and writes NDJSON to stdout, providing a
// testable mock that can be spawned by integration tests to verify
// protocol mode integration without needing a real agent CLI.
//
// Special input prefixes:
//
//	MOCK_RATE_LIMIT:  - emit a rate_limit_event, no response
//	MOCK_PERMISSION:  - emit control_request, wait for control_response
//	MOCK_ERROR:       - emit error result
//	MOCK_CRASH:       - exit with code 1 immediately
//	MOCK_DELAY_MS:N:  - sleep N milliseconds before responding
//
// Environment (NDJSON mode):
//
//	MOCK_PROCESSING_MS - simulated processing delay in ms (default 200)
//
// Environment (TUI mode — one-shot PRSplit integration):
//
//	MOCK_CLASSIFY:<json>  - call reportClassification MCP tool with JSON categories
//	MOCK_PLAN:<json>      - call reportSplitPlan MCP tool with JSON stages
//	MOCK_RESOLVE:<json>   - call reportResolution MCP tool with JSON patches/commands
//	MOCK_HEARTBEAT_INTERVAL_MS - send heartbeat calls every N ms while thinking
//	MOCK_PROMPT_MARKER    - custom prompt marker (default "❯ ")
//	MOCK_READY_MARKER     - custom ready marker (default "Ready.")
//	MOCK_ERROR_MESSAGE    - if set, emit error line and exit with code 1
//	MOCK_PROCESSING_MS    - simulated processing delay in ms (default 200)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// inbound represents a parsed NDJSON message from stdin.
type inbound struct {
	Type     string `json:"type"`
	Content  string `json:"content,omitempty"`
	ID       string `json:"id,omitempty"`
	Response string `json:"response,omitempty"`
}

// outbound is a helper for constructing NDJSON output messages.
// JSON field names use camelCase to match the AgentEvent struct in
// protocol_handle.go, which is the authoritative schema definition.
type outbound struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`

	Content       string   `json:"content,omitempty"`
	SessionID     string   `json:"sessionId,omitempty"`
	Tools         []string `json:"tools,omitempty"`
	ID            string   `json:"id,omitempty"`
	Tool          string   `json:"tool,omitempty"`
	Args          any      `json:"args,omitempty"`
	Message       any      `json:"message,omitempty"`
	Thinking      bool     `json:"thinking,omitempty"`
	RetryAfterMs  int      `json:"retryAfterMs,omitempty"`
	CostUSD       float64  `json:"costUsd,omitempty"`
	DurationMs    int64    `json:"durationMs,omitempty"`
	DurationAPIMs int64    `json:"durationApiMs,omitempty"`
}

// pendingPerm tracks a pending permission request awaiting a control_response.
type pendingPerm struct {
	id string
}

// mcpConfig represents the MCP configuration file format used by
// osm:mcpcallback and agent CLIs (e.g., Claude's --mcp-config flag).
type mcpConfig struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers"`
}

type mcpServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// mockMode identifies which one-shot TUI mode is active.
type mockMode int

const (
	mockModeNone     mockMode = iota
	mockModeClassify          // MOCK_CLASSIFY
	mockModePlan              // MOCK_PLAN
	mockModeResolve           // MOCK_RESOLVE
)

func main() {
	tui := flag.Bool("tui", false, "TUI mode: produce terminal-style output instead of NDJSON")
	mcpConfigPath := flag.String("mcp-config", "", "path to MCP config JSON for tool calls")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var err error
	if *tui {
		err = runTUI(ctx, *mcpConfigPath)
	} else {
		err = run(ctx)
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "mockagent: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	// Send system/init on startup.
	writeMsg(out, outbound{
		Type:      "system",
		Subtype:   "init",
		SessionID: "mock-session-001",
		Tools:     []string{"Bash", "Read", "Write"},
	})
	writeMsg(out, outbound{
		Type:    "assistant",
		Subtype: "text",
		Content: "MockAgent ready.",
	})
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush init: %w", err)
	}

	processingMs := envInt("MOCK_PROCESSING_MS", 200)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var pending pendingPerm

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var msg inbound
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// If we have a pending permission request, the next message must be
		// the control_response for that request.
		if pending.id != "" {
			if msg.Type == "control_response" && msg.ID == pending.id {
				writeMsg(out, outbound{
					Type:    "assistant",
					Subtype: "text",
					Content: "Permission granted: " + msg.Response,
				})
				writeMsg(out, outbound{
					Type:          "result",
					Subtype:       "success",
					CostUSD:       0,
					DurationMs:    100,
					DurationAPIMs: 100,
				})
				if err := out.Flush(); err != nil {
					return fmt.Errorf("flush perm response: %w", err)
				}
				pending = pendingPerm{}
			}
			continue
		}

		switch msg.Type {
		case "user":
			p, err := handleUser(ctx, out, msg, processingMs)
			if err != nil {
				return err
			}
			if p.id != "" {
				pending = p
			}
		case "control_response":
		}
	}

	return scanner.Err()
}

// handleUser processes a user message. It returns a pendingPerm if a
// control_request was emitted and we're waiting for a control_response.
func handleUser(ctx context.Context, out *bufio.Writer, msg inbound, processingMs int) (pendingPerm, error) {
	content := msg.Content

	// Emit thinking indicator.
	writeMsg(out, outbound{
		Type:     "assistant",
		Subtype:  "text",
		Thinking: true,
		Content:  "\u00b7 thinking...",
	})
	if err := out.Flush(); err != nil {
		return pendingPerm{}, fmt.Errorf("flush thinking: %w", err)
	}

	// Simulated processing delay.
	select {
	case <-time.After(time.Duration(processingMs) * time.Millisecond):
	case <-ctx.Done():
		return pendingPerm{}, nil
	}

	// Check special prefixes.
	switch {
	case strings.HasPrefix(content, "MOCK_RATE_LIMIT:"):
		writeMsg(out, outbound{
			Type:         "rate_limit_event",
			Subtype:      "warning",
			Message:      "Rate limit exceeded",
			RetryAfterMs: 1000,
		})
		return pendingPerm{}, out.Flush()

	case strings.HasPrefix(content, "MOCK_PERMISSION:"):
		const permID = "mock-perm-001"
		writeMsg(out, outbound{
			Type:    "control_request",
			Subtype: "tool_call",
			Tool:    "Bash",
			Args:    map[string]string{"command": "echo test"},
			ID:      permID,
		})
		if err := out.Flush(); err != nil {
			return pendingPerm{}, fmt.Errorf("flush control_request: %w", err)
		}
		return pendingPerm{id: permID}, nil

	case strings.HasPrefix(content, "MOCK_ERROR:"):
		writeMsg(out, outbound{
			Type:    "assistant",
			Subtype: "text",
			Content: "Error: simulated error",
		})
		writeMsg(out, outbound{
			Type:    "result",
			Subtype: "error",
			Content: "simulated error",
		})
		return pendingPerm{}, out.Flush()

	case strings.HasPrefix(content, "MOCK_CRASH:"):
		_ = out.Flush()
		os.Exit(1)

	case strings.HasPrefix(content, "MOCK_DELAY_MS:"):
		return pendingPerm{}, handleDelay(ctx, out, content)
	}

	// Normal response.
	elapsed := int64(processingMs)
	writeMsg(out, outbound{
		Type:    "assistant",
		Subtype: "text",
		Content: "Response to: " + content,
	})
	writeMsg(out, outbound{
		Type:          "result",
		Subtype:       "success",
		CostUSD:       0,
		DurationMs:    elapsed,
		DurationAPIMs: elapsed,
	})
	return pendingPerm{}, out.Flush()
}

func handleDelay(ctx context.Context, out *bufio.Writer, content string) error {
	// Parse MOCK_DELAY_MS:N
	prefix := "MOCK_DELAY_MS:"
	rest := content[len(prefix):]
	parts := strings.SplitN(rest, ":", 2)
	delayMs, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		writeMsg(out, outbound{
			Type:    "assistant",
			Subtype: "text",
			Content: fmt.Sprintf("Error: invalid MOCK_DELAY_MS value: %s", parts[0]),
		})
		writeMsg(out, outbound{
			Type:    "result",
			Subtype: "error",
			Content: fmt.Sprintf("invalid MOCK_DELAY_MS value: %s", parts[0]),
		})
		return out.Flush()
	}

	select {
	case <-time.After(time.Duration(delayMs) * time.Millisecond):
	case <-ctx.Done():
		return nil
	}

	// The actual message content follows the delay prefix, if present.
	msgContent := "delayed response"
	if len(parts) > 1 {
		msgContent = strings.TrimSpace(parts[1])
	}

	writeMsg(out, outbound{
		Type:    "assistant",
		Subtype: "text",
		Content: "Response to: " + msgContent,
	})
	writeMsg(out, outbound{
		Type:          "result",
		Subtype:       "success",
		CostUSD:       0,
		DurationMs:    int64(delayMs),
		DurationAPIMs: int64(delayMs),
	})
	return out.Flush()
}

func writeMsg(out *bufio.Writer, msg outbound) {
	data, err := json.Marshal(msg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "mockagent: marshal error: %v\n", err)
		return
	}
	_, _ = out.Write(data)
	_ = out.WriteByte('\n')
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// detectMockMode checks environment variables and returns the active
// one-shot TUI mode and its JSON payload.
func detectMockMode() (mockMode, string) {
	if v := os.Getenv("MOCK_CLASSIFY"); v != "" {
		return mockModeClassify, v
	}
	if v := os.Getenv("MOCK_PLAN"); v != "" {
		return mockModePlan, v
	}
	if v := os.Getenv("MOCK_RESOLVE"); v != "" {
		return mockModeResolve, v
	}
	return mockModeNone, ""
}

// mockModeToolName returns the MCP tool name for the given mode.
func mockModeToolName(mode mockMode) string {
	switch mode {
	case mockModeClassify:
		return "reportClassification"
	case mockModePlan:
		return "reportSplitPlan"
	case mockModeResolve:
		return "reportResolution"
	default:
		return ""
	}
}

// runTUI implements TUI mode: the mock produces terminal-style output with
// prompt characters ("❯ "), thinking indicators ("· thinking..."), and
// response text — instead of NDJSON. This mode is used by VTStateDetector
// integration tests that feed raw PTY output through the VTerm emulator.
//
// When one-shot environment variables (MOCK_CLASSIFY, MOCK_PLAN, MOCK_RESOLVE)
// are set, the agent runs a single task: show model selection, think, call the
// appropriate MCP tool, show completion, and exit. This allows the aimux
// TUIStateMachine to transition: Initializing → Ready → Processing → Responding → Ready.
//
// Output format:
//   - Startup:  "MockAgent ready.\r\n" then "❯ " (no trailing newline)
//   - Model select: "❯ claude-sonnet-4-20250514\r\n" (parser detects EVENT_MODEL_SELECT)
//   - Processing: "· thinking...\r\n" (parser detects EVENT_THINKING)
//   - Completion: "✻ Done.\r\n" (parser detects EVENT_COMPLETION)
//   - Ready: "❯ " (parser detects StateReady)
//   - Rate limit: "Rate limit exceeded.\r\n" then "❯ "
//   - Error: "Error: simulated error\r\n" then "❯ "
//   - Permission: "Allow Bash? (y/n): " → waits for response → "Permission granted: {resp}\r\n❯ "
//   - Crash: os.Exit(1)
//   - Delay: MOCK_DELAY_MS:N:{msg} → sleep N ms → normal response
//
// Each output phase is flushed separately so that PTY readers can observe
// intermediate states (e.g. StateProcessing before StateReady).
func runTUI(ctx context.Context, mcpConfigPath string) error {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	promptMarker := envOr("MOCK_PROMPT_MARKER", "❯ ")
	readyMarker := envOr("MOCK_READY_MARKER", "Ready.")

	// Check for one-shot mode (MOCK_CLASSIFY, MOCK_PLAN, MOCK_RESOLVE).
	mode, payload := detectMockMode()
	if mode != mockModeNone {
		return runTUIOneShot(ctx, out, mode, payload, mcpConfigPath, promptMarker, readyMarker)
	}

	// Check for error mode.
	if errMsg := os.Getenv("MOCK_ERROR_MESSAGE"); errMsg != "" {
		fmt.Fprintf(out, "Error: %s\r\n", errMsg)
		if err := out.Flush(); err != nil {
			return fmt.Errorf("flush error: %w", err)
		}
		os.Exit(1)
	}

	// Interactive TUI mode (stdin loop).
	fmt.Fprintf(out, "%s\r\n", readyMarker)
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush init: %w", err)
	}
	fmt.Fprint(out, promptMarker)
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush prompt: %w", err)
	}

	processingMs := envInt("MOCK_PROCESSING_MS", 200)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fmt.Fprint(out, "· thinking...\r\n")
		if err := out.Flush(); err != nil {
			return fmt.Errorf("flush thinking: %w", err)
		}

		select {
		case <-time.After(time.Duration(processingMs) * time.Millisecond):
		case <-ctx.Done():
			return nil
		}

		switch {
		case strings.HasPrefix(line, "MOCK_RATE_LIMIT:"):
			fmt.Fprint(out, "Rate limit exceeded.\r\n")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush rate limit: %w", err)
			}
			fmt.Fprint(out, promptMarker)
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}

		case strings.HasPrefix(line, "MOCK_PERMISSION:"):
			fmt.Fprint(out, "Allow Bash? (y/n): ")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush permission prompt: %w", err)
			}
			if !scanner.Scan() {
				return scanner.Err()
			}
			resp := strings.TrimSpace(scanner.Text())
			if resp == "y" || resp == "yes" {
				fmt.Fprintf(out, "Permission granted: %s\r\n", resp)
			} else {
				fmt.Fprint(out, "Permission denied\r\n")
			}
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush permission response: %w", err)
			}
			fmt.Fprint(out, promptMarker)
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}

		case strings.HasPrefix(line, "MOCK_ERROR:"):
			fmt.Fprint(out, "Error: simulated error\r\n")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush error: %w", err)
			}
			fmt.Fprint(out, promptMarker)
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}

		case strings.HasPrefix(line, "MOCK_CRASH:"):
			_ = out.Flush()
			os.Exit(1)

		case strings.HasPrefix(line, "MOCK_DELAY_MS:"):
			if err := handleDelayTUI(ctx, out, line, promptMarker); err != nil {
				return err
			}

		default:
			fmt.Fprintf(out, "Response to: %s\r\n", line)
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush response: %w", err)
			}
			fmt.Fprint(out, promptMarker)
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}
		}
	}

	return scanner.Err()
}

// runTUIOneShot executes a single TUI-mode task: model selection → thinking →
// MCP tool call → completion. It does not read from stdin.
//
// The output sequence is designed so that the aimux parser detects:
//  1. EVENT_MODEL_SELECT — from "❯ claude-sonnet-4-20250514"
//  2. EVENT_THINKING — from "· thinking..."
//  3. EVENT_COMPLETION — from "✻ Done."
//
// This drives the TUIStateMachine through:
// Initializing → Ready → Processing → Responding → Ready
func runTUIOneShot(ctx context.Context, out *bufio.Writer, mode mockMode, payload string, mcpConfigPath string, promptMarker, readyMarker string) error {
	processingMs := envInt("MOCK_PROCESSING_MS", 200)
	heartbeatMs := envInt("MOCK_HEARTBEAT_INTERVAL_MS", 0)

	// Phase 1: Ready indicator (StateInitializing → StateReady).
	fmt.Fprintf(out, "%s\r\n", readyMarker)
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush ready: %w", err)
	}

	// Phase 2: Model selection marker (parser detects EVENT_MODEL_SELECT).
	fmt.Fprintf(out, "%sclaude-sonnet-4-20250514\r\n", promptMarker)
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush model select: %w", err)
	}

	// Phase 3: Thinking indicator (parser detects EVENT_THINKING).
	fmt.Fprint(out, "· thinking...\r\n")
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush thinking: %w", err)
	}

	// Start heartbeat goroutine if configured.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	if heartbeatMs > 0 && mcpConfigPath != "" {
		go runHeartbeat(heartbeatCtx, mcpConfigPath, time.Duration(heartbeatMs)*time.Millisecond)
	}

	// Simulated processing delay.
	select {
	case <-time.After(time.Duration(processingMs) * time.Millisecond):
	case <-ctx.Done():
		return nil
	}

	// Phase 4: Call MCP tool (if --mcp-config provided).
	toolName := mockModeToolName(mode)
	if mcpConfigPath != "" && toolName != "" {
		if err := callMCPTool(ctx, mcpConfigPath, toolName, payload); err != nil {
			fmt.Fprintf(os.Stderr, "mockagent: MCP tool call %s failed: %v\n", toolName, err)
			// Fall through to completion — the tool call failure is logged
			// but doesn't prevent the agent from completing.
		}
	}

	// Phase 5: Completion marker (parser detects EVENT_COMPLETION).
	fmt.Fprint(out, "✻ Done.\r\n")
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush completion: %w", err)
	}

	// Phase 6: Return to ready prompt.
	fmt.Fprint(out, promptMarker)
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush prompt: %w", err)
	}

	return nil
}

// runHeartbeat periodically calls a heartbeat MCP tool to keep the
// connection alive during long-running operations.
func runHeartbeat(ctx context.Context, mcpConfigPath string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Best-effort heartbeat — errors are logged but not fatal.
			hbCtx, hbCancel := context.WithTimeout(ctx, 5*time.Second)
			if err := callMCPTool(hbCtx, mcpConfigPath, "heartbeat", "{}"); err != nil {
				fmt.Fprintf(os.Stderr, "mockagent: heartbeat failed: %v\n", err)
			}
			hbCancel()
		}
	}
}

// callMCPTool connects to the MCP server described in the config file and
// calls the named tool with the given JSON arguments string.
func callMCPTool(ctx context.Context, configPath, toolName, argsJSON string) error {
	// Read and parse the MCP config file.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read mcp config: %w", err)
	}

	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse mcp config: %w", err)
	}

	// Find the first server entry.
	var serverCfg mcpServerConfig
	for _, s := range cfg.MCPServers {
		serverCfg = s
		break
	}
	if serverCfg.Command == "" {
		return fmt.Errorf("no mcp server found in config")
	}

	// Spawn the bridge command (e.g., "osm mcp-bridge unix /path/to/socket").
	// The bridge connects stdin/stdout to the MCP server's socket.
	cmd := exec.CommandContext(ctx, serverCfg.Command, serverCfg.Args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("bridge stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("bridge stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bridge: %w", err)
	}
	defer func() {
		_ = stdinPipe.Close()
		_ = cmd.Wait()
	}()

	// Connect an MCP client over the bridge's stdio.
	transport := &mcp.IOTransport{
		Reader: stdoutPipe,
		Writer: stdinPipe,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "mockagent",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("mcp connect: %w", err)
	}
	defer session.Close()

	// Parse the arguments JSON.
	var args any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("parse args json: %w", err)
	}

	// Call the tool.
	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: args,
	})
	if err != nil {
		return fmt.Errorf("call tool %s: %w", toolName, err)
	}

	return nil
}

func handleDelayTUI(ctx context.Context, out *bufio.Writer, content string, promptMarker string) error {
	prefix := "MOCK_DELAY_MS:"
	rest := content[len(prefix):]
	parts := strings.SplitN(rest, ":", 2)
	delayMs, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		fmt.Fprintf(out, "Error: invalid MOCK_DELAY_MS value: %s\r\n", parts[0])
		if err := out.Flush(); err != nil {
			return err
		}
		fmt.Fprint(out, promptMarker)
		return out.Flush()
	}

	select {
	case <-time.After(time.Duration(delayMs) * time.Millisecond):
	case <-ctx.Done():
		return nil
	}

	msgContent := "delayed response"
	if len(parts) > 1 {
		msgContent = strings.TrimSpace(parts[1])
	}

	fmt.Fprintf(out, "Response to: %s\r\n", msgContent)
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush delay response: %w", err)
	}
	fmt.Fprint(out, promptMarker)
	return out.Flush()
}

// envOr returns the value of the environment variable named by the key,
// or the provided default value if the variable is not present or empty.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
