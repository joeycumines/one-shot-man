# Workflows (placeholders)

## Code review workflow

**Caption:** Build a single, context-rich prompt for a code review.

**Recommended render:** `docs/visuals/assets/workflow-code-review.png`

**Alt text:** Sequence diagram showing user iteratively adding files/diffs/notes in a TUI, then generating a prompt and copying it to clipboard.

```mermaid
sequenceDiagram
  participant User
  participant OSM as osm code-review (TUI)
  participant Ctx as context store
  participant Git as git
  participant Clip as clipboard

  User->>OSM: start code-review
  loop Iterate context
    User->>OSM: add files / diff / note
    OSM->>Ctx: store items (paths, diff specs, notes)
    OSM->>Git: (optional) run git diff
    Git-->>OSM: diff text
    OSM->>Ctx: store diff output/spec
  end
  User->>OSM: show/copy
  OSM->>Ctx: assemble final prompt
  OSM->>Clip: copy prompt
```

## Prompt-flow workflow

**Caption:** Two-phase prompting: meta-prompt first, then final task prompt.

**Recommended render:** `docs/visuals/assets/workflow-prompt-flow.png`

**Alt text:** Sequence diagram showing goal and context feeding meta-prompt generation, then user pasting an LLM response as the task prompt and assembling final output.

```mermaid
sequenceDiagram
  participant User
  participant OSM as osm prompt-flow (TUI)
  participant Ctx as context store
  participant Clip as clipboard

  User->>OSM: start prompt-flow
  User->>OSM: set goal
  loop Build context
    User->>OSM: add files / diff / note
    OSM->>Ctx: store context
  end
  User->>OSM: generate
  OSM->>Ctx: render meta-prompt template
  User->>OSM: copy meta
  OSM->>Clip: copy meta-prompt

  Note over User: Ask your LLM using the meta-prompt

  User->>OSM: use (paste response as task prompt)
  User->>OSM: copy
  OSM->>Ctx: assemble final prompt (task + context)
  OSM->>Clip: copy final prompt
```
