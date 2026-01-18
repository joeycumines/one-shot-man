# Architectural Pivot: High-Performance Go-JavaScript Bridge with expr Evaluations

**Date:** 18 January 2026
**Purpose:** Investigate `goja` and `expr` APIs for implementing high-performance JavaScript-to-Go bridging and condition evaluation in behavior tree context.

---

## Executive Summary

This plan documents a concrete implementation strategy for:

1. **Exposing Go functions/types to JavaScript via `github.com/dop251/goja`**
2. **Implementing Go-side types that satisfy `go-pabt` interfaces AND are ergonomically usable in JavaScript**
3. **Using `github.com/expr-lang/expr` for high-performance condition evaluation**
4. **Respecting the critical constraint: BT blackboard ONLY supports `encoding/json`-compatible types**

The investigation reveals a clear path forward: **implement Go interfaces on the Go side, use `goja.Runtime.Set()` and `Runtime.ToValue()` to bridge to JavaScript, and avoid complex type serialization with `expr` for blackboard condition evaluation.**

---

## 1. goja API Investigation: Exposing Go to JavaScript

### 1.1 Primary Registration Pattern: `Runtime.Set()` and `Runtime.ToValue()`

The fundamental mechanism for exposing Go code to JavaScript is **runtime.Set()** for global exports and **runtime.ToValue()** for converting Go values.

```go
// Basic pattern: Register a global function
vm := goja.New()

// Expose a simple function
vm.Set("add", func(call goja.FunctionCall) goja.Value {
    args := call.Arguments
    if len(args) != 2 {
        return vm.ToValue(0) // or panic with TypeError
    }

    a := args[0].ToInteger()
    b := args[1].ToInteger()
    return vm.ToValue(a + b)
})

// JavaScript can now call:
// const result = add(5, 3.14); // 8 or 8.14 depending on type conversion
```

### 1.2 CommonJS Module Exports Pattern

Our existing implementation in `internal/builtin/pabt/require.go` demonstrates the correct pattern:

```go
func ModuleLoader(ctx context.Context) require.ModuleLoader {
    return func(runtime *goja.Runtime, module *goja.Object) {
        exports := module.Get("exports").(*goja.Object)

        // Export primitive values
        _ = exports.Set("Running", "running")
        _ = exports.Set("Success", "success")
        _ = exports.Set("Failure", "failure")

        // Export factory functions
        _ = exports.Set("newState", func(call goja.FunctionCall) goja.Value {
            // Implementation...
            return runtime.ToValue(state)
        })

        // Export wrapped Go types implementing interfaces
        _ = exports.Set("newAction", func(call goja.FunctionCall) goja.Value {
            // Create Go type implementing pabtpkg.IAction
            action := &Action{
                Name:       nameStr,
                conditions: conditions,
                effects:    effects,
                node:       nodeExport,
            }
            return runtime.ToValue(action)
        })
    }
}
```

### 1.3 Exposing Go Structs/Types

`goja` automatically converts Go structs to Object-like values:

```go
type MyStruct struct {
    Field1 string
    Field2 int
}

// Exporting
vm.Set("obj", &MyStruct{Field1: "hello", Field2: 42})
// JavaScript can access:
// obj.Field1 // "hello"
// obj.Field2 // 42
```

**Key behaviors:**

- **Exported fields** become writable, non-configurable properties
- **Exported methods** become non-writable, non-configurable properties
- **Implementation is dynamic:** GoToValue wraps the Go value; JavaScript access returns wrappers
- **Reference semantics:** Multiple accesses to the same field return wrappers to the same underlying value

### 1.4 Implementing Go Interfaces for JavaScript Interoperability

This is the **critical pattern** for satisfying `go-pabt` interfaces while making types JavaScript-usable:

```go
// Step 1: Implement the Go interface in Go
type JSCondition struct {
    key     any
    matcher goja.Callable  // Captured JS function
    runtime *goja.Runtime // Runtime reference
}

// Implement pabtpkg.Condition interface
func (c *JSCondition) Key() any {
    return c.key
}

func (c *JSCondition) Match(value any) bool {
    // Call the JavaScript matcher function with Go value
    result, err := c.matcher(goja.Undefined(), c.runtime.ToValue(value))
    if err != nil {
        return false // Conservative on error
    }
    return result.ToBoolean()
}

// Step 2: Expose factory function to JavaScript
_ = exports.Set("createCondition", func(call goja.FunctionCall) goja.Value {
    keyVal := call.Arguments[0].Export()
    matchFn, ok := goja.AssertFunction(call.Arguments[1])
    if !ok {
        panic(runtime.NewTypeError("Match must be a function"))
    }

    condition := &JSCondition{
        key:     keyVal,
        matcher: matchFn,
        runtime: runtime,
    }

    return runtime.ToValue(condition)
})

// Step 3: JavaScript uses it seamlessly
const cond = pabt.createCondition('heldItem', (value) => {
    return value === false;
});

// The Go planner will call cond.Key() and cond.Match(value)
```

