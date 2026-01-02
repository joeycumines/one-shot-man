# WIP: Integrating bt.js with go-behaviortree via Goja

## Goal
Implement a complete, release-ready integration of `bt.js` with `github.com/joeycumines/go-behaviortree` using `github.com/dop251/goja`.

## Architecture Decision
**Go-Centric with JS Leaves (Variant C.2 - Event-Driven Bridge)** as the baseline:
- `go-behaviortree` is the canonical BT engine
- JS is used for leaf behaviors via async bridge on `goja_nodejs/eventloop`
- Go-owned blackboard exposed to JS
- Event-driven bridge for optimal performance

**Important:** Variant C.2 is the baseline only. Full interoperability with bt.js composites (status translation, module compatibility, mixed Go/JS composites) requires additional implementation documented in plan.md.

## Implementation Status

### Core Implementation (Complete)
- [x] Bridge with eventloop, lifecycle context, Done() channel
- [x] Blackboard with thread-safe accessors
- [x] JSLeafAdapter with generation counting (prevents stale callback corruption)
- [x] BlockingJSLeaf with sync.Once and bridge.Done() (prevents deadlock)
- [x] Status string constants (single source of truth)
- [x] Require module with nil-bridge check

### Critical Defect Fixes (C1-C6 from review-1.md)
- [x] C1: Generation counter in JSLeafAdapter to prevent stale callbacks
- [x] C2: Atomic state transition in Tick() (under lock before dispatch)
- [x] C3: BlockingJSLeaf uses select with bridge.Done()
- [x] C4: BlockingJSLeaf uses sync.Once for single send
- [x] C5: go.mod includes goja dependencies (verified)
- [x] C6: Cancellation increments generation counter

### High-Priority Fixes (H1-H7 from review-1.md)
- [x] H1: Context leak documented (one-shot semantics)
- [ ] H2: Promise timeout (optional, not blocking)
- [ ] H3: Require module registration (manual, not auto-registered)
- [ ] H4: Manager tests (Manager is low-priority for now)
- [x] H5: Tests use deterministic synchronization
- [x] H6: Safe type assertions in tests
- [x] H7: Bridge.Done() channel implemented

### Additional Fixes (A1-A14 from review-1.md)
- [x] A1: Event loop termination handled via finalize with generation
- [x] A2: Type impedance documented (Go int vs JS string)
- [x] A3: newNode limitation documented
- [ ] A4: exposeBlackboard type handling (documented, not blocking)
- [x] A5: Bridge nil check added to Require
- [ ] A6: Manager ctx parameter (Manager is low-priority)
- [x] A7: obj.Set errors documented (intentionally ignored)
- [x] A8: Status constants defined and used
- [x] A9: Error wrapping uses %w where appropriate
- [ ] A10: Bridge shutdown race (handled via generation counting)
- [ ] A11: Test count updated
- [ ] A12: Goroutine leak testing (needs goleak integration)
- [ ] A13: Performance claims removed (no unverified claims)
- [x] A14: One-shot context trap documented

### Tests
- [x] JSLeafAdapter state machine tests
- [x] BlockingJSLeaf success/failure/error tests
- [x] Cancellation tests
- [x] Generation guard test (stale callback prevention)
- [x] Bridge stop while waiting test
- [x] Context cancellation test
- [x] Integration tests (composites, ticker, memorize, fork)

## Remaining Work
- [ ] Full interoperability with bt.js composites (requires translation layer)
- [ ] Module system integration (builtin.Register for osm:bt)
- [ ] Goroutine leak detection (goleak integration)
- [ ] Manager tests or removal
- [ ] Promise timeout mechanism

## Status: IN PROGRESS

Critical concurrency defects (C1-C6) fixed. Tests passing. Documentation updated.
Additional work needed for full interoperability per plan.md requirements.
