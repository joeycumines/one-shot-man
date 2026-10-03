# Dataflow from launcher to captured cells

## Launcher behavior

`/Users/joeyc/dev/joeyc-ai/osm/ai-tool.js` selects `termmux` mode for the Claude TUI (`TUI_TOOLS`, lines 42–48; `resolveLaunchMode`, around line 1086). `launchInTermmux` creates a bounded PTY session, wraps it in `termpane`, and puts the pane's view content into the compositor (lines 1368–1425 and 1597–1601).

The launcher does not apply underline styling. It passes `pane.view().content` to `compositor.updatePaneIfNew` and renders the compositor; its explicit terminal response concerns OSC 11 background color, not SGR underline. The JS launch uses `envReplace: false` and overlays plan entries on the inherited environment (`childEnvironment`, lines 1289–1302; termmux launch at line 1599), but that does not preserve the inherited `TERM` unchanged.

`CaptureSession.Start` passes no `Term` override to `pty.Spawn` (`internal/termmux/capture.go:180-190`). `SpawnConfig.applyDefaults` sets an empty `Term` to `xterm-256color` (`internal/termmux/pty/pty.go:96-112`), and the Unix spawn path always overlays `TERM` with `cfg.Term` (`internal/termmux/pty/pty_unix.go:164-185`). `ai-tool.js` supplies no `TERM` in its plan. Thus Claude Code inside the bounded PTY receives `TERM=xterm-256color` unless a plan explicitly sets it, even if the outer direct Claude baseline ran with a tmux-specific `TERM`. The direct baseline's actual `TERM` is not present in either capture.

The child starts at 22x80, then `resizeSessionToPane()` sends a `WindowSize` through the pane model. That is a possible source of transient startup-layout differences, but it does not explain a stable underline attribute by itself. The pane resize path resizes the VTerm before the PTY (`internal/termui/termpane/termpane.go:325-347`; `internal/termmux/manager.go:2754-2766`). The `TERM` difference is a more relevant confounder for the baseline comparison because it can change which terminal capabilities Claude selects.

## Separate startup OSC reply echo

The additional startup text reported by the user matches the launcher's constructed replies byte-for-byte. `terminal-theme.js` builds `ESC ] 10;rgb:<rgb> BEL` followed by `ESC ] 11;rgb:<rgb> BEL` (`/Users/joeyc/dev/joeyc-ai/osm/ai-lib/terminal-theme.js:21-27, 29-53, 63-66`). `ai-tool.js` requests the outer terminal's background color at initialization and writes the reply into the child session when `BackgroundColor` arrives, and again on resize once the color is known (`ai-tool.js:1515, 1544-1557`). The related OSC 11 relay dates to `bc831bff` (Sep. 24); OSC 10 was added in `a255008d` (Oct. 3).

That `session.write()` is child input, not a write to the real terminal: the Go binding routes it through `SessionManager.Input`, which calls the child session's `Write` (`internal/builtin/termmux/passthrough.go:366-369`; `internal/termmux/manager.go:2578-2626`). Unix PTY startup calls `creackpty.Open()` and clears only `TOSTOP`; it does not disable `ECHO` or wait for the child to enter raw mode (`internal/termmux/pty/pty_unix.go:125-135, 273-285`). If the reply arrives while PTY control-character echo is enabled, the line discipline can echo ESC and BEL as `^[` and `^G`, producing the exact visible form reported.

The constructed sequence contains no SGR underline code, and the user says this output occurred before the underline issue. It is therefore a separate likely startup echo/leak, not a plausible direct cause of the underlined screen cells. Static source confirms the reply is sent but does not establish the child's PTY echo mode at that exact moment.

## Pane and compositor path

The JS `termpane.view()` binding refreshes the snapshot and calls `m.ANSIView()` (`internal/builtin/termui/termpane/termpane.go:289-307`). `ANSIView` obtains `CaptureANSI` text from the VTerm snapshot (`internal/termui/termpane/termpane.go:220-230, 563-580`). It returns styled content without absolute CUP positioning, which is the appropriate representation for compositor layers.

