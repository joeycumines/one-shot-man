# Mangled terminal-state analysis

**Verdict:** the submitted PTY trace contains four `CSI >4;2m` and four `CSI >4m` sequences from Claude. XTerm defines these as ModifyOtherKeys controls, not SGR ([XTerm Control Sequences](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html#ModifyOtherKeys)). OSM's VTerm stores the `>` prefix but its `m` handler ignores it and parses the parameters as ordinary SGR: `4;2` and `4` both enable underline. The trace has no ordinary SGR `0` or `24` code to clear it. This is the direct root cause for the mangled run, assuming the supplied trace is from the run shown in the screenshots.

The earlier `4:0` colon-SGR defect is real but absent from this trace. Pubsub changes match the user's regression timeline, but the source changes redraw notifications, not byte parsing or cell attributes; they do not explain how the private CSI sequences became underline.

**Separate observation:** startup text resembling `^[]10;rgb:...^G^[]11;rgb:...^G` matches OSC 10/11 replies that OSM sends into the child PTY. Early PTY echo is a likely explanation; this sequence contains no underline SGR and predates the underline symptom.

| Document | Purpose |
|---|---|
| [01_capture_findings.md](01_capture_findings.md) | What the capture does and does not establish |
| [02_dataflow.md](02_dataflow.md) | Static path from `ai-tool.js` through the VT renderer |
| [03_root_cause.md](03_root_cause.md) | Confirmed parser defect, pubsub history, and incident attribution |
| [04_conclusions.md](04_conclusions.md) | Honesty matrix, alternatives, and confidence limits |

**Evidence bundle:** `pty-trace-w5a69ukw/` is a copy of the complete supplied trace directory, including its README, `bin/claude` shim, and raw `events-55539.jsonl`. `captures/` contains the three compared ANSI captures plus the related raw-output and example captures. `resources/` contains the related `repro-ai-tool`, `startup`, `osc-ab`, and `ab1.png` scratch resources, plus the `hack/record-pty.py` recorder used to capture PTY traffic.

**Reading order:** 01 → 02 → 03 → 04. Read 03 first for the proposed root cause; read 01 and 02 to check its evidence chain.

**Scope and method:** analyzed the supplied raw PTY trace, the prior pane captures, the in-scope `/Users/joeyc/dev/joeyc-ai/osm/ai-tool.js` launcher and `osm/ai-lib/terminal-theme.js`, and the connected VT, pane, PTY, and compositor code. No live session or application was launched, no test was run, and no Go or JavaScript implementation file was changed.

**Caveats:** the trace records PTY I/O, not OSM's in-memory cell snapshots; linking it to the screenshot depends on it being the same mangled run. The direct baseline's `TERM` remains unknown, but that does not affect the demonstrated parser failure for the actual `CSI >4...m` bytes in this trace.
