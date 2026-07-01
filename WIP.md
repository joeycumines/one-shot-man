# WIP

## Session: 2026-07-01 — Address scratch/review-01..03.md (DONE)

All review findings addressed. 18 fixes across 15 files. All three platforms pass.

## Changes

### Review-01..03 fixes (7):
1. pr_split_06_verification.js — .catch() guarded with typeof .then
2. go.mod — go mod tidy removed dop251 direct deps
3. prsplittest/eval.go — globals deleted after settle
4. pty_windows.go — removed STARTF_USESTDHANDLES
5. pr_split_16b — startAnalysis async [s,cmd] return; handleConfigError delegates to processConfigResult; handleConfigState dynamic dispatch
6. pr_split_16d — startAutoAnalysis async [s,cmd]; handleAutoConfigError delegates; dynamic dispatch
7. pr_split_16_async_pipeline_test.go — 4 original + 2 new rejection tests

### Flaky test / data race fixes (5):
8. pty.go — Close() closes slave tty BEFORE grace/force loop (macOS E-state fix; surgical: only tty, wf stays)
9. bubblezone_test.go — waitForZone deadline+sleep (Gosched starvation fix) + RescanUpdatesZones
10. engine_core.go — removed e.vm=nil + e.scripts=nil (DATA RACE); atomic.Bool closed idempotency; executeOnLoop local vm capture; e.scripts protected with globalsMu + slices.Clone
11. manager_exited_pane_test.go — Subscribe before close(readerCh) (TOCTOU fix)
12. passthrough_test.go — echo→sleep 5 (Linux race: echo exited before toggle key read)

### Missing links / regression fixes (4):
13. pr_split_14b_tui_commands_ext.js — handleConfigState dynamic dispatch (same bug as 16b/16d)
14. pr_split_16b — split-view cleanup on config error (splitViewOpenedHere tracking)
15. remain_on_exit_test.go — Subscribe before close (3 instances, same TOCTOU)
16. load_test.go — Gosched busy-waits replaced with time.Sleep (4 instances)

### Adversarial autopsy findings:
- No test weakening found (all assertions intact, no tests skipped/removed)
- Stale comments fixed (references to removed e.vm=nil, line numbers)

## Verification
- macOS: make all PASS (EXIT=0)
- Windows: make all PASS (EXIT=0)
- Linux: make all PASS (EXIT=0)
- gmake lint PASS (vet+staticcheck+deadcode)
- Rule of Two: multiple rounds, all PASS
- Scratch files cleaned (only original review-01..03.md remain)
