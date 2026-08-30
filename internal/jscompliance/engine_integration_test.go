package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestEngine_Integration is the integration target (first executable
// compliance test). It spins the REAL production engine — the same NewEngine
// path the osm binary uses — and proves the harness faithfully drives it
// across globals, several osm:* modules, and a Promise round-trip, asserting
// VALUE correctness (not just presence). A deliberate regression (un-register
// a module, break an export) must fail this test.
//
// This anchors the whole suite: every later spec trusts that this engine
// wiring is sound.
func TestEngine_Integration(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, stdout, _ := newComplianceEngine(t, ctx)

	// output.print routes to stdout; assert content (routing, not just presence).
	if _, err := evalJS(t, engine, `output.print("integration-marker-7")`, defaultEvalTimeout); err != nil {
		t.Fatalf("output.print: %v", err)
	}
	if !strings.Contains(stdout.String(), "integration-marker-7") {
		t.Errorf("output.print did not reach stdout; got %q", stdout.String())
	}

	// Pure modules return correct VALUES (the json.query gut-check, etc.).
	// Modules are NOT globals (the bare `crypto` global is the adapter's
	// WHATWG crypto, which lacks sha256) — they must be require'd.
	cases := map[string]string{
		`require('osm:crypto').sha256('abc')`:            `"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"`,
		`require('osm:json').query({a:{b:1}}, 'a.b')`:    `1`,
		`require('osm:regexp').match('^f(o)+$', 'fooo')`: `true`,
		`require('osm:encoding').hexEncode('A')`:         `"41"`,
		`require('osm:format').formatBytes(2048)`:        `"2.0 kB"`,
	}
	for js, wantExpr := range cases {
		got, err := evalJS(t, engine, js, defaultEvalTimeout)
		if err != nil {
			t.Errorf("%s: %v", js, err)
			continue
		}
		want, err := evalJS(t, engine, wantExpr, defaultEvalTimeout)
		if err != nil {
			t.Fatalf("want-expr %s failed: %v", wantExpr, err)
		}
		if !equalJS(got, want) {
			t.Errorf("%s = %v (%T), want %v", js, got, got, want)
		}
	}

	// A real osm:os async export returns a Promise (binding contract).
	isPromise, err := evalJS(t, engine, `(function(){ var p = require('osm:os').readFile('/nonexistent-jscompliance'); return (p !== null && p !== undefined && typeof p === 'object' && typeof p.then === 'function'); })()`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("readFile promise check: %v", err)
	}
	if b, ok := isPromise.(bool); !ok || !b {
		t.Errorf("osm:os.readFile must return a Promise; got %v (%T)", isPromise, isPromise)
	}

	// A Promise round-trip through the loop (await mechanic end-to-end).
	v, err := evalJS(t, engine, `await new Promise(function(r){ setTimeout(function(){ r('loop-ok'); }, 5); })`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("promise round-trip: %v", err)
	}
	if s, ok := v.(string); !ok || s != "loop-ok" {
		t.Errorf("promise round-trip = %v, want loop-ok", v)
	}

	// The spec runner works end-to-end against a real module.
	runSpecSource(t, engine, "integration-spec", `
		var json = require('osm:json');
		test('json.parse round-trips a value', function() {
			var v = json.parse('{"a":1,"b":[2,3]}');
			assert.deepEqual('json.parse', v, {a:1, b:[2,3]});
		});
		test('json.query returns the value at a dot path', function() {
			assert.equal('json.query', json.query({a:{b:7}}, 'a.b'), 7);
		});
	`, defaultEvalTimeout)
}

// equalJS compares two goja-exported values tolerantly (numbers by value).
func equalJS(a, b any) bool {
	if a == b {
		return true
	}
	if an, ok := asNumberOrNil(a); ok {
		if bn, ok2 := asNumberOrNil(b); ok2 {
			return an == bn
		}
	}
	return false
}

// asNumberOrNil is like asNumber but returns ok=false for non-numbers without
// fataling (for use in comparisons).
func asNumberOrNil(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case uint64:
		return float64(n), true
	}
	return 0, false
}
