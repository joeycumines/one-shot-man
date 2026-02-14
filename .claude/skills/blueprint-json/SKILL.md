---
name: blueprint-json
description: Manages blueprint.json — the exhaustive, flat, sequential task tracker that serves as the structural tether to reality across sessions. Use when user says "blueprint", "blueprint.json", "create blueprint", "update blueprint", "refine blueprint", "expand scope", "track tasks", "plan tasks", "baseline blueprint", or at project start / session resumption. Also use when the user asks to plan or track work exhaustively, or when any task list needs to be created or maintained as a JSON artifact. This skill applies whenever blueprint.json or baseline.blueprint.json is read, written, validated, or referenced.
license: MIT
---

# Blueprint JSON

`blueprint.json` is the single source of truth for all planned, in-progress, and completed work. If a task is not in `blueprint.json`, it is not being done.

The core value of this skill is **forced verification at a structural level**: the agent is compelled to check whether all tasks have actually been completed, and the persistent JSON artifact provides a hook point to expand the list of checks and tasks. Over long context windows, without this structural tether, agents drift, skip steps, and declare "done" prematurely.

## Hard Constraints

These are non-negotiable. Violating any of them is a failure state.

1. **Flat, sequential task list.** Tasks in `sequentialTasks` are a flat array executed in order. One level of sub-items is permitted for grouping only — never deeper.
2. **NO estimates.** No effort estimates, time estimates, duration estimates, story points, t-shirt sizes, or any proxy for cost. Ever. Their presence is actively detrimental to agent progress.
3. **NO priorities.** No priority fields, severity ratings, importance rankings, or any ordering signal other than sequence position. Their presence exacerbates agent laziness.
4. **NO stopping.** The cycle is indefinite. There is always more work. Scope must always expand. Concluding that work is "done" or "good enough" is forbidden.
5. **Incremental updates.** Status must be updated after every sub-task, not batched. The next session's agent reads this file cold — stale state is fatal.
6. **Scope must expand.** After completing all current tasks, ideate new improvements, refinements, and features. Add them. Repeat.
7. **Deviations logged.** Any deviation from the plan must be recorded within the plan itself. The plan must be reassessed holistically after any change.

## Structure

The generic baseline template is in `references/baseline-template.json`. Projects may also maintain a **`baseline.blueprint.json`** in the project root — a project-specific template with pre-filled `globalAlerts`, `continuousVerification`, `mandatoryDirectives`, etc. tailored to the project's tooling. When both exist, prefer the project-specific baseline.

The key sections are:

- **`meta`** — File identity, source reference, and the maintenance instruction (keep the plan up to date; deviations logged within the plan).
- **`globalAlerts`** — Critical operational warnings. Customize these for the project's specific tooling constraints and build system.
- **`mandatoryDirectives`** — Operational directives for context and resource management during execution.
- **`statusSection`** — Current high-level state, updated as work progresses.
- **`sequentialTasks`** — The flat, ordered task array. Each task has a `task`, `description`, and `status` field. Status is one of: `"Not Started"`, `"In Progress"`, `"Done"`.
- **`continuousVerification`** — Standing verification mandate. Define project-specific build/test commands and coverage requirements here.
- **`finalEnforcementProtocol`** — Post-execution warnings, typically repeating critical tooling constraints for reinforcement.

## Lifecycle

### Create

1. If `baseline.blueprint.json` exists in the project root, use it as the starting template. Otherwise, read `references/baseline-template.json` for structure.
2. Populate `sequentialTasks` exhaustively — enumerate every task between the current state and the goal. Err on the side of too many tasks.
3. Ensure every task description is clear enough that a cold-start agent can execute it without additional context.
4. If starting from the generic template, customize `globalAlerts`, `mandatoryDirectives`, and `continuousVerification` for the project's tooling and environment.

### Maintain

1. Update task `status` immediately upon starting or completing each task.
2. Update `statusSection.currentState` to reflect the current high-level position.
3. Log deviations, blockers, or discoveries as amendments to affected tasks.

### Refine

1. Prune tasks whose status is `"Done"` only if they add no future value (e.g. one-off setup steps).
2. Rewrite unclear task descriptions — the next agent reads them cold.
3. Split tasks that are too coarse. Merge tasks that are redundant.
4. Perform a holistic reassessment: does the full sequence still make sense end-to-end?

### Expand

1. After reaching stable-state (all current tasks complete, verified), identify the next frontier.
2. Add new tasks: improvements, integration tests, performance work, documentation, new features.
3. The task list must never be empty. If it is, you have not expanded enough.

### Verify

Before marking a task "Done" or committing, invoke the `strict-review-gate` skill if it is available. The blueprint provides the task-level gate; the review gate provides the correctness gate. They are complementary.

### Produce / Refine Project Baseline

When the user asks to create or refine a `baseline.blueprint.json`:

1. Start from the generic `references/baseline-template.json` (or the existing `baseline.blueprint.json` if refining).
2. Fill or update `globalAlerts`, `mandatoryDirectives`, `continuousVerification`, and `finalEnforcementProtocol` with the project's specific tooling, build commands, and constraints.
3. Leave `sequentialTasks` empty and `statusSection.currentState` blank — these are populated per-blueprint, not in the baseline.
4. Write the result to `baseline.blueprint.json` in the project root.

## Session Resumption

`blueprint.json` is designed to be the first file read at the start of any new session. The agent should:

1. Read `blueprint.json` before doing any other work.
2. Resume from the first task whose status is not `"Done"`.
3. If the project uses a separate session-state file (e.g. for stack traces, file paths, immediate next steps), read that too.

## Anti-Patterns

**Blueprint must NOT contain any of these. If found, remove them immediately.**

```json lines
// WRONG — contains estimate
{
  "task": "Implement auth module",
  "estimatedEffort": "2 hours",
  "status": "Not Started"
}

// WRONG — contains priority
{
  "task": "Fix login bug",
  "priority": "P0",
  "severity": "Critical",
  "status": "Not Started"
}

// WRONG — nested task hierarchy
{
  "task": "Backend work",
  "subtasks": [
    {
      "task": "Database",
      "subtasks": [
        {
          "task": "Schema migration"
        }
      ]
    }
  ]
}
```

**Correct form:**

```json
{
  "task": "Implement auth module: JWT token generation and validation",
  "description": "Create auth/jwt.go with Sign() and Verify() functions. Add unit tests with 100% coverage. Integrate with existing middleware chain.",
  "status": "Not Started"
}
```

## Troubleshooting

- **Empty `sequentialTasks`**: The blueprint was not populated. Re-read the goal and enumerate ALL tasks between current state and completion.
- **Stale task statuses**: You forgot incremental updates. Audit every task — does its status reflect reality?
- **Tasks contain estimates or priorities**: Remove them immediately. They serve no purpose and cause harm.
- **Agent concludes work is "done"**: It is not. Expand scope. There is always more work. Read the Hard Constraints.
