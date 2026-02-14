# Git Sync Design: Structured Repository Format

> Design document for T030, T031, T032.
> Status: **DESIGN** — no git operations implemented yet.

## Overview

osm's git sync feature provides optional synchronization of configuration,
goals, scripts, and prompt notebooks via a plain Git repository. The sync
repository acts as a **chronological notebook** of prompts and notes, with
configuration and goal definitions as a secondary concern.

Design principles:

1. **Git-native** — the repository format is human-readable, diff-friendly,
   and merge-friendly. No binary blobs.
2. **Offline-first** — all operations work on a local clone. Push/pull are
   explicit user actions.
3. **Append-mostly** — notebook entries are chronologically ordered and
   append-only. Configuration uses last-writer-wins.
4. **No API keys** — uses standard `git` CLI via `osm:exec`. No GitHub/GitLab
   API integration required.

---

## 1. Repository Structure (T030)

```
osm-sync-repo/
├── config/                    # Synced configuration
│   └── config                 # Main osm config file (dnsmasq-style)
├── goals/                     # Custom goal definitions
│   ├── my-goal.json           # Goal JSON files (same format as osm goals)
│   └── team-goal.json
├── notebooks/                 # Chronological prompt/note storage
│   ├── 2024/
│   │   ├── 01/
│   │   │   ├── 2024-01-15-code-review.md
│   │   │   └── 2024-01-15-code-review/
│   │   │       ├── prompt.txt
│   │   │       └── context.txtar
│   │   └── 02/
│   │       └── 2024-02-03-refactor-notes.md
│   └── 2025/
│       └── 01/
│           └── 2025-01-10-api-design.md
├── scripts/                   # User scripts (JS)
│   └── my-helper.js
└── README.md                  # Auto-generated index
```

### Directory Descriptions

| Directory      | Purpose                                        | Merge strategy    |
|----------------|-------------------------------------------------|-------------------|
| `config/`      | osm configuration (global + command sections)   | Last-writer-wins  |
| `goals/`       | Goal JSON definitions discovered at load time   | Last-writer-wins  |
| `notebooks/`   | Dated prompt entries (the primary content)       | Append-only       |
| `scripts/`     | User JS scripts discovered via module paths     | Last-writer-wins  |

### File Naming Conventions

Notebook entries use date-prefixed names for chronological ordering:

```
YYYY-MM-DD-<slug>.md          # Single-file entry (markdown with frontmatter)
YYYY-MM-DD-<slug>/            # Multi-file entry (gist-like directory)
├── prompt.txt                # The prompt text
├── context.txtar             # Context files in txtar format
└── metadata.yaml             # Optional metadata
```

Slugs are derived from the entry title: lowercased, spaces replaced with
hyphens, non-alphanumeric characters stripped. Maximum 50 characters.

### Multi-File Prompts

Complex prompts that include context files are stored as directories (similar
to GitHub Gists). The directory contains:

- **`prompt.txt`** — The prompt text as sent to the LLM.
- **`context.txtar`** — Context files bundled in Go's txtar format. This is
  a simple, human-readable archive format that diffs cleanly in Git.
- **`metadata.yaml`** — Optional YAML metadata (session ID, tags, goal used).

Single-file entries use Markdown with YAML frontmatter:

```markdown
---
date: 2024-01-15T10:30:00Z
tags: [code-review, refactoring]
goal: code-review
session: abc123
---

# Code Review: Auth Module Refactor

Review the following changes to the authentication module...
```

### Metadata Schema

YAML frontmatter (single-file) or `metadata.yaml` (multi-file):

```yaml
date: 2024-01-15T10:30:00Z    # ISO 8601 timestamp (required)
tags: [code-review]            # Optional classification tags
goal: code-review              # Goal used to generate this prompt (optional)
session: abc123                # Session ID at time of save (optional)
title: Auth Module Refactor    # Human-readable title (optional, derived from slug)
```

---

## 2. Config Sync Design (T031)

### Configuration Keys

```
sync.repository    <git-url>    # Git repository URL for sync
sync.enabled       true|false   # Enable/disable sync (default: false)
sync.auto-pull     true|false   # Auto-pull on osm startup (default: false)
sync.local-path    <path>       # Local clone path (default: ~/.one-shot-man/sync)
```

### Push/Pull Operations

All git operations use `osm:exec` to shell out to `git`. No git library
dependency.

#### Pull (sync from remote → local)

```
osm sync pull
```

1. If local clone does not exist: `git clone <sync.repository> <sync.local-path>`
2. If local clone exists: `git pull --rebase origin main`
3. Copy `config/config` → osm config location (overwrite)
4. Goal and script directories are auto-discovered via existing mechanisms
   (configure `goal.paths` and `script.module-paths` to include sync paths)

#### Push (sync from local → remote)

```
osm sync push
```

