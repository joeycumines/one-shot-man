package ollama_test

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/ollama"
)

// ============================================================================
// Integration tests for the Ollama HTTP client.
//
// These tests require a real Ollama server and are skipped by default.
//
// To run:
//
//	go test -race -v -count=1 -timeout=5m \
//	  ./internal/builtin/ollama/... \
//	  -args -integration -model=gpt-oss:20b-cloud
//
// Or via make:
//
//	make integration-ollama-http
//
// Prerequisites:
//   - Ollama server running at localhost:11434 (or custom -endpoint)
//   - A model with tool-calling support (gpt-oss:20b-cloud recommended)
// ============================================================================

var (
	integrationEnabled bool
	testModel          string
	testEndpoint       string
)

func TestMain(m *testing.M) {
	flag.BoolVar(&integrationEnabled, "integration", false,
		"enable integration tests that require a real Ollama server")
	flag.StringVar(&testModel, "model", "gpt-oss:20b-cloud",
		"model for integration tests")
	flag.StringVar(&testEndpoint, "endpoint", "http://localhost:11434",
		"Ollama server endpoint")
	flag.Parse()
	os.Exit(m.Run())
}

func skipIfNotIntegration(t *testing.T) {
	t.Helper()
	if !integrationEnabled {
		t.Skip("skipped; use -integration to enable")
	}
}

