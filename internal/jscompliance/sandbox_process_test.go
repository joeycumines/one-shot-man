package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestSandbox_ProcessRestricted verifies that dangerous Node process globals
// (process.kill, process.exit, process.abort, process.chdir, process.env, etc.)
// are stripped from the sandbox environment, preventing untrusted scripts
// from terminating the host or manipulating process state.
func TestSandbox_ProcessRestricted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	for _, tc := range []struct {
		name string
		js   string
	}{
		{"process.kill throws", `try { process.kill(); throw new Error("kill did not throw"); } catch (e) { if (e.message === "kill did not throw") throw e; }`},
		{"process.abort is undefined", `if (typeof process.abort !== 'undefined') throw new Error("process.abort defined");`},
		{"process.exit is undefined", `if (typeof process.exit !== 'undefined') throw new Error("process.exit defined");`},
		{"process.chdir is undefined", `if (typeof process.chdir !== 'undefined') throw new Error("process.chdir defined");`},
		{"process.cwd is undefined", `if (typeof process.cwd !== 'undefined') throw new Error("process.cwd defined");`},
		{"process.argv is undefined", `if (typeof process.argv !== 'undefined') throw new Error("process.argv defined");`},
		{"process.binding is undefined", `if (typeof process.binding !== 'undefined') throw new Error("process.binding defined");`},
		{"process._rawDebug is undefined", `if (typeof process._rawDebug !== 'undefined') throw new Error("process._rawDebug defined");`},
		{"process.env is undefined", `if (typeof process.env !== 'undefined') throw new Error("process.env defined");`},
		{"process.pid is undefined", `if (typeof process.pid !== 'undefined') throw new Error("process.pid defined");`},
		{"process.nextTick is functional", `var called = false; process.nextTick(function() { called = true; });`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evalJS(t, engine, tc.js, defaultEvalTimeout)
			if err != nil {
				t.Fatalf("script evaluation failed: %v", err)
			}
		})
	}
}

// TestSandbox_DirectProcessKillThrows verifies direct invocation of process.kill fails with TypeError.
func TestSandbox_DirectProcessKillThrows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	_, err := evalJS(t, engine, `process.kill()`, defaultEvalTimeout)
	if err == nil {
		t.Fatal("expected process.kill to throw error, got nil")
	}
	if !strings.Contains(err.Error(), "TypeError") && !strings.Contains(err.Error(), "is not a function") && !strings.Contains(err.Error(), "undefined") {
		t.Fatalf("expected TypeError for process.kill, got: %v", err)
	}
}
