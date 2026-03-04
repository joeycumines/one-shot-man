# WIP: R21 DONE — Dead BT functions culled from chunks.

## Status: R01-R21 Done (22 tasks). R22 next: vestigial comments + inconsistencies.

### Session: 2026-03-04 06:46:25 (9-hour mandate)
### Branch: main (working directory has uncommitted changes)

### What R21 did:
1. **Removed 13 dead BT functions from pr_split_11_utilities.js:**
   - 8 BT node factories: createAnalyzeNode, createGroupNode, createPlanNode,
     createSplitNode, createVerifyNode, createEquivalenceNode,
     createSelectStrategyNode, createWorkflowTree
   - 5 BT templates: btVerifyOutput, btRunTests, btCommitChanges,
     btSplitBranch, verifyAndCommit
2. **Updated pr_split_12_exports.js:** Removed 13 dead names from EXPECTED_EXPORTS
3. **Recreated pr_split_bt_test.go:** Only 11 surviving tests (visualization/diff/report)
4. **Cleaned pr_split_11_utilities_test.go:** Removed BTNodeFactories test,
   cleaned AllExportsPresent expected map
5. **Cleaned chunkCompatShim in pr_split_test.go:** Removed 13 dead proxy entries
6. **Cleaned claudemux/pr_split_test.go:** Removed BTWorkflow_WithCompilation,
   CreateWorkflowTree, and BT factory names from ExportedFunctions
7. **Cleaned claudemux/templates_test.go:** Removed 12 dead BT template tests,
   only TestTemplates_ModuleLoads survives
8. **Documented in scratch/pr-split-slop-removed.md**
9. **Verified:** go build passes, 594 command tests GREEN, 721 claudemux tests GREEN

### Next Task: R22 — Vestigial comments and inconsistencies in JS chunks
- Sweep all 14 chunk files for stale task-number comments (T04a, T12, T55, etc.)
- Remove completed TODOs
- Standardize error return formats
- Remove dead variables and console.log statements
