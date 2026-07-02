package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestUnhandledRejection_Observability encodes the RISK-A contract: an
// UNHANDLED Promise rejection (one with no .catch/.then(_,reject)) MUST be
// observable by the host (logged), and the runtime MUST keep running other
// work (not crash).
//
// CURRENT STATE (RISK-A, verified): the runtime does NOT observe unhandled
// rejections. go-eventloop's checkUnhandledRejections (promise.go:946) reads
// js.unhandledCallback under js.mu and SILENTLY DROPS when nil; the callback
// is set only via the WithUnhandledRejection JSOption at NewJS time, but the
// goja-eventloop adapter calls NewJS(loop) with NO options (adapter.go:70) and
// adapter.New takes none (adapter.go:62). *JS has no exported setter. osm's
// runtime.go never wires it. So a rejected promise with no handler vanishes —
// a real observability gap.
//
// FIX PATH (requires a dependency change, deferred): add an exported setter
// `func (js *JS) SetUnhandledRejection(h RejectionHandler)` to the
// joeycumines/go-eventloop fork (the field is already read dynamically under
// js.mu, so a post-construction setter is race-safe), publish a new
// pseudo-version, bump go.mod, and call
// `rt.adapter.JS().SetUnhandledRejection(<slog.Error handler>)` in runtime.go
// INSIDE the loop.Submit closure AFTER rt.adapter.Bind() (runtime.go:138) and
// BEFORE errCh<-nil — on-loop, before any user JS. (A local `replace` is
// rejected because it breaks make-all-in-container / make-all-run-windows,
// which copy only this repo.)
//
// This test is SKIPPED (tracked, not silent) until the fix lands. When it
// does, remove the skip and the assertion below validates end-to-end.
func TestUnhandledRejection_Observability(t *testing.T) {
	t.Skip("TODO(osm): RISK-A — wire WithUnhandledRejection via go-eventloop SetUnhandledRejection setter (see comment); not silent, tracked here")

	ctx := context.Background()
	engine, _, stderr := newComplianceEngine(t, ctx)

	// Create a rejected promise with NO handler (unhandled), then run an
	// unrelated settled promise to force a loop turn (so the unhandled-rejection
	// check runs).
	if _, err := evalJS(t, engine, `(function(){ Promise.reject(new Error('risk-a-marker')); return Promise.resolve('alive'); })()`, defaultEvalTimeout); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// When the handler is wired to slog.Error, the marker must surface in the
	// engine's stderr (the startup/slog sink). Until then this test is skipped.
	if !containsStderr(stderr, "risk-a-marker") {
		t.Errorf("RISK-A: unhandled rejection was not observable (expected 'risk-a-marker' in logs once WithUnhandledRejection is wired)")
	}
}

// containsStderr reports whether want appears in b.String() (a real substring
// check so the skipped RISK-A assertion is valid once activated).
func containsStderr(b interface{ String() string }, want string) bool {
	return b != nil && strings.Contains(b.String(), want)
}
