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

// readTUILines reads lines from a TUI-mode scanner until the given count
// is reached or a timeout fires. Lines have \r\n stripped.
func readTUILines(t *testing.T, scanner *bufio.Scanner, count int) []string {
	t.Helper()
	var lines []string
	for len(lines) < count {
		if !scanner.Scan() {
			t.Fatalf("expected %d lines, got %d", count, len(lines))
		}
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
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

// --- TUI mode tests ---

func TestMockAgent_TUIClassify(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	classifyJSON := `{"categories":[{"name":"auth","files":["login.go"]}]}`

	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_CLASSIFY="+classifyJSON,
	)
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

	// Read output lines. Expected sequence:
	// 1. "Ready." (ready marker)
	// 2. "❯ claude-sonnet-4-20250514" (model selection)
	// 3. "· thinking..." (thinking indicator)
	// 4. "✻ Done." (completion marker)
	// 5. "❯ " (prompt marker — may be on its own line or partial)
	lines := readTUILines(t, scanner, 4)

	// Verify ready marker.
	if lines[0] != "Ready." {
		t.Errorf("line 0: expected 'Ready.', got %q", lines[0])
	}

	// Verify model selection marker — aimux parser detects EVENT_MODEL_SELECT.
	if !strings.Contains(lines[1], "claude-sonnet-4-20250514") {
		t.Errorf("line 1: expected model selection marker, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[1], "❯ ") {
		t.Errorf("line 1: expected prompt marker prefix, got %q", lines[1])
	}

	// Verify thinking indicator — aimux parser detects EVENT_THINKING.
	if !strings.Contains(lines[2], "thinking") {
		t.Errorf("line 2: expected thinking indicator, got %q", lines[2])
	}

	// Verify completion marker — aimux parser detects EVENT_COMPLETION.
	if !strings.Contains(lines[3], "Done") {
		t.Errorf("line 3: expected completion marker, got %q", lines[3])
	}
}

func TestMockAgent_TUIPlan(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	planJSON := `{"stages":[{"name":"stage1","files":["a.go"]}]}`

	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_PLAN="+planJSON,
	)
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

	lines := readTUILines(t, scanner, 4)

	// Verify the full sequence: Ready → Model Select → Thinking → Done.
	if lines[0] != "Ready." {
		t.Errorf("line 0: expected 'Ready.', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "claude-sonnet-4-20250514") {
		t.Errorf("line 1: expected model selection, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "thinking") {
		t.Errorf("line 2: expected thinking, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "Done") {
		t.Errorf("line 3: expected completion, got %q", lines[3])
	}
}

func TestMockAgent_TUIResolve(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	resolveJSON := `{"patches":[{"file":"a.go","content":"fix"}]}`

	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_RESOLVE="+resolveJSON,
	)
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

	lines := readTUILines(t, scanner, 4)

	if lines[0] != "Ready." {
		t.Errorf("line 0: expected 'Ready.', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "claude-sonnet-4-20250514") {
		t.Errorf("line 1: expected model selection, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "thinking") {
		t.Errorf("line 2: expected thinking, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "Done") {
		t.Errorf("line 3: expected completion, got %q", lines[3])
	}
}

func TestMockAgent_TUIModelSelectionMarker(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test that the model selection line matches the aimux parser pattern:
	// `^\s*[❯>]\s+(\S.+)` → EVENT_MODEL_SELECT with modelName field.
	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_CLASSIFY={}",
	)
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

	lines := readTUILines(t, scanner, 4)

	// The model selection line should be "❯ claude-sonnet-4-20250514"
	modelLine := lines[1]
	if !strings.HasPrefix(modelLine, "❯ ") {
		t.Errorf("model line should start with '❯ ', got %q", modelLine)
	}
	modelName := strings.TrimPrefix(modelLine, "❯ ")
	if modelName != "claude-sonnet-4-20250514" {
		t.Errorf("model name should be 'claude-sonnet-4-20250514', got %q", modelName)
	}
}

func TestMockAgent_TUIThinkingDots(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test that the thinking line matches the aimux parser pattern:
	// `(?i)(thinking|analyzing|processing)\s*\.{2,}` → EVENT_THINKING.
	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_CLASSIFY={}",
	)
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

	lines := readTUILines(t, scanner, 4)

	thinkingLine := lines[2]
	// Must contain "thinking" followed by dots.
	if !strings.Contains(thinkingLine, "thinking") {
		t.Errorf("thinking line should contain 'thinking', got %q", thinkingLine)
	}
	if !strings.Contains(thinkingLine, "...") {
		t.Errorf("thinking line should contain '...', got %q", thinkingLine)
	}
}

func TestMockAgent_TUIErrorMessage(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test MOCK_ERROR_MESSAGE env var — should emit error and exit with code 1.
	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(), "MOCK_ERROR_MESSAGE=something went wrong")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Should output "Error: something went wrong"
	if !scanner.Scan() {
		t.Fatal("expected output line")
	}
	line := strings.TrimRight(scanner.Text(), "\r\n")
	if !strings.Contains(line, "Error:") || !strings.Contains(line, "something went wrong") {
		t.Errorf("expected error message, got %q", line)
	}

	// Should exit with non-zero code.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 0 {
				t.Fatal("expected non-zero exit code on MOCK_ERROR_MESSAGE")
			}
		} else if err != nil {
			t.Fatalf("unexpected wait error: %v", err)
		} else {
			t.Fatal("expected non-zero exit code, got exit 0")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for error exit")
	}
}

func TestMockAgent_TUICustomMarkers(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test MOCK_PROMPT_MARKER and MOCK_READY_MARKER customization.
	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_CLASSIFY={}",
		"MOCK_PROMPT_MARKER=> ",
		"MOCK_READY_MARKER=Initialized",
	)
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

	lines := readTUILines(t, scanner, 4)

	// Custom ready marker.
	if lines[0] != "Initialized" {
		t.Errorf("expected custom ready marker 'Initialized', got %q", lines[0])
	}

	// Custom prompt marker in model selection line.
	if !strings.HasPrefix(lines[1], "> ") {
		t.Errorf("expected custom prompt marker '> ', got %q", lines[1])
	}
}

