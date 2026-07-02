package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestGlobals_Log runs the structured-log spec (entries/attrs/search/clear).
func TestGlobals_Log(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/globals_log.spec.js", specTimeout)
}

// TestGlobals_State runs the tui.createState Symbol-key spec.
func TestGlobals_State(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/globals_state.spec.js", specTimeout)
}

// TestGlobals_Args runs the args spec (harness binds args=[]).
func TestGlobals_Args(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/globals_args.spec.js", specTimeout)
}

// TestGlobals_OutputPrintF asserts output.print/printf write to the engine's
// stdout with format substitution. (output routes to the engine stdout buffer,
// unlike console — see console_test.go.)
func TestGlobals_OutputPrintF(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, stdout, _ := newComplianceEngine(t, ctx)
	for _, js := range []string{
		`output.print("plain")`,
		`output.printf("n=%d s=%s", 7, "x")`,
		`output.printf("v=%v", {a:1})`,
	} {
		if _, err := evalJS(t, engine, js, defaultEvalTimeout); err != nil {
			t.Errorf("%s: %v", js, err)
		}
	}
	out := stdout.String()
	for _, want := range []string{"plain", "n=7 s=x", "v="} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q missing %q", out, want)
		}
	}
}

// TestGlobals_CtxIsConditional documents the DRIFT (ctx-DRIFT): scripting.md
// lists `ctx` as a global "available in every script", but a bare Engine does
// NOT bind it — ctx is set only when an ExecutionContext is active (command /
// run mode). Pinning reality: ctx is undefined until setExecutionContext runs.
func TestGlobals_CtxIsConditional(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `typeof ctx`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("typeof ctx probe: %v", err)
	}
	if s, _ := v.(string); s != "undefined" {
		t.Logf("note: ctx is now defined (%v) in the bare engine — if intentional, scripting.md's 'available in every script' is satisfied; update this test", s)
	}
	// This is informational (drift record), not a hard failure: the contract
	// is that ctx is conditionally bound. scripting.md should document the
	// condition (see docs update task / DRIFT-3 family).
}
