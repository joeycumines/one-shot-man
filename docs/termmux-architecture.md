# Termmux Architecture

Termmux is a terminal multiplexer embedded in `osm`, providing tmux-like pane management, window management, copy mode, and mouse support. It is implemented as a Go library under `internal/termmux` with JavaScript bindings exposed via the `osm:termmux` module.

## Package Structure

```
internal/termmux/
├── manager.go          SessionManager (worker goroutine, request/response)
├── capture.go          CaptureSession (in-memory PTY emulation)
├── eventbus.go         EventBus (pub/sub for session events)
├── layout.go           LayoutEngine (tile-based pane geometry)
├── pane_manager.go     paneManager (pane registry, borders, focus)
├── pane_keys.go        PaneKeyHandler (key-to-action routing)
├── prefix_keys.go      PrefixKeyHandler (prefix-key command dispatch)
├── copy_mode_keys.go   CopyModeKeyHandler (vi-like copy mode navigation)
├── window.go           Window, WindowManager (tab-like window management)
├── monitor.go          MonitorConfig, VisualBellState, MonitorState
├── lock.go             SessionLock (bcrypt-based session locking)
├── chooser.go          Chooser (navigable session/pane selector)
├── statusbar/          StatusBar (configurable left/center/right)
├── vt/                 VT220/xterm emulator (parser, screen, render)
└── errors.go           Sentinel errors

internal/builtin/termmux/
├── module.go           JS binding surface (osm:termmux)
├── bounce.go           newBounceController JS binding
├── mouse_forward.go    enableMouseForward JS binding
├── control_router.go   newControlRouter JS binding
```

## Data Flow

```
User Input (keyboard/mouse)
    │
    ▼
PrefixKeyHandler ──(if prefix detected)──▶ Command dispatch
    │                                        │
    │ (no prefix)                            ▼
    ▼                                    WindowManager / PaneManager
SessionManager.Input()                    │
    │                                     ▼
    ▼                                 Session action
CopyModeKeyHandler ──(if copy mode)──▶ Scroll/Select/Search
    │
    │ (normal mode)
    ▼
Child PTY (via CaptureSession or real PTY)
    │
    ▼ (output)
VTerm.Write() → Screen update → Snapshot → EventBus → Subscribers
```

## Concurrency Model

SessionManager uses a **single worker goroutine** pattern. All mutable state (sessions, active ID, window list, monitors) is owned exclusively by the worker. Public methods send typed requests through a channel and block on the reply.

Read-only data (snapshots, session info) is published as immutable copies via `atomic.Pointer` for lock-free reads from any goroutine.

```
Caller goroutine          Worker goroutine
       │                       │
       ├── sendRequest() ──────▶│
       │   (blocks on reply)   ├── dispatch()
       │                       ├── handleRegister/handleInput/...
       │◀── response ──────────┤
       │                       │
```

## Session Types

| Type | Description |
|------|-------------|
| `SessionKindPTY` | Real PTY session (spawns a child process) |
| `SessionKindCapture` | In-memory PTY emulation (no child process) |

## Feature Surface

### Window Management
- NewWindow, NextWindow, PrevWindow, RenameWindow, CloseWindow
- Windows displayed in status bar with active indicator
- Each window has independent pane layout

### Pane Layout
- LayoutEven, LayoutMainHorizontal, LayoutMainVertical
- SplitHorizontal, SplitVertical, ClosePane
- FocusPaneUp/Down/Left/Right
- SwapPanes, ZoomPane (toggle full-screen)
- SetMainRatio (configurable main pane size)

### Copy Mode
- EnterCopyMode / ExitCopyMode
- Vi-like navigation: h/l/j/k, Ctrl+U/D, g/G, 0/$, w/b
- SearchForward / SearchBackward with match highlighting
- SelectStart / SelectEnd / SelectedText / CopySelection
- PageUp / PageDown / HalfPageUp / HalfPageDown
- ScrollToTop / ScrollToBottom

### Prefix Key System
- Configurable prefix key (default: Ctrl+B)
- Command table: c=new-window, n=next, p=prev, d=detach, z=zoom, x=close-pane, %=split-h, "=split-v, [=copy-mode

### Mouse Support
- Click-to-focus (hit-test against pane geometries)
- Drag-to-resize (on pane border dividers)
- Scroll in copy mode (wheel events → ScrollCopyMode)
- enableMouseForward JS binding (coordinate translation, SGR encoding)

### Monitoring
- MonitorBell (BEL character → event + visual bell)
- MonitorActivity (background pane output after idle → event)
- MonitorSilence (no output for configured duration → event)
- VisualBell (configurable duration border flash)
- Per-session MonitorConfig

### Synchronized Panes
- Broadcast input to all panes simultaneously
- Toggle via setSynchronizePanes

### Remain-on-Exit
- Keep pane visible after child process exits
- RespawnSession creates new session in same pane
- Per-pane remain-on-exit toggle

### Pipe-Pane
- Tee raw PTY output to file
- SetPipeFile / ClearPipe (runtime toggle)

### Display Message
- Transient overlay messages with configurable duration
- Auto-dismiss after expiry

### Choose-Tree
- Navigable session/pane selector popup
- Up/Down navigation, cursor highlighting
- Active session indicator

### Capture-Pane
- Region selection (startLine/endLine)
- CopyPaneToClipboard via OSC 52

### Lock-Session
- bcrypt-based password locking
- LockSession / UnlockSession / IsLocked

### Status Bar
- Left/center/right sections
- Window status indicators with active highlighting
- Configurable colors and position (top/bottom)

## JS Binding Surface

All features are exposed via `osm:termmux`:

```javascript
const termmux = require('osm:termmux');
const mgr = termmux.newSessionManager({rows: 24, cols: 80});
const {session, sid} = termmux.newBoundedSession({cmd: '/bin/sh'});
const ctrl = termmux.newBounceController({speed: {x:1, y:1}});
const forward = termmux.enableMouseForward({sessionManager: mgr, ...});
const router = termmux.newControlRouter({keys: {'ctrl+p': 'pause'}});
const chooser = termmux.newChooser(activeID);
```
