# T234: Code Review Splitter Workflow Engine Evaluation

**Status:** Decision — Defer to AI Orchestrator  
**Date:** 2026-02-17  
**Affects:** T238-T255 (AI Orchestrator)

## Context

The code review command (`osm code-review`) currently uses a clipboard-first workflow:
1. Collect diff + context → build prompt from template
2. For large diffs, `splitDiff()` chunks by file boundaries (respecting `DefaultMaxDiffLines`)
3. User copies chunks to clipboard via `review-chunks N` and pastes into their chosen LLM UI

T234 evaluates how to add LLM-calling capability to this workflow — specifically, how a "workflow engine" should orchestrate multi-chunk reviews where the tool calls an LLM API directly.

## Current Architecture

```
code_review.go (Go)
  ├── code_review_template.md (embedded)
  ├── code_review_script.js (embedded, JS)
  │     ├── buildPrompt() → template + context
  │     ├── review-chunks command → splitDiff() → clipboard per chunk
  │     └── interactive terminal (scripting.Terminal)
  └── SplitDiff(diff, maxLines) → []DiffChunk (Go, exposed to JS)
```

Key facts:
- `splitDiff()` is already production-quality (file-boundary splitting, line counting, chunk indexing)
- Template system works (Go text/template with txtar context)
- No LLM API calls anywhere in the codebase today (by design: osm is offline-first)

## Three Approaches Evaluated

### Approach 1: Lightweight BT Engine (osm:bt)

Use the existing `osm:bt` module to orchestrate multi-chunk reviews:

```js
var bt = require('osm:bt');
var fetch = require('osm:fetch');

var reviewTree = bt.sequence([
    bt.node("split-diff", function(bb) {
        bb.set("chunks", splitDiff(bb.get("diff"), maxLines));
        return "success";
    }),
    bt.node("review-each-chunk", function(bb) {
        var chunks = bb.get("chunks");
        for (var i = 0; i < chunks.length; i++) {
            var resp = fetch.fetch(llmEndpoint, {
                method: "POST",
                body: JSON.stringify({ prompt: buildChunkPrompt(chunks[i]) })
            });
            bb.set("review-" + i, resp.json());
        }
        return "success";
    }),
    bt.node("consolidate", function(bb) { /* merge reviews */ })
]);
```

**Pros:**
- Uses existing infrastructure (no new dependencies)
- BT gives retry, fallback, conditional logic out of the box
- Script-level customization (users can modify behavior)

**Cons:**
- Synchronous `fetch()` blocks the entire event loop during LLM calls
- No streaming output — user sees nothing until the full response arrives
- Error recovery is manual (BT helps, but no automatic rate-limit backoff)
- Each LLM provider has different API shapes — template explosion
- BT is designed for game-loop tick patterns, not long-running HTTP workflows

**Verdict:** Possible but wrong tool for this job. BT excels at frame-by-frame decision trees, not minutes-long API call orchestration.

### Approach 2: Per-LLM Templates + Simple Loop

Skip the workflow engine entirely. Add per-provider prompt templates and a simple sequential loop:

```js
// Provider-specific template selection
var provider = config.get("llm.provider"); // "openai", "anthropic", "ollama"
var template = loadTemplate(provider);

// Simple loop
var chunks = splitDiff(diff, maxLines);
for (var i = 0; i < chunks.length; i++) {
    var prompt = template.format(chunks[i]);
    var resp = callLLM(provider, prompt);
    results.push(resp);
}
// Consolidate
var merged = mergeReviews(results);
```

**Pros:**
- Simplest possible implementation
- Easy to understand and debug
- Template-per-provider solves API shape differences

**Cons:**
- No retry/backoff/error handling beyond basic try/catch
- No parallelism (but that's fine for sequential chunk reviews)
- Hardcodes the workflow shape — can't adapt to provider-specific capabilities
- Still requires per-provider API clients (auth, endpoints, model selection)
- Duplicates work that the AI Orchestrator (T238-T255) will do better

**Verdict:** Works for a prototype but will be thrown away when the AI Orchestrator lands.

### Approach 3: AI Orchestrator Subsumption (Recommended)

Defer LLM-calling code review to the AI Orchestrator (T238-T255), which will provide:

- **Provider abstraction** (T243): Single interface for OpenAI, Anthropic, Ollama
- **BT orchestration** (T244): Purpose-built for multi-step LLM workflows
- **PR splitting** (T245): Specifically designed for code-review-at-scale
- **Error recovery** (T248): Rate limiting, retry, cancellation
- **Session isolation** (T249): Per-review state management

The code review command keeps its clipboard-first workflow unchanged. When the AI Orchestrator is ready, a second mode is added:

```
osm code-review                    # existing clipboard mode
osm code-review --provider=ollama  # AI Orchestrator mode (T245)
```

**Pros:**
- No throwaway code — AI Orchestrator is the proper abstraction
- Provider support is solved once for all commands (code-review, goal, prompt-flow)
- Multi-chunk review with streaming, retry, and progress is handled at the infrastructure level
- Clipboard-first remains the default (no API keys required)

**Cons:**
- T238-T255 is a large effort (12+ tasks)
- Code review LLM support is blocked until AI Orchestrator is ready

**Verdict:** Correct long-term approach. The short-term gap (no LLM-calling code review) is already covered by the clipboard workflow.

## Decision

**Defer to AI Orchestrator (Approach 3).**

Rationale:
1. Building a standalone LLM-calling workflow engine for code review alone is wasteful — it duplicates T238-T255 work.
2. The clipboard-first workflow already works well — users paste into any LLM UI, no API keys needed.
3. The diff splitter (`splitDiff`) and template system are ready — they'll plug directly into the AI Orchestrator when it arrives.
4. The `review-chunks` command already handles the multi-chunk UX — the AI Orchestrator just needs to automate "copy chunk → call LLM → collect response."

**No code changes needed for T234.** The existing `code_review.go` and `code_review_script.js` are preserved unchanged. The AI Orchestrator will add a `--provider` flag (T245) that activates LLM-calling mode.

## Impact on Blueprint

- T234: Done (this evaluation)
- T238-T255: Unchanged (AI Orchestrator tasks proceed as planned)
- No new tasks created
