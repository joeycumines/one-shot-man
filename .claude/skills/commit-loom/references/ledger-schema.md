# Ledger Schema

The ledger is a JSON file at `./scratch/commit-loom-<timestamp>.json`. It serves three purposes: operational state (where are we in the cycle), planning artifact (what's left, what's planned), and audit trail (what happened to every hunk).

Design principle: an agent encountering this file from a completely fresh context must be able to resume work by reading it alone. The most critical fields are at the top.

### `_resume_guide.current_cycle` Semantics

The `current_cycle` field in `_resume_guide` indicates the **next** cycle to execute, not the one currently in progress. After recording a commit for cycle 2, the field becomes 3 — meaning "cycle 2 is done, start cycle 3 next." Combined with `current_state`, a resuming agent knows exactly what to do: if `current_state` is `ASSESS` and `current_cycle` is 3, it means "start the assessment phase of cycle 3."

## Top-Level Structure

```json
{
  "_resume_guide": { "...see below..." },
  "meta": { "...see below..." },
  "discovery": { "...see below..." },
  "baseline": { "...see below..." },
  "plan": { "...see below..." },
  "hunk_registry": [ "...see below..." ],
  "cycle_log": [ "...see below..." ],
  "final_summary": null
}
```

## _resume_guide

The very first thing in the file. Designed to be read first and provide everything needed to resume.

```json
{
  "_resume_guide": {
    "skill": "commit-loom",
    "version": 1,
    "what_is_this": "Operational state for a commit-loom run. Read this file to resume.",
    "current_state": "ASSESS",
    "current_cycle": 3,
    "total_cycles_completed": 2,
    "total_hunks": 47,
    "consumed_hunks": 22,
    "remaining_hunks": 25,
    "branches": {
      "backup": "commit-loom-backup-20260425-143000",
      "working": "main"
    },
    "snapshot_file": "scratch/commit-loom-snapshot-20260425.patch",
    "instructions": [
      "Read this file fully.",
      "Check current_state for the state machine position.",
      "Read the plan section for remaining work.",
      "Resume from the cycle indicated by current_cycle."
    ],
    "reread_reminders": {
      "every_cycle_start": "Re-read _resume_guide and plan sections",
      "before_verify": "Re-read discovery.tooling_preferences and discovery.check_suite",
      "before_external_skill": "Re-read that skill's full instructions"
    },
    "key_reminders": [
      "Tooling preferences (discovery.tooling_preferences): READ THIS BEFORE RUNNING CHECKS. Context decay causes wrong tooling choices."
    ]
  }
}
```

## meta

```json
{
  "meta": {
    "started_at": "2026-04-25T14:30:00Z",
    "updated_at": "2026-04-25T15:45:00Z",
    "repository_root": "/path/to/repo",
    "original_head": "abc123def456...",
    "original_head_message": "feat: add user profile page",
    "snapshot_patch": "scratch/commit-loom-snapshot-20260425.patch",
    "snapshot_stat": {
      "files_changed": 12,
      "insertions": 450,
      "deletions": 180
    }
  }
}
```

## discovery

Record of what validation tooling, working standards, and supporting skills/agents were found in the environment.

### check_suite

Per-check commands that the project supports. Each entry has a `command` (the shell invocation) and an `applicable` flag. When a check does not apply to the project, `command` is `null` and a `note` explains why.

```json
{
  "check_suite": {
    "build": { "command": "make build", "applicable": true },
    "typecheck": { "command": "go vet ./...", "applicable": true },
    "lint": { "command": "make lint", "applicable": true },
    "format": { "command": "make fmt-check", "applicable": true },
    "unit_tests": { "command": "make test", "applicable": true },
    "integration_tests": { "command": null, "applicable": false, "note": "No integration test suite found" }
  }
}
```

### tooling_preferences

**CRITICAL: This section prevents context decay from causing wrong tooling choices.** Record explicit tooling guidance that would otherwise be forgotten during long runs. Context decay is real — an agent mid-run may forget "use bunx not npx" or "build must run before tests."