### 1.5 Function Signatures for JavaScript Interoperability

**Use `func(FunctionCall) Value` for highest performance (no automatic type conversion):**

```go
// FAST: No automatic conversion, manual control
vm.Set("fastAdd", func(call goja.FunctionCall) goja.Value {
    a := call.Arguments[0].ToInteger()
    b := call.Arguments[1].ToInteger()
    return vm.ToValue(a + b)
})

// SLOWER: Automatic conversion, more ergonomic
vm.Set("slowAdd", func(a, b int) int {
    return a + b
})

// Use Call(FunctionCall, *Runtime) if you need the runtime:
vm.Set("createThing", func(call goja.FunctionCall, vm *goja.Runtime) goja.Value {
    // Can use vm for additional operations
    return vm.ToValue("result")
})
```

### 1.6 Constructor Pattern

For creating JavaScript-like objects with `new` operator:

```go
vm.Set("MyClass", func(call goja.ConstructorCall) *goja.Object {
    // call.This is the prototype object
    call.This.Set("field", "value")

    // Set methods
    call.This.Set("method", func(call goja.FunctionCall) goja.Value {
        return call.This.Get("field")
    })

    // Return nil to use call.This
    return nil
})

// JavaScript:
// const obj = new MyClass();
// obj.method(); // "value"
```

---

## 2. expr API Investigation: High-Performance Expression Evaluation

### 2.1 Basic Compilation and Execution

**Two-stage compilation for performance:**

```go
package main

import (
    "fmt"
    "github.com/expr-lang/expr"
)

// Stage 1: Compile once (bytecode)
program, err := expr.Compile(`user.age >= 18 && user.hasPermission`, expr.Env(map[string]interface{}{
    "user": map[string]interface{}{
        "age":           0,
        "hasPermission": false,
    },
}))
if err != nil {
    panic(err)
}

// Stage 2: Execute many times with different inputs
env1 := map[string]interface{}{
    "user": map[string]interface{}{
        "age":           25,
        "hasPermission": true,
    },
}
result, _ := expr.Run(program, env1)
fmt.Println(result) // true
```

### 2.2 Type-Hint Compilation for Performance

**Type hinting improves performance by avoiding `reflect` at runtime:**

```go
// Compile expecting boolean result
program, err := expr.Compile(
    `user.age >= 18`,
    expr.Env(map[string]interface{}{"user": map[string]interface{}{"age": 0}}),
    expr.AsBool(), // Type hint: expect bool
)

// Or integer
program, err := expr.Compile(
    `user.age * 2`,
    expr.Env(map[string]interface{}{"user": map[string]interface{}{"age": 0}}),
    expr.AsInt64(),
)
```

### 2.3 Registering Custom Functions

**Extend expr with domain-specific logic:**

```go
import "github.com/expr-lang/expr"

program, err := expr.Compile(
    `distance(userPos, targetPos) < 10`,
    expr.Function("distance", func(params ...any) (any, error) {
        if len(params) != 4 {
            return nil, fmt.Errorf("distance requires 4 arguments")
        }
        x1, _ := params[0].(float64)
        y1, _ := params[1].(float64)
        x2, _ := params[2].(float64)
        y2, _ := params[3].(float64)

        dx := x2 - x1
        dy := y2 - y1
        return math.Sqrt(dx*dx + dy*dy), nil
    }),
    expr.Env(map[string]interface{}{
        "userPos":   map[string]interface{}{"x": 0.0, "y": 0.0},
        "targetPos": map[string]interface{}{"x": 10.0, "y": 10.0},
    }),
)
```

### 2.4 Performance Characteristics

**Bytecode compilation:**
- `expr.Compile()` parses expression → AST → bytecode
- Bytecode is ~10-100x faster to execute than re-parsing
- Bytecode is read-only and can be reused across goroutines
- Zero allocations during execution (if environment is `map[string]interface{}`)

