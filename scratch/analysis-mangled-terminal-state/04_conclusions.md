# Conclusions

## Honesty matrix

| Finding | Classification | Evidence and limit |
|---|---|---|
| The pane visibly renders many underlined cells. | **TRUE** | The ANSI dump has 38 `CSI 4 m` sequences, and the supplied screenshot shows the same underlined body text and UI. This describes rendered cells, not the child's original bytes. |
| The OSM output differs from the direct baseline for overlapping content. | **TRUE** | The baseline has no underline-on SGR in shared content; the original OSM capture has 49, and the same-session re-replication has 38. The new OSM capture shares 16 exact non-empty text rows with the baseline. |
| The supplied child PTY trace contains the sequences that trigger the underline. | **TRUE** | It records four `CSI >4;2m` and four `CSI >4m` sequences, with no ordinary SGR 4, 0, or 24 code. |
| XTerm defines `CSI >4;Pv m` as ModifyOtherKeys, not SGR. | **TRUE** | [XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#ModifyOtherKeys). |
| OSM's VTerm misparses the trace's private `m` sequences as SGR. | **TRUE** | `CSIHandler.Dispatch` routes final `m` to `ParseSGR` without checking `HasInterGt`; parameter 4 sets `Attr.Under` (`csi.go:291-324`; `sgr.go:107-118`). |
| The raw trace clears underline after the private sequences. | **FALSE** | It contains no ordinary SGR 24 (underline off) or 0 (full reset). |
| The VT parser misreads colon-form underline reset `4:0` as underline-on. | **TRUE, separate defect** | `ParseSGRWithSubParams` discards non-color sub-parameters (`sgr.go:197-236`); `ParseSGR` maps 4 to true and 24 to false (`sgr.go:114,131`). The supplied trace has no colon SGR. |
| The user's regression window starts with pubsub changes/fixes. | **USER-REPORTED** | The report is accepted as a timing observation. Repository history identifies `c6e15d1` (Oct. 1) and follow-up watcher fixes, but source history cannot verify when the symptom first appeared. |
| The pubsub refactor changed the VT parser or cell attributes. | **FALSE** | `c6e15d1` replaces `EventSessionOutput` delivery with `outputSignals.mark` after the unchanged `VTerm.Write` and snapshot store; no VT parser or cell-attribute code changed. |
| Pubsub notification loss can set underline on cells. | **FALSE** | The output watcher only sends empty wake-up tokens. It does not write to VTerm or cell attributes (`output_watch.go`; `manager.go:4129-4138`). |
| The OSM VTerm caused the underlines in the traced run. | **TRUE, assuming the trace is from the mangled run** | The raw input contains the exact sequences that the source path applies as SGR underline-on, and the pane captures show the resulting symptom. The trace itself does not contain the OSM cell snapshot. |
| `ai-tool.js` directly adds underline formatting. | **FALSE** | It passes `termpane.view().content` to the compositor; no launcher code in the inspected path styles the child's text (`ai-tool.js:1428-1437`; binding at `termpane.go:289-307`). |
| The startup `^[]10...^G^[]11...^G` output is from OSC replies sent by OSM. | **STRONGLY SUPPORTED** | `terminal-theme.js` constructs those exact OSC 10/11 replies and `ai-tool.js` writes them through `session.write`. The PTY can echo them in caret notation while control-character echo is enabled, but that runtime termios state is not present in the static artifacts. |
| The startup OSC text caused the underline issue. | **FALSE** | The reply strings contain OSC 10/11, not SGR underline; the user reports this startup output predates the underline symptom. |
| This is the older underline-on-erase defect. | **Not supported** | The current erase helper strips all rendition except background, and erase tests cover this behavior (`screen.go:324-334`; `erase_attr_test.go`). The capture's word-cell pattern is more directly explained by SGR parsing or child styles. |
| The cursor issue is present in this capture. | **UNCERTAIN** | User reports it has not resurfaced. The file contains no cursor metadata, so it cannot independently confirm cursor visibility or position. |

## Ranked diagnosis

**Confirmed root cause — HIGH:** the raw trace records eight `CSI >4...m` ModifyOtherKeys commands. OSM's final-`m` handler ignores the `>` prefix and interprets the numeric parameters as SGR, setting `Attr.Under` (and, for `>4;2m`, dim). No ordinary SGR 24 or 0 clears underline in the trace. This is conclusive for the trace; it is the cause of the reported pane corruption if this was the visibly mangled run.

**Separate confirmed defect:** colon-form `4:0` is still incorrectly flattened to `4`, but no colon SGR appears in the trace. It is not the trigger for this run.

**Pubsub finding:** the Oct. 1 change and its follow-ups altered output notification and redraw scheduling, not PTY-byte parsing or cell attributes. The current `ai-tool.js` also re-reads the snapshot on every compositor update and has a 100 ms tick backstop. Pubsub may be temporally related, but it does not explain the parser's misinterpretation of the captured commands.

**Separate startup issue — likely OSC reply echo:** the reported control-text exactly matches the OSC 10/11 reply constructed in `ai-lib/terminal-theme.js` and sent into the child PTY. The PTY setup does not disable echo or wait for the child to enter raw mode, so early replies can be reflected as caret notation. This is distinct from the underline defect and is not supported as its cause.

**Incident-specific confidence — high:** the two OSM captures show underlines where shared direct-baseline content is plain, and the supplied raw trace shows the exact private CSI sequences that OSM mistakenly turns into underline. The direct baseline's `TERM` is unknown, so it remains possible Claude selected these commands only under OSM's xterm environment. That does not weaken the confirmed OSM parser bug.

## Alternatives and non-causes

- **Intentional underline from Claude:** unsupported by the raw trace. The only raw underline trigger is a private ModifyOtherKeys configuration sequence, not an ordinary SGR underline command.
- **Terminal-description difference:** confirmed confounder. The OSM PTY defaults to `xterm-256color`; the direct baseline's `TERM` was not captured. This may affect Claude's output, but does not excuse the emulator's incorrect handling of valid SGR.
- **Erase attribute inheritance:** a real class of bug, but not the strongest explanation here. Current `eraseAttr` retains only BG; a separate colon-parameter parser path is uncovered.
- **Startup resize:** `ai-tool.js` creates the PTY at 22x80 before resizing through `termpane` (`ai-tool.js:1417-1425, 1597-1601`). This could cause a transient repaint/geometry artifact, but does not explain persistent `Under` bits in cells.
- **Cursor addressing/compositor clipping:** the JavaScript binding selects `ANSIView`, specifically the compositable representation without CUP sequences (`termpane.go:563-580`; binding at `builtin/termui/termpane/termpane.go:296`). The dump gives no evidence for this as the underline cause.
- **Pubsub output wake-ups:** these can coalesce or delay redraws, but the current launcher refreshes from the authoritative snapshot on PaneOutput and each 100 ms tick. They do not mutate the underlying underline attributes.
- **OSC 10/11 startup replies:** likely explanation for the separate caret-notation startup text, but the sequences contain no underline SGR and the user reports they predate the underline issue.

## What would settle attribution

The raw child PTY transcript has now settled the parser attribution for the traced run. The remaining limitation is that the trace does not bundle the exact OSM cell snapshot; correlating it to the screenshot assumes it is the same mangled session. No implementation source was changed during this analysis.
