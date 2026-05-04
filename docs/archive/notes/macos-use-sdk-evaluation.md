# MacosUseSDK Integration Evaluation

**Date:** 2026-02-18 (updated; original 2026-02-14)
**Task:** T237 — Evaluate gRPC proxy approach for MacosUseSDK integration
**Status:** Decision document — event loop migration (T011) and goja-grpc replacement (T012) COMPLETE

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

## 2. Current State: Full Async gRPC Integration

### Event Loop Migration — COMPLETE (T011)

The scripting engine was migrated from `dop251/goja_nodejs/eventloop` to `joeycumines/go-eventloop` + `joeycumines/goja-eventloop`. This was the critical blocker identified in the original evaluation.

**What changed:**
- `internal/scripting/runtime.go`: Uses `goeventloop.New()` + `gojaEventloop.New(loop, vm)` + `adapter.Bind()`
- All loop interactions use `loop.Submit(func())` instead of `RunOnLoop(func(*goja.Runtime))`
- JS globals now include: `setTimeout`, `setInterval`, `Promise`, `AbortController`, `AbortSignal`, `TextEncoder`, `TextDecoder`, `URL`, `crypto`, `process.nextTick`, and 50+ more globals via `adapter.Bind()`

### goja-grpc Replacement — COMPLETE (T012)

`osm:grpc` is now a thin wrapper around [joeycumines/goja-grpc](https://github.com/joeycumines/goja-grpc):

- **All four RPC types** — Unary, server-streaming, client-streaming, bidirectional streaming
- **Promise-based** — Client calls return Promises
- **Event-loop native** — Handlers run on the event loop, thread-safe with Goja
- **AbortSignal** — Cancel in-flight RPCs via `AbortController.signal`
- **Metadata** — Send and receive gRPC metadata
- **In-process channels** — Go↔JS interop via `go-inprocgrpc` (no network I/O)
- **require() integration** — Standard Goja module loading
- **Protobuf** — Descriptor loading via `osm:protobuf` module

### Promise-based Fetch — COMPLETE (T013)

`osm:fetch` now provides `fetch(url, opts?) → Promise<Response>` following the browser Fetch API. HTTP requests run in goroutines with Promise resolution on the event loop.

### Integration with MacosUseSDK

All capabilities are now available with no code changes to osm:

```javascript
const grpc = require('osm:grpc');
const pb = require('osm:protobuf');

// Load MacosUseSDK proto descriptors
pb.loadDescriptorSet(MACOSUSESDK_DESCRIPTOR_SET_BYTES);

// Connect to a running MacosUseSDK server
const channel = grpc.dial('localhost:50051', { insecure: true });
const client = grpc.createClient('macosusesdk.v1.MacosUse');

// Async screenshot capture
const screenshot = await client.captureScreenshot({});

// Find UI elements
const elements = await client.findElements({
  parent: 'applications/com.apple.Calculator',
  selector: { role: 'AXButton', text: { contains: '5' } }
});

// Click an element
await client.clickElement({
  name: elements.elements[0].name
});

// Stream UI observations in real-time (server-streaming RPC)
const stream = await client.streamObservations({
  parent: 'applications/com.apple.Calculator'
});
for await (const event of stream) {
  log.info('UI event: ' + JSON.stringify(event));
}

// Cancellation via AbortController
const ac = new AbortController();
setTimeout(() => ac.abort(), 5000);
const result = await client.waitElement(req, { signal: ac.signal });
```

## 3. Use Cases for osm

### High-Value Use Cases

1. **Automated UI Context Gathering** — Script-driven screenshot capture and accessibility tree traversal to build rich context for LLM prompts.

2. **Claude-Mux Visual Verification** — The planned claude-mux system can use MacosUseSDK for visual verification of Claude Code's work: capture screenshots to confirm UI changes, traverse accessibility trees to verify component structure.

3. **Macro-Driven Workflows** — Record and replay sequences of UI interactions as macros. Useful for repetitive tasks that feed into prompt construction.

4. **Real-Time UI Monitoring** — Stream UI change observations via server-streaming RPCs to detect when an application reaches a desired state before proceeding.

### Lower-Value Use Cases

5. **File Dialog Automation** — Automating file open/save dialogs for batch processing. Niche.
6. **Clipboard Integration** — osm already has clipboard output. MacosUseSDK clipboard tools add clipboard history, which has marginal value.

## 4. Platform Implications

### macOS Only

MacosUseSDK is **inherently macOS-only** — it wraps the macOS Accessibility API (`AXUIElement`), CoreGraphics (`CGWindowList`), and AppKit. There is no Linux or Windows equivalent.

**Impact on osm's cross-platform mandate:**

- `osm` runs on Linux, macOS, and Windows. All tests must pass on all three.
- MacosUseSDK integration is **entirely optional** — a script capability, not a core feature.
- The `osm:grpc` module is cross-platform (generic gRPC client). Scripts using it to connect to MacosUseSDK simply won't find a server on non-macOS systems.
- No conditional compilation or build tags needed. Integration is at the script level.

### Server Lifecycle

MacosUseSDK runs as a **separate process**. osm scripts connect to it as a client:
- Users must start the MacosUseSDK server independently (or use `osm:exec` to spawn it).
- The server requires macOS Accessibility permissions (System Preferences > Privacy & Security > Accessibility).
- No impact on osm's build process, binary size, or dependency tree.

## 5. Current Feasibility

| Factor          | Assessment |
|-----------------|------------|
| Code changes    | **Zero** — integration is at script level |
| Dependencies    | **Already present** — go-eventloop, goja-eventloop, goja-grpc, go-inprocgrpc, goja-protobuf |
| Build impact    | **None** |
| Test impact     | **None** — script-level integration |
| Cross-platform  | **Maintained** — osm:grpc is generic |
| Capability gap  | **None** — full streaming, async, cancellation, AbortSignal |

All blockers identified in the original evaluation have been resolved:
- ~~Event loop migration~~ → **DONE** (T011)
- ~~Synchronous-only osm:grpc~~ → **DONE** (T012, replaced with goja-grpc)
- ~~No Promise support~~ → **DONE** (T013, fetch is also Promise-based)

## 6. Remaining Work

### Immediate (No Blockers)

- **Example script** in `scripts/` demonstrating MacosUseSDK connection (optional, low priority)
- This evaluation document is up to date

### Future (Dependent on Other Work)

- Claude-mux visual verification scripts using MacosUseSDK (after T031–T072)
- In-process gRPC channel for zero-overhead Go↔Swift bridging (future architectural decision)

## 7. Prior Art in Codebase

| Location | Reference |
|----------|-----------|
| `internal/builtin/grpc/grpc.go` | `osm:grpc` — thin wrapper around goja-grpc |
| `internal/builtin/grpc/grpc_test.go` | Promise-based echo round-trip test with inprocgrpc |
| `docs/scripting.md` | `osm:grpc` and `osm:protobuf` documented in module table |
| `docs/security.md` | `osm:grpc` security assessment |
| `docs/architecture.md` | `osm:grpc` listed in native modules |
