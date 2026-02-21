# Tool Calling

This guide explains how `osm` implements tool calling (function calling)
with Ollama models, enabling AI agents to interact with the filesystem,
execute commands, and complete multi-step tasks autonomously.

## What is Tool Calling?

Tool calling allows an LLM to request the execution of specific functions
during a conversation. Instead of just generating text, the model can:

1. Analyze the user's request
2. Decide which tools to invoke (e.g., `read_file`, `exec`)
3. Provide structured arguments for the tool
4. Receive the tool's output
5. Continue reasoning with the new information

This creates an **agentic loop** where the model iterates between
thinking and acting until the task is complete.

## Architecture

```
User Request
    │
    ▼
┌──────────────┐
│ AgenticRunner │◄──── SystemPrompt + MaxTurns
│              │
│  ┌─────────┐ │     ┌──────────────┐
│  │  Chat   │─┼────►│ Ollama API   │
│  │ Request │ │     │ (tool_calls) │
│  └─────────┘ │     └──────┬───────┘
│              │            │
│  ┌─────────┐ │     ┌──────▼───────┐
│  │  Tool   │─┼────►│ ToolRegistry │
│  │ Execute │ │     │ (7 built-in) │
│  └─────────┘ │     └──────────────┘
│              │
│  Loop until: │
│  - No tools  │
│  - MaxTurns  │
└──────────────┘
    │
    ▼
AgentResult (FinalContent, Messages, TurnsUsed, ToolCallCount)
```

## Built-in Tools

Seven tools are registered by default via `RegisterBuiltinTools`:

| Tool | Parameters | Description |
|------|-----------|-------------|
| `read_file` | `path` | Read file contents relative to work directory |
| `write_file` | `path`, `content` | Write content to a file |
| `list_dir` | `path` | List directory contents |
| `exec` | `command` | Execute a shell command |
| `grep` | `pattern`, `path`, `flags` (optional) | Search for regex pattern in files |
| `git_diff` | `args` (optional) | Run `git diff` with optional arguments |
| `git_log` | `args` (optional) | Run `git log` with optional arguments |

**Safety features:**
- Path traversal protection on file tools (reject `../` escapes)
- Output truncation for large results
- Working directory scoping
- Note: `exec` runs arbitrary shell commands with no path restriction

## Using Tool Calling in Go

```go
import "github.com/joeycumines/one-shot-man/internal/builtin/ollama"

// Create client and tool registry
client, _ := ollama.NewClient("http://localhost:11434")
reg := ollama.NewToolRegistry()
ollama.RegisterBuiltinTools(reg, "/path/to/project")

// Configure and run
runner, _ := ollama.NewAgenticRunner(ollama.AgentConfig{
    Client:       client,
    Model:        "gpt-oss:20b-cloud",
    Tools:        reg,
    SystemPrompt: "You are a code review assistant.",
    MaxTurns:     10,
    OnToolCall: func(name string, args map[string]interface{}) {
        fmt.Printf("[tool] %s\n", ollama.FormatToolCallSummary(name, args))
    },
})

result, err := runner.Run(ctx, "Review the changes in main.go")
fmt.Println(result.FinalContent)
fmt.Printf("Used %d tools in %d turns\n", result.ToolCallCount, result.TurnsUsed)
```

## Using Tool Calling in JavaScript

```javascript
var ollama = require('osm:ollama');

var client = ollama.createClient('http://localhost:11434');
var reg = ollama.createToolRegistry();
ollama.registerBuiltinTools(reg, '.');

var agent = ollama.createAgent({
    client: client,
    model: 'gpt-oss:20b-cloud',
    tools: reg,
    systemPrompt: 'You are a code review assistant.',
    maxTurns: 10
});

agent.run('Review the changes in main.go').then(function(result) {
    log.info('Result:', result.finalContent);
    log.info('Tools used:', result.toolCallCount);
});
```

## Custom Tools

Register your own tools alongside or instead of built-ins:

```go
reg.Register(ollama.ToolDef{
    Name:        "search_docs",
    Description: "Search project documentation",
    Parameters:  json.RawMessage(`{
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "Search query"}
        },
        "required": ["query"]
    }`),
    Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
        query := args["query"].(string)
        return searchDocs(query), nil
    },
})
```

In JavaScript:

```javascript
reg.register({
    name: 'search_docs',
    description: 'Search project documentation',
    parameters: {
        type: 'object',
        properties: {
            query: { type: 'string', description: 'Search query' }
        },
        required: ['query']
    },
    handler: function(args) { return searchDocs(args.query); }
});
```

## Tool Filtering

Control which tools are available via configuration:

```
[claude-mux]
# Disable all built-in tools (use custom tools only)
ollama-tools-enabled false

# Or keep built-ins but restrict to specific tools
ollama-tools-enabled true
ollama-tools-allowlist read_file,grep,git_diff
```

When `ollama-tools-allowlist` is set, only the named tools are registered.
Tools not in the list are removed after registration via
`ToolRegistry.Remove()`.

## Model Requirements

Not all Ollama models support tool calling. Check capabilities:

```go
info, _ := client.ShowModel(ctx, "gpt-oss:20b-cloud")
if info.SupportsTools() {
    // Model supports tool calling
}
```

Models with the `"tools"` capability in their metadata support structured
tool calls. Models without this capability will ignore the `tools` field
in chat requests.

## Configuration

See [Ollama HTTP reference](reference/ollama.md#configuration) for the
complete list of configuration keys controlling tool calling behavior.

## Agentic Loop Limits

The `MaxTurns` parameter (config: `ollama-max-turns`, default: 10)
limits the number of tool-calling iterations. Each turn is one round of:
model response → tool execution → result injection.

If the model hasn't finished after `MaxTurns`, the runner returns the
last assistant message as `FinalContent` with `TurnsUsed == MaxTurns`.

## Error Handling

- **Tool execution errors** are sent back to the model as tool results
  with the error message. The model can decide how to proceed.
- **Network errors** (Ollama unreachable) abort the agentic loop.
- **Context cancellation** stops the loop immediately.
- **Invalid tool names** from the model are reported as errors in the
  tool result message.
