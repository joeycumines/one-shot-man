#!/usr/bin/env python3
"""Record the first bytes osm ai-tool sends to its terminal, from process start,
with the terminal answering OSC 11 the way Hana's light terminal does."""
import fcntl, os, pty, select, signal, struct, sys, termios, time
out = sys.argv[1]; seconds = float(sys.argv[2])
argv = ["osm", "ai-tool", "--", "-t", "claude"]
pid, fd = pty.fork()
if pid == 0:
    os.chdir("/Users/joeyc/dev/one-shot-man-2")
    os.environ.setdefault("TERM", "xterm-256color"); os.environ["COLORTERM"] = "truecolor"
    os.environ.pop("TMUX", None)
    os.execvp(argv[0], argv)
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))
buf = bytearray(); pending = b""
t = time.time() + seconds
while time.time() < t:
    r, _, _ = select.select([fd], [], [], 0.1)
    if not r: continue
    try: c = os.read(fd, 65536)
    except OSError: break
    if not c: break
    buf += c; pending += c
    reply = b""
    if b"\x1b]11;?" in pending: reply += b"\x1b]11;rgb:ffff/ffff/ffff\x07"
    if b"\x1b]10;?" in pending: reply += b"\x1b]10;rgb:0000/0000/0000\x07"
    if b"\x1b[6n" in pending: reply += b"\x1b[1;1R"
    if reply:
        try: os.write(fd, reply)
        except OSError: break
    pending = pending[-128:]
try: os.kill(pid, signal.SIGKILL)
except ProcessLookupError: pass
try: os.waitpid(pid, 0)
except ChildProcessError: pass
open(out, "wb").write(bytes(buf))
print("bytes=%d" % len(buf))
