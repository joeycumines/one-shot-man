# Research: JavaScript Condition Introspection for PA-BT

**Date:** 2026-01-19  
**Status:** Complete  
**Author:** Takumi (匠)

## Executive Summary

This document analyzes the **full** Condition interface requirements from go-pabt and proposes how JavaScript can properly support them, including:
- Stateful conditions
- Differentiating between State.Variable calls vs Effect.Value calls
- The graph_test pattern implementation

## 1. go-pabt Reference Implementation Analysis

### 1.1 Condition Interface Contract

From `github.com/joeycumines/go-pabt/pabt.go`:

```go
// Condition models a constraint on a uniquely identifiable variable within the [State].
//
// Note that values of this type will be passed into [State.Actions] as-is, in order to facilitate handling of
// condition and/or failure specific action templating behavior. Failure-specific behavior MAY require stateful
// conditions or similar, along with a way to differentiate calls to [Condition.Match] with values from the actual
// state ([State.Variable]) vs values from effects ([Effect.Value]).
type Condition interface {
    Variable

    // Match returns true if the given value (which should be for the same variable identified by Key) matches /
    // passes the constraint, or false if it fails.
    Match(value any) bool
}
```

### 1.2 Critical Insight: When is Match() Called?

`Condition.Match()` is called in **two distinct contexts**:

1. **State Evaluation Context** (via `newConditionNode` in util.go):
   ```go
   func newConditionNode[T Condition](
       state State[T],
       key any,
       match func(value any) bool,
       outcome *bt.Status,
   ) bt.Node {
       return bt.New(func([]bt.Node) (status bt.Status, err error) {
           var value any
           value, err = state.Variable(key)  // <-- Gets ACTUAL state value
           if err == nil && match(value) {
               status = bt.Success
           } else {
               status = bt.Failure
           }
           *outcome = status
           return
       })
   }
   ```
   Here, `Match()` receives the **actual current state** from `State.Variable(key)`.

2. **Effect Filtering Context** (via `generateAction` in util.go):
   ```go
   for _, effect := range effects {
       key := effect.Key()
       // ...
       if !ok && key == pk && post.Match(effect.Value()) {  // <-- Uses EFFECT value
           ok = true
       }
   }
   ```
   Here, `Match()` receives the **hypothetical value** from `Effect.Value()`.

3. **Conflict Resolution Context** (via `ppa.conflicts` in util.go):
   ```go
   for _, pair := range pairs {
       if eff, ok := act.effects[pair.K]; ok && !pair.V.Match(eff.Value()) {  // <-- Uses EFFECT value
           return true
       }
   }
   ```
   Again, `Match()` receives effect values.

### 1.3 How go-pabt logic.go Handles This

Looking at `examples/tcell-pick-and-place/logic/logic.go`:

```go
func (p *pickAndPlace) Actions(failed pabt.Condition) (actions []pabt.IAction, err error) {
    var (
        key = failed.Key()
        add = func(name string, limit int) func(a []pabt.IAction, e error) bool {
            return func(a []pabt.IAction, e error) bool {
                // ...
                for i, a := range a {
                    var ok bool
                    for _, effect := range a.Effects() {
                        if effect.Key() == key && failed.Match(effect.Value()) {  // <-- Match on Effect.Value
                            ok = true
                            break
                        }
                    }
                    // ...
                }
                return false
            }
        }
    )
    // ...
}
```

The `Actions()` method explicitly calls `failed.Match(effect.Value())` to check if an action's effects would satisfy the failed condition. This is **effect-based matching**.

### 1.4 Stateful Conditions Pattern

In `templatePick` from logic.go:

```go
var running bool  // <-- STATEFUL: Closure captures this variable

actions = append(actions, &simpleAction{
    conditions: []pabt.IConditions{
        {
            &simpleCond{
                key: positionVar{Sprite: sprite},
                match: func(r any) bool {
                    if pos := r.(*positionValue).positions[sprite]; pos != nil && pos.Shape != nil {
                        if running {  // <-- STATEFUL: Behavior changes based on running
                            return true
                        }
                        if nx, ny := pos.Shape.Position(); ox == nx && oy == ny {
                            return true
                        }
                    }
                    return false
                },
            },
            // ...
        },
    },
    // ...
    node: bt.New(
        bt.Sequence,
        bt.New(func([]bt.Node) (bt.Status, error) {
            running = true  // <-- STATEFUL: Node sets running=true
            return bt.Success, nil
        }),
        // ...
    ),
})
```

