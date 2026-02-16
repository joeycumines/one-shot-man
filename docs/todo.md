# TODO - NO TOUCHY look with your _eyes_ bud

This is not an actual TODO list. Consider it as much a TODO list as your Product Manager's project roadmap.

- Built-in first-class Git synchronisation support (optional, configurable) **DONE (T104-T106)**
    - Add support to synchronise configuration including goals to a Git repository **DONE (T106)** — sync repo `goals/` and `scripts/` auto-discovered at startup
    - A _structured_ Git repository format that can also act as a notebook (largely chronological) for prompts and notes - including multi-file prompts similar to GitHub gists (or even using GitHub gists as a backend?) sounds good to me tbh **DONE (T104-T105)** — `osm sync save/list/load/init/push/pull` implemented with dated Markdown + YAML frontmatter format
    - This requires some proper designing - don't want to half-bake this one (I have a clear use case for it personally, so I will probably just align with that)
    - Consider adding a sub-feature for syncing "common" config (system prompts, personal preferences, etc.) to a common repo structure, with intelligent conflict handling:
        - Prompt for override when file exists and state is unknown
        - Auto-override for files that exist and are in a known state (tracked via commit SHA)
        - Handle gitignored files specially - personal preference files may need manual updates (since they're typically gitignored and therefore won't receive auto-updates from pulls)
        - Consider special-case handling for resolving compatibility concerns (e.g., schema version mismatches, deprecated fields, etc.)
    - Replace shell-out to system `git` binary with github.com/go-git/go-git/v6 library for core sync operations
        - Current implementation shells out to `git` binary (exec.Command) in sync.go and sync_startup.go. This adds a system dependency and complicates testing.
        - Replace with go-git for: clone, add, commit, push, pull operations
        - Exception: diff commands (`osm ctx add --from-diff`, etc.) continue using CLI git — no change needed there
        - Design constraints for go-git usage:
            - Prefer streaming file reads/writes directly from git objects — avoid temp files, avoid buffering entire files to disk unnecessarily
            - Use case is syncing configuration files — should be small, shouldn't need streaming-large-file support, but don't add unnecessary disk I/O
            - go-git API is "fucky" (their words, not mine) — use pragmatically, don't force it where it fights you
            - Particularly problematic areas to watch: tree traversal, blob reading, working directory management
        - Key current behaviors to preserve:
            - pull uses --rebase strategy (not merge)
            - push commits with timestamp message: "osm sync: <RFC3339>"
            - Conflict detection from stderr output ("CONFLICT" or "could not apply")
            - sync.local-path config key for custom sync root
            - sync.auto-pull runs non-blocking pull on osm startup
        - Implementation files: internal/command/sync.go, internal/command/sync_startup.go
    - ~~Remove unused sync.enabled config key from internal/config/schema.go (line 495). It is defined but never read or used anywhere in the codebase. Also clean up any test or documentation references.~~ **DONE (T131)**
- Add `hot-<shortname>` aliases to copy snippets, activating them based on the name of the... mode? Seems reasonable - that'd cover the fairly coupled/integrated variants of custom scripts, and the builtins which all use it. Integrated nicely, it could be configurable, and there could be a command to output the embedded ones. Examples of intended use case include situational follow-up prompts, e.g. "critical next steps: prove the issue exists and that it is fixed" prompt. Will be important to disclaimer that they are subject to arbitrary change as I tweak it. Maybe a warning if you use it w/o overriding it? Would need another config option to disable the warning lol. **DONE (T072)**
    - I'd personally use this for variants of agentic session kickoff prompts, which I tend to use when I have a populated blueprint.json
- Fix behavior when you use `copy` to copy context using `osm:ctxutil`/`contextManager` - all the current built-in scripts use this implementation in some capacity. It is desirable to support "refreshing" on demand in a just-in-time fashion, just prior to copy. To pick up new files added to a directory, specifically. **DONE (T067)**
- Goal list should probably also show SUMMARIZED params. Might need to consolidate custom list logic. Provide means to avoid the custom logic? Maybe just expose or ensure it is possible to get the context manager list? Should be fine to just require and get the thing, I thought, but maybe I made it a factory function? The state is on the TUIManager or whatever the context type was hmmmmmm. **DONE (T068)**
- `osm goal morale-improver` command should print after `copy`: [Hint: Try a follow-up]       Double the length, and include key specifics from the context that are likely to otherwise fail to be identified - consider this the only context, beyond the codebase.
  OR: Double the length, and include key specifics from the context that are likely to otherwise fail to be identified - consider this the only context, beyond the codebase. **Explicitly** demand use of the todo tool to track exhaustively all known requirements. Reference tools like "#todo" - literal #todo without any quotes (") or backticks (`) etc. Do not use terms that would lead the AI to thinking it is AI - worse outcomes that way. **DONE (T074)**