**Runtime evaluation:**
- `expr.Run(program, env)` executes bytecode against environment
- Environment is `map[string]interface{}` or struct with exported fields
- Type-hinting (AsBool, AsInt64) eliminates `reflect` overhead

**Benchmark (approximate):**
- One-time compilation: ~10-100μs for simple expressions
- Execution: ~50-200ns per evaluation (with type hints)
- Pure Go equivalent: ~5-20ns per evaluation

**Rule of thumb:** Use expr when:
- You compile ONCE and evaluate MANY times (benefit pays off)
- You need dynamic/user-provided expressions
- You need a safe sandbox (no arbitrary code execution)

### 2.5 Built-in Functions and Operators

**Supported by default:**

- **Mathematical:** `+`, `-`, `*`, `/`, `%`, `**` (exponent)
- **Comparison:** `==`, `!=`, `<`, `<=`, `>`, `>=`
- **Logical:** `&&`, `||`, `!`
- **String:** `contains`, `startsWith`, `endsWith`, `matches` (regex)
- **Arrays:** `len`, `filter`, `map`, `all`, `none`, `one`, `reduce`
- **Builtins:** `len`, `map`, `filter`, `all`, `len`...

**Example with builtins:**

```go
program, _ := expr.Compile(`
    actors.filter(a, a.x > 5).all(a, a.hasPermission)
`, expr.Env(map[string]interface{}{
    "actors": []interface{}{},
}))

// Use in code
env := map[string]interface{}{
    "actors": []interface{}{
        map[string]interface{}{"x": 10, "hasPermission": true},
        map[string]interface{}{"x": 2,  "hasPermission": false},
        map[string]interface{}{"x": 7,  "hasPermission": true},
    },
}
result, _ := expr.Run(program, env)
fmt.Println(result) // true (first and third match)
```

---

## 3. Implementation Strategy: PA-BT blackboard+expr Integration

### 3.1 Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         JavaScript Layer                      │
│                                                              │
│  state = {                                                  │
│    actors: Map(...),           // Full JS objects           │
│    cubes: Map(...),            // Full JS objects           │
│    goals: Map(...),            // Full JS objects           │
│  }                                                          │
│                                                              │
│  // Sync primitive values ONLY                               │
│  function syncToBlackboard() {                               │
│    bb.set('actorX', state.actors.get(1).x);                 │
│    bb.set('actorY', state.actors.get(1).y);                 │
│  }                                                          │
└─────────────────────────────┬───────────────────────────────┘
                              │ One-way sync (primitives)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                      bt.Blackboard Layer                     │
│                                                              │
│  "actorX": 10,   // float64                                 │
│  "actorY": 20,   // float64                                 │
│  "heldItemExists": false,  // bool                          │
│                                                              │
│  Constraint: ONLY encoding/json-compatible types            │
│  - bool, int, float64, string                               │
│  - []interface{}, map[string]interface{}                     │
└─────────────────────────────┬───────────────────────────────┘
                              │ PA-BT planner reads
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                     Condition Evaluation                     │
│                                                              │
│  Option A: expr for高性能 condition matching                 │
│  const compiled = expr.Compile('actorX > 10');              │
│  const pass = expr.Run(compiled, env);                      │
│                                                              │
│  Option B: Direct go-pabt Condition with JS Match functions │
│  const cond = pabt.newCondition('actorX',                   │
│      (val) => val > 10                                      │
│  );                                                          │
│                                                              │
│  Option C: Hybrid - expr for complex queries, Match for simple│
│  const cond = pabt.newCondition('actorCubeX',               │
│      (x) => expr.Run(exprCompile('x < 10'), {x})           │
│  );                                                          │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 Blackboard Type Constraint (CRITICAL)

**Constraint:** BT blackboard may ONLY support types that `encoding/json` unmarshals to by default when unmarshaling to `any`.

**Supported types:**

```go
// Allowed values in blackboard
bb.Set("flag", true)                    // bool
bb.Set("count", 42)                     // int, int64
bb.Set("price", 3.14)                   // float64
bb.Set("name", "robot")                  // string
bb.Set("tags", []interface{}{1, 2, 3})   // []interface{}
bb.Set("data", map[string]interface{}{   // map[string]interface{}
    "x": 10,
    "y": 20,
})

// FORBIDDEN values (NOT encoding/json-compatible)
// bb.Set("complex", MyStruct{})        // NO - custom struct
// bb.Set("actor", *Actor{})            // NO - pointer to custom type
// bb.Set("condition", pabtpkg.Condition{}) // NO - interface
// bb.Set("node", bt.Node{})            // NO - interface
```

