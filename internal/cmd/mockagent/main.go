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
// Environment:
//
//	MOCK_PROCESSING_MS - simulated processing delay in ms (default 200)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
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

func main() {
	tui := flag.Bool("tui", false, "TUI mode: produce terminal-style output instead of NDJSON")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var err error
	if *tui {
		err = runTUI(ctx)
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

// runTUI implements TUI mode: the mock produces terminal-style output with
// prompt characters ("❯ "), thinking indicators ("· thinking..."), and
// response text — instead of NDJSON. This mode is used by VTStateDetector
// integration tests that feed raw PTY output through the VTerm emulator.
//
// Output format:
//   - Startup:  "MockAgent ready.\r\n" then "❯ " (no trailing newline)
//   - Processing: "· thinking...\r\n"
//   - Normal response: "Response to: {input}\r\n" then "❯ "
//   - Rate limit: "Rate limit exceeded.\r\n" then "❯ "
//   - Error: "Error: simulated error\r\n" then "❯ "
//   - Permission: "Allow Bash? (y/n): " → waits for response → "Permission granted: {resp}\r\n❯ "
//   - Crash: os.Exit(1)
//   - Delay: MOCK_DELAY_MS:N:{msg} → sleep N ms → normal response
//
// Each output phase is flushed separately so that PTY readers can observe
// intermediate states (e.g. StateProcessing before StateReady).
func runTUI(ctx context.Context) error {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	fmt.Fprint(out, "MockAgent ready.\r\n")
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush init: %w", err)
	}
	fmt.Fprint(out, "❯ ")
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
			fmt.Fprint(out, "❯ ")
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
			fmt.Fprint(out, "❯ ")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}

		case strings.HasPrefix(line, "MOCK_ERROR:"):
			fmt.Fprint(out, "Error: simulated error\r\n")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush error: %w", err)
			}
			fmt.Fprint(out, "❯ ")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}

		case strings.HasPrefix(line, "MOCK_CRASH:"):
			_ = out.Flush()
			os.Exit(1)

		case strings.HasPrefix(line, "MOCK_DELAY_MS:"):
			if err := handleDelayTUI(ctx, out, line); err != nil {
				return err
			}

		default:
			fmt.Fprintf(out, "Response to: %s\r\n", line)
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush response: %w", err)
			}
			fmt.Fprint(out, "❯ ")
			if err := out.Flush(); err != nil {
				return fmt.Errorf("flush prompt: %w", err)
			}
		}
	}

	return scanner.Err()
}

func handleDelayTUI(ctx context.Context, out *bufio.Writer, content string) error {
	prefix := "MOCK_DELAY_MS:"
	rest := content[len(prefix):]
	parts := strings.SplitN(rest, ":", 2)
	delayMs, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		fmt.Fprintf(out, "Error: invalid MOCK_DELAY_MS value: %s\r\n", parts[0])
		if err := out.Flush(); err != nil {
			return err
		}
		fmt.Fprint(out, "❯ ")
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
	fmt.Fprint(out, "❯ ")
	return out.Flush()
}
