# MacosUseSDK Integration Evaluation

**Date:** 2026-02-14
**Task:** T029 — Evaluate MacosUseSDK integration with osm

## What is MacosUseSDK?

[github.com/joeycumines/MacosUseSDK](https://github.com/joeycumines/MacosUseSDK) is a fork of [mediar-ai/MacosUseSDK](https://github.com/mediar-ai/MacosUseSDK) (188 stars, MIT license). It is a macOS accessibility automation framework consisting of:

- **Swift core library** (`Sources/MacosUseSDK/`): UI traversal via Accessibility APIs, input simulation (click, type, keypress, mouse move), visual feedback overlays, application management.
- **Command-line tools**: `TraversalTool`, `InputControllerTool`, `VisualInputTool`, `AppOpenerTool`, `HighlightTraversalTool`, `ActionTool`.
- **Swift gRPC server** (`Server/`): Production-ready server exposing **77 MCP tools** via HTTP/SSE or stdio transport. Supports TLS, API key auth, rate limiting, audit logging.
- **Go MCP proxy layer** (`internal/`, `cmd/`): Go modules for config, server transport, and protobuf handling. Uses `buf` for protobuf code generation.

### 77 MCP Tool Categories

| Category    | Count | Examples                                          |
|-------------|-------|---------------------------------------------------|
| Screenshot  | 4     | Capture screen/window/region/element              |
| Input       | 11    | Click, type, keypress, scroll, drag, gestures     |
| Element     | 10    | Find, get, click, traverse accessibility tree     |
| Window      | 9     | List, focus, move, resize, minimize, close        |
| Display     | 3     | List displays, cursor position                    |
| Clipboard   | 4     | Get/write/clear clipboard, history                |
| Application | 4     | Open, list, get, delete applications              |
| Scripting   | 4     | Execute AppleScript, JavaScript, shell commands   |
| Observation | 5     | Create/stream/manage real-time UI change monitors |
| Session     | 8     | Session/transaction management                    |
| Macro       | 6     | Record/replay macro automation                    |
| File Dialog | 5     | Automate open/save/select file dialogs            |
| Input Query | 2     | Query input state                                 |
| Discovery   | 2     | Scripting dictionaries, accessibility watch       |

## Feasibility Assessment

### Integration Approach: In-Process gRPC Channel

**TO BE CLEAR: This client does exist. `github.com/joeycumines/goja-grpc`.

### Alternative: Simpler Integration via osm:exec

No. Do not do this.

### Alternative: HTTP/MCP Client via osm:fetch

No. Do not do this.

## Prerequisites and Blockers

### Not blockers

1. **In-process gRPC channel implementation** — Does exist.
2. **`github.com/joeycumines/go-eventloop`** — Published.
3. **Cross-platform build strategy** — Not needed, gRPC can run on any platform. The server is OBVIOUSLY separate.

## Conclusion

**Integration is FEASIBLE NOW.** All prerequisites exist:

- `github.com/joeycumines/goja-grpc` provides in-process gRPC channels for Goja
- `github.com/joeycumines/go-eventloop` is published for async operation support
- Cross-platform build is not required (the MacosUseSDK server runs separately)

**Recommendation:** Integration can proceed when prioritized. The implementation path is:

1. Add `goja-grpc` as a dependency
2. Create an `osm:grpc` native module exposing channel creation and RPC calls
3. Scripts can then connect to a running MacosUseSDK gRPC server and invoke any of the 77 MCP tools
