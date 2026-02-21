package ollama

import (
	"encoding/json"
	"fmt"
	"time"
)

// DefaultEndpoint is the default Ollama API base URL.
const DefaultEndpoint = "http://localhost:11434"

// Message represents a chat message in the Ollama API.
type Message struct {
	// Role is the message role: "system", "user", "assistant", or "tool".
	Role string `json:"role"`

	// Content is the text content of the message.
	Content string `json:"content"`

	// ToolCalls contains tool invocations requested by the assistant.
	// Only present when role is "assistant" and the model invokes tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// Images is an optional list of base64-encoded image data for multimodal models.
	Images []string `json:"images,omitempty"`
}

// ToolCall represents a tool invocation from the model.
type ToolCall struct {
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction contains the function name and arguments of a tool call.
type ToolCallFunction struct {
	// Name is the tool function name.
	Name string `json:"name"`

	// Arguments is the parsed arguments map from the model.
	Arguments map[string]interface{} `json:"arguments"`
}

// Tool describes a tool available to the model during chat completion.
type Tool struct {
	// Type is always "function" for Ollama tool calling.
	Type string `json:"type"`

	// Function contains the tool's name, description, and parameter schema.
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function for tool use.
type ToolFunction struct {
	// Name is the function name the model will reference.
	Name string `json:"name"`

	// Description explains what the function does (used by the model for selection).
	Description string `json:"description"`

	// Parameters is the JSON Schema describing the function's input parameters.
	Parameters json.RawMessage `json:"parameters"`
}

// ChatRequest is the request body for POST /api/chat.
type ChatRequest struct {
	// Model is the model name (e.g., "llama3.2", "gpt-oss:20b-cloud").
	Model string `json:"model"`

	// Messages is the conversation history.
	Messages []Message `json:"messages"`

	// Stream controls whether the response is streamed (NDJSON) or returned as a single JSON object.
	// When nil, defaults to Ollama's default (true). Set to ptr(false) for non-streaming.
	Stream *bool `json:"stream,omitempty"`

	// Tools is the list of tools available to the model.
	Tools []Tool `json:"tools,omitempty"`

	// Format constrains the output format. Use "json" for JSON mode.
	Format string `json:"format,omitempty"`

	// Options contains model-specific parameters (temperature, top_p, etc.).
	Options map[string]interface{} `json:"options,omitempty"`

	// KeepAlive controls how long the model stays loaded in memory (e.g., "5m", "0" to unload immediately).
	KeepAlive string `json:"keep_alive,omitempty"`
}

// ChatResponse is the response body from POST /api/chat.
type ChatResponse struct {
	// Model is the model that generated the response.
	Model string `json:"model"`

	// CreatedAt is the timestamp of the response.
	CreatedAt time.Time `json:"created_at"`

	// Message is the assistant's response message.
	Message Message `json:"message"`

	// Done indicates whether this is the final response in a stream.
	Done bool `json:"done"`

	// DoneReason explains why generation stopped (e.g., "stop", "length").
	DoneReason string `json:"done_reason,omitempty"`

	// TotalDuration is the total time spent generating the response (nanoseconds).
	TotalDuration int64 `json:"total_duration,omitempty"`

	// LoadDuration is the time spent loading the model (nanoseconds).
	LoadDuration int64 `json:"load_duration,omitempty"`

	// PromptEvalCount is the number of tokens in the prompt.
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`

	// PromptEvalDuration is the time spent evaluating the prompt (nanoseconds).
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`

	// EvalCount is the number of tokens generated.
	EvalCount int `json:"eval_count,omitempty"`

	// EvalDuration is the time spent generating tokens (nanoseconds).
	EvalDuration int64 `json:"eval_duration,omitempty"`
}

// Model represents a model from GET /api/tags.
type Model struct {
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    struct {
		ParentModel       string   `json:"parent_model"`
		Format            string   `json:"format"`
		Family            string   `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string   `json:"parameter_size"`
		QuantizationLevel string   `json:"quantization_level"`
	} `json:"details"`
}

// ModelListResponse is the response from GET /api/tags.
type ModelListResponse struct {
	Models []Model `json:"models"`
}

// ModelInfo is the response from POST /api/show.
type ModelInfo struct {
	// License is the model's license text.
	License string `json:"license,omitempty"`

	// Modelfile is the model's Modelfile content.
	Modelfile string `json:"modelfile,omitempty"`

	// Parameters is the model's parameter configuration.
	Parameters string `json:"parameters,omitempty"`

	// Template is the model's prompt template.
	Template string `json:"template,omitempty"`

	// Details contains model metadata.
	Details struct {
		ParentModel       string   `json:"parent_model"`
		Format            string   `json:"format"`
		Family            string   `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string   `json:"parameter_size"`
		QuantizationLevel string   `json:"quantization_level"`
	} `json:"details"`

	// ModelInfo contains extended model information.
	ModelInfoMap map[string]interface{} `json:"model_info,omitempty"`

	// Capabilities lists the model's capabilities (e.g., ["completion", "tools", "vision"]).
	Capabilities []string `json:"capabilities,omitempty"`
}

// HasCapability returns true if the model has the specified capability.
func (m ModelInfo) HasCapability(cap string) bool {
	for _, c := range m.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// SupportsTools returns true if the model supports tool calling.
func (m ModelInfo) SupportsTools() bool {
	return m.HasCapability("tools")
}

// RunningModel represents a currently loaded model from GET /api/ps.
type RunningModel struct {
	Name      string    `json:"name"`
	Model     string    `json:"model"`
	Size      int64     `json:"size"`
	Digest    string    `json:"digest"`
	ExpiresAt time.Time `json:"expires_at"`
	Details   struct {
		ParentModel       string   `json:"parent_model"`
		Format            string   `json:"format"`
		Family            string   `json:"family"`
		Families          []string `json:"families"`
		ParameterSize     string   `json:"parameter_size"`
		QuantizationLevel string   `json:"quantization_level"`
	} `json:"details"`
	SizeVRAM int64 `json:"size_vram"`
}

// RunningModelResponse is the response from GET /api/ps.
type RunningModelResponse struct {
	Models []RunningModel `json:"models"`
}

// VersionResponse is the response from GET /api/version.
type VersionResponse struct {
	Version string `json:"version"`
}

// OllamaError represents an error response from the Ollama API.
type OllamaError struct {
	// StatusCode is the HTTP status code.
	StatusCode int

	// Status is the HTTP status line (e.g., "404 Not Found").
	Status string

	// Body is the raw response body (may contain a JSON error object).
	Body string

	// ErrorMessage is the parsed error message from JSON {"error": "..."}, if present.
	ErrorMessage string
}

// Error implements the error interface.
func (e *OllamaError) Error() string {
	if e.ErrorMessage != "" {
		return fmt.Sprintf("ollama: %s: %s", e.Status, e.ErrorMessage)
	}
	return fmt.Sprintf("ollama: %s", e.Status)
}

// BoolPtr returns a pointer to a bool value. Convenience for setting ChatRequest.Stream.
func BoolPtr(b bool) *bool {
	return &b
}
