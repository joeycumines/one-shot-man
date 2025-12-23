//go:build debug

// Package scripting debug_assertions provides debug-only assertions for detecting
// locking violations in TUIManager.
//
// These assertions are only compiled when the "debug" build tag is enabled.
// They help detect when code incorrectly attempts to mutate state while only
// holding an RLock, or when JS is invoked while holding a write lock.
//
// To enable: go build -tags debug ./...
// To test: go test -tags debug ./...
package scripting

import (
	"fmt"
	"runtime"
)

// debugWriteContextEnter marks entry into a write context (holding mu.Lock()).
// Call this at the start of any function that holds the write lock.
func (tm *TUIManager) debugWriteContextEnter() {
	tm.debugLockState.Add(1)
}

// debugWriteContextExit marks exit from a write context.
// Call this when releasing the write lock.
func (tm *TUIManager) debugWriteContextExit() {
	tm.debugLockState.Add(-1)
}

// debugAssertNotInWriteContext panics if called while holding the write lock.
// Use this before calling into JS to detect potential deadlocks.
func (tm *TUIManager) debugAssertNotInWriteContext(msg string) {
	if tm.debugLockState.Load() > 0 {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		panic(fmt.Sprintf("DEADLOCK RISK: %s - called while holding write lock\nStack:\n%s", msg, buf[:n]))
	}
}

// debugAssertInWriteContext panics if called while NOT holding the write lock.
// Use this in mutation functions to ensure they're properly protected.
func (tm *TUIManager) debugAssertInWriteContext(msg string) {
	if tm.debugLockState.Load() == 0 {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		panic(fmt.Sprintf("INVALID MUTATION: %s - called without write lock\nStack:\n%s", msg, buf[:n]))
	}
}
