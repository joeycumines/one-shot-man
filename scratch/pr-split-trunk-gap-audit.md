# Trunk Gap Audit: `wip` vs `origin/main`

**Generated:** 2026-04-15  
**Trunk ref:** `origin/main`  
**Branch:** `wip`  
**Diff scope:** 802 files changed, 276,953 insertions, 9,735 deletions  
**All checks:** PASS (`gmake make-all-with-log` on macOS)

---

## 1. Commit Summary

~500+ commits ahead of trunk. Key thematic groups:

| Group | Representative Commits | Files |
|-------|----------------------|-------|
| pr-split TUI wizard | `7884f580`, `f2235119`, `098a9fc0`, dozens of `pr_split_16_*` | ~120 JS + ~80 test files |
| termmux subsystem | `10f7d913`, `564a3581`, `ffef9002`, `f789fbe7` | ~40 Go + test files |
| claudemux module | `ea04bd28`, `9150dd0e`, `19357c60` | ~25 Go files |
| BubbleTea v2 migration | `1fe02e5a`, `740951e1`, `32cdb362` | ~30 files |
| osm:fetch, osm:flag, etc | `3eb5b26b`, `7c3ae839`, `86d0d0c2` | ~15 Go files |
| Session/storage hardening | Various | ~20 files |
| Documentation | Various | ~30 docs |
| Skills/agent config | Various | ~15 files |
| Goal/script system | Various | ~20 files |
| Config schema/validation | `370ceff9`, `d13c49cd` | ~10 files |
| Coverage gap tests | 100+ commits | ~100 test files |
| Security tests | `3f6b7281`, `f0b04e13` | ~5 test files |
| Benchmarks | Various | ~15 test files |

---

## 2. Public Claims That Overstate Reality

### 2.1 Shell Tab Claims in Architecture Doc and CHANGELOG

**Claim:** `docs/architecture-pr-split-chunks.md` line 215 lists `_renderShellPane | Function | lipgloss | Shell tab pane (interactive shell VTerm)` and line 330 lists `_INTERACTIVE_RESERVED_KEYS | Object | constant | Minimal reserved keys for Shell tab (7 entries)`. CHANGELOG line 77 claims 13 chrome pane renderer test functions covering `_renderShellPane` (placeholder/content/focus/path-truncation/narrow).

**Reality:** The shell tab was removed in Task 8 (commit `f84175ad`). Verify pane IS the interactive shell now. 10 explicit `// Task 8: Shell tab removed` comments across the JS codebase confirm this. The architecture doc entries for `_renderShellPane` are stale — the function no longer exists. The `_INTERACTIVE_RESERVED_KEYS` constant still exists but serves the verify pane, not a dedicated shell tab. The CHANGELOG test coverage entry references `_renderShellPane` tests that were removed (`pr_split_15b_unit_test.go:202` says "Task 8: renderShellPane tests removed").

**Evidence:** `grep -r 'Task 8.*Shell tab removed' internal/command/pr_split*.js` returns 10 matches across 6 files. `grep -r '_renderShellPane' internal/command/pr_split*.js` returns only the removal comments.

### 2.2 Persistence/Resume Claims

**Claim:** CHANGELOG says "pause/resume with checkpoints" and `saveCheckpoint` persistence persists "all runtime caches... to disk for crash recovery." README lists `-resume (continue from a saved plan)`.

**Reality:**
- `prSplit.previousState` is loaded at init but **never consumed by production TUI code**. No resume dialog, screen, or UI shows session state, liveness, or recovery options.
- `prSplit.persistence.cleanup` is exported but **not wired into the clean-exit flow** — `tea.quit()` is called without calling `persistence.cleanup()`, so state files accumulate indefinitely.
- Resume spawns a **new** Claude process (`claudeExecutor.spawn(null, {...})` at `pr_split_10d_pipeline_orchestrator.js:813`), never consulting PID or reattaching to live sessions.
- Claude sessions use `StringIOSession` which has no `Pid()` method — PID metadata is structurally unavailable for attached Claude sessions.

**Evidence:**
- `pr_split_16g_persistence.js:138` loads `previousState` but no TUI handler references it.
- `pr_split_16c_tui_handlers_verify.js:54` calls `tea.quit()` without `persistence.cleanup()`.
- `stringio_session.go` has no `Pid()` method.
- `provider.go:22-45` (`AgentHandle` interface) has no `Pid()` method.

### 2.3 "Error Recovery Supervisor" Claims

**Claim:** CHANGELOG says "error recovery supervisor with retry/restart/escalate/abort flow."

**Reality:** The `Supervisor` in `internal/builtin/claudemux/recovery.go` is fully implemented and tested, but **not used by pr-split or any other production code**. It is exposed as a JS binding (`newSupervisor()`) but never called from `pr_split_09_claude.js` or any other pr-split JS file.

**Evidence:** `grep -r 'newSupervisor\|Supervisor' internal/command/pr_split*.js` returns zero matches.

### 2.4 `osm:claudemux` Component Claims

**Claim:** CHANGELOG presents Guard, Supervisor, Pool, SafetyValidator, and ChoiceResolver as features of the `osm:claudemux` module.

