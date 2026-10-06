# cancelreader fork

A local fork of [github.com/muesli/cancelreader](https://github.com/muesli/cancelreader)
v0.2.2 (MIT, © 2022 Erik Geiser and Christian Muehlhaeuser — see LICENSE),
wired in through a `replace` directive in the root go.mod.

## Why

Upstream `NewReader` on Linux registers the reader and the cancel-signal
pipe with `unix.EpollCtl(EPOLL_CTL_ADD)` and treats any error as fatal.
`EpollCtl` returns `EINTR` when a caught signal (SIGWINCH, or the Go
runtime's SIGURG preemption signal) lands during the call, so a program's
startup could fail with "add reader to epoll interest list" purely because
a signal arrived at the wrong microsecond. Upstream retries `EINTR` around
`EpollWait` but not `EpollCtl`, and the file has not been touched upstream
since 2022 (v0.2.2 is the newest release), so the fix cannot come from a
version bump.

The failure surfaced as `TestJSScriptCommand_Execute_SigtermWithoutListenerRunningProgram`
failing in the Linux container gate ("could not create cancelable reader"),
first with the test's own SIGTERM racing startup, then with ambient signal
delivery. In production the same race can kill an `osm script` run that
uses a bubbletea TUI.

## The deviations

Two, both in `cancelreader_linux.go`:

1. `epollCtl` retries the call while the kernel reports `EINTR` — the same
   policy upstream already applies to `EpollWait`. A caught signal
   (SIGTERM at the wrong microsecond, or the Go runtime's SIGURG
   preemption under load) interrupted the registration and killed
   program startup.

2. `NewReader` falls back to the non-canceling reader when the reader fd
   answers `EPOLL_CTL_ADD` with `EPERM` — the documented error for a fd
   with no poll backing. `/dev/null` is the common case (probed in a
   golang:1.27.0 container: char device, `epoll_ctl` → EPERM). Upstream's
   own dispatch falls back only for non-`File` readers, so a
   File-backed /dev/null stdin (the shape every `go test` / pipeline /
   container run has) could never start a program. The fallback degrades
   to no in-flight-read cancelation, which is harmless for a source that
   never delivers data.

Behaviorally nothing else is changed: the three `Close` paths build
their joined error with `errors.New` instead of `fmt.Errorf` (upstream's
code trips the modern vet non-constant-format check; the repo bar is
vet-clean on every module) and the Windows path uses `syscall.SyscallN`
instead of the deprecated `syscall.Syscall` (staticcheck SA1019).
Upstream's own test suite is carried and runs against the fork in the
gates. If upstream ships the EINTR retry and the EPERM fallback, delete
this fork and the `replace` directive.

## Upstream

Issue not yet filed upstream (checked 2026-10-05: no existing issue or PR
mentions EINTR on EpollCtl). Filing it is a courtesy follow-up, not a
prerequisite for carrying this fork — the defect is real, reproducible
under signal load, and the patch is the standard EINTR-retry idiom.

## Upstream open-PR triage (2026-10-06)

All 9 open PRs on muesli/cancelreader were inspected at diff level and
cross-referenced against this fork. No upstream PR overlaps our two
functional fixes (EINTR retry, EPERM fallback).

### Overlaps with this fork

| PR | Overlap |
|----|---------|
| #26 — Don't leak cancel signal pipe fds | Direct. Upstream closes only the epoll fd on both EpollCtl error paths, leaking the pipe. This fork carries both paths unchanged AND the EPERM fallback path adds a third leak instance (closes epoll but not cancelSignalReader/cancelSignalWriter). |
| #13 — Improve epoll error messages | Partial. Touches the same two EpollCtl error sites this fork routed through `epollCtl`; our messages are still the bare upstream string. |

### Want to adopt

| PR | Priority | Why |
|----|----------|-----|
| #26 | HIGH | Real fd leak; 3 sites in this fork. 2-line fix per site. |
| #28 (draft) | HIGH | `readAsync` leaks a CreateEvent handle per read, swallows GetOverlappedResult errors (returns nil), and the error path skips `<-r.blockingReadSignal`, permanently occupying the signal channel. This fork's readAsync is byte-identical to upstream. |
| #8 | HIGH | Windows dispatch guards only on `f.Fd() != os.Stdin.Fd()`, then unconditionally opens CONIN$. With piped stdin it either fails (no console) or silently reads the console instead of the pipe. The GetConsoleMode probe + fallback is the Windows analog of our Linux EPERM fix. |
| #27 (draft) | MEDIUM | Fallback reader returns (0, ErrCanceled) after consuming bytes in a cancel race. Matters more here since the EPERM fix routes more traffic to the fallback reader. |
| #13 | LOW | fd numbers + %w wrapping in the two error messages this fork still ships bare; early -1 fd check. Compatible with the epollCtl wrapper. |

### Skip

| PR | Why |
|----|-----|
| #21 | Behavioral change: flips ENABLE_PROCESSED_INPUT. Alters signal delivery semantics for bubbletea/ultraviolet on Windows. Design decision, not a defect fix. |
| #24 | Renames module to github.com/abakum/cancelreader (breaks the replace directive), removes prepareConsole entirely, adds three external deps. Not mergeable. |
| #25 | Comment typo. |
| #19 | Dependabot x/sys → 0.9.0; this fork is on v0.47.0. |

### Retraction

An earlier rationale for PR #8 said "our CI/tests run on Windows with
redirected stdin." That was imprecise: the fork's pipe/cancel test
(cancelreader_default_test.go) is `//go:build !windows`, and
TestReaderNonFile uses a non-File reader, so nothing in the fork
exercises the Windows stdin path. The verified relevance of #8 is the
CONIN$-vs-pipe dispatch gap, not existing test coverage.
