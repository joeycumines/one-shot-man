// Package prsplittest provides shared test helpers for PR Split tests.
//
// This package enables PR Split test files in internal/command/ to share
// common engine creation, JS evaluation, git repository setup, and TUI
// mock infrastructure without duplicating boilerplate in every test file.
//
// # Import Constraint
//
// This package MUST NOT import internal/command. Doing so would create a
// Go import cycle since test files in internal/command import this package.
// Engine creation uses [scripting.NewEngineDetailed] directly, and chunk
// loading reads JS files from disk rather than accessing the unexported
// prSplitChunks variable.
//
// # Chunk Discovery
//
// JS chunk files are discovered by globbing pr_split_*.js in the
// internal/command/ directory (located via [runtime.Caller]). Chunk names
// are extracted by stripping the "pr_split_" prefix and ".js" suffix.
// Files are loaded in lexicographic order, matching the production load
// order defined by the prSplitChunks array in pr_split.go.
//
// # Engine Variants
//
//   - [NewChunkEngine]: Drop-in replacement for loadChunkEngine. Loads only
//     specified chunks for isolated unit testing.
//   - [NewTUIEngine]: Loads chunks 00–12, injects TUI mocks, then loads
//     chunks 13–16f. For TUI-level tests.
//   - [NewTUIEngineWithHelpers]: Extends NewTUIEngine with chunk16Helpers
//     (state initializer, mock helpers, message helpers).
//
// # Build Tags
//
// Slow integration/E2E tests should use //go:build prsplit_slow to exclude
// them from the fast feedback loop (make test-prsplit-fast).
package prsplittest