1. Copy current osm config → `config/config` in sync repo
2. `git add -A`
3. `git commit -m "osm sync: <timestamp>"`
4. `git push origin main`

### Config Merge Strategy

**Simple overwrite (last-writer-wins):**

- `pull` overwrites local config with remote config
- `push` overwrites remote config with local config
- No automatic three-way merge

Rationale: osm's config format (dnsmasq-style key-value) is not well-suited
for automatic merging. Manual merge is clearer and less error-prone for
configuration drift.

### Conflict Handling

If `git pull --rebase` encounters conflicts:

1. Display the conflicted files to the user
2. Print instructions for manual resolution
3. Return a non-zero exit code
4. Do NOT attempt automatic conflict resolution

```
Error: sync pull encountered merge conflicts.
Conflicted files:
  - config/config
  - goals/my-goal.json

Resolve conflicts manually in: ~/.one-shot-man/sync
Then run: osm sync push
```

---

## 3. Notebook Design (T032)

### Commands

#### Save Current Prompt

```
osm sync save [--title <title>] [--tags <tag1,tag2>]
```

1. Reads the current session's context (files, diffs, notes, prompt)
2. Generates a dated directory entry under `notebooks/YYYY/MM/`
3. Writes `prompt.txt` with the assembled prompt
4. Writes `context.txtar` with all context items
5. Writes `metadata.yaml` with session info, tags, goal

If the prompt is simple (text only, no context files), writes a single
Markdown file instead of a directory.

#### List Saved Entries

```
osm sync list [--year <YYYY>] [--tag <tag>] [--limit <n>]
```

Lists notebook entries in reverse chronological order:

```
2024-01-15  code-review         Auth Module Refactor
2024-01-10  api-design          REST API v2 Design
2024-01-03  refactoring         Extract Service Layer
```

#### Load Saved Entry

```
osm sync load <entry-slug-or-date>
```

1. Locates the entry in the notebooks directory
2. Loads `prompt.txt` into the current session's context
3. If `context.txtar` exists, extracts and adds context files

### Local-First Operation

The save/list/load commands operate on the **local sync directory** only.
No git operations occur. The user explicitly pushes/pulls:

```
osm sync save --title "My Review"    # Save locally
osm sync push                         # Push to remote
osm sync pull                         # Pull from remote
osm sync list                         # List local entries
osm sync load 2024-01-15-my-review   # Load into session
```

### Entry Deduplication

Entries are identified by their dated path. Saving with the same date and
slug overwrites the previous entry (which is visible in git history). A
numeric suffix is appended for same-day, different-slug conflicts:

```
2024-01-15-code-review.md
2024-01-15-code-review-2.md
```

---

## 4. Implementation Phases

### Phase 1: Local Skeleton (Current — T030-T032)

- [x] Design document (this file)
- [ ] `SyncCommand` with `save` and `list` subcommands (local directory only)
- [ ] Save writes session context to dated directory
- [ ] List reads and displays saved entries
- [ ] No git operations

### Phase 2: Git Operations (Future)

- [ ] `pull` subcommand (`git clone` / `git pull --rebase`)
- [ ] `push` subcommand (`git add` / `git commit` / `git push`)
- [ ] Config sync (copy config to/from sync repo)
- [ ] Conflict detection and user messaging

### Phase 3: Integration (Future)

- [ ] Auto-pull on startup (when `sync.auto-pull = true`)
- [ ] Goal discovery from sync repo (`goal.paths` integration)
- [ ] Script discovery from sync repo (`script.module-paths` integration)
- [ ] `load` subcommand (restore entry into session)

---

## 5. Design Decisions

### Why txtar for context?

Go's `txtar` format is simple, human-readable, and diffs cleanly in git.
It's already used in Go's own test infrastructure. Alternative formats
considered:

- **tar/zip**: Binary, doesn't diff in git
- **Multiple files**: Harder to manage atomically
- **JSON**: Verbose for file content

### Why not use a git library?

Shelling out to `git` via `osm:exec`:

- Works everywhere git is installed (which is everywhere osm is useful)
- No CGo dependency (libgit2)
- No pure-Go library maturity concerns
- Users can inspect and modify the sync repo with standard git tools

### Why append-only notebooks?

Notebook entries are historical records. Modifying or deleting past entries
loses context. Git history provides the audit trail, but the working tree
should always show the complete chronological record.

Configuration is different: it has a single "current" state, so
last-writer-wins is appropriate.

### Why not GitHub Gists as backend?

GitHub Gists were considered as an alternative to a full repository:

- **Pro**: Lower friction (no repo creation), natural multi-file support
- **Con**: Requires GitHub API/authentication, not truly offline
- **Con**: Gists don't support directories (flat file collection)
- **Con**: No chronological organization built-in

A full git repository is more flexible and aligns with osm's offline-first
principle. Gist integration could be a future alternative backend.
