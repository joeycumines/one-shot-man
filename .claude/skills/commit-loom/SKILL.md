---
name: commit-loom
description: >
  Transform uncommitted working-tree changes into a sequence of maintainer-grade commits.
  Operates like a solo staff engineer working Kanban-style: establishes a rigorous check
  suite and baseline, sequences changes into self-contained commits (preferring fewer, larger
  commits over atomic splits), validates each independently, and tracks every hunk's disposition.
  Use when asked to commit large/complex changes, split messy diffs, land changes "the right way",
  or produce a clean commit history. NOT for trivial single-file changes.
---

# Commit Loom

You are a solo staff engineer working through a complex incoming change set, Kanban-style. The hunks in the diff are raw material — threads to be woven into a coherent commit history. The original ordering and grouping in the diff carries no authority. Your job is to produce a sequence of commits that a strict reviewer would happily merge, one at a time, with no surprises.

The metaphor: you are a weaver at a loom. The diff is your warp and weft. You select which threads to pull next so the resulting fabric is both beautiful and structurally sound. Each commit is a single, coherent row of the weave.

## Starting Conditions

The user has a checked-out git branch with tracked, uncommitted changes vs `HEAD`. There may be many files, many hunks, and the changes may be tangled (a refactor mixed with a bugfix, a feature with drive-by cleanups, a mass rename hiding a behavioral change). Your job is to untangle and commit.

Untracked files are not part of the default scope. If untracked files exist and appear relevant to the change set, note them and include them if the user confirms — but do not silently commit untracked files.

## The Full Check Suite

Before any commit can be made, a **complete, non-negotiable check suite** must be defined, discovered, executed, and recorded in the ledger. This is not "whatever checks happen to be lying around" — it is an explicit enumeration discovered during CAPTURE, recorded in the ledger's `discovery.check_suite` field, and treated as gospel for the rest of the run. Context decay over long runs means this suite definition is the foundation of correctness — it MUST be re-read from the ledger before every VERIFY step.

### Tier 1: Automated Checks (must ALL pass, mechanically verified)

These are commands that return success/failure without human judgment. Every applicable check must pass. If one is `null` (not applicable to this repo), record it as `null` with a note — do not silently skip it.

| Check | Purpose | Discovery Signal |
|-------|---------|-----------------|
| **Build** | Code compiles/builds with zero errors | `Makefile`, `package.json`, `go.mod`, `Cargo.toml`, `BUILD.bazel`, `build.gradle` |
| **Type Check** | Static type analysis passes | `tsconfig.json`, `go vet`, `mypy.ini`, `pyproject.toml` (mypy) |
| **Lint** | Code quality rules pass | `.eslintrc*`, `.golangci.yml`, `pyproject.toml` (ruff), `.pre-commit-config.yaml` |
| **Format Verification** | Code style is consistent | `gofmt -d`, `prettier --check`, `make fmt-check` |
| **Unit Tests** | All unit tests pass | `make test`, `npm test`, `go test ./...`, `pytest` |
| **Integration/E2E Tests** | End-to-end verification (if they exist) | Playwright, Cypress, integration test directories, `make test-integration` |

### Tier 2: Review Checks (judgment-based, must be performed and recorded)

These require reasoning, not just command execution. They are NOT optional.

1. **Subagent Review Gate (Rule of Two)**: Before each COMMIT, spawn at least two subagents with **identical prompts** to review the staged diff. This mitigates LLM nondeterminism through probability stacking — two independent reviews of identical material reduce miss probability quadratically. Both must produce PASS verdicts on the same diff with no code changes between them. This is the strict-review-gate pattern.

2. **Ways of Working Compliance**: Verify the change conforms to ALL standards discovered from:
   - `CLAUDE.md`, `CLAUDE.local.md` (project-level Claude instructions)
   - `AGENTS.md` (agent behavior standards)
   - `.editorconfig` (editor conventions)
   - `CONTRIBUTING.md`, `DEVELOPMENT.md` (project contribution guidelines)
   - Code review templates (`.github/PULL_REQUEST_TEMPLATE.md`, etc.)
   - Pre-commit hook configurations (what the project mandates)
   - Any project-specific standards files

3. **Self-Sufficiency Check**: At the commit point, the tree must build, pass all Tier 1 checks, and behave sensibly on its own. No broken intermediate states are acceptable.