func integrationClient(t *testing.T) *ollama.Client {
	t.Helper()
	skipIfNotIntegration(t)
	client, err := ollama.NewClient(testEndpoint)
	if err != nil {
		t.Fatalf("NewClient(%q): %v", testEndpoint, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Health(ctx); err != nil {
		t.Skipf("Ollama not reachable at %s: %v", testEndpoint, err)
	}
	return client
}

// T332: Basic Ollama connectivity and chat.

func TestIntegration_Health(t *testing.T) {
	client := integrationClient(t)
	if !client.IsHealthy(context.Background()) {
		t.Fatal("IsHealthy returned false")
	}
}

func TestIntegration_Version(t *testing.T) {
	client := integrationClient(t)
	ver, err := client.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if ver.Version == "" {
		t.Error("empty version string")
	}
	t.Logf("Ollama version: %s", ver.Version)
}

func TestIntegration_ListModels(t *testing.T) {
	client := integrationClient(t)
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("no models found")
	}
	t.Logf("Found %d models:", len(models))
	for _, m := range models {
		t.Logf("  - %s", m.Name)
	}
}

func TestIntegration_ShowModel(t *testing.T) {
	client := integrationClient(t)
	info, err := client.ShowModel(context.Background(), testModel)
	if err != nil {
		t.Fatalf("ShowModel(%q): %v", testModel, err)
	}
	t.Logf("Capabilities: %v", info.Capabilities)
	t.Logf("SupportsTools: %v", info.SupportsTools())
	if info.HasCapability("completion") {
		t.Log("Model has completion capability")
	}
}

func TestIntegration_BasicChat(t *testing.T) {
	client := integrationClient(t)
	resp, err := client.Chat(context.Background(), ollama.ChatRequest{
		Model: testModel,
		Messages: []ollama.Message{
			{Role: "user", Content: "What is 2+2? Answer with ONLY the number, nothing else."},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	t.Logf("Response: %q", resp.Message.Content)
	if !strings.Contains(resp.Message.Content, "4") {
		t.Errorf("expected '4' in response, got: %q", resp.Message.Content)
	}
}

// T333: Tool calling with real model.

func TestIntegration_ToolCalling(t *testing.T) {
	client := integrationClient(t)

	info, err := client.ShowModel(context.Background(), testModel)
	if err != nil {
		t.Fatalf("ShowModel: %v", err)
	}
	if !info.SupportsTools() {
		t.Skipf("model %q does not support tools", testModel)
	}

	tools := []ollama.Tool{{
		Type: "function",
		Function: ollama.ToolFunction{
			Name:        "calculator",
			Description: "Evaluate an arithmetic expression and return the result",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"expression": {"type": "string", "description": "The arithmetic expression"}
				},
				"required": ["expression"]
			}`),
		},
	}}

	resp, err := client.Chat(context.Background(), ollama.ChatRequest{
		Model: testModel,
		Messages: []ollama.Message{
			{Role: "user", Content: "Use the calculator tool to compute 15 * 7"},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("Chat with tools: %v", err)
	}

	t.Logf("Role: %s, Content: %q, ToolCalls: %d",
		resp.Message.Role, resp.Message.Content, len(resp.Message.ToolCalls))

	if len(resp.Message.ToolCalls) == 0 {
		t.Fatal("expected at least one tool call")
	}

	tc := resp.Message.ToolCalls[0]
	t.Logf("Tool: %s, Args: %v", tc.Function.Name, tc.Function.Arguments)
	if tc.Function.Name != "calculator" {
		t.Errorf("tool name = %q, want 'calculator'", tc.Function.Name)
	}
}

// T334: Multi-turn agentic loop with real model.

func TestIntegration_AgenticLoop(t *testing.T) {
	client := integrationClient(t)

	info, err := client.ShowModel(context.Background(), testModel)
	if err != nil {
		t.Fatalf("ShowModel: %v", err)
	}
	if !info.SupportsTools() {
		t.Skipf("model %q does not support tools", testModel)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"),
		[]byte("The secret number is 42. The color is blue."), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := ollama.NewToolRegistry()
	if err := ollama.RegisterBuiltinTools(reg, dir); err != nil {
		t.Fatal(err)
	}

	var toolCallLog []string
	runner, err := ollama.NewAgenticRunner(ollama.AgentConfig{
		Client: client,
		Model:  testModel,
		Tools:  reg,
		SystemPrompt: "You are a helpful assistant. When asked about file contents, " +
			"use the read_file tool to read the file first, then answer based on what you read.",
		MaxTurns: 5,
		OnToolCall: func(name string, args map[string]interface{}) {
			toolCallLog = append(toolCallLog, name)
			t.Logf("Tool call: %s", ollama.FormatToolCallSummary(name, args))
		},
		OnToolResult: func(name, result string, err error) {
			if err != nil {
				t.Logf("Tool error: %s: %v", name, err)
			} else {
				preview := result
				if len(preview) > 100 {
					preview = preview[:100] + "..."
				}
				t.Logf("Tool result: %s → %q", name, preview)
			}
		},
	})
	if err != nil {
		t.Fatalf("NewAgenticRunner: %v", err)
	}

	result, err := runner.Run(context.Background(),
		"Read the file data.txt and tell me: what is the secret number?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	t.Logf("Final: %q", result.FinalContent)
	t.Logf("Turns: %d, ToolCalls: %d", result.TurnsUsed, result.ToolCallCount)
	t.Logf("Tool call log: %v", toolCallLog)

	foundReadFile := false
	for _, name := range toolCallLog {
		if name == "read_file" {
			foundReadFile = true
			break
		}
	}
	if !foundReadFile {
		t.Error("expected read_file tool call in the agentic loop")
	}

	if !strings.Contains(result.FinalContent, "42") {
		t.Errorf("expected '42' in final content, got: %q", result.FinalContent)
	}
}

// T335: Streaming with real model.

func TestIntegration_Streaming(t *testing.T) {
	client := integrationClient(t)

	reader, err := client.ChatStream(context.Background(), ollama.ChatRequest{
		Model: testModel,
		Messages: []ollama.Message{
			{Role: "user", Content: "Count from 1 to 5, one number per line."},
		},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer reader.Close()

	var chunks int
	var full strings.Builder
	for {
		resp, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			t.Fatalf("Next: %v", nextErr)
		}
		chunks++
		full.WriteString(resp.Message.Content)
	}

	t.Logf("Received %d chunks", chunks)
	t.Logf("Full content: %q", full.String())

	if chunks < 2 {
		t.Errorf("expected multiple streaming chunks, got %d", chunks)
	}
}