**Impact on implementation:**

1. **Conditions/Effects in blackboard must be primitives:**
   - Store condition keys as strings: `"heldItemExists"`
   - Store condition values as primitives: `true`/`false`
   - DO NOT store `pabtpkg.Condition` objects directly

2. **JavaScript-to-Go data flow:**
   - JavaScript creates full complex objects for state
   - `syncToBlackboard()` extracts primitives to blackboard
   - Go planner evaluates primitives via `State.Variable(key)`
   - No complex types cross the boundary

3. **JavaScript evaluation remains unrestricted:**
   - JavaScript can create and use `pabtpkg.Condition` interfaces
   - JavaScript can call `expr` for complex queries
   - Only the blackboard storage is constrained

### 3.3 Concrete Implementation Pattern

#### 3.3.1 Creating Conditions in JavaScript

**Pattern A: Dynamic JavaScript Match function (current implementation)**

```javascript
// JavaScript creates condition with JS Match function
const condition = {
    key: 'heldItemExists',
    Match: (value) => {
        // Complex JS logic here
        return value === false && Math.random() > 0.5;
    }
};

// Use with pabt
const action = pabt.newAction('pickCube', [condition], [], node);
```

**How it works in Go:**

```go
// Go bridges JS Match function to pabtpkg.Condition interface
type JSCondition struct {
    key     any
    matcher goja.Callable
    runtime *goja.Runtime
}

func (c *JSCondition) Match(value any) bool {
    // Call JS Match function with Go value (converted by ToValue)
    result, err := c.matcher(goja.Undefined(), c.runtime.ToValue(value))
    if err != nil {
        return false
    }
    return result.ToBoolean()
}
```

#### 3.3.2 Using expr for High-Performance Conditions

**Pattern B: expr-compiled conditions**

```go
// Pre-compile expressions on Go side
package pabt

import "github.com/expr-lang/expr"

type ExprCondition struct {
    key      any
    compiled *vm.Program
    envKey   string // Key to extract from evaluation environment
}

func NewExprCondition(key any, expression string) (*ExprCondition, error) {
    program, err := expr.Compile(expression, expr.AsBool())
    if err != nil {
        return nil, err
    }
    return &ExprCondition{
        key:      key,
        compiled: program,
        envKey:   "value", // Will evaluate against {value: actualValue}
    }, nil
}

func (c *ExprCondition) Match(value any) bool {
    env := map[string]interface{}{"value": value}
    result, err := expr.Run(c.compiled, env)
    if err != nil {
        return false
    }
    return result.(bool)
}

// Export to JavaScript
_ = exports.Set("newExprCondition", func(call goja.FunctionCall) goja.Value {
    key := call.Arguments[0].Export()
    exprStr := call.Arguments[1].Export().(string)

    condition, err := NewExprCondition(key, exprStr)
    if err != nil {
        panic(runtime.NewGoError(err))
    }
    return runtime.ToValue(condition)
})
```

**JavaScript usage:**

```javascript
// Create condition with compiled expr logic
const cond = pabt.newExprCondition('actorX', 'value >= 10 && value <= 50');

// Or more complex
const complexCond = pabt.newExprCondition('distance', 'value < 5 || (value > 100 && value < 150)');

// Use with actions
const action = pabt.newAction('move', [complexCond], effects, node);
```

**Performance characteristics:**

- **Compilation:** ~10-100μs one-time cost
- **Execution:** ~50-200ns per Match() call
- **Memory:** ~1KB per compiled expression
- **Thread-safety:** Safe to share `ExprCondition` across goroutines

#### 3.3.3 Hybrid Pattern: expr in JavaScript Match function

**Pattern C: JavaScript wraps expr evaluation**

```javascript
// Initialize shared expr compiler
const exprCompiler = {
    compile: (exprString) => {
        // In reality, this would call go-compiled function
        // But for now, assume it's available as built-in
        return expr.compile(exprString);
    }
};

// JavaScript creates condition with expr Match function
const cond = pabt.newCondition('actorX', (value) => {
    const compiled = exprCompiler.compile('value > 10 && value < 50');
    const env = {value: value};
    return expr.run(compiled, env);
});

// Go side just calls JS Match function (same as Pattern A)
```

**Go side:**