```json
{
  "tooling_preferences": {
    "package_manager": {
      "used": "bun",
      "never_use": ["npx", "npm", "yarn"],
      "always_use": ["bunx", "bun run"],
      "note": "bunx for one-off commands, bun run for scripts"
    },
    "build_before_test": {
      "required": true,
      "build_command": "bun run build",
      "test_command": "bun run test",
      "note": "Tests run against ./dist/ which build produces"
    },
    "mcps": [
      { "name": "playwright", "note": "Use for E2E verification of UI changes" }
    ],
    "codebase_tools": [
      { "name": "go-fuzz", "note": "Go fuzz tests run via 'go test -fuzz=FuzzX'. Run standard unit tests first, then fuzz: 'go test -fuzz=FuzzX -fuzztime=30s'" }
    ],
    "custom_scripts": [
      { "path": "./scripts/validate-schema.sh", "note": "Run after any schema changes" }
    ],
    "ci_authoritative": ".github/workflows/ci.yml",
    "ci_uses": ["bun install", "bun run build", "bun run test"]
  }
}
```

### ways_of_working

Project-level conventions discovered from CLAUDE.md, AGENTS.md, .editorconfig, contributing guides, pre-commit hooks, and CI configuration. Captures the rules that commit-loom must respect when generating commits.

```json
{
  "ways_of_working": {
    "claude_md": { "path": "CLAUDE.md", "key_rules": ["Use GNU Make for build tasks", "No direct npm/go commands"] },
    "agents_md": { "path": "AGENTS.md", "key_rules": [] },
    "editorconfig": { "path": ".editorconfig", "key_rules": ["indent_style = space", "indent_size = 2"] },
    "contributing_md": null,
    "pre_commit_hooks": { "path": ".pre-commit-config.yaml", "enforced_checks": ["gofmt", "golangci-lint"] },
    "ci_indicators": [".github/workflows/ci.yml"],
    "additional_standards": []
  }
}
```

### review_pattern

Documents the quality gate strategy. Typically references the strict-review-gate skill for subagent-based Rule of Two enforcement.

```json
{
  "review_pattern": {
    "type": "subagent_rule_of_two",
    "description": "Two subagents with identical prompts review the same diff. Both must PASS.",
    "skill_reference": ".claude/skills/strict-review-gate/SKILL.md"
  }
}
```

### Full discovery example

```json
{
  "discovery": {
    "check_suite": {
      "build": { "command": "make build", "applicable": true },
      "typecheck": { "command": "go vet ./...", "applicable": true },
      "lint": { "command": "make lint", "applicable": true },
      "format": { "command": "make fmt-check", "applicable": true },
      "unit_tests": { "command": "make test", "applicable": true },
      "integration_tests": { "command": null, "applicable": false, "note": "No integration test suite found" }
    },
    "tooling_preferences": {
      "package_manager": {
        "used": "bun",
        "never_use": ["npx", "npm"],
        "always_use": ["bunx", "bun run"]
      },
      "build_before_test": { "required": true },
      "mcps": [],
      "codebase_tools": [],
      "custom_scripts": [],
      "ci_uses": ["bun install", "bun run build", "bun run test"]
    },
    "ways_of_working": {
      "claude_md": { "path": "CLAUDE.md", "key_rules": ["Use GNU Make for build tasks", "No direct npm/go commands"] },
      "agents_md": { "path": "AGENTS.md", "key_rules": [] },
      "editorconfig": { "path": ".editorconfig", "key_rules": ["indent_style = space", "indent_size = 2"] },
      "contributing_md": null,
      "pre_commit_hooks": { "path": ".pre-commit-config.yaml", "enforced_checks": ["gofmt", "golangci-lint"] },
      "ci_indicators": [".github/workflows/ci.yml"],
      "additional_standards": []
    },
    "review_pattern": {
      "type": "subagent_rule_of_two",
      "description": "Two subagents with identical prompts review the same diff. Both must PASS.",
      "skill_reference": ".claude/skills/strict-review-gate/SKILL.md"
    },
    "available_skills": [
      {
        "name": "strict-review-gate",
        "path": ".claude/skills/strict-review-gate/SKILL.md",
        "applicability": "Use before each COMMIT for quality gate"
      }
    ],
    "available_agents": [
      {
        "name": "Takumi",
        "path": ".claude/agents/Takumi.md",
        "applicability": "Domain-specific quality enforcement"
      }
    ],
    "validation_notes": "Repo uses GNU Make. Pre-commit hooks enforce gofmt and golangci-lint."
  }
}
```

## baseline

Snapshot of check results against the clean working tree, captured **before** any hunks are applied. This section is `null` until the CAPTURE phase completes the baseline establishment step. Its purpose is to distinguish pre-existing failures from failures introduced by the commit-loom changes, so that VERIFY can compare cycle results against a known-good (or known-imperfect) starting point.

