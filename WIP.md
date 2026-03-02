# WIP: T01-T60 ALL DONE — Expansion cycle needed

## Status: T56-T60 COMMITTED

### Commits:
- a31a25f: T42-T48 (27 BT/template/utility tests + production fixes)
- 5b756ac: T49 (pre-compute import maps in assessIndependence)
- f5f2521: T50-T55 (performance + portability batch)
- f16b959: T59-T60 (scoring penalties + cleanupBranches worktree fix)
- NEXT: Amend/commit to include pr_split_autofix_strategy_test.go (T56-T58)

### Blueprint State:
- T01-T60: Done (all committed)
- T37: Blocked (Claude auth — needs `claude login` or ANTHROPIC_API_KEY)
- T61+: Not yet planned — expansion cycle needed

### Key learnings:
- Config property names are claudeCommand/claudeModel, NOT command/model
- validateSplitPlan takes stages array directly, not object wrapper
- detect functions build path as 'go.mod' not './go.mod' when dir='.'
- selectStrategy scoring: splitScore decays at 8+ groups, maxSizeScore decays when group>maxPerGroup
- cleanupBranches: git checkout baseBranch can fail in worktree — fallback to --detach HEAD
- GIT LESSON: Always verify `git add` for new files before committing!

### Next:
- Plan T61+ expansion tasks
- Consider: createPRs edge cases, resume UX, error recovery, fix tests
- T04b (real E2E test) still not started — needs Claude auth
- Then: Expansion cycle for T61+