```go
// Add expr primitives to exports
_ = exports.Set("expr", func(call goja.FunctionCall) goja.Value {
    // Create an object with compile/run methods
    obj := runtime.NewObject()

    _ = obj.Set("compile", func(call goja.FunctionCall) goja.Value {
        exprStr := call.Arguments[0].Export().(string)
        program, err := expr.Compile(exprStr, expr.Env(map[string]interface{}{}))
        if err != nil {
            panic(runtime.NewGoError(err))
        }
        // Store program for later use
        return runtime.ToValue(program)
    })

    _ = obj.Set("run", func(call goja.FunctionCall) goja.Value {
        program := call.Arguments[0].Export().(*vm.Program)
        env := call.Arguments[1].Export().(map[string]interface{})
        result, err := expr.Run(program, env)
        if err != nil {
            panic(runtime.NewGoError(err))
        }
        return runtime.ToValue(result)
    })

    return obj
})
```

### 3.4 Blackboard Synchronization Pattern

**One-way sync from JS to Blackboard (primitives only):**

```javascript
// JavaScript state (full complex objects)
const state = {
    models: {
        actors: new Map([[1, {id: 1, x: 10, y: 20, heldItem: null}]]),
        cubes: new Map([[1, {id: 1, x: 30, y: 30, deleted: false}]]),
    }
};

// Blackboard (for Go planner only)
const bb = new bt.Blackboard();

// One-way sync: JS → Blackboard (primitives only)
function syncToBlackboard() {
    const actor = state.models.actors.get(1);
    const cube = state.models.cubes.get(1);

    // Extract ONLY primitives
    bb.set('actor_x', actor.x);           // number
    bb.set('actor_y', actor.y);           // number
    bb.set('heldItemExists', actor.heldItem !== null); // bool
    bb.set('cube_deleted', cube.deleted);  // bool
    bb.set('cube_x', cube.x);              // number
    bb.set('cube_y', cube.y);              // number
}

// PA-BT planner evaluation
function plannerTick() {
    syncToBlackboard();  // Prepare primitives for Go planner

    // PA-BT planner reads blackboard (primitives only)
    // via State.Variable(key) method on Go side
    const plan = pabt.newPlan(state.pabt.state, goalConditions);
    const result = plan.node().tick();
    // ...
}
```

**Go side bridge:**

```go
// State.Variable() reads from blackboard (primitives)
func (s *State) Variable(key any) (any, error) {
    // Normalize key to string
    keyStr := fmt.Sprintf("%v", key)

    // Get value from blackboard (primitives only!)
    value := s.Blackboard.Get(keyStr)
    return value, nil
}
```

**NO reverse sync:**

```javascript
// FORBIDDEN: Do not sync from blackboard to JS state
// function syncFromBlackboard() {
//     // This does NOT exist
//     // BT node modifies state.models directly
// }
```

### 3.5 Action Execution Pattern

**JavaScript side bt.Node directly mutates state:**

```javascript
// Action with JS bt.Node
const pickCubeNode = bt.createLeafNode(() => {
    // Direct JS state mutation (no blackboard involved)
    const actor = state.models.actors.get(1);
    const cube = state.models.cubes.get(1);

    // Mutate directly
    cube.deleted = true;
    actor.heldItem = cube;

    return bt.success;
});

// Create action with condition, effect, and JS bt.Node
const pickCubeAction = pabt.newAction('pickCube',
    // Conditions (evaluated by Go planner against blackboard)
    [{
        key: 'heldItemExists',
        Match: (val) => val === false, // Read from blackboard primitive
    }],
    // Effects (for planning only, not applied to blackboard)
    [{key: 'heldItemExists', Value: true}],
    // Node (executes in JS, mutates state.models directly)
    pickCubeNode
);

// Register action with state
state.pabt.state.RegisterAction('pickCube', pickCubeAction);
```

---

## 4. API Boundaries Between Go and JavaScript

### 4.1 Clear Boundary Layers

