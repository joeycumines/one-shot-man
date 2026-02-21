package ollama

import (
	"context"
	"fmt"
	"strings"
)

// AgentConfig configures the agentic execution loop.
type AgentConfig struct {
	// Client is the Ollama API client to use.
	Client *Client

	// Model is the model name (e.g., "llama3.2").
	Model string

	// Tools is the tool registry containing available tools.
	Tools *ToolRegistry

	// SystemPrompt is an optional system message prepended to the conversation.
	SystemPrompt string

	// MaxTurns is the maximum number of tool-calling rounds.
	// 0 means no limit (will loop until the model stops calling tools).
	MaxTurns int

	// Options contains model-specific parameters (temperature, top_p, etc.).
	Options map[string]interface{}

	// OnToolCall is called when the model invokes a tool. Optional callback for logging/UI.
	OnToolCall func(name string, args map[string]interface{})

	// OnToolResult is called after a tool completes. Optional callback for logging/UI.
	OnToolResult func(name string, result string, err error)

	// OnAssistantMessage is called when the model produces a text response. Optional callback for logging/UI.
	OnAssistantMessage func(content string)
}

// AgentResult contains the final result of an agentic run.
type AgentResult struct {
	// FinalContent is the model's final text response.
	FinalContent string

	// Messages is the full conversation history including tool calls and results.
	Messages []Message

	// TurnsUsed is the number of tool-calling rounds that occurred.
	TurnsUsed int

	// ToolCallCount is the total number of individual tool calls made.
	ToolCallCount int
}

// AgenticRunner orchestrates a multi-turn tool-calling loop with an Ollama model.
//
// The loop works as follows:
//  1. Send the conversation (system + user messages) to the model with available tools.
//  2. If the model responds with tool_calls, execute each tool via the registry.
//  3. Append the assistant message (with tool_calls) and tool results to the conversation.
//  4. Send the updated conversation back to the model.
//  5. Repeat until the model produces a final text response (no tool_calls) or MaxTurns is reached.
type AgenticRunner struct {
	config AgentConfig
}

// NewAgenticRunner creates a new agentic runner with the given configuration.
func NewAgenticRunner(config AgentConfig) (*AgenticRunner, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("ollama: agent requires a Client")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("ollama: agent requires a Model")
	}
	if config.Tools == nil {
		return nil, fmt.Errorf("ollama: agent requires a ToolRegistry")
	}

	return &AgenticRunner{config: config}, nil
}

// Run executes the agentic loop starting with the given user message.
// It returns the final result including the complete conversation history.
func (a *AgenticRunner) Run(ctx context.Context, userMessage string) (*AgentResult, error) {
	messages := a.buildInitialMessages(userMessage)
	return a.RunWithMessages(ctx, messages)
}

// RunWithMessages executes the agentic loop with an existing conversation.
// This allows continuing a previous conversation or providing multi-message context.
func (a *AgenticRunner) RunWithMessages(ctx context.Context, messages []Message) (*AgentResult, error) {
	result := &AgentResult{
		Messages: messages,
	}

	ollamaTools := a.config.Tools.OllamaTools()

	for {
		// Check max turns.
		if a.config.MaxTurns > 0 && result.TurnsUsed >= a.config.MaxTurns {
			// Force a final response without tools.
			resp, err := a.config.Client.Chat(ctx, ChatRequest{
				Model:    a.config.Model,
				Messages: result.Messages,
				Options:  a.config.Options,
				// No tools — force text response.
			})
			if err != nil {
				return result, fmt.Errorf("ollama: agent final turn: %w", err)
			}

			result.Messages = append(result.Messages, resp.Message)
			result.FinalContent = resp.Message.Content

			if a.config.OnAssistantMessage != nil {
				a.config.OnAssistantMessage(resp.Message.Content)
			}

			return result, nil
		}

		// Send chat request with tools.
		resp, err := a.config.Client.Chat(ctx, ChatRequest{
			Model:    a.config.Model,
			Messages: result.Messages,
			Tools:    ollamaTools,
			Options:  a.config.Options,
		})
		if err != nil {
			return result, fmt.Errorf("ollama: agent chat: %w", err)
		}

		// Append assistant message to conversation.
		result.Messages = append(result.Messages, resp.Message)

		// If no tool calls, this is the final response.
		if len(resp.Message.ToolCalls) == 0 {
			result.FinalContent = resp.Message.Content

			if a.config.OnAssistantMessage != nil {
				a.config.OnAssistantMessage(resp.Message.Content)
			}

			return result, nil
		}

		// Process tool calls.
		result.TurnsUsed++
		for _, tc := range resp.Message.ToolCalls {
			result.ToolCallCount++

			if a.config.OnToolCall != nil {
				a.config.OnToolCall(tc.Function.Name, tc.Function.Arguments)
			}

			toolResult, toolErr := a.config.Tools.Execute(ctx, tc.Function.Name, tc.Function.Arguments)

			if a.config.OnToolResult != nil {
				a.config.OnToolResult(tc.Function.Name, toolResult, toolErr)
			}

			// Build tool result message.
			var content string
			if toolErr != nil {
				content = fmt.Sprintf("Error executing %s: %v", tc.Function.Name, toolErr)
			} else {
				content = toolResult
			}

			// Truncate very long tool results.
			const maxToolResult = 50_000
			if len(content) > maxToolResult {
				content = content[:maxToolResult] + "\n... (result truncated)"
			}

			result.Messages = append(result.Messages, Message{
				Role:    "tool",
				Content: content,
			})
		}
	}
}

// buildInitialMessages constructs the starting message list.
func (a *AgenticRunner) buildInitialMessages(userMessage string) []Message {
	var messages []Message

	if a.config.SystemPrompt != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: a.config.SystemPrompt,
		})
	}

	messages = append(messages, Message{
		Role:    "user",
		Content: userMessage,
	})

	return messages
}

// FormatToolCallSummary creates a human-readable summary of a tool call for logging.
func FormatToolCallSummary(name string, args map[string]interface{}) string {
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	if len(parts) == 0 {
		return name + "()"
	}
	return name + "(" + strings.Join(parts, ", ") + ")"
}
