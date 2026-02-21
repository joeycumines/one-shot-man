package ollama_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/builtin/ollama"
)

func TestToolRegistry_Register(t *testing.T) {
	r := ollama.NewToolRegistry()
	err := r.Register(ollama.ToolDef{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			return "ok", nil
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want 1", r.Len())
	}
	if !r.Has("test_tool") {
		t.Error("expected Has(test_tool) == true")
	}
}

func TestToolRegistry_RegisterDuplicate(t *testing.T) {
	r := ollama.NewToolRegistry()
	handler := func(ctx context.Context, args map[string]interface{}) (string, error) { return "ok", nil }
	_ = r.Register(ollama.ToolDef{Name: "dup", Handler: handler})
	err := r.Register(ollama.ToolDef{Name: "dup", Handler: handler})
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error = %q, want 'already registered'", err.Error())
	}
}

func TestToolRegistry_RegisterEmptyName(t *testing.T) {
	r := ollama.NewToolRegistry()
	err := r.Register(ollama.ToolDef{
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) { return "", nil },
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestToolRegistry_RegisterNilHandler(t *testing.T) {
	r := ollama.NewToolRegistry()
	err := r.Register(ollama.ToolDef{Name: "nohandler"})
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestToolRegistry_MustRegister_Panics(t *testing.T) {
	r := ollama.NewToolRegistry()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic from MustRegister with nil handler")
		}
	}()
	r.MustRegister(ollama.ToolDef{Name: "bad"})
}

func TestToolRegistry_Get(t *testing.T) {
	r := ollama.NewToolRegistry()
	r.MustRegister(ollama.ToolDef{
		Name:    "get_test",
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) { return "x", nil },
	})
	got := r.Get("get_test")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "get_test" {
		t.Errorf("Name = %q, want get_test", got.Name)
	}
	missing := r.Get("nonexistent")
	if missing != nil {
		t.Error("expected nil for missing tool")
	}
}

