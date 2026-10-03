#!/usr/bin/env python3
"""Record raw PTY traffic for a command or a matching child process.

Usage:
    python3 hack/record-pty.py -- COMMAND [ARGS...]
    python3 hack/record-pty.py --shim NAME -- OUTER_COMMAND [ARGS...]

The trace contains the exact bytes in both directions, terminal resizes, and
process lifecycle events. It can include private input and output.
"""

import base64
import errno
import fcntl
import json
import os
import pty
import select
import shutil
import shlex
import signal
import struct
import sys
import tempfile
import termios
import time
import tty
from pathlib import Path

TRACE_DIR_ENV = "OSM_DIAG_PTY_TRACE_DIR"
REAL_COMMAND_ENV = "OSM_DIAG_PTY_REAL_COMMAND"
SAFE_ENV_NAMES = (
    "TERM",
    "COLORTERM",
    "LANG",
    "LC_ALL",
    "LC_CTYPE",
    "TERM_PROGRAM",
    "TERM_PROGRAM_VERSION",
    "LINES",
    "COLUMNS",
)


class Trace:
    def __init__(self, file):
        self.file = file
        self.seq = 0

    def emit(self, event, **fields):
        record = {
            "seq": self.seq,
            "monotonic_ns": time.monotonic_ns(),
            "unix_ns": time.time_ns(),
            "event": event,
        }
        record.update(fields)
        self.file.write(json.dumps(record, separators=(",", ":")) + "\n")
        self.file.flush()
        self.seq += 1


def terminal_size(fd):
    try:
        raw = fcntl.ioctl(fd, termios.TIOCGWINSZ, b"\0" * 8)
    except OSError:
        return None
    rows, cols, _, _ = struct.unpack("HHHH", raw)
    if rows == 0 or cols == 0:
        return None
    return rows, cols


def set_terminal_size(fd, size):
    if size is None:
        return
    rows, cols = size
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))


def write_all(fd, data):
    offset = 0
    while offset < len(data):
        written = os.write(fd, data[offset:])
        if written == 0:
            raise OSError("terminal write made no progress")
        offset += written


def wait_status_fields(status):
    if os.WIFEXITED(status):
        return {"exit_code": os.WEXITSTATUS(status)}
    if os.WIFSIGNALED(status):
        signum = os.WTERMSIG(status)
        try:
            return {"signal": signal.Signals(signum).name, "signal_number": signum}
        except ValueError:
            return {"signal_number": signum}
    return {"wait_status": status}


def option_names(args):
    names = []
    for arg in args:
        if arg.startswith("--"):
            names.append(arg.split("=", 1)[0])
        elif arg.startswith("-") and len(arg) > 1:
            names.append(arg[:2])
    return names


def open_trace(trace_dir):
    trace_dir = Path(trace_dir)
    if not trace_dir.is_dir():
        raise OSError("trace directory is missing")
    path = trace_dir / ("events-" + str(os.getpid()) + ".jsonl")
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    flags |= getattr(os, "O_NOFOLLOW", 0)
    fd = os.open(path, flags, 0o600)
    return os.fdopen(fd, "w", encoding="utf-8", buffering=1)


