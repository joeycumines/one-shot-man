# WIP — Session State (Takumi's Desperate Diary)

## Session Start
- **Started:** 2026-03-01 12:57:55
- **Mandate:** 9 hours minimum (ends ~2026-03-01 21:57:55)
- **Start file:** scratch/.session-start (contains epoch seconds)

## Last Commit
- **Hash:** 2dccbc6
- **Subject:** feat(mcpcallback): add Go-native tool registration and wire automatedSplit IPC

## Blueprint State
- **Total tasks:** 46 (T1-T46)
- **ALL DONE:** T1-T46 (46/46 tasks complete)

## Current Task
- **Active:** Rule of Two verification gate → Commit
- **All tasks complete. Awaiting Rule of Two + commit.**

## Verification Status
- `make` (build+lint+test): GREEN — all 49 packages pass
- Stale references: CLEANED (renamed TestPollForFile_MinInterval, fixed PTY skip messages)
- .deadcodeignore: UNCHANGED
- go.mod tidy: CLEAN (no-op)
- CHANGELOG: UPDATED with T29-T40 features

## NO COMMITS YET — Rule of Two required before first commit

### Category C: `sh -c` Shell Execution (RUNTIME)
| File | Lines (approx) | Sites |
|------|----------------|-------|
| `internal/command/pr_split_script.js` | 1219, 1539, 1565, 1596, 1625, 1648, 1653, 1659, 1663, 1665, 1779, 1891, 1938 | 13 sites calling `exec.execv(['sh', '-c', ...])`. Also uses `timeout` utility at line 1216. NOTE: GitHub Actions windows-latest has Git Bash in PATH, so `sh` may be available. Tests skip via pr_split_test.go guards. |

### Category D: `which` Command Usage (RUNTIME)
| File | Lines (approx) | Sites |
|------|----------------|-------|
| `internal/command/pr_split_script.js` | 1594, 2006, 2015, 2031 | 4 sites using `exec.execv(['which', ...])`. Windows uses `where.exe` instead. |

### Category E: Unix Utilities in Shell Commands (RUNTIME)
| File | Lines (approx) | Issue |
|------|----------------|-------|
| `internal/command/pr_split_script.js` | 1596 | `find . -name "*.go" -exec goimports -w {} +` — Unix-only |
| `internal/command/pr_split_script.js` | 1653 | `grep -rl ... \| head -1` — Unix-only |

### Already Properly Handled
- `internal/termmux/` — proper `//go:build` tags (platform_windows.go, resize_windows.go)
- `internal/storage/` — proper platform files (filelock_windows.go, atomic_write_windows.go)
- `internal/session/` — proper platform files (session_windows.go)
- `internal/builtin/pty/` — proper build tags (pty_windows.go returns ErrNotSupported)
- `internal/builtin/claudemux/control_test.go` — 5 tests properly skip on Windows
- `internal/builtin/claudemux/provider_test.go` — 3 tests properly skip (PTY)
- `internal/builtin/claudemux/pr_split_test.go` — skips "PR split uses sh -c"

## Completed This Session

## T10 Stdout Pollution Investigation — COMPLETE

### Initialization Call Sequence
```
ScriptingCommand.Execute(ctx, stdout, stderr)
  → PrepareEngine(ctx, stdout, stderr)
    → NewEngineDetailed(ctx, stdout, stderr, ...)
      → NewTerminalIOStdio()            // creates TUIWriter wrapping os.Stdout
      → NewTUILogger(stdout, ...)        // ring buffer + optional file, NOT stdout
      → NewTUIManagerWithConfig(ctx, ... terminalIO.TUIWriter, sessionID, store)
        → initializeStateManager(sessionID, store)
```

### Key Architectural Finding: stdout Parameter Divorce
The original `stdout` parameter is **NOT** passed to TUI manager. Instead:
- `NewTerminalIOStdio()` creates a fresh `TUIWriter` → `prompt.NewStdoutWriter()` → **os.Stdout directly**
- Engine's `TUILogger` receives original `stdout` param for log routing, but logs go to ring buffer during init
- Result: TUI output always goes to real os.Stdout, bypassing any test buffer or redirect

