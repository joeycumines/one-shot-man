# T232: Streaming Fetch API Evaluation

**Status:** Decision — No further implementation needed  
**Date:** 2026-02-17  
**Affects:** T233 (skip)

## Context

T232 was created to evaluate whether `osm:fetch` needs a streaming/chunked API. At the time, the blueprint assumed `osm:fetch` only provided `fetch()` (which buffers the entire response body). This evaluation was prerequisite to T233, which proposed implementing `fetch(url, {onChunk: fn})`.

## Current State

The `osm:fetch` module (`internal/builtin/fetch/fetch.go`) **already provides streaming**:

| Function | Behavior | Use Case |
|----------|----------|----------|
| `fetch(url, opts?)` | Reads entire body into memory, returns `Response` with `.text()` and `.json()` | JSON APIs, small responses |
| `fetchStream(url, opts?)` | Returns `StreamResponse` with incremental reading | SSE streams, NDJSON, LLM APIs, large responses |

`fetchStream` API surface:
- `.readLine()` → `string | null` — reads next `\n`-delimited line, returns `null` at EOF
- `.readAll()` → `string` — reads remaining body (useful after partial `readLine`)
- `.close()` → `void` — releases HTTP connection

Both functions share the same options: `method`, `headers`, `body`, `timeout`.

## Alternatives Evaluated

### 1. Callback-based streaming (`onChunk`)

```js
// Hypothetical callback approach
fetch(url, {
    onChunk: function(chunk) { /* process chunk */ },
    onDone: function() { /* complete */ }
});
```

**Pros:**
- Familiar pattern from Node.js streams
- Could integrate with Goja event loop for async-like behavior

**Cons:**
- Goja is single-threaded; callbacks would need goroutine + channel coordination
- Adds significant complexity (event loop integration, error propagation, backpressure)
- The synchronous `readLine()` loop already covers 100% of practical use cases
- Callback pattern is less readable than a synchronous while-loop in ES5

**Verdict:** Rejected. The existing `readLine()` approach is simpler, debuggable, and sufficient.

### 2. ReadableStream / async iteration

```js
// Hypothetical Web Streams approach
var reader = resp.body.getReader();
while (true) {
    var result = reader.read(); // { value, done }
    if (result.done) break;
}
```

**Pros:**
- Standards-compliant (WHATWG Streams)
- Familiar to web developers

**Cons:**
- Goja does not support async/await or Promises natively
- Would require a custom Promise polyfill + event loop, adding massive complexity
- Overkill for a CLI tool's scripting layer

**Verdict:** Rejected. Wrong abstraction level for a synchronous ES5 runtime.

### 3. `readChunk(n)` (byte-level reading)

```js
var chunk = resp.readChunk(4096); // read up to 4096 bytes
```

**Pros:**
- Maximum control for binary protocols or non-line-based formats
- Useful for multipart responses or binary file downloads

**Cons:**
- No current use case in the project (all streaming targets are line-based: SSE, NDJSON, LLM APIs)
- Increases API surface without demonstrated need
- Can be added later if a use case emerges

**Verdict:** Deferred. No current need. If binary streaming becomes necessary, this can be added as a backward-compatible extension to `StreamResponse`.

## Decision

**No further streaming implementation is needed.** The existing `fetchStream()` with `readLine()`/`readAll()`/`close()` covers all practical use cases for this project:

1. **LLM API streaming** (primary use case): LLM streaming APIs (OpenAI, Anthropic, etc.) use SSE or NDJSON — both are line-delimited. `readLine()` handles this directly.
2. **Large file download**: `readAll()` or `readLine()` loop with progressive output.
3. **SSE event streams**: `readLine()` loop with empty-line detection for event boundaries.

**T233 should be marked "Skip"** — the chunked/callback approach adds complexity without benefit given the existing line-based streaming.

## Potential Future Enhancements (Not Planned)

If the need arises, these could be added as backward-compatible extensions:

1. `readChunk(n)` for byte-level reading (binary protocols)
2. `readSSE()` returning parsed `{event, data, id}` objects (convenience for SSE)
3. Context cancellation support via `opts.signal` or `opts.cancel`

None of these are needed today. They can be added when a concrete use case demands them.
