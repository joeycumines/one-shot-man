# WIP — Current Session State

## Session
- **Branch**: wip (385+ commits ahead of main)
- **Build**: ALL GREEN — macOS, Linux Docker, Windows — 0 failures on all 3 platforms
- **Blueprint**: T001-T149 DONE. T150+ not yet defined.
- **Latest commits** (this session):
  - `e54d744` — test(bubbletea): add batch 13 coverage tests for runProgram and Require
  - `e3e5390` — test(command): add batch 12 coverage tests for config reset error paths
  - `21b997a` — Add batch 11 coverage tests for logging, bt adapter, and controlAdapter
  - (prior session): `1608329`, `7723a52`, `c827c70`

## What Was Done This Session

### Batch 11 — Committed as 21b997a (35 tests)
- **Scripting**: 14 tests — WithAttrs/WithGroup fileHandler, rotate error paths, symlink validation
- **Command**: 9 tests — InterruptCurrent, EnqueueTask, GetStatus, setActive/clearActive
- **BT**: 12 tests — dispatchJSWithGen error paths, BlockingJSLeaf error paths

### Batch 12 — Committed as e3e5390 (8 tests)
- **Command**: 8 tests — ConfigReset NoConfigPath×2, DiskDeleteError×2, DiskCountLarger, InitExtraArgs, SchemaExtraArgs, ResetUnknownKey
- Originally 14 tests; 6 duplicates identified by review and removed (3 BT, 3 command)

### Batch 13 — Committed as e54d744 (6 tests)
- **Bubbletea**: 6 tests — runProgram signalNotify/signalStop nil guards, pipe input/output TTY fallback, newModel null config, throttle interval clamping
- Coverage: bubbletea 91.2%→91.6%, runProgram 78.3%→80.7%

### Cross-Platform Verification
- All 3 batches verified on macOS, Linux Docker, Windows — ALL GREEN

## Coverage Baselines (current)
| Package | Coverage |
|---------|----------|
| exec, crypto, grpc, regexp, time, unicodetext, goroutineid | 100.0% |
| config | 97.4% |
| argv (root) | 96.0% |
| fetch | 95.3% |
| gitops | 95.3% |
| claudemux | 94.9% |
| pabt | 94.7% |
| bt | 93.4% |
| nextintegerid | 92.9% |
| scripting | 91.7% |
| bubbletea | 91.6% |
| testutil | 90.7% |
| session | 89.9% |
| storage | 89.8% |
| pty | 88.9% |
| command | 87.8% |
| ctxutil | 87.2% |
| builtin/os | 87.0% |
| builtin/argv | 83.3% |
| mouseharness | 80.0% |

## Remaining Frontiers (diminishing returns)
- **command**: dynamicDispatchLoop 17.6%, code_review 46.9%, dispatchTask 61.4% — need complex mux/script infra
- **storage**: AtomicWriteFile 75.8%, ArchiveSession 76.0% — need FS-level error injection
- **session**: Darwin-specific functions 71-78% — platform-dependent
- **builtin/argv**: 83.3% — Goja ExportTo too flexible, hard to trigger fallback paths
- **ctxutil**: exportGojaValue/toObject error paths — need exotic Goja value types
- **mouseharness**: 80% — needs TTY/mouse interaction
- **bubbletea**: runProgram TTY paths, panic recovery — needs real terminal

## Architecture Notes for Next Takumi
- **Rule of Two**: ALWAYS 2 contiguous PASS reviews with ≥10 hypotheses before commit
- **Go 1.26**: t.Parallel() + t.Setenv() CANNOT coexist — will panic
- **staticcheck SA1012**: literal nil as context.Context forbidden — use `var nilCtx context.Context`
- **config.GetConfigPath()**: returns REAL host config path — MUST isolate with t.Setenv("HOME", t.TempDir()) AND t.Setenv("OSM_CONFIG", "")
- **chmod 0555**: Not reliable on macOS root — defensive assertions only
- **ETXTBSY**: Linux — write-to-temp-then-rename for executables
- **SetTestPaths**: Mutates package globals — MUST NOT use t.Parallel()
- **bt.Node**: Function type `func() (bt.Tick, []bt.Node)` — NOT an interface
- **Duplicate detection**: ALWAYS check existing tests before creating new ones (grep for function names)

## Potential Next Work (beyond coverage)
1. **PR splitter rewrite** — T044-T060 in blueprint, major feature work
2. **Documentation accuracy audit** — verify docs match current code
3. **Error message consistency** — standardize format across packages
4. **Benchmark optimization** — profile hot paths
5. **Integration tests** — claudemux end-to-end with actual Claude Code
