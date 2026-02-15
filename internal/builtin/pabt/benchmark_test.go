package pabt

import (
	"testing"

	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"

	bt "github.com/joeycumines/go-behaviortree"
	pabtpkg "github.com/joeycumines/go-pabt"
)

// ============================================================================
// PABT EVALUATION AND PLANNING BENCHMARKS
// ============================================================================
//
// These benchmarks measure condition evaluation performance (ExprLang vs
// GoFunction) and plan creation cost for the PA-BT planning algorithm.
//
// Run with: go test -bench=. -benchmem ./internal/builtin/pabt/
//
// ============================================================================
// PROFILING NOTES (pprof CPU, 2s benchtime, Apple M2 Pro, 2026-02-15)
// ============================================================================
//
// Condition evaluation (cached, per-call):
//   - ExprLang SimpleEquality:  ~90ns/op,  48B, 2 allocs (expr/vm.(*VM).Run)
//   - ExprLang Comparison:      ~88ns/op,  48B, 2 allocs
//   - ExprLang FieldAccess:    ~262ns/op,  64B, 3 allocs (reflect boxing)
//   - ExprLang StringEquality: ~128ns/op,  48B, 2 allocs
//   - ExprLang NilCheck:       ~100ns/op,  48B, 2 allocs
//   - GoFunction (all types):   ~1-13ns/op, 0-16B, 0-1 allocs
//
// ExprLang is 20-85x slower than GoFunction but still sub-microsecond.
// The 2 allocs per ExprLang evaluation are in reflect.packEface (interface
// boxing for the value→env map) and expr/vm.(*VM) internal operations.
//
// Expression compilation:
//   - FirstCompile:  ~8.4µs/op, 11.3KB, 71 allocs
//   - CachedLookup: ~157ns/op,   144B,  3 allocs (LRU hit)
//   - Cache provides 53x speedup for repeated expressions.
//
// pprof analysis:
//   - ExprCondition.Match: ~12% of profiled CPU
//   - expr/vm.(*VM).Run: ~5.3% (the expr-lang evaluator)
//   - FuncCondition.Match: ~15.6% (inlined, just the Go function)
//   - Remaining CPU in test harness, sync primitives, reflect
//
// Conclusion: No optimization targets in our code. ExprLang overhead is
// entirely in the third-party expr-lang/expr library. GoFunction alternative
// is available for performance-critical condition paths. Expression caching
// is working correctly (53x speedup). Plan creation is sub-5µs even with
// multiple candidate actions — delegates to go-pabt library.
//
// Planning:
//   - SimplePlan (1 goal, 1 action):  ~1.7µs/op, 2.5KB, 37 allocs
//   - MultiActionPlan (1 goal, 5 actions): ~4.1µs/op, 4.6KB, 77 allocs
//   - Plan creation cost scales linearly with candidate actions.
// ============================================================================

// BenchmarkEvaluation_SimpleEquality compares evaluation performance
// for a simple equality check: value == 42
func BenchmarkEvaluation_SimpleEquality(b *testing.B) {
	ClearExprCache()

	// Setup expr-lang condition
	exprCond := NewExprCondition("key", "value == 42")
	// Pre-compile by running once
	exprCond.Match(42)

	// Setup pure Go function condition
	funcCond := NewFuncCondition("key", func(v any) bool {
		return v == 42
	})

	b.Run("ExprLang", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			exprCond.Match(42)
		}
	})

	b.Run("GoFunction", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			funcCond.Match(42)
		}
	})
}

// BenchmarkEvaluation_Comparison compares evaluation performance
// for a comparison: value > 10
func BenchmarkEvaluation_Comparison(b *testing.B) {
	ClearExprCache()

	exprCond := NewExprCondition("key", "value > 10")
	exprCond.Match(15) // Pre-compile

	funcCond := NewFuncCondition("key", func(v any) bool {
		if n, ok := v.(int); ok {
			return n > 10
		}
		return false
	})

	b.Run("ExprLang", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			exprCond.Match(15)
		}
	})

	b.Run("GoFunction", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			funcCond.Match(15)
		}
	})
}

// BenchmarkEvaluation_FieldAccess compares evaluation for field access:
// value.X > 0 && value.Y > 0
func BenchmarkEvaluation_FieldAccess(b *testing.B) {
	ClearExprCache()

	type Point struct {
		X int
		Y int
	}

	exprCond := NewExprCondition("key", "value.X > 0 && Value.Y > 0")
	testPoint := Point{X: 5, Y: 10}
	exprCond.Match(testPoint) // Pre-compile

	funcCond := NewFuncCondition("key", func(v any) bool {
		if p, ok := v.(Point); ok {
			return p.X > 0 && p.Y > 0
		}
		return false
	})

	b.Run("ExprLang", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			exprCond.Match(testPoint)
		}
	})

	b.Run("GoFunction", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			funcCond.Match(testPoint)
		}
	})
}

