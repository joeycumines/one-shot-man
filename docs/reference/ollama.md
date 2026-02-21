# Ollama HTTP Reference

The `osm:ollama` module provides a complete Go client, tool registry, and
agentic runner for the [Ollama](https://ollama.ai/) REST API. It supports
native tool calling, streaming, and multi-turn agentic workflows.

## Package: `internal/builtin/ollama`

### Client

`Client` is an HTTP client for the Ollama REST API.

```go
client, err := ollama.NewClient("http://localhost:11434")
client, err := ollama.NewClient("", ollama.WithTimeout(30*time.Second))
```

**Constructor options:**

| Option | Description |
|--------|-------------|
| `WithHTTPClient(c)` | Use a custom `*http.Client` |
| `WithTimeout(d)` | Set HTTP request timeout |

**Methods:**

| Method | Description |
|--------|-------------|
| `Chat(ctx, req)` | Send a chat completion request |
| `ChatStream(ctx, req)` | Start a streaming chat (returns `ChatStreamReader`) |
| `ListModels(ctx)` | List locally available models |
| `ShowModel(ctx, name)` | Show model details (capabilities, parameters) |
| `ListRunning(ctx)` | List currently running models |
| `Version(ctx)` | Get Ollama server version |
| `Health(ctx)` | Check server health (GET /) |
| `IsHealthy(ctx)` | Returns `bool` — true if health check succeeds |

### ChatStreamReader

Returned by `ChatStream`. Read chunks via `Next()`.

```go
reader, err := client.ChatStream(ctx, req)
defer reader.Close()
for {
    resp, err := reader.Next()
    if err == io.EOF { break }
    if err != nil { return err }
    fmt.Print(resp.Message.Content)
}
```

### Types

**Request/Response:**

| Type | Key Fields |
|------|------------|
| `ChatRequest` | Model, Messages, Tools, Stream, Options |
| `ChatResponse` | Message, Done, TotalDuration, EvalCount |
| `Message` | Role, Content, ToolCalls |
| `ToolCall` | Function (Name, Arguments) |

**Model info:**

| Type | Key Fields |
|------|------------|
| `ModelListResponse` | Models []Model |
| `Model` | Name, Size, ModifiedAt |
| `ModelInfo` | Modelfile, Template, Parameters, Capabilities |
| `RunningModel` | Name, Size, ExpiresAt |
| `VersionResponse` | Version |

**Capabilities:** `ModelInfo.HasCapability(name)` checks for capability
strings like `"completion"`, `"tools"`, `"thinking"`.
`ModelInfo.SupportsTools()` is a convenience for `HasCapability("tools")`.

### Tool Registry

`ToolRegistry` is a thread-safe, ordered registry of tool definitions.

```go
reg := ollama.NewToolRegistry()
reg.Register(ollama.ToolDef{
    Name:        "my_tool",
    Description: "Does something",
    Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
    Handler:     func(ctx context.Context, args map[string]interface{}) (string, error) {
        return "result", nil
    },
})
```

**Methods:**

| Method | Description |
|--------|-------------|
| `Register(def)` | Add a tool (error if duplicate) |
| `MustRegister(def)` | Add a tool (panics on error) |
| `Get(name)` | Retrieve a tool by name |
| `Has(name)` | Check if a tool exists |
| `Names()` | Get all names in insertion order |
| `Len()` | Count of registered tools |
| `Remove(name)` | Remove a tool (no-op if missing) |
| `OllamaTools()` | Convert to Ollama API `[]Tool` format |
| `Execute(ctx, name, args)` | Invoke a tool handler |

### Built-in Tools

`RegisterBuiltinTools(reg, workDir)` adds 7 standard tools:

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents (relative to workDir) |
| `write_file` | Write content to a file |
| `list_dir` | List files and directories |
| `exec` | Execute a shell command |
| `grep` | Search for patterns in files |
| `git_diff` | Show git diff output |
| `git_log` | Show git log output |

All file-based tools enforce path traversal protection and truncate large outputs.
The `exec` tool runs arbitrary shell commands scoped to the working directory.

### Agentic Runner

`AgenticRunner` executes a multi-turn tool-calling loop:

1. Send user message → model responds
2. If response contains tool calls → execute tools → send results back
3. Repeat until model responds with no tool calls or max turns reached

```go
config := ollama.AgentConfig{
    Client:       client,
    Model:        "gpt-oss:20b-cloud",
    Tools:        reg,
    SystemPrompt: "You are a helpful assistant.",
    MaxTurns:     10,
    OnToolCall:   func(name string, args map[string]interface{}) { ... },
    OnToolResult: func(name, result string, err error) { ... },
    OnAssistantMessage: func(content string) { ... },
}
runner, err := ollama.NewAgenticRunner(config)
result, err := runner.Run(ctx, "Read main.go and summarize it")
```

**AgentResult:**

| Field | Description |
|-------|-------------|
| `FinalContent` | Last assistant message content |
| `Messages` | Full conversation history |
| `TurnsUsed` | Number of agentic loop iterations |
| `ToolCallCount` | Total tool invocations |

`FormatToolCallSummary(name, args)` produces a human-readable one-liner.

## JavaScript Module: `osm:ollama`

Available in the Goja scripting runtime via `require('osm:ollama')`.

### API

```javascript
var ollama = require('osm:ollama');

// Client
var client = ollama.createClient("http://localhost:11434");
var client = ollama.createClient("", { timeout: 30 }); // seconds

// Promises
client.version()       // → { version: "0.16.2" }
client.listModels()    // → [{ name, model, size, digest, modifiedAt }, ...]
client.showModel(name) // → { modelfile, template, parameters, capabilities, supportsTools, hasCapability(name) }
client.listRunning()   // → [{ name, model, size, digest }, ...]
client.health()        // → undefined (rejects on failure)
client.isHealthy()     // → true/false
client.chat(request)   // → { message, done, totalDuration, ... }

// Tool registry
var reg = ollama.createToolRegistry();
reg.register({
    name: "my_tool",
    description: "Does something",
    parameters: { type: "object", properties: {} },
    handler: function(args) { return "result"; }
});
ollama.registerBuiltinTools(reg, "."); // Register all 7 built-in tools

// Agentic runner
var agent = ollama.createAgent({
    client: client,
    model: "gpt-oss:20b-cloud",
    tools: reg,
    systemPrompt: "You are a helpful assistant.",
    maxTurns: 10
});
agent.run("Read main.go and summarize it")  // → Promise<AgentResult>

ollama.formatToolCallSummary("read_file", { path: "main.go" });
// → 'read_file(path=main.go)'
```

## Configuration

All keys live in the `[claude-mux]` config section:

```
[claude-mux]
ollama-endpoint http://localhost:11434
ollama-model gpt-oss:20b-cloud
ollama-timeout 60s
ollama-max-turns 10
ollama-system-prompt You are a helpful coding assistant.
ollama-tools-enabled true
ollama-tools-allowlist read_file,grep,git_diff
```

| Key | Type | Default | Env Var | Description |
|-----|------|---------|---------|-------------|
| `ollama-endpoint` | string | `http://localhost:11434` | `OSM_OLLAMA_ENDPOINT` | Ollama server URL |
| `ollama-model` | string | (empty) | `OSM_OLLAMA_MODEL` | Model name for ollama-http provider |
| `ollama-timeout` | duration | `60s` | — | HTTP request timeout |
| `ollama-max-turns` | int | `10` | — | Max agentic loop iterations (≥1) |
| `ollama-system-prompt` | string | (empty) | — | Custom system prompt |
| `ollama-tools-enabled` | bool | `true` | — | Register built-in tools |
| `ollama-tools-allowlist` | string | (empty) | — | Comma-separated tool names (empty = all) |

**Model resolution order** (CLI flag → ollama-model → generic model):
1. `--model` CLI flag (highest priority)
2. `ollama-model` config key
3. `model` generic config key (fallback)

## Provider: `ollama-http`

The `ollama-http` provider bridges the Ollama HTTP client and agentic
runner into the claudemux Provider/AgentHandle interface.

```bash
osm claude-mux run --provider ollama-http --model gpt-oss:20b-cloud "Refactor main.go"
```

Unlike the PTY-based `ollama` provider, `ollama-http`:
- Communicates via REST API (no terminal emulation)
- Supports native structured tool calling
- Does not require TUI model navigation
- Uses the agentic runner for multi-turn workflows

## Integration Testing

Run integration tests against a live Ollama server:

```bash
make integration-ollama-http
# Or with custom model/endpoint:
MODEL=llama3.2 ENDPOINT=http://gpu-box:11434 make integration-ollama-http
```

Tests require the `-integration` flag and exercise: health, version,
model listing, model info, basic chat, tool calling, agentic loop,
and streaming.
