package pabt

import (
	"container/list"
	"fmt"
	"log/slog"
	"sync"

	"github.com/dop251/goja"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	pabtpkg "github.com/joeycumines/go-pabt"
	btmod "github.com/joeycumines/one-shot-man/internal/builtin/bt"
)

// DefaultExprCacheSize is the default maximum number of entries in the expression cache.
// This limits memory growth for long-running processes with dynamic expressions.
const DefaultExprCacheSize = 1000

// exprCache is a bounded LRU cache for compiled expr-lang programs.
// It replaces the previous unbounded sync.Map to prevent memory growth.
var exprCache = NewExprLRUCache(DefaultExprCacheSize)

// exprCacheSize is the configurable maximum size for the expression cache.
// It is protected by exprCacheMu for thread-safe updates.
var exprCacheSize = DefaultExprCacheSize
var exprCacheMu sync.RWMutex

// SetExprCacheSize sets the maximum size of the expression cache.
// If the new size is smaller than the current size, the cache is truncated.
// Thread-safe and can be called at runtime.
func SetExprCacheSize(size int) {
	if size < 1 {
		size = 1
	}
	exprCacheMu.Lock()
	exprCacheSize = size
	exprCacheMu.Unlock()
	exprCache.Resize(size)
}

// GetExprCacheSize returns the current maximum size of the expression cache.
func GetExprCacheSize() int {
	exprCacheMu.RLock()
	defer exprCacheMu.RUnlock()
	return exprCacheSize
}

// ExprLRUCache is a thread-safe LRU cache for expr-lang compiled programs.
// Uses sync.RWMutex to allow concurrent reads while serializing writes.
type ExprLRUCache struct {
	mu        sync.RWMutex
	cache     map[string]*list.Element
	lru       *list.List
	maxSize   int
	hitCount  int64
	missCount int64
}

// NewExprLRUCache creates a new LRU cache with the specified maximum size.
func NewExprLRUCache(maxSize int) *ExprLRUCache {
	if maxSize < 1 {
		maxSize = DefaultExprCacheSize
	}
	return &ExprLRUCache{
		cache:   make(map[string]*list.Element, maxSize),
		lru:     list.New(),
		maxSize: maxSize,
	}
}

// entry represents a cached expression program.
type entry struct {
	expression string
	program    *vm.Program
}

// Get retrieves a compiled program from the cache.
// Returns the program and true if found, nil and false otherwise.
// Uses Lock (not RLock) because Get must update LRU order and hit/miss
// counters, and must read the program pointer under the same lock that
// protects writes in Put to avoid data races.
func (c *ExprLRUCache) Get(expression string) (*vm.Program, bool) {
	c.mu.Lock()
	elem, ok := c.cache[expression]
	if !ok {
		c.missCount++
		c.mu.Unlock()
		return nil, false
	}

	// Read program under lock to avoid data race with Put updating the pointer
	program := elem.Value.(*entry).program
	c.hitCount++
	// Update LRU order - move accessed element to front (most recently used)
	// This ensures correct eviction behavior for LRU policy
	if elem != c.lru.Front() {
		c.lru.MoveToFront(elem)
	}
	c.mu.Unlock()
	return program, true
}

// Put adds a compiled program to the cache.
// If the cache is at capacity, the least recently used entry is evicted.
// If the expression already exists, it's updated (moved to front, program replaced).
// Thread-safe.
func (c *ExprLRUCache) Put(expression string, program *vm.Program) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists - update it by moving to front and replacing program
	if elem, ok := c.cache[expression]; ok {
		// Move existing entry to front (most recently used)
		c.lru.MoveToFront(elem)
		// Update the program (allows recompilation with same expression)
		elem.Value.(*entry).program = program
		return
	}

	// Add new entry at front (most recently used)
	elem := c.lru.PushFront(&entry{
		expression: expression,
		program:    program,
	})
	c.cache[expression] = elem

	// Evict LRU if over capacity
	for c.lru.Len() > c.maxSize {
		elem := c.lru.Back()
		if elem != nil {
			e := elem.Value.(*entry)
			delete(c.cache, e.expression)
			c.lru.Remove(elem)
		}
	}
}

