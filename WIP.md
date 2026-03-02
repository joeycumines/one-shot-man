# WIP: T01-T62 ALL DONE — Continuing expansion cycle

## Status: T61-T62 COMMITTED (Rule of Two PASSED)

### Commits:
- a31a25f: T42-T48 (27 BT/template/utility tests + production fixes)
- 5b756ac: T49 (pre-compute import maps in assessIndependence)
- f5f2521: T50-T55 (performance + portability batch)
- f16b959: T59-T60 (scoring penalties + cleanupBranches worktree fix)
- 99f2a42: blueprint+WIP update
- 61a114b: staged T59/T60 test+script files committed
- PENDING: T61-T62 commit (analysis tests: splitsAreIndependent + extractGoPkgs)

### Blueprint State:
- T01-T62: Done (all committed or pending commit)
- T37: Blocked (Claude auth — needs `claude login` or ANTHROPIC_API_KEY)
- T63-T74: Not Started (planned in blueprint)

### T61-T62 Implementation:
- T61: 7 subtests in TestSplitsAreIndependent (pr_split_analysis_test.go)
  - no_dir_overlap_returns_true, exact_dir_overlap_returns_false, both_empty_files_returns_true
  - A_imports_B_package_returns_false, B_imports_A_package_returns_false
  - non_Go_files_dir_overlap_returns_false, no_cross_import_independent
- T62: 5 subtests in TestExtractGoPkgs (pr_split_analysis_test.go)
  - go_files_produce_dir_keys, non_go_files_ignored, null_files_returns_empty
  - root_level_go_file_with_modulePath, empty_modulePath_no_qualified_keys

### Key learnings:
- Config property names are claudeCommand/claudeModel, NOT command/model
- validateSplitPlan takes stages array directly, not object wrapper
- detect functions build path as 'go.mod' not './go.mod' when dir='.'
- selectStrategy scoring: splitScore decays at 8+ groups, maxSizeScore decays when group>maxPerGroup
- cleanupBranches: git checkout baseBranch can fail in worktree — fallback to --detach HEAD
- GIT LESSON: Always verify `git add` for new files before committing!
- require('osm:os') caching: mock injection works because require returns cached module reference

### Next:
- T63: resolveConflicts chained-strategy retry loop (🔴 High risk)
- T64: gitAddChangedFiles rename/quoted path parsing
- T65-T74: See blueprint.json for full list