- ~~Consider consolidating the two prompt variants~~ ~~Ensure the two useful prompt variants,
  (tui.registerMode, and tui.createAdvancedPrompt) are well documented, and are equivalent in
  terms of supported go-prompt features.~~ **DONE**: Consolidated via shared `buildGoPrompt` builder.
  Both `runAdvancedPrompt` (registerMode path) and `jsCreatePrompt` now use the same builder,
  ensuring feature parity (colors, reader/writer injection, maxSuggestion, dynamicCompletion,
  executeHidesCompletions, escapeToggle, key bindings).
    - ~~Rename the "advanced" prompt to something less dumb~~ **DONE**: Renamed to `tui.createPrompt`.
      ~~`tui.createAdvancedPrompt` kept as deprecated alias with warning.~~ **DONE (T075)**: Deprecated alias removed.
- Add option to the osm:ctxutil add context command to add files from a diff (`git diff <what> --name-only`) **DONE (T094) — `add --from-diff [commit-spec]` with gitref completion, 6 tests**
- Expose the Go `flag` package as a JS module `osm:flag` for script authors to use **DONE (T029 — verified via coverage audit)**
    - Probably need to take a look at how arguments are passed down to the `osm script` command, as well
    - _COULD_ Leverage the "lazy init" pattern I originally intended for declarative-style scripts - buuuuut I've since moved to more imperative style ones, so perhaps not
- Add support for completion for arguments for REPL commands within `osm:ctxutil` module using the `osm:flag` module **DONE (T071)**
    - N.B. Unlike the other builtins, a portion of `osm:ctxutil` is partially implemented in JavaScript, as I ported it from a prototype script
    - This is basically extending support to subcommands - this item is just a quality of life improvement
