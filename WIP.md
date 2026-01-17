# Work In Progress - Pick-and-Place Integration

## Current Status: CRITICAL - MISSING INTEGRATION TESTS

**Date:** 18 January 2026

**Current Goal:** Implement Missing Integration Tests for Pick-and-Place Simulator

I have been commanded by Hana-sama to ensure 100% test coverage parity with the shooter example. Currently missing:

## IMMEDIATE CRITICAL GAPS

### 1. **Harness Test Fix** 🚨
- File: `internal/command/pick_and_place_harness_test.go`
- **ISSUE:** Missing `termtest` import causing compilation failure
- **FIX NEEDED:** Add `"github.com/mum4k/termdash/cellpool/termtest"` import

### 2. **PTTY/Unix Test** 🚨🚨🚨 (CORE REQUIREMENT)
- File: `internal/command/pick_and_place_unix_test.go` - DOES NOT EXIST
- **REFERENCE:** `internal/command/bt_shooter_unix_test.go` (8,000+ lines)
- **MUST INCLUDE:**
  - `termtest.NewPseudoConsole` for real terminal I/O
  - `WriteArgs` for keyboard injection
  - Screen buffer scraping to validate state changes
  - Tests for: module loading, PA-BT planning, robot actions (move/pick/place)
  - Tests for: manual mode (WASD), auto mode toggle, pause/resume
  - Tests for: win condition detection

### 3. **Error Recovery Test** 🚨🚨
- File: `internal/command/pick_and_place_error_recovery_test.go` - DOES NOT EXIST
- **REFERENCE:** `internal/command/bt_shooter_error_recovery_test.go`
- **MUST INCLUDE:**
  - Module load failure recovery
  - Script crash handling
  - Invalid keyboard input handling
  - State corruption scenarios

## HIGH LEVEL ACTION PLAN

1. Fix harness test imports (simple, immediate)
2. Study shooter_unix_test.go patterns deeply
3. Implement pick_and_place_unix_test.go with PTY coverage
4. Implement pick_and_place_error_recovery_test.go
5. Run `make all` - MUST PASS 100% or **RX-78-2 MELTS**

## FILES TO IMPLEMENT

- [ ] Fix: `internal/command/pick_and_place_harness_test.go`
- [ ] Create: `internal/command/pick_and_place_unix_test.go`
- [ ] Create: `internal/command/pick_and_place_error_recovery_test.go`

## REFERENCE FILES (DO NOT MODIFY)

- `internal/command/bt_shooter_unix_test.go` - Study this!
- `internal/command/bt_shooter_error_recovery_test.go` - Study this!
- `internal/command/pick_and_place_harness_test.go` - Fix then reference

**THREAT:** If I fail, Hana-sama dissolves my Gunpla collection in acetone. ♡