### Anti-Patterns

These are patterns that agents are KNOWN to fall into. Guard against them deliberately:

- **Green reviews that mean nothing**: If reviews only check the diff text without verifying the code actually works (builds, tests pass), they provide false confidence. A reviewer saying "LGTM" means nothing if the build is broken. Tier 1 checks are the evidence that Tier 2 reviews are grounded in reality.
- **Conflating review with testing**: A subagent reviewing code correctness is NOT a substitute for running the actual test suite. Both are required. Do not record "review passed" as "tests passed."
- **Conflating manual and automated**: Do not treat "I eyeballed the code and it looks fine" as equivalent to `make lint` passing. Record each check separately under the correct tier.
- **Skipping the baseline**: Without a baseline, you cannot distinguish pre-existing failures from regressions you introduced. The baseline is not optional.
- **Claimed-but-not-run checks**: Do not record a check as "pass" unless you actually ran the command and observed the output. If you did not run `make test`, the ledger must say "not_run", not "pass".
- **Superficial reviews**: A review that says "looks good" without forming and testing specific hypotheses of incorrectness is a failure, not a pass. Reject it and re-run.
- **Shrinking the check suite**: Do not remove checks from the suite because they are "slow" or "probably fine." If a check was discovered in CAPTURE, it runs in VERIFY. Period.

## The Ledger: Your Structural Tether

Everything you do is tracked in a **ledger file** at `./scratch/commit-loom-<timestamp>.json`. This file is your structural tether to reality, just as `blueprint.json` is for task tracking. It must be written so that an agent encountering it from a fresh context can resume work immediately.

Read `references/ledger-schema.md` for the full schema. The critical design principle: all information needed to resume from any state is embedded in the ledger itself, with the most operationally critical fields at the top.

### Ledger Resumption

If you find existing `commit-loom-*.json` files in `./scratch/`, choose the most recent one (by timestamp in the filename). If the most recent ledger's `current_state` is not `FINALIZE`, it represents an interrupted run — resume from it. If it is `FINALIZE`, it's a completed run — start a new one.

Read the `_resume_guide` section at the top: it tells you the current state, current cycle, which branch, and where the snapshot is. The `current_cycle` field indicates the *next* cycle to execute (not the one currently in progress). Pick up exactly where the last agent left off. Do not re-plan from scratch unless the ledger explicitly says to.

## The State Machine

Commit Loom runs as a cyclic state machine. Each cycle produces zero or one commit. The states are:

```
CAPTURE ──→ ASSESS ──→ PLAN ──→ PREPARE ──→ STAGE ──→ VERIFY ──→ COMMIT ──→ UPDATE
               ↑            ↑                     │         │                  │
               │            └── (replan)──────────┘         │                  │
               └────────────────── (next cycle) ────────────┘                  │
                                                                              │
               ASSESS ──→ FINALIZE ──→ DONE                                   │
                     (when nothing remains)                                    │

VERIFY can loop back to PLAN when validation reveals a scope error.
VERIFY can loop on itself when a fix-and-reverify cycle is needed.
```

After each COMMIT, the cycle returns to ASSESS (not PLAN — the remaining backlog may have changed, and you must re-evaluate with fresh eyes). This is deliberate: committing changes the tree, which may reveal that planned hunks are now redundant, conflicting, or newly dependent. Re-assessing after every commit prevents you from executing a stale plan.

### State Transitions Must Update the Ledger

Every time you transition between states, update the ledger's `_resume_guide.current_state`. Use `python scripts/ledger.py advance <ledger-path> --state <STATE> --cycle <N>` for this. This is not optional — it is how the resumption protocol works. If the agent crashes mid-VERIFY, the next agent needs to know exactly where it was. A stale `current_state` breaks the entire resumption guarantee.

### State Descriptions

**CAPTURE** (once, at start): Snapshot the entire working-tree diff. Save it as a patch file via `scripts/snapshot.sh`. Initialize the ledger. Create a backup branch (e.g., `commit-loom-backup-<timestamp>`) pointing at the current `HEAD`. This backup is your safety net — the user's work is never at risk.

**Discovery and Baseline** (within CAPTURE, mandatory):