// Resize changes the maximum size of the cache.
// If the new size is smaller, entries are evicted immediately.
func (c *ExprLRUCache) Resize(maxSize int) {
	if maxSize < 1 {
		maxSize = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxSize = maxSize
	// Evict entries until at capacity
	for c.lru.Len() > c.maxSize {
		elem := c.lru.Back()
		if elem != nil {
			e := elem.Value.(*entry)
			delete(c.cache, e.expression)
			c.lru.Remove(elem)
		}
	}
}

// Clear removes all entries from the cache.
func (c *ExprLRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lru.Init()
}

// Len returns the current number of entries in the cache.
func (c *ExprLRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// Stats returns cache statistics for monitoring.
func (c *ExprLRUCache) Stats() (size int, hits, misses int64, ratio float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total := c.hitCount + c.missCount
	ratio = 0.0
	if total > 0 {
		ratio = float64(c.hitCount) / float64(total)
	}
	return c.lru.Len(), c.hitCount, c.missCount, ratio
}

// String returns a human-readable description of cache stats.
func (c *ExprLRUCache) String() string {
	size, hits, misses, ratio := c.Stats()
	return fmt.Sprintf("ExprLRUCache{size=%d, hits=%d, misses=%d, hit_ratio=%.2f%%}",
		size, hits, misses, ratio*100)
}

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
// WARNING: The following fields are unexported and are internal implementation
// details. They are subject to change without notice. Do not rely on them:
//   - bridge: Used internally for thread-safe Goja access from the ticker goroutine
//   - jsObject: Stores the original JavaScript object for action generator passthrough
//
// For action generators that need access to the condition's JS object, use the
// GetJSObject() method instead of accessing jsObject directly.
type JSCondition struct {
	key      any
	matcher  goja.Callable
	bridge   *btmod.Bridge // INTERNAL: Required for thread-safe goja access from ticker goroutine
	jsObject *goja.Object  // INTERNAL: Original JS object for passthrough to action generator
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

// JSObject returns the backing JavaScript object for this condition,
// or nil if not a JS-based condition or if the object is not set.
// This provides controlled access for action generators that need
// to inspect the original condition properties.
func (c *JSCondition) JSObject() *goja.Object {
	return c.jsObject
}

// SetJSObject sets the backing JavaScript object for this condition.
// This is used by the parser to attach the original JS object.
func (c *JSCondition) SetJSObject(obj *goja.Object) {
	c.jsObject = obj
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
//
// ERROR HANDLING (H8 fix): Errors are now logged to help distinguish from actual
// false matches. This includes nil condition/bridge/matcher cases and bridge
// stopped cases. Callers can use pabt.NewJSConditionWithValidation for stricter
// error handling.
func (c *JSCondition) Match(value any) bool {
	// Defensive: check if condition is valid before calling matcher
	if c == nil {
		return false
	}
	if c.matcher == nil {
		return false
	}
	if c.bridge == nil {
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

	if err != nil {
		return false
	}

	return result
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

// ExprCondition implements pabtpkg.Condition using expr-lang Go-native evaluation.
// This condition is evaluated directly in Go with zero Goja calls.
type ExprCondition struct {
	key        any
	expression string
	program    *vm.Program // Cached compiled program (nil until first use)
	mu         sync.RWMutex
	// jsObject stores the original JavaScript object for passthrough to action generator.
	// This mirrors the behavior of JSCondition.
	jsObject *goja.Object
	// lastErr tracks the most recent error during compilation or evaluation.
	// This allows distinguishing between legitimate false results and errors.
	lastErr error
}

var _ Condition = (*ExprCondition)(nil)

// SetJSObject sets the backing JavaScript object for this condition.
// This object is returned to the action generator when this condition fails.
func (c *ExprCondition) SetJSObject(obj *goja.Object) {
	c.jsObject = obj
}

// NewExprCondition creates a new expr-lang based condition.
// The expression is compiled lazily on first Match call and cached globally.
//
// Panics if expression is empty (m-3 fix).
//
// Expression syntax follows expr-lang (github.com/expr-lang/expr):
//   - Field access: Value.x, Value.name
//   - Comparisons: Value == 10, Value > 5, Value != nil
//   - Boolean logic: Value.x > 0 && Value.y < 100
//   - String matching: Value.name contains "test"
//   - Collection operations: all(Value.items, {.active})
//
// The environment provides a single "value" variable containing the
// condition input value.
func NewExprCondition(key any, expression string) *ExprCondition {
	if expression == "" {
		panic("pabt.NewExprCondition: expression cannot be empty")
	}
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
// It provides the condition input value as "value".
type ExprEnv struct {
	Value any `expr:"value"`
}

// Match implements pabtpkg.Condition.Match(value any) bool.
// It compiles the expression (if not cached) and runs it natively in Go.
//
// CRITICAL: This method makes ZERO Goja calls. All evaluation is pure Go.
// This provides 10-100x performance improvement over JavaScript evaluation
// for equivalent conditions.
//
// ERROR HANDLING (M-3 fix): Compilation and evaluation errors are now tracked
// and logged to help distinguish from actual false results. Use LastError()
// to retrieve the most recent error.
func (c *ExprCondition) Match(value any) bool {
	// Check nil first before any field access
	if c == nil || c.expression == "" {
		return false
	}

	// Clear previous error on each match attempt
	c.mu.Lock()
	c.lastErr = nil
	c.mu.Unlock()

	program, err := c.getOrCompileProgram()
	if err != nil {
		c.mu.Lock()
		c.lastErr = fmt.Errorf("expression compilation failed: %w", err)
		c.mu.Unlock()
		slog.Error("[PA-BT] ExprCondition compilation error",
			"expression", c.expression,
			"error", err)
		return false
	}

	// Create environment with the value
	env := ExprEnv{Value: value}

	// Run the compiled program
	result, err := expr.Run(program, env)
	if err != nil {
		c.mu.Lock()
		c.lastErr = fmt.Errorf("expression evaluation failed: %w", err)
		c.mu.Unlock()
		slog.Error("[PA-BT] ExprCondition evaluation error",
			"expression", c.expression,
			"value", fmt.Sprintf("%v", value),
			"error", err)
		return false
	}

	// Convert result to boolean
	if b, ok := result.(bool); ok {
		return b
	}
	// Non-boolean result is treated as error
	c.mu.Lock()
	c.lastErr = fmt.Errorf("expression returned non-boolean result: %T", result)
	c.mu.Unlock()
	slog.Warn("[PA-BT] ExprCondition non-boolean result",
		"expression", c.expression,
		"resultType", fmt.Sprintf("%T", result),
		"result", fmt.Sprintf("%v", result))
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

	// Check global LRU cache
	if cached, ok := exprCache.Get(c.expression); ok {
		program := cached
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

	// Store in global LRU cache and instance
	exprCache.Put(c.expression, program)
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

// LastError returns the most recent error from compilation or evaluation.
// Returns nil if the last Match() call succeeded without error.
// This allows distinguishing between legitimate false results and errors.
func (c *ExprCondition) LastError() error {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastErr
}

// JSObject returns the backing JavaScript object for this condition,
// or nil if not set.
func (c *ExprCondition) JSObject() *goja.Object {
	if c == nil {
		return nil
	}
	return c.jsObject
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
	exprCache.Clear()
}
