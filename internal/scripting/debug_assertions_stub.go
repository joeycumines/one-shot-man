//go:build !debug

// Package scripting debug_assertions_stub provides no-op stubs for debug assertions.
// These are compiled when the "debug" build tag is NOT enabled, ensuring zero runtime cost.
package scripting

// debugWriteContextEnter is a no-op in release builds.
func (tm *TUIManager) debugWriteContextEnter() {}

// debugWriteContextExit is a no-op in release builds.
func (tm *TUIManager) debugWriteContextExit() {}

// The following functions are defined in the debug build but not used in release.
// They are kept as no-ops for API compatibility but marked with underscore-prefixed
// var assignments to suppress staticcheck warnings.

// debugAssertNotInWriteContext is a no-op in release builds.
func (tm *TUIManager) debugAssertNotInWriteContext(_ string) {}

// debugAssertInWriteContext is a no-op in release builds.
func (tm *TUIManager) debugAssertInWriteContext(_ string) {}

// Ensure the functions and fields are "used" to satisfy staticcheck.
// The debugLockState field is only used in the debug build, but must be in the
// struct for all builds. This reference ensures staticcheck doesn't warn.
var (
	_ = (*TUIManager).debugAssertNotInWriteContext
	_ = (*TUIManager).debugAssertInWriteContext
)

// init uses the debugLockState field to suppress staticcheck U1000 warning.
// The field is only accessed in -tags debug builds, but must exist in all builds.
func init() {
	// Reference the field without actually doing anything.
	// This is optimized away by the compiler but satisfies staticcheck.
	_ = func(tm *TUIManager) { _ = tm.debugLockState.Load() }
}
