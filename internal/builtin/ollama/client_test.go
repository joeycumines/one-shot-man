package ollama_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/ollama"
)

// newTestServer creates an httptest.Server that dispatches based on method+path.
func newTestServer(t *testing.T, routes map[string]http.HandlerFunc) (*httptest.Server, *ollama.Client) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range routes {
		mux.HandleFunc(pattern, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := ollama.NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient(%q): %v", srv.URL, err)
	}
	return srv, c
}

func TestNewClient_DefaultEndpoint(t *testing.T) {
	c, err := ollama.NewClient("")
	if err != nil {
		t.Fatalf("NewClient empty: %v", err)
	}
	if c.BaseURL() != ollama.DefaultEndpoint {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), ollama.DefaultEndpoint)
	}
}

func TestNewClient_CustomEndpoint(t *testing.T) {
	c, err := ollama.NewClient("http://myhost:9999")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.BaseURL() != "http://myhost:9999" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), "http://myhost:9999")
	}
}

func TestNewClient_TrailingSlash(t *testing.T) {
	c, err := ollama.NewClient("http://myhost:9999/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.BaseURL() != "http://myhost:9999" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), "http://myhost:9999")
	}
}

func TestNewClient_InvalidURL(t *testing.T) {
	_, err := ollama.NewClient("://bad")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestNewClient_BadScheme(t *testing.T) {
	_, err := ollama.NewClient("ftp://localhost:11434")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "http or https") {
		t.Errorf("error = %q, want mention of http/https", err.Error())
	}
}

func TestDefaultClient(t *testing.T) {
	c := ollama.DefaultClient()
	if c == nil {
		t.Fatal("DefaultClient returned nil")
	}
	if c.BaseURL() != ollama.DefaultEndpoint {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), ollama.DefaultEndpoint)
	}
}

func TestHealth_OK(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("Ollama is running"))
		},
	})

	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestHealth_ServerDown(t *testing.T) {
	// Create and immediately close the server.
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	c, err := ollama.NewClient(url)
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error for closed server")
	}
}

func TestHealth_ServerError(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	})

	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestIsHealthy(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("Ollama is running"))
		},
	})

	if !c.IsHealthy(context.Background()) {
		t.Fatal("expected healthy")
	}
}

func TestVersion(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/version": func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, ollama.VersionResponse{Version: "0.6.2"})
		},
	})

	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v.Version != "0.6.2" {
		t.Errorf("Version = %q, want %q", v.Version, "0.6.2")
	}
}

func TestVersion_Error(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/version": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(t, w, map[string]string{"error": "internal error"})
		},
	})

	_, err := c.Version(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}

	ollamaErr, ok := err.(*ollama.OllamaError)
	if !ok {
		t.Fatalf("expected *OllamaError, got %T", err)
	}
	if ollamaErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", ollamaErr.StatusCode)
	}
	if ollamaErr.ErrorMessage != "internal error" {
		t.Errorf("ErrorMessage = %q, want %q", ollamaErr.ErrorMessage, "internal error")
	}
}

func TestListModels(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/tags": func(w http.ResponseWriter, _ *http.Request) {
			resp := ollama.ModelListResponse{
				Models: []ollama.Model{
					{Name: "llama3.2:latest", Size: 4_000_000_000},
					{Name: "qwen2.5:7b", Size: 7_000_000_000},
				},
			}
			writeJSON(t, w, resp)
		},
	})

	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2", len(models))
	}
	if models[0].Name != "llama3.2:latest" {
		t.Errorf("models[0].Name = %q, want %q", models[0].Name, "llama3.2:latest")
	}
	if models[1].Name != "qwen2.5:7b" {
		t.Errorf("models[1].Name = %q, want %q", models[1].Name, "qwen2.5:7b")
	}
}

func TestShowModel(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/show": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("unmarshal show request: %v", err)
			}
			if req.Name != "llama3.2" {
				t.Errorf("show name = %q, want %q", req.Name, "llama3.2")
			}

			info := ollama.ModelInfo{
				License:      "MIT",
				Capabilities: []string{"completion", "tools"},
			}
			writeJSON(t, w, info)
		},
	})

	info, err := c.ShowModel(context.Background(), "llama3.2")
	if err != nil {
		t.Fatalf("ShowModel: %v", err)
	}
	if info.License != "MIT" {
		t.Errorf("License = %q, want %q", info.License, "MIT")
	}
	if !info.SupportsTools() {
		t.Error("expected SupportsTools() == true")
	}
	if !info.HasCapability("completion") {
		t.Error("expected HasCapability(completion) == true")
	}
	if info.HasCapability("vision") {
		t.Error("expected HasCapability(vision) == false")
	}
}

