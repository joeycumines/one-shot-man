package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func skipSlow(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping subprocess test in short mode")
	}
}

// buildMock builds the mockagent binary and returns its path.
func buildMock(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	srcDir := filepath.Dir(thisFile)
	bin := filepath.Join(t.TempDir(), "mockagent")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// ndjsonMsg is a minimal NDJSON message for parsing test output.
type ndjsonMsg struct {
	Type          string  `json:"type"`
	Subtype       string  `json:"subtype,omitempty"`
	Content       string  `json:"content,omitempty"`
	SessionID     string  `json:"sessionId,omitempty"`
	ID            string  `json:"id,omitempty"`
	Tool          string  `json:"tool,omitempty"`
	Thinking      bool    `json:"thinking,omitempty"`
	RetryAfterMs  int     `json:"retryAfterMs,omitempty"`
	CostUSD       float64 `json:"costUsd,omitempty"`
	DurationMs    int64   `json:"durationMs,omitempty"`
	DurationAPIMs int64   `json:"durationApiMs,omitempty"`
}

// readNDJSON reads NDJSON lines from a scanner until the given count or timeout.
func readNDJSON(t *testing.T, scanner *bufio.Scanner, count int) []ndjsonMsg {
	t.Helper()
	var msgs []ndjsonMsg
	for i := 0; i < count; i++ {
		if !scanner.Scan() {
			t.Fatalf("expected %d messages, got %d", count, len(msgs))
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			i--
			continue
		}
		var msg ndjsonMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("parse NDJSON line %d: %v\nline: %s", i, err, line)
		}
		msgs = append(msgs, msg)
	}
	return msgs
}

func TestMockAgent_Protocol(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=50")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Read init messages: system/init, assistant/text
	msgs := readNDJSON(t, scanner, 2)
	if msgs[0].Type != "system" || msgs[0].Subtype != "init" {
		t.Fatalf("first message should be system/init, got type=%s subtype=%s", msgs[0].Type, msgs[0].Subtype)
	}
	if msgs[0].SessionID != "mock-session-001" {
		t.Fatalf("session_id=%s, want mock-session-001", msgs[0].SessionID)
	}
	if msgs[1].Type != "assistant" || msgs[1].Content != "MockAgent ready." {
		t.Fatalf("second message should be assistant ready, got type=%s content=%s", msgs[1].Type, msgs[1].Content)
	}

	// Send a user message.
	_, _ = stdin.Write([]byte(`{"type":"user","content":"hello"}` + "\n"))

	// Read response: thinking, assistant text, result
	msgs = readNDJSON(t, scanner, 3)

	if msgs[0].Type != "assistant" || !strings.Contains(msgs[0].Content, "thinking") {
		t.Fatalf("expected thinking message, got type=%s content=%s", msgs[0].Type, msgs[0].Content)
	}
	if msgs[1].Type != "assistant" || !strings.Contains(msgs[1].Content, "hello") {
		t.Fatalf("expected response containing 'hello', got type=%s content=%s", msgs[1].Type, msgs[1].Content)
	}
	if msgs[2].Type != "result" || msgs[2].Subtype != "success" {
		t.Fatalf("expected result/success, got type=%s subtype=%s", msgs[2].Type, msgs[2].Subtype)
	}

	stdin.Close()
}

func TestMockAgent_RateLimit(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=50")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Skip init messages.
	readNDJSON(t, scanner, 2)

	// Send rate limit trigger.
	_, _ = stdin.Write([]byte(`{"type":"user","content":"MOCK_RATE_LIMIT:test"}` + "\n"))

	// Read: thinking, rate_limit_event
	msgs := readNDJSON(t, scanner, 2)

	if msgs[0].Type != "assistant" {
		t.Fatalf("expected thinking, got type=%s", msgs[0].Type)
	}
	if msgs[1].Type != "rate_limit_event" {
		t.Fatalf("expected rate_limit_event, got type=%s", msgs[1].Type)
	}
	if msgs[1].Subtype != "warning" {
		t.Fatalf("expected subtype=warning, got %s", msgs[1].Subtype)
	}
	if msgs[1].RetryAfterMs != 1000 {
		t.Fatalf("expected retry_after_ms=1000, got %d", msgs[1].RetryAfterMs)
	}

	stdin.Close()
}

func TestMockAgent_Crash(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=50")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Skip init messages.
	readNDJSON(t, scanner, 2)

	// Send crash trigger.
	_, _ = stdin.Write([]byte(`{"type":"user","content":"MOCK_CRASH:"}` + "\n"))

	// Wait for process to exit with non-zero code.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				t.Fatal("expected non-zero exit code on MOCK_CRASH")
			}
		} else if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		} else {
			t.Fatal("expected non-zero exit code, got exit 0")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for crash exit")
	}
}

func TestMockAgent_Error(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=50")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Skip init messages.
	readNDJSON(t, scanner, 2)

	// Send error trigger.
	_, _ = stdin.Write([]byte(`{"type":"user","content":"MOCK_ERROR:bad"}` + "\n"))

	// Read: thinking, error text, result/error
	msgs := readNDJSON(t, scanner, 3)

	if msgs[0].Type != "assistant" {
		t.Fatalf("expected thinking, got type=%s", msgs[0].Type)
	}
	if msgs[1].Type != "assistant" || !strings.Contains(msgs[1].Content, "Error:") {
		t.Fatalf("expected error text, got type=%s content=%s", msgs[1].Type, msgs[1].Content)
	}
	if msgs[2].Type != "result" || msgs[2].Subtype != "error" {
		t.Fatalf("expected result/error, got type=%s subtype=%s", msgs[2].Type, msgs[2].Subtype)
	}

	stdin.Close()
}

func TestMockAgent_Permission(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "MOCK_PROCESSING_MS=50")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Wait()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Skip init messages.
	readNDJSON(t, scanner, 2)

	// Send permission trigger.
	_, _ = stdin.Write([]byte(`{"type":"user","content":"MOCK_PERMISSION:run command"}` + "\n"))

	// Read: thinking, control_request
	msgs := readNDJSON(t, scanner, 2)

	if msgs[0].Type != "assistant" {
		t.Fatalf("expected thinking, got type=%s", msgs[0].Type)
	}
	if msgs[1].Type != "control_request" || msgs[1].Subtype != "tool_call" {
		t.Fatalf("expected control_request/tool_call, got type=%s subtype=%s", msgs[1].Type, msgs[1].Subtype)
	}
	if msgs[1].ID != "mock-perm-001" {
		t.Fatalf("expected id=mock-perm-001, got %s", msgs[1].ID)
	}

	// Send control_response.
	_, _ = stdin.Write([]byte(`{"type":"control_response","id":"mock-perm-001","response":"allow"}` + "\n"))

	// Read: permission granted text, result/success
	msgs = readNDJSON(t, scanner, 2)

	if msgs[0].Type != "assistant" || !strings.Contains(msgs[0].Content, "Permission granted") {
		t.Fatalf("expected permission granted text, got type=%s content=%s", msgs[0].Type, msgs[0].Content)
	}
	if msgs[1].Type != "result" || msgs[1].Subtype != "success" {
		t.Fatalf("expected result/success, got type=%s subtype=%s", msgs[1].Type, msgs[1].Subtype)
	}

	stdin.Close()
}
