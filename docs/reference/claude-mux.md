# Claude-Mux Reference

Multi-instance Claude Code orchestration via `osm claude-mux`.

## Command

```
osm claude-mux <subcommand> [options]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-pool-size` | `4` | Maximum number of concurrent Claude instances |

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `status` | Show configuration and system health |
| `start` | Initialize orchestration infrastructure |
| `stop` | Shut down managed instances |
| `submit` | Submit a task for processing |

## Building Blocks

The claude-mux system is composed of these Go types, all accessible from JavaScript via `require('osm:claudemux')`:

### Parser (`parser.go`)

Classifies raw PTY output lines into typed events. Extensible via custom patterns.

- Event types: `Text`, `RateLimit`, `Permission`, `ModelSelect`, `SSOLogin`, `Completion`, `ToolUse`, `Error`, `Thinking`
- Model navigation: `ParseModelMenu(lines)`, `NavigateToModel(menu, target)` for automated model selection

### Guard (`guard.go`)

Event-driven PTY output monitors. Fail-closed (deny by default).

| Monitor | Description | Default |
|---------|-------------|---------|
| Rate limit | Exponential backoff on rate-limit events | Enabled, 2× multiplier, 120s cap |
| Permission | Policy enforcement for permission prompts | Deny (fail-closed) |
| Crash | Max restart tracking | 3 restarts before escalation |
| Output timeout | Idle output detection | 300s (5 min) |

Actions: `None`, `Pause`, `Reject`, `Restart`, `Escalate`, `Timeout`

### MCPGuard (`mcp_guard.go`)

MCP tool call monitors.

| Monitor | Description | Default |
|---------|-------------|---------|
| Frequency limit | Sliding window call counting | 50 calls / 10s |
| Repeat detection | Consecutive identical tool+args | 5 max repeats |
| No-call timeout | Idle agent detection | 10 min |
| Tool allowlist | Tool name validation | Disabled |

### Supervisor (`recovery.go`)

Error recovery state machine: `Idle → Running → Recovering → Draining → Stopped`.

| Error Class | Default Action | Escalation |
|-------------|---------------|------------|
| PTY-EOF | Restart | Escalate after max retries |
| PTY-Crash | ForceKill → Restart | Escalate |
| PTY-Error | Retry → Restart | Escalate |
| MCP-Timeout | Retry → Restart | Escalate |
| MCP-Malformed | Retry once | Escalate |

Config: `MaxRetries=3`, `MaxForceKills=1`, `RetryDelay=5s`, `ShutdownTimeout=30s`, `ForceKillTimeout=10s`

### Pool (`pool.go`)

Concurrent instance management with acquire/release. Round-robin dispatch, sync.Cond blocking.

State machine: `Idle → Running → Draining → Closed`

Config: `MaxSize=4`

### Panel (`panel.go`)

TUI multi-instance display. Alt+1..9 pane switching, PgUp/PgDown scrollback.

Config: `MaxPanes=9`, `ScrollbackSize=10000`

Health indicators: `●` running, `✖` error, `○` idle, `■` stopped

### ManagedSession (`session_mgr.go`)

Unified monitoring pipeline composing Parser + Guard + MCPGuard + Supervisor.

States: `Idle → Active → Paused/Failed → Closed`

Key methods:
- `ProcessLine(line, now)` → parse + guard check → `LineResult{Event, GuardEvent, Action}`
- `ProcessCrash(exitCode, now)` → guard + supervisor → recovery decision
- `ProcessToolCall(call)` → MCPGuard check → tool call result
- `Shutdown()` → graceful drain → `RecoveryDecision`

### Safety (`safety.go`)

Intent classification (Unknown/ReadOnly/Code/Destructive/Network/Credential), scope assessment (File/Repo/Infra), risk scoring (0.0–1.0), policy enforcement (Allow/Warn/Confirm/Block).

### ChoiceResolver (`choice.go`)

Multi-criteria decision analysis. Default criteria: complexity (0.25), risk (0.25), maintainability (0.25), performance (0.25). Produces ranked recommendations with justification.

### InstanceRegistry (`instance.go`)

Isolated state directories per Claude instance. Thread-safe via sync.Map. Each instance gets `<baseDir>/<id>/` with `state.json` tracking.

## Configuration

Configuration keys in `osm.conf`:

| Key | Default | Description |
|-----|---------|-------------|
| `claude-mux.provider` | `claude-code` | Claude provider name |
| `claude-mux.env-inherit` | `true` | Inherit parent environment |
| `claude-mux.permission-policy` | `reject` | How to handle permission prompts |
| `claude-mux.rate-limit-backoff-sec` | `30` | Initial backoff seconds |
| `claude-mux.max-agents` | `4` | Maximum concurrent agents |
| `claude-mux.pty-rows` | `24` | PTY row count |
| `claude-mux.pty-cols` | `80` | PTY column count |

## Shell Completion

Shell completion is available for all subcommands:

```bash
# Bash
source <(osm completion bash)

# Zsh
source <(osm completion zsh)

# Fish
osm completion fish > ~/.config/fish/completions/osm.fish

# PowerShell
osm completion powershell | Invoke-Expression
```

## See Also

- [Architecture](../architecture-claude-mux.md) — Full two-channel architecture design
- [Scripting](../scripting.md#osmclaudemux-claude-mux-orchestration) — JavaScript API reference
- [Command Reference](command.md#osm-claude-mux) — CLI usage
