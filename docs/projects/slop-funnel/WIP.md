# WIP - Slop Funnel TUI Design

## Session: 2026-04-19 (Session 4 — RECOVERY + EXPANSION)

### ⚠️ DATA LOSS EVENT
Sessions 2-3 produced design work that was LOST from the OpenPencil document. Verified at session start:
- **Design tokens**: ZERO collections exist (claimed 37 tokens, 115 bindings — all gone)
- **Content rebuild**: Content still at 0:20 (original), not 0:182 (claimed rebuild)
- **Chunk pane**: Does NOT exist. No ChunkQueue, ChunkDetail, split view.
- **Contrast errors**: 10+ dark-on-dark instances still present
- **Structural issues**: BottomSpacer orphan, generic "Frame" names, overflow, inconsistent gaps
- **Header title**: Still says "git-branch-merge" not "slop-funnel"

**Root cause**: Unknown. Possibly document not saved, crash, or session boundary issue.

**Recovery plan**: Rebuild ALL lost work (Tasks 1-8), then expand into multi-screen design (Tasks 9-24).

### Session Timer
- **Start**: 2026-04-19 02:10:18 (epoch 1776528618)
- **Deadline**: 2026-04-19 11:10:18 (9 hours)
- **File**: `docs/projects/slop-funnel/.session-start`

### Current State — VERIFIED
Screen1 (0:6) 720x375, dark TUI mockup, ORIGINAL structure from Session 1:
```
Screen1 (0:6) 720x375
└── Header (0:7) 720x20 — dots, divider, "git-branch-merge" title
├── Menu (0:14) 720x18 — [File] [Session] [Chunks] [View] [?] Help
└── Content (0:20) 720x337 — ORIGINAL, NOT rebuilt
    ├── SessionBar (0:21) — CONTRAST ERRORS (SessionSep, SessionTime)
    ├── BranchSummary (0:30) — 6px gap (wrong), generic "Frame" names
    ├── ChunkStatus (0:55) — OLD status rows, NOT chunk list
    ├── Divider1 (0:81) — separator
    ├── ProgressSection (0:82) — OVERFLOW issues, generic names
    ├── Divider2 (0:90) — separator
    ├── Actions (0:91) — old action buttons, contrast errors
    ├── StatusBar (0:100) — CONTRAST ERRORS (4 instances)
    └── BottomSpacer (0:105) — ORPHAN, 100x0, empty
```

### Verified Errors (from describe on 0:6)
**Contrast Errors (10+)**:
- MenuHelp #888 on #0D0D0D (0:19)
- SessionSep #444 on #1A1A2E (0:25)
- SessionTime #888 on #1A1A2E (0:29)
- HintText #888 on #000 (0:60), HintSep #444 on #000 (0:61)
- ActionCommitText #888 on #333 (0:95), ActionPauseText #888 on #333 (0:97), ActionQuitText #888 on #333 (0:99)
- ProgressLabel #888 on #000 (0:84), ProgressFiles #666 on #000 (0:88)
- StatusHint #666 on #1E1E1E (0:101), StatusChunks #888 on #1E1E1E (0:102), StatusSep #444 on #1E1E1E (0:103), StatusWorktree #888 on #1E1E1E (0:104)

**Structural Issues**:
- BottomSpacer (0:105) empty orphan
- 2+ generic "Frame" siblings in SessionBar, ProgressSection
- Content gap inconsistent (16, 6, 8)
- BranchSummary gap 6px (off 8px grid)
- ProgressSection children overflow (39px > 28px available)
- StatusRows Frame children 100px tall in 24px rows

### Product Context (from Hana-san — PRESERVED)

**Core Concept**: Exhaustive extraction of value from 1-N divergent branches
- **Flow**: Changes flow FROM source branches INTO a user-chosen trunk/base branch
- **Exhaustive**: Every hunk from every branch diff must be explicitly considered
- **Agent Support**: Must support `claude` + other agents via ACP
- **External Sort Analogy**: Like an external sort algorithm for changes

**Terminology (CONFIRMED)**:
- **Chunk** = A group of 1-N hunks from any/all source branches, reviewable as ONE UNIT
- **Patch** = The diff/patch that is the mergable payload of a chunk
- Agent may REIMPLEMENT the patch — chunks are NOT just "apply hunks verbatim"
- Even chunks where ALL hunks are discarded must flow through human-in-the-loop review
- A chunk is NEVER silently skipped

**All Confirmed Decisions**: See blueprint.json globalAlerts (30+ entries)

### Design Decisions Log (PRESERVED from Sessions 1-3)
1. Split list + detail view chosen over tabbed/stacked approach (confirmed by Hana-san)
2. Chunk list shows 5 rows with scroll indicator for overflow
3. Selected row has green-tinted background (#002200) for clear focus
4. Status dots use color-coded shapes: ● ready, ◌ blocked, ✗ rejected
5. Detail panel shows: header+badge, meta, files, hunks, agent notes
6. Agent section is collapsible (collapsed by default, shows status badge)
7. Actions bar has two groups: chunk actions (left) and session actions (right)
8. Branch cards use distinct colors for each source branch
9. All text meets contrast requirements (min #BBBBBB secondary, #CCCCCC primary)

### Rebuild Plan (Tasks 1-8)
The WIP describes the TARGET state for Screen1 after rebuild:
- Content (target) → SessionBar, BranchSummary (color-coded cards), ChunkPane (list+detail), Actions, StatusBar
- ChunkPane → ChunkPaneHeader, ChunkQueue (5 rows, scroll), ChunkDivider, ChunkDetail
- All design tokens bound, zero hardcoded colors, zero describe errors

### Expansion Plan (Tasks 9-24)
- Screen2: Branch Selection / Session Setup
- Screen3: Chunk Rejection / Restructure (per-hunk granularity)
- Screen4: Agent Activity Monitor (ACP integration)
- Screen5: Session Summary / Completion
- Screen6: Pause / Session Interrupt
- Screen7: Hunk Detail / Raw Hunk Viewer
- Screen8: Session Resume / History
- Screen9: Help / Keybindings Reference
- Flow Diagram page: state transition diagram
- Interaction States page: micro-interaction states
- Component library on Components page
- Implementation handoff annotations
- Edge case and error state designs

### Session History
- **Session 1**: Initial Screen1 creation, identified contrast + structural issues
- **Session 2**: CLAIMED fixes — contrast, layout, naming (ALL LOST)
- **Session 3**: CLAIMED design tokens, 37 variables, 115 bindings (ALL LOST)
- **Session 4 (CURRENT)**: Verified data loss. Replanning. Rebuild + multi-screen expansion. 9-hour mandate.
