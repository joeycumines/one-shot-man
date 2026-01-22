# WIP - Pick-and-Place DYNAMIC Obstacle Handling

## Current Goal
Complete architectural overhaul: Remove ALL hardcoded blockade handling, implement DYNAMIC obstacle discovery per PA-BT principles.

## Status
- **Date**: 2026-01-22
- **Phase**: IMPLEMENTATION - Phase 1 starting

## Reference
- See `./blueprint.json` for EXHAUSTIVE task list (5 phases, 27 tasks)
- See `./planning.md` for full analysis with 3 subagent reviews

## Architecture Principle
**Agent MUST NOT bake in ANY knowledge of blockade layout.** Obstacles are discovered dynamically via pathfinding and cleared to arbitrary locations.

## Completed
- ✅ Removed actionCache (ISSUE-001)
- ✅ Changed threshold to 1.5 (ISSUE-002)
- ✅ Added infinite loop guard (ISSUE-004)
- ✅ Created planning.md with 3 reviews
- ✅ Updated blueprint.json exhaustively

## Current Focus
**Phase 1: Infrastructure Prerequisites**
- P1-T01: Add findFirstBlocker() function
- P1-T02: Remove GOAL_BLOCKADE_IDS
- P1-T03: Remove DUMPSTER_ID
- P1-T04: Remove isInBlockadeRing()
- P1-T05: Remove goalBlockade_X_cleared keys
- P1-T06: Remove goalPathCleared key

## High Level Action Plan
1. ✅ Re-read review.md
2. ✅ Re-read example script  
3. ✅ Create planning.md with 3 reviews
4. ✅ Update blueprint.json exhaustively
5. ⏳ Implement Phase 1: Infrastructure
6. ⏳ Implement Phase 2: Action Refactoring
7. ⏳ Implement Phase 3: ActionGenerator Overhaul
8. ⏳ Implement Phase 4: Cleanup
9. ⏳ Run Phase 5: Testing

## Key Insight
The current implementation is WRONG because it:
- Pre-defines `GOAL_BLOCKADE_IDS = [100..115]` 
- Forces obstacles to `DUMPSTER_ID` at (8,4)
- Uses "God-Precondition" with 16 explicit blockade conditions

The CORRECT approach:
- Dynamic blocker discovery via pathfinding
- Place obstacles at ANY free adjacent cell
- Only minimal preconditions (hands empty, at entity)
- Let failures trigger replanning