func TestListRunning(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/ps": func(w http.ResponseWriter, _ *http.Request) {
			resp := ollama.RunningModelResponse{
				Models: []ollama.RunningModel{
					{Name: "llama3.2:latest", Size: 4_000_000_000, SizeVRAM: 2_000_000_000},
				},
			}
			writeJSON(t, w, resp)
		},
	})

	models, err := c.ListRunning(context.Background())
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if models[0].SizeVRAM != 2_000_000_000 {
		t.Errorf("SizeVRAM = %d, want 2000000000", models[0].SizeVRAM)
	}
}

func TestChat_NonStreaming(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req ollama.ChatRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("unmarshal chat: %v", err)
			}
			if req.Stream == nil || *req.Stream {
				t.Error("expected stream=false for non-streaming Chat")
			}
			if req.Model != "llama3.2" {
				t.Errorf("model = %q, want llama3.2", req.Model)
			}
			if len(req.Messages) != 1 {
				t.Errorf("len(messages) = %d, want 1", len(req.Messages))
			}

			resp := ollama.ChatResponse{
				Model: "llama3.2",
				Done:  true,
				Message: ollama.Message{
					Role:    "assistant",
					Content: "Hello, world!",
				},
				DoneReason: "stop",
				EvalCount:  42,
			}
			writeJSON(t, w, resp)
		},
	})

	resp, err := c.Chat(context.Background(), ollama.ChatRequest{
		Model: "llama3.2",
		Messages: []ollama.Message{
			{Role: "user", Content: "Say hello"},
		},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Message.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", resp.Message.Content, "Hello, world!")
	}
	if !resp.Done {
		t.Error("expected Done=true")
	}
	if resp.EvalCount != 42 {
		t.Errorf("EvalCount = %d, want 42", resp.EvalCount)
	}
}

