# PR Split Integration Testing Guide

This document explains how to run and interpret `osm pr-split` integration
tests. These tests validate the automated PR splitting pipeline—from diff
analysis through branch creation to equivalence verification.

## Test Tiers

### Tier 1: Unit Tests (no external dependencies)

```bash
make test
```

All `pr_split_*_test.go` files in `internal/command/` run as part of the
standard test suite. They use in-process JavaScript evaluation via Goja and
mock exec/git operations. No real git repos or AI agents are needed.

Key test files:
- `pr_split_analysis_test.go` — Diff analysis, language detection, import parsing
- `pr_split_grouping_test.go` — File grouping strategies (directory, extension, chunks)
- `pr_split_planning_test.go` — Split plan creation, save/load, dependency analysis
- `pr_split_execution_test.go` — Branch creation, cherry-pick, cleanup
- `pr_split_verification_test.go` — Verify splits, equivalence checks
- `pr_split_pipeline_test.go` — Conflict resolution, Claude executor, validation
- `pr_split_prompt_test.go` — Prompt rendering, heuristic fallback
- `pr_split_integration_test.go` — End-to-end with real git repos (local only)
- `pr_split_autosplit_recovery_test.go` — Recovery, resume, mock MCP pipeline

### Tier 2: Mock MCP Integration (no real AI)

```bash
make integration-test-prsplit-mcp
```

Runs `TestIntegration_AutoSplitMockMCP` which exercises the **full automated
pipeline** using a mock MCP server that provides pre-programmed classification
and plan responses. This test:

1. Creates a real git repository with feature branch
2. Spawns the JS engine with full pr-split script
3. Initialises mock MCP callback (no Claude binary)
4. Runs `automatedSplit()` through all 8 steps
5. Verifies branch creation, split execution, and equivalence

**This is the most important integration test.** It validates the entire
pipeline without requiring any AI infrastructure.

### Tier 3: Real Claude Integration (requires agent binary)

```bash
make integration-test-prsplit
```

Runs tests matching `TestIntegration_(.*Claude|AutoSplitComplex)` with a real
Claude Code binary. Requires:

- `claude` binary on PATH (Claude Code 2.x)
- Network access for AI model inference
- 15-minute timeout (real agent responses take time)

#### Configuring the Claude command

The `integration-test-prsplit` target accepts these variables:

```bash
# Default: uses 'claude' on PATH
make integration-test-prsplit

# Custom command and arguments
make integration-test-prsplit CLAUDE_COMMAND=ollama CLAUDE_ARGS="launch claude --model minimax-m2.5:cloud --"

# Custom model
make integration-test-prsplit INTEGRATION_MODEL=claude-sonnet-4-20250514
```

These are wired through TestMain flags:
- `-integration` — Enables integration tests (tests skip without this)
- `-claude-command=<cmd>` — Claude binary name/path
- `-claude-arg=<arg>` — Repeated for each argument to the Claude command
- `-integration-model=<model>` — Model name passed to Claude

#### Running a single real integration test

```bash
go test -race -v -count=1 -timeout=10m \
    ./internal/command/... \
    -run TestIntegration_AutoSplitWithClaude_Pipeline \
    -integration -claude-command=claude
```

**Note:** Custom flags (`-integration`, `-claude-command`) must appear AFTER
the package path (`./internal/command/...`), not before.

#### Expected behavior

The real Claude test:
1. Creates a git repo with 8 changed files across 4 directories
2. Spawns Claude Code via PTY with MCP callback configuration
3. Sends a classification prompt
4. Waits for Claude to call the `reportClassification` MCP tool
5. Generates a split plan from the classification
6. Executes splits, verifies, and checks equivalence

If Claude takes longer than the per-step timeout (default 60s for evalJS),
the test will fail with `evalJS timed out`. This is a timeout configuration
issue, not a code bug. Increase `-timeout` and consider model speed.

## Interpreting Failures

### `evalJS timed out after 60s`

The JavaScript evaluation didn't complete within the evalJS timeout. Common
causes:
- Real Claude is slow to respond (increase timeout)
- Event loop deadlock (check goroutine traces)
- Promise never resolves (check async chain)

### `FAIL . [setup failed]`

