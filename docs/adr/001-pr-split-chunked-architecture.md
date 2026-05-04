# ADR 001: PR-Split Chunked Architecture

**Status:** Accepted  
**Date:** 2026-01-15  
**Deciders:** joeycumines

## Context

The `osm pr-split` command implements an entire automated PR-splitting
pipeline — diff analysis, grouping strategies, plan persistence, branch
execution, verification, conflict resolution, Claude integration, and a
full-screen BubbleTea TUI — entirely in JavaScript executed via the goja
runtime. The codebase grew beyond 10,000 lines of JS and a single monolithic
file became untenable.

Key pressures:

1. **Go embed limits.** `//go:embed` inlines file contents at build time.
   A single 10K+ line string constant dominates compile-time memory and
   makes `go vet` / IDE indexing sluggish.
2. **Test isolation.** Unit-testing a single function (e.g.,
   `analyzeDiff`) required loading the entire script including the TUI,
   Claude executor, and pipeline orchestrator. Tests were slow and fragile.
3. **Merge conflicts.** Multiple contributors editing different subsystems
   (pipeline vs. TUI vs. grouping) collided on every change because they
   shared one file.
4. **Late-binding dependencies.** Some cross-references between subsystems
   (e.g., conflict resolution referencing prompt templates) created apparent
   circular dependencies that were hard to reason about in a flat file.

## Decision

Split the monolithic pr-split script into **numbered chunks** — self-contained
IIFE modules that are loaded sequentially into a single goja VM. Each chunk
attaches its exports to a shared `globalThis.prSplit` namespace.

### Chunk contract

- Each chunk is a separate `.js` file embedded via `//go:embed`.
- Chunks are loaded in numerical order by `loadChunkedScript()`.
- Chunk N may reference symbols from chunks 0..N-1 but not N+1+.
- Cross-references that appear circular are resolved via late binding:
  the reference is a function call, not a parse-time evaluation, so the
  target symbol exists by the time it is invoked.

### Naming convention

```
pr_split_{NN}[{suffix}]_{domain}.js
```

- `NN` — two-digit group number (00–16).
- `suffix` — optional letter (a–f) for sub-chunks within a group.
- `domain` — lowercase descriptor (`core`, `analysis`, `tui_update`, etc.).

### Testing granularity

A `prsplittest.NewChunkEngine` helper loads arbitrary subsets of chunks,
enabling unit tests that exercise a single chunk with minimal mocking —
without paying the cost of loading all 30 chunks and the TUI stack.

## Consequences

### Positive

- **Fast targeted tests.** A chunk-00 unit test runs in <100 ms.
- **Clear dependency graph.** Sequential numbering makes dependencies
  explicit and prevents accidental cycles.
- **Parallel development.** TUI changes (13–16) do not conflict with
  pipeline changes (10a–10d).
- **Smaller compile units.** Each `//go:embed` string is 200–800 lines
  instead of 10K+.

### Negative

- **Export manifest maintenance.** Chunk 12 must list every exported symbol.
  Missing entries cause a runtime warning. This is enforced by the
  `TestChunk12_ExportManifest` test.
- **Load-order sensitivity.** Adding a new chunk or reordering requires
  updating the `prSplitChunks` array in `pr_split.go`.
- **Late-binding subtlety.** Developers must understand that "chunk 08
  references chunk 09" is valid because the call site is in chunk 10d which
  loads after both. This is documented in [architecture-pr-split-chunks.md](../architecture-pr-split-chunks.md).

### Neutral

- **No module system.** Goja does not support ES modules or CommonJS
  (`require()` is provided by the osm scripting engine, not by goja itself).
  The IIFE-per-chunk pattern is the closest idiomatic equivalent.

## Related

- [PR-Split Chunk Architecture](../architecture-pr-split-chunks.md)
- [PR-Split Integration Testing](../pr-split-testing.md)