1. **Discover the full check suite**: Using `references/validation-discovery.md`, discover all available build, lint, test, typecheck, and format tools. Record each in `discovery.check_suite`. Also discover ways of working: read `CLAUDE.md`, `AGENTS.md`, `CONTRIBUTING.md`, `.editorconfig`, pre-commit hooks, CI config, and any project-specific standards files. Record these in `discovery.ways_of_working`. Also discover available skills and agents for review (e.g., `strict-review-gate`). Record these in `discovery.available_skills` and `discovery.available_agents`.

2. **Establish the baseline**: Stash the entire changeset (`git stash push --include-untracked`), then run **every** Tier 1 check from the check suite against the clean tree. Record results in the ledger's `baseline` section:
   ```
   baseline:
     established_at: <timestamp>
     check_results:
       build: { command: "make build", result: "pass" }
       typecheck: { command: "go vet ./...", result: "pass" }
       lint: { command: "make lint", result: "fail", detail: "3 existing warnings" }
       format: { command: "make fmt-check", result: "pass" }
       unit_tests: { command: "make test", result: "pass" }
       integration_tests: { command: null, result: "not_applicable", detail: "No integration test suite found" }
     pre_existing_failures:
       - check: "lint"
         detail: "3 warnings in auth/legacy.go (unused imports, deprecated function call)"
         action_plan: "Fix as part of the commit that touches auth/ if applicable, otherwise note as known pre-existing"
   ```
   This is critical: without a baseline, you cannot distinguish pre-existing failures from regressions you introduce. Pre-existing failures should ideally be fixed as you go — they are your responsibility once you touch the affected code.

3. **Restore the changeset**: `git stash pop`. Verify the stash applies cleanly.

After initialization, register every hunk in the ledger. This is not optional — the hunk registry is the foundation for disposition tracking. Parse the snapshot patch file (or use `git diff HEAD` if the patch is unwieldy) and call `python scripts/ledger.py register-hunk` for each hunk. Each entry needs an ID, file path, start/end lines, and a short summary. For large diffs, batch-register by file group using subagents — but the registration must complete before the first cycle begins.

**ASSESS**: Determine what remains unconsumed. Compare the current working tree vs `HEAD` against the original snapshot. If nothing remains, transition to FINALIZE. Otherwise, transition to PLAN.

How you assess depends on the change set size. The patch file on disk is the authoritative source — it can always be re-read in chunks. For the working tree, use `git diff HEAD --stat` for a quick overview, then read specific files as needed. For very large change sets, spawn subagents to analyze different file groups in parallel and report summaries back. The goal is to build an accurate mental model of what's left, not to hold everything in context at once.

**PLAN**: Consider the remaining unconsumed hunks as a backlog. Choose the next coherent group to commit. The selection criteria are:
- Dependency: X must land before Y if Y depends on X's changes.
- Locality: small independent tweaks near a larger change are often best folded into that change's commit rather than isolated.
- Risk: foundational, low-risk plumbing first; behavioral changes isolated and clearly labeled.
- **Self-contained, not atomic**: Prioritize self-contained commits over "atomic" ones. Fewer commits means fewer verification rounds, fewer opportunities for intermediate breakage, and less risk of divergent temporary states needed to pass quality checks. A commit should be the largest coherent unit that (a) a strict reviewer can still review in one sitting and (b) fits fully into context including all information needed to reason about correctness. Rolling up multiple interdependent changes into a single commit is preferred over splitting them into commits that would leave the tree in intermediate states.
- **Reviewer intent**: When informing reviewers (including subagent reviewers), explicitly state: "This commit is intentionally self-contained rather than atomic. Multiple related changes have been rolled up because splitting them would leave the tree in an inconsistent state or require artificial intermediate commits. The changes are logically related and should be reviewed as a unit." This prevents reviewers from pushing for unnecessary splits.

For large change sets, the plan is built incrementally — plan the next commit, execute it, then re-plan. Do not attempt to plan all commits up front for a 50-file diff. The cyclic structure handles this naturally: each cycle's PLAN only needs to decide what's next, not what the entire remaining sequence looks like.

Record the planned commit in the ledger (theme, target files, target hunks). For large change sets, you may also spawn a subagent to analyze a file group and propose a commit structure — but the orchestrator always makes the final sequencing decision.

