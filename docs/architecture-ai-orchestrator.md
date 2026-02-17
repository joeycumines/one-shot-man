# AI Orchestrator — Architecture Design Document

> **Status:** Proposal  
> **Date:** 2026-02-17  
> **Scope:** T238 (this document), gates T239–T255  
> **Author:** Generated from blueprint analysis

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Architecture Approach A: Minimal (Script-First)](#3-architecture-approach-a-minimal-script-first)
4. [Architecture Approach B: Clean Architecture (Module-First)](#4-architecture-approach-b-clean-architecture-module-first)
5. [Architecture Approach C: Pragmatic Balance (Recommended)](#5-architecture-approach-c-pragmatic-balance-recommended)
6. [Cross-Cutting Concerns](#6-cross-cutting-concerns)
7. [Implementation Roadmap](#7-implementation-roadmap)
8. [Decision](#8-decision)

---

## 1. Executive Summary

The **AI Orchestrator** is a subsystem of `osm` that enables programmatic spawning, monitoring, and coordination of AI coding agents — primarily Claude Code — from within JavaScript workflows. It extends osm's existing behavior tree engine, MCP server, and scripting infrastructure to automate multi-agent development workflows: spawning agents in isolated PTY sessions, feeding them prompts, monitoring their output for rate limits and permission prompts, and orchestrating complex workflows like PR splitting.

The orchestrator does **not** replace osm's clipboard-first philosophy. It is an opt-in capability layer for power users who want to automate repetitive multi-step AI workflows while retaining full control over prompt construction and verification.

### Why Now?

1. **T234 Decision:** The [code-review-splitter evaluation](archive/notes/t234-code-review-workflow-evaluation.md) explicitly deferred LLM-calling capabilities to the AI Orchestrator, confirming that building per-command LLM integrations would be wasteful duplication.
2. **Infrastructure Ready:** The behavior tree engine (`osm:bt`, `osm:pabt`), MCP server (`osm mcp`), session isolation (`internal/session`), and scripting engine are all production-quality. The orchestrator builds on top of them, not beside them.
3. **User Demand:** Workflows that require spawning multiple Claude Code instances (one per microservice, one per PR chunk) are currently manual. The orchestrator automates the tedium while keeping humans in the loop for verification.

---

## 2. Problem Statement

### What osm Can Do Today

```
User → osm → [build prompt] → clipboard → paste into LLM UI → read response → repeat
```

osm excels at constructing structured prompts from diffs, files, and templates. It has no knowledge of — and no dependency on — any AI provider. This is a feature, not a limitation.

### What osm Cannot Do Today

1. **Spawn and manage AI agent processes.** No PTY allocation, no process lifecycle management.
2. **Communicate bidirectionally with running agents.** The MCP server is unidirectional (agent → osm). There is no osm → agent channel.
3. **Detect and respond to agent output patterns.** Rate limit errors, permission prompts, model selection menus, and SSO login flows all require manual intervention.
4. **Orchestrate multi-agent workflows.** Splitting a large change into multiple PRs, running each through a separate agent, verifying results, and rebasing — all manual.
5. **Isolate concurrent agent sessions.** Running two agents simultaneously risks session state corruption.

### What the AI Orchestrator Enables

```
User → osm orchestrate → [spawn N agents] → [feed prompts via BT] → [monitor PTY output]
                              ↓                     ↓                      ↓
                        PTY per agent          MCP bidirectional      Pattern matching
                              ↓                     ↓                      ↓
                        [detect rate limits] → [auto-retry] → [collect results] → [PR split]
```

The orchestrator turns osm from a prompt-construction tool into a prompt-execution coordinator — while keeping prompts themselves as first-class, human-inspectable artifacts.

---

## 3. Architecture Approach A: Minimal (Script-First)

### Philosophy

Maximum leverage of existing JavaScript scripting infrastructure. All orchestration logic lives in JS scripts. Go adds only the thinnest possible native modules (`osm:pty`, `osm:mcp/client`). No new Go command, no new Go abstractions. Users write orchestration scripts the same way they write any osm script.

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                          JavaScript Layer                            │
│                                                                      │
│  orchestrate.js (user script)                                        │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  const pty = require('osm:pty');                               │  │
│  │  const bt  = require('osm:bt');                                │  │
│  │  const mcp = require('osm:mcp/client');                       │  │
│  │                                                                │  │
│  │  // Spawn Claude Code in a PTY                                │  │
│  │  const session = pty.spawn('claude', ['--mcp', ...]);         │  │
│  │  // Build prompt, write to PTY stdin                          │  │
│  │  session.write(prompt);                                       │  │
│  │  // Read output, parse for patterns                           │  │
│  │  const line = session.readLine();                             │  │
│  │  if (patterns.isRateLimit(line)) { ... }                      │  │
│  │                                                                │  │
│  │  // BT orchestration (existing infrastructure)                │  │
│  │  const tree = bt.sequence([spawnNode, promptNode, verifyNode]);│  │
│  │  bt.newTicker(1000, tree);                                    │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  Pattern Library (JS modules, require-able)                          │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  patterns/rate-limit.js     → regex matchers for 429/backoff  │  │
│  │  patterns/permission.js     → detect Y/N prompts, reject      │  │
│  │  patterns/model-select.js   → navigate model selection menu   │  │
│  │  patterns/sso-login.js      → detect SSO flows                │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
                              │
                    require('osm:pty')
                    require('osm:mcp/client')
                              │
┌──────────────────────────────────────────────────────────────────────┐
│                            Go Layer                                  │
│                                                                      │
│  internal/builtin/pty/        ← NEW: PTY spawning module            │
│  ├── pty.go                   (spawn, read, write, resize, close)   │
│  ├── pty_unix.go              (creack/pty on macOS/Linux)           │
│  ├── pty_windows.go           (ConPTY via golang.org/x/sys)        │
│  └── pty_test.go                                                    │
│                                                                      │
│  internal/builtin/mcpclient/  ← NEW: MCP client module             │
│  ├── client.go                (connect, call tools, subscribe)      │
│  └── client_test.go                                                 │
│                                                                      │
│  (everything else: existing — bt, pabt, exec, session, etc.)        │
└──────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
1. User runs: osm script orchestrate.js
2. Script requires osm:pty → Go allocates PTY via creack/pty
3. Script spawns "claude --mcp-server osm" in PTY
4. Script writes prompt to PTY stdin (session.write)
5. Script reads PTY stdout line-by-line (session.readLine)
6. JS pattern matchers test each line:
   - Rate limit → sleep + retry
   - Permission prompt → send "N\n"
   - Model selection → send arrow keys + enter
   - SSO login → abort with error
7. Completed output → collected, verified, optionally committed
8. Multiple agents: repeat steps 2–7 per agent, orchestrated by osm:bt
```

### API Surface

#### `osm:pty` Module

```go
// internal/builtin/pty/pty.go

// PTYSession represents a spawned process in a pseudo-terminal.
type PTYSession struct {
    mu      sync.Mutex
    cmd     *exec.Cmd
    pty     *os.File     // creack/pty file descriptor (Unix)
    done    chan struct{}
    exitErr error
}

// Spawn creates a new PTY session for the given command.
// JS: pty.spawn(command, args, opts?) → PTYSession
func Spawn(ctx context.Context, command string, args []string, opts SpawnOpts) (*PTYSession, error)

// SpawnOpts configures PTY spawning.
type SpawnOpts struct {
    Env     map[string]string  // Additional environment variables
    Dir     string             // Working directory
    Rows    int                // Terminal rows (default: 24)
    Cols    int                // Terminal cols (default: 80)
}

// Write sends data to the PTY stdin.
// JS: session.write(data)
func (s *PTYSession) Write(data string) error

// ReadLine reads a line from PTY stdout (blocking, with timeout).
// JS: session.readLine(timeoutMs?) → string | null
func (s *PTYSession) ReadLine(timeout time.Duration) (string, error)

// Read reads up to n bytes from PTY stdout.
// JS: session.read(n) → string
func (s *PTYSession) Read(n int) (string, error)

// Resize changes the PTY window size.
// JS: session.resize(rows, cols)
func (s *PTYSession) Resize(rows, cols int) error

// Close terminates the PTY session.
// JS: session.close()
func (s *PTYSession) Close() error

// Wait blocks until the process exits.
// JS: session.wait() → {code: number, error: string|null}
func (s *PTYSession) Wait() (int, error)

// IsAlive returns whether the subprocess is still running.
// JS: session.isAlive() → bool
func (s *PTYSession) IsAlive() bool
```

#### `osm:mcp/client` Module

```go
// internal/builtin/mcpclient/client.go

// MCPClient connects to an MCP server (typically running inside Claude Code).
type MCPClient struct {
    transport *mcp.StdioTransport
    // ...
}

// Connect establishes a connection to an MCP server.
// JS: mcpClient.connect(transport) → MCPClient
func Connect(ctx context.Context, r io.Reader, w io.Writer) (*MCPClient, error)

// CallTool invokes an MCP tool on the connected server.
// JS: client.callTool(name, args) → result
func (c *MCPClient) CallTool(name string, args map[string]any) (any, error)

// ListTools returns available tools from the server.
// JS: client.listTools() → [{name, description, inputSchema}]
func (c *MCPClient) ListTools() ([]ToolInfo, error)
```

### Testing Strategy

1. **PTY module:** Unit tests spawn `/bin/echo` and `/bin/cat` (not real AI agents). Test read/write/close/resize. Platform-specific tests for Unix vs Windows (ConPTY).
2. **Pattern matching:** Pure JS unit tests — feed known strings through matchers, assert detection.
3. **Integration:** `TestMain`-gated tests (T247) with real Claude Code, disabled by default, enabled via env var in CI.
4. **BT orchestration:** Existing `osm:bt` tests cover tree execution. Orchestration scripts tested with mock PTY sessions (`/bin/cat` echo server).

### Trade-offs

| Aspect | Assessment |
|--------|------------|
| **Minimal Go changes** | ✅ Only 2 new modules (~400 LOC each). Leverages all existing infra. |
| **User flexibility** | ✅ Full scriptability. Users customize everything via JS. |
| **Safety-critical paths in JS** | ❌ Rate limit handling, permission rejection, signal forwarding — all in JS. A bug in pattern matching could accept a dangerous permission prompt. |
| **No type safety for orchestration** | ❌ Complex orchestration logic in dynamic JS. Refactoring is fragile. |
| **Testing** | ⚠️ JS pattern matchers need extensive test coverage. No compile-time guarantees. |
| **Platform compat** | ⚠️ PTY module needs careful platform abstraction in Go, but orchestration scripts are platform-independent. |
| **Discoverability** | ❌ Users must know to write orchestration scripts. No built-in `osm orchestrate` command. |

---

## 4. Architecture Approach B: Clean Architecture (Module-First)

### Philosophy

Full Go module system with strongly-typed interfaces, clear package boundaries, and dependency injection. Every concern — PTY management, output parsing, provider abstraction, workflow orchestration — is a Go package with an explicit interface. JavaScript is relegated to leaf-node behaviors and user customization.

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                      osm CLI                                         │
│                                                                      │
│  cmd/osm/main.go                                                    │
│  ├── OrchestrateCommand (NEW Go command)                            │
│  │   ├── --provider=claude-code|ollama                              │
│  │   ├── --workflow=pr-split|single-prompt|multi-agent              │
│  │   └── --config=orchestrator.json                                 │
│  └── [all existing commands unchanged]                              │
└──────────────────────┬───────────────────────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────────────────────┐
│                internal/orchestrator/                                 │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  orchestrator.go — Top-level Orchestrator                    │    │
│  │                                                              │    │
│  │  type Orchestrator struct {                                  │    │
│  │      providers  ProviderRegistry                             │    │
│  │      sessions   SessionManager                               │    │
│  │      parser     OutputParser                                 │    │
│  │      recovery   RecoveryPolicy                               │    │
│  │      workflow   WorkflowEngine                               │    │
│  │  }                                                           │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐   │
│  │  provider/       │  │  parser/         │  │  recovery/       │   │
│  │                  │  │                  │  │                  │   │
│  │ Provider iface   │  │ OutputParser     │  │ RecoveryPolicy   │   │
│  │ ClaudeCodeProv   │  │ RateLimitDet     │  │ CrashDetector    │   │
│  │ OllamaProv       │  │ PermissionDet    │  │ HangTimeout      │   │
│  │ ProviderRegistry │  │ ModelSelectDet   │  │ GraceShutdown    │   │
│  └──────────────────┘  │ SSODetector      │  └──────────────────┘   │
│                         └──────────────────┘                         │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐   │
│  │  workflow/       │  │  pty/            │  │  session/        │   │
│  │                  │  │                  │  │                  │   │
│  │ WorkflowEngine   │  │ PTYManager       │  │ AgentSession     │   │
│  │ PRSplitWorkflow  │  │ PTYSession       │  │ SessionIsolator  │   │
│  │ SinglePrompt     │  │ SignalForwarder  │  │ MCPChannel       │   │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘   │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  mcp/                                                        │    │
│  │  MCPBridge — bidirectional MCP for agent communication       │    │
│  │  registerSession, reportProgress, reportResult,              │    │
│  │  requestGuidance                                             │    │
│  └──────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
1. User runs: osm orchestrate --provider=claude-code --workflow=pr-split
2. OrchestrateCommand creates Orchestrator with injected dependencies
3. Orchestrator.Run():
   a. ProviderRegistry.Get("claude-code") → ClaudeCodeProvider
   b. SessionIsolator.NewSession() → AgentSession{id, stateDir, mcpChannel}
   c. PTYManager.Spawn("claude", args, env) → PTYSession
   d. WorkflowEngine.Execute(workflow, ptySession):
      i.  Write prompt to PTY
      ii. OutputParser.ParseStream(ptySession.Stdout):
          - Lines → RateLimitDetector → pause/retry
          - Lines → PermissionDetector → reject
          - Lines → ModelSelectDetector → navigate
          - Lines → SSODetector → abort
          - Lines → CompletionDetector → collect result
      iii. RecoveryPolicy monitors for crash/hang
      iv.  On success → verify → commit (or queue next chunk)
   e. Repeat for each workflow step (PR chunk, branch, etc.)
4. Results aggregated, cleanup, exit
```

### API Surface

#### Core Interfaces

```go
// internal/orchestrator/provider/provider.go

// Provider abstracts an AI agent backend.
type Provider interface {
    // Name returns the provider identifier (e.g., "claude-code", "ollama").
    Name() string

    // Spawn starts an agent session and returns a handle.
    Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error)

    // Capabilities returns what this provider supports.
    Capabilities() ProviderCapabilities
}

// AgentHandle represents a running agent instance.
type AgentHandle interface {
    // Send writes input to the agent.
    Send(ctx context.Context, input string) error

    // Receive returns a channel that emits parsed output events.
    Receive() <-chan OutputEvent

    // Close terminates the agent.
    Close() error

    // SessionID returns the isolated session identifier.
    SessionID() string
}

// ProviderCapabilities declares what a provider supports.
type ProviderCapabilities struct {
    MCP       bool // Supports MCP tool calling
    Streaming bool // Supports streaming output
    MultiTurn bool // Supports multi-turn conversation
}

// ProviderRegistry manages available providers.
type ProviderRegistry interface {
    Register(p Provider)
    Get(name string) (Provider, error)
    List() []string
}
```

```go
// internal/orchestrator/parser/parser.go

// OutputEvent represents a parsed event from agent output.
type OutputEvent struct {
    Type      EventType
    Raw       string            // Original text
    Parsed    map[string]string // Extracted fields
    Timestamp time.Time
}

type EventType int

const (
    EventText          EventType = iota // Normal text output
    EventRateLimit                      // 429 / rate limit detected
    EventPermission                     // Permission prompt (Y/N)
    EventModelSelect                    // Model selection menu
    EventSSOLogin                       // SSO/OAuth redirect
    EventCompletion                     // Task completed signal
    EventError                          // Error output
)

// OutputParser transforms raw PTY output into structured events.
type OutputParser interface {
    // Parse processes a line of output and returns zero or more events.
    Parse(line string) []OutputEvent

    // RegisterPattern adds a custom detection pattern.
    RegisterPattern(name string, pattern *regexp.Regexp, eventType EventType)
}
```

```go
// internal/orchestrator/workflow/workflow.go

// Workflow defines a multi-step orchestration plan.
type Workflow interface {
    // Name returns the workflow identifier.
    Name() string

    // Steps returns the ordered execution steps.
    Steps() []Step

    // OnStepComplete is called after each step finishes.
    OnStepComplete(step Step, result StepResult) error
}

// Step represents a single unit of work in a workflow.
type Step interface {
    // Execute runs this step with the given agent.
    Execute(ctx context.Context, agent AgentHandle) (StepResult, error)
}

// StepResult contains the outcome of executing a step.
type StepResult struct {
    Success bool
    Output  string
    Metrics StepMetrics
}
```

```go
// internal/orchestrator/recovery/recovery.go

// RecoveryPolicy defines how failures are handled.
type RecoveryPolicy interface {
    // OnCrash handles agent process crash.
    OnCrash(ctx context.Context, agent AgentHandle, err error) RecoveryAction

    // OnHang handles agent timeout (no output for N seconds).
    OnHang(ctx context.Context, agent AgentHandle, duration time.Duration) RecoveryAction

    // OnRateLimit handles rate limit detection.
    OnRateLimit(ctx context.Context, event OutputEvent) RecoveryAction
}

type RecoveryAction int

const (
    ActionRetry   RecoveryAction = iota // Retry the current step
    ActionSkip                          // Skip and continue
    ActionAbort                         // Abort the workflow
    ActionWait                          // Wait and retry after delay
)
```

### Testing Strategy

1. **Interface-driven mocking:** Every interface (`Provider`, `AgentHandle`, `OutputParser`, `RecoveryPolicy`, `Workflow`) has a test double. No real AI providers in unit tests.
2. **Table-driven parser tests:** Hundreds of test cases mapping raw output strings to expected `OutputEvent` types. Patterns extracted from real Claude Code output.
3. **Integration tests via TestMain:** T247 — `go test -tags=integration` with real Claude Code, gated by env var.
4. **Workflow simulation:** Mock `AgentHandle` that replays recorded output sequences, allowing full workflow testing without any external process.
5. **Platform tests:** PTY spawning tests run on all three CI platforms.

### Trade-offs

| Aspect | Assessment |
|--------|------------|
| **Type safety** | ✅ All interfaces have compile-time guarantees. Refactoring is safe. |
| **Testability** | ✅ Interface-driven design enables comprehensive mocking. |
| **Separation of concerns** | ✅ Each package has a single responsibility. |
| **Code volume** | ❌ Massive. 8+ new packages, 2000+ LOC before any real logic. Interface ceremony dominates early development. |
| **User customization** | ❌ Users can't modify workflow logic without writing Go code. JS scripting is an afterthought. |
| **Contradicts osm's DNA** | ❌ osm is a scripting-first tool. This approach buries the scripting engine under enterprise abstractions. |
| **Velocity** | ❌ T239–T255 would take 3× longer due to interface design, mock generation, and package wiring. |
| **Learning curve** | ⚠️ New contributors must understand the full dependency graph before modifying any component. |
| **Over-engineering risk** | ❌ Designing provider abstractions before knowing what Ollama/other providers actually need leads to abstraction inversion. |

---

## 5. Architecture Approach C: Pragmatic Balance (Recommended)

### Philosophy

**Go for infrastructure and safety. JavaScript for workflow logic and user customization.**

Safety-critical paths — PTY management, signal forwarding, output parsing for security-sensitive patterns (permission rejection), and session isolation — are implemented in Go with strong typing and comprehensive tests. Workflow orchestration, BT composition, prompt construction, and user-facing customization remain in JavaScript, leveraging the existing `osm:bt`/`osm:pabt` infrastructure and the `tui` API.

The key insight: **the BT engine already exists and works**. We don't need a new "workflow engine" in Go. We need Go modules that expose PTY and MCP capabilities to the *existing* BT engine.

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         JavaScript Layer                                  │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │  BT Orchestration (existing osm:bt + osm:pabt)                  │    │
│  │                                                                  │    │
│  │  const bt   = require('osm:bt');                                │    │
│  │  const pty  = require('osm:pty');                               │    │
│  │  const orc  = require('osm:orchestrator');                      │    │
│  │                                                                  │    │
│  │  // BT Template: spawn-and-prompt                               │    │
│  │  bt.sequence([                                                  │    │
│  │    spawnAgent(provider, config),    // osm:pty                  │    │
│  │    sendPrompt(agent, prompt),       // osm:pty write            │    │
│  │    waitForResponse(agent, parser),  // osm:orchestrator parse   │    │
│  │    verifyOutput(agent, checks),     // osm:exec (run tests)    │    │
│  │    commitChanges(message),          // osm:exec (git commit)   │    │
│  │  ]);                                                            │    │
│  └──────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │  Workflow Scripts (user-modifiable, goal-discoverable)           │    │
│  │                                                                  │    │
│  │  scripts/orchestrate-pr-split.js                                │    │
│  │  scripts/orchestrate-multi-agent.js                             │    │
│  │  scripts/orchestrate-code-review.js                             │    │
│  │  goals/orchestrate.json (goal definition)                       │    │
│  └──────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                     require('osm:pty')
                     require('osm:orchestrator')
                              │
┌──────────────────────────────────────────────────────────────────────────┐
│                          Go Layer (Safety-Critical)                       │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────┐        │
│  │  internal/builtin/pty/               ← NEW MODULE (T239)    │        │
│  │                                                              │        │
│  │  // PTY lifecycle management with signal forwarding          │        │
│  │  pty.go          — Spawn, Read, Write, Resize, Close         │        │
│  │  pty_unix.go     — creack/pty (macOS + Linux)                │        │
│  │  pty_windows.go  — ConPTY (Windows)                          │        │
│  │  signal.go       — SIGINT/SIGTERM forwarding                 │        │
│  │  require.go      — osm:pty module registration               │        │
│  └──────────────────────────────────────────────────────────────┘        │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────┐        │
│  │  internal/builtin/orchestrator/      ← NEW MODULE (T241+)   │        │
│  │                                                              │        │
│  │  // Output parsing + agent abstraction + session isolation   │        │
│  │  parser.go       — OutputParser (rate limits, permissions)   │        │
│  │  patterns.go     — Compiled regex patterns for Claude Code   │        │
│  │  provider.go     — Provider interface + registry             │        │
│  │  claude_code.go  — Claude Code provider (PTY-based)          │        │
│  │  session.go      — AgentSession isolation                    │        │
│  │  recovery.go     — Error recovery policies                   │        │
│  │  safety.go       — Permission prompt rejection (CRITICAL)    │        │
│  │  require.go      — osm:orchestrator module registration      │        │
│  └──────────────────────────────────────────────────────────────┘        │
│                                                                          │
│  ┌──────────────────────────────────────────────────────────────┐        │
│  │  internal/command/mcp.go             ← EXTENDED (T240)       │        │
│  │                                                              │        │
│  │  // New MCP tools for bidirectional agent communication      │        │
│  │  + registerSession(sessionID, capabilities)                  │        │
│  │  + reportProgress(sessionID, status, data)                   │        │
│  │  + reportResult(sessionID, result)                           │        │
│  │  + requestGuidance(sessionID, question, options)             │        │
│  └──────────────────────────────────────────────────────────────┘        │
│                                                                          │
│  [Existing modules — unchanged]                                          │
│  internal/builtin/bt/         — BT bridge (thread-safe JS↔Go)          │
│  internal/builtin/pabt/       — PA-BT planning engine                   │
│  internal/builtin/exec/       — Shell command execution                  │
│  internal/session/            — Session ID & isolation                   │
│  internal/storage/            — Session persistence                      │
│  internal/command/mcp.go      — MCP server (existing tools)             │
│  internal/config/             — Configuration management                 │
└──────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

```
                  ┌─────────────────┐
                  │  User invokes   │
                  │  osm script     │
                  │  orchestrate.js │
                  └────────┬────────┘
                           │
                           ▼
              ┌────────────────────────┐
              │  JS: Build prompt from │
              │  context (existing     │
              │  context manager)      │
              └────────────┬───────────┘
                           │
                           ▼
              ┌────────────────────────┐
              │  JS: BT tree setup     │
              │  bt.sequence([...])    │
              │  with osm:pty leaves   │
              └────────────┬───────────┘
                           │
              ┌────────────▼───────────┐
              │  osm:pty.spawn()       │
              │  Go: creack/pty alloc  │◄─── PTY allocation (Go, safety-critical)
              │  Go: signal forwarding │
              └────────────┬───────────┘
                           │
              ┌────────────▼───────────┐
              │  osm:pty.write(prompt) │
              │  Go: write to PTY fd   │
              └────────────┬───────────┘
                           │
              ┌────────────▼───────────────────────────┐
              │  Output parsing pipeline               │
              │                                        │
              │  PTY stdout ──► Go: OutputParser        │
              │                    │                    │
              │        ┌───────────┼───────────┐       │
              │        ▼           ▼           ▼       │
              │  RateLimit    Permission    ModelSel   │
              │  Detector     Rejector      Navigator  │◄── Pattern matching (Go)
              │  (pause+      (send "N",   (arrow+    │
              │   retry)       CRITICAL)    enter)     │
              │                                        │
              │  Unmatched lines ──► JS callback       │◄── Content routing (JS)
              │                      (user-defined     │
              │                       processing)      │
              └────────────────────┬───────────────────┘
                                   │
              ┌────────────────────▼──────────────────┐
              │  JS: BT verification step             │
              │  exec("make test")                    │
              │  exec("git diff --check")             │
              └────────────────────┬──────────────────┘
                                   │
              ┌────────────────────▼──────────────────┐
              │  JS: Commit / PR creation             │
              │  exec("git commit -m '...'")         │
              └───────────────────────────────────────┘
```

### API Surface

#### `osm:pty` Module (Go — T239)

The PTY module is a thin, safe Go wrapper around `creack/pty` (Unix) and ConPTY (Windows). It handles the OS-specific details and exposes a clean JavaScript API.

```go
// internal/builtin/pty/pty.go
package pty

import (
    "context"
    "io"
    "os/exec"
    "sync"
    "time"
)

// Session represents a process running in a pseudo-terminal.
// The zero value is not usable; create via Spawn().
type Session struct {
    mu       sync.Mutex
    cmd      *exec.Cmd
    ptyFile  io.ReadWriteCloser // platform-specific PTY handle
    done     chan struct{}
    exitCode int
    exitErr  error
    cancel   context.CancelFunc

    // Output buffer for line-based reading
    scanner  *bufio.Scanner
    lineCh   chan string
}

// SpawnConfig configures a PTY session.
type SpawnConfig struct {
    Command string            // Executable path or name
    Args    []string          // Command arguments
    Env     map[string]string // Additional environment variables (merged with os.Environ)
    Dir     string            // Working directory (default: caller's CWD)
    Rows    uint16            // Terminal rows (default: 24)
    Cols    uint16            // Terminal cols (default: 80)
}

// Spawn allocates a PTY and starts the command.
// The returned Session must be closed to prevent resource leaks.
func Spawn(ctx context.Context, cfg SpawnConfig) (*Session, error)

// Write sends data to the PTY (agent's stdin).
func (s *Session) Write(data []byte) (int, error)

// WriteString is a convenience for Write([]byte(str)).
func (s *Session) WriteString(str string) error

// ReadLine reads the next line of output (blocking).
// Returns ("", io.EOF) when the process exits.
// Timeout of 0 means block indefinitely.
func (s *Session) ReadLine(timeout time.Duration) (string, error)

// Resize changes the PTY window dimensions (SIGWINCH).
func (s *Session) Resize(rows, cols uint16) error

// Signal sends a signal to the child process.
func (s *Session) Signal(sig os.Signal) error

// Close terminates the child process and releases the PTY.
// Sends SIGTERM, waits 5s, then SIGKILL.
func (s *Session) Close() error

// Wait blocks until the child process exits.
// Returns the exit code (0 for success).
func (s *Session) Wait() (exitCode int, err error)

// IsAlive returns true if the child process is running.
func (s *Session) IsAlive() bool

// Pid returns the child process PID. Returns 0 if not started.
func (s *Session) Pid() int
```

JavaScript exposure:

```javascript
const pty = require('osm:pty');

// Spawn a process in a PTY
const session = pty.spawn({
    command: 'claude',
    args: ['--print', '--mcp-server', 'osm mcp'],
    env: { CLAUDE_MODEL: 'claude-sonnet-4-20250514' },
    rows: 40,
    cols: 120,
});

// Write to agent
session.writeString('Fix the failing test in internal/foo/bar_test.go\n');

// Read output line by line
while (session.isAlive()) {
    const line = session.readLine(30000); // 30s timeout
    if (line === null) break; // EOF
    log.debug('agent: ' + line);
}

// Get exit code
const result = session.wait();
log.info('exit code: ' + result.code);

// Always close
session.close();
```

#### `osm:orchestrator` Module (Go — T241, T243, T248, T249)

This module provides the output parsing pipeline, provider abstraction, error recovery, and session isolation — all the safety-critical orchestration logic.

```go
// internal/builtin/orchestrator/provider.go
package orchestrator

import "context"

// Provider is the abstraction for AI agent backends.
// First implementation: Claude Code via PTY.
// Design supports future providers (Ollama, API-based agents).
type Provider interface {
    // Name returns the provider identifier.
    Name() string

    // Spawn starts an agent and returns a handle for interaction.
    Spawn(ctx context.Context, cfg AgentConfig) (Agent, error)
}

// Agent represents a running AI agent instance.
type Agent interface {
    // ID returns the unique session identifier for this agent.
    ID() string

    // Send writes a prompt/instruction to the agent.
    Send(ctx context.Context, input string) error

    // ReadLine reads the next line of output (blocking with timeout).
    ReadLine(timeout time.Duration) (string, error)

    // IsAlive returns whether the agent process is running.
    IsAlive() bool

    // Close terminates the agent gracefully.
    Close() error

    // PTY returns the underlying PTY session (if applicable).
    // Returns nil for non-PTY providers (e.g., API-based).
    PTY() *pty.Session
}

// AgentConfig is the configuration for spawning an agent.
type AgentConfig struct {
    Provider    string            // Provider name (for registry lookup)
    Model       string            // Model identifier
    WorkDir     string            // Working directory for the agent
    Env         map[string]string // Additional environment variables
    MCPServers  []string          // MCP server commands to attach
    SessionID   string            // Isolated session ID (auto-generated if empty)
    Rows        uint16            // Terminal rows
    Cols        uint16            // Terminal cols
}
```

```go
// internal/builtin/orchestrator/parser.go
package orchestrator

import (
    "regexp"
    "time"
)

// EventType classifies a parsed output event.
type EventType int

const (
    EventText         EventType = iota // Normal text output
    EventRateLimit                     // Rate limit / 429 / backoff
    EventPermission                    // Permission prompt requiring Y/N
    EventModelSelect                   // Model selection menu
    EventSSOLogin                      // SSO / OAuth login flow
    EventCompletion                    // Agent signaled completion
    EventToolUse                       // MCP tool invocation
    EventError                         // Error message
    EventThinking                      // Agent thinking/processing indicator
)

// OutputEvent represents a parsed, classified line of agent output.
type OutputEvent struct {
    Type      EventType
    Line      string            // Raw line text
    Fields    map[string]string // Extracted fields (e.g., "retryAfter": "30")
    Timestamp time.Time
}

// Parser transforms raw PTY output lines into classified events.
// It is NOT safe for concurrent use from multiple goroutines.
type Parser struct {
    patterns []patternEntry
}

type patternEntry struct {
    name     string
    re       *regexp.Regexp
    typ      EventType
    extract  func([]string) map[string]string // submatch → fields
}

// NewParser creates a parser pre-loaded with Claude Code output patterns.
func NewParser() *Parser

// Parse classifies a single line of output.
// Returns EventText if no pattern matches.
func (p *Parser) Parse(line string) OutputEvent

// AddPattern registers a custom detection pattern.
// This is exposed to JS for user-defined pattern extensions.
func (p *Parser) AddPattern(name string, pattern string, eventType EventType) error
```

```go
// internal/builtin/orchestrator/safety.go
package orchestrator

// PermissionPolicy defines how permission prompts are handled.
// THIS IS THE MOST SAFETY-CRITICAL CODE IN THE ORCHESTRATOR.
//
// By default, ALL permission prompts are REJECTED.
// Users must explicitly opt-in to auto-approval patterns.
type PermissionPolicy struct {
    // DefaultAction is applied when no rule matches.
    // MUST be ActionReject for safety.
    DefaultAction PermissionAction

    // Rules are evaluated in order. First match wins.
    Rules []PermissionRule
}

type PermissionAction int

const (
    // ActionReject sends "N" to the permission prompt (DEFAULT, SAFE).
    ActionReject PermissionAction = iota

    // ActionApprove sends "Y" to the permission prompt (DANGEROUS).
    // Only allowed for explicitly whitelisted patterns.
    ActionApprove

    // ActionAsk pauses and asks the human operator (SAFEST but blocks).
    ActionAsk
)

// PermissionRule matches a permission prompt pattern to an action.
type PermissionRule struct {
    Pattern *regexp.Regexp
    Action  PermissionAction
    Reason  string // Human-readable explanation for audit log
}

// DefaultPermissionPolicy returns the safe default: reject everything.
func DefaultPermissionPolicy() PermissionPolicy {
    return PermissionPolicy{DefaultAction: ActionReject}
}
```

```go
// internal/builtin/orchestrator/recovery.go
package orchestrator

import "time"

// RecoveryConfig defines error recovery behavior.
type RecoveryConfig struct {
    // MaxRetries is the maximum number of retries for a single operation.
    MaxRetries int // default: 3

    // RateLimitBackoff is the initial wait after a rate limit.
    RateLimitBackoff time.Duration // default: 30s

    // HangTimeout is the maximum time to wait for agent output.
    HangTimeout time.Duration // default: 5m

    // CrashRestartDelay is the wait before restarting a crashed agent.
    CrashRestartDelay time.Duration // default: 5s

    // GraceShutdownTimeout is the time to wait for graceful shutdown
    // before sending SIGKILL.
    GraceShutdownTimeout time.Duration // default: 10s
}

// DefaultRecoveryConfig returns sensible defaults.
func DefaultRecoveryConfig() RecoveryConfig {
    return RecoveryConfig{
        MaxRetries:           3,
        RateLimitBackoff:     30 * time.Second,
        HangTimeout:          5 * time.Minute,
        CrashRestartDelay:    5 * time.Second,
        GraceShutdownTimeout: 10 * time.Second,
    }
}
```

```go
// internal/builtin/orchestrator/session.go
package orchestrator

import (
    "fmt"
    "os"
    "path/filepath"
)

// AgentSession provides isolated state for a single agent instance.
// Each agent gets its own:
// - Session ID (unique, non-colliding with other agents)
// - State directory (for temporary files, logs)
// - MCP channel (independent communication)
type AgentSession struct {
    ID       string // Unique session identifier
    StateDir string // Isolated directory for this agent's state
    LogFile  string // Agent-specific log file path
}

// NewAgentSession creates an isolated session for an agent.
// The state directory is created under the osm data directory.
func NewAgentSession(baseDir string) (*AgentSession, error) {
    id := generateAgentSessionID()
    stateDir := filepath.Join(baseDir, "agents", id)
    if err := os.MkdirAll(stateDir, 0o700); err != nil {
        return nil, fmt.Errorf("create agent state dir: %w", err)
    }
    return &AgentSession{
        ID:       id,
        StateDir: stateDir,
        LogFile:  filepath.Join(stateDir, "agent.log"),
    }, nil
}
```

JavaScript exposure (unified module):

```javascript
const orc = require('osm:orchestrator');

// --- Provider ---
// Create a Claude Code agent
const agent = orc.spawnAgent({
    provider: 'claude-code',
    model: 'claude-sonnet-4-20250514',
    workDir: '/path/to/project',
    mcpServers: ['osm mcp'],
});

// --- Output Parsing ---
// Create a parser (pre-loaded with Claude Code patterns)
const parser = orc.newParser();

// Read and parse output
while (agent.isAlive()) {
    const line = agent.readLine(30000);
    if (line === null) break;

    const event = parser.parse(line);
    switch (event.type) {
        case orc.EVENT_RATE_LIMIT:
            log.warn('Rate limited, waiting ' + event.fields.retryAfter + 's');
            time.sleep(parseInt(event.fields.retryAfter) * 1000);
            break;
        case orc.EVENT_PERMISSION:
            // Go-side safety: permission is auto-rejected by default
            // JS only sees the event AFTER rejection
            log.warn('Permission prompt rejected: ' + event.line);
            break;
        case orc.EVENT_COMPLETION:
            log.info('Agent completed task');
            break;
        default:
            output.print(event.line);
    }
}

// --- Recovery ---
const recovery = orc.newRecovery({
    maxRetries: 3,
    rateLimitBackoff: 30000, // ms
    hangTimeout: 300000,     // ms
});

// --- Session Isolation ---
// Each agent gets isolated state automatically via agent.id
log.info('Agent session: ' + agent.id());
```

#### MCP Extension (Go — T240)

Extend the existing `internal/command/mcp.go` with new tools for bidirectional agent communication:

```go
// Added to newMCPServer() in internal/command/mcp.go

// --- registerSession ---
// Agent registers itself with osm, declaring capabilities
type mcpRegisterSessionInput struct {
    SessionID    string   `json:"sessionId" jsonschema:"Unique session identifier"`
    Capabilities []string `json:"capabilities" jsonschema:"List of agent capabilities"`
}

// --- reportProgress ---
// Agent reports progress on current task
type mcpReportProgressInput struct {
    SessionID string  `json:"sessionId" jsonschema:"Session identifier"`
    Status    string  `json:"status" jsonschema:"Current status (working, blocked, waiting)"`
    Progress  float64 `json:"progress" jsonschema:"Completion percentage (0-100)"`
    Message   string  `json:"message" jsonschema:"Human-readable progress message"`
}

// --- reportResult ---
// Agent reports completion of a task
type mcpReportResultInput struct {
    SessionID string `json:"sessionId" jsonschema:"Session identifier"`
    Success   bool   `json:"success" jsonschema:"Whether the task succeeded"`
    Output    string `json:"output" jsonschema:"Task output or error message"`
    FilesChanged []string `json:"filesChanged,omitempty" jsonschema:"List of modified files"`
}

// --- requestGuidance ---
// Agent requests human guidance on an ambiguous situation
type mcpRequestGuidanceInput struct {
    SessionID string   `json:"sessionId" jsonschema:"Session identifier"`
    Question  string   `json:"question" jsonschema:"Question for the human operator"`
    Options   []string `json:"options,omitempty" jsonschema:"Available options"`
    Context   string   `json:"context,omitempty" jsonschema:"Additional context"`
}
```

#### BT Templates (JS — T244)

Pre-built behavior tree patterns exposed as reusable functions. These use the existing `osm:bt` and `osm:pabt` infrastructure — no new Go code.

```javascript
// scripts/bt-templates/orchestrator.js
// Require-able BT templates for AI orchestration workflows

const bt = require('osm:bt');
const pty = require('osm:pty');
const orc = require('osm:orchestrator');
const exec = require('osm:exec');

// Template: Spawn an agent, send a prompt, wait for completion
exports.spawnAndPrompt = function(config, prompt) {
    const bb = new bt.Blackboard();

    return bt.sequence([
        // Step 1: Spawn agent
        bt.createLeafNode(function() {
            const agent = orc.spawnAgent(config);
            bb.set('agent', agent);
            bb.set('parser', orc.newParser());
            return bt.success;
        }),

        // Step 2: Send prompt
        bt.createLeafNode(function() {
            const agent = bb.get('agent');
            agent.send(prompt);
            return bt.success;
        }),

        // Step 3: Wait for completion (with parsed output monitoring)
        bt.createLeafNode(function() {
            const agent = bb.get('agent');
            const parser = bb.get('parser');

            while (agent.isAlive()) {
                const line = agent.readLine(30000);
                if (line === null) break;

                const event = parser.parse(line);
                if (event.type === orc.EVENT_COMPLETION) {
                    bb.set('result', event);
                    return bt.success;
                }
                if (event.type === orc.EVENT_RATE_LIMIT) {
                    return bt.running; // BT will re-tick
                }
            }
            return bt.failure;
        }),

        // Step 4: Cleanup
        bt.createLeafNode(function() {
            const agent = bb.get('agent');
            agent.close();
            return bt.success;
        }),
    ]);
};

// Template: Run tests and verify output
exports.verifyOutput = function(command) {
    return bt.createLeafNode(function() {
        const result = exec.exec('sh', '-c', command);
        return result.code === 0 ? bt.success : bt.failure;
    });
};

// Template: Git commit with message
exports.commitChanges = function(message) {
    return bt.createLeafNode(function() {
        const addResult = exec.exec('git', 'add', '-A');
        if (addResult.code !== 0) return bt.failure;
        const commitResult = exec.exec('git', 'commit', '-m', message);
        return commitResult.code === 0 ? bt.success : bt.failure;
    });
};

// Template: Split diffs into branches (T245)
exports.prSplit = function(branches) {
    return bt.sequence(branches.map(function(branch) {
        return bt.sequence([
            bt.createLeafNode(function() {
                const result = exec.exec('git', 'checkout', '-b', branch.name);
                return result.code === 0 ? bt.success : bt.failure;
            }),
            exports.spawnAndPrompt(branch.config, branch.prompt),
            exports.verifyOutput(branch.verify || 'make test'),
            exports.commitChanges(branch.message),
        ]);
    }));
};
```

### Testing Strategy (Layered)

#### Layer 1: Go Unit Tests (safety-critical paths)

| Package | Test Focus | Method |
|---------|-----------|--------|
| `internal/builtin/pty/` | PTY spawn, read, write, resize, close, signal | Spawn `/bin/cat`, `/bin/echo`; platform tests for Windows ConPTY |
| `internal/builtin/orchestrator/parser.go` | Pattern matching accuracy | Table-driven: 100+ real Claude Code output lines → expected EventType |
| `internal/builtin/orchestrator/safety.go` | Permission rejection | **CRITICAL**: Every known permission prompt format MUST be rejected |
| `internal/builtin/orchestrator/recovery.go` | Retry logic, timeout handling | Time-based tests with short durations |
| `internal/builtin/orchestrator/session.go` | Isolation guarantees | Concurrent session creation, directory cleanup |

#### Layer 2: JS Integration Tests (workflow logic)

| Script | Test Focus | Method |
|--------|-----------|--------|
| BT templates | Tree composition and execution | Mock agent (echo server PTY) |
| Orchestration scripts | End-to-end workflow | Scripted `/bin/cat` simulating agent responses |
| Pattern library | JS-side pattern extensions | Pure JS unit tests |

#### Layer 3: TestMain Integration Tests (T247 — real agents)

```go
// cmd/osm/orchestrator_integration_test.go
// +build integration

func TestMain(m *testing.M) {
    if os.Getenv("OSM_INTEGRATION_PROVIDER") == "" {
        fmt.Println("Skipping integration tests (set OSM_INTEGRATION_PROVIDER)")
        os.Exit(0)
    }
    os.Exit(m.Run())
}

func TestClaudeCodeSpawnAndPrompt(t *testing.T) {
    // Only runs with: OSM_INTEGRATION_PROVIDER=claude-code go test -tags=integration
}
```

### Trade-offs

| Aspect | Assessment |
|--------|------------|
| **Safety** | ✅ Permission rejection, PTY management, signal forwarding — all in Go with compile-time guarantees. |
| **Flexibility** | ✅ BT composition, workflow customization, prompt construction — all in JS, user-modifiable. |
| **Code volume** | ✅ ~800 LOC Go (2 modules), ~400 LOC JS (templates). No interface ceremony. |
| **Existing infra** | ✅ Uses osm:bt, osm:pabt, osm:exec, session, MCP — no reinvention. |
| **Testability** | ✅ Go safety tests are table-driven. JS workflows tested with mock PTYs. Integration tests gated. |
| **Platform compat** | ✅ PTY abstraction in Go handles Unix/Windows. JS scripts are platform-independent. |
| **Discoverability** | ✅ BT templates are require-able. Orchestration scripts can be goals (auto-discovered). |
| **Learning curve** | ⚠️ Users need to understand both Go module APIs and BT patterns. But BT is already documented. |
| **Future providers** | ⚠️ Provider interface is minimal. Adding Ollama requires implementing Agent interface. Not over-designed for unknown requirements. |

---

## 6. Cross-Cutting Concerns

### 6.1 Security

#### PTY Injection

**Risk:** Malicious agent output containing ANSI escape sequences that alter terminal state, inject keystrokes, or execute commands when displayed.

**Mitigation:**
- Output parsing in Go strips known dangerous escape sequences before passing raw lines to JS.
- The `Parser` operates on sanitized text. Raw bytes are only accessible through explicit opt-in (`session.readRaw()`).
- ANSI sequences needed for TUI multiplexing (T246) are processed in Go, not passed through to user scripts.

#### Permission Prompt Handling

**Risk:** Agent requests file deletion, network access, or code execution that the user hasn't approved.

**Mitigation:**
- `PermissionPolicy` defaults to `ActionReject` (hardcoded, not configurable via config file).
- Every permission prompt is logged to the audit log with full context.
- `ActionApprove` rules require explicit code-level opt-in (not configurable via environment variables or config files to prevent accidental exposure).
- The safety module is the **one exception** to osm's "all logic in JS" principle — it MUST remain in Go.

#### Credential Handling

**Risk:** API keys, tokens, or credentials exposed in logs, prompts, or agent output.

**Mitigation:**
- Environment variables containing secrets (matching patterns like `*_KEY`, `*_TOKEN`, `*_SECRET`) are redacted in log output.
- Agent spawn environment is inherited from the parent process — no credential storage in osm.
- MCP session registration does not transmit credentials.

### 6.2 Platform Compatibility

| Component | macOS/Linux | Windows |
|-----------|-------------|---------|
| PTY allocation | `creack/pty` (POSIX `openpty()`) | `golang.org/x/sys/windows` ConPTY |
| Signal forwarding | `SIGINT`, `SIGTERM`, `SIGWINCH` | `GenerateConsoleCtrlEvent` (Ctrl+C), `TerminateProcess` |
| Session isolation | Standard `os.MkdirAll`, `/` paths | `filepath.Join`, `%LOCALAPPDATA%` |
| Process management | `syscall.Kill`, `os.Process.Signal` | `windows.OpenProcess`, `windows.TerminateProcess` |
| MCP transport | Stdio (pipe-based) | Stdio (pipe-based, identical) |

The PTY module (`internal/builtin/pty/`) uses build-tagged files (`pty_unix.go`, `pty_windows.go`) for platform-specific implementation. All exported functions have identical signatures — the platform difference is encapsulated.

**go.mod dependency:** `github.com/creack/pty v1.1.24` is already in go.mod (used by tests today). No new dependency for Unix PTY. Windows ConPTY requires only `golang.org/x/sys` (also already present).

### 6.3 Session Isolation (T249)

Each spawned agent gets an `AgentSession` with:

1. **Unique ID:** Generated via `session.GetSessionID()` with an `agent--` namespace prefix to prevent collisions with user sessions.
2. **Isolated state directory:** `~/.local/share/osm/agents/<agent-id>/` containing the agent's logs, temporary files, and state.
3. **Independent MCP channel:** Each agent connects to its own MCP server instance (or shares one with session-scoped tool namespacing).
4. **Environment isolation:** Agent environment inherits from parent but adds `OSM_SESSION=<agent-id>` to prevent cross-agent state pollution.

Concurrent agents are safe because:
- PTY file descriptors are per-process (no sharing).
- Blackboard state is per-BT-tree (no global state).
- Agent sessions use distinct directories (no file contention).
- MCP sessions are identified by `sessionId` (multiplexed on single server if needed).

### 6.4 Error Recovery (T248)

The recovery system operates at two levels:

**Go level (automatic, non-configurable safety):**
- Process crash detection via `Wait()` + exit code analysis.
- SIGKILL escalation after grace timeout.
- PTY file descriptor cleanup on crash (prevent FD leaks).

**JS level (configurable, user-customizable):**
- Rate limit backoff (exponential, configurable via `RecoveryConfig`).
- Hang detection (no output for N seconds → restart or abort).
- Retry logic (BT `fallback` nodes naturally retry failed branches).
- User-defined recovery handlers via BT tree composition.

```javascript
// Example: Retry with exponential backoff
const retryWithBackoff = bt.fallback([
    attemptTask,                                  // Try once
    bt.sequence([                                  // On failure:
        bt.createLeafNode(() => {
            time.sleep(recovery.rateLimitBackoff);
            return bt.success;
        }),
        attemptTask,                               // Retry
    ]),
    bt.createLeafNode(() => {
        log.error('Task failed after retries');
        return bt.failure;
    }),
]);
```

### 6.5 Performance

| Operation | Expected Latency | Notes |
|-----------|-----------------|-------|
| PTY spawn | ~50ms | Fork + exec + PTY allocation |
| PTY write | ~1ms | Write syscall to PTY fd |
| PTY readLine | Blocking | Depends on agent output speed |
| Parser.Parse | ~5μs | Pre-compiled regex matching |
| BT tick | ~10μs | Depends on tree depth |
| Agent startup | ~2-5s | Claude Code initialization time |

The performance bottleneck is the AI agent itself, not the orchestrator. PTY I/O and parsing are negligible compared to LLM inference time.

---

## 7. Implementation Roadmap

### Dependency Graph

```
T238 (this doc) ─┐
                  ├──► T239 (PTY module) ────────────────────────────────┐
                  │                                                      │
                  ├──► T240 (MCP bidirectional) ──┐                      │
                  │                               │                      │
                  ├──► T242 (Config & env mgmt)   │                      │
                  │                               │                      │
                  └──► T241 (PTY output parsing) ─┤                      │
                                                  │                      │
                       T243 (Provider abstraction)◄┘                     │
                            │                                            │
                            ├──► T244 (BT templates) ◄───────────────────┘
                            │         │
                            │         ├──► T245 (PR splitting workflow)
                            │         │
                            │         └──► T250 (User building blocks)
                            │
                            ├──► T246 (TUI multiplexing)
                            │
                            ├──► T247 (Integration testing)
                            │
                            ├──► T248 (Error recovery)
                            │
                            └──► T249 (Session isolation)
                            
T251 (Claude Code multiplexer) ◄── T243 + T244 + T249
T252 (Ollama module) ◄── T243
T253 (Claude Code parser prod) ◄── T241
T254 (Safety validation) ◄── T241 + T248
T255 (Ideal choice resolution) ◄── T251 + T252
```

### Phase 1: Foundation (T239, T240, T241, T242)

**Goal:** PTY spawning works, MCP is bidirectional, output can be parsed.

| Task | Package | LOC Estimate | Dependencies |
|------|---------|-------------|--------------|
| T239 | `internal/builtin/pty/` | ~400 | `creack/pty` (in go.mod) |
| T240 | `internal/command/mcp.go` (extend) | ~200 | Existing MCP server |
| T241 | `internal/builtin/orchestrator/parser.go` | ~300 | None |
| T242 | `internal/config/` (extend) | ~150 | Existing config |

**Deliverable:** `osm script -e 'var pty = require("osm:pty"); var s = pty.spawn({command: "echo", args: ["hello"]}); output.print(s.readLine(1000)); s.close();'`

### Phase 2: Orchestration (T243, T244, T248, T249)

**Goal:** Agent abstraction, BT templates, recovery, and isolation.

| Task | Package | LOC Estimate | Dependencies |
|------|---------|-------------|--------------|
| T243 | `internal/builtin/orchestrator/provider.go`, `claude_code.go` | ~300 | T239, T241 |
| T244 | `scripts/bt-templates/orchestrator.js` | ~400 (JS) | T243, existing osm:bt |
| T248 | `internal/builtin/orchestrator/recovery.go` | ~200 | T243 |
| T249 | `internal/builtin/orchestrator/session.go` | ~150 | Existing session infra |

**Deliverable:** `osm script orchestrate-single-prompt.js` spawns Claude Code, sends a prompt, parses output, handles rate limits, and collects the result.

### Phase 3: Advanced Workflows (T245, T246, T247, T250)

**Goal:** PR splitting, TUI multiplexing, integration tests, user-facing API.

| Task | Package | LOC Estimate | Dependencies |
|------|---------|-------------|--------------|
| T245 | `scripts/orchestrate-pr-split.js` | ~500 (JS) | T244 |
| T246 | `internal/builtin/orchestrator/tui.go` (or `internal/termui/`) | ~600 | T239, existing bubbletea |
| T247 | `cmd/osm/orchestrator_integration_test.go` | ~300 | T243 |
| T250 | Documentation + module exposure | ~200 | T239, T243, T244 |

**Deliverable:** Full PR splitting workflow with TUI showing active agent sessions.

### Phase 4: Multi-Provider (T251, T252, T253, T254, T255)

**Goal:** Claude Code multiplexer, Ollama, production parser, safety validation.

| Task | Package | LOC Estimate | Dependencies |
|------|---------|-------------|--------------|
| T251 | `internal/builtin/orchestrator/multiplexer.go` | ~400 | T243, T244, T249 |
| T252 | `internal/builtin/orchestrator/ollama.go` | ~250 | T243 |
| T253 | `internal/builtin/orchestrator/patterns.go` (production) | ~300 | T241 |
| T254 | `internal/builtin/orchestrator/safety.go` (hardened) | ~200 | T241, T248 |
| T255 | `scripts/ideal-choice-resolution.js` | ~300 (JS) | T251, T252 |

**Deliverable:** Run multiple Claude Code instances simultaneously, with Ollama as a local testing alternative, full safety validation, and intelligent provider selection.

---

## 8. Decision

### Recommended: Approach C (Pragmatic Balance)

**Rationale:**

1. **Matches osm's DNA.** osm is a scripting-first tool with Go infrastructure. The orchestrator follows the same pattern: Go for safety, JS for logic. This is not a new architecture — it's the existing architecture extended.

2. **Leverages existing infrastructure.** The BT engine (`osm:bt`, `osm:pabt`), MCP server, session system, and scripting engine are production-quality. Approach C builds on top of them rather than replacing or duplicating them.

3. **Safety where it matters.** Permission prompt rejection and PTY lifecycle management are too critical for dynamic JS. These MUST be in compiled, type-checked Go with extensive test coverage. Approach A puts safety in JS (risky). Approach B puts everything in Go (unnecessary).

4. **Pragmatic scope.** Approach C requires ~800 LOC of new Go code (2 modules) and ~400 LOC of JS templates. Approach B would require 2000+ LOC of Go interface ceremony before any real logic. The Go interfaces in Approach C are minimal — `Provider`, `Agent`, `Parser` — and will grow only when real requirements (Ollama, multi-provider) demand it.

5. **User customization is preserved.** Orchestration scripts are goals — users can discover, copy, modify, and share them. The BT templates are `require()`-able. Users who want different workflows write JS, not Go.

6. **T234 alignment.** The code-review-splitter evaluation explicitly chose "defer to AI Orchestrator." Approach C delivers on that promise with minimal wasted effort.

### What This Decision Means for T239–T255

- **T239 (PTY):** Implement `internal/builtin/pty/` as described. This is the foundation.
- **T240 (MCP):** Extend `internal/command/mcp.go` with 4 new tools. No new package.
- **T241 (Parser):** Implement `internal/builtin/orchestrator/parser.go`. Table-driven pattern matching with Claude Code output corpus.
- **T242 (Config):** Extend `internal/config/schema.go` with orchestrator config keys. No new package.
- **T243 (Provider):** Implement `Provider` and `Agent` interfaces in `internal/builtin/orchestrator/`. First impl: Claude Code via PTY.
- **T244 (BT templates):** JS scripts using existing `osm:bt`. No Go code. BT templates are `require()`-able.
- **T245 (PR split):** JS orchestration script using T244 templates. Goal-discoverable.
- **T246 (TUI mux):** Extends existing bubbletea infrastructure. May need a thin Go layer for PTY output multiplexing.
- **T247 (Testing):** TestMain integration tests. Gated by env var. Standard Go testing patterns.
- **T248 (Recovery):** Go-level safety (crash detection, SIGKILL escalation) + JS-level policy (retry, backoff).
- **T249 (Isolation):** Build on existing `internal/session/` with agent namespace prefix.
- **T250 (Building blocks):** Documentation + module exposure. The modules from T239/T243 ARE the building blocks.
- **T251–T255 (Advanced):** Multiplexer, Ollama, production parser, safety hardening. These extend the foundation without changing the architecture.

### File Location Summary

| New File | Purpose | Task |
|----------|---------|------|
| `internal/builtin/pty/pty.go` | PTY session management | T239 |
| `internal/builtin/pty/pty_unix.go` | Unix PTY (creack/pty) | T239 |
| `internal/builtin/pty/pty_windows.go` | Windows ConPTY | T239 |
| `internal/builtin/pty/signal.go` | Signal forwarding | T239 |
| `internal/builtin/pty/require.go` | `osm:pty` module registration | T239 |
| `internal/builtin/orchestrator/parser.go` | Output event parser | T241 |
| `internal/builtin/orchestrator/patterns.go` | Compiled regex patterns | T241/T253 |
| `internal/builtin/orchestrator/provider.go` | Provider + Agent interfaces | T243 |
| `internal/builtin/orchestrator/claude_code.go` | Claude Code provider | T243 |
| `internal/builtin/orchestrator/session.go` | Agent session isolation | T249 |
| `internal/builtin/orchestrator/recovery.go` | Error recovery policies | T248 |
| `internal/builtin/orchestrator/safety.go` | Permission rejection (**CRITICAL**) | T254 |
| `internal/builtin/orchestrator/require.go` | `osm:orchestrator` module registration | T243 |
| `scripts/bt-templates/orchestrator.js` | BT orchestration templates | T244 |
| `scripts/orchestrate-pr-split.js` | PR splitting workflow | T245 |

### Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Claude Code output format changes break parser | High | Medium | Version-specific pattern sets. Parser extensible via `AddPattern`. CI integration tests detect breakage. |
| Permission prompt format not recognized → auto-approved | Low | **CRITICAL** | Default-reject policy. Unknown prompts are rejected. Safety tests cover every known format. |
| ConPTY on Windows is flaky | Medium | Medium | Windows CI testing. Graceful fallback to non-PTY `exec` mode (degraded but functional). |
| Event loop migration (T237 Tier 2) changes scripting semantics | Low | High | Orchestrator modules use synchronous Go APIs exposed to sync JS calls. No async dependency. Migration is orthogonal. |
| BT is wrong abstraction for long-running workflows | Medium | Low | BT `running` status + ticker pattern handles long waits naturally. `createBlockingLeafNode` prevents event loop starvation. |

---

*This document is the gate for T239–T255. All implementation tasks should reference this architecture and follow the recommended Approach C patterns.*