- **established_at**: ISO 8601 timestamp when the baseline was captured.
- **stash_ref**: The git stash reference created to preserve the working tree before baseline checks run.
- **check_results**: Per-check outcome from running each applicable check in `discovery.check_suite` against the clean tree. Each entry has a `command`, a `result` (one of `pass`, `fail`, `not_applicable`), and an optional `detail` string for context.
- **pre_existing_failures**: List of checks that did not pass at baseline. Each entry identifies the check, provides detail on the failure, and states an `action_plan` describing how commit-loom should treat it during VERIFY (e.g., fix if a commit touches that area, otherwise note as known).

```json
{
  "baseline": {
    "established_at": "2026-04-25T14:31:00Z",
    "stash_ref": "stash@{0}",
    "check_results": {
      "build": { "command": "make build", "result": "pass" },
      "typecheck": { "command": "go vet ./...", "result": "pass" },
      "lint": { "command": "make lint", "result": "fail", "detail": "3 existing warnings in auth/legacy.go" },
      "format": { "command": "make fmt-check", "result": "pass" },
      "unit_tests": { "command": "make test", "result": "pass" },
      "integration_tests": { "command": null, "result": "not_applicable", "detail": "No integration test suite found" }
    },
    "pre_existing_failures": [
      {
        "check": "lint",
        "detail": "3 warnings in auth/legacy.go (unused imports, deprecated function call)",
        "action_plan": "Fix as part of the commit that touches auth/ if applicable, otherwise note as known pre-existing"
      }
    ]
  }
}
```

## plan

```json
{
  "plan": {
    "goal_state": "All hunks consumed via a sequence of self-sufficient, validated commits.",
    "initial_hunk_count": 47,
    "initial_file_count": 12,
    "_next_cycle": 3,
    "planned_commits": [
      {
        "id": 1,
        "theme": "refactor: extract auth middleware into dedicated module",
        "description": "Move JWT validation logic from server/handlers.go into auth/middleware.go with proper error propagation.",
        "target_files": ["auth/middleware.go", "auth/middleware_test.go", "server/handlers.go"],
        "target_hunk_ids": [1, 2, 5, 6, 14],
        "status": "DONE",
        "commit_sha": "def456...",
        "commit_message": "refactor: extract auth middleware into dedicated module\n\n...",
        "dispositions": [
          {
            "hunk_id": 1,
            "action": "included_as_is",
            "note": null
          },
          {
            "hunk_id": 2,
            "action": "included_with_modifications",
            "note": "Added missing error return for expired token case"
          },
          {
            "hunk_id": 5,
            "action": "included_as_is",
            "note": null
          },
          {
            "hunk_id": 6,
            "action": "refactored",
            "note": "Original hunk was a band-aid fix; replaced with proper error propagation through middleware chain"
          },
          {
            "hunk_id": 14,
            "action": "folded_into_commit_N",
            "note": "Small import cleanup in same file; folded in rather than isolated"
          }
        ],
        "validation_results": {
          "build": "pass",
          "lint": "pass",
          "test": "pass",
          "review_gate": "pass (2/2 contiguous)"
        },
        "issues_found_during_verify": [],
        "fixes_applied_during_verify": []
      },
      {
        "id": 2,
        "theme": "fix: resolve token expiry edge case in concurrent sessions",
        "description": "Fix race condition where concurrent requests could use a stale token.",
        "target_files": ["auth/session.go", "auth/session_test.go"],
        "target_hunk_ids": [3, 7, 8],
        "status": "PLANNED",
        "commit_sha": null,
        "commit_message": null,
        "dispositions": [],
        "validation_results": null,
        "issues_found_during_verify": [],
        "fixes_applied_during_verify": []
      }
    ],
    "replan_count": 0,
    "replan_log": []
  }
}
```

### Replanning

When a cycle reveals that the plan is stale (e.g., a refactor made a planned hunk redundant, or a dependency was missed), increment `replan_count` and append to `replan_log`:

```json
{
  "replan_count": 1,
  "replan_log": [
    {
      "cycle": 2,
      "trigger": "Commit 1 refactor made hunks 9 and 10 redundant — the extracted middleware handles their cases",
      "affected_commit_ids": [3],
      "action_taken": "Removed hunks 9 and 10 from commit 3; added them as 'dropped_redundant'; updated commit 3 scope"
    }
  ]
}
```

## hunk_registry

Every hunk from the original snapshot, tracked from discovery through disposition.

