#!/usr/bin/env python3
"""Drive `osm ai-tool` from process start in a light-answering pty, select a
tool with the keyboard, and record every byte. Also records the picker's exit
and the child's alt-screen entry, so the gap between them can be inspected.
"""
import fcntl, os, pty, select, signal, struct, sys, termios, time
out = sys.argv[1]; seconds = float(sys.argv[2]); rights = int(sys.argv[3])
argv = ["osm", "ai-tool"]
pid, fd = pty.fork()
if pid == 0:
    os.chdir("/Users/joeyc/dev/one-shot-man-2")
    os.environ.setdefault("TERM", "xterm-256color"); os.environ["COLORTERM"] = "truecolor"
    os.environ.pop("TMUX", None)
    os.execvp(argv[0], argv)
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 120, 0, 0))
buf = bytearray(); pending = b""; sent = False; key_time = None
t = time.time() + seconds
while time.time() < t:
    r, _, _ = select.select([fd], [], [], 0.1)
    if r:
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
    if not sent and b"Status: ready" in bytes(buf):
        time.sleep(0.5)
        try: os.write(fd, b"\x1b[C" * rights + b"\r")
        except OSError: break
        sent = True; key_time = time.time()
    if sent and key_time and time.time() - key_time > seconds - 6: break
try: os.kill(pid, signal.SIGKILL)
except ProcessLookupError: pass
try: os.waitpid(pid, 0)
except ChildProcessError: pass
d = bytes(buf); open(out, "wb").write(d)
print("bytes=%d launched=%s" % (len(d), sent))
