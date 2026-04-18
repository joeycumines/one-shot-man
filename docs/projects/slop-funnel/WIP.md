# WIP - Slop Funnel TUI Design

## Session: 2026-04-18 (Session 3)

### Current State
- Screen1 fully rebuilt in OpenPencil MCP on "Page 1" (720x375, dark TUI mockup)
- **ZERO error-severity issues** in describe validation
- All 14+ contrast errors fixed (19 set_fill operations across 2 sessions)
- All 70+ generic node names replaced with semantic names
- All spacing on 8px grid
- Layout overflow issues resolved (FILL sizing on key nodes)
- Chunk list + detail split view designed and built
- Product direction clarified through 5 rounds of Q&A
- **Design tokens complete**: 37 semantic COLOR variables in `slop-funnel` collection (0:319)
- **All 115 colored nodes bound to tokens**: zero hardcoded colors remain in Screen1

### Screen1 Node Map (Current)

```
Screen1 (0:6) 720x375
└── Content (0:182) — 8px gap, hug
    ├── SessionBar (0:183) — session ID, repo path, time
    ├── BranchSummary (0:192) — trunk + 3 source branch cards
    │   ├── TrunkDisplay (0:194) — "main" + "TRUNK" badge + "142 commits"
    │   ├── FlowArrow (0:201) — "+←" indicator
    │   ├── Branch1Card (0:202) — "feature/ai-refactor" (purple)
    │   ├── Branch2Card (0:207) — "feature/llm-cleanup" (pink)
    │   └── Branch3Card (0:212) — "feature/exploration-v3" (blue)
    ├── ChunkPane (0:217) — Split list + detail
    │   ├── ChunkPaneHeader (0:218) — "CHUNKS" + stats + hunks progress
    │   ├── ChunkQueue (0:223) — 5 chunk rows + scroll indicator
    │   │   ├── ChunkRow1 (0:224) — ● #7 ready (GREEN, SELECTED #002200 bg)
    │   │   ├── ChunkRow2 (0:232) — ● #6 ready (GREEN)
    │   │   ├── ChunkRow3 (0:240) — ◌ #5 blocked (ORANGE, ↓#6 dep)
    │   │   ├── ChunkRow4 (0:248) — ● #4 ready (GREEN)
    │   │   ├── ChunkRow5 (0:256) — ✗ #3 rejected (RED #FF8888)
    │   │   └── ScrollIndicator (0:264) — "↓ 2 more ↓"
    │   ├── ChunkDivider (0:266) — 1px separator
    │   └── ChunkDetail (0:267) — Selected chunk detail, 8px gap, 8px pad
    │       ├── DetailHeader (0:268) — "● #7 refactor auth middleware" + READY badge
    │       ├── DetailMeta (0:272) — source + deps info
    │       ├── DetailFiles (0:275) — "files:" + 3 file names
    │       ├── DetailHunks (0:280) — 5 hunk lines with ✔ and line ranges
    │       └── DetailAgent (0:286) — "agent:" + collapsible note
    ├── Actions (0:289) — [Enter] APPROVE, [X] REJECT, [E] EXT DIFF | [P] PAUSE, [Q] QUIT
    └── StatusBar (0:302) — KeyHints, PendingCount, Separator, WorktreeInfo
```

### Design Token System (Variable Collection: `slop-funnel`, ID 0:319)

**Collection**: `slop-funnel` (ID 0:319), Mode ID 0:320, 37 COLOR variables (IDs 0:321–0:357)
**All 115 colored nodes in Screen1 are bound to these tokens — zero hardcoded colors remain.**

