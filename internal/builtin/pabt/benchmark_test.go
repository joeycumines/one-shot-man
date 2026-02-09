package pabt

import (
	"testing"
)

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