func TestMockAgent_TUIInteractiveMode(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test that TUI mode without one-shot env vars still works interactively.
	cmd := exec.Command(bin, "-tui")
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

	// Read startup: "Ready.\r\n" then "❯ "
	if !scanner.Scan() {
		t.Fatal("expected ready line")
	}
	readyLine := strings.TrimRight(scanner.Text(), "\r\n")
	if readyLine != "Ready." {
		t.Fatalf("expected 'Ready.', got %q", readyLine)
	}

	// Send a message.
	_, _ = stdin.Write([]byte("hello world\n"))

	// Read thinking + response.
	if !scanner.Scan() {
		t.Fatal("expected thinking line")
	}
	thinkingLine := strings.TrimRight(scanner.Text(), "\r\n")
	if !strings.Contains(thinkingLine, "thinking") {
		t.Fatalf("expected thinking indicator, got %q", thinkingLine)
	}

	if !scanner.Scan() {
		t.Fatal("expected response line")
	}
	responseLine := strings.TrimRight(scanner.Text(), "\r\n")
	if !strings.Contains(responseLine, "hello world") {
		t.Fatalf("expected response containing 'hello world', got %q", responseLine)
	}

	stdin.Close()
}

func TestMockAgent_TUIHeartbeat(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test that MOCK_HEARTBEAT_INTERVAL_MS is accepted without error.
	// The heartbeat requires a valid MCP server to connect to, so we
	// just verify the process starts and produces the expected output
	// without the heartbeat causing a crash (it logs errors to stderr
	// but continues).
	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_CLASSIFY={}",
		"MOCK_HEARTBEAT_INTERVAL_MS=100",
	)
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

	// Should still produce the normal one-shot output sequence.
	lines := readTUILines(t, scanner, 4)

	if lines[0] != "Ready." {
		t.Errorf("line 0: expected 'Ready.', got %q", lines[0])
	}
	if !strings.Contains(lines[1], "claude-sonnet-4-20250514") {
		t.Errorf("line 1: expected model selection, got %q", lines[1])
	}
	if !strings.Contains(lines[2], "thinking") {
		t.Errorf("line 2: expected thinking, got %q", lines[2])
	}
	if !strings.Contains(lines[3], "Done") {
		t.Errorf("line 3: expected completion, got %q", lines[3])
	}
}