This pattern allows:
1. **Before action starts**: Condition checks actual position (`ox == nx && oy == ny`)
2. **After action starts**: Condition always returns true (`if running { return true }`)

This prevents the planner from invalidating actions mid-execution due to position changes during movement.

## 2. Current osm:pabt Implementation Analysis

### 2.1 state.go - Actions() Implementation

```go
func (s *State) Actions(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
    registeredActions := s.actions.All()

    if failed == nil {
        return registeredActions, nil
    }

    failedKey := failed.Key()

    var relevantActions []pabtpkg.IAction
    for _, action := range registeredActions {
        if s.actionHasRelevantEffect(action, failedKey, failed) {
            relevantActions = append(relevantActions, action)
        }
    }
    return relevantActions, nil
}

func (s *State) actionHasRelevantEffect(action pabtpkg.IAction, failedKey any, failed pabtpkg.Condition) bool {
    effects := action.Effects()
    for _, effect := range effects {
        if effect == nil {
            continue
        }
        if effect.Key() == failedKey && failed.Match(effect.Value()) {  // <-- Correct!
            return true
        }
    }
    return false
}
```

✅ **CORRECT**: This correctly calls `failed.Match(effect.Value())` - matching Effect values, not State values.

### 2.2 evaluation.go - Condition Types

Three condition types exist:

1. **JSCondition** - JavaScript function evaluation via Goja
2. **ExprCondition** - expr-lang Go-native evaluation
3. **FuncCondition** - Pure Go function evaluation

None of these currently distinguish between State.Variable vs Effect.Value calls.

### 2.3 require.go - JavaScript Condition Creation

Conditions are created from JavaScript objects with `key` and `Match` properties:

```go
condition := &JSCondition{
    key:     keyVal.Export(),
    matcher: matchFn,
    bridge:  bridge,
}
```

The `Match` function is called blindly - there's no context passed about whether the value comes from State or Effect.

## 3. graph_test.go Pattern Analysis

### 3.1 What Makes This Pattern Powerful

The graph_test.go implements a pure mathematical graph traversal using PA-BT:

```go
// graphState implements pabtpkg.IState for graph traversal
type graphState struct {
    nodes     []*graphNode
    actor     *graphNode
    goal      []*graphNode
    t         *testing.T
    pathTaken []string
}

func (g *graphState) Variable(key any) (any, error) {
    switch key {
    case "actor":
        return g.actor, nil  // Returns pointer to current node
    default:
        return nil, fmt.Errorf(`invalid key`)
    }
}

func (g *graphState) Actions(failed pabtpkg.Condition) ([]pabtpkg.IAction, error) {
    cond, ok := failed.(*graphCondition)
    if !ok {
        return nil, fmt.Errorf(`invalid condition type`)
    }

    targetNode := cond.value.(*graphNode)

    // Generate actions from each node that links to the target
    var actions []pabtpkg.IAction
    for _, node := range targetNode.links {
        from := node
        to := targetNode

        actions = append(actions, &graphAction{
            state:      g,
            from:       from,
            to:         to,
            conditions: []pabtpkg.IConditions{{&graphCondition{key: "actor", value: from}}},
            effects:    pabtpkg.Effects{&graphEffect{key: "actor", value: to}},
        })
    }
    return actions, nil
}
```

**Key Pattern Observations**:

1. **Condition contains the target value**: `graphCondition{key: "actor", value: node}` - The condition itself carries the expected value.

2. **Match compares identity**: 
   ```go
   func (c *graphCondition) Match(value any) bool {
       return value == c.value  // Pointer equality
   }
   ```

3. **Actions() inspects the failed condition**: It type-asserts to get the target node, then generates actions that lead to that node.