func TestToolRegistry_Names(t *testing.T) {
	r := ollama.NewToolRegistry()
	handler := func(ctx context.Context, args map[string]interface{}) (string, error) { return "", nil }
	r.MustRegister(ollama.ToolDef{Name: "alpha", Handler: handler})
	r.MustRegister(ollama.ToolDef{Name: "beta", Handler: handler})
	r.MustRegister(ollama.ToolDef{Name: "gamma", Handler: handler})
	names := r.Names()
	if len(names) != 3 {
		t.Fatalf("len(names) = %d, want 3", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "gamma" {
		t.Errorf("names = %v, want [alpha beta gamma]", names)
	}
}

func TestToolRegistry_OllamaTools(t *testing.T) {
	r := ollama.NewToolRegistry()
	r.MustRegister(ollama.ToolDef{
		Name:        "my_tool",
		Description: "Does stuff",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`),
		Handler:     func(ctx context.Context, args map[string]interface{}) (string, error) { return "", nil },
	})
	tools := r.OllamaTools()
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if tools[0].Type != "function" {
		t.Errorf("Type = %q, want function", tools[0].Type)
	}
	if tools[0].Function.Name != "my_tool" {
		t.Errorf("Function.Name = %q, want my_tool", tools[0].Function.Name)
	}
}

func TestToolRegistry_Execute(t *testing.T) {
	r := ollama.NewToolRegistry()
	r.MustRegister(ollama.ToolDef{
		Name: "echo",
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			return fmt.Sprintf("echoed: %v", args["msg"]), nil
		},
	})
	result, err := r.Execute(context.Background(), "echo", map[string]interface{}{"msg": "hello"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "echoed: hello" {
		t.Errorf("result = %q, want 'echoed: hello'", result)
	}
}

func TestToolRegistry_Execute_Unknown(t *testing.T) {
	r := ollama.NewToolRegistry()
	_, err := r.Execute(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestBuiltinTools_ReadFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o644)
	r := ollama.NewToolRegistry()
	if err := ollama.RegisterBuiltinTools(r, dir); err != nil {
		t.Fatal(err)
	}
	result, err := r.Execute(context.Background(), "read_file", map[string]interface{}{"path": "hello.txt"})
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if result != "world" {
		t.Errorf("result = %q, want 'world'", result)
	}
}

func TestBuiltinTools_ReadFile_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	r := ollama.NewToolRegistry()
	ollama.RegisterBuiltinTools(r, dir)
	_, err := r.Execute(context.Background(), "read_file", map[string]interface{}{"path": "../../etc/passwd"})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestBuiltinTools_WriteFile(t *testing.T) {
	dir := t.TempDir()
	r := ollama.NewToolRegistry()
	ollama.RegisterBuiltinTools(r, dir)
	result, err := r.Execute(context.Background(), "write_file", map[string]interface{}{
		"path": "sub/new.txt", "content": "hello from write_file",
	})
	if err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if !strings.Contains(result, "Wrote") {
		t.Errorf("result = %q", result)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "sub", "new.txt"))
	if string(data) != "hello from write_file" {
		t.Errorf("written = %q", string(data))
	}
}

func TestBuiltinTools_ListDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), nil, 0o644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
	r := ollama.NewToolRegistry()
	ollama.RegisterBuiltinTools(r, dir)
	result, err := r.Execute(context.Background(), "list_dir", map[string]interface{}{})
	if err != nil {
		t.Fatalf("list_dir: %v", err)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("missing a.txt: %q", result)
	}
	if !strings.Contains(result, "subdir/") {
		t.Errorf("missing subdir/: %q", result)
	}
}

func TestBuiltinTools_Exec(t *testing.T) {
	dir := t.TempDir()
	r := ollama.NewToolRegistry()
	ollama.RegisterBuiltinTools(r, dir)
	result, err := r.Execute(context.Background(), "exec", map[string]interface{}{"command": "echo hello world"})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(result, "hello world") {
		t.Errorf("result = %q", result)
	}
}

func TestFormatToolCallSummary(t *testing.T) {
	s := ollama.FormatToolCallSummary("read_file", map[string]interface{}{"path": "/tmp/x"})
	if !strings.HasPrefix(s, "read_file(") {
		t.Errorf("summary = %q", s)
	}
	s2 := ollama.FormatToolCallSummary("no_args", nil)
	if s2 != "no_args()" {
		t.Errorf("summary = %q, want 'no_args()'", s2)
	}
}

func TestAgenticRunner_BasicRun(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			json.NewEncoder(w).Encode(ollama.ChatResponse{
				Model: "test", Done: true,
				Message: ollama.Message{Role: "assistant", ToolCalls: []ollama.ToolCall{
					{Function: ollama.ToolCallFunction{Name: "echo", Arguments: map[string]interface{}{"msg": "world"}}},
				}},
			})
		case 2:
			json.NewEncoder(w).Encode(ollama.ChatResponse{
				Model: "test", Done: true,
				Message: ollama.Message{Role: "assistant", Content: "The echo returned: world"},
			})
		default:
			t.Errorf("unexpected call %d", n)
		}
	}))
	defer srv.Close()
	client, _ := ollama.NewClient(srv.URL)
	reg := ollama.NewToolRegistry()
	reg.MustRegister(ollama.ToolDef{
		Name: "echo", Description: "Echo", Parameters: json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			return fmt.Sprintf("echoed: %v", args["msg"]), nil
		},
	})
	var toolCalls []string
	runner, err := ollama.NewAgenticRunner(ollama.AgentConfig{
		Client: client, Model: "test", Tools: reg, SystemPrompt: "You are a test agent.",
		OnToolCall: func(name string, args map[string]interface{}) { toolCalls = append(toolCalls, name) },
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), "say hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalContent != "The echo returned: world" {
		t.Errorf("FinalContent = %q", result.FinalContent)
	}
	if result.TurnsUsed != 1 {
		t.Errorf("TurnsUsed = %d, want 1", result.TurnsUsed)
	}
	if result.ToolCallCount != 1 {
		t.Errorf("ToolCallCount = %d, want 1", result.ToolCallCount)
	}
	if len(result.Messages) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(result.Messages))
	}
}

func TestAgenticRunner_MaxTurns(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 2 {
			json.NewEncoder(w).Encode(ollama.ChatResponse{
				Model: "test", Done: true,
				Message: ollama.Message{Role: "assistant", ToolCalls: []ollama.ToolCall{
					{Function: ollama.ToolCallFunction{Name: "echo", Arguments: map[string]interface{}{"msg": "again"}}},
				}},
			})
		} else {
			json.NewEncoder(w).Encode(ollama.ChatResponse{
				Model: "test", Done: true,
				Message: ollama.Message{Role: "assistant", Content: "Forced final response"},
			})
		}
	}))
	defer srv.Close()
	client, _ := ollama.NewClient(srv.URL)
	reg := ollama.NewToolRegistry()
	reg.MustRegister(ollama.ToolDef{
		Name: "echo", Handler: func(ctx context.Context, args map[string]interface{}) (string, error) { return "ok", nil },
	})
	runner, _ := ollama.NewAgenticRunner(ollama.AgentConfig{Client: client, Model: "test", Tools: reg, MaxTurns: 2})
	result, err := runner.Run(context.Background(), "loop forever")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.TurnsUsed != 2 {
		t.Errorf("TurnsUsed = %d, want 2", result.TurnsUsed)
	}
	if result.FinalContent != "Forced final response" {
		t.Errorf("FinalContent = %q", result.FinalContent)
	}
}

func TestAgenticRunner_NoToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollama.ChatResponse{
			Model: "test", Done: true,
			Message: ollama.Message{Role: "assistant", Content: "Direct answer: 42"},
		})
	}))
	defer srv.Close()
	client, _ := ollama.NewClient(srv.URL)
	reg := ollama.NewToolRegistry()
	reg.MustRegister(ollama.ToolDef{
		Name: "dummy", Handler: func(ctx context.Context, args map[string]interface{}) (string, error) { return "", nil },
	})
	runner, _ := ollama.NewAgenticRunner(ollama.AgentConfig{Client: client, Model: "test", Tools: reg})
	result, err := runner.Run(context.Background(), "What is 6*7?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.FinalContent != "Direct answer: 42" {
		t.Errorf("FinalContent = %q", result.FinalContent)
	}
	if result.TurnsUsed != 0 {
		t.Errorf("TurnsUsed = %d, want 0", result.TurnsUsed)
	}
}

func TestNewAgenticRunner_Validation(t *testing.T) {
	client := ollama.DefaultClient()
	reg := ollama.NewToolRegistry()
	reg.MustRegister(ollama.ToolDef{
		Name: "x", Handler: func(ctx context.Context, args map[string]interface{}) (string, error) { return "", nil },
	})
	tests := []struct {
		name   string
		config ollama.AgentConfig
	}{
		{"no_client", ollama.AgentConfig{Model: "test", Tools: reg}},
		{"no_model", ollama.AgentConfig{Client: client, Tools: reg}},
		{"no_tools", ollama.AgentConfig{Client: client, Model: "test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ollama.NewAgenticRunner(tt.config)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}
