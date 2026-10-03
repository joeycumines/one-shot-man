# Root cause

## Incident mechanism: ModifyOtherKeys CSI is parsed as SGR

**Status:** confirmed in the supplied PTY trace and in the OSM parser path.

The trace records four `CSI >4;2m` and four `CSI >4m` sequences in 4,620 bytes emitted by the `claude` child. XTerm documents `CSI >4;Pv m` as ModifyOtherKeys configuration, not SGR ([XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#ModifyOtherKeys)). The two observed forms configure level 2 and restore the terminal's default ModifyOtherKeys setting, respectively.

The parser preserves `>` in the CSI prefix buffer (`internal/termmux/vt/parser.go:174-181`). `VTerm.Write` supplies a callback for detecting `>` but its `isPrivate` flag only represents `?` (`internal/termmux/vt/vt.go:90-92, 201-204`). While other CSI handling can consult the prefix callback, the final-`m` case does not; it passes the numeric parameters to `ParseSGR` (`internal/termmux/vt/csi.go:82-84, 291-324`).

As a result, `CSI >4;2m` is misread as SGR `[4, 2]` and `CSI >4m` as SGR `[4]`. SGR code 4 sets `Attr.Under = true`; code 2 also sets dim (`internal/termmux/vt/sgr.go:107-118`). The trace has no ordinary SGR 0 or 24 code, so it does not clear underline after these sequences.

That is the direct explanation for the underlined VTerm cells. `RenderCapture` serializes the resulting cell attributes as ordinary SGR transitions (`internal/termmux/vt/render.go:104-145`), so the pane snapshots show `4m` even though the trace contains no ordinary underline-on SGR. The JavaScript launcher passes the pane view through to the compositor; it does not add underline formatting.

The trace does not itself contain OSM's cell snapshots. Assuming it is from the run shown in the supplied mangled-output screenshot, this establishes the incident trigger; independently, it proves that the OSM VT parser treats the exact child-output sequences as underline-on.

## Separate confirmed defect: colon underline “off” is parsed as “on”

This independent source defect remains confirmed — `internal/termmux/vt/sgr.go:197-236` — but does not appear in the supplied trace, which has no colon-form SGR.

The CSI parser preserves colon groups, and `csi.go` selects `ParseSGRWithSubParams` when a group has multiple values (`parser.go:357-366`; `csi.go:291-312`). That function handles extended color groups specially, but flattens other groups to their first value. Thus `CSI 4:0m` becomes SGR `[4]`: `ParseSGR` sets `Under = true` for code 4, while code 24 is the supported underline-off operation (`sgr.go:114,131`). `4:` has the same risk because the parser represents a trailing empty sub-parameter as zero.

This behavior entered with `a2fe4cc` (`feat(vt): ... colon SGR`, 2026-08-30), which added generic flattening while implementing colon truecolor support. It can independently cause stuck underlining if a child emits `4:0`, but the supplied trace rules it out as the trigger for this run.

The separate erased-blank defect was addressed in `a63eb84` (`vt: erase to background only, not to the whole pen`, 2026-10-03). Current erase operations construct blank cells from only the current background (`screen.go:324-334` and erase/scroll call sites). That defect can produce underlined blank cells and horizontal rules; it is not the best explanation for the raw private-CSI sequence or the underlined glyph cells here.

### Pubsub timeline: what changed and what did not

The user's observation that the issue dates from the pubsub changes is important for locating the regression window. The relevant change is `c6e15d1` (2026-10-01): it removed `EventSessionOutput` from the EventBus and introduced `outputSignals`/`OutputWatcher` for redraw notifications. In both the parent and current code, `handleSessionOutput` writes the same `so.data` to `ms.vterm`, captures the screen, and stores the snapshot before notifying the pane. The notification changed; the SGR parser and the attributes stored in cells did not.

The follow-up output-watch commits also do not set styles. `0c597df` introduced scratch-slice reuse while sending watcher notifications outside the lock; `4c2ae87` removed that reuse and sends under the lock. The intermediate version could race over watcher targets and miss or misdirect wake-ups, but the current HEAD includes the correction. At most, such a defect changes redraw timing or which pane wakes. It cannot create `Attr.Under`.

In the current launcher, both a `PaneOutput` and each 100 ms `Tick` call `updateCompositor()` (`ai-tool.js:1510-1536`). That calls `pane.view()`, which refreshes the Go pane from the latest screen snapshot before returning ANSI content (`internal/builtin/termui/termpane/termpane.go:315-329`). A pubsub notification loss may postpone an intermediate frame until the next tick; it does not explain persistent underlined glyphs in a later captured pane state.

The pubsub timing remains a valid regression clue, but the trace and parser path identify a different direct styling mechanism: the final-`m` handler applies SGR to `CSI >4;2m` and `CSI >4m`. The pubsub diffs do not alter that parser path or set cell attributes. They may be temporally related to when the behavior became visible, but this evidence does not identify a pubsub change as the cause.

### Separate startup output: OSC reply echo

The user reports startup text in caret notation corresponding to OSC 10/11 color replies, and says it predates the underline issue. The OSM script builds and sends those exact reply bytes into the child PTY; the Unix PTY setup does not disable echo or wait for the child to enter raw mode (see 02_dataflow.md). If the PTY still echoes control characters when the asynchronous background-color response arrives, this explains the visible `^[]10...^G^[]11...^G` text.

This is a separate likely startup echo/leak. The OSC replies contain no SGR underline, and the user explicitly dates this output before the underline symptom. It is not the underline root cause.

## Incident attribution: exact parser failure identified

The direct baseline has no underline-on SGR in shared content, while both OSM pane captures contain many underlined cells. More importantly, the raw child transcript now shows the exact input that sets underline in the OSM VTerm: eight xterm ModifyOtherKeys sequences incorrectly dispatched as SGR. The OSM-run path is responsible for that misrendering.

The raw trace records `TERM=xterm-256color`, the eight private `m` sequences, and no ordinary SGR underline-on/off or full-reset codes. The direct baseline's `TERM` is still unknown, so the captures alone cannot explain why Claude emitted these commands only in the OSM run. That environment difference does not change the parser contract: OSM must not interpret a private `>` control as graphic rendition.

The OSM terminal path needs to be considered as a pair: it advertises an xterm terminal to the child and implements that terminal in a custom VTerm. The parser must handle the SGR forms a child can legitimately emit under that terminal description. Merely changing `TERM` to match the direct baseline would mask a compatibility gap rather than repair it.

## Why the existing underline tests do not clear this defect

The existing CSI tests cover `>` for device attributes, but the SGR branch does not test or reject that prefix. A regression test should feed `CSI >4;2m` and `CSI >4m` through `VTerm.Write` and verify they do not set underline or dim. Tests should also verify ordinary `CSI 4m` still enables underline and `CSI 24m` clears it.

`sgr_test.go` tests simple `4` and `24` forms (`:49-85`), not colon-form underline style. The current tests therefore establish ordinary underline set/clear behavior but do not exercise `[4, 0]` through `ParseSGRWithSubParams`.

`underline_gap_test.go` tests the separate case where an application intentionally underlines words while leaving blank gaps untouched. `erase_attr_test.go` covers erase operations inheriting underline from the active pen. Those tests do not cover `4:0`. The erase implementation currently uses `eraseAttr`, which preserves only the background (`screen.go:324-334`) and is not the best explanation for this capture's word-cell attributes.

## Correction targets

For the incident trigger, keep CSI private-prefix commands out of SGR dispatch. Either implement ModifyOtherKeys mode handling or ignore the `>`-prefixed `m` command without mutating graphic rendition. Add the VTerm regression cases above.

The separate colon-SGR defect should receive its own fix and tests: handle underline sub-parameters explicitly (`4:0` clears; `4:1` through `4:5` set the boolean underline attribute), preserve existing extended-color behavior, and verify a VTerm sequence of `4:1`, `4:0`, then plain text leaves the later cells un-underlined.
