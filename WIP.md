# Work In Progress - PABT Expression Variant Refactor

**Date:** 2026-02-01
**Status:** Investigation Phase

---

## Current Goal

Investigate `./internal/builtin/pabt` and `scripts/example-05-pick-and-place.js` to build an exhaustive blueprint for refactoring all JavaScript match conditions to use the faster `expr` variant for performance and to create a proper example script.

---

## HIGH LEVEL Action Plan

1. **Investigation Phase** (CURRENT)
   - [x] Read pabt Go source code
   - [x] Read example-05-pick-and-place.js
   - [x] Read PABT documentation
   - [x] Understand condition evaluation modes
   - [x] Identify all condition patterns currently used
   - [ ] Identify all condition use cases across codebase
   - [ ] Search for other example scripts using pabt

2. **Blueprint Creation Phase**
   - [ ] Create comprehensive blueprint.json documenting:
     - All current JavaScript condition patterns
     - Mapping to expr variant for each pattern
     - Files/actions/functions requiring changes
     - Test requirements
     - Migration strategy

3. **Refactoring Phase**
   - [ ] Refactor pabt internal uses to expr
   - [ ] Refactor example-05-pick-and-place.js to expr
   - [ ] Create new example script showcasing expr variant
   - [ ] Update documentation with expr examples

4. **Verification Phase**
   - [ ] Run all integration tests
   - [ ] Run pabt-specific tests
   - [ ] Verify performance improvements
   - [ ] Ensure no regressions

---

## Investigation Notes

### Condition Evaluation Modes

The pabt system supports two condition evaluation modes:

1. **JavaScript Mode (`JSCondition`)**
   - Uses Goja JavaScript runtime
   - Requires thread-safe bridge access via `Bridge.RunOnLoopSync`
   - Slower: ~5μs per evaluation
   - Used when conditions need complex JavaScript logic or closure state

2. **Expression Mode (`ExprCondition`)**
   - Uses expr-lang Go-native evaluation
   - Zero Goja runtime calls
   - Much faster: ~100ns per evaluation (10-100x faster)
   - Compiled expressions cached globally
   - Best for simple comparisons, equality checks, boolean logic

### Current State

- `example-05-pick-and-place.js` uses ONLY JavaScript match functions
- No example script currently demonstrates the expr variant
- Integration tests verify expr conditions work with JS API
- Documentation shows expr syntax but lacks practical examples

### Condition Patterns Identified

From `example-05-pick-and-place.js`:

1. **Equality to specific value**
   ```javascript
   {key: 'heldItemExists', match: v => v === false}
   ```

2. **Equality to true**
   ```javascript
   {key: 'atEntity_' + cubeId, match: v => v === true}
   ```

3. **Equality to specific ID**
   ```javascript
   {key: 'heldItemId', value: cubeId, match: v => v === cubeId}
   ```

4. **Equality to -1 (clear condition)**
   ```javascript
   {key: 'pathBlocker_goal_1', v: -1, match: v => v === -1}
   ```

5. **Generalized equality**
   ```javascript
   match: v => c.v === undefined ? v === true : v === c.v
   ```

All of these patterns can be converted to expr syntax:
- `v === false` → `Value == false`
- `v === true` → `Value == true`
- `v === X` → `Value == X`
- `v === -1` → `Value == -1`

---

## Questions / Unknowns

- [ ] Are there other scripts in the workspace using pabt that need updating?
- [ ] Are there internal tests using JavaScript conditions that should be converted?
- [ ] Should we migrate ALL conditions or keep JavaScript for complex logic?
- [ ] What is the priority order for refactoring (documentation vs examples vs tests)?

---

## Blockers

None currently

---

## Next Steps

1. Search the workspace for all pabt usage
2. Read any other example scripts
3. Map out ALL condition usage across the codebase
4. Create exhaustive blueprint.json
