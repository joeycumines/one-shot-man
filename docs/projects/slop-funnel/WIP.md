# WIP - Slop Funnel TUI Design

## Session: 2026-04-19 (Session 5 — EXPANSION)

### Session Timer
- **Start**: 2026-04-19 02:10:18 (epoch 1776528618)
- **Deadline**: 2026-04-19 11:10:18 (9 hours)
- **File**: `docs/projects/slop-funnel/.session-start`
- **Last verified**: ~1.5h elapsed, ~7.5h remaining

### Current State — COMPREHENSIVE DESIGN DOCUMENT

**Page 1 (ID shifts — always re-query)** — 20+ design artifacts:

#### 9 Core Screens (3x3 Flow Layout)
```
Row 1 — PRIMARY FLOW:
  Screen2-Setup        | Screen1 (DASHBOARD/HUB) | Screen5-Complete
  Branch selection     | Chunk review loop       | Session summary
  [Enter] START        | 90% of user time        | All hunks resolved

Row 2 — EXCURSIONS FROM DASHBOARD:
  Screen3-Reject       | Screen7-HunkDetail      | Screen4-Agent
  Per-hunk rejection   | Raw diff viewer         | ACP monitor
  [X] reject           | [H] view hunk           | [A] agent

Row 3 — OVERLAYS (from ANY screen):
  Screen6-Pause        | Screen9-Help            | Screen8-Resume
  Session interrupt    | Keybindings ref         | Saved sessions (later)
  [P] pause            | [?] help                | --resume flag
```

#### Intermediate States
- **Screen1-Staged** (720x375) — Post-approve, pre-confirm. Green staging banner.
  Actions: [Y] CONFIRM COMMIT | [N] UNSTAGE. User reviews externally.
- **Screen-Analyzing** (720x375) — Loading state. Agent progress through phases:
  scan branches → group hunks → analyze deps → resolve conflicts
- **Screen-ConfirmQuit** (720x375) — Modal overlay. Red warning about unresolved hunks.
  Options: [S] Save+Exit | [Q] Force quit | [Esc] Cancel

#### Responsive Variants
- **Screen1-Narrow** (480x375) — ~60 cols. Abbreviated labels, compressed detail.
  Menu: [F][S][C][V][?]. Chunk rows truncated. Detail: 4 lines.
- **Screen1-Wide** (960x375) — ~120 cols. Side-by-side panes: queue LEFT, detail RIGHT.
  Full file lists, hunk breakdown, agent section. Full StatusBar.
- **ResponsiveAnnotation** — Breakpoints: 480/720/960px. Min: 40 cols (320px).
  KEY: narrow=stacked, wide=side-by-side. Bubble Tea handles resize events.

#### Edge Case Screens
- **EdgeCase-DirtyTree** (720x375) — Trunk has uncommitted changes.
  Options: stash, commit first, switch worktree, abort.
- **EdgeCase-AgentCrash** (720x375) — Agent unresponsive (45s timeout).
  Options: wait, restart agent, save+exit, force manual mode.
  Key: session state is SAFE, no data loss.
- **EdgeCase-BranchDeleted** (720x375) — Source branch modified/deleted mid-session.
  Options: re-scan, ignore, detach branch. Note: already-extracted hunks are safe.
- **EdgeCase-TooSmall** (320x120) — Terminal below minimum size.
  Shows error + current vs minimum dimensions. Session auto-paused.

#### Cross-Branch Chunk Design
- **Screen1-CrossBranch** (720x375) — Chunk with hunks from 2+ branches.
  CROSS-BRANCH badge (yellow). Hunks grouped by branch with color-coded dots.
  Each branch section: file, line range, description.
  Agent note explains WHY cross-branch grouping was chosen.
  Reject: can reject per-branch hunks. Rejected hunks return to original branch pool.
- **CrossBranchAnnotation** — Design thinking explaining the approach.

#### Flow Annotations
- **Header** (2400x110) — Title + primary flow arrows
- **Gap-Row1toRow2** — Excursion triggers with key bindings
- **Gap-Row2toRow3** — Overlay triggers with key bindings
- **ChunkLifecycle** (2400x130) — Internal state machine:
  PENDING → READY → IN_REVIEW → APPROVED | REJECTED → pool return
  Notes: BLOCKED, BACKPRESSURE, EXHAUSTIVE GUARANTEE
- **DesignRationale** (2400x200) — 10 key design decisions explained

#### Use Case Scenarios
- **ApproveFlowAnnotation** (720x200) — Complete approve flow walkthrough
- **RejectFlowUseCase** (720x380) — Alice's rejection of Chunk #2 step by step

**Flow Diagram Page** — Text-based state diagram with design rationale

### Node ID Instability Note
Node IDs SHIFT after every batch of modifications. ALWAYS re-query with
`find_nodes` or `query_nodes` before referencing IDs.

### Product Context (PRESERVED)
See blueprint.json globalAlerts (35+ entries) for complete product context.
Key: Exhaustive extraction, chunks reviewable as one unit, agent may REIMPLEMENT,
ACP integration, external sort analogy, one chunk = one commit.

### Design Decisions (Session 5)
1-9: See previous sessions (preserved in blueprint.json)
10. Terminal visual constraints: ZERO rounded corners, NO bold, NO effects
11. Color is primary differentiator
12. ANALYZING screen: loading state with agent progress phases
13. CONFIRM_QUIT: modal overlay with destructive action warning
14. Screen layout tells the story: primary/excursions/overlays
15. Chunk lifecycle: PENDING→READY→IN_REVIEW→APPROVED|REJECTED→pool
16. Responsive behavior mandatory: terminals resize, arbitrary shapes
17. Breakpoints: 480px narrow, 720px standard, 960px wide
18. Min terminal: 40 cols (320px). Below that: error message
19. Cross-branch chunks: hunks from 2+ branches, color-coded by source
20. Agent feedback loop: rejection reasons → agent re-grouping input
21. Session state always safe on error — no data loss guarantee

### Session History
- **Session 1**: Initial Screen1 creation
- **Session 2-3**: ALL LOST (data loss event)
- **Session 4**: Verified data loss. Replanning.
- **Session 5 (CURRENT)**: Complete redesign. 9 core screens + 3 intermediate
  states + 2 responsive variants + 4 edge cases + cross-branch chunk design.
  All fonts Courier New 10px Regular. All rounded corners stripped. All bold
  stripped. Flow annotations. Use case scenarios. Design rationale. Document saved.

### Immediate Next Steps
1. Design chunk dependency visualization (dependency graph, blocked→ready)
2. Create design token system (slop-funnel variable collection)
3. Component library on Components page
4. Interaction states page (focus, hover, pressed, disabled)
5. Implementation handoff annotations (data model, events, ACP)
6. Continue expanding — True Perfection has no finish line