### Stdout Writes During Initialization

| # | File | Line | Content | Stream | MCP Risk | Classification |
|---|------|------|---------|--------|----------|----------------|
| 1 | `tui_manager.go` | 114 | `"Warning: Failed to initialize state persistence (session %q): %v\n"` | **stdout** (via TUIWriter) | **HIGH** | Informational — can suppress |
| 2 | `state_manager.go` | 82 | `"WARNING: Session schema version mismatch..."` | stderr (Go log.Printf) | Low | Informational — stderr OK |
| 3 | `engine_core.go` | 482 | Panic stack trace | stderr | Low | Necessary — stderr OK |
| 4 | Startup logger | validateModulePaths | Invalid path warnings | stderr | Low | Informational — stderr OK |

### Critical Finding
**Only ONE stdout write during init: `tui_manager.go:114`** — writes to TUIWriter (wraps os.Stdout) when primary state persistence backend fails. This is conditional (requires storage failure) but when it fires, it would corrupt MCP stdio JSON-RPC protocol.

### TUILogger is Safe During Init
- Stores logs in memory ring buffer via slog handler
- `PrintToTUI()` writes to `l.tuiWriter` (original stdout) but is only called from user script code, not init
- Default slog handler does NOT write to stdout — writes to file handler if configured, otherwise buffer only

### Fix Strategy (for T11-T12)
- Route tui_manager.go:114 warning through logger or stderr instead of TUIWriter
- Consider: `--quiet-init` flag or `output.setSuppressInit(true)` JS API
- Simplest fix: Change `fmt.Fprintf(output, ...)` to `fmt.Fprintf(os.Stderr, ...)` at line 114

## T11 Design: Configurable Stdout Control for osm script

### Analysis
T10 found exactly **1 stdout write** during initialization: `tui_manager.go:114`. All other init writes already go to stderr or the log ring buffer. This substantially reduces the design scope.

### Options Considered

| Option | Complexity | Backward Compat | Correct? |
|--------|-----------|-----------------|----------|
| A: `--quiet-init` CLI flag | Medium | Yes (opt-in) | Overkill for 1 write |
| B: `output.setSuppressInit(true)` JS API | High | Yes (opt-in) | Overkill, fires after init |
| C: Route warning to stderr | **Minimal** | **Yes** — warnings on stderr are Unix convention | **Yes** |
| D: Route warning through slog | Medium | Yes | Over-engineered |

### Chosen: Option C — Route warning to stderr

