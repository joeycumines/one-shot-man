# MacosUseSDK Integration Evaluation

**Date:** 2026-02-17 (updated; original 2026-02-14)
**Task:** T237 — Evaluate gRPC proxy approach for MacosUseSDK integration
**Status:** Decision document (supersedes prior T029 evaluation)

## 1. What is MacosUseSDK?

[mediar-ai/MacosUseSDK](https://github.com/mediar-ai/MacosUseSDK) (188 stars, MIT license) is a macOS accessibility automation framework written in Swift. It provides programmatic access to the macOS Accessibility API for UI traversal, input simulation, and application management.

[joeycumines/MacosUseSDK](https://github.com/joeycumines/MacosUseSDK) is a fork (same owner as `osm`) that extends the upstream with:

- **gRPC API layer** — A well-designed Protocol Buffer API following Google AIPs, with Go code generation (`buf`)
- **Go MCP proxy** — Go modules under `internal/` and `cmd/` for server transport and protobuf handling
- **Production features** — TLS, API key auth, rate limiting, Prometheus metrics, structured audit logging
- **Two transports** — stdio (JSON-RPC 2.0 for Claude Desktop) and HTTP/SSE

### Components

- **Swift core library** (`Sources/MacosUseSDK/`): UI traversal via Accessibility APIs, input simulation (click, type, keypress, mouse move), visual feedback overlays, application management.
- **Command-line tools**: `TraversalTool`, `InputControllerTool`, `VisualInputTool`, `AppOpenerTool`, `HighlightTraversalTool`, `ActionTool`.
- **Swift gRPC server** (`Server/`): Production-ready server exposing **77 MCP tools** via HTTP/SSE or stdio transport.
- **Go MCP proxy layer** (`internal/`, `cmd/`): Go modules for config, server transport, and protobuf handling. Uses `buf` for protobuf code generation.

### 77 MCP Tool Categories

| Category        | Count | Key Capabilities                                        |
|-----------------|-------|---------------------------------------------------------|
| Screenshot      | 4     | Screen/window/region/element capture, OCR extraction    |
| Input           | 11    | Click, type, keypress, scroll, drag, gestures, hover    |
| Element         | 10    | Find, get, click, traverse accessibility tree, wait     |
| Window          | 9     | List, focus, move, resize, minimize, close              |
| Display         | 3     | List displays, cursor position                          |
| Clipboard       | 4     | Get/write/clear clipboard, history                      |
| Application     | 4     | Open, list, get, delete applications                    |
| Scripting       | 4     | Execute AppleScript, JavaScript (JXA), shell commands   |
| Observation     | 5     | Create/stream real-time UI change monitors              |
| Session         | 8     | Session/transaction management with ACID semantics      |
| Macro           | 6     | Record/replay macro automation, loops, conditionals     |
| File Dialog     | 5     | Automate open/save/select file dialogs                  |
| Input Query     | 2     | Query input state                                       |
| Discovery       | 2     | Scripting dictionaries, accessibility watch             |

### Proto API Structure

```
proto/macosusesdk/
├── type/              # Common types (AIP-213)
│   ├── element.proto    # UI element and traversal types
│   ├── geometry.proto   # Point and geometric types
│   └── selector.proto   # Selector grammar definitions
└── v1/                # API v1 definitions
    ├── application.proto
    ├── clipboard.proto
    ├── condition.proto
    ├── input.proto
    ├── macos_use.proto  # MacosUse service definition
    ├── macro.proto
    ├── observation.proto
    ├── screenshot.proto
    ├── script.proto
    ├── session.proto
    └── window.proto
```

The API follows Google AIPs strictly: resource-oriented design, long-running operations (AIP-151), pagination (AIP-158), standard methods (AIP-130–135), custom methods (AIP-136).

## 2. Integration Approach: gRPC Proxy via osm:grpc

### Current State of osm:grpc

`osm` already has a working `osm:grpc` module (`internal/builtin/grpc/grpc.go`) that provides:

```javascript
const grpc = require('osm:grpc');

// Load proto descriptors (base64-encoded FileDescriptorSet)
grpc.loadDescriptorSet(base64String);

// Connect to gRPC server
const conn = grpc.dial('localhost:50051', { insecure: true });

// Make unary RPC call
const resp = conn.invoke('/package.Service/Method', { field: 'value' });

// Close connection
conn.close();
```

**Current limitations:**
- **Synchronous only** — `conn.invoke()` blocks the Goja event loop. No streaming support.
- **Uses `google.golang.org/grpc` directly** — Not `goja-grpc`. This was a deliberate choice because osm's scripting engine uses `dop251/goja_nodejs/eventloop` which is **incompatible** with the `go-eventloop` required by `goja-grpc`.
- **Dynamic messages via protojson** — Converts JS objects ↔ proto messages through JSON marshaling with `dynamicpb`.

### The goja-grpc Library

[joeycumines/goja-grpc](https://github.com/joeycumines/goja-grpc) (created 2026-02-13) provides a far richer JavaScript gRPC experience:

- **All RPC types** — Unary, server-streaming, client-streaming, bidirectional streaming
- **Promise-based** — Client calls return promises; server handlers can return promises
- **Event-loop native** — Handlers run on the event loop, thread-safe with Goja
- **AbortSignal** — Cancel in-flight RPCs via `AbortController.signal`
- **Metadata** — Send and receive gRPC metadata
- **In-process channels** — Go↔JS interop via `go-inprocgrpc` (no network I/O)
- **require() integration** — Standard Goja module loading

**Hard dependency:** Requires `go-eventloop` (NOT `dop251/goja_nodejs/eventloop`). This is the single blocker for dropping `goja-grpc` into `osm` as-is.

### Three Integration Tiers

#### Tier 1: Use Current osm:grpc (Available NOW)

Works today, no code changes needed:

```javascript
const grpc = require('osm:grpc');

// Load MacosUseSDK proto descriptors
grpc.loadDescriptorSet(MACOSUSESDK_DESCRIPTOR_SET_B64);

// Connect to a running MacosUseSDK server
const conn = grpc.dial('localhost:50051', { insecure: true });

// Capture screenshot
const screenshot = conn.invoke('/macosusesdk.v1.MacosUse/CaptureScreenshot', {});

// Find UI elements
const elements = conn.invoke('/macosusesdk.v1.MacosUse/FindElements', {
  parent: 'applications/com.apple.Calculator',
  selector: { role: 'AXButton', text: { contains: '5' } }
});

// Click an element
conn.invoke('/macosusesdk.v1.MacosUse/ClickElement', {
  name: elements.elements[0].name
});

conn.close();
```

**Pros:** Works immediately. No event loop migration. No new dependencies.
**Cons:** No streaming (can't use `StreamObservations`, `WatchAccessibility`). Blocking calls freeze the UI during long operations (screenshot OCR, macro execution). Can't cancel in-flight RPCs.

#### Tier 2: Migrate to go-eventloop + goja-grpc (Medium Effort)

Requires migrating osm's scripting engine from `dop251/goja_nodejs/eventloop` to `go-eventloop`. This unlocks:

```javascript
const grpc = require('grpc');
const pb = require('protobuf');

pb.loadDescriptorSet(descriptorBytes);
const client = grpc.createClient('macosusesdk.v1.MacosUse');

// Async unary RPC
const screenshot = await client.captureScreenshot({});

// Server-streaming: watch for UI changes
const stream = await client.watchAccessibility({
  parent: 'applications/com.apple.Calculator'
});
while (true) {
  const { value, done } = await stream.recv();
  if (done) break;
  console.log('UI changed:', value);
}

// Cancellation
const ac = new AbortController();
setTimeout(() => ac.abort(), 5000);
const result = await client.waitElement(req, { signal: ac.signal });
```

**Pros:** Full streaming support. Promise-based async. AbortSignal cancellation. In-process channels (no network overhead). Bidirectional streaming for real-time observation.
**Cons:** Event loop migration is a significant refactor — it touches the entire scripting subsystem. Every existing module would need verification.

#### Tier 3: In-Process gRPC Channel (Highest Potential)

Go-side MacosUseSDK server implementation running in the same process as osm, connected via `go-inprocgrpc`. The server would bridge to the Swift accessibility APIs via a subprocess.

**This is future-looking and not recommended now.** The MacosUseSDK server is a separate Swift process — running it in-process with Go would require fundamental architectural changes to MacosUseSDK itself.

## 3. Use Cases for osm

### High-Value Use Cases

1. **Automated UI Context Gathering** — Script-driven screenshot capture and accessibility tree traversal to build rich context for LLM prompts. e.g., "Capture the current state of this dialog and its accessible elements, add to prompt context."

2. **AI Orchestrator Enhancement** — The planned AI Orchestrator (T238–T255) could use MacosUseSDK for visual verification of Claude Code's work. Capture screenshots to confirm UI changes, traverse accessibility trees to verify DOM/component structure.

3. **Macro-Driven Workflows** — Record and replay sequences of UI interactions as macros. Useful for repetitive tasks that feed into prompt construction (e.g., navigating to a specific code file in an IDE, selecting text, capturing it).

4. **Real-Time UI Monitoring** — Stream UI change observations to detect when an application reaches a desired state before proceeding with prompt construction. (Requires Tier 2.)

### Lower-Value Use Cases

5. **File Dialog Automation** — Automating file open/save dialogs for batch processing. Niche.
6. **Clipboard Integration** — osm already has clipboard output. MacosUseSDK clipboard tools add clipboard history, which has marginal value.

## 4. Platform Implications

### macOS Only

MacosUseSDK is **inherently macOS-only** — it wraps the macOS Accessibility API (`AXUIElement`), CoreGraphics (`CGWindowList`), and AppKit. There is no Linux or Windows equivalent.

**Impact on osm's cross-platform mandate:**

- `osm` runs on Linux, macOS, and Windows. All tests must pass on all three.
- MacosUseSDK integration must be **entirely optional** — a script capability, not a core feature.
- The `osm:grpc` module is already cross-platform (it's a generic gRPC client). Scripts using it to connect to MacosUseSDK simply won't find a server to connect to on non-macOS systems.
- No conditional compilation or build tags needed. The integration is at the script level.

### Server Lifecycle

MacosUseSDK runs as a **separate process**. osm scripts connect to it as a client. This means:
- Users must start the MacosUseSDK server independently (or osm scripts could use `osm:exec` to spawn it).
- The server requires macOS Accessibility permissions (System Preferences > Privacy & Security > Accessibility).
- No impact on osm's build process, binary size, or dependency tree for the Tier 1 approach.

## 5. Feasibility Assessment

### Tier 1 (Current osm:grpc → MacosUseSDK server): **Feasible NOW**

| Factor          | Assessment |
|-----------------|------------|
| Code changes    | **Zero** — osm:grpc already works |
| Dependencies    | **None new** — already has google.golang.org/grpc |
| Build impact    | **None** |
| Test impact     | **None** — integration is at script level |
| Cross-platform  | **Maintained** — osm:grpc is generic |
| Capability gap  | **Significant** — no streaming, blocking calls |

### Tier 2 (go-eventloop + goja-grpc): **Feasible, High Effort**

| Factor          | Assessment |
|-----------------|------------|
| Code changes    | **Major** — event loop migration across scripting subsystem |
| Dependencies    | go-eventloop, goja-eventloop, goja-grpc, go-inprocgrpc, goja-protobuf |
| Build impact    | Moderate — new deps, but all pure Go |
| Test impact     | **Extensive** — all scripting tests need verification |
| Cross-platform  | **Maintained** — all deps are pure Go |
| Capability gap  | **Minimal** — full streaming, async, cancellation |

### Event Loop Migration Blocker

The comment in `internal/builtin/grpc/grpc.go` explicitly documents the situation:

> This module uses google.golang.org/grpc directly rather than goja-grpc, because the current osm scripting engine uses dop251/goja_nodejs/eventloop which is incompatible with the go-eventloop required by goja-grpc. Once the event loop subsystem is migrated to go-eventloop, this module can be replaced with a thin wrapper around goja-grpc for full promise-based async support.

The event loop migration is the **critical path** for Tier 2. It is not specific to MacosUseSDK — it also unlocks promise-based async for all osm scripting.

## 6. Recommendation

### Decision: **Proceed with Tier 1 (immediate), plan Tier 2 (future)**

**Rationale:**

1. **Tier 1 is free.** `osm:grpc` already exists and works. The only deliverable is example scripts and documentation showing how to connect to a MacosUseSDK server. Zero code changes to osm itself.

2. **Tier 2 is valuable but orthogonal.** The event loop migration benefits ALL of osm's async story, not just MacosUseSDK. It should be planned as its own epic, not gated on this integration.

3. **The use cases are real but not urgent.** Automated UI context gathering and AI Orchestrator visual verification are compelling, but they depend on other in-progress work (T238–T255 AI Orchestrator).

4. **Platform constraint is acceptable.** MacosUseSDK is macOS-only, but the integration is purely at the script level. Scripts that use it simply document the macOS requirement. No core osm behavior changes.

### Immediate Actions

- **No code changes required.** Tier 1 works today.
- One potential deliverable: an example script in `scripts/` demonstrating MacosUseSDK connection (low priority, can be done anytime).
- Document the integration path in this evaluation (done).

### Future Actions (Not Blocking)

- Event loop migration from `dop251/goja_nodejs/eventloop` to `go-eventloop` (unlocks Tier 2 for all async use cases, not just MacosUseSDK)
- Replace `osm:grpc` synchronous implementation with thin `goja-grpc` wrapper (after event loop migration)
- AI Orchestrator visual verification scripts using MacosUseSDK (after T238–T255)

## 7. Prior Art in Codebase

| Location | Reference |
|----------|-----------|
| `internal/builtin/grpc/grpc.go` | Working `osm:grpc` module with `loadDescriptorSet`, `dial`, `invoke`, `close` |
| `internal/builtin/grpc/grpc_test.go` | Unit tests for `osm:grpc` |
| `docs/scripting.md` | `osm:grpc` documented in module table |
| `docs/security.md` | `osm:grpc` security assessment |
| `docs/todo.md:75-76` | Original integration idea with gRPC proxy mechanism |
| `docs/architecture.md:154` | `osm:grpc` listed in native modules |
| `.claude/skills/takumi-prompting/references/prompt-library.md` | Cross-project reference to MacosUseSDK |
