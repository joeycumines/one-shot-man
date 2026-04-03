# PR-Split Integration Testing

This document describes the test infrastructure, patterns, and runner
configuration for the `osm pr-split` feature.

## Test taxonomy

Tests are organized into three tiers, each with a dedicated engine loader:

| Tier | Loader | Scope | Typical duration |
|:-----|:-------|:------|:-----------------|
| **Chunk unit** | `prsplittest.NewChunkEngine(t, overrides, chunkNames...)` | Single chunk in isolation | <100 ms |
| **Integration** | `loadPrSplitEngineWithEval(t, overrides)` | All 30 chunks, mock external deps | 1–5 s |
| **Binary E2E** | `buildOSMBinary` + PTY harness | Full compiled binary in a real terminal | 7–30 s |

### Chunk unit tests

Load only the chunk under test plus its dependencies:

```go
func TestChunk00_RuntimeObject(t *testing.T) {
    t.Parallel()
    evalJS := prsplittest.NewChunkEngine(t, nil, "00_core")
    val, err := evalJS(`typeof globalThis.prSplit.runtime`)
    if err != nil { t.Fatal(err) }
    if val != "object" { t.Fatalf("got %v", val) }
}
```

`prsplittest.NewChunkEngine` creates a goja engine, loads only the listed
chunks, and injects `t.TempDir()` as the working directory. This enables
fast, focused tests that exercise individual chunk functions without loading
the full TUI or pipeline stack.

### Integration tests

Load the complete engine and exercise cross-chunk workflows:

```go
func TestChunk10d_CancellationBeforeFirstStep(t *testing.T) {
    skipSlow(t)
    t.Parallel()
    _, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)
    evalJS(`globalThis.prSplit.isCancelled = function() { return true; }`)
    raw, err := evalJS(`JSON.stringify(await globalThis.prSplit.automatedSplit({disableTUI: true}))`)
    // ... parse and assert JSON result
}
```

`loadPrSplitEngineWithEval` loads all 30 chunks plus the chunk-compat shim.
It returns four values: a thread-safe output buffer, a command-dispatch
function, a synchronous `evalJS`, and an async-aware `evalJSAsync`. Mock
JavaScript dependencies by reassigning globals before calling the function
under test.

### Binary E2E tests

Build the real `osm` binary and drive it through a pseudo-terminal:

```go
//go:build unix

func TestBinaryE2E_FullFlowToExecution(t *testing.T) {
    skipSlow(t)
    repoDir := setupBinaryTestRepo(t)
    ptmx, buf, cleanup := startPTYBinary(t, repoDir, "-verify=true")
    defer cleanup()
    waitForPTYOutput(t, buf, "PR Split Wizard", 10*time.Second)
    // send keystrokes, assert screen output...
}
```

E2E tests are guarded with `//go:build unix` since they require PTY support.
The binary is compiled once per test run (`sync.Once`) and cached.

## Running tests

```bash
# Fast: skip all slow/E2E tests (~100s)
make test-prsplit-fast

# Full: include slow integration tests (~600s)
make test-prsplit-all

# E2E only
make test-prsplit-e2e

# Single test by name
make test-run T=TestChunk10d_CancellationBeforeFirstStep
```

The `-short` flag is used to skip slow tests. The `skipSlow(t)` helper (in
`main_test.go`) calls `t.Skip()` when `testing.Short()` is true. All tests
that build the binary, spawn PTY processes, or run multi-step pipelines use
`skipSlow(t)`.

## Git repo test fixtures

Three helpers create git repositories of increasing complexity:

| Helper | Location | What it creates |
|:-------|:---------|:----------------|
| `setupMinimalGitRepo` | `pr_split_git_detect_test.go` | Single `README.md` on `main` — for validation tests |
| `setupTestGitRepo` | `pr_split_test.go` | `main` + `feature` branch with multi-directory files — for pipeline tests |
| `setupBinaryTestRepo` | `pr_split_binary_e2e_test.go` | `.gitignore`, nested Go packages, realistic structure — for E2E |

All fixtures use `t.TempDir()` and are automatically cleaned up.

## Test helpers

| Helper | Purpose |
|:-------|:--------|
| `pushd(t, dir)` | Change working directory, restore on cleanup |
| `safeBuffer` | Thread-safe `bytes.Buffer` for concurrent test output |
| `prsplittest.NumVal(v)` | Extract numeric value from goja results |
| `parseOrchestratorResult(t, raw)` | JSON-parse pipeline result into Go struct |
| `waitForPTYOutput(t, buf, text, timeout)` | Block until PTY output contains expected text |

## Build tags

| Tag | Files | Purpose |
|:----|:------|:--------|
| `unix` | `pr_split_binary_e2e_test.go`, `pr_split_pty_unix_test.go`, others | PTY-dependent tests |
| `!windows` | `pr_split_06b_shell_test.go`, `pr_split_16_e2e_lifecycle_test.go` | Shell integration |

Tests without build tags run on all platforms.

## Timeouts

- Individual package tests: `go test -timeout=300s`
- Full suite with E2E: `go test -timeout=900s` (or `-timeout=20m` via `make all`)
- Per-test `evalJS` timeout: 60s default, configurable via `_evalTimeout` override