func TestChat_WithTools(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req ollama.ChatRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("unmarshal: %v", err)
			}
			if len(req.Tools) != 1 {
				t.Errorf("len(tools) = %d, want 1", len(req.Tools))
			}
			if req.Tools[0].Function.Name != "get_weather" {
				t.Errorf("tool name = %q, want get_weather", req.Tools[0].Function.Name)
			}

			resp := ollama.ChatResponse{
				Model: "llama3.2",
				Done:  true,
				Message: ollama.Message{
					Role:    "assistant",
					Content: "",
					ToolCalls: []ollama.ToolCall{
						{
							Function: ollama.ToolCallFunction{
								Name:      "get_weather",
								Arguments: map[string]interface{}{"location": "Tokyo"},
							},
						},
					},
				},
			}
			writeJSON(t, w, resp)
		},
	})

	weatherSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"location": {"type": "string", "description": "City name"}
		},
		"required": ["location"]
	}`)

	resp, err := c.Chat(context.Background(), ollama.ChatRequest{
		Model: "llama3.2",
		Messages: []ollama.Message{
			{Role: "user", Content: "What's the weather in Tokyo?"},
		},
		Tools: []ollama.Tool{
			{
				Type: "function",
				Function: ollama.ToolFunction{
					Name:        "get_weather",
					Description: "Get current weather for a location",
					Parameters:  weatherSchema,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Chat with tools: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("tool call name = %q, want get_weather", tc.Function.Name)
	}
	loc, ok := tc.Function.Arguments["location"].(string)
	if !ok || loc != "Tokyo" {
		t.Errorf("location = %v, want Tokyo", tc.Function.Arguments["location"])
	}
}

func TestChat_ToolCallResponseRoundtrip(t *testing.T) {
	// Simulate a multi-turn tool-calling conversation:
	// 1. User asks -> model returns tool_call
	// 2. User sends tool result -> model returns final answer
	var callCount atomic.Int32

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req ollama.ChatRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("unmarshal: %v", err)
			}

			n := callCount.Add(1)
			switch n {
			case 1:
				// First call: return tool call
				writeJSON(t, w, ollama.ChatResponse{
					Model: "llama3.2",
					Done:  true,
					Message: ollama.Message{
						Role: "assistant",
						ToolCalls: []ollama.ToolCall{
							{Function: ollama.ToolCallFunction{
								Name:      "read_file",
								Arguments: map[string]interface{}{"path": "/tmp/test.txt"},
							}},
						},
					},
				})
			case 2:
				// Second call: verify tool result message is present
				foundToolMsg := false
				for _, msg := range req.Messages {
					if msg.Role == "tool" {
						foundToolMsg = true
						if msg.Content != "file contents here" {
							t.Errorf("tool content = %q, want 'file contents here'", msg.Content)
						}
					}
				}
				if !foundToolMsg {
					t.Error("expected tool role message in second call")
				}

				writeJSON(t, w, ollama.ChatResponse{
					Model: "llama3.2",
					Done:  true,
					Message: ollama.Message{
						Role:    "assistant",
						Content: "The file contains: file contents here",
					},
				})
			default:
				t.Errorf("unexpected call count: %d", n)
			}
		},
	})

	// Turn 1: user message -> tool call
	resp1, err := c.Chat(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "Read /tmp/test.txt"}},
		Tools: []ollama.Tool{{
			Type: "function",
			Function: ollama.ToolFunction{
				Name:        "read_file",
				Description: "Read a file",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			},
		}},
	})
	if err != nil {
		t.Fatalf("Turn 1: %v", err)
	}
	if len(resp1.Message.ToolCalls) != 1 {
		t.Fatalf("Turn 1: expected 1 tool call, got %d", len(resp1.Message.ToolCalls))
	}

	// Turn 2: send tool result
	msgs := []ollama.Message{
		{Role: "user", Content: "Read /tmp/test.txt"},
		resp1.Message, // assistant with tool_calls
		{Role: "tool", Content: "file contents here"},
	}
	resp2, err := c.Chat(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: msgs,
	})
	if err != nil {
		t.Fatalf("Turn 2: %v", err)
	}
	if resp2.Message.Content != "The file contains: file contents here" {
		t.Errorf("Turn 2 content = %q", resp2.Message.Content)
	}
}

func TestChat_HTTPError(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(t, w, map[string]string{"error": "model 'nonexistent' not found"})
		},
	})

	_, err := c.Chat(context.Background(), ollama.ChatRequest{
		Model:    "nonexistent",
		Messages: []ollama.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	ollamaErr, ok := err.(*ollama.OllamaError)
	if !ok {
		t.Fatalf("expected *OllamaError, got %T: %v", err, err)
	}
	if ollamaErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", ollamaErr.StatusCode)
	}
	if !strings.Contains(ollamaErr.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", ollamaErr.Error())
	}
}

func TestChatStream(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req ollama.ChatRequest
			_ = json.Unmarshal(body, &req)
			if req.Stream == nil || !*req.Stream {
				t.Error("expected stream=true for ChatStream")
			}

			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("expected Flusher")
			}

			chunks := []ollama.ChatResponse{
				{Model: "llama3.2", Message: ollama.Message{Role: "assistant", Content: "Hello"}, Done: false},
				{Model: "llama3.2", Message: ollama.Message{Role: "assistant", Content: " world"}, Done: false},
				{Model: "llama3.2", Message: ollama.Message{Role: "assistant", Content: "!"}, Done: true, DoneReason: "stop", EvalCount: 3},
			}

			for _, chunk := range chunks {
				data, _ := json.Marshal(chunk)
				_, _ = w.Write(data)
				_, _ = w.Write([]byte("\n"))
				flusher.Flush()
			}
		},
	})

	reader, err := c.ChatStream(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = reader.Close() }()

	var parts []string
	for {
		chunk, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		parts = append(parts, chunk.Message.Content)
		if chunk.Done {
			if chunk.EvalCount != 3 {
				t.Errorf("final EvalCount = %d, want 3", chunk.EvalCount)
			}
		}
	}

	combined := strings.Join(parts, "")
	if combined != "Hello world!" {
		t.Errorf("streamed content = %q, want %q", combined, "Hello world!")
	}
}

func TestChatStream_Error(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(t, w, map[string]string{"error": "bad request"})
		},
	})

	_, err := c.ChatStream(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for bad request")
	}
}

func TestChatStream_CloseIdempotent(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			data, _ := json.Marshal(ollama.ChatResponse{
				Model:   "llama3.2",
				Done:    true,
				Message: ollama.Message{Role: "assistant", Content: "done"},
			})
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n"))
		},
	})

	reader, err := c.ChatStream(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	// Close multiple times — should not panic.
	for i := 0; i < 3; i++ {
		if err := reader.Close(); err != nil {
			t.Errorf("Close() #%d: %v", i+1, err)
		}
	}

	// Reading after close.
	_, err = reader.Next()
	if err == nil {
		t.Fatal("expected error reading after close")
	}
}

func TestChatStream_ReaderAfterDone(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			data, _ := json.Marshal(ollama.ChatResponse{
				Model:   "llama3.2",
				Done:    true,
				Message: ollama.Message{Role: "assistant", Content: "done"},
			})
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n"))
		},
	})

	reader, err := c.ChatStream(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	defer func() { _ = reader.Close() }()

	// Read the done message.
	chunk, err := reader.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !chunk.Done {
		t.Error("expected Done=true")
	}

	// Next call should return EOF.
	_, err = reader.Next()
	if err != io.EOF {
		t.Errorf("expected io.EOF after done, got %v", err)
	}
}

func TestChat_ContextCancelled(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, _ *http.Request) {
			// Delay to allow cancellation.
			time.Sleep(5 * time.Second)
			writeJSON(t, w, ollama.ChatResponse{})
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := c.Chat(ctx, ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestOllamaError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  ollama.OllamaError
		want string
	}{
		{
			name: "with_error_message",
			err:  ollama.OllamaError{Status: "404 Not Found", ErrorMessage: "model not found"},
			want: "ollama: 404 Not Found: model not found",
		},
		{
			name: "without_error_message",
			err:  ollama.OllamaError{Status: "500 Internal Server Error"},
			want: "ollama: 500 Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBoolPtr(t *testing.T) {
	p := ollama.BoolPtr(true)
	if p == nil || !*p {
		t.Error("BoolPtr(true) should return *true")
	}
	p2 := ollama.BoolPtr(false)
	if p2 == nil || *p2 {
		t.Error("BoolPtr(false) should return *false")
	}
}

func TestModelInfo_Capabilities(t *testing.T) {
	m := ollama.ModelInfo{
		Capabilities: []string{"completion", "tools", "vision"},
	}

	if !m.SupportsTools() {
		t.Error("expected SupportsTools() == true")
	}
	if !m.HasCapability("vision") {
		t.Error("expected HasCapability(vision) == true")
	}
	if m.HasCapability("nonexistent") {
		t.Error("expected HasCapability(nonexistent) == false")
	}

	// Empty capabilities.
	m2 := ollama.ModelInfo{}
	if m2.SupportsTools() {
		t.Error("empty capabilities should not support tools")
	}
}

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 5 * time.Second}
	c, err := ollama.NewClient("http://localhost:11434", ollama.WithHTTPClient(custom))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	// We can't inspect the client directly, but at least verify no panic.
	_ = c
}

func TestWithTimeout(t *testing.T) {
	c, err := ollama.NewClient("http://localhost:11434", ollama.WithTimeout(30*time.Second))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c
}

func TestChat_WithOptions(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/chat": func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var req ollama.ChatRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("unmarshal: %v", err)
			}
			if req.Options == nil {
				t.Error("expected options to be set")
			}
			temp, ok := req.Options["temperature"]
			if !ok {
				t.Error("expected temperature in options")
			}
			if tempFloat, ok := temp.(float64); !ok || tempFloat != 0.7 {
				t.Errorf("temperature = %v, want 0.7", temp)
			}

			writeJSON(t, w, ollama.ChatResponse{
				Model:   "llama3.2",
				Done:    true,
				Message: ollama.Message{Role: "assistant", Content: "ok"},
			})
		},
	})

	_, err := c.Chat(context.Background(), ollama.ChatRequest{
		Model:    "llama3.2",
		Messages: []ollama.Message{{Role: "user", Content: "hi"}},
		Options:  map[string]interface{}{"temperature": 0.7},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

// writeJSON is a test helper that writes a JSON response.
func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("writeJSON: %v", err)
	}
}