func TestMockAgent_TUICompletionMarker(t *testing.T) {
	skipSlow(t)
	bin := buildMock(t)

	// Test that the completion marker "✻ Done." is emitted.
	// The aimux parser pattern `(?i)(task|operation)\s+(complete|completed|finished|done)`
	// should match "Done" in the completion line.
	cmd := exec.Command(bin, "-tui")
	cmd.Env = append(os.Environ(),
		"MOCK_PROCESSING_MS=50",
		"MOCK_PLAN={}",
	)
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

	lines := readTUILines(t, scanner, 4)

	completionLine := lines[3]
	if !strings.Contains(completionLine, "Done") {
		t.Errorf("completion line should contain 'Done', got %q", completionLine)
	}
}

// --- Unit tests for helper functions ---

func TestDetectMockMode(t *testing.T) {
	tests := []struct {
		name        string
		classify    string
		plan        string
		resolve     string
		wantMode    mockMode
		wantPayload string
	}{
		{
			name:     "no mode",
			wantMode: mockModeNone,
		},
		{
			name:        "classify",
			classify:    `{"categories":[]}`,
			wantMode:    mockModeClassify,
			wantPayload: `{"categories":[]}`,
		},
		{
			name:        "plan",
			plan:        `{"stages":[]}`,
			wantMode:    mockModePlan,
			wantPayload: `{"stages":[]}`,
		},
		{
			name:        "resolve",
			resolve:     `{"patches":[]}`,
			wantMode:    mockModeResolve,
			wantPayload: `{"patches":[]}`,
		},
		{
			name:        "classify takes priority",
			classify:    `{"c":1}`,
			plan:        `{"s":1}`,
			resolve:     `{"p":1}`,
			wantMode:    mockModeClassify,
			wantPayload: `{"c":1}`,
		},
		{
			name:        "plan takes priority over resolve",
			plan:        `{"s":1}`,
			resolve:     `{"p":1}`,
			wantMode:    mockModePlan,
			wantPayload: `{"s":1}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set/unset env vars for this test.
			if tt.classify != "" {
				t.Setenv("MOCK_CLASSIFY", tt.classify)
			}
			if tt.plan != "" {
				t.Setenv("MOCK_PLAN", tt.plan)
			}
			if tt.resolve != "" {
				t.Setenv("MOCK_RESOLVE", tt.resolve)
			}

			mode, payload := detectMockMode()
			if mode != tt.wantMode {
				t.Errorf("detectMockMode() mode = %v, want %v", mode, tt.wantMode)
			}
			if payload != tt.wantPayload {
				t.Errorf("detectMockMode() payload = %q, want %q", payload, tt.wantPayload)
			}
		})
	}
}

func TestMockModeToolName(t *testing.T) {
	tests := []struct {
		mode mockMode
		want string
	}{
		{mockModeNone, ""},
		{mockModeClassify, "reportClassification"},
		{mockModePlan, "reportSplitPlan"},
		{mockModeResolve, "reportResolution"},
	}
	for _, tt := range tests {
		if got := mockModeToolName(tt.mode); got != tt.want {
			t.Errorf("mockModeToolName(%v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_MOCK_EXISTING", "value")
	if got := envOr("TEST_MOCK_EXISTING", "default"); got != "value" {
		t.Errorf("envOr existing key = %q, want %q", got, "value")
	}
	if got := envOr("TEST_MOCK_MISSING", "default"); got != "default" {
		t.Errorf("envOr missing key = %q, want %q", got, "default")
	}
}