- QoL improvements to prompt-flow, e.g. allow `use` without `goal` or `generate` (i.e. add one-step mode), add `footer` support for the second prompt **DONE (T066, T092, T093) — auto-generate on first copy (T066), one-step mode (T092), footer in prompt-flow CLI and goal system (T093)**
    - **Refinement**: Make the first `copy` command (and ONLY the first copy) automatically perform `generate` prior to copying. Currently, if you forget to run `generate` and just use `copy`, it copies what appears to be just a newline. Since the "meta-prompt" (what gets generated) is editable and `generate` overwrites it, only the first copy should trigger this automatic generate - subsequent copies should work as-is so the user can edit the meta-prompt without it being blown away.
        - Note: Any `generate` operation likewise clears the "can trigger auto-generate" state from the first copy - after a generate, the next `copy` will NOT auto-generate (because the user has now explicitly generated, so they know what they're copying). Only the very first copy before any generate should auto-generate. **DONE (T066)**
- Add `exec` context builder command as part of `contextManager`
    - To clarify, this is intended to mean "add scaffolding to the `osm:ctxutil` module to allow executing arbitrary shell commands and capturing their output as context, inclusive of providing means to aid wiring up the REPL commands to interact with it"
    - Essentially a generalization of the existing `diff` command - consider what would be necessary to retain the EXACT existing behavior for the `diff` command, when porting it to use the same implementation under the hood
    - INCLUDE completion support (see the below item - might be tricky / seems like it might require piggybacking off of shell completion logic? Alternatives exist though. Explore all of them.) **DONE (T069)**
- Consider integrating git diff completion support into the diff `contextManager` command **DONE (T070)**
    - Unlike the generalised `exec` command, this is specifically for the existing `diff` command - should be feasible to implement specifically. Probably want to use Go directly. Honestly, could remove the dependency on native Git. Might regret, the sole viable Go implementation is a pain to work with.
- Command and option tightening and validation across the board
    - Need to revalidate how the logging API is wired up - the option for log level and output path was added to `osm script`, but should be configurable for _all_ script-like commands, that exercise the scripting engine, could probably use some additional means to configure them, and probably need refactor to wire up more sane (I was using it for debugging / as a means to implement integration tests w/o depending on scraping PTYs - I didn't properly validate it) **DONE (T047, T111) — All 5 script commands share `scriptCommandBase.PrepareEngine()` with unified log.level/log.file/log.buffer flags. Config fallback via `resolveLogConfig()`. Log API stabilized: removed \"undercooked\" label, expanded documentation.**
    - Consider making commands and subcommands correctly fail upon receiving unexpected arguments or options **DONE (T073)**
- Investigate/fix/implement `osm config <key> <value>` to persist config changes to disk **DONE (T065)**
- Add ability to include JS modules directly **PARTIAL — Module resolution implemented via `script.module-paths` config key and `require()` in Goja runtime; standards-compliant ESM not pursued**
    - Need to pick the module resolution / loader strategy. Need to revalidate my understanding of the current standards in the JS ecosystem. Almost certainly want to pick a "sane" subset, tailored for the specific intended use cases. Standards compliance is ideal, however-want to avoid ruling out future interoperability.
- Implement partially-compliant fetch API backed by the Go http client **DONE (T028 — verified via coverage audit)**
    - If no streaming is required, this is actually quite straightforward: https://gist.github.com/joeycumines/c7da3dbb786428dcaf45f5884cd99798
    - There _is_ nuance in the allowed headers and wiring up of options such as host (nuance not reflected in that gist), but still, fairly straightforward
    - Streaming support is... er, involved.
        - I've implemented multiple variants of application-layer shims, in the past, but I'd likely want to expose an in-process gRPC channel between client (JS) and server (Go) to properly support it
        - An alternative, even-more-cooked approach: Implement a reactor-style HTTP client in Go, backed by the (as-yet unpublished) `github.com/joeycumines/go-eventloop` module, and expose a _compliant_ fetch API on top of that
            - Honestly supporting a full-on HTTP client sounds, well, ridiculous - this would be _fun_ but probably not _sane_
        - Explore alternative: An eventloop-native in-process gRPC channel implementation, that exposes a Goja JS API _and_ a Go-compatible API <--- This is the sexiest option, but various challenges exist. Seriously attractive to be able to avoid serializing messages at all, though - this can support by far the lowest overhead e.g. allocation implementation.
- Evaluate potential integration with `github.com/joeycumines/MacosUseSDK`
    - The in-process gRPC channel implementation idea would allow for exposing gRPC APIs to JS code - strongly consider implementing that then using a gRPC proxy mechanism to expose MacOSUseSDK functionality to JS code
- Add support for https://code.visualstudio.com/docs/copilot/customization/prompt-files ? **PARTIAL — `.prompt.md` file discovery implemented via `prompt.file-paths` config key and goal system; VS Code-specific integration not pursued**
- Code review splitter - prompts seem particularly LLM dependent, stalled
    - This would be far easier as a proper workflow engine lol
- Refine "goal" and "script" autodiscovery mechanisms (currently prototype status/needs attention) **PARTIAL (T109) — added `osm goal paths` and `osm script paths` subcommands with source annotations, existence status, config validation warnings, and shell completions. Debug logging already existed via goal.debug-discovery and script.debug-discovery config keys.**
- Investigate implementing Anthropic prompt library (https://platform.claude.com/docs/en/resources/prompt-library/library) **PARTIAL (T074, T107) — morale-improver, bug-buster, code-optimizer, code-explainer, meeting-notes adapted from Anthropic Prompt Library; pii-scrubber and prose-polisher added as Tier 2 goals**
- Iterate on configuration model for better extensibility and consistency (feels undercooked) **PARTIAL (T065, T082) — schema-aware validation, persistence, and comprehensive documentation added**
- Enhance definitions and integration with `github.com/joeycumines/go-prompt` implementation
- Review `tview`/`tcell` support for refinement or removal (probably just leave it as-is for now, remove eventually - bubbletea is the winner of this one) **DONE (T103) — Full audit (T097), then executed removal: deleted 6 tview files (~2,100 lines), removed TViewManagerProvider, go mod tidy removed tview/tcell deps, updated all docs.**
- Plan system-style logging (file output, tailing) - likely deferred **DONE (T111) — `osm log` command with `tail`/`follow` subcommands, `-f`/`-follow` flags, rotation detection, `log.file`/`log.level`/`log.buffer-size`/`log.max-size-mb`/`log.max-files` config keys. JS `log` API stabilized with 8 methods documented.**
- Fix duplicate log lines for purged sessions etc? **DONE (T095) — Already fixed: cleanup.go returns CleanupReport, session.go writes through io.Writer. Regression test `TestSessionsPurge_NoDuplicateLogLines` confirms no duplication.**
- Implement automatic session cleanup scheduler using SessionConfig (AutoCleanupEnabled, CleanupIntervalHours, MaxAgeDays, MaxCount, MaxSizeMB) **DONE (T096) — CleanupScheduler wired via cleanup_helper.go into scriptCommandBase. 7+3 tests. All 5 SessionConfig fields respected.**
- Breaking change / migration while the going is good: align on `~/.osm` as the config directory **DONE (T126) — Default config directory migrated from `~/.one-shot-man/` to `~/.osm/` with backward-compatible fallback. Session storage migrated from `{UserConfigDir}/one-shot-man/sessions/` to `{UserConfigDir}/osm/sessions/`. Sync default from `~/.one-shot-man/sync` to `~/.osm/sync`. All docs updated.**
- Add "which one is better" builtin goal to internal/command/goal_builtin.go **DONE (T127)**
    - Implement exhaustive options analysis, tailored for a wide range of use cases **DONE (T127)** — 5 comparison types (general/technology/architecture/strategy/design), weighted scoring matrices, 6-phase analysis methodology
    - Needs specific use case variants, much like many of the existing goals (morale-improver, bug-buster, code-optimizer, etc.) **DONE (T127)**
    - Leverage the same patterns already established in goal_builtin.go (stateVars, hotSnippets, flagDefs, promptOptions) **DONE (T127)** — comparisonType stateVar, comparisonTypeInstructions promptOptions, set-type command, deeper-analysis and devils-advocate hot-snippets
    - Target maximizing utility and general usefulness across different decision-making scenarios **DONE (T127)**
    - MUST be integration tested properly **DONE (T127)** — 8 tests covering metadata, stateVars, promptOptions, commands, hotSnippets, postCopyHint, uniqueness, JSON roundtrip, list output

---

## Bugs / Observations

### 2026-02-14 Path Ambiguity in Txtar Context Building **DONE (T064)**

When building txtar context from multiple files, the current implementation in `internal/scripting/context.go:ToTxtar()` and the `computeUniqueSuffixes` helper (`context.go:499-576`) can produce misleading paths that obscure the actual filesystem relationships between files.

**Problem 1: Implicit common roots not visible**

Given two relative paths like `a/b/file.go` and `c/d/file.go`, the current logic produces disambiguated names like `a/b/file.go` and `c/d/file.go`. However, there's no explicit indication whether these files share a common root directory (e.g., if both were under the same parent `proj/`). An LLM or human can't easily tell from the paths alone if these files are siblings in the same directory tree or completely unrelated.

**Problem 2: Collision resolution creates false directory impressions**

When files share basenames (e.g., `handlers.go` in multiple directories), the code expands paths upward until unique. The result (e.g., `a/handlers.go` vs `b/handlers.go`) creates the visual impression that both files are in directories `a/` and `b/` respectively—but if `a/` doesn't actually exist as a containing directory (e.g., only `a/handlers.go` exists but `a/` itself was never added as a tracked path), this is misleading.

Conversely: when two files with different basenames end up with relative paths that make them look like they're in the same directory (e.g., `dir/file1.go` and `dir/file2.go`), but due to other files or directories in the context, the actual relationship is ambiguous or the files aren't actually under a common `dir/`.

**Suggested fix direction:**

The `ToTxtar` function should consider:
1. Computing and emitting the lowest common ancestor (LCA) of all tracked paths, and either prefixing paths with it or explicitly documenting it in a comment within the txtar
2. When expanding paths upward for disambiguation, verify that the implied parent directory is actually tracked (exists in the context) - if not, either skip using that level or indicate visually (e.g., `~a/handlers.go` to mean "under a/ which is not itself in context")
3. For non-colliding basenames that happen to end up in what looks like the same directory, consider whether the full path should be preserved to avoid false impressions of proximity

This affects both human readability and LLM understanding of the context structure.

---

### osm:ctxutil txtar diff context root formatting

When using `osm:ctxutil` to generate a txtar diff, the context root should be placed **outside** the txtar code block, with the path wrapped in backticks around it (e.g., `` `path/to/root` ``). The current implementation embeds the context root inside the code block without backticks around the path.

---

## AI Orchestrator / Claude Code Integration (2026-02-17)

### Vision Statement

osm should be able to orchestrate Claude Code (and potentially other TUI-based AI assistants) as a subprocess, using it as a tool for performing complex, multi-step operations. The primary use case is **interactive PR splitting**: breaking down very large AI-generated change sets into reviewable, verifiable, mergeable chunks.

**Critical Constraint**: This is intended to support focused workflows leveraging existing tools. The most critical constraint is: **no interest in building yet another agentic harness that no one will use**.

### Original Problem Statement (User's Description)

> "With AI it is very easy to have this very, very large change set which no human has reviewed. This is annoying to unfuck to validate to verify. But if you don't verify, it turns into slop. Slop is bad.
>
> Ideally each stage of the PR would be something which works. And if it was something which was being wholly AI generated. It would. It would be viable, most likely, to interleave a sort of interactive rebasing, as it were. Where the branch gets essentially rebased into individual components or commits. And the process of making sure that they're sane could become something which is done in tandem with the AI. The AI doing the leg work.
>
> Once once changes are split out into a coherent, reviewable, ideally mergeable separately chunks. Then it is a lot easier to process. Even with LLMs it comes. Much, much easier. Because this contact size is smaller and also more reliable to even validate with LLMs."

### Key Design Principles

1. **No Direct API Integration**: Avoid adding API keys or network calls to osm. Leverage existing TUI tools (Claude Code, gh CLI, gemini CLI, codium CLI, etc.)
2. **Minimal Configuration Burden**: Don't require users to set up complex workflow systems. Use existing tools that "just work"
   - User's stance: "I really do not want to make the configuration burden for this kind of tool any worse"
   - Not attractive for "average biggest developer to go all in and setting up a workflow system"
3. **TUI Multiplexing**: osm acts as a multiplexer that can swap between its own TUI and the external tool's TUI, potentially via meta-key switching (tmux-style)
   - User is a tmux user: "I use tmux, so make sure to note that and avoid conflicts"
   - Must avoid conflicting with tmux keybindings
4. **Modular Implementation**: Subprocess orchestration is ONE of the means. All implementations must be modular and flexible. The system should support multiple interaction models (spawn + PTY, external process monitoring, TTY transfer), with users choosing the approach that fits their workflow.
5. **Configuration Strategy Alignment**: Per the todo items - align with Claude's high-level configuration strategy for osm
6. **Building Blocks Exposed**: Various building blocks must be exposed to users of osm for their own implementations. There's various examples in scripts/, including for PA-BT.
7. **Behavior Tree Orchestration**: Use behavior trees (PABT) for high-level workflow control with prebuilt action templates
8. **Hybrid Communication**: MCP for data exfiltration (Claude → osm), PTY parsing for setup/init/permission handling
   - **PTY output capture is mandatory**: Required for rate limit handling, error detection, context clearing, and output verification
   - User's explicit requirement: "Always verify any inputs (as in, sent over the PTY) result in the expected outputs"
9. **Slop Prevention**: "Slop is bad" — unverified AI-generated changes accumulate into unmaintainable codebases

### Integration Architecture: Hybrid Approach

#### Component 1: PTY Spawning and Terminal Management

- **PTY Wrapper Module**: Extend `osm:exec` (or create `osm:pty`) to support spawning processes with full PTY
- **Terminal State Preservation**: Save/restore terminal state before/after external app
- **Signal Forwarding**: Forward SIGWINCH, SIGINT, SIGTERM to external process
- **Input/Output Bridging**: Bridge terminal input to external process and output back to osm

**Reference**: `internal/scripting/pty_test.go` shows PTY capability using `github.com/creack/pty`

#### Component 2: MCP-Based Data Exfiltration

- **Bidirectional MCP**: osm already has an MCP server (`internal/command/mcp.go`) with 6 tools
- **Claude as MCP Client**: Claude Code runs with MCP client enabled, connecting back to osm
- **Session Identification**: Each Claude instance must clearly identify which osm session it's communicating with
- **Tool Exfiltration**: Claude uses MCP tools to send structured data back to osm (e.g., split PR descriptions, test results)

**Key Insight**: The existing MCP server (`addFile`, `addDiff`, `addNote`, `listContext`, `buildPrompt`, `getGoals`) provides a solid foundation for bidirectional communication.

#### Component 3: PTY Parsing for Setup/Init

Numerous conditions require PTY parsing:
- **Rate limit prompts**: Detect and wait/handle rate limit messages
- **Permission prompts**: Conservative rejection with reasons (be secure)
- **Model selection menus**: Handle `ollama launch claude --config` TUI wrapper to select model before launching Claude
- **SSO login flows**: Handle authentication prompts for tools like AWS

**Security Principle**: Default to rejecting permission prompts with clear reasons. Be secure by default.

### Configuration and Environment Variables

Three-tier approach:

1. **Shared with osm**: Claude Code inherits osm's environment (API keys, MCP config, etc.)
2. **Script Hooks**: JavaScript scripts run hooks to inject env vars at spawn time
   - Example: `op plugin` to inject secrets from 1Password CLI
   - Example: AWS SSO login via `aws-vault` or similar
3. **Profile-Based Configuration**: Separate env var profiles configurable in osm config
   - Command-line flags/options override
   - Special handling for provider-specific commands (e.g., `ollama launch claude --config`)

### Provider Abstraction

**First Pass**: Single instance support only

**Future**: Modular, extensible design for:
- Multiple concurrent Claude Code instances (parallel PR processing)
- Different providers (GitHub CLI, Gemini CLI, Codium CLI, etc.)
- Provider-specific configuration (e.g., ollama model selection TUI navigation)

### Orchestration: Behavior Trees (Internal Implementation Detail)

> User's original insight: "And I guess I think that using PABT would be ideal. However, that requires the LLM to have an understanding of how to build PABT and how to generate the plan essentially on failure. That honestly sounds like a good idea. Because what that would mean is that there are clear points at which the state can be validated. And which action can be taken? Honestly, that sounds like an amazing idea."

Behavior trees are an **internal implementation detail** for workflow orchestration. Key characteristics:

- **High-level orchestration**: Behavior trees manage the workflow structure
- **Action templates**: Prebuilt modes for common operations
- **Claude-centric execution**: Most steps involve executing operations through Claude Code
- **Prompt-based control**: Provide prompts to Claude, which uses MCP to communicate back

**Planning on Failure**: When a step fails, the LLM should be able to generate a new plan. This requires:
- Clear validation points (state checks)
- Actionable remediation steps
- **Skills Integration**: Skills system (e.g., "board" skill) provides domain knowledge for remediation
  - Skills contain knowledge of how to perform specific tasks
  - Skills are invoked when steps fail to guide corrective action
  - Context from the running osm session is available to skills for task-specific actions

### PR Splitting: Specific Requirements

#### Branch Structure

The splitting process creates a **linear series of branches**:
- Each branch has a root (base branch) it merges into
- The bottom-most branch is based on the trunk (original branch)
- Branches form a hierarchy from "inner" to "outer" (list outer → most inner)
- Each staged part becomes its own mergeable unit

#### Interactive Rebasing Workflow

The workflow interleaves AI assistance with interactive rebasing:
- Branch gets rebased into individual components or commits
- Process of ensuring sanity is done in tandem with AI ("AI doing the leg work")
- Specific example: "making sure that it is a same slice" (test validation)
- User controls safety, verification, and explicit checks

#### Equivalence Verification

Critical verification requirement:
- **Final branch must be representative of the original branch that was split out**
- The split-out changes must preserve the semantics of the original
- User explicitly validates that nothing is lost or changed unexpectedly
- This verification step is non-negotiable — "slop is bad"

#### Context Size Benefits

Why splitting matters for LLMs:
- Smaller context size is more reliable to validate with LLMs
- Each chunk can be independently verified and tested
- Reviewable chunks are easier to process (for humans and LLMs)
- Prevents "slop" from accumulating in large unreviewed changesets

### Specific Tool References

#### Provider-Specific Commands

**Ollama Integration**:
- Command: `ollama launch claude --config`
- Must navigate through ollama's TUI menu
- Select model (e.g., `gpt-oss:20b-cloud`)
- Get to the common "entrypoint" (Claude Code TUI)
- Model selection happens before Claude Code launches

**AWS Integration**:
- Tool: `aws-vault` — specifically mentioned by user ("I forget what it's called. aws vault?")
- Runs command that provides environment variables just for that session
- Doesn't leak credentials into system environment
- Used when Claude Code needs AWS permissions

**1Password Integration**:
- Tool: `op plugin` (1Password CLI)
- Connects to 1Password app on macOS
- Injects secrets at spawn time
- Avoids storing API keys in plain text

### Rejected Alternatives

**Explicitly Out of Scope**:

1. **MacOS Automation**: Requires system daemon with gRPC API access — explicitly rejected
2. **Direct API Integration**: User stated "I'd actually really like not to integrate any APIs" — tool calling harness "gets really tricky really quickly"
3. **Complex Workflow Systems**: User stated "I really do not want to make the configuration burden for this kind of tool any worse" — not attractive for "average biggest developer to go all in and setting up a workflow system"
4. **Agentic Flows Without Data Return**: User noted "the problem is of course with agentic flows is that it is really hard to get data back" — this must be solved via MCP

**Why Claude Code?**

User's assessment: "Claude code just works. Claude code is sound, it isn't the best, it does a lot of things and it doesn't do the best at a lot of things. But it does work effectively. And it is a tool which could be integrated."

This pragmatic endorsement drives the choice — it works, it's available, and it avoids the configuration burden of alternatives.

### Testing Requirements: Special-Case TestMain

#### Testing Mechanism

The integration mechanism being tested:
1. **Spawn Claude command**: osm spawns Claude Code (or provider-specific wrapper)
2. **Prompt with task instructions**: Provide prompt that instructs on the task
3. **Response via MCP**: Claude uses MCP to communicate back to wrapping osm process

This specific flow must be testable in isolation.

#### TestMain Implementation

- **TestMain with CLI flags**: Configure provider(s) to test against
- **Environment variables for secrets**: Use env vars for API keys (never hardcode)
- **Disabled by default**: Tests must be explicitly enabled (e.g., `-test-integration` flag)
- **Provider-specific configuration**: Each provider may have specific config requirements

**User's Original Testing Requirements**:
> "Write up a full on task like: For testing purposes of this ([specific mechanism to end up with a running claude command] -> Prompt that instructs on the task and response mechanism via MCP back to the wrapping osm process) it will be necessary to implement a special-case `TestMain` with CLI flags and environment variables (the latter for secrets, CLI flags to specify options) to configure provider(s) to test against (disabled by default). Individual providers may have specific configuration. For testing purposes, specify the requirement to use that `ollama launch claude --config` command, and to navigate through the menu to select `gpt-oss:20b-cloud`, and whatever else is necessary to get it to the common 'entrypoint'."

**Example: Ollama Testing**
```bash
# Test with ollama provider
go test -v -run TestClaudeIntegration -provider=ollama -model=gpt-oss:20b-cloud

# Special handling required:
# - Navigate through ollama's TUI menu
# - Select the model
# - Get to the common "entrypoint" (Claude Code TUI)
```

### Codebase Analysis Summary

**Existing Building Blocks**:

1. **MCP Server** (`internal/command/mcp.go`): 6 tools for context management, stdio transport
2. **TUI Manager** (`internal/scripting/tui_manager.go`): Mode switching, command registration, go-prompt integration
3. **PTY Support** (`internal/scripting/pty_test.go`): Proof-of-concept PTY usage
4. **Goal System** (`internal/command/goal_builtin.go`): Declarative workflow definitions with state variables
5. **Scripting Runtime** (`internal/scripting/runtime.go`): Goja with event loop, thread-safe execution
6. **Process Execution** (`internal/builtin/exec/exec.go`): Current implementation doesn't use PTY (captures stdout/stderr)

**Key Patterns**:

- **Event Loop Model**: All JavaScript execution on single event loop goroutine
- **Provider Interfaces**: Components implement interfaces (TerminalOpsProvider, EventLoopProvider)
- **Shared State**: ContextManager provides unified file/diff/notes state
- **Thread Safety**: Atomic operations, mutexes, goroutine ID tracking prevent races
- **Lifecycle Management**: Context-based cleanup with proper shutdown sequences

### Use Cases Beyond PR Splitting

- **Multi-step code generation**: Generate, test, validate, refine
- **Documentation generation**: Analyze code, generate docs, validate examples
- **Refactoring orchestration**: Identify refactoring opportunities, apply changes, verify tests
- **Test generation**: Generate tests, run them, fix failures iteratively

### Open Questions for Architectural Design

1. **Meta-key switching**: Should osm support tmux-style meta-key switching between osm TUI and Claude Code TUI?
2. **Session isolation**: How should osm isolate multiple Claude Code sessions (future multi-instance)?
3. **Error recovery**: How should osm handle Claude Code crashes, hangs, or unexpected exits?
4. **Progress reporting**: How should osm report progress from long-running Claude operations?
5. **Cancellation**: How should users cancel in-progress Claude operations from osm?

### Related Items in TODO

- "Code review splitter - prompts seem particularly LLM dependent, stalled" — **This is the precursor** to the AI orchestrator vision
- "Add `exec` context builder command" — May need extension for PTY-based execution
- "Implement partially-compliant fetch API" — May be relevant for certain API operations (though we want to avoid direct API integration)

---

### Next Steps: Architectural Design Phase

This ideation document preserves the vision, requirements, and constraints. The next phase is **architectural design**, where:

1. Multiple implementation approaches will be explored (minimal changes, clean architecture, pragmatic balance)
2. Trade-offs will be compared
3. A concrete implementation plan will be created
4. User approval will be sought before implementation begins

**Preserved Context for Planner**:
- User's original detailed description (above)
- Clarifying question answers (hybrid integration, configurable automation, multi-tier config, modular design)
- Codebase patterns (MCP, TUI, PTY, goal system, scripting runtime)
- Testing requirements (TestMain, provider-specific config, ollama example)

---

## Claude Code Multiplexer with Ollama Safety Validation

> **Note**: This section is demonstrative and non-authoritative. It represents initial brainstorming rather than a finalized design. The specifics (configuration keys, file paths, API surface, architecture) require proper validation against actual requirements and constraints before implementation. Avoid treating this as specification.

### Purpose

Create a script that acts as a multiplexer between Claude Code prompts and the actual Claude Code execution, using a local Ollama-powered mechanism to:
1. Identify and categorize Claude Code prompts
2. Validate the safety/suitability of prompts before execution
3. Resolve the "ideal choice" when multiple options exist
4. Wrap Claude Code itself for controlled execution

This is distinct from the broader AI Orchestrator (above) — this is a focused, lightweight multiplexer that runs locally with Ollama, without requiring external APIs or complex workflow orchestration.

### Core Behavior: Prompt Multiplexing

The script intercepts/detects Claude Code prompts and routes them appropriately:

1. **Prompt Identification**: Detect when Claude Code is being invoked and parse the incoming prompt/session
2. **Categorization**: Classify prompts into categories (e.g., code review, refactoring, documentation, testing, general Q&A)
3. **Routing**: Route prompts based on category to appropriate handling strategies
4. **Aggregation**: When multiple prompts are detected, aggregate and prioritize them

### Ollama-Powered Validation

**Critical Design Constraint**: All validation happens locally via Ollama — no external API calls (only the local Ollama HTTP API), no network dependency beyond downloading Ollama models initially.

**Integration Method**: Use Ollama's HTTP API (`http://localhost:11434` by default) for chat completions. Tool calling support is optional — simpler prompting with structured output parsing may suffice for validation tasks; if needed, consider:
1. **Function calling** (if supported by the model): Leverage Ollama's built-in tool/function calling
2. **JSON mode**: Use `json` format with structured prompts for parsing responses
3. **Split prompts**: Separate classification, scoring, and recommendation into sequential calls

#### Safety Validation Categories

1. **Intent Classification**: Determine if the prompt is:
   - A legitimate development task (code change, review, refactor, test)
   - A potentially destructive operation (force push, delete, drop data)
   - A security-sensitive operation (credential handling, secrets, permissions)
   - An external network request (API calls, fetching URLs)

2. **Scope Assessment**: Evaluate the scope of changes:
   - File-level changes (single file modifications)
   - Module-level changes (multiple related files)
   - Repository-level changes (widespread changes across codebase)
   - Infrastructure changes (config files, dependencies, CI/CD)

3. **Risk Scoring**: Assign risk scores based on:
   - Destructive potential (force operations, deletions)
   - Scope breadth (how much code is affected)
   - Reversibility (can changes be easily undone?)
   - External dependencies (network calls, API keys, third-party services)

#### Ideal Choice Resolution

When multiple approaches exist for a given prompt, use Ollama to determine the ideal choice:

1. **Multi-candidate analysis**: Present multiple solution approaches to Ollama
2. **Criteria weighting**: Apply weighted criteria (correctness, efficiency, maintainability, safety)
3. **Recommendation generation**: Output a recommended approach with justification
4. **User confirmation**: Require explicit user confirmation before execution

### Claude Code Wrapper

The script must wrap Claude Code execution with controlled invocation:

1. **Spawn mechanism**: Use PTY spawning (via `osm:pty` or extended `osm:exec`) to run Claude Code
2. **Environment isolation**: Control environment variables passed to Claude Code
3. **Session management**: Manage Claude Code sessions with proper cleanup
4. **Input/output bridging**: Bridge stdin/stdout/stderr between osm and Claude Code
5. **Signal handling**: Forward signals (SIGINT, SIGTERM) appropriately

### Native Go Support Requirements

The script requires native Go code support for:

1. **PTY Module Extension** (`osm:pty` or extend `osm:exec`):
   - Full PTY support with terminal state preservation
   - Signal forwarding (SIGWINCH, SIGINT, SIGTERM)
   - Input/output streaming with proper buffering

2. **Ollama Integration** (`osm:ollama` or similar):
   - Local HTTP client to Ollama API (typically `http://localhost:11434`)
   - Model listing and selection
   - Chat completion API integration
   - Streaming response support (for interactive use)
   - Connection health checking

3. **Prompt Parsing** (`osm:claude` or similar):
   - Claude Code output parsing (detect prompts, errors, rate limits)
   - Session state tracking
   - Tool call interception (for MCP integration)

### Script Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Claude Code Multiplexer                     │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐  │
│  │   Prompt    │    │   Ollama     │    │   Claude Code    │  │
│  │ Detector/   │───▶│   Safety     │───▶│   Wrapper        │  │
│  │ Parser      │    │   Validator  │    │   (PTY Spawn)    │  │
│  └──────────────┘    └──────────────┘    └──────────────────┘  │
│         │                   │                     │             │
│         ▼                   ▼                     ▼             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────────┐  │
│  │  Category   │    │   Ideal      │    │   Output/        │  │
│  │  Classifier │    │   Choice     │    │   Response       │  │
│  └──────────────┘    │   Resolver   │    │   Handler        │  │
│                      └──────────────┘    └──────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Configuration

> **Note**: The configuration keys below are illustrative only. Actual keys should align with osm's existing config patterns and schema validation.

```
# Claude Code Multiplexer configuration

# Ollama endpoint (default: http://localhost:11434)
claude-mux.ollama-endpoint=http://localhost:11434

# Default model to use for validation
claude-mux.ollama-model=llama3.2:latest

# Auto-approve safe prompts without Ollama validation
claude-mux.auto-approve=false

# Require explicit confirmation for high-risk operations
claude-mux.confirm-destructive=true

# Enable detailed logging of Ollama interactions
claude-mux.debug=false
```

### Implementation Files

> **Note**: File paths and module organization are illustrative. Actual structure should follow existing osm patterns (e.g., check `internal/scripting/` for existing module patterns).

- **Native Go modules**:
  - `internal/scripting/ollama.go` — Ollama API client
  - `internal/scripting/claude.go` — Claude Code wrapper/parser
  - `internal/scripting/pty.go` — PTY support extension

- **Script implementation**:
  - `scripts/claude-mux.js` — Main multiplexer script
  - `scripts/claude-mux/validator.js` — Safety validation logic
  - `scripts/claude-mux/resolver.js` — Ideal choice resolution

### Safety Guarantees

1. **Fail-closed**: On any error (Ollama unavailable, parsing failure), default to blocking execution
2. **Explicit confirmation**: Never auto-execute destructive operations
3. **Audit logging**: Log all decisions (prompt categorization, risk scores, recommendations)
4. **Timeout handling**: Set reasonable timeouts for Ollama responses
5. **Model validation**: Verify Ollama model is available before relying on it

### Testing Requirements

1. **Ollama connectivity**: Test with various Ollama states (running, stopped, no model)
2. **Prompt classification**: Test classification accuracy across different prompt types
3. **Claude Code wrapper**: Test PTY spawning, signal handling, cleanup
4. **End-to-end**: Test full flow with real Claude Code invocation

### Related Items

- AI Orchestrator / Claude Code Integration (above) — This is a focused, lightweight version
- PTY Spawning and Terminal Management (above) — Required for Claude Code wrapper
- MCP-Based Data Exfiltration (above) — Complementary for bidirectional communication
