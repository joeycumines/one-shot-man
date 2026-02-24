# Claude-Mux Architecture

> **Status:** Core building blocks implemented (T001–T019). CLI entry point (`osm claude-mux`) operational.
> **Last updated:** 2026-02-20

## Contents

1. [Overview](#1-overview)
2. [Design Philosophy](#2-design-philosophy)
3. [Two-Channel Architecture](#3-two-channel-architecture)
4. [Full Invocation Path](#4-full-invocation-path)
5. [Module Reference](#5-module-reference)
   - [osm:pty](#51-osmpty)
   - [osm:claudemux](#52-osmclaudemux)
   - [BT Templates](#53-bt-templates)
   - [PR Splitting Workflow](#54-pr-splitting-workflow)
   - [Automated Splitting Pipeline](#55-automated-splitting-pipeline)
6. [MCP Feedback Protocol](#6-mcp-feedback-protocol)
7. [Security Model](#7-security-model)
8. [Session Isolation](#8-session-isolation)
9. [Platform Support](#9-platform-support)
10. [Testing](#10-testing)
11. [Roadmap](#11-roadmap)

---

## 1. Overview

**Claude-Mux** is the `osm` subsystem for programmatic spawning, monitoring, and
coordination of Claude Code instances from JavaScript workflows. It extends osm's
behavior tree engine (`osm:bt`, `osm:pabt`), MCP server, and scripting infrastructure
to automate multi-agent development workflows.

The name reflects the architecture: a **multiplexer** for Claude Code instances,
managing multiple concurrent sessions with isolated state, PTY I/O channels, and
structured MCP feedback channels.

### What Claude-Mux provides

- **PTY channel** — Spawn processes in pseudo-terminals for launch sequencing and
  session lifecycle monitoring (`osm:pty`).
- **MCP channel** — Structured bidirectional data exchange: progress reports, results,
  guidance requests, errors (`osm:claudemux` + extended `osm:mcp`).
- **Output classification** — Parse agent terminal output into typed events: rate
  limits, permission prompts, model selection, SSO flows, tool invocations, errors.
- **Provider abstraction** — Pluggable provider registry with a Claude Code
  implementation backed by PTY + MCP.
- **Session isolation** — Per-instance state directories, independent PTY handles,
  locked session IDs, resource cleanup.
- **Workflow scripts** — Composable JavaScript workflows using BT templates.

### What Claude-Mux does not do

- Replace osm's clipboard-first philosophy — opt-in for power users.
- Call any AI API directly — all communication goes through PTY or MCP channels.
- Manage API keys or credentials — secrets remain in the parent environment.

---

## 2. Design Philosophy

**Go for infrastructure and safety. JavaScript for workflow logic.**

Safety-critical paths — PTY lifecycle, signal handling, permission prompt rejection,
output parsing — are implemented in Go with type safety and comprehensive tests.
Workflow orchestration, prompt construction, and user customization remain in
JavaScript, leveraging the existing BT engine.

| Concern | Layer | Rationale |
|---------|-------|-----------|
| PTY spawn, resize, signal, close | Go (`internal/builtin/pty`) | OS-specific, safety-critical, requires `creack/pty` |
| Output pattern matching | Go (`internal/builtin/claudemux`) | Compiled regex, table-driven, extensible from JS |
| Permission prompt rejection | Go (default-reject policy) | Too critical for dynamic JS — must never accidentally approve |
| Provider abstraction | Go (`Provider`, `AgentHandle` interfaces) | Type-safe contract for multiple backends |
| Agent lifecycle monitoring | Go (goroutine + channel) | Process exit detection requires OS-level wait |
| MCP feedback tools | Go (`internal/command/mcp.go`) | Schema validation, idempotency, sequence numbers |
| BT tree composition | JS (embedded in `internal/command/pr_split_script.js`) | Leverages existing `osm:bt`, part of `osm pr-split` command |
| Workflow scripts | Go+JS (`internal/command/pr_split.go` + embedded JS) | Built-in command, no external scripts |
| Prompt construction | JS (existing `context` / `output` globals) | Unchanged — same API as all osm scripts |

---

## 3. Two-Channel Architecture

Claude-Mux uses **two separate channels** for communication with each Claude Code
instance. They serve distinct purposes and must not be conflated.

```
┌─────────────────────────────────────────────────────────────────┐
│  osm (parent process)                                           │
│                                                                 │
│  ┌───────────────┐        ┌─────────────────────────────────┐  │
│  │  PTY Channel  │        │  MCP Channel                    │  │
│  │               │        │                                 │  │
│  │  pty.Spawn()  │        │  Per-instance MCP server        │  │
│  │  ├── Write()  │        │  ├── reportProgress(step, %)    │  │
│  │  ├── Read()   │        │  ├── reportResult(type, data)   │  │
│  │  ├── Resize() │        │  ├── requestGuidance(question)  │  │
│  │  └── Signal() │        │  └── reportError(type, msg)     │  │
│  └───────┬───────┘        └──────────────┬──────────────────┘  │
│          │                               │                      │
└──────────┼───────────────────────────────┼──────────────────────┘
           │ PTY fd (stdin/stdout/stderr)  │ MCP JSON config
           │                               │ (injected at spawn)
           ▼                               ▼
   ┌───────────────────────────────────────────────────────────┐
   │  Claude Code instance (child process)                     │
   │                                                           │
   │  Session lifecycle events → PTY stdout                   │
   │  Structured feedback → MCP tools (reportProgress, etc.)  │
   └───────────────────────────────────────────────────────────┘
```

### Channel responsibilities

| Channel | Direction | Purpose |
|---------|-----------|---------|
| **PTY** | Bidirectional | Launch sequencing, model selection navigation, session lifecycle, rate-limit/permission detection |
| **MCP** | Inbound (from Claude) | Structured progress/result/guidance/error reporting |

**Why two channels?**

PTY is inherently unstructured — free-form text output. It is suitable for detecting
session lifecycle events (launch, model selection, rate limit, crash) but poor for
structured data transfer. MCP is structured protocol-based and ideal for returning
results, requesting guidance, and reporting partial progress. Combining both gives
full control over the agent lifecycle without parsing fragile JSON from terminal output.

---

## 4. Full Invocation Path

The full path from `osm` to a running Claude Code session has three sequential phases:

```
Phase 1: Wrapper launch     Phase 2: Model selection    Phase 3: Task execution
                           (TUI navigation via PTY)
   osm spawns              PTY reads model list          Claude active
   ─────────────────►      ──────────────────────►       ─────────────────►
                                                          ├── PTY: monitoring
   ollama launch claude    parser detects menu            │   (rate-limit,
   --config <mcp-config>   send arrow keys + enter        │    permission,
                           parser detects selected         │    lifecycle)
                           model confirmation              │
                                                          └── MCP: feedback
                                                              (results, errors,
                                                               guidance req.)
```

### Phase 1: Wrapper launch

```
osm claudemux spawn (or JS: registry.spawn('claude-code', opts))
  │
  ├─ 1. Generate per-instance MCP config → temp JSON file
  │      { mcpServers: { osm: { url: "http://127.0.0.1:<random-port>/mcp" }}}
  │
  ├─ 2. Start per-instance MCP server endpoint
  │      Binds to random TCP port (Windows) or Unix socket (macOS/Linux)
  │
  └─ 3. pty.Spawn("ollama", ["launch", "claude", "--config", configPath], opts)
         Creates PTY, starts process, background goroutine monitors exit
```

### Phase 2: Model selection (TUI navigation)

Claude Code presents an interactive model selection menu over the PTY:

```
  > gpt-oss:20b-cloud
    gpt-oss:8b-cloud
    gpt-oss:7b-local
```

The PTY output parser detects the model selection menu (`EVENT_MODEL_SELECT`). The
model selection navigator generates the required key sequences (↑/↓ arrows + Enter)
to programmatically select the configured model. Parser then confirms the selected
model and transitions to Phase 3.

```javascript
var claudemux = require('osm:claudemux');
var parser = claudemux.newParser();

// After spawning, read PTY output until model selection menu detected
var event = parser.parse(line);
if (event.type === claudemux.EVENT_MODEL_SELECT) {
    // event.fields.availableModels = ['gpt-oss:20b-cloud', ...]
    // event.fields.selectedIndex = 0
    // Navigate: send arrow keys to position, then '\n'
    agent.send('\n');   // accept first/correct model
}
```

### Phase 3: Task execution

Once the model selection completes, Claude Code is ready. The task is submitted as
input over the PTY channel. Structured feedback flows back over MCP:

```
JS sends task prompt via agent.send(prompt)
    │
    ▼ PTY stdin
Claude Code processes task
    │
    ├──► PTY stdout   Agent lifecycle monitoring only (rate-limit, crash, etc.)
    │    (parser.parse per line → EVENT_RATE_LIMIT | EVENT_PERMISSION | EVENT_TEXT)
    │
    └──► MCP tools    Structured feedback
         ├── reportProgress(sessionId, step, message, percent)
         ├── reportResult(sessionId, resultType, data)
         ├── requestGuidance(sessionId, question, options)
         └── reportError(sessionId, errorType, message, recoverable)
```

---

## 5. Module Reference

### 5.1. `osm:pty`

**Package:** `internal/builtin/pty`
**Platform:** Unix complete (macOS + Linux). Windows: `ErrNotSupported` (ConPTY planned).

Spawns processes in pseudo-terminals with full bidirectional I/O, window resizing,
and signal delivery.

#### JavaScript API

```javascript
var pty = require('osm:pty');

var proc = pty.spawn('bash', ['-l'], {
    rows: 24, cols: 80,
    dir: '/path/to/project',
    env: { TERM: 'xterm-256color' }
});

proc.write('echo hello\n');
var output = proc.read();          // "" on no available data, up to 4096 bytes
proc.resize(48, 120);
proc.signal('SIGINT');             // 'SIGTERM', 'SIGKILL', etc.

var result = proc.wait();          // { code: 0, error: null }
proc.close();                      // SIGTERM → 5s wait → SIGKILL. Idempotent.
```

#### Go API

```go
type SpawnConfig struct {
    Command string
    Args    []string
    Env     map[string]string
    Dir     string
    Rows    uint16   // default: 24
    Cols    uint16   // default: 80
    Term    string   // default: "xterm-256color"
}

func Spawn(ctx context.Context, cfg SpawnConfig) (*Process, error)
func (p *Process) Write(data string) error
func (p *Process) Read() (string, error)
func (p *Process) Resize(rows, cols uint16) error
func (p *Process) Signal(sig string) error
func (p *Process) Wait() (exitCode int, err error)
func (p *Process) IsAlive() bool
func (p *Process) Pid() int
func (p *Process) Close() error
```

---

### 5.2. `osm:claudemux`

**Package:** `internal/builtin/claudemux`
**Status:** Core building blocks implemented (T003–T019). CLI: `osm claude-mux`.

Provides output classification, guard rails, error recovery, concurrent instance
management, TUI multiplexing, safety validation, choice resolution, and session
isolation for multi-instance Claude Code orchestration.

See [Claude-Mux Reference](reference/claude-mux.md) for complete API documentation
and [Scripting](scripting.md#osmclaudemux-claude-mux-orchestration) for the
JavaScript API.

#### Output Parser

Classifies raw terminal output lines into typed events via compiled regex patterns.

**Event types:**

| Constant | Value | Description | Example match |
|----------|-------|-------------|---------------|
| `EVENT_TEXT` | 0 | Normal text (no pattern matched) | — |
| `EVENT_RATE_LIMIT` | 1 | Rate limit / 429 / backoff | `"try again in 30 seconds"` |
| `EVENT_PERMISSION` | 2 | Permission prompt (Y/N) | `"Allow? [y/N]"` |
| `EVENT_MODEL_SELECT` | 3 | Model selection menu detected | `"Select a model"` |
| `EVENT_SSO_LOGIN` | 4 | SSO / OAuth flow | `"Opening your browser"` |
| `EVENT_COMPLETION` | 5 | Task completed | `"Task completed"` |
| `EVENT_TOOL_USE` | 6 | MCP tool invocation | `"Calling tool: readFile"` |
| `EVENT_ERROR` | 7 | Error message | `"Error: file not found"` |
| `EVENT_THINKING` | 8 | Thinking indicator | `"Thinking..."` |

```javascript
var claudemux = require('osm:claudemux');

var parser = claudemux.newParser();
var event = parser.parse('Try again in 30 seconds');
// event.type === claudemux.EVENT_RATE_LIMIT
// event.fields.retryAfter === "30"
// event.pattern === "rate-limit-try-again"

// Add custom patterns for provider-specific output
parser.addPattern('my-done', 'BUILD SUCCESSFUL', claudemux.EVENT_COMPLETION);

// Human-readable event type name
claudemux.eventTypeName(claudemux.EVENT_RATE_LIMIT); // "rate_limit"
```

#### Provider Abstraction

```javascript
var claudemux = require('osm:claudemux');

var registry = claudemux.newRegistry();
var claude = claudemux.claudeCode({ command: 'claude' }); // or full path
registry.register(claude);

var agent = registry.spawn('claude-code', {
    model: 'gpt-oss:20b-cloud',
    dir: '/path/to/project',
    rows: 24, cols: 200
});

agent.send('Fix the failing test in pkg/foo_test.go\n');
var output = agent.receive();   // Non-blocking read from PTY

var result = agent.wait();      // { code: 0, error: null }
agent.close();                  // Graceful shutdown
```

**Go interfaces:**

```go
// Provider abstracts an AI agent backend.
type Provider interface {
    Name() string
    Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error)
    Capabilities() ProviderCapabilities
}

// AgentHandle represents a running agent instance.
type AgentHandle interface {
    Send(input string) error
    Receive() (string, error)    // Returns ("", io.EOF) on agent exit
    Close() error
    IsAlive() bool
    Wait() (int, error)
}
```

**Registry** is thread-safe via `sync.RWMutex`:

```go
func NewRegistry() *Registry
func (r *Registry) Register(p Provider) error    // ErrProviderExists if duplicate
func (r *Registry) Get(name string) (Provider, error)
func (r *Registry) List() []string               // Sorted names
func (r *Registry) Spawn(ctx, name, opts) (AgentHandle, error)
```

---

### 5.3. BT Templates

**File:** `internal/command/pr_split_script.js` (embedded via `//go:embed`)
**Status:** Consolidated into `osm pr-split` built-in command (v5.0.0).
**Dependencies:** `osm:bt`, `osm:claudemux`, `osm:exec`

Reusable behavior tree building blocks for Claude-Mux workflows.
All leaf factories use `bt.createBlockingLeafNode` for sequential execution semantics.

#### Leaf Node Factories

Each factory returns a `bt.Node` and communicates via a shared `bt.Blackboard`.

| Factory | BB reads | BB writes | Purpose |
|---------|----------|-----------|---------|
| `btVerifyOutput(bb, command)` | — | `verifyCode`, `verified` | Run shell command, check exit |
| `btRunTests(bb, command)` | — | `testCode`, `testsPassed` | Run test command |
| `btCommitChanges(bb, message)` | — | `commitOutput`, `committed` | `git add -A && git commit` |
| `btSplitBranch(bb, branchName)` | — | `currentBranch`, `branchCreated` | `git checkout -b` |

Claude interaction (spawn, prompt, response) is handled by `ClaudeCodeExecutor`
rather than individual BT leaf nodes. See [§5.5 Automated Splitting Pipeline](#55-automated-splitting-pipeline).

#### Workflow Composers

```javascript
var ps = globalThis.prSplit;
var bt = require('osm:bt');

var bb = new bt.Blackboard();

// Run tests → optional verify → commit
ps.verifyAndCommit(bb, {
    testCommand: 'make test',
    verifyCommand: 'git diff --check',
    message: 'fix: resolve compilation error in parser'
});
```

---

### 5.4. PR Splitting Workflow

**Command:** `osm pr-split` (built-in, `internal/command/pr_split.go`)
**Script:** `internal/command/pr_split_script.js` (embedded via `//go:embed`)
**Dependencies:** `osm:bt`, `osm:exec`

Splits a large diff into a linear series of stacked, independently-reviewable branches.

#### Architecture

```
Analysis → Grouping → Planning → Execution → Verification
   │           │          │           │             │
analyzeDiff  groupBy*  createSplit executeSplit  verifyEquivalence
               (6        Plan       (linear         (git tree
             strategies)            stacking)       hash compare)
```

#### Grouping strategies

| Strategy | Function | Use case |
|----------|----------|----------|
| Directory | `groupByDirectory(files, depth)` | Group by top-level package |
| Directory-deep | `groupByDirectory(files, 2)` | Group by two-level nesting |
| Extension | `groupByExtension(files)` | Separate `.go` from `.md` |
| Pattern | `groupByPattern(files, patterns)` | Named regex patterns |
| Chunks | `groupByChunks(files, maxPerGroup)` | Fixed-size groups |
| Dependency | `groupByDependency(files)` | Go import graph merge |
| Auto | `selectStrategy(files)` | Auto-select best strategy |

#### Linear branch stacking

```
main → split/01-cmd → split/02-docs → split/03-pkg
```

Each split branch is based on the previous, creating a linear series that can be
reviewed and merged independently in order.

#### Equivalence verification

`verifyEquivalence(plan)` compares git tree hashes: the last split branch must have
an identical tree to the source branch (`splitTree === sourceTree`).

### 5.5. Automated Splitting Pipeline

When Claude Code is available, `automatedSplit(config)` orchestrates a 10-step
pipeline that combines AI classification with deterministic execution:

```
┌─ Step  1: Analyze diff ──────────── analyzeDiff()
│   │
│   ▼
│  Step  2: Spawn Claude ──────────── ClaudeCodeExecutor.resolve() + spawn()
│   │                                  │
│   ├── [Claude unavailable] ──────── heuristicFallback()  ← (exit pipeline)
│   │
│   ▼
│  Step  3: Send classification ───── renderClassificationPrompt() → handle.send()
│   │
│   ▼
│  Step  4: Receive classification ── pollForFile('classification.json')
│   │                                  │
│   │                                  └── assigns uncategorized if files missing
│   ▼
│  Step  5: Generate split plan ───── classificationToGroups() → createSplitPlan()
│   │                                  │
│   │                                  └── also checks for Claude-provided split-plan.json
│   ▼
│  Step  6: Execute split ─────────── executeSplit(plan)  [skipped in dry-run]
│   │
│   ▼
│  Step  7: Verify splits ─────────── verifySplits(plan)
│   │
│   ├── [all pass] ───────────────── continue to Step 10
│   │
│   ▼
│  Step  8: Resolve conflicts ─────── resolveConflictsWithClaude()
│   │                                  ├── AUTO_FIX_STRATEGIES (local)
│   │                                  └── claude-fix (sends to Claude)
│   │
│   ├── [reSplitNeeded = true] ────── Step 9
│   │
│   ▼
│  Step  9: Re-split ──────────────── re-classify → re-plan → re-execute → re-verify
│   │                                  (up to maxReSplits iterations)
│   │
│   ▼
└─ Step 10: Equivalence + report ──── verifyEquivalence() + assessIndependence()
```

#### MCP tool usage

| Step | Direction | MCP tool / File |
|------|-----------|-----------------|
| 3 | osm → Claude | PTY stdin (classification prompt) |
| 4 | Claude → osm | `classification.json` in resultDir |
| 5 | Claude → osm | `split-plan.json` (optional, validated if present) |
| 8 | osm → Claude | PTY stdin (conflict resolution prompt) |
| 8 | Claude → osm | `resolution.json` in resultDir |
| 9 | osm → Claude | PTY stdin (re-classification prompt) |
| 9 | Claude → osm | Updated `classification.json` |

#### Fallback paths

Three fallback scenarios are handled:

1. **Claude unavailable** (Step 2 fails): Falls back to `heuristicFallback()`,
   which uses the configured grouping strategy (default: `directory`) without
   any Claude interaction. Report includes `fallbackUsed: true`.

2. **Fix strategies exhausted** (Step 8): If `resolveConflicts` exhausts its
   `retryBudget` and strategies still fail, sets `reSplitNeeded: true` to
   trigger Step 9. Report includes `reSplitFiles` and `reSplitReason`.

3. **Re-split exhausted** (Step 9 loops > `maxReSplits`): Pipeline exits with
   current state. Equivalence may fail. User can intervene manually via TUI
   commands (`move`, `fix`, `execute`).

#### Configuration knobs

| Config | Default | Effect |
|--------|---------|--------|
| `classifyTimeoutMs` | 120000 | Max wait for classification.json |
| `planTimeoutMs` | 120000 | Max wait for split-plan.json |
| `resolveTimeoutMs` | 180000 | Max wait per resolution attempt |
| `pollIntervalMs` | 1000 | File polling interval |
| `maxResolveRetries` | 3 | Max retries per failed split |
| `maxReSplits` | 1 | Max full re-classification cycles |
| `retryBudget` | 3 | TUI-settable resolve attempt budget |

---

## 6. MCP Feedback Protocol

MCP is the **primary** means by which Claude Code instances send structured data back
to `osm`. PTY is for session lifecycle only.

### Tools

All MCP feedback tools are scoped to a `sessionId` to support concurrent instances.
See [`osm mcp` reference](reference/command.md#osm-mcp) for full JSON schemas.

#### `registerSession`

Creates a new agent session with a unique ID and capability list. The session ID
is validated (non-empty, max 256 chars, no control characters).

```json
{
  "sessionId": "agent-1",
  "capabilities": ["code-review", "testing"]
}
```

#### `reportProgress`

Signals ongoing work with status and percent completion.

```json
{
  "sessionId": "agent-1",
  "status": "working",
  "progress": 45.0,
  "message": "Found 47 changed files across 8 packages",
  "seq": 1
}
```

Status must be one of: `working`, `blocked`, `waiting`, `idle`. Progress is clamped
to 0–100.

#### `reportResult`

Signals task completion with success/failure and output.

```json
{
  "sessionId": "agent-1",
  "success": true,
  "output": "All tests passed",
  "filesChanged": ["pkg/parser.go", "pkg/parser_test.go"],
  "seq": 2
}
```

#### `requestGuidance`

Pauses the agent and asks the orchestrating workflow for a decision.

```json
{
  "sessionId": "agent-1",
  "question": "Should I rewrite pkg/parser.go from scratch or patch in-place?",
  "options": ["rewrite", "patch", "skip"],
  "context": "The current code has 12 known issues.",
  "seq": 3
}
```

The JavaScript workflow receives this via the MCP server and can pause the BT tree
until the user (or another automated decision-maker) provides the answer.

#### `heartbeat`

Updates the session's heartbeat timestamp to signal the agent is still alive.
Orchestrators detect stale agents by comparing `lastHeartbeat` against a timeout.

```json
{
  "sessionId": "agent-1"
}
```

#### `getSession` / `listSessions`

`getSession` retrieves full session state and **drains** queued events (progress,
result, guidance). `listSessions` returns summaries with event counts.

### Idempotency

The `reportProgress`, `reportResult`, and `requestGuidance` tools accept an optional
`seq` field for deduplication. The MCP server maintains a per-session `lastSeq`
counter. When `seq` > 0:

- If `seq` > `lastSeq`: processed normally, `lastSeq` updated
- If `seq` ≤ `lastSeq`: silently skipped as duplicate (returns `"duplicate seq N"`)
- If `seq` = 0 or omitted: no deduplication, always processed

---

## 7. Security Model

### Permission prompt rejection

**This is the most safety-critical code in Claude-Mux.**

When a Claude Code instance requests file deletion, network access, or code execution,
it produces a permission prompt on stdout. Claude-Mux provides defense in depth:

1. **Go parser (primary):** `Parser.Parse()` classifies lines matching permission
   patterns as `EVENT_PERMISSION`. Built-in patterns match `Allow? [y/N]`,
   `do you want to allow/proceed/continue`, and `permission required/needed/denied`.

2. **JS templates (secondary):** `waitForResponse` sends `"n\n"` for every
   `EVENT_PERMISSION` event, explicitly rejecting the prompt.

3. **Default-reject policy:** If a permission prompt fails to match any pattern,
   it is classified as `EVENT_TEXT` and the agent receives no `"y"` response. The
   absence of a "yes" response does not equal approval — Claude Code treats silence
   as rejection.

Custom patterns can be added via `parser.addPattern()` for provider-specific prompt
formats not covered by the built-ins.

### PTY isolation

- Each agent runs in its own PTY with an independent file descriptor.
- PTY output is read in Go and passed through the parser before reaching JavaScript.
- `Close()` sends `SIGTERM`, waits 5 seconds, then `SIGKILL`. Resource leaks are
  prevented by explicit cleanup.

### MCP endpoint isolation

- Each Claude Code instance connects to its own per-instance MCP endpoint.
- Endpoint URLs are not shared between sessions.
- Session IDs in MCP calls are validated against known active sessions.

### Credential handling

- Agents inherit the parent environment. No credentials are stored by Claude-Mux.
- The `Env` field in `SpawnOpts` adds variables but never removes inherited ones.
- MCP tool calls never transmit credentials.

---

## 8. Session Isolation

Each Claude Code instance gets fully isolated state:

```
~/.osm/claude-sessions/
├── <session-id-1>/     # First instance
│   ├── state.json
│   ├── mcp-config.json   # Per-instance MCP endpoint config
│   └── logs/
└── <session-id-2>/     # Second instance
    ├── state.json
    ├── mcp-config.json
    └── logs/
```

- **Session ID** — Auto-determined via the existing session locking mechanism.
  Each instance acquires its own lock file to prevent corruption.
- **MCP config** — Written to the session directory at spawn time, injected into
  Claude Code as `--config <path>`. Cleaned up on `Close()`.
- **PTY handle** — Owned by the instance struct; not shared.
- **Resource cleanup** — `Close()` removes the MCP config file, terminates the PTY
  process, and releases the session lock.

---

## 9. Platform Support

| Component | macOS | Linux | Windows |
|-----------|-------|-------|---------|
| `osm:pty` spawn/read/write/resize/signal | ✅ | ✅ | ⬜ ErrNotSupported |
| `osm:claudemux` parser | ✅ | ✅ | ✅ |
| `osm:claudemux` provider/registry | ✅ | ✅ | ✅ |
| `osm:claudemux` Claude Code provider | ✅ | ✅ | ⬜ (needs PTY) |
| BT templates (embedded in `osm pr-split`) | ✅ | ✅ | ✅ (spawn excluded) |
| PR splitting workflow | ✅ | ✅ | ✅ (pure `osm:exec` git) |
| Goal discovery | ✅ | ✅ | ✅ |

**Windows PTY (planned):** ConPTY support via `golang.org/x/sys/windows`. The
`Process` struct and `processHandle` interface are designed for platform-specific
backends. The `pty_windows.go` stub returns `ErrNotSupported` until ConPTY is wired.

---

## 10. Testing

### Test coverage

| File | Coverage |
|------|----------|
| `internal/builtin/claudemux/parser_test.go` | Parser patterns, custom patterns, event type names |
| `internal/builtin/claudemux/provider_test.go` | Registry CRUD, concurrent access, error cases |
| `internal/builtin/claudemux/templates_test.go` | BT template loading, leaf execution, PA-BT actions |
| `internal/builtin/claudemux/pr_split_test.go` | Grouping, analysis, execution, equivalence, BT workflow |
| `internal/builtin/pty/pty_test.go` | PTY spawn, read/write, resize, signal, close lifecycle |

### Testing strategy

**Layer 1 — Go unit tests (safety-critical paths):**
- Parser: Table-driven tests mapping raw output lines to expected `EventType` values.
- Provider: Registry operations, concurrent registration, error wrapping.
- PTY: Spawn system echo/cat binaries, test read/write/close lifecycle.

**Layer 2 — JS integration tests (workflow logic):**
- BT templates: Load embedded `pr_split_script.js` in a Goja runtime, verify exports, execute nodes
  with mocked dependencies.
- PR splitting: Create temporary git repos, execute full split workflows, verify tree
  hash equivalence.

**Layer 3 — Integration tests (real agents, P025):**
- Gated by `-integration` flag (disabled by default).
- Requires `ollama launch claude` installed and accessible.
- Tests spawn real agents, send prompts, navigate model selection, verify MCP feedback.
- Config.mk target: `make integration-test`.

---

## 11. TUI Multiplexing

### Overview

TUI multiplexing allows the user to switch between osm's TUI (go-prompt REPL) and
Claude Code's TUI (running in a PTY) without leaving osm. This is implemented in
`internal/termui/mux/` as the `TUIMux` type.

### Design Rationale: Command-Blocking Model

The TUI mux uses a **command-blocking model** rather than a reader-interception model.
This is simpler and more robust:

1. User types `claude` in osm's go-prompt REPL.
2. The `claude` TUI command handler calls `TUIMux.RunPassthrough(ctx)`.
3. `RunPassthrough` blocks, running a forwarding loop:
   - Read raw bytes from stdin → write to Claude's PTY (except toggle key)
   - Read bytes from Claude's PTY → write to stdout
4. When user presses the toggle key (Ctrl+], `0x1D`), the loop exits.
5. The command handler returns, go-prompt resumes.

This avoids modifying `TerminalIO`, `TUIReader`, or any go-prompt internals.
go-prompt is naturally paused because its command handler is still blocking.

```
┌──────────────────────────────────────────────────────────────────────┐
│  Terminal (real stdin/stdout)                                         │
│                                                                      │
│  ┌────────────────────┐         ┌──────────────────────────────┐    │
│  │  osm mode (normal) │         │  Claude mode (passthrough)   │    │
│  │                    │         │                              │    │
│  │  stdin → go-prompt │  Ctrl+] │  stdin → Claude PTY          │    │
│  │  stdout ← go-prompt│ ◄─────► │  stdout ← Claude PTY         │    │
│  │                    │         │                              │    │
│  │  TUIReader/Writer  │         │  Raw byte forwarding         │    │
│  │  (normal path)     │         │  (TUIMux.RunPassthrough)     │    │
│  └────────────────────┘         └──────────────────────────────┘    │
│                                                                      │
│               ┌─────────────────────────────────┐                    │
│               │  Status Bar (last terminal row)  │                    │
│               │  [osm] or [Claude] | status     │                    │
│               │  Ctrl+] to toggle               │                    │
│               └─────────────────────────────────┘                    │
└──────────────────────────────────────────────────────────────────────┘
```

### Terminal State Management

When switching modes, terminal state must be cleanly managed:

**Switch to Claude (enter passthrough):**
1. Save osm's terminal state via `term.GetState(fd)`
2. Put terminal in raw mode (for byte-level forwarding)
3. Set scroll region to rows 1..(height-1) for status bar
4. Start bidirectional forwarding goroutines
5. Render status bar on last row

**Switch to osm (exit passthrough):**
1. Cancel forwarding goroutines
2. Reset scroll region to full terminal
3. Restore saved terminal state
4. Clear status bar
5. Return from RunPassthrough (go-prompt resumes)

### Status Bar

A single-line status bar on the bottom row shows:
- Active mode: `[osm]` or `[Claude]`
- Claude status: idle, thinking, tool-use, error
- Toggle hint: `Ctrl+] to switch`

The status bar uses ANSI escape sequences:
- **Save/restore cursor:** `\033[s` / `\033[u`
- **Scroll region:** `\033[1;Nr` (restrict to first N rows)
- **Position:** `\033[N;1H` (move to row N)
- **Styling:** Reverse video `\033[7m` for visibility

### Forwarding Architecture

Two goroutines handle bidirectional I/O:

```go
// stdin → Claude PTY (with toggle key interception)
go func() {
    buf := make([]byte, 4096)
    for {
        n, err := stdin.Read(buf)
        if err != nil || ctx.Err() != nil { return }
        // Scan for toggle key (0x1D = Ctrl+])
        for i := 0; i < n; i++ {
            if buf[i] == 0x1D {
                cancel() // exit passthrough
                return
            }
        }
        child.Write(buf[:n])
    }
}()

// Claude PTY → stdout
go func() {
    buf := make([]byte, 4096)
    for {
        n, err := child.Read(buf)
        if err != nil { cancel(); return } // Claude exited
        stdout.Write(buf[:n])
    }
}()
```

### Edge Cases

1. **Claude exits while muxed:** The PTY read goroutine gets EOF, cancels the
   context, `RunPassthrough` returns with a "Claude exited" status.

2. **Terminal resize (SIGWINCH):** The mux propagates new dimensions to Claude's
   PTY via `pty.Resize()` and updates the scroll region for the status bar.

3. **Background MCP monitoring:** While Claude's TUI is active, osm's MCP server
   continues running. MCP events are logged and can be queried after switching back.

### Go API

```go
package mux

type Side int
const (
    SideOsm    Side = iota
    SideClaude
)

type TUIMux struct { /* ... */ }

func New(stdin io.Reader, stdout io.Writer, termFd int) *TUIMux
func (m *TUIMux) Attach(child io.ReadWriteCloser) error
func (m *TUIMux) Detach() error
func (m *TUIMux) RunPassthrough(ctx context.Context) (ExitReason, error)
func (m *TUIMux) ActiveSide() Side
func (m *TUIMux) SetToggleKey(key byte)
func (m *TUIMux) SetResizeFunc(fn func(rows, cols uint16) error)

type ExitReason int
const (
    ExitToggle     ExitReason = iota // user pressed toggle key
    ExitChildExit                    // child process exited
    ExitContext                      // context cancelled
    ExitError                        // I/O error
)
```

### JavaScript API

Exposed via `tui.mux` global in pr-split:

```javascript
// Attach Claude's PTY
tui.mux.attach(agentHandle);

// Block until user toggles back (or Claude exits)
var result = tui.mux.switchToClaude();
// result.reason: 'toggle' | 'child-exit' | 'error'
// result.exitCode: number (if child-exit)

// Query state
tui.mux.isClaudeActive(); // false (since switchToClaude returned)

// Detach
tui.mux.detach();
```

---

## 12. Roadmap

### Completed

| Task | Description |
|------|-------------|
| **T239** | PTY module — `internal/builtin/pty/` |
| **T241** | PTY output parsing — `internal/builtin/claudemux/parser.go` |
| **T243** | Provider abstraction — Provider/AgentHandle/Registry + ClaudeCodeProvider |
| **T244** | BT orchestration templates — embedded in `internal/command/pr_split_script.js` |
| **T245** | PR splitting workflow — `osm pr-split` built-in command |
| **P008** | This document (architecture-claude-mux.md) |

### In progress / planned

| Task | Description |
|------|-------------|
| **P009** | Rename `orchestrator` package → `claudemux`, module `osm:orchestrator` → `osm:claudemux` |
| **P010** | ~~Rename `scripts/bt-templates/orchestrator.js` → `claude-mux.js`~~ **Done** → Consolidated into `osm pr-split` |
| **P013** | MCP feedback protocol — `reportProgress`, `reportResult`, `requestGuidance`, `reportError` |
| **P014** | Dynamic MCP config per Claude instance — startup sequencing, per-port config, cleanup |
| **P015** | Session isolation — `~/.osm/claude-sessions/<id>/`, independent state dirs |
| **P016** | Guard rails harness — rate-limit detection, permission monitoring, crash recovery |
| **P017** | Error recovery and cancellation — PTY/MCP error detection, restart strategy |
| **P018** | Concurrent instance management — instance pool, task queue, health tracking |
| **P019** | TUI multiplexing — meta-key switching, multi-panel view, output scrollback |
| **P020** | JS API for claude-mux building blocks |
| **P021** | Claude Code wrapper/parser JS native module — `createSession`, `sendTask`, `onResult` |
| **P022** | Safety validation — intent classification, scope assessment, risk scoring |
| **P023** | Ideal choice resolution — multi-candidate analysis, criteria weighting |
| **P024** | `osm claude-mux` command — wire pool + MCP + guard rails + session isolation |
| **P025** | Integration testing — real agent, env-gated, model selection, MCP verification |
