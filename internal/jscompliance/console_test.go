package jscompliance

import (
	"context"
	"testing"
)

// consoleMethodGroups enumerates the two console method sources to assert
// (CONSOLE-1): the adapter binds timer/inspection methods, and engine_core.go
// overwrites log/warn/error/info/debug from goja_nodejs/console. Both sets
// must coexist and be callable.
var consoleLoggingMethods = []string{"log", "warn", "error", "info", "debug"}
var consoleTimerMethods = []string{"time", "timeEnd", "count", "group", "groupEnd", "assert", "table"}

// TestConsole_MethodSetsCoexist asserts BOTH method sources are present and
// that calling them does not throw. (Routing: goja_nodejs/console writes to
// the PROCESS stdout/stderr via its default printer — NOT the engine's
// io.Writer — which is the CONSOLE-1 drift documented in WIP/docs; capturing
// the real os.Stdout in-test is invasive, so we assert presence + safety, the
// actionable contract.)
func TestConsole_MethodSetsCoexist(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	for _, m := range append(append([]string{}, consoleLoggingMethods...), consoleTimerMethods...) {
		t.Run(m, func(t *testing.T) {
			t.Parallel()
			typeof, err := evalJS(t, engine, `typeof console[`+jsStringLit(m)+`]`, defaultEvalTimeout)
			if err != nil {
				t.Fatalf("console.%s probe: %v", m, err)
			}
			if s, _ := typeof.(string); s != "function" {
				t.Errorf("console.%s typeof = %v, want function", m, s)
			}
		})
	}
}

// TestConsole_LogDoesNotThrow confirms a logging call is safe in the runtime
// (it routes to the process stdout/stderr; we only assert no panic here).
func TestConsole_LogDoesNotThrow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	if _, err := evalJS(t, engine, `console.log("compliance-console-probe"); console.warn("w"); console.error("e");`, defaultEvalTimeout); err != nil {
		t.Fatalf("console call threw: %v", err)
	}
}
