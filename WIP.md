# WIP: Integrating bt.js with go-behaviortree via Goja

## Goal
Implement a complete, release-ready integration of `bt.js` with `github.com/joeycumines/go-behaviortree` using `github.com/dop251/goja`.

## Architecture Decision
**Go-Centric with JS Leaves (Variant C.2 - Event-Driven Bridge)** as recommended in the spec:
- `go-behaviortree` is the canonical BT engine
- JS is used for leaf behaviors via async bridge on `goja_nodejs/eventloop`
- Go-owned blackboard exposed to JS
- Event-driven bridge for optimal performance

## Implementation Plan

### Phase 1: Foundation
- [x] Understand repository structure and existing patterns
- [x] Clone bt.js gist
- [x] Add go-behaviortree dependency
- [x] Create internal/builtin/btbridge package structure
- [x] Implement Bridge struct with eventloop setup
- [x] Implement threadsafe Blackboard

### Phase 2: Core Bridge
- [x] Implement JSLeafAdapter state machine (non-blocking)
- [x] Register runLeaf JS helper during VM init
- [x] Add panic recovery and error reporting
- [x] Implement promise-finalization guarantees
- [x] Implement BlockingJSLeaf alternative for simpler use cases

### Phase 3: JavaScript Module
- [x] Copy bt.js to internal/builtin/btbridge/bt.js
- [x] Create osm:bt module registration (Require function)
- [x] Expose bt.js primitives: running, success, failure
- [x] Expose go-behaviortree composites: Sequence, Selector, etc.
- [x] Expose bridge-specific helpers: createLeafNode, createBlackboard

### Phase 4: Tree Construction Helpers
- [x] Create Go-side helpers for building trees with JS leaves
- [x] Implement tree construction example (in integration tests)
- [x] Add Manager/Ticker integration with cancellation propagation

### Phase 5: Testing & Hardening
- [x] Unit tests for Blackboard
- [x] Unit tests for Bridge lifecycle (Start/Stop, RunOnLoop)
- [x] Unit tests for JSLeafAdapter state machine
- [x] Integration tests for JS leaf execution
- [x] Tests for error handling (panics, promise rejections)
- [x] Tests for cancellation propagation
- [ ] Tests for concurrent execution (advanced scenarios)
- [ ] Memory leak tests (goroutine leaks)

### Phase 6: Documentation
- [x] Document public API (package-level doc.go)
- [x] Add usage examples (in doc.go and integration tests)
- [ ] Update docs/scripting.md if needed

## Deviations Log
- Module registration: btbridge is kept as a standalone package rather than auto-registered
  in builtin.Register due to the complexity of Bridge lifecycle management. Users can
  import and use the package directly.
- bt.js ES modules: The original bt.js uses ES module syntax which isn't compatible with
  CommonJS require. The Go-side primitives are exposed instead, with bt.js embedded for
  future use or bundled workflows.

## Current Status
Phase 5 MOSTLY COMPLETE - all core tests passing
Phase 6 IN PROGRESS - documentation added

## Remaining Work
- [ ] Additional stress tests for concurrent scenarios
- [ ] Memory/goroutine leak detection tests  
- [ ] Consider adding to docs/scripting.md
- [ ] Run code_review and codeql_checker
