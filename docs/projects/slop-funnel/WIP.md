# WIP - Slop Funnel TUI Design

## Session: 2026-04-19 (Session 5 — EXPANSION)

### Session Timer
- **Start**: 2026-04-19 02:10:18 (epoch 1776528618)
- **Deadline**: 2026-04-19 11:10:18 (9 hours)
- **File**: `docs/projects/slop-funnel/.session-start`
- **Last verified**: ~1.5h elapsed, ~7.5h remaining

### Current State — SCREENS + FLOW MAP

**Page 1 (0:3984)** — 9 screens in 3x3 flow layout + 2 additional screens + annotations

**Flow Layout (3 rows x 3 columns):**
```
Row 1 — PRIMARY FLOW (y=120):
  Screen2-Setup (0:4963) at (0, 120)     | Screen1 (0:5756) at (800, 120) | Screen5-Complete (0:5214) at (1600, 120)
  SETUP → DASHBOARD (HUB) → COMPLETE

Row 2 — EXCURSIONS FROM DASHBOARD (y=595):
  Screen3-Reject (0:5056) at (0, 595)    | Screen7-HunkDetail (0:5488) at (800, 595) | Screen4-Agent (0:5140) at (1600, 595)
  [X] reject chunk → [H] view hunk → [A] agent monitor

Row 3 — SESSION MANAGEMENT + OVERLAYS (y=1070):
  Screen6-Pause (0:5300) at (0, 1070)    | Screen9-Help (0:5412) at (800, 1070) | Screen8-Resume (0:5348) at (1600, 1070)
  [P] pause → [?] help → --resume (later feature)

Additional screens (y=1600):
  Screen-ConfirmQuit (0:6739) at (0, 1600) — modal overlay for destructive quit
  Screen-Analyzing (0:6681) at (800, 1600) — loading state during initial analysis

Annotations:
  Header (0:6599) at (0, 0) — title + flow arrows
  Gap-Row1toRow2 at (0, 497) — excursion triggers
  Gap-Row2toRow3 at (0, 972) — overlay triggers
  ChunkLifecycle at (0, 1450) — internal state machine
  DesignRationale (0:6750) at (1600, 1600) — 10 design decisions explained
```

**Flow Diagram Page (0:4695)** — text-based state diagram with design rationale

### Node ID Instability Note
Node IDs SHIFT after every batch of modifications (font changes, radius changes, etc).
ALWAYS re-query with `find_nodes` or `query_nodes` before referencing IDs.

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

**All Confirmed Decisions**: See blueprint.json globalAlerts (35+ entries)

### Design Decisions Log (Session 5 additions)
10. Terminal visual constraints: ZERO rounded corners, NO bold text weight, NO visual effects
11. Color is primary differentiator (not weight, not size, not shape)
12. ANALYZING screen: loading state showing agent progress through phases
13. CONFIRM_QUIT: modal overlay with warning about unresolved hunks
14. Screen layout tells the story: primary flow across top, excursions below, overlays at bottom
15. Chunk lifecycle: PENDING → READY → IN_REVIEW → APPROVED | REJECTED → pool return

### Session History
- **Session 1**: Initial Screen1 creation, identified contrast + structural issues
- **Session 2**: CLAIMED fixes — contrast, layout, naming (ALL LOST)
- **Session 3**: CLAIMED design tokens, 37 variables, 115 bindings (ALL LOST)
- **Session 4**: Verified data loss. Replanning. Rebuild + multi-screen expansion. 9-hour mandate.
- **Session 5 (CURRENT)**: Rebuilt all 9 screens. Fixed ALL fonts to Courier New 10px Regular.
  Stripped 71 rounded corners and 43 bold weights for terminal fidelity.
  Created flow layout with annotations. Created Analyzing + ConfirmQuit screens.
  Added Design Rationale annotation. Started product flow mapping.

### Immediate Next Steps (Priority Order)
1. **Deep product review**: Walk through each screen and verify content matches confirmed requirements
2. **Screen annotations**: Add design thinking notes ON each screen (not just the overview)
3. **Task 4 (BranchSummary)**: Still "Not Started" in blueprint — color-coded branch cards
4. **Design tokens**: Create slop-funnel variable collection, bind all colored nodes
5. **Component library**: Components page with reusable TUI elements
6. **Edge cases**: Empty repo, agent crash, dirty working tree, branch deleted mid-session
7. **Interaction states**: Focus states, hover, pressed, disabled on dedicated page
8. **Implementation handoff**: Annotate each screen with component decomposition + data model

### What Hana-san Wants (DIRECTIVE.txt)
- Think like PM + Designer + Engineer
- Map out ACTUAL flows, not just arrange boxes
- Multiple screens with clear relationships
- Annotations showing design thinking
- Prove competence through exhaustive, detailed work
- CONTINUOUS expansion toward True Perfection
