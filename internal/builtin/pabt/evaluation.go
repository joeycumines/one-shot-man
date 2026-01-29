package pabt

import (
	"sync"

	"github.com/dop251/goja"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// EvaluationMode specifies how conditions are evaluated at runtime.
// This is a critical performance decision: JavaScript evaluation requires
// thread-safe marshalling via Bridge.RunOnLoopSync, while expr-lang
// evaluation runs natively in Go with zero Goja overhead.
type EvaluationMode int

const (
	// EvalModeJavaScript uses Goja JavaScript runtime for condition evaluation.
	// This mode requires a Bridge for thread-safe runtime access and incurs
	// the overhead of Go→JS→Go value conversion on every match call.
	// Use when conditions require complex JavaScript logic or closure state.
	EvalModeJavaScript EvaluationMode = iota

	// EvalModeExpr uses expr-lang Go-native expression evaluation.
	// This mode compiles expressions to bytecode and runs natively in Go,
	// with zero Goja calls. Compiled programs are cached for performance.
	// Use for simple conditions (comparisons, equality checks) where
	// 10-100x performance improvement is expected.
	EvalModeExpr
)

// String returns the string representation of the evaluation mode.
func (m EvaluationMode) String() string {
	switch m {
	case EvalModeJavaScript:
		return "javascript"
	case EvalModeExpr:
		return "expr"
	default:
		return "unknown"
	}
}

// Condition is an interface that extends pabtpkg.Condition with mode information.
// This allows runtime switching between evaluation modes while maintaining
// compatibility with the go-pabt Condition interface.
type Condition interface {
	pabtpkg.Condition

	// Mode returns the evaluation mode used by this condition.
	Mode() EvaluationMode
}

// JSCondition implements pabtpkg.Condition using JavaScript match function.
// This condition is evaluated via Goja runtime with thread-safe bridge access.
//
// The jsObject field stores the original JavaScript condition object, preserving
// all its properties (including .value) so the action generator can access them
// directly - equivalent to Go's type assertion for accessing internal state.
type JSCondition struct {
	key      any
	matcher  goja.Callable
	bridge   *btmod.Bridge // Required for thread-safe goja access from ticker goroutine
	jsObject *goja.Object  // Original JS object for passthrough to action generator
}

var _ Condition = (*JSCondition)(nil)

// NewJSCondition creates a new JavaScript-based condition.
// The matcher function is called via Bridge.RunOnLoopSync for thread safety.
func NewJSCondition(key any, matcher goja.Callable, bridge *btmod.Bridge) *JSCondition {
	return &JSCondition{
		key:     key,
		matcher: matcher,
		bridge:  bridge,
	}
}

// Key implements pabtpkg.Variable.Key().
func (c *JSCondition) Key() any {
	return c.key
}

// Match implements pabtpkg.Condition.Match(value any) bool.
// It calls the JavaScript matcher function via Bridge.RunOnLoopSync.
//
// CRITICAL: This method is called from the bt.Ticker goroutine, but goja.Runtime
// is NOT thread-safe. We MUST use Bridge.RunOnLoopSync to marshal the call to
// the event loop goroutine where goja operations are safe.
//
// IMPORTANT: If the bridge is stopping, we return false immediately to avoid
// blocking on RunOnLoopSync. The bridge's Done() channel is closed when stopping,
// so RunOnLoopSync would return an error anyway, but early exit improves shutdown
// responsiveness.
func (c *JSCondition) Match(value any) bool {
	// Defensive: check if condition is valid before calling matcher
	if c == nil || c.matcher == nil || c.bridge == nil {
		return false
	}

	// Early exit if bridge is stopping - avoids blocking in RunOnLoopSync
	if !c.bridge.IsRunning() {
		return false
	}

	var result bool
	err := c.bridge.RunOnLoopSync(func(vm *goja.Runtime) error {
		res, callErr := c.matcher(goja.Undefined(), vm.ToValue(value))
		if callErr != nil {
			return callErr
		}
		result = res.ToBoolean()
		return nil
	})

	// On error (including event loop not running), return false
	return err == nil && result
}

// Mode returns EvalModeJavaScript.
func (c *JSCondition) Mode() EvaluationMode {
	return EvalModeJavaScript
}

// JSEffect implements pabtpkg.Effect interface.
// This is the JavaScript-compatible effect that can be created from JS code
// and consumed by the Go pabt planning system.
type JSEffect struct {
	key   any
	value any
}

var _ pabtpkg.Effect = (*JSEffect)(nil)

// NewJSEffect creates a new JavaScript-compatible effect.
func NewJSEffect(key, value any) *JSEffect {
	return &JSEffect{
		key:   key,
		value: value,
	}
}

// Key implements pabtpkg.Variable.Key().
func (e *JSEffect) Key() any {
	return e.key
}

// Value implements pabtpkg.Effect.Value().
func (e *JSEffect) Value() any {
	return e.value
}

// exprCache caches compiled expr-lang programs for performance.
// The cache is keyed by the expression string for fast lookup.
var exprCache sync.Map // map[string]*vm.Program

// ExprCondition implements pabtpkg.Condition using expr-lang Go-native evaluation.
// This condition is evaluated directly in Go with zero Goja calls.
type ExprCondition struct {
	key        any
	expression string
	program    *vm.Program // Cached compiled program (nil until first use)
	mu         sync.RWMutex
}

var _ Condition = (*ExprCondition)(nil)

// NewExprCondition creates a new expr-lang based condition.
// The expression is compiled lazily on first Match call and cached globally.
//
// Expression syntax follows expr-lang (github.com/expr-lang/expr):
//   - Field access: Value.x, Value.name
//   - Comparisons: Value == 10, Value > 5, Value != nil
//   - Boolean logic: Value.x > 0 && Value.y < 100
//   - String matching: Value.name contains "test"
//   - Collection operations: all(Value.items, {.active})
//
// The environment provides a single "Value" variable containing the
// condition input value.
func NewExprCondition(key any, expression string) *ExprCondition {
	return &ExprCondition{
		key:        key,
		expression: expression,
	}
}

// Key implements pabtpkg.Variable.Key().
func (c *ExprCondition) Key() any {
	return c.key
}

// ExprEnv is the environment struct for expr-lang evaluation.
// It provides the condition input value as "Value".
type ExprEnv struct {
	Value any
}

// Match implements pabtpkg.Condition.Match(value any) bool.
// It compiles the expression (if not cached) and runs it natively in Go.
//
// CRITICAL: This method makes ZERO Goja calls. All evaluation is pure Go.
// This provides 10-100x performance improvement over JavaScript evaluation
// for equivalent conditions.
func (c *ExprCondition) Match(value any) bool {
	if c == nil || c.expression == "" {
		return false
	}

	program, err := c.getOrCompileProgram()
	if err != nil {
		return false
	}

	// Create environment with the value
	env := ExprEnv{Value: value}

	// Run the compiled program
	result, err := expr.Run(program, env)
	if err != nil {
		return false
	}

	// Convert result to boolean
	if b, ok := result.(bool); ok {
		return b
	}
	return false
}

// getOrCompileProgram returns the cached compiled program or compiles it.
func (c *ExprCondition) getOrCompileProgram() (*vm.Program, error) {
	// Fast path: check if already compiled for this instance
	c.mu.RLock()
	if c.program != nil {
		prog := c.program
		c.mu.RUnlock()
		return prog, nil
	}
	c.mu.RUnlock()

	// Check global cache
	if cached, ok := exprCache.Load(c.expression); ok {
		program := cached.(*vm.Program)
		// Double-check locking: another goroutine may have set it already
		c.mu.Lock()
		if c.program == nil {
			c.program = program
		}
		c.mu.Unlock()
		return program, nil
	}

	// Compile the expression
	program, err := expr.Compile(c.expression,
		expr.Env(ExprEnv{}),
		expr.AsBool(),
		expr.AllowUndefinedVariables(),
	)
	if err != nil {
		return nil, err
	}

	// Store in global cache and instance
	exprCache.Store(c.expression, program)
	// Double-check locking: another goroutine may have set it already
	c.mu.Lock()
	if c.program == nil {
		c.program = program
	}
	c.mu.Unlock()

	return program, nil
}

// Mode returns EvalModeExpr.
func (c *ExprCondition) Mode() EvaluationMode {
	return EvalModeExpr
}

// FuncCondition implements pabtpkg.Condition using a Go function.
// This is useful for conditions that need direct Go logic without
// JavaScript or expression compilation overhead.
type FuncCondition struct {
	key     any
	matchFn func(any) bool
}

var _ Condition = (*FuncCondition)(nil)

// NewFuncCondition creates a new function-based condition.
// The matchFn is called directly on Match without any overhead.
func NewFuncCondition(key any, matchFn func(any) bool) *FuncCondition {
	return &FuncCondition{
		key:     key,
		matchFn: matchFn,
	}
}

// Key implements pabtpkg.Variable.Key().
func (c *FuncCondition) Key() any {
	return c.key
}

// Match implements pabtpkg.Condition.Match(value any) bool.
func (c *FuncCondition) Match(value any) bool {
	if c == nil || c.matchFn == nil {
		return false
	}
	return c.matchFn(value)
}

// Mode returns EvalModeExpr since it's Go-native (no JavaScript).
func (c *FuncCondition) Mode() EvaluationMode {
	return EvalModeExpr
}

// Effect wraps pabtpkg.Effect with evaluation mode awareness.
// Effects don't need runtime evaluation, but tracking mode helps
// with debugging and introspection.
type Effect struct {
	key   any
	value any
}

var _ pabtpkg.Effect = (*Effect)(nil)

// NewEffect creates a new effect.
func NewEffect(key, value any) *Effect {
	return &Effect{
		key:   key,
		value: value,
	}
}

// Key implements pabtpkg.Variable.Key().
func (e *Effect) Key() any {
	return e.key
}

// Value implements pabtpkg.Effect.Value().
func (e *Effect) Value() any {
	return e.value
}

// ClearExprCache clears the global expression cache.
// This is useful for testing to ensure consistent state.
func ClearExprCache() {
	exprCache.Range(func(key, _ any) bool {
		exprCache.Delete(key)
		return true
	})
}