The launcher sends that string and a generation to the compositor (`ai-tool.js:1428-1437`). `UpdatePaneIfNew` only replaces cached content when the generation changes (`internal/termui/compositor/compositor.go:99-115`); it does not add SGR styling. `RenderCapture` emits SGR transitions from each screen cell's stored `Attr` (`internal/termmux/vt/render.go:104-133`). Therefore, underlines in the final pane must come from underlined VTerm cells or from a downstream renderer interpreting their SGR representation; the JavaScript launcher does not manufacture them.

## Pubsub change: output notifications, not terminal parsing

The user's onset report points to the Oct. 1 pubsub/output refactor (`c6e15d1`). The diff confirms a real redraw-path change: `EventSessionOutput` carrying raw bytes was removed from `EventBus`; after `handleSessionOutput` writes the bytes to `VTerm`, captures the screen, and stores the snapshot, it now calls `outputSignals.mark(sessionID)`. The old and new paths therefore keep the same byte-to-VTerm and snapshot ordering; the changed part is how consumers learn that a snapshot is ready.

The follow-up commits touched notification and subscriber lifecycle only. `0c597df` briefly moved watcher notification outside the lock using a reusable scratch slice; `4c2ae87` removed that scratch reuse and restored signaling under the lock. These changes can affect which watcher receives a redraw signal, but neither writes to a VTerm, screen cell, or `Attr.Under`.

For this launcher, a `PaneOutput` calls `updateCompositor()`, and the 100 ms tick also calls it (`ai-tool.js:1510-1536`). Each compositor update calls `pane.view()`, whose Go binding calls `RefreshSnapshot()` before `ANSIView()` (`internal/builtin/termui/termpane/termpane.go:315-329`). A lost/coalesced wake-up can delay an intermediate render; the next tick reads the latest published snapshot. It cannot account for underline attributes in that snapshot.

## Private CSI ModifyOtherKeys prefix is misparsed as SGR

The supplied PTY transcript contains four `CSI >4;2m` and four `CSI >4m` sequences. XTerm specifies `CSI >4;Pv m` for ModifyOtherKeys configuration; it changes how modified keypresses are reported, not text rendition ([XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#ModifyOtherKeys)).

The parser stores `>` in its CSI prefix/intermediate buffer (`internal/termmux/vt/parser.go:174-181`). `VTerm.Write` marks only `?` as `isPrivate`, but also provides a `HasInterGt` callback to the CSI handler (`internal/termmux/vt/vt.go:82-98, 201-204`). The callback is used for device-attribute handling, but the final-`m` branch in `CSIHandler.Dispatch` does not inspect it (`internal/termmux/vt/csi.go:82-84, 291-324`). It therefore parses `CSI >4;2m` as SGR parameters `[4, 2]` and `CSI >4m` as `[4]`; `ParseSGR` sets underline for code 4 and dim for code 2 (`internal/termmux/vt/sgr.go:107-118`).

The raw transcript contains no ordinary SGR code 24 (underline off) or 0 (full reset). Thus, once these private sequences reach the SGR handler, the current rendition's underline bit remains set for subsequently written cells. `RenderCapture` serializes those cell attributes as ordinary SGR transitions (`internal/termmux/vt/render.go:104-145`), explaining why pane snapshots show `4m` even though Claude's raw stream contains no ordinary underline-on SGR. The output was not produced by the JS launcher.

## VT SGR path

`VTerm` connects its parser's colon-separated sub-parameters to the CSI handler (`internal/termmux/vt/vt.go:99-100`; `internal/termmux/vt/parser.go:357-366`). For an SGR sequence containing a colon group, `CSIHandler.Dispatch` routes the grouped parameters to `ParseSGRWithSubParams` (`internal/termmux/vt/csi.go:291-312`). That function handles sub-parameters specially for extended colors, but for other codes it retains only the first value (`internal/termmux/vt/sgr.go:197-236`).

The resulting `Attr.Under` is attached to cells when characters are written (`internal/termmux/vt/screen.go:1450-1504`). `RenderCapture` then emits `CSI 4 m` when entering an underlined cell and a reset when returning to default attributes. This matches the form seen in the capture, but the renderer has normalized the original input; it cannot be used to infer whether the child sent `4`, `4:0`, or another underline form.

The confirmed `4:0` behavior is a separate parser defect: the supplied raw transcript contains no colon-form SGR, so it is not the trigger in this trace.