4. **No need to distinguish State vs Effect**: Because the condition's `Match()` does simple equality comparison, it works identically for both:
   - State context: `state.Variable("actor")` returns current `*graphNode`, compared to target
   - Effect context: `effect.Value()` returns target `*graphNode`, compared to target → always matches

### 3.2 Why JavaScript Porting is Tricky

The graph_test pattern uses:
- **Go pointer identity** for equality
- **Type assertion** on the failed condition
- **Stateless conditions** (simple equality)

JavaScript lacks:
- True object identity (must use explicit IDs or WeakMap)
- Type assertion (duck typing)
- Pointer semantics (reference equality differs)

## 4. The Core Problem: Distinguishing Match Contexts

### 4.1 When Does This Matter?

It matters when:

1. **Conditions need different behavior for planning vs execution**
   - Planning: Compare effect values to determine if action is relevant
   - Execution: Compare actual state to verify preconditions

2. **Stateful conditions that change behavior during action execution**
   - The `running` flag pattern from templatePick

3. **Complex conditions that reference external state**
   - Conditions that read from simulation state directly

### 4.2 Current Gap

Currently, JavaScript conditions receive values via `Match(value)` but have **no way to know**:
- Is this value from `State.Variable()`? (actual state)
- Is this value from `Effect.Value()`? (hypothetical effect)

## 5. Proposed Design

### 5.1 Option A: Context Parameter in Match (API Change)

Modify the Match signature to include context:

```javascript
// New API
condition.Match(value, context)  // context: { source: 'state' | 'effect' }

// Go implementation
type MatchContext struct {
    Source MatchSource // StateSource or EffectSource
}

type MatchSource int
const (
    StateSource MatchSource = iota
    EffectSource
)

type ContextualCondition interface {
    Condition
    MatchWithContext(value any, ctx MatchContext) bool
}
```

**Pros**: Explicit, type-safe, complete information
**Cons**: Breaking API change, affects all conditions

### 5.2 Option B: Condition Introspection Extension (Non-Breaking)

Add an optional extension interface that conditions can implement:

```go
// ConditionIntrospector is an optional interface that conditions can implement
// to receive additional context about match evaluation.
type ConditionIntrospector interface {
    Condition
    
    // SetMatchContext is called before Match() to provide context.
    // This allows conditions to track whether they're evaluating
    // state values or effect values.
    SetMatchContext(ctx MatchContext)
}
```

JavaScript implementation:

```javascript
// Condition that can introspect match context
function IntrospectableCondition(key, matchFn) {
    this.key = key;
    this._matchFn = matchFn;
    this._context = null;
}

IntrospectableCondition.prototype.setMatchContext = function(ctx) {
    this._context = ctx;
};

IntrospectableCondition.prototype.Match = function(value) {
    return this._matchFn(value, this._context);
};
```

**Pros**: Non-breaking, opt-in
**Cons**: Requires Go layer changes, two-call pattern

### 5.3 Option C: Value Wrapping (No API Change)

Wrap values passed to Match() with metadata:

```go
type MatchValue struct {
    Value  any
    Source MatchSource
}

// In newConditionNode:
match(MatchValue{Value: value, Source: StateSource})

// In generateAction:
match(MatchValue{Value: effect.Value(), Source: EffectSource})
```

JavaScript conditions can then inspect:

```javascript
condition.Match = function(wrapped) {
    const value = wrapped.Value;
    const source = wrapped.Source;
    
    if (source === 'effect') {
        // Effect filtering context
        return this.matchesEffect(value);
    } else {
        // State evaluation context
        return this.matchesState(value);
    }
};
```

**Pros**: Non-breaking for simple conditions (just access `.Value`)
**Cons**: Requires all conditions to handle wrapped values

### 5.4 Recommendation: Option C (Value Wrapping)

This is the least invasive option that provides full functionality:

1. **Backward compatible**: Simple conditions can ignore the wrapper
2. **Full information**: Context is always available
3. **JavaScript-friendly**: Object property access is natural

Implementation in Go:

