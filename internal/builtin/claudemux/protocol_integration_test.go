package claudemux

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// collectEvents reads from handle.Receive() in a loop with a timeout,
// classifying each NDJSON line into an OutputEvent. Returns when done
// returns true, io.EOF is received, or timeout expires.
func collectEvents(t *testing.T, handle AgentHandle, timeout time.Duration, done func(events []OutputEvent) bool) []OutputEvent {
	t.Helper()

	deadline := time.After(timeout)
	var events []OutputEvent

	for {
		ch := make(chan struct {
			line string
			err  error
		}, 1)
		go func() {
			line, err := handle.Receive()
			ch <- struct {
				line string
				err  error
			}{line, err}
		}()

		select {
		case result := <-ch:
			if result.err != nil {
				if errors.Is(result.err, io.EOF) {
					return events
				}
				t.Logf("receive error: %v (collected %d events)", result.err, len(events))
				return events
			}
			if result.line == "" {
				continue
			}
			oe := classifyNDJSON(result.line)
			events = append(events, oe)
			if done(events) {
				return events
			}
		case <-deadline:
			t.Logf("collectEvents timeout after %v (%d events collected)", timeout, len(events))
			return events
		}
	}
}

// classifyNDJSON parses a raw NDJSON line and classifies it into an
// OutputEvent using the same logic as protocolHandle.translateEvent.
func classifyNDJSON(line string) OutputEvent {
	var ce ClaudeEvent
	if err := json.Unmarshal([]byte(line), &ce); err != nil {
		return OutputEvent{Type: EventText, Line: line}
	}

	oe := OutputEvent{
		Type:   EventText,
		Line:   line,
		Fields: make(map[string]string),
	}

	switch ce.Type {
	case claudeEventSystem:
		if ce.Subtype == "init" {
			oe.Type = EventCompletion
			oe.Pattern = "system-init"
		}
	case claudeEventAssistant:
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

func hasEventType(events []OutputEvent, typ EventType) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func hasResultSuccess(events []OutputEvent) bool {
	for _, e := range events {
		if e.Type == EventCompletion && e.Pattern == "result-success" {
			return true
		}
	}
	return false
}

func hasContentContaining(events []OutputEvent, substr string) bool {
	for _, e := range events {
		if strings.Contains(e.Line, substr) {
			return true
		}
	}
	return false
}

func findEventByType(events []OutputEvent, typ EventType) *OutputEvent {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}
	return nil
}

func spawnMock(t *testing.T) AgentHandle {
	t.Helper()
	prov := &MockClaudeProvider{ProcessingMs: 50}
	handle, err := prov.Spawn(context.Background(), SpawnOpts{Mode: ModeProtocol})
	if err != nil {
		t.Fatalf("MockClaudeProvider.Spawn: %v", err)
	}
	return handle
}

func waitReady(t *testing.T, handle AgentHandle) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := handle.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func formatEvents(events []OutputEvent) string {
	var b strings.Builder
	for i, e := range events {
		b.WriteString(EventTypeName(e.Type))
		if e.Pattern != "" {
			b.WriteString("(")
			b.WriteString(e.Pattern)
			b.WriteString(")")
		}
		if len(e.Fields) > 0 {
			b.WriteString(" ")
			b.WriteString(strings.Replace(e.Line, "\n", "\\n", -1))
		}
		if i < len(events)-1 {
			b.WriteString(", ")
		}
	}
	return b.String()
}

func TestProtocol_WaitReady(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := handle.WaitReady(ctx); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestProtocol_SimplePrompt(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()
	waitReady(t, handle)

	if err := handle.Send("hello"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	events := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasResultSuccess(events)
	})

	if !hasContentContaining(events, "hello") {
		t.Fatalf("expected output containing 'hello', got %d events:\n%s",
			len(events), formatEvents(events))
	}
}

func TestProtocol_MultiTurn(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()
	waitReady(t, handle)

	if err := handle.Send("first"); err != nil {
		t.Fatalf("Send first: %v", err)
	}
	events1 := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasResultSuccess(events)
	})
	if !hasContentContaining(events1, "first") {
		t.Fatalf("turn 1: expected output containing 'first', got %d events:\n%s",
			len(events1), formatEvents(events1))
	}

	if err := handle.Send("second"); err != nil {
		t.Fatalf("Send second: %v", err)
	}
	events2 := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasResultSuccess(events)
	})
	if !hasContentContaining(events2, "second") {
		t.Fatalf("turn 2: expected output containing 'second', got %d events:\n%s",
			len(events2), formatEvents(events2))
	}
}

func TestProtocol_RateLimitEvent(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()
	waitReady(t, handle)

	if err := handle.Send("MOCK_RATE_LIMIT:test"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	events := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasEventType(events, EventRateLimit)
	})

	if !hasEventType(events, EventRateLimit) {
		t.Fatalf("expected EventRateLimit in output, got %d events:\n%s",
			len(events), formatEvents(events))
	}
}

func TestProtocol_PermissionPrompt(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()
	waitReady(t, handle)

	if err := handle.Send("MOCK_PERMISSION:test"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	events := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasEventType(events, EventPermission)
	})

	permEvent := findEventByType(events, EventPermission)
	if permEvent == nil {
		t.Fatalf("expected EventPermission in output, got %d events:\n%s",
			len(events), formatEvents(events))
	}

	var ce ClaudeEvent
	if err := json.Unmarshal([]byte(permEvent.Line), &ce); err != nil {
		t.Fatalf("parse permission event: %v", err)
	}
	permID := ce.ID
	if permID == "" {
		t.Fatal("permission event has no ID")
	}

	controlResp, err := json.Marshal(map[string]string{
		"type":     "control_response",
		"id":       permID,
		"response": "allow",
	})
	if err != nil {
		t.Fatalf("marshal control_response: %v", err)
	}
	if err := handle.Send(string(controlResp)); err != nil {
		t.Fatalf("Send control_response: %v", err)
	}

	postEvents := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasResultSuccess(events)
	})

	if !hasContentContaining(postEvents, "Permission granted") {
		t.Fatalf("expected 'Permission granted' after control_response, got %d events:\n%s",
			len(postEvents), formatEvents(postEvents))
	}
}

func TestProtocol_ErrorEvent(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()
	waitReady(t, handle)

	if err := handle.Send("MOCK_ERROR:test"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	events := collectEvents(t, handle, 5*time.Second, func(events []OutputEvent) bool {
		return hasEventType(events, EventError)
	})

	if !hasEventType(events, EventError) {
		t.Fatalf("expected EventError in output, got %d events:\n%s",
			len(events), formatEvents(events))
	}
}

func TestProtocol_Crash(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()
	waitReady(t, handle)

	if err := handle.Send("MOCK_CRASH:"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	exitCode, err := handle.Wait()
	if err != nil {
		t.Logf("Wait returned error (acceptable for crash): %v", err)
	}
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code on MOCK_CRASH, got %d", exitCode)
	}
}

func TestProtocol_WaitReadyTimeout(t *testing.T) {
	handle := spawnMock(t)
	defer handle.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handle.WaitReady(ctx)
	if !errors.Is(err, ErrNotReady) {
		t.Fatalf("expected ErrNotReady, got: %v", err)
	}
}
