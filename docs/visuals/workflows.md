# Workflows

This document describes the core workflows provided by osm with visual diagrams and recordings.

## Recorded Demos

Interactive GIF recordings of each workflow are available in [`docs/visuals/gifs/`](gifs/):

| Workflow                 | Recording                                                                         | Description                              |
|--------------------------|-----------------------------------------------------------------------------------|------------------------------------------|
| Quickstart               | ![quickstart.gif](https://vhs.charm.sh/vhs-231LLSBeReAzfRuYvDcWhh.gif)            | Quick overview of osm commands           |
| Super-Document (Visual)  | ![super-document-visual.gif](https://vhs.charm.sh/vhs-7FBO5VD1uWeeW3hN3xFWdd.gif) | Visual TUI document builder              |
| Super-Document (Shell)   | ![super-document-shell.gif](https://vhs.charm.sh/vhs-66rfa16MOgVKY2Mu7Yoyxn.gif)  | Shell/REPL document builder              |
| Super-Document (Interop) | ![super-document-interop.gif](https://vhs.charm.sh/vhs-TQnbpQp3N1tC6hKXQCYyC.gif) | Switching between visual and shell modes |
| Code Review              | ![code-review.gif](https://vhs.charm.sh/vhs-mcoAMgE9oEdDMwdzDdYuZ.gif)            | Code review prompt builder               |
| Prompt Flow              | ![prompt-flow.gif](https://vhs.charm.sh/vhs-2OgtCV13DDgTkOy2WPrWui.gif)           | Two-step prompt builder                  |
| Goal                     | ![goal.gif](https://vhs.charm.sh/vhs-68PSt8wdj7OCCsXmMoY7hL.gif)                  | Goal-based workflows                     |

> **Regenerating Demos:** Run `make generate-tapes-and-gifs` (requires [VHS](https://github.com/charmbracelet/vhs)).

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
    User ->> OSM: start code-review
    loop Iterate context
        User ->> OSM: add files / diff / note
        OSM ->> Ctx: store items (paths, diff specs, notes)
        OSM ->> Git: (optional) run git diff
        Git -->> OSM: diff text
        OSM ->> Ctx: store diff output/spec
    end
    User ->> OSM: show/copy
    OSM ->> Ctx: assemble final prompt
    OSM ->> Clip: copy prompt
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
    User ->> OSM: start prompt-flow
    User ->> OSM: set goal
    loop Build context
        User ->> OSM: add files / diff / note
        OSM ->> Ctx: store context
    end
    User ->> OSM: generate
    OSM ->> Ctx: render meta-prompt template
    User ->> OSM: copy meta
    OSM ->> Clip: copy meta-prompt
    Note over User: Ask your LLM using the meta-prompt
    User ->> OSM: use (paste response as task prompt)
    User ->> OSM: copy
    OSM ->> Ctx: assemble final prompt (task + context)
    OSM ->> Clip: copy final prompt
```
