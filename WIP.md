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
- [ ] Add go-behaviortree dependency
- [ ] Create internal/builtin/btbridge package structure
- [ ] Implement Bridge struct with eventloop setup
- [ ] Implement threadsafe Blackboard

### Phase 2: Core Bridge
- [ ] Implement JSLeafAdapter state machine (non-blocking)
- [ ] Register runLeaf JS helper during VM init
- [ ] Add panic recovery and error reporting
- [ ] Implement promise-finalization guarantees
- [ ] Implement BlockingJSLeaf alternative for simpler use cases

### Phase 3: JavaScript Module
- [ ] Copy bt.js to internal/builtin/btbridge/bt.js
- [ ] Create osm:bt module registration
- [ ] Expose bt.js primitives: running, success, failure
- [ ] Expose bt.js composites: node, tick, sequence, fallback, parallel, etc.
- [ ] Expose bridge-specific helpers: createLeaf, createBlackboard

### Phase 4: Tree Construction Helpers
- [ ] Create Go-side helpers for building trees with JS leaves
- [ ] Implement tree construction example
- [ ] Add Manager/Ticker integration with cancellation propagation

### Phase 5: Testing & Hardening
- [ ] Unit tests for Blackboard
- [ ] Unit tests for Bridge lifecycle (Start/Stop, RunOnLoop)
- [ ] Unit tests for JSLeafAdapter state machine
- [ ] Integration tests for JS leaf execution
- [ ] Tests for error handling (panics, promise rejections)
- [ ] Tests for cancellation propagation
- [ ] Tests for concurrent execution
- [ ] Memory leak tests (goroutine leaks)

### Phase 6: Documentation
- [ ] Document public API
- [ ] Add usage examples
- [ ] Update docs/scripting.md if needed

## Deviations Log
(Record any deviations from the plan here)

## Current Status
Phase 1: Understanding repository - COMPLETE
