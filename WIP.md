# WIP.md

**Current State:**
- The overall goal is to refine `blueprint.json` to introduce a guided PR split wizard interface for `osm pr-split` that makes error resolution incredibly easy and interactive.
- Adjusted W01-W07 to guarantee the entire wizard logic—including `PlanEditor`—is implemented purely in JavaScript.
- Added `W00: Termmux JavaScript Facade (Builtin)` based on explicit feedback. `internal/termmux` will remain independent without Goja bindings, while a new package (e.g. `internal/builtin/termmux`) will mediate interaction between JS components (like `osm:bt`) and the core termmux module.

**Next Steps for Takumi:**
- The Next Takumi should read `blueprint.json` sequentially. The next open tasks will be either the unfinished R tasks (`R00a`, `R00`, `R28`, etc.) or immediately proceeding to `W00` based on priority.
- Continue burning through the `sequentialTasks`, checking conditions via GNU Make `make all` & `make integration-test-prsplit-mcp`.

**Tooling Log Context:**
- Edited `/Users/joeyc/dev/one-shot-man/blueprint.json` to append W00 and ensure W01-W07 mandate pure JS.
