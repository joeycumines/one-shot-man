# WIP — Session Continuation

## Current State

- **T001-T005**: ALL DONE. Rule of Two passed (2/2).
- **T104+T105**: ALL DONE. Rule of Two passed (2/2). Cross-platform zero failures.
- **T135+T136**: ALL DONE. Deadcode + betteralign audits clean.
- **Branch**: `wip` (ahead of `main`). All changes committed.
- **All tests passing**: macOS + Linux Docker + Windows all zero failures.

## T011: Eventloop Migration — IN PROGRESS

**Status**: Modifying 4 production files (tests handled separately).

### Files to modify (in order):
1. `internal/builtin/register.go` — EventLoopProvider interface + bt call
2. `internal/scripting/runtime.go` — Core Runtime struct + constructor + methods
3. `internal/scripting/engine_core.go` — QueueSetGlobal/QueueGetGlobal + EventLoop→Loop
4. `internal/builtin/bt/bridge.go` — Bridge struct + constructor + RunOnLoop→Submit

### API mapping:
- `*eventloop.EventLoop` → `*goeventloop.Loop`
- `loop.Start()` → `go loop.Run(ctx)`
- `loop.Stop()` → `loop.Shutdown(ctx)` + cancel run context
- `loop.RunOnLoop(func(*goja.Runtime))` → `loop.Submit(func())`
- Goja adapter: `gojaEventloop.New(loop, vm)` + `adapter.Bind()`

### Import aliases:
```go
goeventloop "github.com/joeycumines/go-eventloop"
gojaEventloop "github.com/joeycumines/goja-eventloop"
```

## Immediate Next Step

1. T011: Migrate test files (9 files) — IN PROGRESS
   - testutil/eventloop.go
   - bt/bridge_test.go, adapter_test.go, integration_test.go, benchmark_throughput_test.go
   - bubbletea/runner_test.go
   - orchestrator/templates_test.go, pr_split_test.go
   - pabt/require_test.go
2. T011: go get new deps, go mod tidy, run make
3. T164: CHANGELOG completeness  
4. T161: Goal autodiscovery test hardening
