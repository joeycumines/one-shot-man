#!/usr/bin/env python3
"""Answer a TUI child's terminal handshake in a pty and record its render.

Usage: osc-handshake-probe.py <mode> <outfile> [seconds]

Modes (what the OSC 10 / OSC 11 queries are answered with):
  both-light   OSC 10 + OSC 11 -> rgb:ffff/ffff/ffff
  osc11-light  OSC 11 only     -> rgb:ffff/ffff/ffff   (no OSC 10 reply)
  both-dark    OSC 10 + OSC 11 -> rgb:0000/0000/0000
  none         nothing

Everything else the child probes (CPR, DA1/DA2, pixel size, XTVERSION, kitty
keyboard flags, DECRQM, tmux passthrough) is answered identically in every
mode, so any difference in the rendered palette is attributable to the OSC
10/11 replies alone.
"""
import fcntl
import os
import pty
import re
import select
import signal
import struct
import sys
import termios
import time

mode = sys.argv[1]
outfile = sys.argv[2]
seconds = float(sys.argv[3]) if len(sys.argv) > 3 else 10.0

LIGHT = b"rgb:ffff/ffff/ffff"
DARK = b"rgb:0000/0000/0000"


def answers_for(buf):
    out = b""
    if mode != "none" and b"\x1b]11;?" in buf:
        out += b"\x1b]11;" + (DARK if mode == "both-dark" else LIGHT) + b"\x07"
    if mode == "both-light" and b"\x1b]10;?" in buf:
        out += b"\x1b]10;" + LIGHT + b"\x07"
    if mode == "both-dark" and b"\x1b]10;?" in buf:
        out += b"\x1b]10;" + DARK + b"\x07"
    # Cursor position report — the child blocks on this to seed its cursor.
    if b"\x1b[6n" in buf:
        out += b"\x1b[1;1R"
    # Pixel size (CSI 14 t).
    if b"\x1b[14t" in buf:
        out += b"\x1b[4;480;1280t"
    # Primary/secondary device attributes.
    if b"\x1b[c" in buf and b"\x1b[?1;2c" not in buf:
        out += b"\x1b[?62;22c"
    if b"\x1b[>c" in buf:
        out += b"\x1b[>0;276;0c"
    # XTVERSION.
    if b"\x1b[>0q" in buf:
        out += b"\x1bP>|probe 1.0\x1b\\"
    # Kitty keyboard protocol flags query.
    if b"\x1b[?u" in buf:
        out += b"\x1b[?0u"
    # DECRQM: report every queried mode as "not recognised" (0), which is the
    # honest answer from a terminal that implements none of them.
    for m in re.finditer(rb"\x1b\[\?(\d+)\$p", buf):
        out += b"\x1b[?" + m.group(1) + b";0$y"
    # tmux passthrough DCS: unwrap and answer the inner query too.
    for m in re.finditer(rb"\x1bPtmux;((?:\x1b\x1b|\x1b[^\x1b])*)\x1b\\", buf):
        inner = m.group(1).replace(b"\x1b\x1b", b"\x1b")
        out += b"\x1bPtmux;" + answers_for(inner).replace(b"\x1b", b"\x1b\x1b") + b"\x1b\\"
    return out


pid, fd = pty.fork()
if pid == 0:
    os.environ.setdefault("TERM", "xterm-256color")
    os.environ["COLORTERM"] = "truecolor"
    os.environ.pop("TMUX", None)
    os.environ.pop("TERM_PROGRAM", None)
    os.execvp("opencode", ["opencode"])

fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 100, 0, 0))

captured = bytearray()
pending = b""
deadline = time.time() + seconds
while time.time() < deadline:
    r, _, _ = select.select([fd], [], [], 0.2)
    if not r:
        continue
    try:
        chunk = os.read(fd, 65536)
    except OSError:
        break
    if not chunk:
        break
    captured += chunk
    pending += chunk
    reply = answers_for(pending)
    if reply:
        try:
            os.write(fd, reply)
        except OSError:
            break
    pending = pending[-256:]

try:
    os.kill(pid, signal.SIGKILL)
except ProcessLookupError:
    pass
os.waitpid(pid, 0)

data = bytes(captured)
with open(outfile, "wb") as fh:
    fh.write(data)


def palette(prefix):
    counts = {}
    for m in re.finditer(rb"\x1b\[" + prefix + rb";2;(\d+);(\d+);(\d+)m", data):
        counts[m.group(0)] = counts.get(m.group(0), 0) + 1
    return counts


bgs = palette(b"48")
fgs = palette(b"38")
print("mode=%s bytes=%d" % (mode, len(data)))
print("  distinct truecolor backgrounds: %d" % len(bgs))
for k, v in sorted(bgs.items(), key=lambda kv: -kv[1])[:8]:
    print("    %s x%d" % (k.decode(), v))
print("  distinct truecolor foregrounds: %d" % len(fgs))
for k, v in sorted(fgs.items(), key=lambda kv: -kv[1])[:8]:
    print("    %s x%d" % (k.decode(), v))
print("  osc10-queries=%d osc11-queries=%d" % (
    data.count(b"\x1b]10;?"), data.count(b"\x1b]11;?")))
