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

## The deviation

`epollCtl` in `cancelreader_linux.go` retries the call while the kernel
reports `EINTR` — the same policy upstream already applies to `EpollWait`.
Nothing else is changed behaviorally: the three `Close` paths also build
their joined error with `errors.New` instead of `fmt.Errorf` (upstream's
code trips the modern vet non-constant-format check; the repo bar is
vet-clean on every module). Upstream's own test suite is carried and runs
against the fork in the gates. If upstream ships the retry, delete this
fork and the `replace` directive.

## Upstream

Issue not yet filed upstream (checked 2026-10-05: no existing issue or PR
mentions EINTR on EpollCtl). Filing it is a courtesy follow-up, not a
prerequisite for carrying this fork — the defect is real, reproducible
under signal load, and the patch is the standard EINTR-retry idiom.