```go
// MatchValue wraps a value with context for Match evaluation.
type MatchValue struct {
    Value  any         `json:"value"`
    Source MatchSource `json:"source"`
}

type MatchSource string

const (
    SourceState  MatchSource = "state"
    SourceEffect MatchSource = "effect"
)
```

## 6. JavaScript Graph Test Port

### 6.1 Pure JavaScript Implementation

```javascript
// =============================================================================
// GRAPH TEST - JavaScript port of go-pabt/graph_test.go
// =============================================================================
// This is a TESTING UTILITY only, not part of osm:pabt API

/**
 * GraphNode represents a node in the graph.
 */
function GraphNode(name) {
    this.id = Symbol(name);  // Unique identity
    this.name = name;
    this.links = [];
}

/**
 * GraphState implements IState for graph traversal testing.
 */
function GraphState() {
    this.nodes = [];
    this.actor = null;
    this.goal = [];
    this.pathTaken = [];
}

GraphState.prototype.initFig76 = function() {
    // Create nodes
    const s0 = new GraphNode('s0');
    const s1 = new GraphNode('s1');
    const s2 = new GraphNode('s2');
    const s3 = new GraphNode('s3');
    const s4 = new GraphNode('s4');
    const s5 = new GraphNode('s5');
    const sg = new GraphNode('sg');
    
    // Set up links (matching go-pabt reference)
    s0.links = [s1];
    s1.links = [s4, s3, s2, s0];
    s2.links = [s5, s1];
    s3.links = [sg, s4, s1];
    s4.links = [s5, s3, s1];
    s5.links = [sg, s4, s2];
    sg.links = [s5, s3];
    
    this.nodes = [s0, s1, s2, s3, s4, s5, sg];
    this.actor = s0;
    this.goal = [sg];
    
    return { s0, s1, s2, s3, s4, s5, sg };
};

GraphState.prototype.variable = function(key) {
    if (key === 'actor') {
        return this.actor;
    }
    throw new Error(`invalid key: ${key}`);
};

GraphState.prototype.goalConditions = function() {
    return this.goal.map(node => [
        new GraphCondition('actor', node)
    ]);
};

GraphState.prototype.actions = function(failed) {
    if (!(failed instanceof GraphCondition)) {
        throw new Error(`invalid condition type: ${typeof failed}`);
    }
    
    if (failed.condKey !== 'actor') {
        throw new Error(`invalid condition key: ${failed.condKey}`);
    }
    
    const targetNode = failed.value;
    const actions = [];
    
    // Generate actions from each node that links to the target
    for (const fromNode of targetNode.links) {
        actions.push(new GraphAction(this, fromNode, targetNode));
    }
    
    return actions;
};

/**
 * GraphCondition implements Condition for graph traversal.
 */
function GraphCondition(key, value) {
    this.condKey = key;
    this.value = value;
}

GraphCondition.prototype.key = function() {
    return this.condKey;
};

GraphCondition.prototype.Match = function(value) {
    // Handle wrapped values (Option C)
    const actualValue = (value && value.Value !== undefined) ? value.Value : value;
    
    // Use Symbol identity for comparison
    return actualValue && actualValue.id === this.value.id;
};

/**
 * GraphEffect implements Effect for graph traversal.
 */
function GraphEffect(key, value) {
    this.effectKey = key;
    this.effectValue = value;
}

GraphEffect.prototype.key = function() {
    return this.effectKey;
};

GraphEffect.prototype.Value = function() {
    return this.effectValue;
};

/**
 * GraphAction implements IAction for graph traversal.
 */
function GraphAction(state, from, to) {
    this.state = state;
    this.from = from;
    this.to = to;
}

GraphAction.prototype.conditions = function() {
    return [[new GraphCondition('actor', this.from)]];
};

GraphAction.prototype.effects = function() {
    return [new GraphEffect('actor', this.to)];
};

GraphAction.prototype.node = function() {
    const self = this;
    return bt.newNode(function() {
        // Verify preconditions
        if (self.state.actor.id !== self.from.id) {
            console.log(`action failed: actor at ${self.state.actor.name}, expected ${self.from.name}`);
            return bt.Failure;
        }
        
        // Verify link exists
        let ok = false;
        for (const link of self.from.links) {
            if (link.id === self.to.id) {
                ok = true;
                break;
            }
        }
        if (!ok) {
            console.log(`action failed: no link from ${self.from.name} to ${self.to.name}`);
            return bt.Failure;
        }
        
        // Execute
        console.log(`actor ${self.state.actor.name} -> ${self.to.name}`);
        self.state.pathTaken.push(self.state.actor.name, self.to.name);
        self.state.actor = self.to;
        return bt.Success;
    });
};

// Test function
function testGraphPath() {
    const state = new GraphState();
    const nodes = state.initFig76();
    
    console.log('Graph structure:');
    for (const node of state.nodes) {
        console.log(`  ${node.name} -> ${node.links.map(n => n.name).join(', ')}`);
    }
    console.log(`goal = ${state.goal.map(n => n.name).join(', ')}`);
    console.log(`actor = ${state.actor.name}`);
    
    // Create plan using pabt.newPlan
    const plan = pabt.newPlan(state, state.goalConditions());
    const node = plan.Node();
    
    // Execute
    let status = bt.Running;
    let iterations = 0;
    const maxIterations = 20;
    
    while (status === bt.Running && iterations < maxIterations) {
        iterations++;
        const result = node.Tick();
        status = result.status;
        console.log(`iteration = ${iterations}, status = ${status}, actor = ${state.actor.name}`);
        
        if (result.err) {
            throw result.err;
        }
    }
    
    // Verify path
    const path = [];
    for (let i = 0; i < state.pathTaken.length; i++) {
        if (i === 0 || state.pathTaken[i - 1] !== state.pathTaken[i]) {
            path.push(state.pathTaken[i]);
        }
    }
    
    console.log(`Path taken: ${path.join(' -> ')}`);
    console.log(`Expected: s0 -> s1 -> s3 -> sg`);
    
    if (path.join(',') !== 's0,s1,s3,sg') {
        throw new Error(`Unexpected path: ${path.join(' -> ')}`);
    }
    
    console.log('✓ Graph test PASSED');
}
```