def run_pty_proxy(real_command, command_argv, trace_dir):
    try:
        trace_file = open_trace(trace_dir)
    except OSError as err:
        print("PTY recorder: cannot create trace: " + str(err), file=sys.stderr)
        return 1

    trace = Trace(trace_file)
    stdin_fd = sys.stdin.fileno()
    stdout_fd = sys.stdout.fileno()
    outer_size = terminal_size(stdin_fd)
    old_termios = None
    old_handlers = {}
    child_pid = 0
    master_fd = -1
    child_status = None
    master_open = False
    stdin_open = True
    pending_input = bytearray()
    pending_signals = []
    exit_recorded = False
    cleanup_errors = []

    try:
        if os.isatty(stdin_fd):
            old_termios = termios.tcgetattr(stdin_fd)
            tty.setraw(stdin_fd, termios.TCSANOW)

        def record_signal(signum, _frame):
            pending_signals.append(signum)

        for signum in (signal.SIGHUP, signal.SIGINT, signal.SIGQUIT, signal.SIGTERM):
            old_handlers[signum] = signal.signal(signum, record_signal)

        child_pid, master_fd = pty.fork()
        if child_pid == 0:
            try:
                set_terminal_size(sys.stdin.fileno(), outer_size)
                os.execv(real_command, command_argv)
            except OSError as err:
                os.write(2, ("PTY recorder: exec failed: " + str(err) + "\n").encode())
                os._exit(127)

        master_open = True
        os.set_blocking(master_fd, False)
        set_terminal_size(master_fd, outer_size)
        env = {name: os.environ[name] for name in SAFE_ENV_NAMES if name in os.environ}
        trace.emit(
            "start",
            format="pty-trace-v1",
            child_pid=child_pid,
            command_name=Path(command_argv[0]).name,
            executable_name=Path(real_command).name,
            argv_count=len(command_argv) - 1,
            argv_options=option_names(command_argv[1:]),
            environment=env,
            outer_tty=os.isatty(stdin_fd),
        )
        if outer_size is not None:
            trace.emit("window_size", rows=outer_size[0], cols=outer_size[1], reason="initial")

        while master_open or child_status is None:
            while pending_signals:
                signum = pending_signals.pop(0)
                try:
                    trace.emit("signal", name=signal.Signals(signum).name)
                except ValueError:
                    trace.emit("signal", number=signum)
                if child_status is None:
                    try:
                        os.killpg(child_pid, signum)
                    except ProcessLookupError:
                        pass

            if child_status is None:
                waited, status = os.waitpid(child_pid, os.WNOHANG)
                if waited == child_pid:
                    child_status = status

            read_fds = []
            if master_open:
                read_fds.append(master_fd)
            if stdin_open:
                read_fds.append(stdin_fd)
            write_fds = [master_fd] if master_open and pending_input else []

            try:
                readable, writable, _ = select.select(read_fds, write_fds, [], 0.1)
            except InterruptedError:
                continue

            if stdin_fd in readable:
                try:
                    data = os.read(stdin_fd, 65536)
                except OSError as err:
                    if err.errno not in (errno.EIO, errno.EBADF):
                        raise
                    data = b""
                if data:
                    pending_input.extend(data)
                else:
                    stdin_open = False

            if master_open and master_fd in writable:
                try:
                    written = os.write(master_fd, pending_input)
                    if written > 0:
                        data = bytes(pending_input[:written])
                        trace.emit("io", direction="to_command", bytes=written,
                                   data_b64=base64.b64encode(data).decode("ascii"))
                    del pending_input[:written]
                except BlockingIOError:
                    pass
                except OSError as err:
                    if err.errno in (errno.EIO, errno.EBADF):
                        master_open = False
                    else:
                        raise

            if master_open and master_fd in readable:
                try:
                    data = os.read(master_fd, 65536)
                except OSError as err:
                    if err.errno not in (errno.EIO, errno.EBADF):
                        raise
                    data = b""
                if data:
                    trace.emit("io", direction="from_command", bytes=len(data),
                               data_b64=base64.b64encode(data).decode("ascii"))
                    write_all(stdout_fd, data)
                else:
                    master_open = False

            size = terminal_size(stdin_fd)
            if size is not None and size != outer_size:
                outer_size = size
                if master_open:
                    set_terminal_size(master_fd, size)
                trace.emit("window_size", rows=size[0], cols=size[1], reason="changed")

        status = child_status
        fields = wait_status_fields(status)
        trace.emit("exit", **fields)
        exit_recorded = True
        if "exit_code" in fields:
            return fields["exit_code"]
        if "signal_number" in fields:
            return min(128 + fields["signal_number"], 255)
        return 1
    finally:
        failure = sys.exc_info()[1]
        if old_termios is not None:
            try:
                termios.tcsetattr(stdin_fd, termios.TCSANOW, old_termios)
            except OSError as err:
                cleanup_errors.append(("terminal restore", err))

        if failure is not None:
            try:
                trace.emit(
                    "relay_error",
                    error_type=type(failure).__name__,
                    errno=getattr(failure, "errno", None),
                )
            except OSError as err:
                cleanup_errors.append(("record relay error", err))

        if child_pid and child_status is None:
            try:
                os.killpg(child_pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            except OSError as err:
                cleanup_errors.append(("send SIGTERM", err))
            try:
                deadline = time.monotonic() + 0.5
                while child_status is None and time.monotonic() < deadline:
                    try:
                        waited, status = os.waitpid(child_pid, os.WNOHANG)
                    except InterruptedError:
                        continue
                    if waited == child_pid:
                        child_status = status
                        break
                    time.sleep(0.02)
            except ChildProcessError:
                pass
            except OSError as err:
                cleanup_errors.append(("wait for child", err))
            if child_status is None:
                try:
                    os.killpg(child_pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                except OSError as err:
                    cleanup_errors.append(("send SIGKILL", err))
                try:
                    waited, status = os.waitpid(child_pid, os.WNOHANG)
                    if waited == child_pid:
                        child_status = status
                except ChildProcessError:
                    pass
                except OSError as err:
                    cleanup_errors.append(("reap child", err))

        if child_pid and not exit_recorded:
            fields = wait_status_fields(child_status) if child_status is not None else {}
            fields["aborted"] = failure is not None
            fields["reap_pending"] = child_status is None
            try:
                trace.emit("exit", **fields)
            except OSError as err:
                cleanup_errors.append(("record child exit", err))

        if master_fd >= 0:
            try:
                os.close(master_fd)
            except OSError as err:
                cleanup_errors.append(("close PTY", err))
        for signum, handler in old_handlers.items():
            signal.signal(signum, handler)
        try:
            trace_file.close()
        except OSError as err:
            cleanup_errors.append(("close trace", err))
        for action, err in cleanup_errors:
            print("PTY recorder: " + action + " failed: " + str(err), file=sys.stderr)


def create_trace_directory(shim_name=None):
    path = Path(tempfile.mkdtemp(prefix="pty-trace-"))
    bin_dir = None
    if shim_name is not None:
        bin_dir = path / "bin"
        bin_dir.mkdir(mode=0o700)
        shim_path = bin_dir / shim_name
        shim_source = (
            "#!/bin/sh\nexec "
            + shlex.quote(sys.executable)
            + " "
            + shlex.quote(str(Path(__file__).resolve()))
            + " --_pty_shim "
            + shlex.quote(shim_name)
            + ' "$@"\n'
        )
        fd = os.open(shim_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o700)
        with os.fdopen(fd, "w", encoding="utf-8") as shim_file:
            shim_file.write(shim_source)
    readme = (
        "PTY trace\n\n"
        "events-*.jsonl contains timestamped records. Each io record has a "
        "direction and base64-encoded raw bytes: to_command is input written "
        "to the command's PTY; from_command is output read from it. Resize "
        "and process lifecycle events are recorded in the same sequence.\n\n"
        "Direct mode runs one command in a PTY. Shim mode prepends a private "
        "PATH directory so an outer command can launch the selected command "
        "through this recorder.\n\n"
        "This records observable PTY traffic, not a command's internal state "
        "or an outer terminal emulator's in-memory cell snapshots; raw output "
        "can be replayed to reconstruct terminal state.\n\n"
        "Traces can contain private input and output. Review before sharing. "
        "Only terminal/locale environment values and command option names are "
        "recorded as metadata; full arguments and other environment values "
        "are not.\n"
    )
    (path / "README.txt").write_text(readme, encoding="utf-8")
    return path, bin_dir


def resolve_command(command, search_path):
    resolved = shutil.which(command, path=search_path)
    if not resolved:
        return None
    return str(Path(resolved).resolve())


def usage():
    print(
        "Usage:\n"
        "  python3 record-pty.py -- COMMAND [ARGS...]\n"
        "  python3 record-pty.py --shim NAME -- OUTER_COMMAND [ARGS...]\n\n"
        "Example for Claude through OSM:\n"
        "  python3 hack/record-pty.py --shim claude -- osm ai-tool -- "
        "-t claude -p PROVIDER -m MODEL -- --resume",
        file=sys.stderr,
    )


def launch():
    args = sys.argv[1:]
    shim_mode = bool(args and args[0] == "--shim")
    if shim_mode:
        if len(args) < 4 or args[2] != "--":
            usage()
            return 2
        shim_name = args[1]
        outer_argv = args[3:]
        if (
            not shim_name
            or shim_name in (".", "..")
            or os.path.basename(shim_name) != shim_name
        ):
            print("PTY recorder: shim name must be a command name, not a path", file=sys.stderr)
            return 2
        command_argv = None
    elif args and args[0] == "--" and len(args) > 1:
        shim_name = None
        command_argv = args[1:]
        outer_argv = None
    else:
        usage()
        return 2

    original_path = os.environ.get("PATH", "")
    command_name = shim_name if shim_mode else command_argv[0]
    real_command = resolve_command(command_name, original_path)
    if not real_command:
        print("PTY recorder: command was not found: " + command_name, file=sys.stderr)
        return 127

    if shim_mode:
        outer_command = resolve_command(outer_argv[0], original_path)
        if not outer_command:
            print("PTY recorder: outer command was not found: " + outer_argv[0], file=sys.stderr)
            return 127

    trace_dir, bin_dir = create_trace_directory(shim_name)
    print("PTY trace: " + str(trace_dir), file=sys.stderr)
    if not shim_mode:
        return run_pty_proxy(real_command, command_argv, trace_dir)

    environment = os.environ.copy()
    environment["PATH"] = str(bin_dir) + os.pathsep + original_path
    environment[TRACE_DIR_ENV] = str(trace_dir)
    environment[REAL_COMMAND_ENV] = real_command
    os.execvpe(outer_command, outer_argv, environment)
    return 127


def run_shim(target_name, command_args):
    real_command = os.environ.get(REAL_COMMAND_ENV)
    trace_dir = os.environ.get(TRACE_DIR_ENV)
    if not real_command or not trace_dir:
        print("PTY recorder: shim configuration is incomplete", file=sys.stderr)
        return 126
    command_argv = [target_name, *command_args]
    return run_pty_proxy(real_command, command_argv, trace_dir)


args = sys.argv[1:]
if args and args[0] == "--_pty_shim":
    if len(args) < 2:
        print("PTY recorder: shim name is missing", file=sys.stderr)
        raise SystemExit(126)
    raise SystemExit(run_shim(args[1], args[2:]))
raise SystemExit(launch())