// BenchmarkEvaluation_StringEquality compares string equality checks
func BenchmarkEvaluation_StringEquality(b *testing.B) {
	ClearExprCache()

	exprCond := NewExprCondition("key", `Value == "hello"`)
	exprCond.Match("hello") // Pre-compile

	funcCond := NewFuncCondition("key", func(v any) bool {
		return v == "hello"
	})

	b.Run("ExprLang", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			exprCond.Match("hello")
		}
	})

	b.Run("GoFunction", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			funcCond.Match("hello")
		}
	})
}

// BenchmarkEvaluation_NilCheck compares nil check performance
func BenchmarkEvaluation_NilCheck(b *testing.B) {
	ClearExprCache()

	exprCond := NewExprCondition("key", "value != nil")
	exprCond.Match("something") // Pre-compile

	funcCond := NewFuncCondition("key", func(v any) bool {
		return v != nil
	})

	b.Run("ExprLang", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			exprCond.Match("something")
		}
	})

	b.Run("GoFunction", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			funcCond.Match("something")
		}
	})
}

// BenchmarkEvaluation_CompileTime measures expression compilation time
// which should only occur once per unique expression
func BenchmarkEvaluation_CompileTime(b *testing.B) {
	b.Run("ExprLang_FirstCompile", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ClearExprCache() // Force recompilation
			cond := NewExprCondition("key", "value > 0")
			cond.Match(1)
		}
	})

	b.Run("ExprLang_CachedLookup", func(b *testing.B) {
		ClearExprCache()
		// Pre-populate cache
		cond := NewExprCondition("key", "value > 0")
		cond.Match(1)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// New condition with same expression uses cache
			cond2 := NewExprCondition("key2", "value > 0")
			cond2.Match(1)
		}
	})
}

// BenchmarkSimple_ActionConditions benchmarks action condition evaluation
func BenchmarkSimple_ActionConditions(b *testing.B) {
	// Create a simple action with multiple conditions
	cond1 := NewSimpleCond("key1", func(v any) bool { return v == 1 })
	cond2 := NewSimpleCond("key2", func(v any) bool { return v == 2 })
	cond3 := NewSimpleCond("key3", func(v any) bool { return v == 3 })

	action := NewActionBuilder().
		WithConditions(cond1, cond2, cond3).
		Build()

	b.Run("Action_Conditions_Access", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = action.Conditions()
		}
	})
}

// BenchmarkPlanCreation measures the cost of creating a PA-BT plan.
// This benchmarks the pabtpkg.INew call which constructs the plan tree
// by searching for actions that satisfy goal conditions.
func BenchmarkPlanCreation(b *testing.B) {
	b.Run("SimplePlan", func(b *testing.B) {
		// Plan with 1 goal and 1 action — minimal planning
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bb := new(btmod.Blackboard)
			bb.Set("atTarget", false)
			state := NewState(bb)

			action := &Action{
				Name: "move",
				conditions: []pabtpkg.IConditions{
					{NewSimpleCond("ready", func(v any) bool { return v == true })},
				},
				effects: []pabtpkg.Effect{
					&SimpleEffect{key: "atTarget", value: true},
				},
				node: bt.New(func([]bt.Node) (bt.Status, error) {
					return bt.Success, nil
				}),
			}
			state.RegisterAction("move", action)

			goal := []pabtpkg.IConditions{
				{NewSimpleCond("atTarget", func(v any) bool { return v == true })},
			}

			plan, err := pabtpkg.INew(state, goal)
			if err != nil {
				b.Fatal(err)
			}
			_ = plan
		}
	})

	b.Run("MultiActionPlan", func(b *testing.B) {
		// Plan with 1 goal and 5 candidate actions — more realistic planning
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bb := new(btmod.Blackboard)
			bb.Set("x", 0)
			bb.Set("y", 0)
			bb.Set("atTarget", false)
			state := NewState(bb)

			for _, name := range []string{"moveRight", "moveLeft", "moveUp", "moveDown", "wait"} {
				action := &Action{
					Name: name,
					conditions: []pabtpkg.IConditions{
						{NewSimpleCond("ready", func(v any) bool { return v == true })},
					},
					effects: []pabtpkg.Effect{
						&SimpleEffect{key: "atTarget", value: true},
					},
					node: bt.New(func([]bt.Node) (bt.Status, error) {
						return bt.Success, nil
					}),
				}
				state.RegisterAction(name, action)
			}

			goal := []pabtpkg.IConditions{
				{NewSimpleCond("atTarget", func(v any) bool { return v == true })},
			}

			plan, err := pabtpkg.INew(state, goal)
			if err != nil {
				b.Fatal(err)
			}
			_ = plan
		}
	})
}