**Reality:** All five are fully implemented, tested, and exposed as JS bindings — but **none are used in production**. pr-split uses only `newRegistry()`, `claudeCode()` / `ollama()`, `registry.register()`, and `registry.spawn()`. The five higher-level components are available building blocks but invisible to pr-split.

| Component | Implemented | Tested | JS Binding | Used in Production |
|-----------|------------|--------|-----------|-------------------|
| Guard | ✅ | ✅ | `newGuard()` | ❌ |
| Supervisor | ✅ | ✅ | `newSupervisor()` | ❌ |
| Pool | ✅ | ✅ | `newPool()` | ❌ |
| SafetyValidator | ✅ | ✅ | `newSafetyValidator()` | ❌ |
| ChoiceResolver | ✅ | ✅ | `newChoiceResolver()` | ❌ |

**Evidence:** `module.go:179,260,276,404,446` expose bindings. No `pr_split*.js` file calls any of these constructors.

### 2.5 Documentation Architecture Claims

**Claim:** `docs/architecture-pr-split-chunks.md` documents 30 chunks including Shell pane renderers and shell-specific handlers.

**Reality:** Shell tab was removed in Task 8. The architecture doc still references shell pane functions that no longer exist. The doc structure is accurate for the current chunk count but references to shell-specific functions are stale.

---

## 3. Proven Blockers for End-to-End Claude Interaction

### 3.1 Claude Lifecycle Is Polling-Based (Works, But Fragile)

`pollClaudeScreenshot()` in `pr_split_16d_tui_handlers_claude.js` polls on a tick interval. This works but means:
- Latency between Claude output and TUI update depends on tick frequency
- No event-driven notification of crashes, bells, or exits
- `tuiMux.on('output', cb)` events are available but not wired for primary rendering

**Evidence:** `pr_split_16d_tui_handlers_claude.js:890` (`pollClaudeScreenshot` function definition) uses tick-based polling.

### 3.2 Input Translation Lives in App JS

`keyToTermBytes`, `mouseToTermBytes`, `computeSplitPaneContentOffset`, and `writeMouseToPane` are all implemented in pr-split-specific JS (`pr_split_16d_tui_handlers_claude.js:629-776`, `pr_split_16e_tui_update.js:64-84`). These encode BubbleTea key/mouse events to VT100 terminal bytes. This is not reusable — another command wanting pane input would need to re-implement this.

### 3.3 No Fixture-Backed Strategy Quality Tests

The `auto` strategy depends on Claude. The heuristic strategies (directory, extension, chunks, dependency) have unit tests but no fixture-backed tests proving quality on realistic trunk-like diffs (multi-directory Go changes, generated-file churn, rename-heavy diffs, mixed docs+code).

### 3.4 Verify Has Three Overlapping Modes

Verification spans three modes without clear canonical selection:

1. **Persistent Interactive Shell** (Unix only) — PTY-based, user signals pass/fail manually
2. **One-Shot CaptureSession** — Single command, exit-code-based
3. **Async Fallback** (`verifySplitAsync`) — No SessionManager, promise-based

The persistent shell is preferred on Unix but falls back silently. No user-visible indication of which mode is active. Windows always gets one-shot mode. The async fallback triggers when CaptureSession creation fails — but the user doesn't know why.

---

## 4. Strategy-Quality Risks

### 4.1 Auto Strategy Requires Claude

The `auto` strategy in `pr_split_02_grouping.js` sends diffs to Claude for classification. If Claude is unavailable, it falls back to heuristic mode. This fallback path is not tested with fixture diffs.

### 4.2 Dependency Strategy Is Go-Only

The `dependency` strategy in `pr_split_02_grouping.js` analyzes Go import graphs. It has no equivalent for non-Go languages. This limits utility for polyglot repositories.

### 4.3 No Trunk-Like Fixture Tests

There are no fixture-backed tests that take a realistic multi-file diff (like the ~800-file diff on this branch) and verify that strategy selection produces reviewable, well-motivated splits.

---

## 5. What Actually Works (Verified by Passing Tests)

| Feature | Test Target | Status |
|---------|------------|--------|
| Full build (macOS) | `make-all-with-log` | ✅ PASS |
| termmux SessionManager | `test-termmux` | ✅ PASS |
| PTY spawn/read/write | `test-termmux` | ✅ PASS |
| VTerm rendering | `test-termmux` | ✅ PASS |
| pr-split fast unit tests | `test-prsplit-fast` | ✅ PASS |
| pr-split E2E lifecycle | `test-prsplit-e2e` | ✅ PASS |
| Inline keystroke → PTY | `test-inline-e2e` | ✅ PASS |
| Claude SessionID pinning | Unit tests | ✅ PASS |
| Session-specific passthrough | Unit tests | ✅ PASS |
| PTY resize delegation | Unit tests | ✅ PASS |
| BubbleTea v2 TUI | Unit/integration | ✅ PASS |
| Cross-build (linux, darwin, windows) | `cross-build` | ✅ PASS |

### Known Platform Gaps

