package claudemux

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/ollama"
)

// OllamaHTTPProvider implements Provider using the Ollama HTTP API with
// native tool-calling support. Unlike OllamaProvider (PTY-based), this
// provider communicates via the REST API and supports structured tool
// calls through the AgenticRunner.
//
// This provider does NOT require TUI model navigation — the model is
// specified directly via SpawnOpts.Model or the provider's default Model.
type OllamaHTTPProvider struct {
	// Endpoint is the Ollama server URL (default: "http://localhost:11434").
	Endpoint string
	// Model is the default model name. Can be overridden via SpawnOpts.Model.
	Model string
	// SystemPrompt is the system message for the agentic loop.
	SystemPrompt string
	// MaxTurns limits agentic loop iterations (default: 10).
	MaxTurns int
	// Timeout is the HTTP request timeout for Ollama API calls (default: 60s).
	Timeout time.Duration
	// ToolsEnabled controls whether built-in tools are registered (default: true).
	ToolsEnabled *bool
	// ToolsAllowlist is a comma-separated list of tool names to register.
	// Empty means all built-in tools. Only effective when ToolsEnabled is true.
	ToolsAllowlist string
}

// Name returns "ollama-http".
func (p *OllamaHTTPProvider) Name() string { return "ollama-http" }

// Capabilities returns the capabilities of the Ollama HTTP provider.
func (p *OllamaHTTPProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		MCP:       false, // Uses native tool calling, not MCP
		Streaming: true,  // Output streamed via callbacks
		MultiTurn: true,  // Agentic loop is inherently multi-turn
		ModelNav:  false, // Model specified via config, no TUI navigation
	}
}

// Spawn creates an AgentHandle that wraps the Ollama HTTP agentic loop.
// The handle buffers output from tool call/result/assistant callbacks and
// exposes it through the standard Send/Receive interface.
func (p *OllamaHTTPProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}

	var clientOpts []ollama.ClientOption
	if p.Timeout > 0 {
		clientOpts = append(clientOpts, ollama.WithTimeout(p.Timeout))
	}
	client, err := ollama.NewClient(endpoint, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("ollama-http: create client: %w", err)
	}

	model := opts.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return nil, fmt.Errorf("ollama-http: model is required (set via SpawnOpts.Model or provider default)")
	}

	dir := opts.Dir
	if dir == "" {
		dir = "."
	}

	reg := ollama.NewToolRegistry()
	toolsEnabled := p.ToolsEnabled == nil || *p.ToolsEnabled // default true
	if toolsEnabled {
		if err := ollama.RegisterBuiltinTools(reg, dir); err != nil {
			return nil, fmt.Errorf("ollama-http: register tools: %w", err)
		}
		// Apply allowlist filter if specified.
		if p.ToolsAllowlist != "" {
			allowed := make(map[string]bool)
			for _, name := range strings.Split(p.ToolsAllowlist, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					allowed[name] = true
				}
			}
			// Remove tools not in the allowlist.
			for _, name := range reg.Names() {
				if !allowed[name] {
					reg.Remove(name)
				}
			}
		}
	}

	maxTurns := p.MaxTurns
	if maxTurns == 0 {
		maxTurns = 10
	}

	systemPrompt := p.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful coding assistant. Use the available tools " +
			"to read files, execute commands, and complete the user's task."
	}

	config := ollama.AgentConfig{
		Client:       client,
		Model:        model,
		Tools:        reg,
		SystemPrompt: systemPrompt,
		MaxTurns:     maxTurns,
	}

	return newOllamaHTTPHandle(ctx, config)
}

// ollamaHTTPHandle wraps an AgenticRunner as an AgentHandle.
// Output from tool calls and assistant messages is buffered in a channel
// and drained by Receive().
type ollamaHTTPHandle struct {
	ctx    context.Context
	cancel context.CancelFunc
	runner *ollama.AgenticRunner
	output chan string
	done   chan struct{}

	mu       sync.Mutex
	err      error
	started  bool
	finished bool
}

func newOllamaHTTPHandle(ctx context.Context, config ollama.AgentConfig) (*ollamaHTTPHandle, error) {
	ctx, cancel := context.WithCancel(ctx)

	// Use a buffered channel to avoid blocking callbacks.
	h := &ollamaHTTPHandle{
		ctx:    ctx,
		cancel: cancel,
		output: make(chan string, 64),
		done:   make(chan struct{}),
	}

	// Wire callbacks to push output to channel.
	config.OnToolCall = func(name string, args map[string]interface{}) {
		summary := ollama.FormatToolCallSummary(name, args)
		select {
		case h.output <- fmt.Sprintf("[tool] %s\n", summary):
		case <-ctx.Done():
		}
	}
	config.OnToolResult = func(name, result string, err error) {
		var msg string
		if err != nil {
			msg = fmt.Sprintf("[tool-error] %s: %v\n", name, err)
		} else {
			preview := result
			if len(preview) > 200 {
				preview = preview[:200] + "..."
			}
			msg = fmt.Sprintf("[tool-result] %s: %s\n", name, preview)
		}
		select {
		case h.output <- msg:
		case <-ctx.Done():
		}
	}
	config.OnAssistantMessage = func(content string) {
		select {
		case h.output <- content + "\n":
		case <-ctx.Done():
		}
	}

	runner, err := ollama.NewAgenticRunner(config)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ollama-http: create runner: %w", err)
	}
	h.runner = runner

	return h, nil
}

// Send starts the agentic loop with the given input.
// Only the first call triggers execution; subsequent calls return an error.
func (h *ollamaHTTPHandle) Send(input string) error {
	h.mu.Lock()
	if h.started || h.finished {
		h.mu.Unlock()
		if h.finished {
			return fmt.Errorf("ollama-http: handle is closed")
		}
		return fmt.Errorf("ollama-http: already started")
	}
	h.started = true
	h.mu.Unlock()

	go func() {
		_, err := h.runner.Run(h.ctx, input)

		h.mu.Lock()
		h.err = err
		h.finished = true
		h.mu.Unlock()

		close(h.output)
		close(h.done)
	}()

	return nil
}

// Receive returns the next chunk of output or io.EOF when done.
// Blocks until output is available, the agent finishes, or context is cancelled.
func (h *ollamaHTTPHandle) Receive() (string, error) {
	select {
	case chunk, ok := <-h.output:
		if !ok {
			// Channel closed — agent finished.
			h.mu.Lock()
			err := h.err
			h.mu.Unlock()
			if err != nil {
				return "", err
			}
			return "", io.EOF
		}
		return chunk, nil
	case <-h.ctx.Done():
		return "", h.ctx.Err()
	}
}

// Close terminates the agent and releases resources.
func (h *ollamaHTTPHandle) Close() error {
	h.cancel()
	// Wait briefly for goroutine to finish.
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
	}
	return nil
}

// IsAlive returns whether the agent is still processing.
func (h *ollamaHTTPHandle) IsAlive() bool {
	select {
	case <-h.done:
		return false
	default:
		return true
	}
}

// Wait blocks until the agent finishes and returns the exit code.
// Returns 0 on success, 1 on error.
func (h *ollamaHTTPHandle) Wait() (int, error) {
	select {
	case <-h.done:
		h.mu.Lock()
		err := h.err
		h.mu.Unlock()
		if err != nil {
			return 1, err
		}
		return 0, nil
	case <-h.ctx.Done():
		return 1, h.ctx.Err()
	}
}
