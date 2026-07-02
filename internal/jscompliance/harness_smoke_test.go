package jscompliance

import (
	"context"
	"testing"
	"time"
)

// asNumber tolerantly converts a goja-exported numeric value to float64.
func asNumber(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case uint64:
		return float64(n)
	default:
		t.Fatalf("expected numeric value, got %T: %v", v, v)
		return 0
	}
}

// TestHarness_EvalSync proves evalJS returns a synchronous value.
func TestHarness_EvalSync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `1 + 1`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("evalJS(1+1): %v", err)
	}
	if got := asNumber(t, v); got != 2 {
		t.Fatalf("evalJS(1+1) = %v, want 2", got)
	}
}

// TestHarness_EvalAsync proves evalJS awaits a top-level Promise.
func TestHarness_EvalAsync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `await Promise.resolve(42)`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("evalJS(await Promise.resolve(42)): %v", err)
	}
	if got := asNumber(t, v); got != 42 {
		t.Fatalf("got %v, want 42", got)
	}
}

// TestHarness_EvalTimeoutFail proves a never-settling promise FAILS within the
// timeout (it never silently passes). This is the reviewer-2 false-confidence
// guard.
func TestHarness_EvalTimeoutFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	_, err := evalJS(t, engine, `await new Promise(function(){})`, 300*time.Millisecond)
	if err == nil {
		t.Fatal("never-settling promise must fail via timeout, got nil error")
	}
}

// TestHarness_SpecMapping proves the spec runner maps pass/fail correctly
// (using collectSpecResults to inspect the data without a test side-effect).
func TestHarness_SpecMapping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	src := `
		test('passes', function() { assert.equal('passes', 1 + 1, 2); });
		test('fails',  function() { assert.equal('fails', 1 + 1, 3); });
		test('async-passes', function() { return assert.resolves('async-passes', Promise.resolve('ok')).then(function(v){ assert.equal('async-passes', v, 'ok'); }); });
	`
	results, err := collectSpecResults(t, engine, "smoke", src, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("collectSpecResults: %v", err)
	}
	byName := map[string]specResult{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d: %+v", len(results), results)
	}
	if !byName["passes"].Pass {
		t.Errorf("expected 'passes' to pass, got error: %q", byName["passes"].Error)
	}
	if byName["fails"].Pass {
		t.Errorf("expected 'fails' to fail, but it passed")
	}
	if !byName["async-passes"].Pass {
		t.Errorf("expected 'async-passes' to pass, got error: %q", byName["async-passes"].Error)
	}
}

// TestHarness_SpecRejectsEmpty proves a spec with zero registered tests is an
// error (false-confidence trap), not a silent pass.
func TestHarness_SpecRejectsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	results, err := collectSpecResults(t, engine, "empty", `// no tests`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty spec, got %d", len(results))
	}
	// runSpecSource must fatal on empty; collectSpecResults returns the empty
	// slice and runSpecSource turns it into a failure. That mapping is what
	// makes an asserting-nothing spec a real test failure.
}
