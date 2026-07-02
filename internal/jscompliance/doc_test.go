// Package jscompliance holds the production-grade JavaScript Runtime
// compliance test suite for the osm command.
//
// osm embeds a Goja-based JavaScript runtime (see internal/scripting) and
// exposes a host API: the osm:* native modules (internal/builtin), the
// global objects (output, log, context, ctx, tui, args), the CommonJS
// module system, and the event-loop/Promise/timer surface provided by the
// goja-eventloop adapter. This package verifies that every one of those
// surfaces behaves according to its contract — and that any deviation
// surfaces as a real test failure, not a silent skip.
//
// # Suite shape
//
// The suite is split into two tiers:
//
//   - FAST tier (always-on under `gmake all`): the data-driven module
//     contract table (TestModuleContract), ESM rejection, globals presence,
//     module resolution, security, the ES2020+ adapter global-surface
//     inventory, and the core ECMAScript semantics spec. No I/O, runs in a
//     couple of seconds, never mutates host state.
//   - SLOW tier (guarded by skipSlow / testing.Short): behavioral specs that
//     perform real I/O (file, process, HTTP via httptest, temp git repos),
//     exercise timers, and the curated test262 baseline.
//
// # How to add coverage
//
//   - New osm:* module / export: add a row to moduleContracts in
//     modules_test.go (the single contract authority — cross-checked against
//     the module's module.go exports, not the docs alone). TestModuleContract
//     then covers it automatically.
//   - New behavioral assertion: add specs/<area>.spec.js authored against
//     specs/harness.js (the assert runtime) and a thin Go driver that calls
//     runSpec.
//
// # Harness contract
//
// Every test builds the REAL production engine via newComplianceEngine,
// which mirrors internal/scripting.newTestEngine verbatim. All JavaScript
// runs on the engine's single event-loop goroutine; the Go side observes
// results by attaching Go-side handlers to Promises (never by blocking on a
// Promise expression). See harness_test.go.
//
// docs/scripting.md is the contract authority this suite pins; where code
// and docs disagree, the disagreement is recorded as a drift finding and
// resolved (fixed or documented with evidence).
package jscompliance
