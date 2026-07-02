package jscompliance

import (
	"context"
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

// TestCoreMicrotask pins the WithStrictMicrotaskOrdering guarantee.
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
