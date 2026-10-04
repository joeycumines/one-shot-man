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