| Platform | Target | Status | Notes |
|----------|--------|--------|-------|
| macOS | `make-all-with-log` | ✅ PASS | Primary dev platform |
| Linux (Docker) | `make-all-in-container` | ⚠️ BLOCKED | Pre-existing go.mod replace directive |
| Windows | `make-all-run-windows` | ⚠️ BLOCKED | ConPTY smoke test timeout (20m) |

---

## 6. File-by-File Explanation of Key Changed Directories

### `internal/termmux/` (NEW — ~40 files, ~15K lines)

The entire terminal multiplexer subsystem. Contains:
- **SessionManager** (`manager.go`) — Worker-based session dispatcher with fan-out events
- **EventBus** (`eventbus.go`) — Typed event fan-out (output, bell, exit, closed, activated)
- **CaptureSession** (`capture.go`) — PTY-backed command capture with VTerm rendering
- **StringIOSession** (`stringio_session.go`) — String I/O wrapper for non-PTY sessions (Claude)
- **Passthrough** (`passthrough.go`) — Raw terminal passthrough mode
- **VTerm** (`vt/`) — Pure-Go VT100 terminal emulator with SGR, CSI, UTF-8, wide chars
- **PTY** (`pty/`) — Cross-platform PTY (Unix pty, Windows ConPTY)
- **Persistence** (`persistence.go`) — Session state serialization

### `internal/builtin/claudemux/` (NEW — ~25 files, ~15K lines)

Claude Code integration module. Contains:
- **Provider** (`provider.go`, `claude_code.go`) — PTY-based Claude Code spawning
- **Guard** (`guard.go`) — Health/rate-limit monitoring (unused in production)
- **Supervisor** (`recovery.go`) — Lifecycle state machine (unused in production)
- **Pool** (`pool.go`) — Concurrent instance management (unused in production)
- **Safety** (`safety.go`) — Action intent/risk classification (unused in production)
- **Choice** (`choice.go`) — Weighted criteria evaluation (unused in production)
- **Module** (`module.go`) — JS bindings for all components

### `internal/command/pr_split*.go` + `pr_split_*.js` (NEW — ~30 JS chunks, ~120 test files)

The pr-split TUI wizard. 30 JS chunk files implementing:
- Diff analysis, grouping strategies, planning, validation, execution
- Verification (PTY-based with VTerm rendering)
- Claude integration (spawn, passthrough, question detection)
- Full BubbleTea graphical wizard (7 screens, overlays, split-view)
- Pipeline orchestration with async processing

### `internal/builtin/termmux/` (NEW — ~4 files)

JS bindings for the termmux SessionManager (`module.go`).

### Key Modified Files

| File | Change | Notes |
|------|--------|-------|
| `internal/scripting/engine_core.go` | Major refactor | NewEngine replaces NewEngineDeprecated, SubmitInternal FIFO fix |
| `internal/builtin/bubbletea/bubbletea.go` | v2 migration | BubbleTea v2 API changes |
| `internal/builtin/bubbletea/keys_gen.go` | Rewrite | Supplementary-plane rune fix |
| `cmd/osm/main.go` | Additions | New command registrations |
| `docs/architecture.md` | Major rewrite | SessionManager, termmux docs |
| `CHANGELOG.md` | Major additions | All new features documented |
| `go.mod` | Dependency updates | BubbleTea v2, new deps |

---

## 7. Recommendations for Next Tasks

### Immediate (Blocks Prime-Time)

1. **Fix CHANGELOG stale shell tab references** — Remove `_renderShellPane` entries, update shell-related claims
2. **Wire persistence cleanup into exit flow** — Call `persistence.cleanup()` on clean exit
3. **Document claudemux components as available building blocks** — Don't claim production usage that doesn't exist

### High Value (Improves Production Readiness)

4. **Add fixture-backed strategy quality tests** — Prove strategy selection on realistic diffs
5. **Wire Claude lifecycle events** — Replace polling with event-driven updates
6. **Move input translation to reusable layer** — Extract from app JS into termmux-facing module
7. **Collapse verify modes** — Choose one canonical path, make fallbacks explicit
8. **Fix persistence resume** — Consume `previousState`, show honest session state in TUI

### Platform (Unblocks Cross-Platform)

9. **Fix go.mod replace directive** — Blocking Linux container builds
10. **Fix ConPTY smoke test timeout** — Blocking Windows CI

---

## 8. Conclusion

The `wip` branch represents a massive amount of work — a complete terminal multiplexer, VT100 emulator, Claude Code integration layer, and BubbleTea TUI wizard for PR splitting. The core subsystems (termmux, VTerm, SessionManager) are well-tested and architecturally sound. The Claude pinned-SessionID work, PTY resize delegation, and session-specific passthrough are all recently fixed and verified.

However, public documentation overstates the current capabilities in several areas: persistence/resume is structurally present but operationally inert, five claudemux components are fully built but unused, the shell tab no longer exists but is still documented, and cross-platform verification is blocked by pre-existing issues. The verification experience spans three overlapping modes without clear user-facing distinction.

The branch is not yet prime-time for production use — but the gap is narrowing and the remaining work is well-defined.
