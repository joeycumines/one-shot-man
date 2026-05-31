package claudemux

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// ClaudeEventType is the type field in a Claude Code NDJSON event.
type ClaudeEventType string

const (
	claudeEventSystem         ClaudeEventType = "system"
	claudeEventAssistant      ClaudeEventType = "assistant"
	claudeEventResult         ClaudeEventType = "result"
	claudeEventControlRequest ClaudeEventType = "control_request"
	claudeEventRateLimit      ClaudeEventType = "rate_limit_event"
	claudeEventToolUse        ClaudeEventType = "tool_use"
)

// ClaudeEvent represents a single NDJSON line from Claude Code's stream-json output.
// Fields are lenient: only Type and Subtype are required for routing; everything
// else is optional and extracted as needed by the event translator.
type ClaudeEvent struct {
	Type         ClaudeEventType `json:"type"`
	Subtype      string          `json:"subtype,omitempty"`
	Content      string          `json:"content,omitempty"`
	Message      json.RawMessage `json:"message,omitempty"`
	Tool         string          `json:"tool,omitempty"`
	Args         json.RawMessage `json:"args,omitempty"`
	ID           string          `json:"id,omitempty"`
	RetryAfterMs int             `json:"retryAfterMs,omitempty"`
	Thinking     bool            `json:"thinking,omitempty"`
}

type claudeAssistantMessage struct {
	Content  string `json:"content,omitempty"`
	Thinking bool   `json:"thinking,omitempty"`
	Role     string `json:"role,omitempty"`
}

// protocolHandle implements AgentHandle for Claude Code's NDJSON protocol mode.
// It spawns the agent with --output-format stream-json --input-format stream-json
// and communicates via stdin/stdout pipes (no PTY).
type protocolHandle struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner

	mu      sync.Mutex
	readyCh chan struct{}
	eventCh chan OutputEvent
	doneCh  chan struct{}
	err     error
}

// Compile-time interface check.
var _ AgentHandle = (*protocolHandle)(nil)

// newProtocolHandle creates a protocolHandle from a prepared exec.Cmd.
// The caller must have set up StdinPipe and StdoutPipe on cmd before calling.
// Starts the scanner goroutine; the caller must Start() the cmd.
func newProtocolHandle(cmd *exec.Cmd) (*protocolHandle, error) {
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("claudemux: stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return nil, fmt.Errorf("claudemux: stdout pipe: %w", err)
	}

	h := &protocolHandle{
		cmd:     cmd,
		stdin:   stdinPipe,
		scanner: bufio.NewScanner(stdoutPipe),
		readyCh: make(chan struct{}),
		eventCh: make(chan OutputEvent, 64),
		doneCh:  make(chan struct{}),
	}

	// Set a generous scanner buffer for large JSON events.
	h.scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	go h.scanLoop()

	return h, nil
}

func (h *protocolHandle) scanLoop() {
	defer close(h.doneCh)
	defer close(h.eventCh)

	for h.scanner.Scan() {
		line := h.scanner.Text()
		if line == "" {
			continue
		}

		var ce ClaudeEvent
		if err := json.Unmarshal([]byte(line), &ce); err != nil {
			// Unparseable line — emit as raw text so the caller sees it.
			h.sendEvent(OutputEvent{
				Type: EventText,
				Line: line,
			})
			continue
		}

		oe := h.translateEvent(ce, line)
		h.sendEvent(oe)
	}

	if err := h.scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		h.mu.Lock()
		if h.err == nil {
			h.err = err
		}
		h.mu.Unlock()
	}
}

func (h *protocolHandle) sendEvent(oe OutputEvent) {
	select {
	case h.eventCh <- oe:
	case <-h.doneCh:
	}
}