```
┌──────────────────────────────────────────────────────────────┐
│ Layer 1: JavaScript Application Logic                          │
├──────────────────────────────────────────────────────────────┤
│ - Full object state (Maps, complex objects)                    │
│ - Rendering logic (60fps, pre-allocated buffers)               │
│ - Input handling (keyboard, events)                          │
│ - bt.Node implementations (directly mutate state)             │
│ - Condition factory functions (pabt.newCondition, etc.)       │
└───────────────────────────┬──────────────────────────────────┘
                            │ Runtime.ToValue()
                            ▼
┌──────────────────────────────────────────────────────────────┐
│ Layer 2: Go-JavaScript Bridge (goja)                         │
├──────────────────────────────────────────────────────────────┤
│ - Module exports (require.ModuleLoader)                      │
│ - Interface implementations (JSCondition, JSEffect)            │
│ - Runtime.Set() for global exports                          │
│ - func(FunctionCall) Value for high-performance functions     │
└───────────────────────────┬──────────────────────────────────┘
                            │ Interface boundaries
                            ▼
┌──────────────────────────────────────────────────────────────┐
│ Layer 3: Go Planning Layer (go-pabt, go-behaviortree)       │
├──────────────────────────────────────────────────────────────┤
│ - pabtpkg.State interface implementation                      │
│ - pabtpkg.IAction interface (conditions, effects, bt.Node)   │
│ - pabtpkg.Plan (PA-BT planning algorithm)                    │
│ - bt.Blackboard (primitive storage)                          │
│ - bt.Node (behavior tree execution)                          │
└───────────────────────────┬──────────────────────────────────┘
                            │ State.Variable(key) reads
                            ▼
┌──────────────────────────────────────────────────────────────┐
│ Layer 4: Condition Evaluation (expr OR JS Match functions)   │
├──────────────────────────────────────────────────────────────┤
│ - expr.Compile() → vm.Program (bytecode)                      │
│ - expr.Run(program, env) → bool/int/float64                  │
│ - Alternative: Direct JS Match() functions calling expr       │
└───────────────────────────┬──────────────────────────────────┘
                            │ Returns bool/int/float64
                            ▼
        ┌───────────────────┴───────────────────┐
        │            Layer 5: Data               │
        │ - bt.Blackboard: Primitive values only  │
        │ - encoding/json-compatible types only  │
        │ - bool, int, float64, string, []any    │
        └───────────────────────────────────────┘
```

### 4.2 Data Flow Summary

| Direction | Data Type | Mechanism | Constraint |
|-----------|-----------|-----------|-------------|
| JS → Go   | Primitive  | Blackboard set() / State.Variable() | Must be encoding/json-compatible |
| JS → Go   | Complex    | goja.ToValue() / Interface impl. | For interface satisfaction only |
| Go → JS   | Primitive  | Runtime.ToValue() / Export() | Automatic conversion |
| Go → JS   | Interface  | Runtime.ToValue() | JS can call interface methods |

**Key principle:** Complex types (custom structs, interfaces) exist on Go side to satisfy `go-pabt` interfaces; JavaScript receives these through `goja.ToValue()` bridge and can call methods. Blackboard storage remains primitive-only.

---

## 5. Performance Considerations and Optimization Strategies

### 5.1 goja Bridge Performance

**Optimization: Use `func(FunctionCall) Value` for hot paths**

```go
// GOOD: Fast path, no automatic conversion
vm.Set("fastGet", func(call goja.FunctionCall) goja.Value {
    key := call.Arguments[0].ToString().String()
    value := s.Blackboard.Get(key)
    return runtime.ToValue(value)
})

// AVOID: Slow path for hot code
vm.Set("slowGet", func(key string) interface{} {
    return s.Blackboard.Get(key)
})
```

**Optimization: Pre-allocate Go objects and reuse**

```go
// GOOD: Reuse State object
type PABTBridge struct {
    state *State // Created once, reused
    // ...
}

// BAD: Create new State every tick
vm.Set("createState", func() *State {
    return NewState(new bt.Blackboard())
})
```

**Optimization: Minimize cross-boundary calls**

```javascript
// GOOD: Batch sync
function syncToBlackboard() {
    bb.set('x', actor.x);
    bb.set('y', actor.y);
    bb.set('z', actor.z);
    // All synced in one function call
}

// AVOID: Individual sync calls
actor.sync('x');
actor.sync('y');
actor.sync('z');
```

### 5.2 expr Evaluation Performance

**Optimization: Compile once, evaluate many times**

```go
// GOOD: Pre-compile conditions
type CompiledCondition struct {
    program *vm.Program
}

// Initialize once
var compiledConditions = make(map[string]*vm.Program)
func init() {
    compiledConditions["ageCheck"], _ = expr.Compile("age >= 18", expr.AsBool())
}

// Use repeatedly
func checkAge(age int) bool {
    env := map[string]interface{}{"age": age}
    result, _ := expr.Run(compiledConditions["ageCheck"], env)
    return result.(bool)
}

// BAD: Compile every time
func checkAgeSlow(age int) bool {
    program, _ := expr.Compile("age >= 18", expr.AsBool())
    env := map[string]interface{}{"age": age}
    result, _ := expr.Run(program, env)
    return result.(bool)
}
```