#### Background Tokens (16)
| Token Name | ID | Value | Used By |
|---|---|---|---|
| bg-screen | 0:321 | #000000 | Screen1 root, BranchSummary |
| bg-header | 0:322 | #1E1E1E | Header, StatusBar |
| bg-menu | 0:323 | #0D0D0D | Menu, Actions bar |
| bg-session | 0:324 | #1A1A2E | SessionBar |
| bg-pane | 0:325 | #0A0A0A | ChunkPane |
| bg-pane-header | 0:326 | #111111 | ChunkPaneHeader |
| bg-queue | 0:327 | #050505 | ChunkQueue |
| bg-detail | 0:328 | #080808 | ChunkDetail |
| bg-selected | 0:329 | #002200 | Selected chunk row, status badge bg |
| bg-button | 0:330 | #333333 | Secondary buttons, divider |
| bg-button-primary | 0:331 | #00FF00 | Approve button |
| bg-trunk | 0:332 | #001100 | Trunk display card |
| bg-trunk-badge | 0:333 | #004400 | "TRUNK" badge background |
| bg-branch-1 | 0:334 | #110022 | Branch 1 card (ai-refactor) |
| bg-branch-1-tag | 0:335 | #110033 | Branch 1 tag in chunk rows |
| bg-branch-2 | 0:336 | #220011 | Branch 2 card (llm-cleanup) |
| bg-branch-3 | 0:337 | #001122 | Branch 3 card (exploration-v3) |

#### Text Tokens (18)
| Token Name | ID | Value | Used By |
|---|---|---|---|
| text-primary | 0:338 | #FFFFFF | Menu items, detail title |
| text-highlight | 0:339 | #DDDDDD | Selected chunk description |
| text-body | 0:340 | #CCCCCC | File names, scroll indicator |
| text-secondary | 0:341 | #BBBBBB | Stats, meta, counts |
| text-muted | 0:342 | #AAAAAA | Help menu, key hints |
| text-accent | 0:343 | #00FFFF | (reserved) |
| text-button | 0:344 | #CCCCCC | Secondary button labels |
| text-ready | 0:345 | #00FF00 | Ready status, hunk check marks |
| text-hunk | 0:346 | #00CC00 | Hunk line ranges |
| text-blocked | 0:347 | #FF9900 | Blocked status (unused in current screen) |
| text-rejected | 0:348 | #FF8888 | Rejected chunk text |
| text-warning | 0:349 | #FFFF00 | "CHUNKS" title, flow arrow |
| text-branch-1 | 0:350 | #CC99FF | Branch 1 name/pipes |
| text-branch-2 | 0:351 | #FF99CC | Branch 2 name/pipes |
| text-branch-3 | 0:352 | #99CCFF | Branch 3 name/pipes |
| text-button-primary | 0:357 | #000000 | Approve button label |

#### Structural Tokens (3)
| Token Name | ID | Value | Used By |
|---|---|---|---|
| dot-close | 0:353 | #FF5F57 | Window dot red |
| dot-minimize | 0:354 | #FFBD2E | Window dot yellow |
| dot-maximize | 0:355 | #28C840 | Window dot green |
| border-default | 0:356 | #3E3E3E | Header divider |

### Product Context (from Hana-san)

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

**All Confirmed Decisions**: See blueprint.json globalAlerts (22+ entries)

### Design Decisions Log
1. Split list + detail view chosen over tabbed/stacked approach (confirmed by Hana-san)
2. Chunk list shows 5 rows with scroll indicator for overflow
3. Selected row has green-tinted background (#002200) for clear focus
4. Status dots use color-coded shapes: ● ready, ◌ blocked, ✗ rejected
5. Detail panel shows: header+badge, meta, files, hunks, agent notes
6. Agent section is collapsible (collapsed by default, shows status badge)
7. Actions bar has two groups: chunk actions (left) and session actions (right)
8. Branch cards use distinct colors for each source branch
9. All text meets contrast requirements (min #BBBBBB secondary, #CCCCCC primary)

### Next Steps
- [x] Get Hana-san review on chunk list + detail design (Task 6 acceptance criteria)
- [x] Define design tokens as OpenPencil variables (Task 9) — DONE, 37 tokens, 115 bindings
- [ ] Create component library on Components page (Task 8)
- [ ] Design additional screens (Task 7)
- [ ] Document interaction patterns and keyboard nav (Task 10)
- [ ] Final design review and export (Task 11)

### Session History
- **Session 1**: Initial Screen1 creation, identified 14+ contrast errors + overflow issues
- **Session 2**: Fixed all contrast errors, rebuilt Content frame, added chunk list + detail split view
- **Session 2 (continuation)**: Fixed 19 remaining contrast errors, renamed 70+ nodes, 8px grid compliance, zero errors in validation
- **Session 3**: Created `slop-funnel` variable collection with 37 COLOR tokens. Bound all 115 colored nodes to semantic tokens. Zero hardcoded colors remain. Tokens exported as CSS custom properties.