Go test couldn't compile the root package. This happens when custom flags
like `-integration` appear before the package path. Move them after
`./internal/command/...`.

### `branches created but equivalence failed`

The combined diff of all split branches doesn't match the original feature
branch diff. Check:
- File assignment: are all files covered exactly once?
- Cherry-pick failures: did any split silently drop changes?
- Binary files: equivalence uses tree hash comparison

### `Pipeline failed: ...` with resume instructions

The pipeline saved its state to `.pr-split-plan.json`. You can resume:

```bash
osm pr-split --resume
```

This skips classification and jumps to the execution phase using the saved
plan.

## Adding New Integration Tests

1. Add test functions to `internal/command/pr_split_*_test.go`
2. Use `loadPrSplitEngineWithEval(t, configOverrides)` to get an evalJS helper
3. For real git operations, use `setupTestPipeline(t, opts)` or create a
   temp repo with `t.TempDir()` + `gitInit()`
4. For mock MCP, see `TestIntegration_AutoSplitMockMCP` as the reference pattern
5. Tests matching `TestIntegration_*` are automatically included in integration targets

## Timeout Architecture

The pr-split pipeline has a **dual-layer timeout** design. Understanding
both layers is critical for debugging timeout failures.

### Layer 1: Go `evalJS` Timeout (Test Infrastructure)

The `loadPrSplitEngineWithEval` helper returns `evalJS` and `evalJSAsync`
closures. Each closure waits for the JS engine to signal completion via a
`done` channel. If the channel isn't signaled within the timeout, the Go
side aborts with `evalJS timed out after <duration>`.

- **Default:** 60 seconds (sufficient for unit tests and mock MCP)
- **Override:** Pass `"_evalTimeout": 10 * time.Minute` in `configOverrides`
- **Error signature:** `evalJS timed out after 60s` or `evalJSAsync timed out after 60s`

The `_evalTimeout` key is extracted from overrides before they're passed to
JS, so it never reaches the runtime. This is purely a Go-level test knob.

### Layer 2: JS `timeoutMs` (Pipeline Logic)

Inside the JavaScript engine, `prSplitConfig.timeoutMs` controls the
per-step MCP timeout — how long `mcpCallbackObj.waitFor(tool, timeoutMs)`
will block waiting for Claude to respond with tool results.

- **Default:** 120000ms (2 minutes) for unit tests
- **Typical real AI:** 300000ms (5 minutes) — `int64(5 * 60 * 1000)`
- **Error signature:** `MCP timed out waiting for <tool>` or similar

### Interaction Between Layers

```
Go test                     JS Engine                    Claude Process
  │                            │                              │
  ├─ evalJS(code) ────────────▶│                              │
  │  starts time.After(evalTO) │                              │
  │                            ├─ automatedSplit() ──────────▶│
  │                            │   waitFor(tool, timeoutMs)   │
  │                            │                    ◀─────────┤ tool result
  │                            │◀─ done channel               │
  │◀─ result ──────────────────┤                              │
```

If `evalTO < timeoutMs`, the Go side kills the evaluation before the JS
pipeline can even time out normally. This was the root cause of the initial
real-Claude test failures: Go's 60s default expired while Claude was still
thinking (typically 90-180s for large diffs).

**Rule:** For real AI tests, always set `_evalTimeout` to at least
`2 × timeoutMs` to give the JS layer room to handle its own timeouts
gracefully.

### EAGAIN Retry (Non-Blocking)

When writing to the PTY handle, `EAGAIN` / `EWOULDBLOCK` errors are retried
up to 3 times with 10ms async delays (`await new Promise(r => setTimeout(r, 10))`).
This yields to the event loop between retries, preventing stalls. Non-EAGAIN
errors fail immediately without retry.

## Make Targets Reference

| Target | Description | Requires |
|--------|-------------|----------|
| `make test` | All unit tests | Nothing |
| `make integration-test-prsplit-mcp` | Mock MCP pipeline | Nothing |
| `make integration-test-prsplit` | Real Claude pipeline | Claude binary |
| `make run-single-test TEST=X PKG=Y` | Run one test | Depends on test |
| `make clean-test-artifacts` | Remove `.pr-split-plan.json` | Nothing |
