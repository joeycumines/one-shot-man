# WIP.md

**Current State:**
- The overall goal is to refine `blueprint.json` to introduce a guided PR split wizard interface for `osm pr-split` that makes error resolution incredibly easy and interactive.
- Adjusted W01-W07 to guarantee the entire wizard logic—including `PlanEditor`—is implemented purely in JavaScript.
- Updated `W00: Termmux JavaScript Facade (Builtin)` based on explicit audio feedback. Clarified that `internal/termmux` will be fully stripped of Goja bindings and an independent `internal/builtin/termmux` facade must exist. Also added an explicit note to evaluate signal handling and `try-lock` behavior during this extraction, especially detaching behavior.
- Added `W08: Review and Verify API Tidy & Breaking Changes` to the end of the W-series. This task acts as a final sweep after all Wizard changes to assess internal APIs for rough edges, encouraging breaking changes that improve code quality, and specifically testing OS platform integrations (macOS/Windows) regarding signals.

**Next Steps for Takumi:**
- The Next Takumi should read `blueprint.json` sequentially. The next open tasks will be either the unfinished R tasks (`R00a`, `R00`, `R28`, etc.) or immediately proceeding to `W00` based on priority.
- Continue burning through the `sequentialTasks`, checking conditions via GNU Make `make all` & `make integration-test-prsplit-mcp`.

**Tooling Log Context:**
- Edited `/Users/joeyc/dev/one-shot-man/blueprint.json` to clarify W00 and append W08.
