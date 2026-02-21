# Security Posture: JS Sandbox Boundaries

> **TL;DR:** osm is a local developer tool. The Goja JS runtime provides
> **language-level isolation** (JS cannot call arbitrary Go code), but
> **not security isolation** (exec, readFile, fetch are intentionally unrestricted).
> The security boundary is the OS user's permissions.

## Design Philosophy

osm runs scripts on the developer's own machine, with the developer's own
credentials. It is **not** a sandbox, container, or multi-tenant execution
environment. The threat model assumes the user trusts the scripts they choose
to run — just like they trust shell scripts they execute.

## Goja Runtime Isolation

The [Goja](https://github.com/dop251/goja) JavaScript engine provides:

| Property | Status |
|---|---|
| Separate VM per `Engine` instance | ✅ Verified |
| No shared global state between VMs | ✅ Verified |
| No prototype pollution across VMs | ✅ Verified |
| No access to Go `reflect` package | ✅ Verified |
| No access to Go `unsafe` package | ✅ Verified |
| No access to Go `runtime` package | ✅ Verified |
| No `os.Exit` / process termination | ✅ Verified |
| No Node.js builtins (`fs`, `child_process`, `net`, etc.) | ✅ Verified |
| No browser globals (`window`, `document`, `fetch`) | ✅ Verified |
| Context cancellation interrupts execution | ✅ Verified |

These properties are continuously verified by `internal/security_sandbox_test.go`.

## Module Security Surface

### Registered Modules

All native modules use the `osm:` prefix and are registered in
`internal/builtin/register.go`. The JS `require()` system only loads:

1. **`osm:` prefixed modules** — explicitly registered Go functions
2. **File-based modules** — `.js` files from configured module paths

Attempts to `require('go:os')`, `require('node:fs')`, or other prefixes fail.

### Module-by-Module Analysis

#### `osm:exec` — Command Execution

| API | Security Notes |
|---|---|
| `exec(cmd, ...args)` | Uses `exec.CommandContext` — **no shell**. Arguments are passed directly, not interpreted. Shell metacharacters like `$(id)` are literal strings. |
| `execv(cmd, args[])` | Same as `exec` but takes args as array. |

- **Stdin:** Wired to `os.Stdin` (by design — the user is the operator)
- **No shell expansion:** `exec('echo', '$(id)')` outputs the literal `$(id)`
- **No dangerous extras:** No `spawn`, `fork`, `kill`, `system`, or `popen`

#### `osm:os` — File Operations

| API | Security Notes |
|---|---|
| `readFile(path)` | Reads any file accessible to the user. **No path restrictions** — by design. |
| `fileExists(path)` | Checks existence. No restrictions. |
| `writeFile(path, content, opts?)` | Writes/creates files. Path resolved to absolute. No path restrictions. |
| `appendFile(path, content, opts?)` | Appends to files. Same behavior as `writeFile`. |
| `openEditor(path)` | Opens `$EDITOR`. Wires stdin/stdout/stderr. |
| `clipboardCopy(text)` | Uses `pbcopy`/`clip`/`xclip` via `exec.CommandContext`. |
| `getenv(name)` | Reads environment variables. No restrictions. |

- **Controlled file writes:** `writeFile` and `appendFile` can create/modify files accessible to the user. No destructive operations (`unlink`, `mkdir`, `rename`, `chmod`, etc.)
- **No `setenv`/`unsetenv`:** Cannot modify the process environment

#### `osm:fetch` — HTTP Client

| API | Security Notes |
|---|---|
| `fetch(url, opts)` | Promise-based HTTP client. Unrestricted — no URL filtering, no SSRF mitigation. |

- **No restrictions:** Intentional for a local dev tool. The user controls which scripts run.
- **No server-side:** No `createServer`, `listen`, or socket APIs.

#### `osm:grpc` — gRPC Client & Server (via goja-grpc)

| API | Security Notes |
|---|---|
| `createClient(service)` | Creates gRPC client stub. Methods return Promises. |
| `createServer(service, handler)` | Creates in-process gRPC server (no network binding). |
| `dial(target, opts)` | Opens gRPC channel. Supports insecure option. |
| `status` | gRPC status code constants. |
| `metadata` | gRPC metadata construction. |
| `enableReflection(server)` | Enables gRPC reflection on a server. |
| `createReflectionClient(channel)` | Creates reflection client for service discovery. |

- **In-process channel:** Uses `go-inprocgrpc` — server runs in-process without binding a network port.
- **No raw network server:** No `listen`, `serve`, or `Server` constructor. `createServer` registers handlers on the in-process channel only.
- **Promise-based:** All RPC calls return Promises (unary, server-streaming, client-streaming, bidirectional).

#### `osm:protobuf` — Protocol Buffers

| API | Security Notes |
|---|---|
| `loadDescriptorSet(bytes)` | Loads binary FileDescriptorSet into the protobuf registry. |

- **Read-only registry:** Only loads descriptors for use with `osm:grpc`. No file system access.

#### `osm:text/template` — Go Templates

| API | Security Notes |
|---|---|
| `new(name)` | Creates a `text/template`. |
| `parse(text)` | Parses template string. |
| `execute(data)` | Renders with data. |

- **`text/template`** (not `html/template`): No auto-escaping, but acceptable for a CLI tool.
- JS functions can be registered as template funcs, but they execute in the same Goja VM.

#### Low-Risk Modules

| Module | API Surface | Risk |
|---|---|---|
| `osm:time` | `sleep(ms)` | Minimal — only delays |
| `osm:argv` | `parseArgv`, `formatArgv` | String processing only |
| `osm:nextIntegerID` | `new()` → counter | Pure computation |
| `osm:flag` | Go `flag` wrapper | Argument parsing |
| `osm:unicodetext` | `width`, `truncate` | String processing |
| `osm:ctxutil` | Context manager | Uses git diff via exec |
| `osm:lipgloss` | Terminal styling | Pure rendering |
| `osm:bubbletea` | TUI framework | Terminal I/O |
| `osm:bubblezone` | Mouse hit-testing | UI only |
| `osm:bubbles/*` | Text input/viewport | UI only |
| `osm:termui/*` | Scrollbar widget | UI only |
| `osm:bt` | Behavior trees | JS orchestration |
| `osm:pabt` | Planning-augmented BT | JS orchestration |

## Global Scope

The Goja VM exposes exactly these globals:

| Global | Purpose |
|---|---|
| `ctx` | Context management (alias) |
| `context` | Context management (files, diffs, git state) |
| `log` | Logging (debug, info, warn, error, printf) |
| `output` | Output formatting (print, printf) |
| `tui` | Terminal UI integration |
| `require` | Module loading (CommonJS) |

**Not present:** `process`, `Buffer`, `global`, `__filename`, `__dirname`,
`window`, `document`, `navigator`, `Deno`, `exit`, `quit`.

## Require System

The `require()` function resolves modules in this order:

1. Check registered `osm:` native modules
2. Check file paths relative to the requiring module
3. Check configured module search paths (`WithModulePaths`)
4. Walk parent directories for `node_modules/` folders

Unknown prefixes (`go:`, `node:`, etc.) are rejected. The `shebangStrippingLoader`
removes `#!` lines from loaded files for Unix compatibility.

`__dirname` and `__filename` are set per-module by the CommonJS loader, not
globally. They are not available in the top-level script scope.

### Module Path Hardening

When module search paths are configured (equivalent to `NODE_PATH`), a
hardened resolution layer activates with three security mechanisms:

| Mechanism | What It Prevents |
|---|---|
| **Path traversal blocking** | Bare module names with `../` components (e.g., `require('x/../../secret')`) cannot escape the module directory via either global folders or `node_modules` walk. |
| **Symlink escape detection** | Symlinks within module directories that resolve outside the allowed paths are blocked at file-read time. The pre-symlink path must be within an allowed dir, AND the post-symlink resolved path must also stay within allowed dirs. |
| **Circular dependency warning** | `require()` is wrapped with a cycle tracker that detects `a → b → a` loops and logs a warning. Execution continues (matching Node.js behavior of returning partial exports). |

**Implementation:** `internal/scripting/module_hardening.go`

The hardened path resolver applies two checks:

1. **Check 1 (Global folder containment):** When the resolution base exactly
   matches a configured global folder, the resolved path must stay within
   the allowed directories.

2. **Check 2 (Bare module traversal):** When a bare module name (not starting
   with `.`, `..`, or `/`) contains `..` path components, the resolved path
   must stay within the resolution base directory. This catches traversal via
   the `node_modules` directory walk where goja constructs base paths that
   don't match any configured global folder.

Relative requires (`./foo`, `../bar`) are exempt from these checks — their
`../` traversal from within a script is legitimate Node.js behavior.

**Test coverage:** `internal/scripting/module_hardening_test.go` (12 tests)
+ `internal/security_sandbox_test.go` (4 path traversal test functions).

## Context Cancellation

When the Go context is cancelled (e.g., Ctrl+C), `context.AfterFunc` calls
`vm.Interrupt()` to halt JavaScript execution. This prevents runaway scripts
and ensures clean shutdown.

## Test Coverage

Security properties are verified by three test files:

| File | Focus | Tests |
|---|---|---|
| `internal/security_test.go` | Path traversal, command injection, env vars, TUI input | ~40 tests |
| `internal/security_input_test.go` | Input validation across all entry points (exec, readFile, config, fetch) | 35+ tests |
| `internal/security_sandbox_test.go` | JS sandbox boundaries, VM isolation, module API surfaces, require() traversal | 23+ tests |
| `internal/scripting/module_hardening_test.go` | Module path hardening: traversal, symlink escape, circular deps, validation | 12 tests |

## Known Intentional "Risks"

These are **not bugs** — they are design decisions for a local developer tool:

1. **`readFile` can read any file** — the user is the operator
2. **`exec` passes stdin from `os.Stdin`** — interactive commands need terminal input
3. **`fetch` has no URL restrictions** — the user controls which scripts run
4. **`getenv` reads all environment variables** — needed for tool/editor configuration
5. **`openEditor` wires stdio** — that's literally what editors need
6. **`clipboardCopy` executes system clipboard commands** — that's the tool's core purpose
