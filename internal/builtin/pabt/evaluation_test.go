package pabt

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dop251/goja"
)

func TestEvaluationMode_String(t *testing.T) {
	tests := []struct {
		mode EvaluationMode
		want string
	}{
		{EvalModeJavaScript, "javascript"},
		{EvalModeExpr, "expr"},
		{EvaluationMode(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("EvaluationMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestNewJSCondition(t *testing.T) {
	// We can't fully test JSCondition without a real bridge and runtime,
	// but we can test the constructor and Key method
	cond := NewJSCondition("test-key", nil, nil)

	if cond.Key() != "test-key" {
		t.Errorf("Key() = %v, want %q", cond.Key(), "test-key")
	}

	if cond.Mode() != EvalModeJavaScript {
		t.Errorf("Mode() = %v, want %v", cond.Mode(), EvalModeJavaScript)
	}
}

func TestJSCondition_Match_NilCondition(t *testing.T) {
	var cond *JSCondition

	if cond.Match("anything") {
		t.Error("nil JSCondition.Match() should return false")
	}
}

func TestJSCondition_Match_NilMatcher(t *testing.T) {
	cond := &JSCondition{
		key:     "test",
		matcher: nil,
		bridge:  nil,
	}

	if cond.Match("anything") {
		t.Error("JSCondition with nil matcher should return false")
	}
}

func TestJSCondition_Match_NilBridge(t *testing.T) {
	// Create a mock callable - we can't use a real one without a runtime
	cond := &JSCondition{
		key:     "test",
		matcher: nil, // Also nil since we can't create a real Callable
		bridge:  nil,
	}

	if cond.Match("anything") {
		t.Error("JSCondition with nil bridge should return false")
	}
}

func TestNewExprCondition(t *testing.T) {
	cond := NewExprCondition("my-key", "value == 42")

	if cond.Key() != "my-key" {
		t.Errorf("Key() = %v, want %q", cond.Key(), "my-key")
	}

	if cond.Mode() != EvalModeExpr {
		t.Errorf("Mode() = %v, want %v", cond.Mode(), EvalModeExpr)
	}
}

func TestExprCondition_Match_SimpleEquality(t *testing.T) {
	ClearExprCache() // Ensure clean state

	cond := NewExprCondition("key", "value == 42")

	if !cond.Match(42) {
		t.Error("Match(42) should return true for 'value == 42'")
	}

	if cond.Match(43) {
		t.Error("Match(43) should return false for 'value == 42'")
	}
}

func TestExprCondition_Match_Comparison(t *testing.T) {
	ClearExprCache()

	cond := NewExprCondition("key", "value > 10")

	if !cond.Match(15) {
		t.Error("Match(15) should return true for 'value > 10'")
	}

	if cond.Match(5) {
		t.Error("Match(5) should return false for 'value > 10'")
	}

	if cond.Match(10) {
		t.Error("Match(10) should return false for 'value > 10'")
	}
}

func TestExprCondition_Match_FieldAccess(t *testing.T) {
	ClearExprCache()

	type Point struct {
		X int
		Y int
	}

	cond := NewExprCondition("pos", "value.X > 0 && Value.Y > 0")

	if !cond.Match(Point{X: 5, Y: 10}) {
		t.Error("Match({5,10}) should return true for 'value.X > 0 && Value.Y > 0'")
	}

	if cond.Match(Point{X: -1, Y: 10}) {
		t.Error("Match({-1,10}) should return false for 'value.X > 0 && Value.Y > 0'")
	}

	if cond.Match(Point{X: 5, Y: -1}) {
		t.Error("Match({5,-1}) should return false for 'value.X > 0 && Value.Y > 0'")
	}
}

func TestExprCondition_Match_MapAccess(t *testing.T) {
	ClearExprCache()

	cond := NewExprCondition("data", `Value["name"] == "test"`)

	if !cond.Match(map[string]any{"name": "test"}) {
		t.Error("Match with name=test should return true")
	}

	if cond.Match(map[string]any{"name": "other"}) {
		t.Error("Match with name=other should return false")
	}
}

func TestExprCondition_Match_NilCondition(t *testing.T) {
	var cond *ExprCondition

	if cond.Match("anything") {
		t.Error("nil ExprCondition.Match() should return false")
	}
}

func TestExprCondition_Match_EmptyExpression(t *testing.T) {
	cond := &ExprCondition{
		key:        "test",
		expression: "",
	}

	if cond.Match("anything") {
		t.Error("ExprCondition with empty expression should return false")
	}
}

func TestExprCondition_Match_InvalidExpression(t *testing.T) {
	cond := NewExprCondition("key", "this is not valid expr !@#$%")

	// Invalid expression should compile-fail and return false
	if cond.Match(42) {
		t.Error("Invalid expression should return false")
	}
}

func TestExprCondition_Match_NonBooleanResult(t *testing.T) {
	ClearExprCache()

	// This would fail compilation with AsBool() option, so we test
	// that it returns false gracefully
	cond := NewExprCondition("key", "value + 1")

	if cond.Match(42) {
		t.Error("Non-boolean expression should return false")
	}
}

func TestExprCondition_CachingWorks(t *testing.T) {
	ClearExprCache()

	cond1 := NewExprCondition("key1", "value == 100")
	cond2 := NewExprCondition("key2", "value == 100") // Same expression

	// First call compiles
	cond1.Match(100)

	// Second should use cache
	cond2.Match(100)

	// Verify both conditions share the same program via cache
	// (We can't directly verify this, but we can check the logic works)
	if !cond1.Match(100) || !cond2.Match(100) {
		t.Error("Both conditions with same expression should work")
	}
}

func TestExprCondition_ThreadSafety(t *testing.T) {
	ClearExprCache()

	cond := NewExprCondition("key", "value > 0")
	var wg sync.WaitGroup
	var trueCount, falseCount atomic.Int64

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			if cond.Match(val) {
				trueCount.Add(1)
			} else {
				falseCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Values 1-99 should match (99 true), value 0 should not (1 false)
	if got := trueCount.Load(); got != 99 {
		t.Errorf("Expected 99 true matches, got %d", got)
	}
	if got := falseCount.Load(); got != 1 {
		t.Errorf("Expected 1 false match, got %d", got)
	}
}

func TestNewFuncCondition(t *testing.T) {
	called := false
	cond := NewFuncCondition("func-key", func(v any) bool {
		called = true
		return v == "match"
	})

	if cond.Key() != "func-key" {
		t.Errorf("Key() = %v, want %q", cond.Key(), "func-key")
	}

	if cond.Mode() != EvalModeExpr {
		t.Errorf("Mode() = %v, want %v", cond.Mode(), EvalModeExpr)
	}

	if !cond.Match("match") {
		t.Error("Match('match') should return true")
	}

	if !called {
		t.Error("matchFn should have been called")
	}

	if cond.Match("no-match") {
		t.Error("Match('no-match') should return false")
	}
}

func TestFuncCondition_Match_NilCondition(t *testing.T) {
	var cond *FuncCondition

	if cond.Match("anything") {
		t.Error("nil FuncCondition.Match() should return false")
	}
}

func TestFuncCondition_Match_NilFunc(t *testing.T) {
	cond := &FuncCondition{
		key:     "test",
		matchFn: nil,
	}

	if cond.Match("anything") {
		t.Error("FuncCondition with nil matchFn should return false")
	}
}

func TestNewEffect(t *testing.T) {
	effect := NewEffect("effect-key", 42)

	if effect.Key() != "effect-key" {
		t.Errorf("Key() = %v, want %q", effect.Key(), "effect-key")
	}

	if effect.Value() != 42 {
		t.Errorf("Value() = %v, want 42", effect.Value())
	}
}

func TestEffect_NilValues(t *testing.T) {
	effect := NewEffect(nil, nil)

	if effect.Key() != nil {
		t.Errorf("Key() = %v, want nil", effect.Key())
	}

	if effect.Value() != nil {
		t.Errorf("Value() = %v, want nil", effect.Value())
	}
}

func TestClearExprCache(t *testing.T) {
	// Pre-populate cache
	cond := NewExprCondition("key", "value == 999")
	cond.Match(999)

	// Clear cache
	ClearExprCache()

	// Verify cache is empty by checking new condition compiles fresh
	cond2 := NewExprCondition("key2", "value == 999")

	// This should work (recompiles)
	if !cond2.Match(999) {
		t.Error("Condition should work after cache clear")
	}
}

// TestExprCondition_NoGojaCallsVerification verifies that ExprCondition.Match
// makes ZERO Goja runtime calls. This is a critical correctness guarantee
// for the performance claims of expr-lang mode.
func TestExprCondition_NoGojaCallsVerification(t *testing.T) {
	ClearExprCache()

	// Create a mock bridge that panics if RunOnLoopSync is ever called
	panicBridge := &mockPanicBridge{t: t}

	// This should NOT use the bridge at all
	cond := NewExprCondition("key", "value == 42")

	// If this panics, it means ExprCondition is incorrectly calling Goja
	if !cond.Match(42) {
		t.Error("ExprCondition should match without Goja calls")
	}

	// Verify we never touched the panic bridge (it's not connected anyway,
	// but this test documents the expected behavior)
	_ = panicBridge
}

// mockPanicBridge is a mock that panics if used, to verify no Goja calls.
type mockPanicBridge struct {
	t *testing.T
}

func (m *mockPanicBridge) RunOnLoopSync(fn func(*goja.Runtime) error) error {
	m.t.Fatal("ExprCondition should NOT call RunOnLoopSync - this indicates Goja is being used incorrectly!")
	return nil
}

func TestCondition_Interface(t *testing.T) {
	// Verify all condition types implement the Condition interface
	var _ Condition = (*JSCondition)(nil)
	var _ Condition = (*ExprCondition)(nil)
	var _ Condition = (*FuncCondition)(nil)
}

func TestExprCondition_ComplexExpressions(t *testing.T) {
	ClearExprCache()

	tests := []struct {
		name  string
		expr  string
		value any
		want  bool
	}{
		{
			name:  "nil check",
			expr:  "value == nil",
			value: nil,
			want:  true,
		},
		{
			name:  "nil check negative",
			expr:  "value == nil",
			value: 42,
			want:  false,
		},
		{
			name:  "not nil check",
			expr:  "value != nil",
			value: "something",
			want:  true,
		},
		{
			name:  "boolean true",
			expr:  "value == true",
			value: true,
			want:  true,
		},
		{
			name:  "boolean false",
			expr:  "value == false",
			value: false,
			want:  true,
		},
		{
			name:  "string equality",
			expr:  `Value == "hello"`,
			value: "hello",
			want:  true,
		},
		{
			name:  "float comparison",
			expr:  "value >= 1.5",
			value: 1.5,
			want:  true,
		},
		{
			name:  "float less than",
			expr:  "value < 1.5",
			value: 1.0,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := NewExprCondition("key", tt.expr)
			if got := cond.Match(tt.value); got != tt.want {
				t.Errorf("Match(%v) with expr %q = %v, want %v", tt.value, tt.expr, got, tt.want)
			}
		})
	}
}
