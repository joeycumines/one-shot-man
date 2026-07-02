package jscompliance

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// returnsPromise evaluates js (which must call an I/O export) and asserts the
// result is a Promise (the JS Binding Contract: I/O bindings MUST be async).
func returnsPromise(t *testing.T, engine *scripting.Engine, js string) {
	t.Helper()
	v, err := evalJS(t, engine, `(function(){ var p = `+js+`; return (p !== null && p !== undefined && typeof p === 'object' && typeof p.then === 'function'); })()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("%s: %v", js, err)
	}
	if b, ok := v.(bool); !ok || !b {
		t.Errorf("BINDING CONTRACT: %s did NOT return a Promise (I/O must be async)", js)
	}
}

// TestBindingContract_IOExportsAreAsync asserts the documented I/O exports
// return Promises (not sync values) — the core JS Binding Contract (CLAUDE.md).
// Targets are chosen to reject fast so the tier stays quick and side-effect-free.
func TestBindingContract_IOExportsAreAsync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	nope := filepath.Join(t.TempDir(), "does-not-exist")

	returnsPromise(t, engine, `require('osm:os').readFile(`+jsStringLit(nope)+`)`)
	returnsPromise(t, engine, `require('osm:exec').execv(['this-binary-does-not-exist-xyz'])`)
	returnsPromise(t, engine, `require('osm:path').glob(`+jsStringLit(filepath.Join(t.TempDir(), "*.nosuchext"))+`)`)
}

// TestBindingContract_LoopLivenessDuringIO asserts the event loop is NOT
// monopolized by an async I/O op's background goroutine: a setTimeout callback
// fires WHILE a long async exec is pending. If exec.execv blocked the loop
// synchronously, the timer could not fire until it returned — failing this.
//
// Uses `sleep 1` (unix) as a reliably-slow async op; the event-loop machinery
// is platform-agnostic, so unix coverage (macOS + Linux gates) is sufficient
// evidence. skipSlow (1s exec). Skipped on Windows + short mode.
func TestBindingContract_LoopLivenessDuringIO(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS == "windows" {
		t.Skip("liveness-via-sleep uses unix `sleep`; loop machinery is platform-agnostic")
	}
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)

	v, err := evalJS(t, engine, `(function(){
		return new Promise(function(resolve){
			var timerFired = false;
			require('osm:exec').execv(['sleep','1']).then(function(){ resolve(timerFired); });
			setTimeout(function(){ timerFired = true; }, 100);
		});
	})()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("liveness probe failed: %v", err)
	}
	if b, ok := v.(bool); !ok || !b {
		t.Errorf("BINDING CONTRACT: event loop was monopolized — 100ms setTimeout did not fire during the 1s async exec")
	}
}

