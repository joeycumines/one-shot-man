package jscompliance

import (
	"context"
	"os"
	"testing"
	"time"
)

// specTimeout is the per-spec cap for the FAST core specs. Ample vs the
// tiny timer durations (~5-30ms) inside them; a hang fails fast.
const specTimeout = 30 * time.Second

// TestCoreES runs the core ECMAScript semantics spec (the 11 axes).
func TestCoreES(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/core_es.spec.js", specTimeout)
}

// TestCorePromises runs the Promise / async-await / combinators spec.
func TestCorePromises(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/core_promises.spec.js", specTimeout)
}

// TestCoreMicrotask pins the strict microtask ordering (always-on since 20260823) guarantee.
func TestCoreMicrotask(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/core_microtask.spec.js", specTimeout)
}

// TestCoreTimers pins timer semantics.
func TestCoreTimers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/core_timers.spec.js", specTimeout)
}

// TestCoreAbort pins AbortController / AbortSignal (incl ES2024 statics).
func TestCoreAbort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/core_abort.spec.js", specTimeout)
}

// TestCoreES_ForkBlocked runs the GOJA-FORK-BLOCKED ES2024+ features that are expected to FAIL.
// This is separated from TestCoreES so that gmake test-jscompliance (main tier) can pass while
// the dedicated test-jscompliance-fork-blocked target shows the expected failures.
// When the goja fork is updated, these tests will pass and can be promoted.
func TestCoreES_ForkBlocked(t *testing.T) {
	if os.Getenv("JS_COMPLIANCE_FORK_BLOCKED") == "" {
		t.Skip("skipping fork-blocked test (set JS_COMPLIANCE_FORK_BLOCKED=1 to run)")
	}
	if testing.Short() {
		t.Skip("skipping slow fork-blocked test in short mode")
	}
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/core_es_fork_blocked.spec.js", specTimeout)
}