**Rationale:**
1. There is exactly one stdout write during init. A whole suppression framework is unwarranted.
2. The write is a WARNING about storage failure. Unix convention: warnings and errors go to stderr.
3. No API surface changes. No config flags. No backward compatibility concerns.
4. Existing scripts see no behavior change (they weren't expecting this warning on stdout anyway — it only fires on storage failure).
5. MCP stdio is immediately clean — zero stdout bytes before user script.

### Changes Required

| File | Change |
|------|--------|
| `internal/scripting/tui_manager.go:114` | `fmt.Fprintf(output, "Warning: ...")` → `fmt.Fprintf(os.Stderr, "Warning: ...")` |

### Initialization Sequence (After Fix)
```
ScriptingCommand.Execute()
  → PrepareEngine()
    → NewEngineDetailed()
      → NewTerminalIOStdio()          // TUIWriter → os.Stdout
      → NewTUILogger(stdout, ...)      // ring buffer, no stdout writes
      → NewTUIManagerWithConfig()
        → initializeStateManager()
          → storage failure?
            → YES: fmt.Fprintf(os.Stderr, "Warning:...") [STDERR, not stdout]
            → NO: proceed silently
      → setupGlobals()
  → engine.ExecuteScript(script)       // FIRST stdout write is user-controlled
```

### Backward Compatibility Guarantee
- **Before fix:** Warning goes to stdout (via TUIWriter). Only fires on storage failure (rare).
- **After fix:** Warning goes to stderr. Any tool capturing stdout sees no behavior change for normal operation. The warning was never part of the "API surface" — it's an error condition.
- **Test impact:** No tests rely on this warning appearing on stdout. Tests that mock storage failures would need to capture stderr to see it.

### Future-Proofing
If new stdout writes are introduced during init in the future, this design decision establishes the precedent: **all diagnostic/warning/error output during initialization goes to stderr**. The engine's stdout is reserved for user-controlled output only.

## T14 Design: osm:mcp Module API

### JavaScript API Surface

```javascript
const mcp = require('osm:mcp');

// 1. Create server
const server = mcp.createServer('my-server', '1.0.0');

// 2. Register tools (before run())
server.addTool({
  name: 'greet',
  description: 'Greet a user',
  inputSchema: {
    type: 'object',
    properties: {
      name: { type: 'string', description: 'Name to greet' }
    },
    required: ['name']
  }
}, async (input) => {
  // input.name is already parsed from JSON
  return { text: `Hello, ${input.name}!` };
});

// 3. Start serving (blocking — returns Promise)
await server.run('stdio');  // binds to process stdin/stdout

// 4. Close (graceful shutdown)
server.close();
```

### Type Signatures

```typescript
// Module export
interface MCPModule {
  createServer(name: string, version: string): MCPServer;
}

// Server object
interface MCPServer {
  // Register a tool. Must be called before run().
  addTool(toolDef: ToolDefinition, handler: ToolHandler): void;

  // Start serving on the given transport. Blocks (returns Promise).
  // transport: 'stdio' (reads stdin, writes stdout)
  run(transport: 'stdio'): Promise<void>;

  // Stop the server gracefully. Resolves the run() promise.
  close(): void;
}

interface ToolDefinition {
  name: string;
  description: string;
  inputSchema: JSONSchema;         // JSON Schema object
}

// Handler receives parsed input, returns result
type ToolHandler = (input: Record<string, any>) => Promise<ToolResult> | ToolResult;

interface ToolResult {
  text?: string;                   // Text content response
  error?: string;                  // Error message (mutually exclusive with text)
  isError?: boolean;               // Explicit error flag
}
```

### Method Lifecycle

```
createServer()  ─→  addTool() (N times)  ─→  run('stdio')  ─→  [serving]  ─→  close()
     │                    │                       │                              │
     │                    │                       │                              │
  Creates Go             Builds                  Creates                    Cancels ctx,
  mcp.Server             tool list               StdioTransport,            terminates
  with name/ver          (stored,                calls server.Run()         server.Run()
                         not yet                  in background
                         registered)              goroutine. Returns
                                                  Promise.
```

### Event Loop Integration

**Problem:** MCP tool handlers are called from Go's HTTP/transport goroutine, but JS callbacks MUST run on the event loop thread (goja.Runtime is not thread-safe).

**Solution:** Use `adapter.JS().NewChainedPromise()` with `adapter.Loop().Submit()`:

```
MCP client calls tool
  → Go mcp.AddTool handler invoked (on MCP goroutine)
    → handler creates Go channel for response
    → adapter.Loop().Submit(func() {
        // Now on event loop thread — safe to call JS
        jsResult := jsHandler(input)
        // If async: jsResult is a Promise → chain .then()
        // Send result back via channel
      })
    → Block on channel (with timeout)
    → Return *mcp.CallToolResult to MCP
```

For async JS handlers (returning Promise):
```
adapter.Loop().Submit(func() {
  promise := jsHandler(input)
  if isPromise(promise) {
    // Use adapter to chain .then() / .catch()
    promise.then(func(value) { resultChan <- value })
    promise.catch(func(err) { errorChan <- err })
  } else {
    resultChan <- promise  // synchronous result
  }
})
```

### Error Handling Semantics

| JS Handler Returns | MCP Result |
|---|---|
| `{ text: 'hello' }` | `CallToolResult{Content: [TextContent{Text: "hello"}]}` |
| `{ error: 'bad input' }` | `CallToolResult` with `.SetError()` |
| `{ isError: true, text: 'failed' }` | `CallToolResult{IsError: true, Content: [TextContent{Text: "failed"}]}` |
| throws/rejects | `CallToolResult` with `.SetError()` |
| timeout (30s) | `CallToolResult` with `.SetError("tool handler timeout")` |

### JSON Schema for Tool Registration

Input schemas are passed as JS objects and marshaled to JSON for the MCP SDK. The MCP SDK then handles schema validation and input parsing. Example:

```javascript
{
  type: 'object',
  properties: {
    path: { type: 'string', description: 'File path' },
    recursive: { type: 'boolean', description: 'Recurse into dirs' }
  },
  required: ['path']
}
```

This is marshaled to `json.RawMessage` and set as the tool's `RawInputSchema` field.

### Go Implementation Strategy

**Package:** `internal/builtin/mcpmod/` (following the `fetchmod`, `grpcmod` convention)

**Registration (register.go):**
```go
registry.RegisterNativeModule(prefix+"mcp", mcpmod.Require(
    eventLoopProvider.Adapter(),
    eventLoopProvider.Loop(),
    eventLoopProvider.Runtime(),
))
```

**Key Go types:**
```go
type mcpServer struct {
  server   *mcp.MCPServer       // go-sdk server
  tools    []toolRegistration   // pending tools (before run)
  running  bool                 // true after run() called
  cancel   context.CancelFunc   // stops server.Run()
  adapter  *gojaeventloop.Adapter
  loop     *goeventloop.Loop
  runtime  *goja.Runtime
}

type toolRegistration struct {
  name        string
  description string
  inputSchema json.RawMessage
  handler     goja.Callable      // JS callback
}
```

### Files to Create/Modify
| File | Action |
|------|--------|
| `internal/builtin/mcpmod/mcp.go` | NEW — module implementation |
| `internal/builtin/mcpmod/mcp_test.go` | NEW — unit tests |
| `internal/builtin/register.go` | MODIFY — add registration |

## T19 Design: osm:mcpcallback Module API

### Purpose

Disposable IPC channel that hosts an MCP server over a local socket transport
(UDS on Unix, loopback TCP on Windows). Enables the pr-split JS pipeline to
receive tool call results as synchronous callbacks instead of file polling.

### JavaScript API Surface

```javascript
const mcp = require('osm:mcp');
const MCPCallback = require('osm:mcpcallback');

// 1. Create MCP server and register tools
const server = mcp.createServer('pr-split', '1.0.0');
server.addTool({
  name: 'reportClassification',
  description: 'Report file classification for PR split',
  inputSchema: { type: 'object', properties: { categories: { type: 'array' } } }
}, function(input) {
  // Process classification synchronously
  classificationData = input.categories;
  return { text: 'Classification accepted' };
});

// 2. Create MCPCallback wrapping the server (do NOT call server.run())
const cb = new MCPCallback({ server: server });

// 3. Initialize — starts listener, generates bootstrap script
await cb.init();

// 4. Access connection details (available after init())
cb.scriptPath;   // '/tmp/osm-mcpcb-abc123/bootstrap.js'
cb.address;      // '/tmp/osm-mcpcb-abc123/osm.sock' (Unix) or '127.0.0.1:54321' (Windows)
cb.transport;    // 'unix' or 'tcp'

// 5. Spawn Claude Code with MCP config pointing to callback
exec.execv(['claude', '--mcp-config=' + cb.mcpConfigPath]);

// 6. Close — tears down listener, removes temp files, cancels context
await cb.close();  // Idempotent
```

### Type Signatures

```typescript
interface MCPCallbackOptions {
  server: MCPServer;     // From osm:mcp createServer() — MUST NOT have run() called
}

class MCPCallback {
  constructor(options: MCPCallbackOptions);

  // Lifecycle
  init(): Promise<void>;     // Start listener, accept connection, serve MCP
  close(): Promise<void>;    // Tear down, clean temp resources. Idempotent.

  // Properties (available after init() resolves)
  readonly scriptPath: string;    // Path to bootstrap JS file for sub-process
  readonly address: string;       // Transport address (UDS path or TCP host:port)
  readonly transport: 'unix' | 'tcp';  // Transport type
  readonly mcpConfigPath: string; // Path to MCP JSON config file for Claude Code
}
```

### Method Lifecycle

```
new MCPCallback({server})
  │
  │ Stores reference to Go *mcp.Server (via __goServer on JS object)
  │ Validates: server exists, has not run() yet
  │
  ├─→ init()
  │     │
  │     ├─ Create temp dir: os.MkdirTemp("", "osm-mcpcb-")
  │     │   Permissions: 0700
  │     │
  │     ├─ Start listener:
  │     │   Unix: net.Listen("unix", tempDir+"/osm.sock")
  │     │   Windows: net.Listen("tcp", "127.0.0.1:0")
  │     │
  │     ├─ Generate bootstrap script → tempDir/bootstrap.js
  │     │   Contains: transport address, MCP client connection code
  │     │   Permissions: 0600
  │     │
  │     ├─ Generate MCP config → tempDir/mcp-config.json
  │     │   Claude Code compatible MCP server configuration
  │     │
  │     ├─ Start accept loop in background goroutine:
  │     │   conn := listener.Accept()
  │     │   transport := &mcp.IOTransport{Reader: conn, Writer: conn}
  │     │   server.Run(ctx, transport)  // Blocks until conn closes
  │     │
  │     └─ Returns Promise<void> (resolves when listener is ready)
  │
  └─→ close()
        │
        ├─ Cancel context (stops server.Run())
        ├─ Close listener (stops Accept())
        ├─ Remove temp dir (socket, scripts, config)
        └─ Set closed=true (idempotent guard)
```

### Go Server Access Pattern

The MCPCallback Go code needs the Go `*mcp.Server` from the osm:mcp JS server
object. The mcpmod package stores a hidden reference on the JS object:

```go
// In mcpmod/mcp.go — jsCreateServer():
obj := runtime.NewObject()
_ = obj.Set("addTool", s.jsAddTool())
_ = obj.Set("run", s.jsRun())
_ = obj.Set("close", s.jsClose())
_ = obj.Set("__goServer", runtime.ToValue(s.server))  // Expose *mcp.Server to Go
return obj
```

MCPCallback extracts the Go server via:
```go
goServerVal := serverJSObj.Get("__goServer")
goServer := goServerVal.Export().(*mcp.Server)
```

### Transport Selection

| Platform | Transport | Listener | Address Format |
|----------|-----------|----------|----------------|
| Linux    | UDS       | `net.Listen("unix", path)` | `/tmp/osm-mcpcb-*/osm.sock` |
| macOS    | UDS       | `net.Listen("unix", path)` | `/tmp/osm-mcpcb-*/osm.sock` |
| Windows  | TCP       | `net.Listen("tcp", "127.0.0.1:0")` | `127.0.0.1:PORT` |

**macOS 104-char socket path limit:**
- System `TMPDIR` on macOS is long (`/var/folders/xx/.../T/`)
- Use `/tmp/` prefix via `os.MkdirTemp("/tmp", "osm-mcpcb-")` on macOS
- Path: `/tmp/osm-mcpcb-abc123/osm.sock` = ~38 chars (well under 104)
- Reference: `internal/builtin/claudemux/control.go` uses same pattern

### MCP Over Socket Transport

The MCP SDK's `IOTransport` wraps any `io.ReadCloser` + `io.WriteCloser`:

```go
conn, _ := listener.Accept()
transport := &mcp.IOTransport{
    Reader: conn,   // net.Conn implements io.ReadCloser
    Writer: conn,   // net.Conn implements io.WriteCloser
}
err := server.Run(ctx, transport)
```

For single-connection use (pr-split: one Claude subprocess):
```go
go func() {
    conn, err := listener.Accept()
    if err != nil { return }
    defer conn.Close()
    transport := &mcp.IOTransport{Reader: conn, Writer: conn}
    server.Run(ctx, transport)  // Blocks until conn closes or ctx cancelled
}()
```

### Bootstrap Script Generation

Generated `bootstrap.js` for sub-process execution:
```javascript
// Auto-generated by osm:mcpcallback — do not edit
// Connects to host MCP server at: /tmp/osm-mcpcb-abc123/osm.sock
// Transport: unix

// This script is NOT executed directly — it provides connection config
// for tools that speak MCP natively (e.g., Claude Code).
// See mcpConfigPath for Claude Code integration.
```

Generated `mcp-config.json` for Claude Code:
```json
{
  "mcpServers": {
    "pr-split-callback": {
      "command": "socat",
      "args": ["STDIO", "UNIX-CONNECT:/tmp/osm-mcpcb-abc123/osm.sock"]
    }
  }
}
```

On Windows (TCP):
```json
{
  "mcpServers": {
    "pr-split-callback": {
      "command": "socat",
      "args": ["STDIO", "TCP:127.0.0.1:54321"]
    }
  }
}
```

Note: The exact connection mechanism depends on what the Claude Code subprocess
supports. `socat` is one approach; an alternative is a tiny Go-based connector
binary bundled with osm, or using the osm:mcp module's future client support.

### Security Model

1. **UDS permissions:** Socket dir 0700, socket file inherits. Only the owning user can connect.
2. **TCP loopback:** `127.0.0.1` only — no external network exposure. Random OS-assigned port.
3. **Temp file permissions:** Bootstrap script and MCP config at 0600.
4. **Cleanup guarantees:**
   - `close()` removes all temp resources (dir, socket, scripts)
   - Context cancellation triggers cleanup
   - `context.Done()` channel watcher as fallback cleanup path
   - Idempotent — safe to call multiple times

### Error Handling

| Scenario | Behavior |
|----------|----------|
| server already running | init() throws "server is already running" |
| init() called twice | throws "already initialized" |
| close() before init() | no-op (idempotent) |
| close() called twice | no-op (idempotent) |
| listener.Accept() fails | rejects init() promise |
| sub-process never connects | server.Run() blocks until ctx cancelled |
| sub-process disconnects | server.Run() returns, MCPCallback stays alive for reconnect |
| temp dir creation fails | init() rejects with OS error |

### Go Implementation Strategy

**Package:** `internal/builtin/mcpcallbackmod/`

**Registration (register.go):**
```go
registry.RegisterNativeModule(prefix+"mcpcallback",
    mcpcallbackmod.Require(eventLoopProvider.Adapter(), eventLoopProvider.Loop()))
```

**Key Go types:**
```go
type mcpCallback struct {
    server   *mcp.Server
    listener net.Listener
    tempDir  string
    ctx      context.Context
    cancel   context.CancelFunc
    adapter  *gojaeventloop.Adapter
    loop     *goeventloop.Loop
    runtime  *goja.Runtime

    mu          sync.Mutex
    initialized bool
    closed      bool
}
```

### Files to Create/Modify

| File | Action |
|------|--------|
| `internal/builtin/mcpmod/mcp.go` | MODIFY — add `__goServer` property to JS object |
| `internal/builtin/mcpcallbackmod/mcpcallback.go` | NEW — module implementation |
| `internal/builtin/mcpcallbackmod/mcpcallback_test.go` | NEW — unit tests |
| `internal/builtin/register.go` | MODIFY — add registration |

1. Pre-T1 bug fixes (gitAddChangedFiles, sendToHandle single-write, commit error checking, test fixes)
2. Rule of Two review gate passed
3. Committed 66be949
