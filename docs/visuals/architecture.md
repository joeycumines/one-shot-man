# Architecture diagram (placeholder)

**Caption:** High-level data/control flow for `osm`.

**Recommended render:** `docs/visuals/assets/architecture.png`

**Alt text:** Diagram showing `osm` CLI commands calling Go command implementations, some of which invoke an embedded JavaScript engine; sessions and local storage persist state; clipboard/editor integration sit alongside.

```mermaid
flowchart LR
  subgraph CLI[osm CLI]
    main[cmd/osm/main.go]
    registry[command registry]
  end

  subgraph Commands[internal/command]
    cmdScript[script]
    cmdGoal[goal]
    cmdPF[prompt-flow]
    cmdCR[code-review]
    cmdSession[session]
    cmdCompletion[completion]
  end

  subgraph Engine[internal/scripting]
    goja[Goja JS runtime]
    globals[globals: ctx/context/output/log/tui]
    modules["require('osm:*') modules"]
  end

  subgraph Storage[internal/storage]
    fs[(fs backend)]
    mem[(memory backend)]
  end

  subgraph Host[host integrations]
    editor[$EDITOR/$VISUAL]
    clipboard[clipboard copy]
    git[git via exec]
  end

  main --> registry
  registry --> Commands

  cmdScript --> Engine
  cmdGoal --> Engine
  cmdPF --> Engine
  cmdCR --> Engine

  Engine --> Storage
  cmdSession --> Storage

  modules --> Host
  cmdCompletion --> registry
  cmdCompletion --> cmdGoal
```
