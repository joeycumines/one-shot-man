# WIP - Current Task

## Current Goal

REVIEW ALL tasks from T001 onwards using Rule of Two (2+ subagents per task).
Failing to meet 100% unit test coverage, or failing to include integration testing whenever useful, is a review failure, as is failing to explicitly check these (you can use a separate subagent).
Failing to _verify_ that the FULL build - all checks - pass on ALL platforms is a review failure.
Do not mark any task "Done" until you have two contiguous, issue-free reviews.
Reviews must be issue-free. All three platforms (Linux, macOS, Windows) must be verified for the full build, if there's even a small chance of a platform-specific issue.
Separately review for alignment and fitness for purpose.
COMMIT immediately after all these conditions pass.

## Status

SLOPPED. See blueprint.json.

## High Level Action Plan

4. 🔄 Begin Rule of Two reviews from T001 (2 subagents per task)
5. ⬜ Fix all review findings
6. ⬜ Commit verified work INCREMENTALLY

See `./blueprint.json` for the full task list.
