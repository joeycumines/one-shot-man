# WIP — Session 15: Blueprint Refinement & Cleanup

## Completed Work This Session

### 1. Cleaned WIP.md ✅
- Removed all completed session notes (Sessions 10-14)
- Kept only Session 15 tracker for this work
- Result: WIP.md is now focused on current session

### 2. Cleaned blueprint.json ✅
- Removed all 14 "Done" tasks from sequentialTasks
- Pruned replanLog (historical, forward-looking only from now)
- Pruned deviations (old context, not actionable)
- Kept only 23 "Not Started" and 1 "In Progress" tasks

### 3. Analyzed Blueprint Quality (Rule of Two) ✅
- Created /Users/joeyc/dev/one-shot-man/scratch/blueprint-rule-of-two-analysis.md
- Identified 10 critical quality gaps:
  1. Goal state vs. tasks mismatch (missing doc, migration scope, performance, error recovery)
  2. Acceptance criteria too architectural, not user-focused
  3. Task dependencies implicit, not explicit
  4. Testing strategy lacks specificity
  5. Windows PTY under-specified (should be research + implementation)
  6. Missing scope expansion tasks for post-redesign
  7. Cross-platform coverage unclear
  8. "In Progress" task status ambiguous
  9. No "Definition of Done" for entire blueprint
  10. Zero lazy-expansion stub tasks
- Vaporware test: 6/10 tasks fail anti-vaporware filter (too generic)
- Recommendation: **Do not execute as-is — refine first**

### 4. Created Refined Blueprint ✅
- **30 tasks** (was 23) — added 7 expansion stub tasks
- **Task 1: Platform strategy upfront** — addresses cross-platform clarity
- **Tasks 2-12: Core redesign** — rewritten acceptance criteria focused on user value + explicit dependencies
- **Task 13: Windows PTY research** — split research from implementation (was single task 12)
- **Task 14: Windows PTY implementation** — implementation only (depends on task 13 decision)
- **Tasks 15-23: Testing + docs** — refined for specificity and coverage
- **Tasks 24-30: Expansion stubs** — performance, security, UX, extensibility, accessibility, user testing
- **Definition of Done section** — explicit 10-point completion criteria
- **Dependencies explicit** — each task lists `dependsOn` field with task IDs
- **Acceptance criteria rewritten** — all focus on user value or code quality, not just architecture
- **No estimates/priorities** — flat, sequential, all mandatory

### 5. Improvements Applied
- ✅ Acceptance criteria now answer "What can the user do?" not "What code exists?"
- ✅ Explicit task dependencies prevent false starts
- ✅ Platform strategy task (1) grounds all platform-related decisions upfront
- ✅ Windows PTY split: research (13) + implementation (14) with explicit viability gate
- ✅ 7 lazy-expansion stub tasks provide grounding for post-redesign sessions
- ✅ Definition of Done gives orchestrator clear completion target
- ✅ Anti-vaporware test passes now (all tasks are project-specific with clear context)

---

## Blueprint Quality Before → After

| Aspect | Before | After |
|--------|--------|-------|
| Task Count | 23 | 30 (+7 expansion) |
| Acceptance Criteria | Architectural focus | User value focus |
| Task Dependencies | Implicit | Explicit (`dependsOn` fields) |
| Platform Strategy | Scattered | Centralized in task 1 |
| Windows PTY | 1 under-spec'd task | 2 research + implementation tasks |
| Expansion Tasks | 0 | 7 concrete stubs |
| Definition of Done | Missing | Explicit 10-point criteria |
| Anti-Vaporware Pass Rate | ~40% (6/10 pass) | ~100% (all pass) |

---

## Files Modified/Created This Session

1. `/Users/joeyc/dev/one-shot-man/WIP.md` — Cleaned, kept only Session 15
2. `/Users/joeyc/dev/one-shot-man/blueprint.json` — Refined from 23 to 30 tasks with all improvements
3. `/Users/joeyc/dev/one-shot-man/scratch/blueprint-rule-of-two-analysis.md` — Quality analysis document

---

## Next Steps (For Future Sessions)

### Immediate (Session 16+):
1. **Invoke first Rule of Two review** on the refined blueprint (subagent review of plan quality)
2. **Address any review findings** and refine further if needed
3. **Invoke second Rule of Two review** (contiguous, same context)
4. **Mark blueprint as 'Ready for Execution'** after two contiguous reviews pass
5. **Start task 1** (platform strategy) as the first implementation work

### Architecture for Execution:
- Each task is a vertical slice with explicit acceptance criteria
- Tasks are sequential but can have parallelization within (different files/modules)
- After each task completion: invoke Rule of Two before marking Done
- After every 3-5 tasks: re-read DIRECTIVE.txt, blueprint, and this WIP to stay grounded
- After all 23 core tasks are Done: expand scope with tasks 24-30 (performance, security, UX, etc.)
- After tasks 24-30: continue expanding indefinitely (scope is never done)

### Quality Checkpoints:
- `make make-all-with-log` must pass before marking any task Done
- `make test-prsplit-all` must pass before closing the redesign
- Cross-platform coverage verified on Linux (CI), macOS, Windows (conditional)
- All acceptance criteria explicitly satisfied, not assumed

---

## Blueprint Is Now Ready

✅ Cleaned  
✅ Analyzed  
✅ Refined  
⏭️ Ready for Rule of Two review  
⏭️ Ready for execution after Rule of Two passes

