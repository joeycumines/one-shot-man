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
// loading reads JS files from disk rather than accessing the embedded
// manifest or chunkFS variable.
//
// # Chunk Discovery
//
// JS chunk files are discovered by reading pr_split_manifest.json in the
// internal/command/ directory (located via [runtime.Caller]). The manifest
// defines chunk IDs, file names, and load order. Chunk sources are read
// from disk using the file names from the manifest.
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
// # Test Gating
//
// Slow integration/E2E tests call skipSlow(t) at the top of the function
// body so that `go test -short` provides a fast feedback loop while
// `go test` (without -short) runs the full suite.
package prsplittest
