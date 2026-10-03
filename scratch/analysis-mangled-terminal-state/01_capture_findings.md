# Capture findings

## Separate startup output reported by the user

The user also reports startup output that looks like `^[]10;rgb:ffff/ffff/ffff^G^[]11;rgb:ffff/ffff/ffff^G`, and says it predates the underline symptom. This is recorded as a separate observation, not as evidence for the underline diagnosis. The source constructs exactly those OSC 10/11 reply sequences; see the handshake path in 02.

## What is in the file

`scratch/borked-output.ansi` is a short styled screen snapshot: 1,518 bytes, 25 newline-delimited rows, and a trailing newline. Static parsing finds 131 CSI sequences, all SGR sequences; there is no CUP (`CSI row;column H`) or other cursor-motion sequence in the file. The dump has 38 `CSI 4 m` sequences and 39 `CSI 0 m` sequences.

The visible text remains legible and ordered. The symptom is in cell styling: underline is repeatedly present on text, including the assistant prose and the active UI/status region. Row 1 begins with underline and a white background SGR despite having no printable text in the dump; because capture output trims trailing blanks, this establishes styled blank cells but not that every cell in that row has those attributes.

Representative raw spans in rows 13–16 alternate `CSI 4 m` and `CSI 0 m` around words. This is not merely a terminal-wide underline mode left active in the serialized file. The pane state contains many cells that are underlined and cells that are not. The file alone cannot say whether those attributes were intended by Claude Code or produced by the VT parser.

The supplied screenshot of this same ANSI file confirms that the underlining is visibly rendered: the body text and UI labels have underline strokes, rather than the appearance being limited to escape-code text or an ANSI viewer. It does not add the child PTY bytes, so it confirms the symptom but not which layer introduced the cell attributes.

## Direct baseline comparison

`scratch/approximate-baseline.ansi` is a 1,327-byte, 25-row capture of the same resumed session, at approximately the same terminal size, opened directly in Claude Code. Its shared text includes the skill list, the 28-second and 21-second activity lines, and the beginning of the same grounding-audit response. In those corresponding rows, the direct baseline has no underline SGR at all.

The original OSM capture has 38 standalone `CSI 4 m`, 10 `CSI 0;4 m`, and one `CSI 1;4 m` sequence (49 sequences that enable underline), plus 39 `CSI 0 m` resets. The baseline has zero underline-on sequences. The screenshot makes the difference visible: the OSM version underlines body text and UI labels that are plain in the direct baseline. This is strong evidence that the difference is introduced by the OSM launch/terminal path, not a stable Claude UI style for that content.

This comparison establishes the output difference between the direct and OSM runs, but not which OSM layer caused it. Both are downstream pane snapshots, the content is approximately aligned rather than byte-identical, and the OSM PTY changes `TERM` to `xterm-256color` by default (see 02). Claude may therefore emit different bytes in the two runs.

## Same-session OSM re-replication

The additional `scratch/same-session-re-replicate-via-resume.ansi` capture reproduces the underline corruption through OSM using the same resume methodology as the direct Claude baseline. It is 1,494 bytes and 25 rows, with 111 SGR sequences; 38 SGR codes enable underline (26 `4m`, 11 `0;4m`, and one `2;4m`). The direct baseline has zero underline-on codes. After removing ANSI sequences and trailing whitespace, the new OSM capture and direct baseline have 16 identical non-empty rows, including the same activity line, assistant content, and TUI labels.

This repeat makes a one-off partial redraw or unique session state a weaker explanation. It localizes the persistent corruption to OSM's run path, but by itself cannot distinguish a VTerm misparse from output Claude selects under OSM's terminal description. The subsequently supplied PTY trace provides the missing child-output bytes.

## Raw child PTY trace

The supplied `/var/folders/_r/v0qs308n49952w5gddyqznbw0000gn/T/pty-trace-w5a69ukw/events-55539.jsonl` has 51 events, including 4,620 bytes from the `claude` command and 276 bytes sent to it. The recorded environment has `TERM=xterm-256color`. Its child-output stream contains four `CSI >4;2m` sequences and four `CSI >4m` sequences. It contains no ordinary SGR code `4`, `0`, or `24`, and no colon-form SGR. (The `24` in color sequences such as `38;5;24` is a palette index, not an SGR underline-off code.)

XTerm documents `CSI >4;Pv m` as ModifyOtherKeys configuration, not graphic rendition ([XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#ModifyOtherKeys)). In OSM, the `>` prefix is retained by the parser but the final `m` is dispatched through SGR without checking that prefix. The parameter lists `[4, 2]` and `[4]` consequently set `Attr.Under`; no ordinary SGR `24` or `0` in this trace clears it. This is direct, run-specific evidence for the underline mechanism, provided this trace corresponds to the visibly mangled run.

The trace also records one OSC 10 and one OSC 11 reply sent to the child, consistent with the separate startup-output observation described in 02. Those OSC replies do not set underline.

## What the capture command means

The supplied command is `tmux capture-pane -p -e -t 8` (the user's compact spelling was `-pet 8`). `-p` prints pane contents and `-e` includes SGR styling. It serializes the selected pane's current screen cells. It is not a recording of bytes written by Claude Code to its PTY.

That distinction matters here. The SGRs in this file describe the final cell attributes after the inner PTY/VTerm/render/compositor path and the outer tmux have handled the output. The dump cannot identify whether Claude emitted a particular SGR form, whether the inner parser normalized it incorrectly, or how it changed over time.

## Cursor observation

The user's observation is that the cursor issue has not returned. This artifact cannot independently confirm cursor position: it is a cell-state dump, and it contains no cursor-position sequence or cursor metadata. Nothing in this capture points to cursor movement as the cause of the underlines.
