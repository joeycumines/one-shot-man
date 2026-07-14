package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestUnhandledRejection_Observability encodes the RISK-A contract: an
// UNHANDLED Promise rejection (one with no .catch/.then(_,reject)) MUST be
// observable by the host (logged), and the runtime MUST keep running other
// work (not crash). This is the HostPromiseRejectionTracker host hook
// (ecma-262 §27.2.1.9 + HTML §8.5.3.3).
//
// CURRENT STATE (RISK-A): the runtime does NOT observe unhandled rejections.
// go-eventloop's checkUnhandledRejections (promise.go:946) reads
// js.unhandledCallback under js.mu and SILENTLY DROPS when nil; the callback
// is set only via the WithUnhandledRejection JSOption at NewJS time, but the
// goja-eventloop adapter calls NewJS(loop) with NO options (adapter.go:70) and
// adapter.New takes none (adapter.go:62). *JS has no exported setter. osm's
// runtime.go never wires it. So a rejected promise with no handler vanishes —
// a real observability gap.
//
// FIX PATH (requires an eventloop-fork change): add an exported setter
// `func (js *JS) SetUnhandledRejection(h RejectionHandler)` to the
// joeycumines/go-eventloop fork (the field is already read dynamically under
// js.mu, so a post-construction setter is race-safe), publish a new
// pseudo-version, bump go.mod, and call
// `rt.adapter.JS().SetUnhandledRejection(<slog.Error handler>)` in runtime.go
// INSIDE the loop.Submit closure AFTER rt.adapter.Bind() (runtime.go:138) and
// BEFORE errCh<-nil — on-loop, before any user JS.
//
// EVENTLOOP-FORK-BLOCKED — per the compliance directive, this test FAILS
// (not skips) until the fork is updated. A WIP refactor is in progress in the
// eventloop fork that may address this. When the fix lands, this test passes.
func TestUnhandledRejection_Observability(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, stderr := newComplianceEngine(t, ctx)

	// Create a rejected promise with NO handler (unhandled), then run an
	// unrelated settled promise to force a loop turn (so the unhandled-rejection
	// check runs).
	if _, err := evalJS(t, engine, `(function(){ Promise.reject(new Error('risk-a-marker')); return Promise.resolve('alive'); })()`, defaultEvalTimeout); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// When the handler is wired to slog.Error, the marker must surface in the
	// engine's stderr (the startup/slog sink). Until the fork fix lands, this
	// assertion FAILS — the rejection vanishes silently.
	if !containsStderr(stderr, "risk-a-marker") {
		t.Errorf("RISK-A: unhandled rejection was not observable (expected 'risk-a-marker' in logs once WithUnhandledRejection is wired)")
	}
}

// containsStderr reports whether want appears in b.String() (a real substring
// check so the skipped RISK-A assertion is valid once activated).
func containsStderr(b interface{ String() string }, want string) bool {
	return b != nil && strings.Contains(b.String(), want)
}

// TestUnhandledRejection_DoesNotCrash is the ACTIONABLE RISK-A guard that
// works TODAY (no fork fix needed): an unhandled Promise rejection must NOT
// crash the runtime or stall the event loop — other work must continue. A
// regression that turned the current "silent" behavior into a CRASH (or a loop
// stall) would fail this. (Full observability — the rejection surfacing in
// logs — remains the tracked t.Skip above, pending the fork setter.)
//
// The rejection is scheduled inside a timer callback so evalJS returns the
// timer id (a sync value) and attaches NO handler — leaving the rejection
// genuinely unhandled.
func TestUnhandledRejection_DoesNotCrash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	// Schedule an unhandled rejection; the expression returns 1 (sync), so
	// evalJS attaches no Promise handler.
	if _, err := evalJS(t, engine, `(setTimeout(function(){ Promise.reject(new Error('risk-a-no-crash')); }, 0), 1)`, defaultEvalTimeout); err != nil {
		t.Fatalf("schedule rejection: %v", err)
	}
	// After the rejection lands and the unhandled-check runs, unrelated async
	// work must still complete (loop alive, no crash/stall).
	v, err := evalJS(t, engine, `await new Promise(function(r){ setTimeout(function(){ r('alive-after-rejection'); }, 50); })`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("RISK-A: post-rejection work failed — the loop crashed or stalled after an unhandled rejection: %v", err)
	}
	if s, _ := v.(string); s != "alive-after-rejection" {
		t.Errorf("RISK-A: post-rejection value = %v, want 'alive-after-rejection'", v)
	}
}