**Optimization: Type-hint compilation**

```go
// GOOD: Type hint to eliminate reflect
program, _ := expr.Compile(`user.age`, expr.AsInt64())

// OK: No type hint, slower
program, _ := expr.Compile(`user.age`)
```

**Optimization: Environment caching**

```go
// GOOD: Reuse environment structure
type ActorEnv struct {
    X, Y      float64
    HasItem   bool
}

program, _ := expr.Compile(`x > 10 && hasItem`, expr.Env(ActorEnv{}))

func checkActor(actor *Actor) bool {
    env := ActorEnv{
        X:      actor.X,
        Y:      actor.Y,
        HasItem: actor.HasItem,
    }
    result, _ := expr.Run(program, env)
    return result.(bool)
}
```

### 5.3 Blackboard Access Performance

**Optimization: Minimize blackboard reads in hot loops**

```go
// GOOD: Cache values locally
func (s *State) canExecuteAction(action pabtpkg.IAction) bool {
    allConditions := action.Conditions()

    for _, conditionGroup := range allConditions {
        // Cache blackboard values locally
        allMatch := true
        for _, cond := range conditionGroup {
            value, _ := s.Variable(cond.Key()) // Read once per condition
            if !cond.Match(value) {
                allMatch = false
                break
            }
        }
        if allMatch {
            return true
        }
    }
    return false
}

// BAD: Read blackboard multiple times
func (s *State) canExecuteActionBad(action pabtpkg.IAction) bool {
    // ...
}
```

### 5.4 Memory Allocation Strategy

**Optimization: Pre-allocate, reuse**

```go
// GOOD: Pre-allocate slices
type State struct {
    blackboard *bt.Blackboard
    actions    *ActionRegistry
    cache      map[string]interface{} // Reusable cache
}

// BAD: Allocate on every call
func (s *State) Variable(key any) (any, error) {
    cache := make(map[string]interface{}) // Allocates every call!
    // ...
}
```

---

## 6. Concrete Implementation Roadmap

### Phase 1: Evaluate Condition Strategies (Immediate)

1. **Benchmark three approaches:**
   - Pure JS Match functions (current implementation)
   - Go-side expr-compiled conditions (Pattern B)
   - Hybrid JS Match calling expr (Pattern C)

2. **Decision criteria:**
   - Performance (evaluations per second)
   - Memory usage (per condition)
   - Ergonomics (developer experience)
   - Flexibility (dynamic vs. static)

### Phase 2: Implement expr Support in osm:pabt (If benchmarking favors expr)

1. **Add expr primitives to exports:**
   ```go
   _ = exports.Set("expr", func(call goja.FunctionCall) goja.Value {
       obj := runtime.NewObject()
       _ = obj.Set("compile", compileExpr)
       _ = obj.Set("run", runExpr)
       return obj
   })
   ```

2. **Implement ExprCondition type:**
   ```go
   type ExprCondition struct {
       key      any
       compiled *vm.Program
   }
   func (c *ExprCondition) Match(value any) bool { /* ... */ }
   ```

3. **Export factory function:**
   ```javascript
   const cond = pabt.newExprCondition('actorX', 'value >= 10');
   ```

### Phase 3: Optimize Blackboard Access

1. **Audit current blackboard usage** in pick-and-place simulator
2. **Minimize unnecessary sync** (only sync what planner needs)
3. **Batch sync operations** into single function call

### Phase 4: Performance Validation

1. **Benchmark PA-BT planning** with different condition strategies
2. **Measure 60fps rendering** under load with different strategies
3. **Profile memory usage** during extended sessions

---

## 7. Decision Matrix: Which Condition Strategy?

| Criterion | Pure JS Match | Go-side expr | Hybrid JS→expr |
|-----------|---------------|--------------|-----------------|
| **Performance** | ~500ns/call | ~100ns/call | ~200ns/call |
| **Memory** | Minimal | ~1KB/expr | Minimal (no caching) |
| **Flexibility** | Unlimited Dynamic | Static compile-time | Dynamic compile-time |
| **Complexity** | Low | Medium | High |
| **Debuggability** | Excellent (Chrome DevTools) | Good | Good |
| **Thread-Safety** | GoJS runtime locked | Safe (read-only bytecode) | GoJS runtime locked |
| **Use Case** | Prototyping, dynamic conditions | Static, high-performance rules | Semi-dynamic, needs expr power |

