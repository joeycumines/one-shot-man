# Task 61 — Rule of Two: Pass 1 (Sanity Review)

**Reviewer:** Blind code reviewer (no prior context)  
**Date:** 2026-04-08  
**Scope:** Tab-based Ctrl+Tab cycling — 12 files, ~250 insertions, ~26 deletions  

## Acceptance Criteria Recap

> Ctrl+Tab must cycle through ALL focusable targets: wizard → claude → output → verify (if active) → wizard → ...  
> Ctrl+O remains as tab-only cycling within the bottom pane.  
> Tab bar hint text updated to "cycle". All existing tests updated, new test added.

---

## Per-File Review

### 1. `pr_split_16e_tui_update.js` (lines 697–720) — **PASS**

Core cycling logic:

```js
var allTargets = ['wizard'];
var tabs = listSplitViewTabs(s);   // → ['claude', 'output'] or ['claude', 'output', 'verify']
for (var ti = 0; ti < tabs.length; ti++) allTargets.push(tabs[ti]);
var current = (s.splitViewFocus === 'wizard') ? 'wizard' : (s.splitViewTab || 'claude');
var curIdx = allTargets.indexOf(current);
if (curIdx < 0) curIdx = 0;          // ← orphaned tab fallback
var nextTarget = allTargets[(curIdx + 1) % allTargets.length];
```

- **Correctness:** Builds dynamic target list. Modulo wrap handles all sizes (2, 3, 4 targets).
- **Edge case — orphaned verify tab:** If `splitViewTab === 'verify'` but verify isn't active, `indexOf` returns -1 → falls back to index 0 (wizard) → advances to claude. Graceful recovery, no crash. ✓
- **Edge case — `splitViewTab` is falsy:** `|| 'claude'` default. Always valid. ✓
- **Ctrl+O unchanged:** Separate handler at line 810 still cycles only `listSplitViewTabs(s)` — bottom pane tabs only. ✓

### 2. `pr_split_16f_tui_model.js` (line 584) — **PASS**

```js
var splitHint = 'Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close';
```

"switch" → "cycle". Verified in source. ✓

### 3. `pr_split_tui_interaction_test.go` — New `TestCtrlTabCyclesThroughTargets` — **PASS**

End-to-end test through `prSplit._wizardUpdateImpl`. Two phases:

| Phase | Steps | Expected |
|-------|-------|----------|
| Without verify | wizard→claude→output→wizard | 3-step wrap ✓ |
| With verify (`verifyScreen` set) | output→verify, verify→wizard | 4-step wrap ✓ |

- Uses `skipSlow(t)` guard. ✓
- Parallel. ✓
- Manually constructs `verifyState` with `verifyScreen: '$ running verify...'` to trigger verify tab presence. ✓
- Asserts both `splitViewFocus` and `splitViewTab` at each step. ✓

### 4. `pr_split_16_verify_expand_nav_test.go` — `TestChunk16_T38_CtrlTabSwitchesPanes` — **PASS**

Updated assertions:
- wizard → claude (1st Ctrl+Tab)
- claude → output with `focus='claude'`, `tab='output'` (2nd Ctrl+Tab) ✓
- output → wizard (3rd Ctrl+Tab) ✓
- Disabled split-view: no-op ✓

### 5. `pr_split_16_split_mouse_test.go` — `TestChunk16_SplitView_TabFocusSwitch` — **PASS**

Updated assertions:
- wizard → claude → output → wizard (3-step cycle, no verify) ✓
- With active verify session (`activeVerifySession` mock): wizard → claude (T380 guard removed) ✓
- Comment notes "T61: no longer toggles back to wizard" ✓

### 6. `pr_split_16_keyboard_crash_test.go` (lines 170–210) — **PASS**

Updated assertions:
- wizard → claude → output → wizard (3-step cycle) ✓
- With verify session: wizard → claude (T380) ✓

### 7. `pr_split_16_input_routing_test.go` — `TestInputRouting_CtrlTabSwitchesFocus` — **PASS**

Updated assertions (two sub-tests):
- wizard → claude ✓
- claude → output (T61) ✓
- output → wizard (no verify, wrap) ✓
- `wizardState` preserved across all transitions ✓

### 8. `pr_split_16_vterm_key_forwarding_test.go` (lines 740–780) — **PASS**

Updated assertion:
- From claude tab: Ctrl+Tab advances to output (not back to wizard) ✓
- NOT forwarded to child PTY (`__writtenBytes.length === 0`) ✓

### 9. `pr_split_16_vterm_claude_pane_test.go` — `TestChunk16_VTerm_SplitViewFocusSwitch_CtrlTab` — **PASS**

Updated assertions (3-step cycle):
- wizard → claude ✓
- claude → output (`focus='claude'`, `tab='output'`) with T61 comment ✓
- output → wizard (wrap) ✓

### 10. `testdata/golden/tab-bar-all-tabs.golden` — **PASS**

```
─┤    Claude     Output   Verify  · ▲ Wizard · Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close ├─
```

### 11. `testdata/golden/tab-bar-verify-only.golden` — **PASS**

```
─┤    Claude     Output   Verify  · ▲ Wizard · Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close ├─
```

### 12. `pr_split_13_tui_test.go` (line 4839) — **PASS**

Fixed stale negative assertion:

```go
if strings.Contains(s, "Ctrl+Tab: cycle") || strings.Contains(s, "Ctrl+O: tab") {
    t.Fatalf("error state should not render split-view chrome, got:\n%s", s)
}
```

Previously used `"Ctrl+Tab: switch"` — now matches the actual rendered text. ✓

---

## Codebase-Wide Stale Reference Search

| Search Pattern | Scope | Matches |
|---|---|---|
| `Ctrl+Tab: switch` | Entire workspace | **0** ✓ |
| `Ctrl+Tab: cycle` | `internal/command/**` | 4 (2 golden files, 1 JS source, 1 Go test) — all correct |
| `binary.*toggle\|wizard.*↔.*pane` | `internal/command/**` | **0** ✓ |

### Minor Observations (NOT failures)

1. **Code comments** in `pr_split_16d_tui_handlers_claude.js` (lines 579, 608) say `// switch focus between panes`. This uses "switch" as a verb (to change/alternate), not as the old UI label. The behavior is still legitimately described as "switching focus." Updating these comments would be a cosmetic improvement but is not functionally stale.

2. **Test function names** like `TestChunk16_SplitView_TabFocusSwitch` and `TestInputRouting_CtrlTabSwitchesFocus` retain "Switch" in identifiers. Their assertion logic is fully updated. Renaming would be purely cosmetic.

---

## Summary

| Criterion | Status |
|---|---|
| Cycling logic correct (wizard → claude → output → [verify] → wizard) | ✅ |
| Edge case: orphaned verify tab handled | ✅ |
| Edge case: no verify tab handled | ✅ |
| Ctrl+O unchanged (tab-only cycle) | ✅ |
| Hint text "switch" → "cycle" | ✅ |
| Golden files consistent | ✅ |
| All 7 existing tests updated with cycling assertions | ✅ |
| New comprehensive test added (with/without verify) | ✅ |
| pr_split_13_tui_test.go stale assertion fixed | ✅ |
| Zero remaining stale "Ctrl+Tab: switch" references | ✅ |

## OVERALL VERDICT: **PASS** ✅

No functional issues found. All acceptance criteria met. All assertions match the new cycling behavior. No stale references remain.
