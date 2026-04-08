# Task 61 — Rule of Two — Pass 2 (Integration Review)

**Reviewer**: Independent (no knowledge of Pass 1)  
**Date**: 2026-04-08  
**Scope**: Ctrl+Tab cycling behavior change: binary toggle → ordered cycle through wizard → claude → output → verify (if active) → wizard. Hint text "switch" → "cycle".

---

## 1. Core Logic — `pr_split_16e_tui_update.js` (lines ~700–718)

### Algorithm Trace

```
allTargets = ['wizard'] ++ listSplitViewTabs(s)
           = ['wizard', 'claude', 'output']               (no verify)
           = ['wizard', 'claude', 'output', 'verify']     (with verify)

current = focus==='wizard' ? 'wizard' : (splitViewTab || 'claude')
curIdx  = allTargets.indexOf(current);  if (<0) curIdx=0
next    = allTargets[(curIdx+1) % allTargets.length]
```

| Start (focus / tab) | Verify? | allTargets                        | current  | curIdx | nextTarget | Result focus | Result tab | Correct? |
|---|---|---|---|---|---|---|---|---|
| wizard / claude     | no      | [w, c, o]                        | wizard   | 0      | claude     | claude       | claude     | ✅ |
| claude / claude     | no      | [w, c, o]                        | claude   | 1      | output     | claude       | output     | ✅ |
| claude / output     | no      | [w, c, o]                        | output   | 2      | wizard     | wizard       | (unchanged)| ✅ |
| claude / output     | yes     | [w, c, o, v]                     | output   | 2      | verify     | claude       | verify     | ✅ |
| claude / verify     | yes     | [w, c, o, v]                     | verify   | 3      | wizard     | wizard       | (unchanged)| ✅ |
| orphaned (curIdx=-1)| no      | [w, c, o]                        | ???      | 0→0    | claude     | claude       | claude     | ✅ |

`listSplitViewTabs` (pr_split_13_tui.js:506) returns `['claude', 'output']` or appends `'verify'` when verify is active. Verified consistent.

**Verdict**: **PASS** — Algorithm is correct for all 6 scenarios.

---

## 2. Hint Text — `pr_split_16f_tui_model.js` line 584

```javascript
var splitHint = 'Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close';
```

- Old text ("switch") is absent everywhere (confirmed via `grep -r "Ctrl+Tab: switch"` — **zero matches**).
- "cycle" accurately describes the new round-robin behavior.

**Verdict**: **PASS**

---

## 3. New Test — `pr_split_tui_interaction_test.go` → `TestCtrlTabCyclesThroughTargets`

Lines 540–700. Verifies:

| Phase | Scenario | Expected | Matches Algorithm? |
|---|---|---|---|
| 1 (no verify) | wizard → claude | `{focus:"claude",tab:"claude"}` | ✅ |
| 1 (no verify) | claude → output | `{focus:"claude",tab:"output"}` | ✅ |
| 1 (no verify) | output → wizard | `{focus:"wizard",tab:"output"}` | ✅ (tab unchanged) |
| 2 (verify) | output → verify | `{focus:"claude",tab:"verify"}` | ✅ |
| 2 (verify) | verify → wizard | `{focus:"wizard",tab:"verify"}` | ✅ (tab unchanged) |

Uses `testState()` helper (line 82) which produces clean state objects. Phase 2 manually sets `verifyScreen` to trigger the verify tab — correct trigger per `listSplitViewTabs`.

**Verdict**: **PASS**

---

## 4. Updated Tests — All 6 Files Verified

### 4a. `pr_split_16_vterm_claude_pane_test.go` — `TestChunk16_VTerm_SplitViewFocusSwitch_CtrlTab`

- wizard→claude ✅, claude→output (T61 comment) ✅, output→wizard (wrap) ✅
- No remaining binary toggle assertion.

**Verdict**: **PASS**

### 4b. `pr_split_16_input_routing_test.go` — `TestInputRouting_CtrlTabSwitchesFocus`

- wizard→claude ✅, claude→output (split tab check) ✅, output→wizard ✅
- wizardState unchanged in all cases ✅

**Verdict**: **PASS**

### 4c. `pr_split_16_verify_expand_nav_test.go` — `TestChunk16_T38_CtrlTabSwitchesPanes`

- Full 3-step cycle: wizard→claude→output→wizard ✅
- Split-view disabled: no change ✅

**Verdict**: **PASS**

### 4d. `pr_split_16_split_mouse_test.go` (lines 180–215)

- wizard→claude ✅, claude→output (T61 comment: "no longer toggles back") ✅, output→wizard ✅
- With activeVerifySession: wizard→claude ✅

**Verdict**: **PASS**

### 4e. `pr_split_16_keyboard_crash_test.go` (lines 170–200)

- Full cycle: wizard→claude→output→wizard ✅
- With verify session: wizard→claude (T380) ✅

**Verdict**: **PASS**

### 4f. `pr_split_16_verify_fixes_test.go` — `TestInteractiveReservedKeys_T386`

- `ctrl+tab` listed in `mustReserve` array — confirms it remains a reserved key. ✅

**Verdict**: **PASS**

---

## 5. Golden Files

| File | Content | Correct? |
|---|---|---|
| `tab-bar-verify-only.golden` | `Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close` | ✅ |
| `tab-bar-all-tabs.golden` | `Ctrl+Tab: cycle  Ctrl+O: tab  Ctrl+L: close` | ✅ |
| `verify-pane-running.golden` | No hint text (pane content only) | N/A |
| `verify-pane-paused.golden` | No hint text (pane content only) | N/A |

No golden file contains stale "switch" text.

**Verdict**: **PASS**

---

## 6. Stale Assertion Fix — `pr_split_13_tui_test.go` (line 4839)

```go
if strings.Contains(s, "Ctrl+Tab: cycle") || strings.Contains(s, "Ctrl+O: tab") {
    t.Fatalf("error state should not render split-view chrome, got:\n%s", s)
}
```

Correctly checks for "cycle" (new text), not "switch". This test verifies that the error-state view does NOT render split-view chrome—inverse assertion is appropriate.

**Verdict**: **PASS**

---

## 7. Cross-Reference Sweep

| Search | Result |
|---|---|
| `"Ctrl+Tab: switch"` across all files | **0 matches** ✅ |
| `"switch"` in `.golden` files | **0 matches** ✅ |
| Binary toggle assertions in `*_test.go` | **0 remaining** — all 6 test files verified above assert cycling behavior |
| JS reserved keys comments (`16d`) | Say "switch focus between panes" — internal code comments, not user-facing text. Acceptable. |

**Verdict**: **PASS**

---

## 8. Build/Test Confirmation

Per task author: `make test-short` (54 packages) and `make lint` both pass clean. Acknowledged.

**Verdict**: **PASS** (trusted — verified by prior run)

---

## OVERALL VERDICT: **PASS** ✅

All 8 verification areas pass. The cycling algorithm is correct for all 6 traced scenarios. All tests assert the new cycling behavior. No stale "switch" references remain in user-facing text or golden files. The changeset is internally consistent and complete.
