#!/usr/bin/env python3
"""A/B the real launcher under a pty that answers OSC 11 (and OSC 10).

Usage: ab-run.py <mode> <outfile> <script> <seconds> [flags...]
Invoked as: osm script <script> <flags...>
"""
import fcntl, os, pty, select, signal, struct, sys, termios, time

mode, outfile, script = sys.argv[1], sys.argv[2], sys.argv[3]
seconds = float(sys.argv[4]); flags = sys.argv[5:]
LIGHT, DARK = b"rgb:ffff/ffff/ffff", b"rgb:0000/0000/0000"

argv = ["osm", "script", script] + flags
pid, fd = pty.fork()
if pid == 0:
    os.chdir("/Users/joeyc/dev/one-shot-man-2")
    os.environ.setdefault("TERM", "xterm-256color")
    os.environ["COLORTERM"] = "truecolor"
    os.environ.pop("TMUX", None)
    os.execvp(argv[0], argv)
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))

captured = bytearray(); pending = b""
deadline = time.time() + seconds
while time.time() < deadline:
    r, _, _ = select.select([fd], [], [], 0.1)
    if r:
        try: chunk = os.read(fd, 65536)
        except OSError: break
        if not chunk: break
        captured += chunk; pending += chunk
        reply = b""
        if mode != "none":
            rgb = LIGHT if mode == "light" else DARK
            if b"\x1b]11;?" in pending: reply += b"\x1b]11;" + rgb + b"\x07"
            if b"\x1b]10;?" in pending: reply += b"\x1b]10;" + rgb + b"\x07"
        if b"\x1b[6n" in pending: reply += b"\x1b[1;1R"
        if reply:
            try: os.write(fd, reply)
            except OSError: break
        pending = pending[-128:]
try: os.kill(pid, signal.SIGKILL)
except ProcessLookupError: pass
try: os.waitpid(pid, 0)
except ChildProcessError: pass
data = bytes(captured)
open(outfile, "wb").write(data)
print("mode=%s bytes=%d script=%s" % (mode, len(data), script))