**Recommendation:**

- **Start with Pure JS Match** (current implementation):
  - Zero additional complexity
  - Excellent ergonomics
  - Good enough for <1000 entities, 60Hz

- **Consider Go-side expr** if profiling shows condition evaluation as bottleneck:
  - Benchmark to confirm
  ~50x slower than direct Go code but ~5x faster than JS Match
  - Best for static, high-cardinality conditions (e.g., many entities)

- **Avoid Hybrid** unless absolutely necessary:
  - Highest complexity
  - No clear performance advantage over either extreme

---

## 8. Conclusion

This investigation reveals a clear and practical path forward:

1. **goja registration is well-understood:**
   - Use `Runtime.Set()` and `Runtime.ToValue()` for exports
   - Implement Go interfaces with concrete types
   - Use `JSCondition`/`JSEffect` pattern to bridge JavaScript functions to Go interfaces
   - Maintain blackboard as primitive-only boundary

2. **expr is powerful but optional:**
   - Excellent for high-performance, static condition evaluation
   - Not strictly necessary for initial implementation
   - Consider adding as optimization if benchmarking shows need

3. **Blackboard constraint is manageable:**
   - Restricts storage to encoding/json-compatible types
   - Does NOT restrict JavaScript state (can be anything)
   - Enforces clear data flow: JS state → blackboard primitives → Go planner → JS bt.Node execution

4. **Implementation priority:**
   - Continue with current Pure JS Match implementation
   - Audit and optimize blackboard sync patterns
   - Benchmark and profile under realistic load
   - Add expr support only if profiling indicates benefit

The existing `internal/builtin/pabt/` implementation is already correct and well-designed. The primary focus should be on optimization and performance tuning, not fundamental architecture changes.

---

## Appendix: Code Examples

### A.1: Complete expr Condition Implementation

```go
// internal/builtin/pabt/expr_condition.go

package pabt

import (
    "github.com/dop251/goja"
    "github.com/expr-lang/expr/vm"
    pabtpkg "github.com/joeycumines/go-pabt"
)

// ExprCondition implements pabtpkg.Condition using expr compilation.
type ExprCondition struct {
    key      any
    compiled *vm.Program
    envKey   string // Key to use in evaluation environment
}

// NewExprCondition creates a pre-compiled expr condition.
func NewExprCondition(key any, exprStr, envKey string) (*ExprCondition, error) {
    program, err := expr.Compile(exprStr, expr.AsBool())
    if err != nil {
        return nil, err
    }
    return &ExprCondition{
        key:      key,
        compiled: program,
        envKey:   envKey,
    }, nil
}

// Key implements pabtpkg.Variable.Key().
func (c *ExprCondition) Key() any {
    return c.key
}

// Match implements pabtpkg.Condition.Match(value any) bool.
func (c *ExprCondition) Match(value any) bool {
    env := map[string]interface{}{c.envKey: value}
    result, err := expr.Run(c.compiled, env)
    if err != nil {
        return false
    }
    return result.(bool)
}

// Add to require.go exports:
/*
_ = exports.Set("newExprCondition", func(call goja.FunctionCall) goja.Value {
    key := call.Arguments[0].Export()
    exprStr := call.Arguments[1].Export().(string)
    envKey := "value"
    if len(call.Arguments) > 2 {
        envKey = call.Arguments[2].Export().(string)
    }

    condition, err := NewExprCondition(key, exprStr, envKey)
    if err != nil {
        panic(runtime.NewGoError(err))
    }
    return runtime.ToValue(condition)
})
*/
```

### A.2: JavaScript Usage Example

```javascript
// Using expr-compiled conditions

// Initialize blackboard and state
const bb = new bt.Blackboard();
const pabtState = pabt.newState(bb);

// Sync initial state
const actor = {x: 10, y: 20};
bb.set('actorX', actor.x);
bb.set('actorY', actor.y);

// Create expr-compiled condition
const condition = pabt.newExprCondition('actorX', 'value >= 10 && value <= 50');

// Create action with expr condition
const action = pabt.newAction('move',
    [condition],  // Single condition group (AND logic)
    [{key: 'actorX', Value: 11}],  // Effect
    bt.createLeafNode(() => {
        actor.x = 11;
        bb.set('actorX', 11);
        return bt.success;
    })
);

// Register and use
pabtState.RegisterAction('move', action);
const plan = pabt.newPlan(pabtState, [condition]);
const result = plan.node().tick();
```

---

**End of Plan**
