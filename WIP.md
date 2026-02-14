# WIP - Takumi's Desperate Diary

## Current State
- **T001**: ✅ DONE (6 commits: 370ceff, 3e382f1, 5ec578e, 5cc8cb4, 7455539, 84ec222)
- **T002**: ✅ DONE (commit d13c49c)
- **T003**: ✅ DONE (commit aaf6173)
- **T004**: ✅ DONE (commit 9212fd1) — Rule of Two PASSED
- **T005**: ✅ DONE (commit 6f18a1f) — Rule of Two PASSED
- **T006**: ✅ DONE (commit 6b1b0b9) — Rule of Two PASSED
- **T007**: ✅ DONE (commit 31a4c84) — Rule of Two PASSED
- **T008**: ✅ DONE (commit 293924c) — Rule of Two PASSED
- **T009**: ✅ DONE (commit b2c93d5) — Rule of Two PASSED
- **T010**: ✅ DONE (commit 7f19318) — Rule of Two PASSED
- **T011**: ✅ DONE (commit d50e796) — Rule of Two PASSED
- **T012**: ✅ DONE (commit d825887) — Rule of Two PASSED
- **T013**: 🔄 NEXT — Integrate hot-snippet configuration from config file

## T013 Context
- Currently hot-snippets are passed as JS options to contextManager
- Add Go-side [hot-snippets] config section support
- Parse in internal/config/ and pass to contextManager via goal/command infra
- Users define snippets in config without writing JS
- JS-defined snippets merge with config-defined (JS wins on conflicts)
- Add tests for config parsing and merge behavior
- Key files: internal/config/config.go, internal/builtin/ctxutil/contextManager.js, goal.go