func (h *protocolHandle) translateEvent(ce ClaudeEvent, rawLine string) OutputEvent {
	oe := OutputEvent{
		Type:   EventText,
		Line:   rawLine,
		Fields: make(map[string]string),
	}

	switch ce.Type {
	case claudeEventSystem:
		if ce.Subtype == "init" {
			oe.Type = EventCompletion
			oe.Pattern = "system-init"
			// Signal readiness.
			select {
			case <-h.readyCh:
				// Already closed.
			default:
				close(h.readyCh)
			}
		}

	case claudeEventAssistant:
		// Extract content from nested message if top-level is empty.
		content := ce.Content
		thinking := ce.Thinking
		if content == "" && ce.Message != nil {
			var msg claudeAssistantMessage
			if err := json.Unmarshal(ce.Message, &msg); err == nil {
				content = msg.Content
				if msg.Thinking {
					thinking = true
				}
			}
		}
		if thinking {
			oe.Type = EventThinking
			oe.Pattern = "assistant-thinking"
			if content != "" {
				oe.Fields["content"] = content
			}
		} else if ce.Subtype == "text" || content != "" {
			oe.Type = EventText
			if content != "" {
				oe.Fields["content"] = content
			}
		}

	case claudeEventResult:
		switch ce.Subtype {
		case "success":
			oe.Type = EventCompletion
			oe.Pattern = "result-success"
		case "error":
			oe.Type = EventError
			oe.Pattern = "result-error"
			if ce.Content != "" {
				oe.Fields["message"] = ce.Content
			}
		}

	case claudeEventControlRequest:
		if ce.Subtype == "tool_call" {
			oe.Type = EventPermission
			oe.Pattern = "control-request-tool-call"
			if ce.Tool != "" {
				oe.Fields["tool"] = ce.Tool
			}
			if ce.ID != "" {
				oe.Fields["id"] = ce.ID
			}
		}

	case claudeEventRateLimit:
		oe.Type = EventRateLimit
		oe.Pattern = "rate-limit-event"
		if ce.RetryAfterMs > 0 {
			oe.Fields["retryAfterMs"] = fmt.Sprintf("%d", ce.RetryAfterMs)
		}

	case claudeEventToolUse:
		oe.Type = EventToolUse
		oe.Pattern = "tool-use"
		if ce.Tool != "" {
			oe.Fields["toolName"] = ce.Tool
		}
	}

	return oe
}

func (h *protocolHandle) Send(input string) error {
	var msg []byte

	// If the input is already a JSON object with a "type" field, send it
	// as-is — this allows callers to send control_response and other
	// protocol messages directly without them being wrapped in a user envelope.
	if trimmed := strings.TrimSpace(input); len(trimmed) > 0 && trimmed[0] == '{' {
		var probe map[string]json.RawMessage
		if json.Unmarshal([]byte(trimmed), &probe) == nil {
			if _, hasType := probe["type"]; hasType {
				msg = []byte(trimmed)
			}
		}
	}

	if msg == nil {
		var err error
		msg, err = json.Marshal(map[string]string{
			"type":    "user",
			"content": input,
		})
		if err != nil {
			return fmt.Errorf("claudemux: marshal user message: %w", err)
		}
	}

	msg = append(msg, '\n')
	_, err := h.stdin.Write(msg)
	if err != nil {
		return fmt.Errorf("claudemux: write to stdin: %w", err)
	}
	return nil
}

func (h *protocolHandle) Receive() (string, error) {
	oe, ok := <-h.eventCh
	if !ok {
		h.mu.Lock()
		err := h.err
		h.mu.Unlock()
		if err != nil {
			return "", err
		}
		return "", io.EOF
	}
	return oe.Line, nil
}

// ReceiveEvent returns the next typed OutputEvent from the NDJSON stream.
// This is the structured equivalent of Receive() — callers that need event
// type information (not just the raw line) should prefer ReceiveEvent.
// Returns (OutputEvent{}, io.EOF) when the stream is exhausted.
func (h *protocolHandle) ReceiveEvent() (OutputEvent, error) {
	oe, ok := <-h.eventCh
	if !ok {
		h.mu.Lock()
		err := h.err
		h.mu.Unlock()
		if err != nil {
			return OutputEvent{}, err
		}
		return OutputEvent{}, io.EOF
	}
	return oe, nil
}

func (h *protocolHandle) Close() error {
	// Ensure readyCh is closed so WaitReady doesn't block forever.
	select {
	case <-h.readyCh:
	default:
		close(h.readyCh)
	}

	_ = h.cmd.Process.Kill()
	_ = h.stdin.Close()
	return nil
}

func (h *protocolHandle) IsAlive() bool {
	if h.cmd.Process == nil {
		return false
	}
	return h.cmd.ProcessState == nil || !h.cmd.ProcessState.Exited()
}

func (h *protocolHandle) Wait() (int, error) {
	err := h.cmd.Wait()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func (h *protocolHandle) Resize(_, _ int) error { return nil }

func (h *protocolHandle) WaitReady(ctx context.Context) error {
	select {
	case <-h.readyCh:
		return nil
	case <-ctx.Done():
		return ErrNotReady
	case <-h.doneCh:
		// Process exited before becoming ready.
		h.mu.Lock()
		err := h.err
		h.mu.Unlock()
		if err != nil {
			return fmt.Errorf("claudemux: agent exited before ready: %w", err)
		}
		return ErrNotReady
	}
}