```json
{
  "hunk_registry": [
    {
      "id": 1,
      "file": "auth/middleware.go",
      "start_line": 23,
      "end_line": 45,
      "summary": "Extract JWT validation into separate function",
      "original_content_hash": "sha256:abc123...",
      "disposition": {
        "action": "included_as_is",
        "commit_id": 1,
        "note": null
      },
      "status": "consumed"
    },
    {
      "id": 9,
      "file": "server/handlers.go",
      "start_line": 100,
      "end_line": 115,
      "summary": "Temporary workaround for stale token issue",
      "original_content_hash": "sha256:def456...",
      "disposition": {
        "action": "dropped_redundant",
        "commit_id": null,
        "note": "Made redundant by proper middleware extraction in commit 1"
      },
      "status": "consumed"
    },
    {
      "id": 30,
      "file": "api/routes.go",
      "start_line": 50,
      "end_line": 55,
      "summary": "Add /admin/audit-log endpoint",
      "original_content_hash": "sha256:789abc...",
      "disposition": null,
      "status": "unconsumed"
    }
  ]
}
```

### Status values
- `unconsumed` — Not yet addressed
- `in_progress` — Currently being prepared for a commit
- `consumed` — Addressed (check disposition for how)

### Disposition actions
| Action | Meaning |
|--------|---------|
| `included_as_is` | Committed without modification |
| `included_with_modifications` | Committed with changes (see note) |
| `refactored` | Replaced by different code (see note) |
| `folded_into_commit_N` | Merged into another commit (see note) |
| `dropped_redundant` | No longer needed (see note) |
| `dropped_incorrect` | Was wrong, removed (see note) |
| `deferred` | Intentionally left for later (see note) |

## cycle_log

Append-only log of every cycle executed.

```json
{
  "cycle_log": [
    {
      "cycle": 1,
      "started_at": "2026-04-25T14:35:00Z",
      "ended_at": "2026-04-25T14:37:00Z",
      "state_transitions": ["ASSESS", "PLAN", "PREPARE", "STAGE", "VERIFY", "COMMIT", "UPDATE"],
      "commit_id": 1,
      "commit_sha": "def456...",
      "validation_results": {
        "build": "pass",
        "lint": "pass",
        "test": "pass"
      },
      "issues_found": [],
      "fixes_applied": [],
      "duration_seconds": 120,
      "notes": "Clean cycle. No replanning needed."
    },
    {
      "cycle": 2,
      "started_at": "2026-04-25T14:38:00Z",
      "ended_at": "2026-04-25T14:42:00Z",
      "state_transitions": ["ASSESS", "PLAN", "PREPARE", "STAGE", "VERIFY", "PLAN", "PREPARE", "STAGE", "VERIFY", "COMMIT", "UPDATE"],
      "commit_id": 2,
      "commit_sha": "789abc...",
      "validation_results": {
        "build": "pass",
        "lint": "fail (then pass after fix)",
        "test": "pass"
      },
      "issues_found": ["Lint: unused import in auth/session.go after refactor"],
      "fixes_applied": ["Removed unused import, re-ran lint"],
      "duration_seconds": 240,
      "notes": "First verify caught a lint error. Fixed inline. Re-planned cycle to include the fix."
    }
  ]
}
```

## final_summary

Populated at the end of the run. `null` during execution.

```json
{
  "final_summary": {
    "completed_at": "2026-04-25T16:00:00Z",
    "total_cycles": 7,
    "total_commits": 7,
    "commit_shas": ["def456...", "789abc...", "..."],
    "commit_summaries": [
      "refactor: extract auth middleware into dedicated module",
      "fix: resolve token expiry edge case in concurrent sessions",
      "..."
    ],
    "hunk_accounting": {
      "total": 47,
      "included_as_is": 32,
      "included_with_modifications": 8,
      "refactored": 3,
      "folded": 2,
      "dropped_redundant": 1,
      "dropped_incorrect": 0,
      "deferred": 1
    },
    "validation_summary": {
      "all_commits_passed": true,
      "any_known_issues": false,
      "review_gate_used": true,
      "review_gate_passes": "2/2 contiguous on all commits"
    },
    "follow_ups": [
      "Hunk #30 (admin audit-log endpoint) deferred — depends on database migration not in this change set",
      "Consider adding integration tests for concurrent session handling (commit 2)"
    ],
    "ledger_path": "scratch/commit-loom-20260425-143000.json"
  }
}
```
