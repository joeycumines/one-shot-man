# AI Orchestrator — Architecture

> **Status:** Active — Foundation implemented (T238–T245), advancing (T246–T255)  
> **Last updated:** 2026-02-17

## Contents

1. [Overview](#1-overview)
2. [Design Philosophy](#2-design-philosophy)
3. [Architecture](#3-architecture)
4. [Module Reference](#4-module-reference)
   - [osm:pty](#41-osmpty)
   - [osm:orchestrator](#42-osmorchestrator)
   - [BT Orchestration Templates](#43-bt-orchestration-templates)
   - [PR Splitting Workflow](#44-pr-splitting-workflow)
5. [Data Flow](#5-data-flow)
6. [Security Model](#6-security-model)
7. [Platform Support](#7-platform-support)
8. [Testing](#8-testing)
9. [Event Loop Migration Path](#9-event-loop-migration-path)
10. [Roadmap](#10-roadmap)
11. [Design History](#11-design-history)

---

## 1. Overview

The AI Orchestrator is a subsystem of `osm` for programmatic spawning, monitoring, and coordination of AI coding agents from within JavaScript workflows. It extends osm's behavior tree engine (`osm:bt`, `osm:pabt`), MCP server, and scripting infrastructure to automate multi-agent development workflows.

**What the orchestrator provides:**

- **PTY management** — Spawn processes in pseudo-terminals with full read/write/resize/signal control (`osm:pty`).
- **Output classification** — Parse agent terminal output into typed events: rate limits, permission prompts, model selection, SSO flows, tool invocations, errors (`osm:orchestrator`).
- **Provider abstraction** — Pluggable provider registry with a Claude Code implementation backed by PTY (`osm:orchestrator`).
- **Workflow scripts** — Composable JavaScript workflows using BT templates for multi-step agent orchestration.
- **PR splitting** — Automated splitting of large diffs into linear branch series with equivalence verification.

**What the orchestrator does not do:**

- Replace osm's clipboard-first philosophy. It is opt-in for power users.
- Call any AI API directly. Communication happens through PTY I/O.
- Manage API keys or credentials. All secrets remain in the parent environment.

---

## 2. Design Philosophy

**Go for infrastructure and safety. JavaScript for workflow logic.**

Safety-critical paths — PTY lifecycle, signal handling, permission prompt rejection, output parsing — are implemented in Go with type safety and comprehensive tests. Workflow orchestration, prompt construction, and user customization remain in JavaScript, leveraging the existing BT engine.

This matches osm's existing pattern: Go provides the native modules; JavaScript composes them into workflows.

| Concern | Layer | Rationale |
|---------|-------|-----------|
| PTY spawn, resize, signal, close | Go (`internal/builtin/pty`) | OS-specific, safety-critical, requires `creack/pty` |
| Output pattern matching | Go (`internal/builtin/orchestrator`) | Compiled regex, table-driven, extensible from JS |
| Permission prompt rejection | Go (default-reject policy) | Too critical for dynamic JS — must never accidentally approve |
| Provider abstraction | Go (`Provider`, `AgentHandle` interfaces) | Type-safe contract for multiple backends |
| Agent lifecycle monitoring | Go (goroutine + channel) | Process exit detection requires OS-level wait |
| BT tree composition | JS (`scripts/bt-templates/orchestrator.js`) | User-modifiable, leverages existing `osm:bt` |
| Workflow scripts | JS (`scripts/orchestrate-*.js`) | User-facing, goal-discoverable, require()-able |
| Prompt construction | JS (existing `context` / `output` globals) | Unchanged — same API as all osm scripts |

---

## 3. Architecture

### Component Diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                        JavaScript Layer                              │
│                                                                      │
│  Workflow Scripts (user-modifiable, goal-discoverable)               │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  scripts/orchestrate-pr-split.js     — PR splitting workflow  │  │
│  │  goals/orchestrate-pr-split.json     — goal discovery config  │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  BT Orchestration Templates (require-able, composable)              │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  scripts/bt-templates/orchestrator.js                         │  │
│  │   ├── 7 leaf factories (spawn, prompt, wait, verify, ...)     │  │
│  │   ├── 2 workflow composers (spawnAndPrompt, verifyAndCommit)  │  │
│  │   └── PA-BT action library (7 actions with preconditions)     │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  Uses: osm:bt, osm:pabt, osm:exec, osm:pty, osm:orchestrator      │
└──────────────────────────────────────────────────────────────────────┘
                              │
                     require('osm:pty')
                     require('osm:orchestrator')
                              │
┌──────────────────────────────────────────────────────────────────────┐
│                     Go Layer (Safety-Critical)                       │
│                                                                      │
│  internal/builtin/pty/                                              │
│  ├── pty.go            Process, SpawnConfig, Read/Write/Signal/Wait │
│  ├── pty_unix.go       creack/pty (macOS + Linux)                   │
│  ├── pty_windows.go    Stub (ConPTY planned)                        │
│  └── module.go         osm:pty Goja bridge                          │
│                                                                      │
│  internal/builtin/orchestrator/                                      │
│  ├── parser.go         OutputParser (20+ Claude Code patterns)      │
│  ├── provider.go       Provider / AgentHandle / Registry interfaces │
│  ├── claude_code.go    ClaudeCodeProvider (PTY-backed)              │
│  └── module.go         osm:orchestrator Goja bridge                 │
│                                                                      │
│  Existing modules (unchanged)                                        │
│  ├── internal/builtin/bt/     — BT bridge (thread-safe JS↔Go)      │
│  ├── internal/builtin/pabt/   — PA-BT planning engine              │
│  ├── internal/builtin/exec/   — Shell command execution             │
│  ├── internal/session/        — Session ID & locking                │
│  └── internal/command/mcp.go  — MCP server                         │
└──────────────────────────────────────────────────────────────────────┘
```

### Module Dependency Graph

```
osm:orchestrator ──► osm:pty (claude_code.go imports pty.Spawn)
       │
       ▼
  Provider / AgentHandle interfaces
       │
       ▼
  bt-templates/orchestrator.js ──► osm:bt, osm:orchestrator, osm:exec
       │
       ▼
  orchestrate-pr-split.js ──► osm:bt, osm:exec
       │
       ▼
  goals/orchestrate-pr-split.json (discovery)
```

---

## 4. Module Reference

### 4.1. `osm:pty`

**Package:** `internal/builtin/pty`  
**Status:** Implemented (T239). Unix complete, Windows stub.

Spawns processes in pseudo-terminals with full bidirectional I/O, window resizing, and signal delivery.

#### Go API

```go
// SpawnConfig configures a PTY session.
type SpawnConfig struct {
    Command string            // Executable path or name (required)
    Args    []string          // Command arguments
    Env     map[string]string // Additional env vars (merged with os.Environ)
    Dir     string            // Working directory (default: caller's CWD)
    Rows    uint16            // Terminal rows (default: 24)
    Cols    uint16            // Terminal columns (default: 80)
    Term    string            // TERM env var (default: "xterm-256color")
}

// Process represents a running process attached to a PTY.
// All methods are goroutine-safe.
type Process struct { /* ... */ }

func Spawn(ctx context.Context, cfg SpawnConfig) (*Process, error)
func (p *Process) Write(data string) error
func (p *Process) Read() (string, error)      // Up to 4096 bytes, non-blocking
func (p *Process) Resize(rows, cols uint16) error
func (p *Process) Signal(sig string) error     // "SIGINT", "SIGTERM", etc.
func (p *Process) Wait() (exitCode int, err error)
func (p *Process) IsAlive() bool
func (p *Process) Pid() int
func (p *Process) Close() error               // SIGTERM → 5s wait → SIGKILL
```

#### JavaScript API

```javascript
var pty = require('osm:pty');

var proc = pty.spawn('bash', ['-l'], {
    rows: 24, cols: 80,
    dir: '/path/to/project',
    env: { TERM: 'xterm-256color' }
});

proc.write('echo hello\n');
var output = proc.read();          // "" on EOF, up to 4096 bytes
proc.resize(48, 120);
proc.signal('SIGINT');

var result = proc.wait();          // { code: 0, error: null }
proc.close();                      // Idempotent cleanup
```

#### Platform Implementation

| Platform | Backend | Status |
|----------|---------|--------|
| macOS / Linux | `creack/pty` via `pty.StartWithSize` | ✅ Implemented |
| Windows | `ErrNotSupported` (ConPTY planned) | ⬜ Stub |

Process lifecycle: a background goroutine calls `cmd.Wait()` and closes the `done` channel, allowing concurrent `Wait()` and `IsAlive()` callers to observe exit.

### 4.2. `osm:orchestrator`

**Package:** `internal/builtin/orchestrator`  
**Status:** Implemented (T241 parser, T243 provider).

Provides output classification, provider abstraction, and agent lifecycle management.

#### Output Parser

Classifies raw terminal output lines into typed events via compiled regex patterns.

**Event types:**

| Constant | Value | Description | Example Match |
|----------|-------|-------------|---------------|
| `EVENT_TEXT` | 0 | Normal text (no pattern matched) | — |
| `EVENT_RATE_LIMIT` | 1 | Rate limit / 429 / backoff | `"try again in 30 seconds"` |
| `EVENT_PERMISSION` | 2 | Permission prompt (Y/N) | `"Allow? [y/N]"` |
| `EVENT_MODEL_SELECT` | 3 | Model selection menu | `"Select a model"` |
| `EVENT_SSO_LOGIN` | 4 | SSO / OAuth flow | `"Opening your browser"` |
| `EVENT_COMPLETION` | 5 | Task completed | `"Task completed"` |
| `EVENT_TOOL_USE` | 6 | MCP tool invocation | `"Calling tool: readFile"` |
| `EVENT_ERROR` | 7 | Error message | `"Error: file not found"` |
| `EVENT_THINKING` | 8 | Thinking indicator | `"Thinking..."` |

**Built-in patterns** (20+): Rate limit detection (`try again in N`, `rate limit`, `too many requests`, `429`, `quota exceeded`), permission prompts (`Allow? [y/N]`, `do you want to allow`), model selection, SSO/OAuth flows (`opening browser`, `visit https://`), completion signals, tool use parsing, error prefixes, thinking indicators.

```javascript
var orc = require('osm:orchestrator');

var parser = orc.newParser();
var event = parser.parse('Try again in 30 seconds');
// event.type === orc.EVENT_RATE_LIMIT
// event.fields.retryAfter === "30"
// event.pattern === "rate-limit-try-again"

// Add custom patterns
parser.addPattern('my-done', 'BUILD SUCCESSFUL', orc.EVENT_COMPLETION);
```

**Go API:**

```go
func NewParser() *Parser
func (p *Parser) Parse(line string) OutputEvent
func (p *Parser) AddPattern(name, pattern string, eventType EventType) error
```

#### Provider Abstraction

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
    Receive() (string, error)    // Returns ("", io.EOF) on exit
    Close() error
    IsAlive() bool
    Wait() (int, error)
}

// SpawnOpts configures agent spawning.
type SpawnOpts struct {
    Model string
    Env   map[string]string
    Dir   string
    Rows  uint16
    Cols  uint16
    Args  []string
}
```

**Registry** manages named providers. Thread-safe via `sync.RWMutex`.

```go
func NewRegistry() *Registry
func (r *Registry) Register(p Provider) error    // ErrProviderExists if duplicate
func (r *Registry) Get(name string) (Provider, error)
func (r *Registry) List() []string               // Sorted names
func (r *Registry) Spawn(ctx context.Context, name string, opts SpawnOpts) (AgentHandle, error)
```

**Claude Code provider** (`ClaudeCodeProvider`):

- Name: `"claude-code"`
- Capabilities: `{MCP: true, Streaming: true, MultiTurn: true}`
- Spawn: Creates a PTY session running `claude` (configurable) with optional `--model` flag
- AgentHandle backed by `pty.Process` — `Send()` writes to PTY, `Receive()` reads from PTY

```javascript
var orc = require('osm:orchestrator');

var registry = orc.newRegistry();
var claude = orc.claudeCode({ command: '/usr/local/bin/claude' });
registry.register(claude);

var agent = registry.spawn('claude-code', {
    model: 'claude-sonnet-4-20250514',
    dir: '/path/to/project'
});

agent.send('Fix the failing test\n');
var output = agent.receive();   // Read agent output
agent.close();                  // Graceful shutdown
```

### 4.3. BT Orchestration Templates

**File:** `scripts/bt-templates/orchestrator.js`  
**Status:** Implemented (T244).  
**Dependencies:** `osm:bt`, `osm:orchestrator`, `osm:exec`

Reusable behavior tree building blocks for AI orchestration workflows. All leaf factories use `bt.createBlockingLeafNode` for sequential execution semantics.

#### Leaf Node Factories

Each factory returns a `bt.Node` and communicates via a shared `bt.Blackboard`.

| Factory | Blackboard Reads | Blackboard Writes | Purpose |
|---------|-----------------|-------------------|---------|
| `spawnClaude(bb, registry, providerName?, spawnOpts?)` | — | `agent`, `parser`, `agentSpawned` | Spawn agent via provider registry |
| `sendPrompt(bb, prompt)` | `agent` | `promptSent` | Write prompt to agent stdin |
| `waitForResponse(bb, opts?)` | `agent`, `parser` | `response`, `responseReceived`, `rateLimited` | Read/parse output until completion |
| `verifyOutput(bb, command)` | — | `verifyCode`, `verified` | Run shell command, check exit |
| `runTests(bb, command?)` | — | `testCode`, `testsPassed` | Run test command |
| `commitChanges(bb, message)` | — | `commitOutput`, `committed` | `git add -A && git commit` |
| `splitBranch(bb, branchName)` | — | `currentBranch`, `branchCreated` | `git checkout -b` |

**Security:** `waitForResponse` automatically sends `"n\n"` to any detected permission prompt (`EVENT_PERMISSION`), rejecting it and setting `permissionRejected` on the blackboard. This is the JS-level safety net; the Go-level parser classification is the primary defense.

#### Workflow Composers

```javascript
// Sequence: spawn → send prompt → wait for response
templates.spawnAndPrompt(bb, registry, {
    provider: 'claude-code',
    prompt: 'Fix the bug in foo.go',
    spawnOpts: { dir: '/project' }
});

// Sequence: run tests → [verify] → commit
templates.verifyAndCommit(bb, {
    testCommand: 'make test',
    verifyCommand: 'git diff --check',
    message: 'Fix: resolve test failure'
});
```

#### PA-BT Action Library

`createPlanningActions(pabt, bb, registry, config)` returns 7 named actions with preconditions and effects for automatic plan synthesis via backchaining:

```
Goal: committed=true
  └─ CommitChanges (needs testsPassed=true)
       └─ RunTests (needs responseReceived=true)
            └─ WaitForResponse (needs promptSent=true)
                 └─ SendPrompt (needs agentSpawned=true)
                      └─ SpawnClaude (no preconditions)
```

The PA-BT planner synthesizes this chain automatically. If `RunTests` fails, the planner's PPA structure re-evaluates and can re-run the prompt→test cycle.

### 4.4. PR Splitting Workflow

**Script:** `scripts/orchestrate-pr-split.js`  
**Goal:** `goals/orchestrate-pr-split.json`  
**Status:** Implemented (T245).  
**Dependencies:** `osm:bt`, `osm:exec`

Splits a large diff into a linear series of stacked, independently-reviewable branches.

#### Architecture

The workflow has four functional layers:

```
Analysis ──► Grouping ──► Planning ──► Execution ──► Verification
   │             │            │             │              │
analyzeDiff   groupBy*    createSplit   executeSplit   verifyEquivalence
analyzeDiff     (4         Plan          (linear         (tree hash
  Stats       strategies)               stacking)       comparison)
```

#### Analysis

`analyzeDiff(config)` and `analyzeDiffStats(config)` detect changed files between branches using `git merge-base` for accuracy (handles diverged branches correctly).

```javascript
var analysis = prSplit.analyzeDiff({ baseBranch: 'main', dir: '.' });
// { files: ['pkg/a.go', 'cmd/b.go'], error: null,
//   baseBranch: 'main', currentBranch: 'feature' }

var stats = prSplit.analyzeDiffStats({ baseBranch: 'main' });
// { files: [{name: 'pkg/a.go', additions: 42, deletions: 7}, ...] }
```

#### Grouping Strategies

| Strategy | Function | Use Case |
|----------|----------|----------|
| **Directory** | `groupByDirectory(files, depth)` | Group by top-level package (`depth=1`) or sub-package (`depth=2`) |
| **Extension** | `groupByExtension(files)` | Separate `.go` from `.js` from `.md` |
| **Pattern** | `groupByPattern(files, patterns)` | Named regex patterns, e.g., `{tests: /test/, docs: /\.md$/}` |
| **Chunks** | `groupByChunks(files, maxPerGroup)` | Fixed-size groups for uniform PR sizes |

All return `{ groupName: [file1, file2, ...] }`.

#### Planning & Validation

`createSplitPlan(groups, config)` produces a plan with ordered splits, each containing a branch name, file list, and commit message. Groups are sorted alphabetically.

`validatePlan(plan)` checks: at least one split, no empty splits, no duplicate files across splits, all splits named.

#### Execution: Linear Branch Stacking

`executeSplit(plan)` creates branches where each builds on the previous:

```
main → split/01-cmd → split/02-docs → split/03-pkg
  │         │               │               │
  └─────────┘               │               │
  base for 01          base for 02      base for 03
```

For each split:
1. Check out the current base branch
2. Create a new branch (`git checkout -b split/NN-name`)
3. Check out files from the source branch (`git checkout source -- file`)
4. Stage and commit
5. The new branch becomes the base for the next split

The original branch is restored after all splits are created.

#### Verification

- **`verifySplit(branch, config)`** — Check out a branch and run a command (e.g., `make test`).
- **`verifySplits(plan)`** — Run verification on all split branches.
- **`verifyEquivalence(plan)`** — Compare tree hashes: the last split branch should have an identical git tree to the source branch. If `splitTree === sourceTree`, no content was lost or duplicated.
- **`cleanupBranches(plan)`** — Delete all split branches.

#### BT Integration

Six node factories wrap each layer for behavior tree composition:

```javascript
var bb = new bt.Blackboard();
var tree = prSplit.createWorkflowTree(bb, {
    baseBranch: 'main',
    groupStrategy: 'directory',
    branchPrefix: 'split/'
});
bt.tick(tree);  // analyze → group → plan → split → equivalence
```

#### Goal Definition

`goals/orchestrate-pr-split.json` enables `osm goal pr-split` with:
- TUI prompt-building workflow
- Custom commands: `set-base`, `set-group`, `set-max`
- Hot snippets: `split-review`, `split-reorder`
- State variables: `baseBranch`, `groupStrategy`, `maxFilesPerSplit`

---

## 5. Data Flow

### Agent Orchestration (BT Templates)

```
User runs: osm script workflow.js
  │
  ▼
JS: Build BT tree using templates
  │  templates.spawnAndPrompt(bb, registry, config)
  │
  ▼
JS→Go: registry.spawn('claude-code', opts)
  │  → ClaudeCodeProvider.Spawn()
  │  → pty.Spawn(ctx, cfg)                          ← PTY allocation
  │  → background goroutine monitors process exit
  │
  ▼
JS→Go: agent.send(prompt)
  │  → pty.Process.Write()                           ← Write to PTY fd
  │
  ▼
JS→Go: agent.receive()
  │  → pty.Process.Read()                            ← Read from PTY fd
  │  → parser.Parse(line)                            ← Classify output
  │
  ├─ EVENT_RATE_LIMIT → return bt.running (re-tick after delay)
  ├─ EVENT_PERMISSION → agent.send('n\n'), set permissionRejected
  ├─ EVENT_COMPLETION → set response, return bt.success
  └─ EVENT_TEXT       → accumulate output
  │
  ▼
JS: Verify (exec tests, git diff --check)
  │
  ▼
JS: Commit (git add -A, git commit)
```

### PR Splitting (Direct Git Workflow)

```
User runs: osm goal pr-split  (or osm script orchestrate-pr-split.js)
  │
  ▼
analyzeDiff({ baseBranch: 'main' })
  │  → git merge-base main feature
  │  → git diff --name-only <merge-base> feature
  │
  ▼
groupByDirectory(files, 1)
  │  → { 'cmd': [...], 'pkg': [...], 'docs': [...] }
  │
  ▼
createSplitPlan(groups, config)
  │  → validatePlan(plan) ✓
  │
  ▼
executeSplit(plan)
  │  → for each split:
  │       git checkout <base>
  │       git checkout -b split/NN-name
  │       git checkout source -- file1 file2 ...
  │       git add -A && git commit -m "..."
  │       base = split/NN-name (stacking)
  │
  ▼
verifyEquivalence(plan)
  │  → git rev-parse split/03-docs^{tree}
  │  → git rev-parse feature^{tree}
  │  → assert: splitTree === sourceTree
```

---

## 6. Security Model

### Permission Prompt Rejection

**This is the most safety-critical code in the orchestrator.**

When an AI agent requests file deletion, network access, or code execution, it produces a permission prompt on stdout. The orchestrator must detect and reject these.

**Defense in depth:**

1. **Go parser (primary):** `Parser.Parse()` classifies lines containing permission patterns as `EVENT_PERMISSION`. Three built-in patterns match `Allow? [y/N]`, `do you want to allow/proceed/continue`, and `permission required/needed/denied`.

2. **JS templates (secondary):** `waitForResponse` sends `"n\n"` when it encounters `EVENT_PERMISSION`. This rejects the prompt.

3. **Default-reject policy:** If a permission prompt matches no pattern, it is classified as `EVENT_TEXT` and the agent receives no `"y"` response. The absence of a response does not equal approval — most agents treat silence as rejection or timeout.

Custom patterns can be added via `parser.addPattern()` for provider-specific prompt formats.

### PTY Isolation

- Each agent runs in its own PTY with an independent file descriptor.
- PTY output is read in Go (`Process.Read()`) and passed to the parser before reaching JS.
- `Close()` sends SIGTERM, waits 5 seconds, then SIGKILL. Resource leaks are prevented by explicit cleanup.

### Credential Handling

- Agents inherit environment from the parent process. No credentials are stored.
- The `Env` field in `SpawnOpts` adds variables but never removes inherited ones.
- MCP session registration (when implemented) will not transmit credentials.

---

## 7. Platform Support

| Component | macOS | Linux | Windows |
|-----------|-------|-------|---------|
| `osm:pty` spawn/read/write/resize/signal | ✅ `creack/pty` | ✅ `creack/pty` | ⬜ `ErrNotSupported` |
| `osm:orchestrator` parser | ✅ | ✅ | ✅ |
| `osm:orchestrator` provider/registry | ✅ | ✅ | ✅ |
| `osm:orchestrator` Claude Code provider | ✅ | ✅ | ⬜ (needs PTY) |
| BT templates | ✅ | ✅ | ✅ (except spawn) |
| PR splitting workflow | ✅ | ✅ | ✅ (pure `osm:exec` git) |
| Goal discovery | ✅ | ✅ | ✅ |

**Windows PTY:** ConPTY support via `golang.org/x/sys/windows` is planned. The `Process` struct and `processHandle` interface are designed for platform-specific backends. Windows will require `pty_windows.go` to implement `Spawn()` using `CreatePseudoConsole`.

**Build tags:** PTY uses `//go:build !windows` / `//go:build windows` for platform separation. No conditional compilation elsewhere — the orchestrator parser and provider abstraction are pure Go.

---

## 8. Testing

### Test Files

| File | Tests | Coverage |
|------|-------|----------|
| `internal/builtin/orchestrator/parser_test.go` | Parser patterns, custom patterns, event type names | Core parser logic |
| `internal/builtin/orchestrator/provider_test.go` | Registry CRUD, concurrent access, error cases | Provider abstraction |
| `internal/builtin/orchestrator/templates_test.go` | BT template loading, leaf execution, PA-BT actions | T244 templates |
| `internal/builtin/orchestrator/pr_split_test.go` | Grouping, validation, analysis, execution, equivalence, BT workflow | T245 workflow |
| `internal/builtin/pty/pty_test.go` | PTY spawn, read/write, resize, signal, close | PTY module |

### Testing Strategy

**Layer 1 — Go unit tests (safety-critical paths):**
- Parser: Table-driven tests mapping raw output lines to expected `EventType` values.
- Provider: Registry operations, concurrent registration, error wrapping.
- PTY: Spawn `/bin/echo` and `/bin/cat`, test read/write/close lifecycle.

**Layer 2 — JS integration tests (workflow logic):**
- BT templates: Load `orchestrator.js` in a Goja runtime, verify exports, execute nodes with mocked dependencies.
- PR splitting: Create temporary git repos, execute full split workflows, verify tree hash equivalence.
- Goal definition: Validate JSON structure, command handlers, state variables.

**Layer 3 — Integration tests (real agents, T247):**
- Gated by `OSM_INTEGRATION_PROVIDER` environment variable.
- Disabled by default in CI — requires actual Claude Code binary.
- Tests spawn real agents, send prompts, parse output.

**Test helpers:**
- `prSplitTestEnv(t)` — Creates a Goja runtime with `osm:bt`, `osm:exec`, and `osm:orchestrator` registered, loads `orchestrate-pr-split.js` and returns the exports object.
- `initTestGitRepo(t, dir)` — Creates a git repo with `git init -b main` (portable across platforms), configures user identity, and makes an initial commit.

---

## 9. Event Loop Migration Path

> **Status: COMPLETE.** Both the event loop migration (T011) and goja-grpc integration (T012) are done. This section is retained for historical context.

### Completed: Event Loop Migration (T011)

Replaced `dop251/goja_nodejs/eventloop` with `github.com/joeycumines/go-eventloop` + `goja-eventloop` adapter. The new event loop supports Promises, async/await, AbortController, TextEncoder/Decoder, URL, and process.nextTick.

### Completed: goja-grpc Integration (T012)

Replaced the synchronous `osm:grpc` module (raw `google.golang.org/grpc` with `conn.invoke()`) with a thin wrapper around `joeycumines/goja-grpc`. The new module provides:

1. **Promise-based RPC** — All gRPC calls return Promises (unary, server-streaming, client-streaming, bidirectional).
2. **In-process channel** — Uses `go-inprocgrpc` for zero-network-overhead internal communication.
3. **Separate protobuf module** — `osm:protobuf` (via `goja-protobuf`) handles `FileDescriptorSet` loading.
4. **Full API** — `createClient`, `createServer`, `dial`, `status`, `metadata`, `enableReflection`, `createReflectionClient`.

### Remaining

Post-migration, the orchestrator gains:
- Streaming output parsing (event-driven instead of polling `Read()`)
- Promise-based agent communication
- `goja-grpc` for MacosUseSDK streaming integration (observation streams, real-time UI monitoring)

---

## 10. Roadmap

### Completed

| Task | Description | Deliverable |
|------|-------------|-------------|
| **T238** | Architecture document | This document |
| **T239** | PTY module | `internal/builtin/pty/` — `osm:pty` |
| **T240** | MCP bidirectional tools | Extended `internal/command/mcp.go` |
| **T241** | PTY output parsing | `internal/builtin/orchestrator/parser.go` |
| **T242** | Configuration management | Extended `internal/config/` |
| **T243** | Provider abstraction | Provider/AgentHandle/Registry + ClaudeCodeProvider |
| **T244** | BT orchestration templates | `scripts/bt-templates/orchestrator.js` |
| **T245** | PR splitting workflow | `scripts/orchestrate-pr-split.js` + goal definition |

### Planned

| Task | Description | Dependencies |
|------|-------------|-------------|
| **T246** | TUI multiplexing — meta-key switching, terminal state save/restore, visual status bar | T239 |
| **T247** | Integration testing — TestMain with real Claude Code, env-gated | T243 |
| **T248** | Error recovery — crash detection, rate limit backoff, hang timeout | T243 |
| **T249** | Session isolation — per-agent state directory, namespace prefix | Existing session |
| **T250** | User building blocks — documentation, module exposure | T239, T243, T244 |
| **T251** | Claude Code multiplexer — concurrent agent management | T243, T249 |
| **T252** | Ollama provider — local LLM backend | T243 |
| **T253** | Production parser — Claude Code output corpus | T241 |
| **T254** | Safety validation — comprehensive permission prompt coverage | T241, T248 |
| **T255** | Ideal choice resolution — intelligent provider selection | T251, T252 |

### Dependency Graph

```
T246 (TUI mux) ◄── T239
T247 (Integration tests) ◄── T243
T248 (Error recovery) ◄── T243
T249 (Session isolation) ◄── existing session
T250 (Building blocks) ◄── T239, T243, T244
T251 (Multiplexer) ◄── T243, T249
T252 (Ollama) ◄── T243
T253 (Production parser) ◄── T241
T254 (Safety validation) ◄── T241, T248
T255 (Ideal choice) ◄── T251, T252
```

---

## 11. Design History

### Approach Selection (T238)

Three architectures were evaluated:

| Approach | Name | Verdict |
|----------|------|---------|
| A | Script-First (all orchestration in JS, Go adds only `osm:pty`) | Rejected: safety-critical paths in dynamic JS |
| B | Module-First (full Go module system with interfaces, DI, 8+ packages) | Rejected: 2000+ LOC of interface ceremony, contradicts osm's scripting-first DNA |
| **C** | **Pragmatic Balance (Go for safety, JS for logic)** | **Selected** |

Approach C was selected because:
1. It matches osm's existing architecture — Go provides native modules, JS composes them.
2. Safety-critical paths (PTY, permission rejection) are in compiled Go with type safety.
3. Workflow logic remains in user-modifiable JS, leveraging the existing BT engine.
4. Minimal new Go code (~800 LOC for two modules) — no interface ceremony.
5. BT templates and workflow scripts are goal-discoverable and `require()`-able.

The full three-approach analysis was part of the original document. It served its purpose during the design phase and has been archived.