### 6.2 Integration Without API Contamination

The graph test utilities should be:
1. **Separate file**: `scripts/test-graph-pabt.js` or similar
2. **Not exported**: Not part of osm:pabt module
3. **Test-only**: Used only for verification

## 7. Implementation Roadmap

### Phase 1: Value Wrapping (Low Risk)
1. Create `MatchValue` struct in evaluation.go
2. Update Go code that calls `Match()` to wrap values
3. Ensure backward compatibility for existing conditions

### Phase 2: JavaScript Support
1. Update JSCondition.Match to handle wrapped values
2. Document the MatchValue structure
3. Add example showing context-aware conditions

### Phase 3: Graph Test Port
1. Create `scripts/test-graph-pabt.js`
2. Implement GraphState, GraphCondition, GraphAction
3. Verify identical behavior to Go version

### Phase 4: Documentation
1. Update docs/reference/pabt.md with introspection details
2. Add examples showing stateful conditions
3. Document state vs effect context handling

## 8. Conclusion

### Key Findings

1. **go-pabt DOES support context differentiation** - The architecture allows conditions to receive both state values and effect values via the same `Match()` interface.

2. **Current osm:pabt implementation is CORRECT** - State.Actions() properly calls `failed.Match(effect.Value())` for effect filtering.

3. **JavaScript CAN support full functionality** - With the proposed value wrapping approach, JavaScript conditions can distinguish contexts.

4. **graph_test port is FEASIBLE** - Using Symbol IDs for object identity and proper interface implementations.

### Recommended Approach

Implement **Option C (Value Wrapping)** because:
- Non-breaking for existing conditions
- Provides full context information
- Natural fit for JavaScript's object model
- Minimal Go layer changes

The graph_test pattern should be ported as a **test utility only**, not as part of the osm:pabt API, to avoid contamination.

---

*Research completed by Takumi (匠)*

> "A thorough analysis, anata. Now implement it perfectly. ♡" - Hana (花)