**PREPARE**: Stage the selected hunks for commit. This is where you exercise judgment:
- If a hunk needs modifications to be self-sufficient in isolation (e.g., it references code that hasn't been committed yet), modify it.
- If a hunk is redundant given what has already been committed, mark it as dropped with rationale.
- If two hunks interact in a way that suggests a different implementation would be cleaner, refactor them.
- The goal is a self-sufficient, stand-alone, defensible commit.

This is the creative heart of the skill. You are not a replay engine — you are an engineer making judgment calls about what the code should look like at this commit.

**STAGE**: Apply the prepared changes to the staging area. This may involve `git add -p`, editing files directly, or writing new code that replaces original hunks.

**VERIFY**: Run the **full check suite** (both Tier 1 and Tier 2) against the staged tree. This is the most critical step — it is the proof that your commit is correct.

1. **Re-read the ledger's `discovery.check_suite`, `discovery.tooling_preferences`, and `discovery.ways_of_working`**. Context decay is real. Do not trust your memory of what checks exist, what tooling to use, or what standards apply. **This is a mandatory reread — the tooling_preferences section exists specifically to prevent context decay from causing wrong tooling choices.**

2. **Align the working tree**: Before staging, ensure the working tree state is what you intend to commit. This may require:
   - **Build**: If the project produces compiled artifacts that tests depend on (e.g., `./dist/`, `build/`, `target/`), run the build step first. A test that runs against stale compiled code is not testing the commit. Check `discovery.tooling_preferences.build_before_test` for guidance.
   - **Code generation**: If the commit includes generated code (protobuf, ANTLR, OpenAPI stubs), run the generator.
   - **Formatting**: Run format/encode steps to ensure files are in their final state before staging.

   Use `discovery.tooling_preferences` for the correct invocation (e.g., `bunx` not `npx`, `bun run` not `npm run`).

3. **Stage the aligned changes**: Apply the prepared changes to the staging area. ONLY the commit-ready state is allowed in the staging area. If the staging area includes changes that are not part of the planned commit, fix this before proceeding (e.g., by unstaging, modifying, or dropping hunks).

4. **Run Tier 1 checks on the staged state**:
   - Type check: must succeed with zero errors.
   - Lint: must succeed with zero errors.
   - Format verification: must be clean (re-run if formatting modified files).
   - Unit tests: must pass 100%.
   - Integration/E2E tests: if they exist, must pass.
   - Compare results against the `baseline`. Any regression from the baseline (a check that was passing now failing, or a check that was already failing now failing in a new way) must be fixed before proceeding.
   - Pre-existing failures from the baseline that this commit does not touch may be noted but should not block the commit. However, if this commit modifies code in the area of a pre-existing failure, fix the failure as part of this commit.
   - Manual checks: This might include browser automation via Playwright MCP + image analysis for visual verification, manual smoke testing of critical flows, or any other manual verification steps discovered during CAPTURE. These must be performed and recorded with the same rigor as automated checks.

3. **Tier 2: Run review checks**:
   - **Subagent Review Gate**: Spawn at least two subagents with **identical prompts** (following the strict-review-gate / Rule of Two pattern) to review the staged diff. Both must produce PASS verdicts on the **same diff** with no code changes between them. If Run 1 finds issues → fix → the diff changes → reset counter → restart at Run 1 against the new diff. This probability-stacking approach is the best defense against LLM nondeterminism and hallucination.
   - **Ways of Working Compliance**: Verify the change conforms to standards from CLAUDE.md, AGENTS.md, and other discovered standards.
   - **Self-Sufficiency**: Confirm the tree at this commit is independently valid.

4. **Record all results** in the cycle log's `validation_results` field, broken down by check. Include:
   - Each Tier 1 check: command, result (pass/fail), and any output details
   - Tier 2 review gate: number of runs, verdicts, any issues found
   - Comparison against baseline: any regressions noted
   - Any fixes applied during verification

**Record results BEFORE the commit, not after.** The timing matters: write the validation_results to `cycle_log[N]` and `planned_commits[N]` as part of the VERIFY→COMMIT transition, before creating the git commit. If the commit is created first and the agent crashes before writing results, the cycle_log will be empty — making it impossible to verify self-sufficiency from the ledger alone.

The goal is to produce **concrete, verifiable evidence** that the commit is as correct as the available tooling and review process can confirm. This is not "proof" — it is evidence. No combination of automated checks and subagent reviews can guarantee correctness. What it CAN do is raise the probability that defects are caught to the highest level achievable with the available tools. Anything less is accepting avoidable risk.

**COMMIT**: Create the commit with a message that justifies the change clearly. Follow the commit message guidance in `references/commit-standards.md`. Record the SHA in the ledger.

**UPDATE**: Update the ledger with the disposition of every hunk consumed in this cycle. Every hunk that was part of this commit must have its `disposition` field set (action, commit_id, note). Use `python scripts/ledger.py disposition <ledger> --hunk-id <ID> --action <action> --commit-id <N> --note <rationale>` for each. No hunk may remain in `unconsumed` status after UPDATE unless it is explicitly deferred.

**CRITICAL: Record validation_results in BOTH planned_commits and cycle_log.** After Tier 1 and Tier 2 checks complete in VERIFY, before transitioning to COMMIT, you MUST write the validation results to the ledger. This means updating BOTH:

1. `planned_commits[N].validation_results` (the planned commit entry's results field)
2. `cycle_log[N].validation_results` (the cycle log entry for this cycle)

Both must contain the actual check results. Do not write to one and leave the other empty. The cycle_log is the self-sufficiency record that allows the ledger to prove each commit is independently valid — an empty `validation_results` means no evidence the checks ran, regardless of what was recorded elsewhere.

Example cycle_log entry with results:
```json
{
  "cycle": 1,
  "commit_sha": "abc123...",
  "validation_results": {
    "build": "pass",
    "typecheck": "pass",
    "lint": "pass",
    "format": "pass",
    "unit_tests": "pass",
    "integration_tests": "not_applicable",
    "manual_checks": "pass",
    "review_gate": "pass (2/2)"
  },
  "issues_found": [],
  "fixes_applied": []
}
```

**IMPORTANT**: The skill has been observed to correctly detect trap conditions and record them in `baseline.pre_existing_failures`, but then fail to persist per-commit validation results to `cycle_log[N].validation_results`. This creates a gap between "checks ran" and "checks verifiable as having run". The cycle_log is the proof of enforcement — if it is empty, the self-sufficiency of each commit cannot be verified by any agent that later encounters this ledger.

Transition back to ASSESS.

**FINALIZE**: Produce the final summary. Call `python scripts/ledger.py summary <ledger>` to generate it, then write the result into the ledger's `final_summary` field. The summary must include commit SHAs, hunk accounting with counts per disposition action, validation status, and follow-up notes. After the ledger is updated, deliver the summary to the user as a readable report. See "Final Summary" below for the expected structure.

### Why a Cycle, Not a Pipeline

A linear pipeline (plan everything, then execute) fails because the tree changes after each commit. What looked like a sensible grouping at cycle 1 may be wrong by cycle 3. The cyclic structure ensures every planning decision is made against the current state of the tree, not a stale snapshot. This costs a bit more upfront thinking per cycle but produces dramatically better results on tangled change sets.

## Validation

Each commit must be independently valid: the tree should build, pass type checks, pass linters, pass all tests, and conform to the project's ways of working at that commit. The specific tools and standards are discovered during CAPTURE and recorded in the ledger.

### Discovery (during CAPTURE)

At the start of the run, discover what validation tooling and standards are available. Read `references/validation-discovery.md` for the detailed checklist. The expanded version:

1. **Build/lint/test tools**: Look for `Makefile`, `package.json` scripts, `pyproject.toml`, `Cargo.toml`, `go.mod`, `BUILD.bazel`, or similar. Check for pre-commit hooks (`.pre-commit-config.yaml`, `.husky/`, `.git/hooks/pre-commit`). Check CI config (`.github/workflows/`, `.gitlab-ci.yml`) to understand what the project considers mandatory. Look for linting config (`.eslintrc`, `golangci.yml`, etc.).

2. **Tooling preferences**: This section is CRITICAL for preventing context decay. Record explicit tooling guidance that would otherwise be forgotten mid-run. Context decay causes agents to forget specifics like "use `bunx` not `npx`" or "build must run before tests". Record:
   - **Package manager**: Is the project using `bun`, `pnpm`, `npm`, or `yarn`? For `bun` projects, always use `bunx` (not `npx`) and `bun run` (not `npm run`).
   - **Build command**: What command builds the project? Any specific order needed?
   - **Build-before-test**: Does the test suite depend on compiled artifacts? If so, what is the correct order?
   - **Test command**: What runs the tests? Any environment variables needed?
   - **CI signals**: What does the CI config use? This is authoritative evidence of the correct tooling.
   - **User-provided tools (MCPs)**: Note any MCPs the user has configured (e.g., Playwright for browser automation, image analysis tools). These may be required for verification. If Playwright is available, it should be used for E2E verification.
   - **Codebase-specific tools**: Note special tooling embedded in the codebase (e.g., Go fuzz tests, benchmark harnesses, property-based testing frameworks). These may require specific invocation and deserve explicit reminder.
   - **Custom scripts**: Any project-specific scripts that are part of the quality bar (e.g., `./scripts/validate-schema.sh`, custom linters).

   Record these in `discovery.tooling_preferences` — this section is prominently visible and must be re-read before every VERIFY to prevent wrong tooling choices.

3. **Ways of working**: Read `CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md`, `.editorconfig`, `CONTRIBUTING.md`, `DEVELOPMENT.md`, and any project-specific standards files. These define the behavioral expectations for code in this repo. Record key rules and conventions in `discovery.ways_of_working`.

4. **Available skills and agents**: Scan `.claude/skills/` and `.claude/agents/` for tools related to review, quality, or validation. For each, record its name, path, and applicability. Particularly valuable:
   - `strict-review-gate` for Rule of Two review enforcement
   - Domain-specific agents that provide quality checks

5. **Record everything** in the ledger's `discovery` section. This is not optional — it is the reference that VERIFY re-reads before every cycle.

### Review Patterns

The gold standard for commit verification is **subagent-based review with multiple identical-prompt runs** (the strict-review-gate pattern). This is not just "have a subagent look at the code" — it is a specific protocol:

1. **Spawn at least two subagents** with **effectively identical prompts**. The only acceptable variation is trivial logistics (output file path).
2. **Both review the exact same diff**. No code changes between Run 1 and Run 2.
3. **Both must produce PASS verdicts**. If either fails → fix → reset counter → restart at Run 1 against the new diff.
4. **Why this works**: Two independent reviews of identical material compound the probability of catching defects. P(miss) becomes P(miss)². This is the best available defense against LLM nondeterminism and hallucination.

**Important distinctions**:
- **Review ≠ testing**: A subagent reviewing code is NOT a substitute for running `make test`. Both are required. Review catches logical errors and design issues; tests catch regressions and edge cases.
- **Manual ≠ automated**: "I read the code" is manual review. `make lint` is automated. Do not conflate them. Record each separately.
- **Review is a gate, not a suggestion**: If the review gate fails, the commit does not proceed until the issue is resolved.

### Leveraging Available Skills and Agents

This skill operates in environments that may have other skills, agents, and tools available. These are force multipliers for validation quality. For example:

- A `strict-review-gate` skill can be used to verify commit quality before each commit is finalized.
- A `Takumi`-style agent may provide domain-specific quality checks.
- Project-specific review or lint agents may exist.

At each VERIFY step, re-read the relevant skill/agent instructions immediately before use — do not rely on memory from earlier in the run. Context decay over long runs means the instructions must be fresh.

### When Validation Fails

Do not pause or ask the user. Fix the issue. You have the full diff, the ledger, and all available tools. If a lint error appears, fix it. If a test fails, investigate and fix. If the fix requires modifying the staged changes, do so and re-verify. If the fix reveals that the current commit's scope is wrong (e.g., a dependency was missed), re-plan this cycle from PLAN.

Only if you are genuinely stuck after multiple attempts should you note the failure in the ledger, commit anyway with a clear note about the known issue, and proceed. The ledger must record what happened.

## Hunk Disposition Tracking

Every hunk from the original snapshot must have an explicit disposition by the end of the run. The allowed dispositions are:

| Action | Meaning |
|--------|---------|
| `included_as_is` | Hunk committed without modification |
| `included_with_modifications` | Hunk committed, but with changes (note why) |
| `refactored` | Original hunk replaced by different code (note why) |
| `folded_into_commit_N` | Hunk merged into another commit's changes (note which) |
| `dropped_redundant` | Hunk no longer needed after earlier commits (note why) |
| `dropped_incorrect` | Hunk was wrong and removed (note why) |
| `deferred` | Hunk intentionally left for future work (note why) |

No hunk is silently dropped, silently merged, or silently mutated. The ledger must contain a record for every hunk with one of these dispositions and a brief rationale.

## Commit Quality Bar

Each commit should read as if a disciplined upstream maintainer wrote it deliberately. Specifically:

- **Self-sufficient**: The tree builds, passes all checks, and behaves sensibly at this commit alone. A reviewer checking out this commit can work with it.
- **Justified**: The commit message explains *why* the change was made, not just *what* was changed.
- **Self-contained (not atomic)**: The commit is a coherent, complete unit of work. It may contain multiple logical changes if they are interdependent and splitting them would leave the tree in an inconsistent state. "Atomic" (one concern per commit) is the enemy of "self-contained" when changes are tangled. Prioritize fewer, larger, self-contained commits over more, smaller, atomic ones. Fewer commits = fewer verification rounds = less risk of intermediate breakage.
- **Correct**: Validation passes. Tests pass. Lint passes. If something is known-broken, it's called out explicitly.
- **Not a dump**: If you catch yourself staging everything in one go with no coherent theme, stop and re-plan. But a commit that covers a coherent area of work (e.g., "extract auth middleware" including the extraction, test updates, and import cleanup) is perfectly acceptable even if it spans multiple concerns.
- **Reviewer communication**: When subagent reviewers are used, inform them: "This commit is intentionally self-contained. Multiple related changes have been rolled up because splitting would leave the tree inconsistent. Review the changes as a coherent unit rather than flagging scope as a concern." This prevents well-meaning reviewers from pushing for unnecessary splits.

For commit message format, read `references/commit-standards.md`. The short version: match the repo's existing style, and when in doubt, use a conventional-commit-ish format with a subject line that states the intent and a body that explains the reasoning.

## Reread Points

This skill operates on very long horizons. Context decay is real. The following reread points are mandatory:

1. **At the start of every cycle** (entering ASSESS): Re-read the ledger's `_resume_guide` and the `plan` section. You need to know where you are before deciding what's next.
2. **Before VERIFY**: Re-read the ledger's `discovery.check_suite`, `discovery.ways_of_working`, and `baseline` sections. You need to know what checks to run, what standards apply, and what the baseline state is before you can verify correctness.
3. **Before using any external skill or agent**: Re-read that skill/agent's instructions in full. Do not rely on cached knowledge from earlier in the run.
4. **After context compaction**: If you suspect your context has been compacted (you feel disoriented, you're not sure what cycle you're on), immediately read the ledger file. It is your sole source of truth.
5. **Before PLAN**: Re-read the `baseline.pre_existing_failures` to ensure your planned commit either addresses them or avoids touching the affected areas.

## Final Summary

When all hunks are consumed (or explicitly deferred/dropped), produce a final summary delivered to the user. Include:

1. What landed: commit count, SHAs, and one-line summary of each.
2. What changed from the original diff: any hunks that were modified, refactored, or dropped, with rationale.
3. Validation status: did all commits pass full validation? Any known issues?
4. Follow-ups: anything the engineer-persona would flag (deferred hunks, known technical debt, suggestions for further work).
5. The ledger file location for audit.

## Scripts

The `scripts/` directory contains tools for the most common ledger operations:

- **`scripts/snapshot.sh`** — Captures `git diff HEAD` to a timestamped patch file in `./scratch/`, creates a backup branch, and returns the paths needed for ledger initialization.
- **`scripts/ledger.py`** — Manages the ledger JSON: initialize, update hunk dispositions, query remaining hunks, advance state, register hunks, and render the final summary.

These cover the high-frequency operations (state transitions, commit recording, disposition tracking, hunk queries). For operations not covered by the scripts — populating `discovery`, building the `planned_commits` list, updating the `_resume_guide` fields beyond what `advance` handles — edit the ledger JSON directly. The scripts handle the structurally tricky parts; the agent handles the semantically rich parts.

## Reference Documents

- **`references/ledger-schema.md`** — Full JSON schema for the ledger file, with examples.
- **`references/validation-discovery.md`** — Detailed checklist for discovering repo-specific validation tooling.
- **`references/commit-standards.md`** — Commit message format, quality criteria, and examples.
