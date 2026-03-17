# WIP — Takumi's Desperate Diary

## Session 3 Start
2026-03-17 — Blueprint creation session for PR Split UI/termmux/refactor mandate

### Mandate (from Hana-san)
Fix 6 production issues:
1. EQUIV_CHECK button interactivity & visual rendering bugs
2. Cancel/Back button styling inconsistencies
3. Verify shell integration via termmux (FULLY-FEATURED MULTIPLEX TERMINAL)
4. Visual correctness audit across all screens
5. JS source file splitting (files too large)
6. Test package restructuring for performance

### Domain Model (First-Hand Knowledge)

#### EQUIV_CHECK Button Bugs (Root Causes CONFIRMED)
- **Not highlightable**: `viewVerificationScreen()` (pr_split_15_tui_views.js:1880-1890) NEVER applies focus styling. Always uses static `secondaryButton()`/`primaryButton()`. Other screens (config:1046, plan-review:1339, finalization:2002) DO compute focus state.
- **Not clickable**: Buttons ARE zone-marked, but string concatenation (`+`) of multi-line bordered elements produces broken zone boundaries. `secondaryButton()` renders 3-line output (border top/content/border bottom). When concatenated with `+`, subsequent buttons stack vertically instead of aligning horizontally.
- **nav-back missing**: `getFocusElements()` for EQUIV_CHECK (pr_split_16_tui_core.js:2469-2479) includes `[equiv-reverify, equiv-revise, nav-next, nav-cancel]` but NOT `nav-back`. Back button is zone-marked in renderNavBar and mouse-clickable, but unreachable via Tab.
- **Cancel borderless**: `renderNavBar()` uses `focusedButton()` (NO border) for focused Cancel/Back, but `secondaryButton()` (HAS roundedBorder) for unfocused. This causes 3-line→1-line layout shift on focus. Should use `focusedSecondaryButton()` (same border, different colors) per T011 pattern.
- **String concat vs joinHorizontal**: renderNavBar uses `lipgloss.joinHorizontal` (correct). viewVerificationScreen, viewPlanReviewScreen, viewFinalizationScreen all use `+` concatenation (BROKEN for bordered elements).

#### JS File Sizes (CONFIRMED)
- pr_split_16_tui_core.js: **5,089 lines** (29% of codebase!)
- pr_split_15_tui_views.js: **2,569 lines** (has T060 documented split plan)
- pr_split_10_pipeline.js: **2,097 lines**
- pr_split_14_tui_commands.js: **1,303 lines**
- Total 17 files, 17,670 lines

#### Termmux Shell Integration (CONFIRMED APPROACH)
- **Existing primitives**: `termmux.newCaptureSession(cmd, args, opts)`, `mux.attach(handle)`, `mux.switchTo()` (blocking passthrough), `mux.detach()`
- **No `spawnInteractive()` exists** — but all pieces are there in JS bindings
- **Shell button**: T211/T007 stub exists in pr_split_16_tui_core.js:5885-5920 (pause verify → TODO spawnInteractive → resume)
- **Design**: Create JS-level `spawnShellSession()` orchestrating existing primitives: newCaptureSession→detach Claude→attach shell→switchTo→cleanup→re-attach Claude

#### Test Structure (CONFIRMED)
- 68 PR Split test files in `package command` (internal/command/)
- internal/command: 1011s (17 min), internal/scripting: 477s (8 min)
- Tests access internal JS via eval (`prSplit._dirname()`, etc.)
- No separate test package exists
- Shared helpers duplicated across files

#### Chunk Loading Mechanism
- `//go:embed` directives in pr_split.go (lines 25-79)
- `prSplitChunks` ordered array (lines 82-106)
- `loadChunkedScript()` iterates array (lines 108-119)
- Adding new chunks: add embed directive + array entry, NO other changes needed

### Blueprint: 36 tasks (T300-T335)
- Block A (T300-T307): EQUIV_CHECK button fixes + visual audit
- Block B (T308): State management hardening
- Block C (T320-T330): Termmux shell integration (feature)
- Block D (T309): Claude mux investigation
- Block E (T310-T314): JS file splitting
- Block F (T315-T319): Test package restructuring
- Block G (T331-T335): Infrastructure, benchmarks, docs, finalization

### Blueprint Review Status
- **Pass 1**: FAIL → 5 findings (C1,C2,M1,M2,M3) → ALL FIXED
- **Pass 2**: FAIL → 4 findings (C3,M4,M5,L6) → ALL FIXED
- **Pass 3**: FAIL → 3 critical (T312 fabricated classifyWithClaude/planWithClaude, T310 fabricated viewPlanningScreen/viewSplitDetailScreen/viewFileManagementScreen, T310 wrong name viewCancelConfirm→viewConfirmCancelOverlay) + 3 moderate (T310 15b missing functions, T300 already implemented, line numbers) → ALL FIXED
- **Pass 4**: **PASS** — 1 LOW finding (file line count off by 10)
- **Pass 5**: **PASS** — 1 LOW finding (T311 handler names self-mitigating)
- **RULE OF TWO: SATISFIED** ✅ (Passes 4+5 contiguous on same diff)

### Blueprint Status: FINALIZED
36 tasks (T300-T335), 6 blocks (A-F), all verified against source code.
Ready for execution.

### Session Progress
- **T300**: Done ✅ — committed `5a6efad4` (EQUIV_CHECK focus styling)
- **T301**: Done ✅ — committed `274e460d` (nav-back in EQUIV_CHECK focus elements)
- **T302**: Done ✅ — committed `48ea09ba` (Back/Cancel use focusedSecondaryButton)
- **T302b**: Done ✅ — cross-build verified: linux/amd64, darwin/amd64, windows/amd64 all OK
- **Next**: T303 (EQUIV_CHECK button layout: joinHorizontal)
