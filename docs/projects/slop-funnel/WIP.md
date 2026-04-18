# WIP - Slop Funnel TUI Design

## Session: 2026-04-18

### Current State
- Screen1 exists in OpenPencil MCP on "Page 1" (720x375, dark TUI mockup)
- Screen1 represents the main dashboard view of the `slop-funnel` TUI
- Identified numerous design issues (color contrast, layout, overflow)

### Product Context (from Hana-san)

**Core Concept**: Exhaustive extraction of value from 1-N divergent branches
- **Flow**: Changes flow FROM source branches INTO a user-chosen trunk/base branch (NOT necessarily common base)
- **Exhaustive**: Every hunk from every branch diff must be explicitly considered
- **Agent Support**: Must support `claude` + other agents via ACP (Agent Communication Protocol)
- **External Sort Analogy**: Like an external sort algorithm for changes

**Terminology (CONFIRMED)**:
- **Chunk** = A group of 1-N hunks from any/all source branches, reviewable as ONE UNIT by the human-in-the-loop
- **Patch** = The diff/patch that is the mergable payload of a chunk (term TBD — Hana-san said I need to decide)
- The agent may REIMPLEMENT the patch — chunks are NOT just "apply hunks verbatim"
- Even chunks where ALL hunks are discarded must still flow through human-in-the-loop review
- A chunk is NEVER silently skipped — this is the core guarantee of "exhaustive"

**Integration (CONFIRMED)**:
- New osm subcommand (`osm slop-funnel` or similar)
- Lives in one-shot-man repo
- Uses osm's Go framework, scripting runtime, and session management
- **Session state**: All slop-funnel state tracked via osm sessions (`osm session id` etc). TUI reads/writes session state.

**Launch Flow (CONFIRMED)**:
- Branches selected entirely WITHIN the TUI — not CLI args
- May also support go-prompt terminal mode bootstrapped like `osm super-document`
- Seamless transition between "terminal-style TUI" and "advanced interactive user interface TUI"
- The entire experience is interactive

**Chunk Lifecycle (CONFIRMED)**:
- Chunks are STAGED UP by a workflow-style orchestration (internal, with backpressure)
- Chunks can be REJECTED with a reason
- The orchestrator stages an appropriate number of chunks while the user works on earlier ones
- **One chunk = one commit**
- Agent integration details TBD (to be refined over time)

**Agent Interaction (CONFIRMED)**:
- There MUST be feedback and interaction
- Higher-level than osm pr-split (less direct)
- May need to surface agent's train of thought for debugging/guidance
- Details TBD — dependent on agentic tool capabilities, requiring exploration and research

**Design Decisions (CONFIRMED)**:
- Screen1 structure is roughly right — refine, don't redesign from scratch
- Discovery phase — need proper requirements elicitation, not abstract questions
- Hana-san wants me to "throw shit at the wall" and learn through specific questions

### Screen1 Issues Identified
1. **Color contrast errors**: Multiple dark-on-dark text (gray on black) - 14 instances
2. **Layout overflow**: Frame children (100px) overflow parent rows (24px) in StatusRows
3. **Inconsistent gaps**: Content children use mixed gaps (16, 6, 8)
4. **Generic naming**: Multiple "Frame" siblings - ambiguous
5. **HUG sizing + justify=between**: Won't work as expected
6. **BottomSpacer**: 100px width, 0 height, empty - orphan
7. **Visual concerns from AI analysis**: information density, color overload, terminology clarity

### Next Steps
- [ ] Proper requirements elicitation with Hana-san (specific scenario-based questions)
- [ ] Fix Screen1 contrast errors (mechanical, no product dependency)
- [ ] Fix Screen1 structural issues (mechanical, no product dependency)
