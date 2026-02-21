package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolDef defines a tool that can be invoked by the agentic loop.
type ToolDef struct {
	// Name is the unique identifier for this tool (e.g., "read_file", "exec").
	Name string

	// Description explains what the tool does (shown to the model).
	Description string

	// Parameters is the JSON Schema describing the tool's input parameters.
	Parameters json.RawMessage

	// Handler executes the tool and returns the result as a string.
	// The arguments map contains the parsed arguments from the model's tool call.
	// The context should be respected for cancellation.
	Handler ToolHandler
}

// ToolHandler is the function signature for tool implementations.
type ToolHandler func(ctx context.Context, args map[string]interface{}) (string, error)

// ToOllamaTool converts this ToolDef to the Ollama API Tool format.
func (d *ToolDef) ToOllamaTool() Tool {
	return Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        d.Name,
			Description: d.Description,
			Parameters:  d.Parameters,
		},
	}
}

// ToolRegistry is a thread-safe registry of tool definitions.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolDef
	order []string // preserves insertion order
}

// NewToolRegistry creates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*ToolDef),
	}
}

// Register adds a tool definition to the registry.
// Returns an error if a tool with the same name is already registered.
func (r *ToolRegistry) Register(def ToolDef) error {
	if def.Name == "" {
		return fmt.Errorf("ollama: tool name must not be empty")
	}
	if def.Handler == nil {
		return fmt.Errorf("ollama: tool %q must have a handler", def.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[def.Name]; exists {
		return fmt.Errorf("ollama: tool %q already registered", def.Name)
	}

	copied := def
	r.tools[def.Name] = &copied
	r.order = append(r.order, def.Name)
	return nil
}

// MustRegister is like Register but panics on error.
func (r *ToolRegistry) MustRegister(def ToolDef) {
	if err := r.Register(def); err != nil {
		panic(err)
	}
}

// Get retrieves a tool by name. Returns nil if not found.
func (r *ToolRegistry) Get(name string) *ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// Has returns true if a tool with the given name is registered.
func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Names returns all registered tool names in insertion order.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// Len returns the number of registered tools.
func (r *ToolRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// OllamaTools returns all registered tools in the Ollama API format.
func (r *ToolRegistry) OllamaTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		if def, ok := r.tools[name]; ok {
			tools = append(tools, def.ToOllamaTool())
		}
	}
	return tools
}

// Execute invokes the handler for the named tool with the given arguments.
// Returns an error if the tool is not found or if the handler returns an error.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	r.mu.RLock()
	def, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("ollama: unknown tool %q", name)
	}

	return def.Handler(ctx, args)
}
